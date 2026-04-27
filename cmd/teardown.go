package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/stackrox/roxie/internal/component"
	"github.com/stackrox/roxie/internal/deployer"
	"github.com/stackrox/roxie/internal/env"
	"github.com/stackrox/roxie/internal/logger"
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

	cmd.Flags().BoolVar(&singleNamespace, "single-namespace", false, "Deploy all components in a single namespace ('stackrox' by default)")

	return cmd
}

func runTeardown(cmd *cobra.Command, args []string) error {
	log := logger.New()
	if err := env.Initialize(log); err != nil {
		return err
	}

	components, err := component.FromArgs(args)
	if err != nil {
		return err
	}

	log.Infof("Tearing down %s", components)

	d, err := deployer.New(log)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}
	defer func() {
		if err := d.Cleanup(); err != nil {
			log.Warningf("Failed to invoke deployer cleanup: %v", err)
		}
	}()

	d.SetSingleNamespace(singleNamespace)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	if err := d.Teardown(ctx, components); err != nil {
		return fmt.Errorf("teardown failed: %w", err)
	}

	log.Success("🎉 Teardown complete!")

	return nil
}
