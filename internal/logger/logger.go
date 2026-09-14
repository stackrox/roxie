package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"gopkg.in/yaml.v3"
)

const (
	LevelDim     = slog.Level(-2)
	LevelSuccess = slog.Level(2)
)

// Logger wraps slog.Logger with roxie's CLI output style:
// elapsed MM:SS timestamps, per-level coloring, and stdout/stderr routing.
type Logger struct {
	log   *slog.Logger
	level *slog.LevelVar
}

// New creates a new roxie logger.
func New() *Logger {
	return NewWithWriters(os.Stdout, os.Stderr)
}

// NewWithWriters creates a new roxie logger with configurable writers for stdout/stderr.
func NewWithWriters(stdout, stderr io.Writer) *Logger {
	level := &slog.LevelVar{}
	level.Set(LevelDim)

	h := &handler{
		level:     level,
		startTime: time.Now(),
		stdout:    stdout,
		stderr:    stderr,
	}

	return &Logger{
		log:   slog.New(h),
		level: level,
	}
}

// SetVerbose enables verbose mode for the logger, which means that debug-level messages will be emitted.
func (l *Logger) SetVerbose(verbose bool) {
	if l == nil {
		return
	}
	if verbose {
		l.level.Set(slog.LevelDebug)
	} else {
		l.level.Set(LevelDim)
	}
}

// IsVerbose returns true if verbose mode is enabled.
func (l *Logger) IsVerbose() bool {
	return l != nil && l.level.Level() <= slog.LevelDebug
}

// Info prints an info message with magenta styling.
func (l *Logger) Info(message string) {
	if l == nil {
		return
	}
	l.log.Log(context.Background(), slog.LevelInfo, message)
}

// Infof prints a formatted info message with magenta styling.
func (l *Logger) Infof(format string, args ...any) {
	l.Info(fmt.Sprintf(format, args...))
}

// Error prints an error message with red styling to stderr.
func (l *Logger) Error(message string) {
	if l == nil {
		return
	}
	l.log.Log(context.Background(), slog.LevelError, message)
}

// Errorf prints a formatted error message with red styling to stderr.
func (l *Logger) Errorf(format string, args ...any) {
	l.Error(fmt.Sprintf(format, args...))
}

// Success prints a success message with green styling.
func (l *Logger) Success(message string) {
	if l == nil {
		return
	}
	l.log.Log(context.Background(), LevelSuccess, message)
}

// Successf prints a formatted success message with green styling.
func (l *Logger) Successf(format string, args ...any) {
	l.Success(fmt.Sprintf(format, args...))
}

// Warning prints a warning message with yellow styling.
func (l *Logger) Warning(message string) {
	if l == nil {
		return
	}
	l.log.Log(context.Background(), slog.LevelWarn, message)
}

// Warningf prints a formatted warning message with yellow styling.
func (l *Logger) Warningf(format string, args ...any) {
	l.Warning(fmt.Sprintf(format, args...))
}

// Dim prints a faint message that is always visible.
// For verbose-only output, use Debug/Debugf.
func (l *Logger) Dim(message string) {
	if l == nil {
		return
	}
	l.log.Log(context.Background(), LevelDim, message)
}

// Dimf prints a formatted faint message that is always visible.
func (l *Logger) Dimf(format string, args ...any) {
	l.Dim(fmt.Sprintf(format, args...))
}

// Debug prints a faint message only visible in verbose mode.
func (l *Logger) Debug(message string) {
	if l == nil {
		return
	}
	l.log.Log(context.Background(), slog.LevelDebug, message)
}

// Debug prints a formatted faint message only visible in verbose mode.
func (l *Logger) Debugf(format string, args ...any) {
	l.Debug(fmt.Sprintf(format, args...))
}

// LogMultilineYaml marshals v to YAML and logs it line by line at debug level.
// It is a no-op unless verbose mode is enabled, so callers need no guard.
func (l *Logger) LogMultilineYaml(v any) {
	if !l.IsVerbose() {
		return
	}
	bytes, err := yaml.Marshal(v)
	if err != nil {
		l.Debugf("failed to marshal YAML: %v", err)
		return
	}
	l.Debug("-------------------------")
	for line := range strings.SplitSeq(string(bytes), "\n") {
		l.Debug(line)
	}
	l.Debug("-------------------------")
}

// handler implements slog.Handler with roxie's CLI output format.
type handler struct {
	level     *slog.LevelVar
	startTime time.Time
	stdout    io.Writer
	stderr    io.Writer
}

// Retrurns true if the given log level is enabled for the provided log handler.
func (h *handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *handler) Handle(_ context.Context, r slog.Record) error {
	elapsed := time.Since(h.startTime)
	if !r.Time.IsZero() {
		elapsed = r.Time.Sub(h.startTime)
	}

	minutes := int(elapsed.Minutes())
	seconds := int(elapsed.Seconds()) % 60
	timestamp := color.GreenString("%02d:%02d", minutes, seconds)
	message := styleForLevel(r.Level).Sprint(r.Message)

	w := h.stdout
	if r.Level >= slog.LevelError {
		w = h.stderr
	}

	fmt.Fprintf(w, "%s %s\n", timestamp, message)
	return nil
}

func (h *handler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *handler) WithGroup(_ string) slog.Handler      { return h }

func styleForLevel(level slog.Level) *color.Color {
	switch {
	case level >= slog.LevelError:
		return color.New(color.FgRed, color.Bold)
	case level >= slog.LevelWarn:
		return color.New(color.FgYellow, color.Bold)
	case level >= LevelSuccess:
		return color.New(color.FgGreen, color.Bold)
	case level >= slog.LevelInfo:
		return color.New(color.FgMagenta, color.Bold)
	default:
		return color.New(color.Faint)
	}
}
