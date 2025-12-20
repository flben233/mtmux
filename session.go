package mtmux

import (
	"bufio"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/goccy/go-json"
)

type Session struct {
	ID            string
	outConnChan   chan net.Conn
	connBundle    []net.Conn
	streams       sync.Map
	config        *Config
	streamOpen    chan string
	streamConfirm chan string
	isClosed      bool
	numStreams    atomic.Int64
	ctx           context.Context
}

type Frame struct {
	StreamID string
	// Data      []byte
	DataLen   uint64
	DataIndex uint64
	IsEOF     bool // Signals that the sender has closed the write end
}

type ControlMsg struct {
	Type string
	Data string
}

const (
	CONTROL_STREAM_ID        = "0"
	CONTROL_TYPE_OPEN_STREAM = "OPEN_STREAM"
	CONTROL_STREAM_CONFIRMED = "STREAM_CONFIRMED"
)

func newSession(conns []net.Conn, config *Config) *Session {
	connChannel := make(chan net.Conn, len(conns))
	for _, conn := range conns {
		connChannel <- conn
	}
	return &Session{
		ID:            rand.Text(),
		outConnChan:   connChannel,
		connBundle:    conns,
		config:        config,
		streamOpen:    make(chan string, 1),
		streamConfirm: make(chan string, 1),
	}
}

func Server(conns []net.Conn, config *Config) (*Session, error) {
	return newSession(conns, config), nil
}

func Client(conns []net.Conn, config *Config) (*Session, error) {
	return newSession(conns, config), nil
}

// inbound handles incoming data from a single connection in the bundle.
func (s *Session) inbound(ctx context.Context, conn net.Conn) {
	reader := bufio.NewReader(conn)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		data, err := reader.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			} else if errors.Is(err, net.ErrClosed) {
				return
			}
			fmt.Println("Error reading from connection:", err)
			return
		}
		// Process the received frame
		var f Frame
		json.Unmarshal([]byte(data), &f)
		streamAny, ok := s.streams.Load(f.StreamID)
		if !ok {
			fmt.Println("Received frame for unknown stream:", f.StreamID)
			continue
		}
		stream := streamAny.(*Stream)
		if stream.IsClosed() {
			fmt.Println("Received frame for closed stream:", f.StreamID)
			continue
		}

		//fmt.Println(s.ID, "Read from connection", string(f.Data))
		// if f.DataIndex != stream.ReadIndex.Load()+1 {
		// 	fmt.Println("Out of ordered frame. ", f.DataIndex, stream.ReadIndex.Load()+1)
		// }
		rawData := make([]byte, f.DataLen)
		_, err = io.ReadFull(reader, rawData)
		if err != nil {
			fmt.Println("Error reading frame data:", err)
			continue
		}

		err = stream.Deliver(rawData, f.DataIndex, f.IsEOF)
		if err != nil {
			if errors.Is(err, io.ErrClosedPipe) {
				s.streams.Delete(f.StreamID)
			} else {
				fmt.Println("Error delivering data to stream:", err)
			}
		}
	}
}

// outbound handles outgoing data for a single stream.
func (s *Session) outbound(ctx context.Context, stream *Stream) {
	for {
		select {
		case <-ctx.Done():
			return
		case dataBlock, ok := <-stream.WriteBuf:
			f := Frame{
				StreamID:  stream.ID,
				DataLen:   0,
				DataIndex: stream.WriteIndex.Add(1),
				IsEOF:     !ok,
			}
			conn := <-s.outConnChan
			if !ok {
				frameData, _ := json.MarshalNoEscape(f)
				frameData = append(frameData, '\n')
				_, err := conn.Write(frameData)
				if err != nil {
					fmt.Println("Error writing EOF frame to connection:", err)
				}
				s.outConnChan <- conn
				fmt.Println(stream.ID, "exited.")
				return
			} else {
				// Read data from stream's WriteBuf
				totalSize := len(dataBlock)
				originalData := dataBlock
				for totalSize > 0 {
					size := min(max(totalSize, 32*1024), totalSize)
					totalSize -= size
					data := dataBlock[:size]
					dataBlock = dataBlock[size:]
					f.DataLen = uint64(size)
					// Send frame over one of the connections in the bundle
					frameData, err := json.Marshal(f)
					if err != nil {
						fmt.Println("Error marshaling frame:", err)
						continue
					}
					frameData = append(frameData, '\n')
					frameData = append(frameData, data...)
					_, err = conn.Write(frameData)
					if err != nil {
						fmt.Println("Error writing frame to connection:", err)
					}
				}
				s.outConnChan <- conn
				stream.PutBuffer(originalData[:0])
			}
		}
	}
}

