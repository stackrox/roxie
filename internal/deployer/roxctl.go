package deployer

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// RoxctlOptions contains options for running roxctl commands
type RoxctlOptions struct {
	Args              []string          // Command arguments (e.g., ["central", "crs", "generate", "cluster-name"])
	Env               map[string]string // Additional environment variables
	UseAuthentication bool              // Whether to set ROX_ADMIN_PASSWORD and ROX_CA_CERT_FILE
	MaxAttempts       int               // Maximum number of retry attempts (default: 5)
	RetryDelay        int               // Base retry delay in seconds (default: 10, multiplied by attempt number)
}

// RoxctlResult contains the result of a roxctl command execution
type RoxctlResult struct {
	Stdout string
	Stderr string
}

// runRoxctl executes a roxctl command with automatic retries on transient errors
func (d *Deployer) runRoxctl(ctx context.Context, opts RoxctlOptions) (*RoxctlResult, error) {
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = 5
	}
	if opts.RetryDelay <= 0 {
		opts.RetryDelay = 10
	}

	// List of transient/retryable errors
	retryableErrors := []string{
		"connection refused",
		"connection reset",
		"connection timed out",
		"timed out",
		"timeout",
		"network is unreachable",
		"temporary failure in name resolution",
		"no route to host",
		"tls handshake timeout",
		"eof",
		"bad gateway",
		"service unavailable",
		"context deadline exceeded",
		"no such host",
	}

	var lastStderr string
	var lastErr error

	for attempt := 1; attempt <= opts.MaxAttempts; attempt++ {
		if attempt > 1 {
			waitTime := time.Duration(attempt*opts.RetryDelay) * time.Second
			d.logger.Infof("Retrying roxctl command (attempt %d/%d) after %v...", attempt, opts.MaxAttempts, waitTime)
			time.Sleep(waitTime)
		}

		cmd := exec.CommandContext(ctx, "roxctl", opts.Args...)

		cmd.Env = os.Environ()

		if opts.UseAuthentication {
			if d.centralPassword != "" {
				cmd.Env = append(cmd.Env, fmt.Sprintf("ROX_ADMIN_PASSWORD=%s", d.centralPassword))
			}
			if d.roxCACertFile != "" {
				cmd.Env = append(cmd.Env, fmt.Sprintf("ROX_CA_CERT_FILE=%s", d.roxCACertFile))
			}
		}

		for k, v := range opts.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		lastStderr = stderr.String()
		lastErr = err

		if err == nil {
			return &RoxctlResult{
				Stdout: stdout.String(),
				Stderr: lastStderr,
			}, nil
		}

		isRetryable := false
		for _, retryErr := range retryableErrors {
			if strings.Contains(strings.ToLower(lastStderr), strings.ToLower(retryErr)) {
				isRetryable = true
				break
			}
		}

		if !isRetryable || attempt == opts.MaxAttempts {
			d.logger.Errorf("roxctl error: %s", lastStderr)
			return nil, fmt.Errorf("roxctl command failed: %w", err)
		}

		d.logger.Warningf("Transient error in roxctl command: %s", lastStderr)
	}

	return nil, fmt.Errorf("roxctl command failed after %d attempts: %w", opts.MaxAttempts, lastErr)
}
