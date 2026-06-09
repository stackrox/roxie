package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"
	"github.com/stackrox/roxie/internal/env"
	"github.com/stackrox/roxie/internal/logger"
	"github.com/stackrox/roxie/internal/manifest"
)

func newShellCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shell [-- command [args...]]",
		Short: "Open a subshell for an existing ACS Central deployment",
		Long: `Open an interactive subshell with ACS environment variables
set for an existing ACS Central deployment.

This command reads the roxie manifest secret from the cluster,
re-fetches the CA certificate, and spawns an interactive subshell
with the environment variables set.

If a command is given after "--", it is executed in the modified environment
instead of spawning a subshell.

Examples:
  roxie shell
  roxie shell -- roxctl central whoami
  roxie shell -- bash -c 'echo $ROX_ENDPOINT'`,
		Run: func(cmd *cobra.Command, args []string) {
			err := runShell(cmd, args)
			if err != nil {
				if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
					// Propagate exit error from the child process.
					os.Exit(exitErr.ExitCode())
				}
				cmd.PrintErrln(err)
				os.Exit(1)
			}
		},
		DisableFlagParsing: false,
	}

	cmd.Flags().StringVar(&shell, "shell", "", "Shell to spawn")

	return cmd
}

func runShell(cmd *cobra.Command, args []string) error {
	log := logger.New()
	if err := env.Initialize(log); err != nil {
		return err
	}

	if os.Getenv("ROXIE_SHELL") != "" {
		return errors.New("already in a roxie sub-shell (ROXIE_SHELL environment variable is set), please exit the shell and try again")
	}

	log.Info("Loading manifest from cluster...")

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	m, err := manifest.LoadManifestSecret(ctx, log)
	if err != nil {
		return fmt.Errorf("failed to load roxie manifest: %w", err)
	}
	log.Dim("roxie manifest loaded")

	// We need this for the setup of the CA cert.
	tempDir, err := os.MkdirTemp("", "roxie-shell-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	centralDeploymentInfo, err := manifest.ManifestToCentralDeploymentInfo(ctx, log, tempDir, m)
	if err != nil {
		return fmt.Errorf("extracting central deployment info from manifest: %w", err)
	}

	return runCommandOrSubshell(centralDeploymentInfo, log, args)
}
