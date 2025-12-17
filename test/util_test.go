package mtmux

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/flben233/mtmux"
)

func TestWaitWithContext_Succeeds(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		time.Sleep(20 * time.Millisecond)
		wg.Done()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if err := mtmux.WaitWithContext(&wg, ctx); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestWaitWithContext_TimesOut(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := mtmux.WaitWithContext(&wg, ctx); err == nil {
		t.Fatalf("expected error due to timeout, got nil")
	}
}
