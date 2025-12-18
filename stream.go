package mtmux

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type Stream struct {
	ID            string
	ReadBuf       chan []byte
	WriteBuf      chan []byte
	ReadDeadline  time.Time
	WriteDeadline time.Time
	ReadIndex     atomic.Uint64 // Received data index
	WriteIndex    atomic.Uint64 // Sent data index
	UnorderedBuf  map[uint64][]byte
	mu            sync.Mutex
}

func (s *Stream) Read(b []byte) (n int, err error) {
	if s.ReadDeadline.IsZero() {
		s.ReadDeadline = time.Now().Add(30 * time.Hour)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Until(s.ReadDeadline))
	s.ReadDeadline = time.Time{}
	defer cancel()
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case tb := <-s.ReadBuf:
		n := copy(b, tb)
		if n < len(tb) {
			// Put back remaining data
			remaining := make([]byte, len(tb)-n)
			copy(remaining, tb[n:])
			s.ReadBuf <- remaining
		}
		return n, nil
	}
}

func (s *Stream) Write(b []byte) (n int, err error) {
	if s.WriteDeadline.IsZero() {
		s.WriteDeadline = time.Now().Add(30 * time.Hour)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Until(s.WriteDeadline))
	s.WriteDeadline = time.Time{}
	defer cancel()
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	// maxSize := 32 * 1024
	// for len(b) > maxSize {
	// 	chunk := make([]byte, maxSize)
	// 	copy(chunk, b[:maxSize])
	// 	b = b[maxSize:]
	// 	s.WriteBuf <- chunk
	// }
	// if len(b) < maxSize {
	nb := make([]byte, len(b))
	copy(nb, b)
	s.WriteBuf <- nb
	// }
	// s.WriteBuf <- b
	return len(b), nil
}

func (s *Stream) Close() error {
	// close(s.ReadBuf)
	// close(s.WriteBuf)
	// Flush unordered buffer
	fmt.Println(s.ID, "Remaining: ", len(s.UnorderedBuf))
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

func (s *Stream) Deliver(data []byte, idx uint64) {
	s.mu.Lock()
	fmt.Println("Deliver: ", idx)
	if s.ReadIndex.Load()+1 == idx {
		s.ReadBuf <- data
		s.ReadIndex.Store(idx)
	} else {
		if idx != s.ReadIndex.Load()+1 {
			fmt.Println("Deliver Out of ordered frame. ", idx, s.ReadIndex.Load()+1)
		}
		s.UnorderedBuf[idx] = data
	}
	for i := s.ReadIndex.Load() + 1; ; i++ {
		if d, ok := s.UnorderedBuf[i]; ok {
			s.ReadBuf <- d
			s.ReadIndex.Add(1)
			delete(s.UnorderedBuf, i)
		} else {
			fmt.Println(s.ID, " Remaining: ", len(s.UnorderedBuf))
			break
		}
	}
	s.mu.Unlock()
}
