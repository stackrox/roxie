package helpers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/stackrox/roxie/internal/logger"
)

// ExecOptions contains options for running kubectl commands
type ExecOptions struct {
	Stdin                io.Reader                // Optional stdin for commands.
	RetryablePredicate   func(stderr string) bool // Check if stderr contains indicates of a retryable error.
	MaxAttempts          int                      // Maximum number of retry attempts (default: 3)
	RetryDelay           int                      // Base retry delay in seconds (default: 2)
	IgnoreErrors         bool                     // If true, return nil on errors (useful for cleanup operations)
	SkipLoggingOnRetry   bool                     // If true, don't log on failures.
	SkipLoggingOnFailure bool                     // If true, don't log on failures.
}

var defaultExecOptions = ExecOptions{
	MaxAttempts: 3,
	RetryDelay:  1,
}

// ExecResult contains the result -- stdout and stderr -- of a command execution.
type ExecResult struct {
	Stdout string
	Stderr string
}

type ExecOption func(*ExecOptions)

func WithStdin(stdin io.Reader) ExecOption {
	return func(opts *ExecOptions) {
		opts.Stdin = stdin
	}
}

func WithRetryablePredicate(predicate func(stderr string) bool) ExecOption {
	return func(opts *ExecOptions) {
		opts.RetryablePredicate = predicate
	}
}

func WithMaxAttempts(maxAttempts int) ExecOption {
	return func(opts *ExecOptions) {
		opts.MaxAttempts = maxAttempts
	}
}

func WithRetryDelay(retryDelay int) ExecOption {
	return func(opts *ExecOptions) {
		opts.RetryDelay = retryDelay
	}
}

func WithIgnoreErrors(ignoreErrors bool) ExecOption {
	return func(opts *ExecOptions) {
		opts.IgnoreErrors = ignoreErrors
	}
}

func WithSkipLoggingOnRetry(skipLogging bool) ExecOption {
	return func(opts *ExecOptions) {
		opts.SkipLoggingOnRetry = skipLogging
	}
}

func WithSkipLoggingOnFailure(skipLogging bool) ExecOption {
	return func(opts *ExecOptions) {
		opts.SkipLoggingOnFailure = skipLogging
	}
}

// Exec executes a command with support for retries. Automatically emits stderr to the logger in the context.
func Exec(ctx context.Context, args []string, optFns ...ExecOption) (*ExecResult, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("no arguments provided")
	}
	command := strings.Join(args, " ")

	opts := defaultExecOptions
	for _, optFn := range optFns {
		optFn(&opts)
	}

	logr := logger.FromContext(ctx)
	var logOnFailure bool
	var logOnRetry bool

	if logr != nil {
		logOnFailure = !opts.SkipLoggingOnFailure
		logOnRetry = !opts.SkipLoggingOnRetry
	}

	var lastStderr string
	var lastErr error

	for attempt := 1; attempt <= opts.MaxAttempts; attempt++ {
		if attempt > 1 {
			waitTime := time.Duration(attempt*opts.RetryDelay) * time.Second
			if logOnRetry {
				logr.Infof("Retrying command (attempt %d/%d) after %v...", attempt, opts.MaxAttempts, waitTime)
			}
			time.Sleep(waitTime)
		}

		cmd := exec.CommandContext(ctx, args[0], args[1:]...)

		if opts.Stdin != nil {
			// For retry attempts, we need to reset the reader if it's a bytes.Reader
			if reader, ok := opts.Stdin.(*bytes.Reader); ok {
				reader.Seek(0, io.SeekStart)
			}
			cmd.Stdin = opts.Stdin
		}

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		var exitCode int
		err := cmd.Run()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); !ok {
				return nil, fmt.Errorf("command '%s' failed with unexpected error: %w", command, err)
			} else {
				exitCode = exitErr.ExitCode()
			}
		}

		lastStderr = stderr.String()
		lastErr = err

		if err == nil {
			return &ExecResult{
				Stdout: stdout.String(),
				Stderr: lastStderr,
			}, nil
		}

		if opts.IgnoreErrors {
			return &ExecResult{
				Stdout: stdout.String(),
				Stderr: lastStderr,
			}, nil
		}

		if logOnFailure {
			logr.Warningf("Error during invocation of command '%s', returned exit code %d", command, exitCode)
		}

		for _, stderrLine := range strings.Split(strings.TrimSpace(lastStderr), "\n") {
			if logOnFailure {
				logr.Dimf("stderr: %s", stderrLine)
			}
		}

		isRetryable := false
		if opts.RetryablePredicate != nil {
			isRetryable = opts.RetryablePredicate(lastStderr)
		}

		if !isRetryable || attempt == opts.MaxAttempts {
			return &ExecResult{
				Stdout: stdout.String(),
				Stderr: lastStderr,
			}, fmt.Errorf("command '%s' failed: %w", command, err)
		}

	}

	return &ExecResult{
		Stdout: "",
		Stderr: lastStderr,
	}, fmt.Errorf("command '%s' failed after %d attempts: %w", command, opts.MaxAttempts, lastErr)
}
