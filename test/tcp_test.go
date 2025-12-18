package mtmux

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/flben233/mtmux"
)

func TestTCPTransport(t *testing.T) {
	go func() {
		cfg := mtmux.DefaultConfig()
		ln, err := mtmux.Listen("tcp", "127.0.0.1:12345", int(cfg.Tunnels))
		if err != nil {
			t.Errorf("Listen returned error: %v", err)
			return
		}
		defer ln.Close()
		bundle, err := ln.Accept()
		if err != nil {
			t.Errorf("Accept returned error: %v", err)
			return
		}
		fmt.Println(len(bundle), "Connection(s) established.")
		session, err := mtmux.Server(bundle, cfg)
		if err != nil {
			t.Errorf("Server returned error: %v", err)
			return
		}
		fmt.Println("server", session.ID)
		session.Start(context.Background())
		defer session.Close()
		time.Sleep(100 * time.Millisecond)
		stream, err := session.AcceptStream()
		if err != nil {
			t.Errorf("AcceptStream returned error: %v", err)
			return
		}
		fmt.Println("AcceptStream success")
		stream.Write([]byte("hello"))
		buf := make([]byte, 1024)
		n, err := stream.Read(buf)
		if err != nil {
			t.Errorf("Read returned error: %v", err)
			return
		}
		if string(buf[:n]) != "world" {
			t.Errorf("unexpected data: %q", buf[:n])
		}
		fmt.Println(string(buf[:n]))
		time.Sleep(10000 * time.Millisecond)
	}()
	fmt.Println("Test start")
	time.Sleep(1000 * time.Millisecond)
	cfg := mtmux.DefaultConfig()
	bundle, err := mtmux.Dial("tcp", "127.0.0.1:12345", int(cfg.Tunnels))
	if err != nil {
		t.Fatalf("Dial returned error: %v", err)
	}
	session, err := mtmux.Client(bundle, cfg)
	if err != nil {
		t.Fatalf("Client returned error: %v", err)
	}
	session.Start(context.Background())
	fmt.Println("client", session.ID)
	defer session.Close()
	stream, err := session.OpenStream()
	if err != nil {
		t.Fatalf("OpenStream returned error: %v", err)
	}
	buf := make([]byte, 1024)
	n, err := stream.Read(buf)
	if err != nil {
		t.Fatalf("Read returned error: %v", err)
	}
	if string(buf[:n]) != "hello" {
		t.Fatalf("unexpected data: %q", buf[:n])
	}
	fmt.Println(string(buf[:n]))
	_, err = stream.Write([]byte("world"))
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	time.Sleep(1000 * time.Millisecond)
}

func TestMultiStreamsTCPTransport(t *testing.T) {
	go func() {
		cfg := mtmux.DefaultConfig()
		ln, err := mtmux.Listen("tcp", "127.0.0.1:12345", int(cfg.Tunnels))
		if err != nil {
			t.Errorf("Listen returned error: %v", err)
			return
		}
		defer ln.Close()
		bundle, err := ln.Accept()
		if err != nil {
			t.Errorf("Accept returned error: %v", err)
			return
		}
		fmt.Println(len(bundle), "Connection(s) established.")
		session, err := mtmux.Server(bundle, cfg)
		if err != nil {
			t.Errorf("Server returned error: %v", err)
			return
		}
		fmt.Println("server", session.ID)
		session.Start(context.Background())
		defer session.Close()
		time.Sleep(100 * time.Millisecond)
		for i := 0; i < 8; i++ {
			stream, err := session.AcceptStream()
			if err != nil {
				t.Errorf("AcceptStream returned error: %v", err)
				return
			}
			fmt.Println("AcceptStream success")
			go func(stream *mtmux.Stream) {
				stream.Write([]byte(stream.ID + " hello"))
				buf := make([]byte, 1024)
				n, err := stream.Read(buf)
				if err != nil {
					t.Errorf("Read returned error: %v", err)
					return
				}
				if string(buf[:n]) != stream.ID+" world" {
					t.Errorf("unexpected data: %q", buf[:n])
				}
				fmt.Println(string(buf[:n]))
			}(stream.(*mtmux.Stream))
		}
		time.Sleep(10000 * time.Millisecond)
	}()
	fmt.Println("Test start")
	time.Sleep(1000 * time.Millisecond)
	cfg := mtmux.DefaultConfig()
	bundle, err := mtmux.Dial("tcp", "127.0.0.1:12345", int(cfg.Tunnels))
	if err != nil {
		t.Errorf("Dial returned error: %v", err)
	}
	session, err := mtmux.Client(bundle, cfg)
	if err != nil {
		t.Errorf("Client returned error: %v", err)
	}
	session.Start(context.Background())
	fmt.Println("client", session.ID)
	defer session.Close()
	for i := 0; i < 8; i++ {
		go func() {
			stream, err := session.OpenStream()
			if err != nil {
				t.Errorf("OpenStream returned error: %v", err)
			}
			go func(stream *mtmux.Stream) {
				buf := make([]byte, 1024)
				n, err := stream.Read(buf)
				if err != nil {
					t.Errorf("Read returned error: %v", err)
				}
				if string(buf[:n]) != stream.ID+" hello" {
					t.Errorf("unexpected data: %q", buf[:n])
				}
				fmt.Println(string(buf[:n]))
				_, err = stream.Write([]byte(stream.ID + " world"))
				if err != nil {
					t.Errorf("Write returned error: %v", err)
				}
				time.Sleep(1000 * time.Millisecond)
			}(stream.(*mtmux.Stream))
		}()
	}
	time.Sleep(10000 * time.Millisecond)
}
