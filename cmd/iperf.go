package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"runtime/pprof"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/flben233/mtmux"
)

func startMTMuxServer() {
	cfg := mtmux.DefaultConfig()
	ln, err := mtmux.Listen("tcp", "127.0.0.1:16345", int(cfg.Tunnels))
	if err != nil {
		log.Printf("Listen returned error: %v", err)
		return
	}
	defer ln.Close()

	bundle, err := ln.Accept()
	if err != nil {
		log.Printf("Accept returned error: %v", err)
		return
	}
	log.Println(len(bundle), "Connection(s) established.")

	session, err := mtmux.Server(bundle, cfg)
	if err != nil {
		log.Printf("Server returned error: %v", err)
		return
	}
	log.Println("server", session.ID)
	session.Start(context.Background())
	defer session.Close()

	for {
		stream, err := session.AcceptStream()
		if err != nil {
			log.Printf("AcceptStream returned error: %v", err)
			return
		}
		log.Println("AcceptStream success")

		go func(stream *mtmux.Stream) {
			reader := bufio.NewReader(stream)
			defer stream.Close()
			addrLine, err := reader.ReadString('\n')
			if err != nil {
				log.Printf("Read addr from stream error: %v", err)
				stream.Close()
				return
			}
			addr := strings.TrimSpace(addrLine)
			log.Printf("requested addr: %s", addr)

			conn, err := net.Dial("tcp", addr)
			if err != nil {
				log.Printf("Dial returned error: %v", err)
				stream.Close()
				return
			}
			log.Printf("Connected to target %s (local=%s remote=%s)", addr, conn.LocalAddr(), conn.RemoteAddr())

			defer conn.Close()
			var wg sync.WaitGroup
			wg.Add(2)

			// Copy from stream to target conn (upload direction)
			go func() {
				defer wg.Done()
				n, err := io.Copy(conn, reader)
				if err != nil {
					log.Printf("stream->conn copy error: %v", err)
				}
				conn.(*net.TCPConn).CloseWrite()
				log.Printf("Exit server copy from stream to conn (copied %d bytes)", n)
			}()

			// Copy from target conn to stream (download direction - results go back this way)
			go func() {
				defer wg.Done()
				n, err := io.Copy(stream, conn)
				if err != nil && err != io.EOF {
					log.Printf("conn->stream copy error: %v", err)
				}
				stream.CloseWrite()
				conn.(*net.TCPConn).CloseRead()
				log.Printf("Exit server copy from conn to stream (copied %d bytes)", n)
			}()

			wg.Wait()
		}(stream)
	}
}

func startMTMuxClient() {
	log.Println("Test start")
	time.Sleep(1000 * time.Millisecond)

	cfg := mtmux.DefaultConfig()
	bundle, err := mtmux.Dial("tcp", "127.0.0.1:16345", int(cfg.Tunnels))
	if err != nil {
		log.Printf("Dial returned error: %v", err)
		return
	}
	session, err := mtmux.Client(bundle, cfg)
	if err != nil {
		log.Printf("Client returned error: %v", err)
		return
	}
	session.Start(context.Background())
	log.Println("client", session.ID)
	defer session.Close()

	ln, err := net.Listen("tcp", "127.0.0.1:15200")
	if err != nil {
		log.Printf("Listen returned error: %v", err)
		return
	}
	defer ln.Close()
	log.Println("listening 127.0.0.1:15200")

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("Accept returned error: %v", err)
			continue
		}
		stream, err := session.OpenStream()
		if err != nil {
			log.Printf("OpenStream returned error: %v", err)
			return
		}
		// defer stream.Close()

		go func(localConn net.Conn, stream *mtmux.Stream) {
			defer localConn.Close()
			defer stream.Close()

			time.Sleep(1000 * time.Millisecond)
			target := "127.0.0.1:5201\n"
			_, err = stream.Write([]byte(target))
			if err != nil {
				log.Printf("write addr error: %v", err)
				return
			}
			log.Printf("wrote target %s to stream", strings.TrimSpace(target))

			var wg sync.WaitGroup
			wg.Add(2)

			// Copy from local to stream (upload direction)
			go func() {
				defer wg.Done()
				n, err := io.Copy(stream, localConn)
				if err != nil {
					log.Printf("local->stream copy error: %v", err)
				}
				// Close write end to signal EOF to remote
				stream.CloseWrite()
				localConn.(*net.TCPConn).CloseRead()
				log.Printf("Exit client copy from local to stream (copied %d bytes)", n)
			}()

			// Copy from stream to local (download direction - results come back this way)
			go func() {
				defer wg.Done()
				n, err := io.Copy(localConn, stream)
				if err != nil && err != io.EOF {
					log.Printf("stream->local copy error: %v", err)
				}
				localConn.(*net.TCPConn).CloseWrite()
				log.Printf("Exit client copy from stream to local (copied %d bytes)", n)
			}()

			wg.Wait()
			log.Println("connection proxy finished")
		}(conn, stream)
	}
}

// copyWithLogging copies from src to dst with periodic progress logs.
// name is a descriptive tag for logs, peer is the remote address or role.
func copyWithLogging(dst io.Writer, src io.Reader, name, peer string) {
	buf := make([]byte, 32*1024)
	var total int64
	lastLog := time.Now()
	for {
		n, rerr := src.Read(buf)
		if n > 0 {
			written := 0
			for written < n {
				fmt.Println(name, "Copying", n, "bytes to", peer)
				wn, werr := dst.Write(buf[written:n])
				if wn > 0 {
					written += wn
					total += int64(wn)
				}
				if werr != nil {
					log.Printf("%s write error to %s: %v", name, peer, werr)
					return
				}
			}
		}
		now := time.Now()
		if now.Sub(lastLog) >= 2*time.Second {
			log.Printf("%s progress to %s: %d bytes", name, peer, total)
			lastLog = now
		}
		if rerr != nil {
			if rerr != io.EOF {
				log.Printf("%s read error from %s: %v", name, peer, rerr)
			}
			log.Printf("%s finished to %s: total %d bytes", name, peer, total)
			return
		}
	}
}

func main() {
	f, _ := os.Create("CPU.out")
	defer f.Close()
	pprof.StartCPUProfile(f)
	// defer pprof.StopCPUProfile()
	go startMTMuxServer()
	time.Sleep(2000 * time.Millisecond)
	go startMTMuxClient()
	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan // 阻塞直到收到信号
	// 收到信号后停止 profiling
	pprof.StopCPUProfile()
}
