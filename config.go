package mtmux

import "time"

type Config struct {
	KeepAliveInterval time.Duration
	KeepAliveTimeout  time.Duration
	Tunnels           int32
	Timeout           time.Duration
}

func DefaultConfig() *Config {
	return &Config{
		KeepAliveInterval: 30 * time.Second,
		KeepAliveTimeout:  90 * time.Second,
		Tunnels:           8,
		Timeout:           30 * time.Second,
	}
}
