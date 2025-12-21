package mtmux

import (
	"bytes"
	"sync"
)

type SafeBuffer struct {
	buf  *bytes.Buffer
	lock sync.RWMutex
}

func (sb *SafeBuffer) Write(p []byte) (n int, err error) {
	sb.lock.Lock()
	defer sb.lock.Unlock()
	return sb.buf.Write(p)
}

func (sb *SafeBuffer) Read(p []byte) (n int, err error) {
	sb.lock.RLock()
	defer sb.lock.RUnlock()
	return sb.buf.Read(p)
}

func (sb *SafeBuffer) Len() int {
	sb.lock.RLock()
	defer sb.lock.RUnlock()
	return sb.buf.Len()
}

func (sb *SafeBuffer) ReadWhenNotEmpty(p []byte) (n int, err error) {
	sb.lock.RLock()
	defer sb.lock.RUnlock()
	if sb.buf.Len() == 0 {
		return 0, nil
	}
	return sb.buf.Read(p)
}

func (sb *SafeBuffer) Reset() {
	sb.lock.Lock()
	defer sb.lock.Unlock()
	sb.buf.Reset()
}
