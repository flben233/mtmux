package mtmux

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Level represents a logging level.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelOff
)

// Logger is the interface users can implement to provide custom logging.
// It includes leveled methods and formatters.
type Logger interface {
	Debug(v ...interface{})
	Info(v ...interface{})
	Warn(v ...interface{})
	Error(v ...interface{})

	Debugf(format string, v ...interface{})
	Infof(format string, v ...interface{})
	Warnf(format string, v ...interface{})
	Errorf(format string, v ...interface{})
}

type defaultLogger struct {
	min Level
}

type loggerHolder struct{ L Logger }

var outMu sync.Mutex

func (l *defaultLogger) should(level Level) bool {
	return level >= l.min && l.min != LevelOff
}

func (l *defaultLogger) Debug(v ...interface{}) {
	if !l.should(LevelDebug) {
		return
	}
	ts := time.Now().Format(time.RFC3339)
	msg := strings.TrimSuffix(fmt.Sprintln(v...), "\n")
	outMu.Lock()
	fmt.Printf("%s DEBUG: %s\n", ts, msg)
	outMu.Unlock()
}
func (l *defaultLogger) Info(v ...interface{}) {
	if !l.should(LevelInfo) {
		return
	}
	ts := time.Now().Format(time.RFC3339)
	msg := strings.TrimSuffix(fmt.Sprintln(v...), "\n")
	outMu.Lock()
	fmt.Printf("%s INFO: %s\n", ts, msg)
	outMu.Unlock()
}
func (l *defaultLogger) Warn(v ...interface{}) {
	if !l.should(LevelWarn) {
		return
	}
	ts := time.Now().Format(time.RFC3339)
	msg := strings.TrimSuffix(fmt.Sprintln(v...), "\n")
	outMu.Lock()
	fmt.Printf("%s WARN: %s\n", ts, msg)
	outMu.Unlock()
}
func (l *defaultLogger) Error(v ...interface{}) {
	if !l.should(LevelError) {
		return
	}
	ts := time.Now().Format(time.RFC3339)
	msg := strings.TrimSuffix(fmt.Sprintln(v...), "\n")
	outMu.Lock()
	fmt.Printf("%s ERROR: %s\n", ts, msg)
	outMu.Unlock()
}

func (l *defaultLogger) Debugf(format string, v ...interface{}) {
	if !l.should(LevelDebug) {
		return
	}
	ts := time.Now().Format(time.RFC3339)
	outMu.Lock()
	fmt.Printf("%s DEBUG: "+format+"\n", append([]interface{}{ts}, v...)...)
	outMu.Unlock()
}
func (l *defaultLogger) Infof(format string, v ...interface{}) {
	if !l.should(LevelInfo) {
		return
	}
	ts := time.Now().Format(time.RFC3339)
	outMu.Lock()
	fmt.Printf("%s INFO: "+format+"\n", append([]interface{}{ts}, v...)...)
	outMu.Unlock()
}
func (l *defaultLogger) Warnf(format string, v ...interface{}) {
	if !l.should(LevelWarn) {
		return
	}
	ts := time.Now().Format(time.RFC3339)
	outMu.Lock()
	fmt.Printf("%s WARN: "+format+"\n", append([]interface{}{ts}, v...)...)
	outMu.Unlock()
}
func (l *defaultLogger) Errorf(format string, v ...interface{}) {
	if !l.should(LevelError) {
		return
	}
	ts := time.Now().Format(time.RFC3339)
	outMu.Lock()
	fmt.Printf("%s ERROR: "+format+"\n", append([]interface{}{ts}, v...)...)
	outMu.Unlock()
}

// (no backwards-compatible Print methods)

// noOutputLogger discards everything.
type noOutputLogger struct{}

func (n *noOutputLogger) Debug(v ...interface{})        {}
func (n *noOutputLogger) Info(v ...interface{})         {}
func (n *noOutputLogger) Warn(v ...interface{})         {}
func (n *noOutputLogger) Error(v ...interface{})        {}
func (n *noOutputLogger) Debugf(string, ...interface{}) {}
func (n *noOutputLogger) Infof(string, ...interface{})  {}
func (n *noOutputLogger) Warnf(string, ...interface{})  {}
func (n *noOutputLogger) Errorf(string, ...interface{}) {}

// (no print methods for noOutputLogger)

var loggerInstance atomic.Value // stores Logger
var minLevel atomic.Int32

func init() {
	// default to Info level
	minLevel.Store(int32(LevelInfo))
	// store a holder pointer so atomic.Value always sees the same concrete type
	loggerInstance.Store(&loggerHolder{L: &defaultLogger{min: LevelInfo}})
}

// SetLogger sets the package-wide logger. Passing nil resets to default.
func SetLogger(l Logger) {
	if l == nil {
		lvl := Level(minLevel.Load())
		loggerInstance.Store(&loggerHolder{L: &defaultLogger{min: lvl}})
		return
	}
	loggerInstance.Store(&loggerHolder{L: l})
}

// SetLevel sets the global minimum logging level. Messages below
// this level will be ignored by the default logger. If a custom
// logger is set it is up to that implementation to respect levels.
func SetLevel(l Level) {
	minLevel.Store(int32(l))
	// if current logger is defaultLogger, update its min
	current := loggerInstance.Load()
	if h, ok := current.(*loggerHolder); ok {
		if dl, ok2 := h.L.(*defaultLogger); ok2 {
			dl.min = l
			// re-store the holder so the updated value is visible
			loggerInstance.Store(h)
		}
	}
}

func getLogger() Logger {
	current := loggerInstance.Load()
	if h, ok := current.(*loggerHolder); ok {
		return h.L
	}
	// fallback (should not happen) — try direct assertion
	if l, ok := current.(Logger); ok {
		return l
	}
	return &noOutputLogger{}
}

// Convenience package-level functions that mirror fmt's Print family.
// Note: package-level Print/Println/Printf helpers intentionally removed.

// Leveled convenience helpers
func Debug(v ...interface{}) { getLogger().Debug(v...) }
func Info(v ...interface{})  { getLogger().Info(v...) }
func Warn(v ...interface{})  { getLogger().Warn(v...) }
func Error(v ...interface{}) { getLogger().Error(v...) }

func Debugf(format string, v ...interface{}) { getLogger().Debugf(format, v...) }
func Infof(format string, v ...interface{})  { getLogger().Infof(format, v...) }
func Warnf(format string, v ...interface{})  { getLogger().Warnf(format, v...) }
func Errorf(format string, v ...interface{}) { getLogger().Errorf(format, v...) }

// SetNoOutputLogger sets a logger that discards all output.
func SetNoOutputLogger() { SetLogger(&noOutputLogger{}) }
