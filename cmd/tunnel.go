package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/flben233/mtmux"
)

func streamCloseWait(stream *mtmux.Stream) {
	time.Sleep(30 * time.Second)
	if !stream.IsClosed() {
		stream.Close()
	}
}

func startServerSide(listenPort uint16, tunnels uint8) {
	mtmux.Infof("Server side tunnel started at 0.0.0.0:%d", listenPort)
	cfg := mtmux.DefaultConfig()
	if tunnels > 0 {
		cfg.Tunnels = int32(tunnels)
	}
	ln, err := mtmux.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", listenPort), int(cfg.Tunnels))
	if err != nil {
		mtmux.Error("Listen returned error: %v", err)
		return
	}
	defer ln.Close()

	bundle, err := ln.Accept()
	if err != nil {
		mtmux.Error("Accept returned error: %v", err)
		return
	}
	mtmux.Info(len(bundle), "Connection(s) established.")

	session, err := mtmux.Server(bundle, cfg)
	if err != nil {
		mtmux.Error("Server returned error: %v", err)
		return
	}
	mtmux.Info("server", session.ID)
	session.Start(context.Background())
	defer session.Close()

	for {
		stream, err := session.AcceptStream()
		if err != nil {
			mtmux.Error("AcceptStream returned error: %v", err)
			return
		}
		mtmux.Info("AcceptStream success")

		go func(stream *mtmux.Stream) {
			defer streamCloseWait(stream)
			reader := bufio.NewReader(stream)
			addrLine, err := reader.ReadString('\n')
			if err != nil {
				mtmux.Error("Read addr from stream error: %v", err)
				return
			}
			addr := strings.TrimSpace(addrLine)
			mtmux.Info("requested addr: %s", addr)

			conn, err := net.Dial("tcp", addr)
			if err != nil {
				mtmux.Error("Dial returned error: %v", err)
				return
			}
			mtmux.Info("Connected to target %s (local=%s remote=%s)", addr, conn.LocalAddr(), conn.RemoteAddr())

			defer conn.Close()
			var wg sync.WaitGroup
			wg.Add(2)

			// Copy from stream to target conn (upload direction)
			go func() {
				defer wg.Done()
				n, err := io.Copy(conn, reader)
				if err != nil {
					mtmux.Error("stream->conn copy error: %v", err)
				}
				mtmux.Info("Stream closed:", stream.IsClosed())
				conn.(*net.TCPConn).CloseWrite()
				mtmux.Info("Exit server copy from stream to conn (copied %d bytes)", n)
			}()

			// Copy from target conn to stream (download direction - results go back this way)
			go func() {
				defer wg.Done()
				n, err := io.Copy(stream, conn)
				if err != nil && err != io.EOF {
					mtmux.Error("conn->stream copy error: %v", err)
				}
				stream.CloseWrite()
				conn.(*net.TCPConn).CloseRead()
				mtmux.Info("Stream closed:", stream.IsClosed())
				mtmux.Info("Exit server copy from conn to stream (copied %d bytes)", n)
			}()

			wg.Wait()
		}(stream)
	}
}

func startClientSide(server string, tunnels uint8, localListenPort uint16, target string) {
	mtmux.Info("Test start")
	time.Sleep(1000 * time.Millisecond)

	cfg := mtmux.DefaultConfig()
	if tunnels > 0 {
		cfg.Tunnels = int32(tunnels)
	}
	bundle, err := mtmux.Dial("tcp", server, int(cfg.Tunnels))
	if err != nil {
		mtmux.Error("Dial returned error: %v", err)
		return
	}
	session, err := mtmux.Client(bundle, cfg)
	if err != nil {
		mtmux.Error("Client returned error: %v", err)
		return
	}
	session.Start(context.Background())
	mtmux.Info("client", session.ID)

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", localListenPort))
	if err != nil {
		mtmux.Error("Listen returned error: %v", err)
		return
	}
	defer ln.Close()
	mtmux.Info("listening 127.0.0.1:%d", localListenPort)

	for {
		conn, err := ln.Accept()
		if err != nil {
			mtmux.Error("Accept returned error: %v", err)
			continue
		}
		stream, err := session.OpenStream()
		if err != nil {
			mtmux.Error("OpenStream returned error: %v", err)
			return
		}

		go func(localConn net.Conn, stream *mtmux.Stream) {
			defer localConn.Close()
			defer streamCloseWait(stream)

			time.Sleep(1000 * time.Millisecond)
			_, err = stream.Write([]byte(target + "\n"))
			if err != nil {
				mtmux.Error("write addr error: %v", err)
				return
			}
			mtmux.Info("wrote target %s to stream", strings.TrimSpace(target))

			var wg sync.WaitGroup
			wg.Add(2)

			// Copy from local to stream (upload direction)
			go func() {
				defer wg.Done()
				n, err := io.Copy(stream, localConn)
				if err != nil {
					mtmux.Error("local->stream copy error: %v", err)
				}
				// Close write end to signal EOF to remote
				stream.CloseWrite()
				localConn.(*net.TCPConn).CloseRead()
				mtmux.Info("Stream closed:", stream.IsClosed())
				mtmux.Info("Exit client copy from local to stream (copied %d bytes)", n)
			}()

			// Copy from stream to local (download direction - results come back this way)
			go func() {
				defer wg.Done()
				n, err := io.Copy(localConn, stream)
				if err != nil && err != io.EOF {
					mtmux.Error("stream->local copy error: %v", err)
				}
				localConn.(*net.TCPConn).CloseWrite()
				mtmux.Info("Stream closed:", stream.IsClosed())
				mtmux.Info("Exit client copy from stream to local (copied %d bytes)", n)
			}()

			wg.Wait()
			mtmux.Info("connection proxy finished")
		}(conn, stream)
	}
}

func main() {
	mode := flag.String("m", "", "Mode: server or client")
	listenPort := flag.Uint("p", 0, "Port to listen on (server mode) or local port to listen on (client mode)")
	serverAddr := flag.String("s", "", "Server address (client mode), e.g., 127.0.0.1:17000")
	target := flag.String("d", "", "Target address to connect to (client mode), e.g., 127.0.0.1:5201")
	tunnels := flag.Uint("t", 8, "Number of tunnels to use, default 8")
	flag.Parse()
	switch *mode {
	case "server":
		if *listenPort == 0 {
			fmt.Println("Please specify a valid listen port with -p")
			return
		}
		go startServerSide(uint16(*listenPort), uint8(*tunnels))
	case "client":
		if *serverAddr == "" {
			fmt.Println("Please specify a valid server address with -s")
			return
		}
		if *listenPort == 0 {
			fmt.Println("Please specify a valid local listen port with -p")
			return
		}
		go startClientSide(*serverAddr, uint8(*tunnels), uint16(*listenPort), *target)
	default:
		fmt.Println("Please specify a valid mode with -m: server or client")
		return
	}
	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan // 阻塞直到收到信号
}
