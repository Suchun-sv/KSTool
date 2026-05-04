// Package log provides a small logging facade. It writes structured records to
// a rotating-friendly file under ~/.kstool and best-effort to syslog.
package log

import (
	"fmt"
	"log/slog"
	"log/syslog"
	"os"
	"os/user"
	"path/filepath"
	"sync"
	"time"
)

const (
	configDir = ".kstool"
	logFile   = "kstool.log"
)

var (
	mu       sync.Mutex
	logger   *slog.Logger
	syslogFd *syslog.Writer
)

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
		return
	}
	dir := filepath.Join(home, configDir)
	_ = os.MkdirAll(dir, 0o700)
	f, err := os.OpenFile(filepath.Join(dir, logFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
		return
	}
	logger = slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo}))
	syslogFd, _ = syslog.New(syslog.LOG_INFO, "kstool")
}

// CurrentUser returns the OS user name, falling back to $USER then "unknown".
func CurrentUser() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	if v := os.Getenv("USER"); v != "" {
		return v
	}
	return "unknown"
}

// Action writes an audit record describing a user action against a job.
func Action(action, jobName string, attrs ...slog.Attr) {
	mu.Lock()
	defer mu.Unlock()
	base := []slog.Attr{
		slog.String("ts", time.Now().Format(time.RFC3339)),
		slog.String("user", CurrentUser()),
		slog.String("action", action),
		slog.String("job", jobName),
	}
	logger.LogAttrs(nil, slog.LevelInfo, "action", append(base, attrs...)...)
	if syslogFd != nil {
		_ = syslogFd.Info(fmt.Sprintf("action=%s user=%s job=%s", action, CurrentUser(), jobName))
	}
}

// Errorf writes an error-level record.
func Errorf(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	logger.Error(fmt.Sprintf(format, args...))
}
