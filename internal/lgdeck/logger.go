package lgdeck

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

// Logger writes timestamped lines to a file and keeps a ring buffer for the UI.
type Logger struct {
	mu   sync.Mutex
	path string
	buf  []string
	cap  int
}

func NewLogger(path string) *Logger {
	return &Logger{path: path, cap: 1000}
}

func (l *Logger) write(level, msg string) {
	line := fmt.Sprintf("[%s] [%s] %s", time.Now().Format("2006-01-02 15:04:05"), level, msg)
	log.Println(line)

	l.mu.Lock()
	defer l.mu.Unlock()
	l.buf = append(l.buf, line)
	if len(l.buf) > l.cap {
		l.buf = l.buf[len(l.buf)-l.cap:]
	}
	if l.path != "" {
		f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = fmt.Fprintln(f, line)
			_ = f.Close()
		}
	}
}

func (l *Logger) Info(format string, args ...any) {
	l.write("INFO", fmt.Sprintf(format, args...))
}

func (l *Logger) Warn(format string, args ...any) {
	l.write("WARN", fmt.Sprintf(format, args...))
}

func (l *Logger) Error(format string, args ...any) {
	l.write("ERROR", fmt.Sprintf(format, args...))
}

// Tail returns the last n lines from the in-memory buffer.
func (l *Logger) Tail(n int) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if n <= 0 || n > len(l.buf) {
		n = len(l.buf)
	}
	out := make([]string, n)
	copy(out, l.buf[len(l.buf)-n:])
	return out
}
