package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
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

func runLogsOperator(cmd *cobra.Command, args []string) error {
	// Build kubectl command
	kubectlArgs := []string{
		"-n", "rhacs-operator-system",
		"logs",
		"-l", "app=rhacs-operator",
	}

	if followLogs {
		kubectlArgs = append(kubectlArgs, "-f")
	}

	// Create context with timeout for initial connection
	// Use a longer timeout (30s) to allow for cluster connection
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	kubectlCmd := exec.CommandContext(ctx, "kubectl", kubectlArgs...)

	// Get stdout pipe for streaming
	stdout, err := kubectlCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	kubectlCmd.Stderr = os.Stderr

	if err := kubectlCmd.Start(); err != nil {
		return fmt.Errorf("failed to start kubectl: %w", err)
	}

	processLogStream(stdout)

	if err := kubectlCmd.Wait(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("timeout connecting to cluster - ensure your kubeconfig is correct and the cluster is reachable")
		}
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

	infoColor := color.New(color.FgWhite)
	debugColor := color.New(color.Faint)
	errorColor := color.New(color.FgRed, color.Bold)
	warnColor := color.New(color.FgYellow)

	for scanner.Scan() {
		line := scanner.Text()

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
