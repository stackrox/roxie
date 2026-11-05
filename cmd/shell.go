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
	"github.com/stackrox/roxie/internal/k8s"
	log "github.com/stackrox/roxie/internal/logger"
	"github.com/stackrox/roxie/internal/manifest"
	"github.com/stackrox/roxie/internal/portforward"
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
			err := runShell(args)
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
	cmd.Flags().BoolVar(&evalMode, "eval", false, "Print environment as eval-able export statements to stdout")

	return cmd
}

func runShell(args []string) error {
	if evalMode && len(args) > 0 {
		return errors.New("--eval cannot be combined with command arguments")
	}

	if err := env.Initialize(); err != nil {
		return err
	}

	if os.Getenv("ROXIE_SHELL") != "" {
		return errors.New("already in a roxie sub-shell (ROXIE_SHELL environment variable is set), please exit the shell and try again")
	}

	log.Info("Loading manifest from cluster...")

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	m, err := manifest.LoadManifestSecret(ctx)
	if err != nil {
		return fmt.Errorf("failed to load roxie manifest: %w", err)
	}
	log.Dim("roxie manifest loaded")
	configBase := m.Config

	config, err := assembleConfigForCommand(&configBase, deploySettingsFromArgs, skipUserConfig)
	if err != nil {
		return err
	}

	// We need this for the setup of the CA cert.
	tempDir, err := os.MkdirTemp("", "roxie-shell-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	if !evalMode {
		defer os.RemoveAll(tempDir)
	}

	centralDeploymentInfo, err := manifest.ManifestToCentralDeploymentInfo(ctx, tempDir, m)
	if err != nil {
		return fmt.Errorf("extracting central deployment info from manifest: %w", err)
	}

	var portForwardPID int
	if config.Central.PortForwardingEnabled() {
		portForwarder := portforward.New(k8s.GetKubectl())
		if evalMode {
			endpoint, pid, err := portForwarder.StartDetached(config.Central.Namespace, "central", 443, 8443)
			if err != nil {
				return fmt.Errorf("failed to start port-forward: %w", err)
			}
			centralDeploymentInfo.Endpoint = endpoint
			portForwardPID = pid
		} else {
			endpoint, err := portForwarder.Start(config.Central.Namespace, "central", 443, 8443)
			if err != nil {
				return fmt.Errorf("failed to start port-forward: %w", err)
			}
			defer portForwarder.Stop()
			centralDeploymentInfo.Endpoint = endpoint
		}
		log.Infof("Port-forwarding Central at %s", centralDeploymentInfo.Endpoint)
	}

	if evalMode {
		printBanner(os.Stderr, centralDeploymentInfo)
		printExports(centralDeploymentInfo)
		if portForwardPID != 0 {
			fmt.Fprintf(os.Stdout, "export ROXIE_PORT_FORWARD_PID=%d\n", portForwardPID)
		}
		return nil
	}

	return runCommandOrSubshell(config.Roxie, centralDeploymentInfo, args)
}
