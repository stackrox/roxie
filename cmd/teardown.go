package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/stackrox/roxie/pkg/deployer"
	"github.com/stackrox/roxie/pkg/logger"
)

func newTeardownCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:       "teardown [component]",
		Short:     "Teardown ACS components",
		Long:      `Teardown ACS components (central, secured-cluster, or both).`,
		ValidArgs: []string{"central", "secured-cluster", "both"},
		Args:      cobra.MaximumNArgs(1),
		RunE:      runTeardown,
	}

	cmd.Flags().BoolVar(&helm, "helm", false, "Force teardown of Helm deployment")

	return cmd
}

func runTeardown(cmd *cobra.Command, args []string) error {
	component := "both"
	if len(args) > 0 {
		component = args[0]
	}

	log := logger.New()

	log.Infof("Tearing down %s", component)

	d, err := deployer.New(log, "", []string{})
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	if err := d.Teardown(ctx, component); err != nil {
		return fmt.Errorf("teardown failed: %w", err)
	}

	log.Success("🎉 Teardown complete!")

	return nil
}
