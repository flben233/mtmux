package mtmux

import (
	"context"
	"net"
	"sync"
)

func WaitWithContext(wg *sync.WaitGroup, ctx context.Context) error {
	c := make(chan struct{})
	go func() {
		wg.Wait()
		close(c)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c:
		return nil
	}
}

func ConnWrapper(conns []net.Conn, wrapper func(conn net.Conn) net.Conn) []net.Conn {
	wrappedConns := make([]net.Conn, len(conns))
	for i, conn := range conns {
		wrappedConns[i] = wrapper(conn)
	}
	return wrappedConns
}

func RemoveElement(slice []net.Conn, index int) []net.Conn {
	slice[index] = nil
	return append(slice[:index], slice[index+1:]...)
}
