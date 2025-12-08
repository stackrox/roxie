package logger

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/fatih/color"
)

// Logger provides logging functionality.
type Logger interface {
	Info(message string)
	Infof(format string, args ...interface{})
	Error(message string)
	Errorf(format string, args ...interface{})
	Success(message string)
	Successf(format string, args ...interface{})
	Warning(message string)
	Warningf(format string, args ...interface{})
	Dim(message string)
	Dimf(format string, args ...interface{})
}

type LoggerImpl struct {
	startTime      time.Time
	stdout         io.Writer
	stderr         io.Writer
	withTimestamps bool
}

// New creates a new Logger instance.
func New() *LoggerImpl {
	return &LoggerImpl{
		startTime: time.Now(),
		stdout:    os.Stdout,
		stderr:    os.Stderr,
	}
}

// NewWithTimestamps creates a new Logger instance with timestamps.
func NewWithTimestamps() *LoggerImpl {
	logger := New()
	logger.withTimestamps = true
	return logger
}

// getTimestamp returns the elapsed time since logger creation in MM:SS format.
func (l *LoggerImpl) getTimestamp() string {
	if !l.withTimestamps {
		return ""
	}
	elapsed := time.Since(l.startTime)
	minutes := int(elapsed.Minutes())
	seconds := int(elapsed.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d ", minutes, seconds)
}

// Info prints an info message with magenta styling
func (l *LoggerImpl) Info(message string) {
	timestamp := color.GreenString(l.getTimestamp())
	info := color.New(color.FgMagenta, color.Bold).Sprint(message)
	fmt.Fprintf(l.stdout, "%s%s\n", timestamp, info)
}

// Infof prints a formatted info message with magenta styling
func (l *LoggerImpl) Infof(format string, args ...interface{}) {
	l.Info(fmt.Sprintf(format, args...))
}

// Error prints an error message with red styling to stderr
func (l *LoggerImpl) Error(message string) {
	timestamp := color.GreenString(l.getTimestamp())
	errMsg := color.New(color.FgRed, color.Bold).Sprint(message)
	fmt.Fprintf(l.stderr, "%s%s\n", timestamp, errMsg)
}

// Errorf prints a formatted error message with red styling to stderr
func (l *LoggerImpl) Errorf(format string, args ...interface{}) {
	l.Error(fmt.Sprintf(format, args...))
}

// Success prints a success message with green styling
func (l *LoggerImpl) Success(message string) {
	timestamp := color.GreenString(l.getTimestamp())
	success := color.New(color.FgGreen, color.Bold).Sprint(message)
	fmt.Fprintf(l.stdout, "%s%s\n", timestamp, success)
}

// Successf prints a formatted success message with green styling
func (l *LoggerImpl) Successf(format string, args ...interface{}) {
	l.Success(fmt.Sprintf(format, args...))
}

// Warning prints a warning message with yellow styling
func (l *LoggerImpl) Warning(message string) {
	timestamp := color.GreenString(l.getTimestamp())
	warning := color.New(color.FgYellow, color.Bold).Sprint(message)
	fmt.Fprintf(l.stdout, "%s%s\n", timestamp, warning)
}

// Warning prints a formatted warning message with yellow styling
func (l *LoggerImpl) Warningf(format string, args ...interface{}) {
	l.Warning(fmt.Sprintf(format, args...))
}

// Dim prints a dimmed message
func (l *LoggerImpl) Dim(message string) {
	timestamp := color.GreenString(l.getTimestamp())
	dim := color.New(color.Faint).Sprint(message)
	fmt.Fprintf(l.stdout, "%s%s\n", timestamp, dim)
}

// Dimf prints a formatted dimmed message
func (l *LoggerImpl) Dimf(format string, args ...interface{}) {
	l.Dim(fmt.Sprintf(format, args...))
}
