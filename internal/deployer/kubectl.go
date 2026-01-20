package deployer

import (
	"context"
	"io"
	"strings"

	"github.com/stackrox/roxie/internal/helpers"
)

// KubectlOptions contains options for running kubectl commands
type KubectlOptions struct {
	Args                 []string  // Command arguments (e.g., ["get", "pod", "my-pod"])
	Stdin                io.Reader // Optional stdin for commands like "apply -f -"
	MaxAttempts          int       // Maximum number of retry attempts (default: 3)
	RetryDelay           int       // Base retry delay in seconds (default: 2)
	IgnoreErrors         bool      // If true, return nil on errors (useful for cleanup operations)
	SkipLoggingOnFailure bool      // If true, don't log on failures.
}

// runKubectl executes a kubectl command with automatic retries on transient errors
func (d *Deployer) runKubectl(ctx context.Context, opts KubectlOptions) (*helpers.ExecResult, error) {
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

	retryablePredicate := func(stderr string) bool {
		stderrLower := strings.ToLower(stderr)
		for _, retryErr := range retryableErrors {
			if strings.Contains(stderrLower, strings.ToLower(retryErr)) {
				return true
			}
		}
		return false
	}

	args := append([]string{d.kubectl}, opts.Args...)
	return helpers.Exec(ctx, args,
		helpers.WithStdin(opts.Stdin),
		helpers.WithRetryablePredicate(retryablePredicate),
		helpers.WithMaxAttempts(opts.MaxAttempts),
		helpers.WithRetryDelay(opts.RetryDelay),
		helpers.WithIgnoreErrors(opts.IgnoreErrors),
		helpers.WithSkipLoggingOnFailure(opts.SkipLoggingOnFailure),
	)
}
