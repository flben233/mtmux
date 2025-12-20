package mtmux

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type Stream struct {
	ID            string
	ReadBuf       *bytes.Buffer
	WriteBuf      chan []byte
	ReadDeadline  time.Time
	WriteDeadline time.Time
	ReadIndex     atomic.Uint64 // Received data index
	WriteIndex    atomic.Uint64 // Sent data index
	UnorderedBuf  map[uint64][]byte
	endIndex      atomic.Uint64
	writeClosed   atomic.Bool
	mu            sync.Mutex
	readCond      *sync.Cond
	eof           atomic.Bool
}

// NewStream creates a new stream with the given streamID
func NewStream(streamID string) *Stream {
	stream := Stream{
		ID:            streamID,
		ReadBuf:       bytes.NewBuffer(make([]byte, 0)),
		WriteBuf:      make(chan []byte),
		ReadDeadline:  time.Time{},
		WriteDeadline: time.Time{},
		UnorderedBuf:  make(map[uint64][]byte),
		readCond:      sync.NewCond(&sync.Mutex{}),
	}
	return &stream
}

func (s *Stream) Read(b []byte) (n int, err error) {
	if s.IsReadClosed() && s.ReadBuf.Len() == 0 {
		return 0, io.EOF
	}
	// Wait for data to be available
	s.readCond.L.Lock()
	for s.ReadBuf.Len() == 0 {
		s.readCond.Wait()
	}
	// Set up context with deadline
	if s.ReadDeadline.IsZero() {
		s.ReadDeadline = time.Now().Add(30 * time.Hour)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Until(s.ReadDeadline))
	s.ReadDeadline = time.Time{}
	defer cancel()
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}
	// Read data from buffer
	n, err = s.ReadBuf.Read(b)
	s.readCond.L.Unlock()
	return n, err
}

func (s *Stream) Write(b []byte) (n int, err error) {
	// Check if write end is already closed
	if s.writeClosed.Load() {
		return 0, io.ErrClosedPipe
	}

	if s.WriteDeadline.IsZero() {
		s.WriteDeadline = time.Now().Add(30 * time.Hour)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Until(s.WriteDeadline))
	s.WriteDeadline = time.Time{}
	defer cancel()
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	nb := make([]byte, len(b))
	copy(nb, b)
	s.WriteBuf <- nb
	return len(b), nil
}

func (s *Stream) Close() error {
	if !s.writeClosed.Load() {
		close(s.WriteBuf)
	}
	s.writeClosed.Store(true)
	s.endIndex.Store(s.ReadIndex.Load())
	// Flush unordered buffer
	fmt.Println(s.ID, "Remaining: ", len(s.UnorderedBuf))
	return nil
}

func (s *Stream) CloseRead() error {
	s.endIndex.Store(s.ReadIndex.Load())
	return nil
}

func (s *Stream) CloseWrite() error {
	s.writeClosed.Store(true)
	close(s.WriteBuf)
	return nil
}

// LocalAddr returns the local network address, if known.
func (s *Stream) LocalAddr() net.Addr {
	return nil
}

// RemoteAddr returns the remote network address, if known.
func (s *Stream) RemoteAddr() net.Addr {
	return nil
}

func (s *Stream) SetDeadline(t time.Time) error {
	s.ReadDeadline = t
	s.WriteDeadline = t
	return nil
}

func (s *Stream) SetReadDeadline(t time.Time) error {
	s.ReadDeadline = t
	return nil
}

func (s *Stream) SetWriteDeadline(t time.Time) error {
	s.WriteDeadline = t
	return nil
}

// DO NOT MODIFY data AFTER PASSING TO Deliver
func (s *Stream) Deliver(data []byte, idx uint64, isEOF bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if read end is closed
	if s.IsReadClosed() {
		fmt.Println("Deliver: stream read end already closed, dropping data", idx)
		return io.ErrClosedPipe
	}

	if isEOF {
		s.endIndex.Store(idx)
		s.eof.Store(true)
	}

	if s.ReadIndex.Load()+1 == idx {
		s.ReadBuf.Write(data)
		s.ReadIndex.Store(idx)
	} else {
		if idx != s.ReadIndex.Load()+1 {
			fmt.Println("Deliver Out of ordered frame. ", idx, s.ReadIndex.Load()+1)
		}
		s.UnorderedBuf[idx] = data
	}

	// Deliver any ordered frames from buffer
	for i := s.ReadIndex.Load() + 1; ; i++ {
		if d, ok := s.UnorderedBuf[i]; ok {
			s.ReadBuf.Write(d)
			s.ReadIndex.Add(1)
			delete(s.UnorderedBuf, i)
		} else {
			// fmt.Println(s.ID, " Remaining: ", len(s.UnorderedBuf))
			break
		}
	}
	s.readCond.Signal()
	return nil
}

func (s *Stream) IsClosed() bool {
	return s.IsReadClosed() && s.IsWriteClosed()
}

func (s *Stream) IsReadClosed() bool {
	return s.eof.Load() && s.endIndex.Load() <= s.ReadIndex.Load()
}

func (s *Stream) IsWriteClosed() bool {
	return s.writeClosed.Load()
}