func (s *Session) streamsGarbageCollector(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		s.streams.Range(func(_ any, streamAny any) bool {
			stream := streamAny.(*Stream)
			if stream.IsClosed() {
				s.streams.Delete(stream.ID)
				fmt.Println("Stream", stream.ID, "garbage collected.")
			}
			return true
		})
		time.Sleep(10 * time.Millisecond)
	}
}

func (s *Session) sendControlMsg(msg *ControlMsg) error {
	msgData, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	streamAny, ok := s.streams.Load(CONTROL_STREAM_ID)
	if !ok {
		return fmt.Errorf("control stream not found")
	}
	stream := streamAny.(*Stream)
	_, err = stream.Write(msgData)
	return err
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
		streamAny, ok := s.streams.Load(CONTROL_STREAM_ID)
		if !ok {
			fmt.Println("control stream not found")
			return
		}
		n, err := streamAny.(*Stream).Read(buf)
		if err != nil {
			fmt.Println("Error reading control message:", err)
			return
		}
		var msg ControlMsg
		json.Unmarshal(buf[:n], &msg)
		switch msg.Type {
		case CONTROL_TYPE_OPEN_STREAM:
			s.streamOpen <- msg.Data
		case CONTROL_STREAM_CONFIRMED:
			s.streamConfirm <- msg.Data
		}
	}
}

func (s *Session) addStream(stream *Stream) {
	s.streams.Store(stream.ID, stream)
	s.numStreams.Add(1)
	go s.outbound(s.ctx, stream)
	fmt.Println(stream.ID, "added.")
}

func (s *Session) Start(ctx context.Context) {
	s.ctx = ctx
	s.addStream(NewStream(CONTROL_STREAM_ID))
	go s.streamsGarbageCollector(ctx)
	for _, conn := range s.connBundle {
		go s.inbound(ctx, conn)
	}
	s.streams.Range(func(_ any, streamAny any) bool {
		stream := streamAny.(*Stream)
		go s.outbound(ctx, stream)
		return true
	})
	go s.handleControlMsg(ctx)
}

func (s *Session) Close() error {
	for _, conn := range s.connBundle {
		conn.Close()
	}
	s.streams.Range(func(_ any, streamAny any) bool {
		stream := streamAny.(*Stream)
		stream.Close()
		return true
	})
	s.isClosed = true
	return nil
}

func (s *Session) IsClosed() bool {
	return s.isClosed
}

func (s *Session) NumStreams() int {
	return int(s.numStreams.Load())
}

func (s *Session) OpenStream() (*Stream, error) {
	streamID := rand.Text()
	stream := NewStream(streamID)
	s.addStream(stream)
	// 1. Send OPEN_STREAM control message
	s.sendControlMsg(&ControlMsg{
		Type: CONTROL_TYPE_OPEN_STREAM,
		Data: streamID,
	})
	// 2. Wait for STREAM_CONFIRMED message
	receivedId := <-s.streamConfirm
	fmt.Println("Stream confirmed:", streamID)
	if receivedId != streamID {
		return nil, fmt.Errorf("stream ID mismatch: expected %s, got %s", streamID, receivedId)
	}

	return stream, nil
}

func (s *Session) AcceptStream() (*Stream, error) {
	// 1. Wait for OPEN_STREAM message
	streamID := <-s.streamOpen
	stream := NewStream(streamID)
	s.addStream(stream)
	fmt.Println("Accept stream:", streamID)
	// 2. Send STREAM_CONFIRMED message
	msg := ControlMsg{
		Type: CONTROL_STREAM_CONFIRMED,
		Data: streamID,
	}
	s.sendControlMsg(&msg)
	return stream, nil
}
