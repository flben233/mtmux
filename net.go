package mtmux

import (
	"net"
	"strconv"
)

type Listener struct {
	Network   string
	Host      string
	Port      uint64
	Tunnels   int
	Listeners []net.Listener
}

func ListenWithCustomFunc(network, addr string, tunnels int, listenFunc func(addr string) (net.Listener, error)) (*Listener, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, err
	}
	listeners := make([]net.Listener, 0)
	for i := 0; i < tunnels; i++ {
		ln, err := listenFunc(net.JoinHostPort(host, strconv.FormatUint(uint64(port+uint64(i)), 10)))
		if err != nil {
			return nil, err
		}
		listeners = append(listeners, ln)
	}
	return &Listener{
		Network:   network,
		Host:      host,
		Port:      port,
		Tunnels:   tunnels,
		Listeners: listeners,
	}, nil
}

func Listen(network, addr string, tunnels int) (*Listener, error) {
	listen := func(addr string) (net.Listener, error) {
		return net.Listen(network, addr)
	}
	return ListenWithCustomFunc(network, addr, tunnels, listen)
}

func (l *Listener) Accept() ([]net.Conn, error) {
	bundle := make([]net.Conn, 0)
	for _, ln := range l.Listeners {
		conn, err := ln.Accept()
		if err != nil {
			return nil, err
		}
		bundle = append(bundle, conn)
	}
	return bundle, nil
}

func (l *Listener) Close() error {
	for _, ln := range l.Listeners {
		err := ln.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (l *Listener) Addr() net.Addr {
	return l.Listeners[0].Addr()
}

func Dial(network, addr string, tunnels int) ([]net.Conn, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, err
	}
	bundle := make([]net.Conn, 0)
	for i := 0; i < tunnels; i++ {
		conn, err := net.Dial(network, net.JoinHostPort(host, strconv.FormatUint(uint64(port+uint64(i)), 10)))
		if err != nil {
			return nil, err
		}
		bundle = append(bundle, conn)
	}
	return bundle, nil
}
