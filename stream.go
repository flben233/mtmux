package mtmux

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type Stream struct {
	ID             string
	ReadBuf        chan []byte
	WriteBuf       chan []byte
	ReadDeadline   time.Time
	WriteDeadline  time.Time
	ReadIndex      atomic.Uint64 // Received data index
	WriteIndex     atomic.Uint64 // Sent data index
	UnorderedBuf   map[uint64][]byte
	readClosed     atomic.Bool // Indicates read end is closed (EOF received)
	writeClosed    atomic.Bool // Indicates write end is closed (EOF sent)
	writeBufClosed atomic.Bool // Indicates WriteBuf channel has been closed
	eofReceived    atomic.Bool // EOF frame received, waiting for unordered buffer to drain
	mu             sync.Mutex
}

func (s *Stream) Read(b []byte) (n int, err error) {
	// Check if read end is already closed
	if s.readClosed.Load() {
		return 0, io.EOF
	}

	if s.ReadDeadline.IsZero() {
		s.ReadDeadline = time.Now().Add(30 * time.Hour)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Until(s.ReadDeadline))
	s.ReadDeadline = time.Time{}
	defer cancel()
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case tb, ok := <-s.ReadBuf:
		if !ok {
			// Channel closed, return EOF
			return 0, io.EOF
		}
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
	// Check if write end is already closed
	if s.writeClosed.Load() {
		return 0, fmt.Errorf("write on closed stream")
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

	// Use defer+recover to handle write to closed channel
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("write to closed stream channel")
		}
	}()

	s.WriteBuf <- nb
	// }
	// s.WriteBuf <- b
	return len(b), nil
}

func (s *Stream) Close() error {
	s.CloseRead()
	s.CloseWrite()
	// Close WriteBuf if not already closed by handleStream
	if !s.writeBufClosed.Swap(true) {
		close(s.WriteBuf)
	}
	// Flush unordered buffer
	fmt.Println(s.ID, "Remaining: ", len(s.UnorderedBuf))
	return nil
}

// CloseRead closes the read end of the stream (called when EOF is received)
func (s *Stream) CloseRead() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Mark EOF as received
	s.eofReceived.Store(true)
	fmt.Println(s.ID, "EOF received, waiting for buffer to drain. Remaining:", len(s.UnorderedBuf))

	// If unordered buffer is empty, close immediately
	if len(s.UnorderedBuf) == 0 {
		if !s.readClosed.Swap(true) {
			close(s.ReadBuf)
			fmt.Println(s.ID, "Read end closed (no pending data)")
		}
	}
	// Otherwise, will be closed in Deliver when buffer drains

	return nil
}

// CloseWrite closes the write end of the stream and sends EOF frame
func (s *Stream) CloseWrite() error {
	if !s.writeClosed.Swap(true) {
		// First time closing write end, send EOF frame
		// Signal to handleStream via a special EOF marker
		// Check if WriteBuf is already closed before sending
		if s.writeBufClosed.Load() {
			// WriteBuf already closed, can't send EOF signal
			fmt.Println(s.ID, "Write end closing, but WriteBuf already closed")
			return nil
		}

		select {
		case s.WriteBuf <- nil: // nil signals EOF
			fmt.Println(s.ID, "Write end closing, EOF signal sent")
		default:
			// WriteBuf is full, send in goroutine with panic protection
			go func() {
				defer func() {
					if r := recover(); r != nil {
						// WriteBuf was closed before we could send, that's ok
						fmt.Println(s.ID, "Write end closing, WriteBuf closed before EOF send")
					}
				}()
				// Double-check before sending
				if !s.writeBufClosed.Load() {
					s.WriteBuf <- nil
					fmt.Println(s.ID, "Write end closing, EOF signal sent (async)")
				}
			}()
		}
	}
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
	defer s.mu.Unlock()

	// Check if read end is closed
	if s.readClosed.Load() {
		fmt.Println("Deliver: stream read end already closed, dropping data", idx)
		return
	}

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

	// Deliver any ordered frames from buffer
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

	// If EOF was received and buffer is now empty, close the read channel
	if s.eofReceived.Load() && len(s.UnorderedBuf) == 0 {
		if !s.readClosed.Swap(true) {
			close(s.ReadBuf)
			fmt.Println(s.ID, "Read end closed (all data delivered after EOF)")
		}
	}
}
