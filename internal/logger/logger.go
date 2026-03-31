// Package logger provides levelled file+stdout logging for ganoidd.
package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Level controls which messages are emitted.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

func ParseLevel(s string) Level {
	switch strings.ToLower(s) {
	case "debug":
		return LevelDebug
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelWarn:
		return "WARN "
	case LevelError:
		return "ERROR"
	default:
		return "INFO "
	}
}

var (
	mu      sync.Mutex
	current = LevelInfo
	lg      *log.Logger
	logFile *os.File
)

func init() {
	// Default: write to stdout only until Init is called.
	lg = log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lmicroseconds)
}

// Init opens (or creates) the log file at path and sets the log level.
// Writes go to both the file and stdout.
func Init(path string, lvl Level) error {
	mu.Lock()
	defer mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	if logFile != nil {
		logFile.Close()
	}
	logFile = f
	current = lvl
	lg = log.New(io.MultiWriter(os.Stdout, f), "", log.Ldate|log.Ltime|log.Lmicroseconds)
	return nil
}

// SetLevel changes the active log level at runtime.
func SetLevel(lvl Level) {
	mu.Lock()
	defer mu.Unlock()
	current = lvl
}

// Close flushes and closes the log file.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if logFile != nil {
		logFile.Close()
		logFile = nil
	}
}

func logf(lvl Level, format string, args ...interface{}) {
	mu.Lock()
	l := current
	mu.Unlock()
	if lvl < l {
		return
	}
	msg := fmt.Sprintf(format, args...)
	lg.Printf("[%s] %s", lvl, msg)
}

func Debug(format string, args ...interface{}) { logf(LevelDebug, format, args...) }
func Info(format string, args ...interface{})  { logf(LevelInfo, format, args...) }
func Warn(format string, args ...interface{})  { logf(LevelWarn, format, args...) }
func Error(format string, args ...interface{}) { logf(LevelError, format, args...) }
