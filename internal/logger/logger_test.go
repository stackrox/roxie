package logger

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDebugOnlyEmitsWhenVerbose(t *testing.T) {
	var buf bytes.Buffer
	l := NewWithWriter(&buf)

	// Debug is suppressed at the default level (LevelDim).
	l.Debug("hidden")
	if buf.Len() != 0 {
		t.Fatalf("expected no output before SetVerbose(true), got %q", buf.String())
	}

	// Enabling verbose lowers the level to slog.LevelDebug, so Debug emits.
	l.SetVerbose(true)
	l.Debug("visible")
	if !strings.Contains(buf.String(), "visible") {
		t.Fatalf("expected debug output to contain %q, got %q", "visible", buf.String())
	}

	// Disabling verbose restores the default level and suppresses Debug again.
	before := buf.Len()
	l.SetVerbose(false)
	l.Debug("hidden again")
	if buf.Len() != before {
		t.Fatalf("expected no new output after SetVerbose(false), got %q", buf.String()[before:])
	}
}

// TestSetDefaultNilIsSafe verifies that storing a nil logger as the package
// default does not cause package-level log calls to panic.
func TestSetDefaultNilIsSafe(t *testing.T) {
	old := SetDefault(nil)
	t.Cleanup(func() { SetDefault(old) })

	assert.Nil(Default(), "expected Default() to return the nil logger that was stored")

	SetVerbose(true)
	assert.False(t, IsVerbose(), "expected IsVerbose() to report false for a nil default logger")

	Info("x")
	Error("x")
	Success("x")
	Warning("x")
	Dim("x")
	Debug("x")
}
