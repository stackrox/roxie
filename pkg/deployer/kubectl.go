package deployer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// KubectlOptions contains options for running kubectl commands
type KubectlOptions struct {
	Args         []string  // Command arguments (e.g., ["get", "pod", "my-pod"])
	Stdin        io.Reader // Optional stdin for commands like "apply -f -"
	MaxAttempts  int       // Maximum number of retry attempts (default: 3)
	RetryDelay   int       // Base retry delay in seconds (default: 2)
	IgnoreErrors bool      // If true, return nil on errors (useful for cleanup operations)
}

// KubectlResult contains the result of a kubectl command execution
type KubectlResult struct {
	Stdout string
	Stderr string
}

// runKubectl executes a kubectl command with automatic retries on transient errors
func (d *Deployer) runKubectl(ctx context.Context, opts KubectlOptions) (*KubectlResult, error) {
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = 3
	}
	if opts.RetryDelay <= 0 {
		opts.RetryDelay = 2
	}

	// List of transient/retryable errors for kubectl
	retryableErrors := []string{
		"connection refused",
		"connection reset",
		"timed out",
		"timeout",
		"temporary failure",
		"network is unreachable",
		"no route to host",
		"tls handshake timeout",
		"eof",
		"broken pipe",
		"context deadline exceeded",
		"unable to connect to the server",
		"server is currently unable to handle the request",
		"the server is currently unable to handle the request",
		"etcdserver: request timed out",
		"etcdserver: leader changed",
		"transport is closing",
	}

	var lastStderr string
	var lastErr error

	for attempt := 1; attempt <= opts.MaxAttempts; attempt++ {
		if attempt > 1 {
			waitTime := time.Duration(attempt*opts.RetryDelay) * time.Second
			d.logger.PrintWithTimestamp(fmt.Sprintf("Retrying kubectl command (attempt %d/%d) after %v...", attempt, opts.MaxAttempts, waitTime))
			time.Sleep(waitTime)
		}

		cmd := exec.CommandContext(ctx, d.kubectl, opts.Args...)

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

		err := cmd.Run()
		lastStderr = stderr.String()
		lastErr = err

		if err == nil {
			return &KubectlResult{
				Stdout: stdout.String(),
				Stderr: lastStderr,
			}, nil
		}

		if opts.IgnoreErrors {
			return &KubectlResult{
				Stdout: stdout.String(),
				Stderr: lastStderr,
			}, nil
		}

		isRetryable := false
		stderrLower := strings.ToLower(lastStderr)
		for _, retryErr := range retryableErrors {
			if strings.Contains(stderrLower, strings.ToLower(retryErr)) {
				isRetryable = true
				break
			}
		}

		if !isRetryable || attempt == opts.MaxAttempts {
			return &KubectlResult{
				Stdout: stdout.String(),
				Stderr: lastStderr,
			}, fmt.Errorf("kubectl command failed: %w", err)
		}

		d.logger.Warning(fmt.Sprintf("Transient error in kubectl command: %s", lastStderr))
	}

	return &KubectlResult{
		Stdout: "",
		Stderr: lastStderr,
	}, fmt.Errorf("kubectl command failed after %d attempts: %w", opts.MaxAttempts, lastErr)
}
