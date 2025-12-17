package mtmux

import (
	"context"
	"net"
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
}

func (s *Stream) Read(b []byte) (n int, err error) {
	if s.ReadDeadline.IsZero() {
		s.ReadDeadline = time.Now().Add(30 * time.Second)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Until(s.ReadDeadline))
	s.ReadDeadline = time.Time{}
	defer cancel()
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case tb := <-s.ReadBuf:
		n := copy(b, tb)
		return n, nil
	}
}

func (s *Stream) Write(b []byte) (n int, err error) {
	if s.WriteDeadline.IsZero() {
		s.WriteDeadline = time.Now().Add(30 * time.Second)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Until(s.WriteDeadline))
	s.WriteDeadline = time.Time{}
	defer cancel()
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	s.WriteBuf <- b
	return len(b), nil
}

func (s *Stream) Close() error {
	close(s.ReadBuf)
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
