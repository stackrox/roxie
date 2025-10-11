package logger

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/fatih/color"
)

// Logger provides timestamped logging functionality
type Logger struct {
	startTime time.Time
	stdout    io.Writer
	stderr    io.Writer
}

// New creates a new Logger instance
func New() *Logger {
	return &Logger{
		startTime: time.Now(),
		stdout:    os.Stdout,
		stderr:    os.Stderr,
	}
}

// getTimestamp returns the elapsed time since logger creation in MM:SS format
func (l *Logger) getTimestamp() string {
	elapsed := time.Since(l.startTime)
	minutes := int(elapsed.Minutes())
	seconds := int(elapsed.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

// PrintWithTimestamp prints a message with a timestamp prefix
func (l *Logger) PrintWithTimestamp(message string) {
	timestamp := color.GreenString(l.getTimestamp())
	fmt.Fprintf(l.stdout, "%s %s\n", timestamp, message)
}

// Info prints an info message with magenta styling
func (l *Logger) Info(message string) {
	timestamp := color.GreenString(l.getTimestamp())
	info := color.New(color.FgMagenta, color.Bold).Sprint(message)
	fmt.Fprintf(l.stdout, "%s %s\n", timestamp, info)
}

// Error prints an error message with red styling to stderr
func (l *Logger) Error(message string) {
	timestamp := color.GreenString(l.getTimestamp())
	errMsg := color.New(color.FgRed, color.Bold).Sprint(message)
	fmt.Fprintf(l.stderr, "%s %s\n", timestamp, errMsg)
}

// Success prints a success message with green styling
func (l *Logger) Success(message string) {
	timestamp := color.GreenString(l.getTimestamp())
	success := color.New(color.FgGreen, color.Bold).Sprint(message)
	fmt.Fprintf(l.stdout, "%s %s\n", timestamp, success)
}

// Warning prints a warning message with yellow styling
func (l *Logger) Warning(message string) {
	timestamp := color.GreenString(l.getTimestamp())
	warning := color.New(color.FgYellow, color.Bold).Sprint(message)
	fmt.Fprintf(l.stdout, "%s %s\n", timestamp, warning)
}

// Dim prints a dimmed message
func (l *Logger) Dim(message string) {
	timestamp := color.GreenString(l.getTimestamp())
	dim := color.New(color.Faint).Sprint(message)
	fmt.Fprintf(l.stdout, "%s %s\n", timestamp, dim)
}
