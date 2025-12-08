package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/stackrox/roxie/internal/logger"
)

var (
	followLogs bool
)

func newLogsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "View logs for ACS components",
		Long:  `View logs for various ACS components.`,
	}

	cmd.AddCommand(newLogsOperatorCmd())

	return cmd
}

func newLogsOperatorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "operator",
		Short: "View logs for the RHACS operator",
		Long:  `View logs for the RHACS operator running in the rhacs-operator-system namespace.`,
		RunE:  runLogsOperator,
	}

	cmd.Flags().BoolVarP(&followLogs, "follow", "f", false, "Follow log output (stream logs)")

	return cmd
}

// validateClusterConnection checks if the cluster is reachable.
func validateClusterConnection(logger logger.Logger, ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "kubectl", "get", "namespaces")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = io.Discard

	if err := cmd.Run(); err != nil {
		for _, stderrLine := range strings.Split(stderr.String(), "\n") {
			logger.Dim(stderrLine)
		}
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("timeout connecting to cluster - ensure your kubeconfig is correct and the cluster is reachable")
		}
		return fmt.Errorf("failed to connect to cluster: %w", err)
	}
	return nil
}

func runLogsOperator(_ *cobra.Command, args []string) error {
	logger := logger.New()

	// Validate cluster connection first (fast-fail)
	validateCtx, validateCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer validateCancel()

	if err := validateClusterConnection(logger, validateCtx); err != nil {
		validateCancel()
		return err
	}

	// Build kubectl command
	kubectlArgs := []string{
		"-n", "rhacs-operator-system",
		"logs",
		"-l", "app=rhacs-operator",
	}

	if followLogs {
		kubectlArgs = append(kubectlArgs, "-f")
	}

	// Create kubectl command without context - allows indefinite streaming.
	kubectlCmd := exec.Command("kubectl", kubectlArgs...)

	// Get stdout pipe for streaming
	stdout, err := kubectlCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// Pass through stderr directly
	kubectlCmd.Stderr = os.Stderr

	// Start the command
	if err := kubectlCmd.Start(); err != nil {
		return fmt.Errorf("failed to start kubectl: %w", err)
	}

	// Process stdout with color coding
	processLogStream(stdout)

	// Wait for command to complete
	if err := kubectlCmd.Wait(); err != nil {
		// Don't return error if kubectl was interrupted (e.g., Ctrl+C)
		if _, ok := err.(*exec.ExitError); ok {
			return nil
		}
		return fmt.Errorf("kubectl command failed: %w", err)
	}

	return nil
}

func processLogStream(reader io.Reader) {
	scanner := bufio.NewScanner(reader)

	// Define colors for different log levels
	infoColor := color.New(color.FgWhite)
	debugColor := color.New(color.Faint)
	errorColor := color.New(color.FgRed, color.Bold)
	warnColor := color.New(color.FgYellow)

	for scanner.Scan() {
		line := scanner.Text()

		// Determine log level and apply appropriate color
		switch {
		case strings.Contains(line, "ERROR"):
			errorColor.Println(line)
		case strings.Contains(line, "WARN"):
			warnColor.Println(line)
		case strings.Contains(line, "DEBUG"):
			debugColor.Println(line)
		case strings.Contains(line, "INFO"):
			infoColor.Println(line)
		default:
			// Default to info color for unrecognized lines
			infoColor.Println(line)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading logs: %v\n", err)
	}
}
