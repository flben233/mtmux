package mtmux

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/flben233/mtmux"
)

// captureStdout temporarily redirects stdout and returns captured output.
func captureStdout(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stdout = old
	return buf.String()
}

func TestDefaultLoggerTimestampAndLevel(t *testing.T) {
	// ensure default logger at Info
	mtmux.SetLogger(nil)
	mtmux.SetLevel(mtmux.LevelInfo)

	out := captureStdout(func() {
		mtmux.Info("hello test")
	})

	if !strings.Contains(out, " INFO: ") {
		t.Fatalf("expected INFO level in output, got: %q", out)
	}

	// timestamp is at the beginning; try to parse prefix
	// Split by space to get timestamp token
	parts := strings.SplitN(strings.TrimSpace(out), " ", 2)
	if len(parts) < 2 {
		t.Fatalf("unexpected log format: %q", out)
	}
	if _, err := time.Parse(time.RFC3339, parts[0]); err != nil {
		t.Fatalf("timestamp not RFC3339: %q (%v)", parts[0], err)
	}
}

func TestSetLevelFiltersLower(t *testing.T) {
	// restore default logger
	mtmux.SetLogger(nil)
	mtmux.SetLevel(mtmux.LevelError)

	out := captureStdout(func() {
		mtmux.Info("this should not appear")
		mtmux.Error("this should appear")
	})

	if strings.Contains(out, "this should not appear") {
		t.Fatalf("info message should have been filtered: %q", out)
	}
	if !strings.Contains(out, "this should appear") {
		t.Fatalf("error message should appear: %q", out)
	}
}

func TestNoOutputLogger(t *testing.T) {
	// set no-output logger
	mtmux.SetNoOutputLogger()

	out := captureStdout(func() {
		mtmux.Info("nothing")
		mtmux.Error("nothing")
	})

	if out != "" {
		t.Fatalf("expected no output, got: %q", out)
	}
}

type bufLogger struct{ buf *bytes.Buffer }

func (b *bufLogger) Debug(v ...interface{})        { b.buf.WriteString("D:") }
func (b *bufLogger) Info(v ...interface{})         { b.buf.WriteString("I:") }
func (b *bufLogger) Warn(v ...interface{})         { b.buf.WriteString("W:") }
func (b *bufLogger) Error(v ...interface{})        { b.buf.WriteString("E:") }
func (b *bufLogger) Debugf(string, ...interface{}) { b.buf.WriteString("Df") }
func (b *bufLogger) Infof(string, ...interface{})  { b.buf.WriteString("If") }
func (b *bufLogger) Warnf(string, ...interface{})  { b.buf.WriteString("Wf") }
func (b *bufLogger) Errorf(string, ...interface{}) { b.buf.WriteString("Ef") }

func TestCustomLoggerUsed(t *testing.T) {
	b := &bytes.Buffer{}
	bl := &bufLogger{buf: b}
	mtmux.SetLogger(bl)
	// call a few methods
	mtmux.Debug("x")
	mtmux.Info("y")
	mtmux.Warn("z")
	mtmux.Error("q")

	s := b.String()
	if !strings.Contains(s, "I:") || !strings.Contains(s, "E:") {
		t.Fatalf("custom logger not used as expected: %q", s)
	}
}
