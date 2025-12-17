package mtmux

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

type Session struct {
	ID         string
	connChan   chan net.Conn
	connBundle []net.Conn
	streams    map[string]*Stream
	config     *Config
	acceptance chan *Stream
	isClosed   bool
}

type Frame struct {
	StreamID  string
	Data      []byte
	DataIndex uint64
}

type ControlMsg struct {
	Type string
	Data string
}

const (
	CONTROL_STREAM_ID        = "0"
	CONTROL_TYPE_KEEP_ALIVE  = "KEEP_ALIVE"
	CONTROL_TYPE_OPEN_STREAM = "OPEN_STREAM"
)

func newSession(conns []net.Conn, config *Config) *Session {
	connChannel := make(chan net.Conn, len(conns))
	for _, conn := range conns {
		connChannel <- conn
	}
	return &Session{
		ID:         rand.Text(),
		connChan:   connChannel,
		connBundle: conns,
		streams:    make(map[string]*Stream),
		config:     config,
		acceptance: make(chan *Stream, 1),
	}
}

func Server(conns []net.Conn, config *Config) (*Session, error) {
	return newSession(conns, config), nil
}

func Client(conns []net.Conn, config *Config) (*Session, error) {
	return newSession(conns, config), nil
}

func (s *Session) newStream(streamID string) *Stream {
	stream := Stream{
		ID:            streamID,
		ReadBuf:       make(chan []byte),
		WriteBuf:      make(chan []byte),
		ReadDeadline:  time.Time{},
		WriteDeadline: time.Time{},
	}
	s.streams[streamID] = &stream
	return &stream
}

// handleConn handles incoming data from a single connection in the bundle.
func (s *Session) handleConn(ctx context.Context, conn net.Conn) {
	scanner := bufio.NewReader(conn)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		data, err := scanner.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			} else if errors.Is(err, net.ErrClosed) {
				return
			}
			fmt.Println("Error reading from connection:", err)
			return
		}
		if len(data) == 0 {
			continue
		}
		// Process the received frame
		var f Frame
		json.Unmarshal([]byte(data), &f)
		stream, ok := s.streams[f.StreamID]
		if !ok {
			continue
		}
		for totalWait := 0 * time.Millisecond; f.DataIndex > stream.ReadIndex.Load()+1; {
			// Wait for missing data
			time.Sleep(1 * time.Millisecond)
			totalWait += 1 * time.Millisecond
			if totalWait > s.config.Timeout {
				fmt.Println("Timeout waiting for missing data.")
				return
			}
		}
		// Deliver data to the appropriate stream
		stream.ReadBuf <- f.Data
		// Update the read index
		stream.ReadIndex.Store(f.DataIndex)
	}
}

// handleStream handles outgoing data for a single stream.
func (s *Session) handleStream(ctx context.Context, streamID string) {
	stream := s.streams[streamID]
	for {
		select {
		case <-ctx.Done():
			return
		case data := <-stream.WriteBuf:
			f := Frame{
				StreamID:  streamID,
				Data:      data,
				DataIndex: stream.WriteIndex.Load() + 1,
			}
			frameData, _ := json.Marshal(f)
			frameData = append(frameData, '\n')
			// Write to one of the connections in the bundle
			for {
				conn := <-s.connChan
				_, err := conn.Write(frameData)
				if err == nil {
					s.connChan <- conn
					stream.WriteIndex.Store(f.DataIndex)
					break
				} else {
					if errors.Is(err, net.ErrClosed) {
						continue
					}
					if len(s.connChan) == 0 {
						fmt.Println("All connections are closed. Stream handler exiting.")
						return
					}
					fmt.Println("Error writing to connection:", err)
				}
			}
		}
	}
}

func (s *Session) sendControlMsg(msg *ControlMsg) error {
	msgData, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	s.streams[CONTROL_STREAM_ID].Write(msgData)
	return nil
}

// handleControlMsg processes incoming control messages on the control stream.
func (s *Session) handleControlMsg(ctx context.Context) {
	buf := make([]byte, 16384)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		// Read control message
		n, err := s.streams[CONTROL_STREAM_ID].Read(buf)
		if err != nil {
			fmt.Println("Error reading control message:", err)
			return
		}
		var msg ControlMsg
		json.Unmarshal(buf[:n], &msg)
		switch msg.Type {
		case CONTROL_TYPE_OPEN_STREAM:
			streamID := msg.Data
			stream := s.newStream(streamID)
			s.acceptance <- stream
		}
	}
}

func (s *Session) keepAlive(ctx context.Context) {
	for {
		// Send keep-alive frames on the control stream
		msg := ControlMsg{
			Type: CONTROL_TYPE_KEEP_ALIVE,
			Data: "",
		}
		s.sendControlMsg(&msg)
		// Sleep for a predefined interval before sending the next keep-alive
		select {
		case <-ctx.Done():
			return
		case <-time.After(s.config.KeepAliveInterval):
		}
	}
}

func (s *Session) Start(ctx context.Context) {
	s.newStream(CONTROL_STREAM_ID)
	for _, conn := range s.connBundle {
		go s.handleConn(ctx, conn)
	}
	for streamID := range s.streams {
		go s.handleStream(ctx, streamID)
	}
	go s.keepAlive(ctx)
	go s.handleControlMsg(ctx)
}

func (s *Session) Close() error {
	for _, conn := range s.connBundle {
		conn.Close()
	}
	for _, stream := range s.streams {
		stream.Close()
	}
	s.isClosed = true
	return nil
}

func (s *Session) IsClosed() bool {
	return s.isClosed
}

func (s *Session) NumStreams() int {
	return len(s.streams)
}

func (s *Session) OpenStream() (net.Conn, error) {
	streamID := rand.Text()
	stream := s.newStream(streamID)
	msg := ControlMsg{
		Type: CONTROL_TYPE_OPEN_STREAM,
		Data: streamID,
	}
	s.sendControlMsg(&msg)
	go s.handleStream(context.Background(), streamID)
	return stream, nil
}

func (s *Session) AcceptStream() (net.Conn, error) {
	stream := <-s.acceptance
	go s.handleStream(context.Background(), stream.ID)
	return stream, nil
}
