package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/stackrox/roxie/internal/component"
	"github.com/stackrox/roxie/internal/deployer"
	"github.com/stackrox/roxie/internal/env"
	log "github.com/stackrox/roxie/internal/logger"
	"github.com/stackrox/roxie/internal/manifest"
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

	// Accepted for backwards compatibility but ignored — teardown now discovers namespaces from the cluster.
	cmd.Flags().Bool("single-namespace", false, "Deprecated: teardown discovers namespaces automatically")
	_ = cmd.Flags().MarkDeprecated("single-namespace", "teardown now discovers namespaces automatically")

	return cmd
}

func runTeardown(cmd *cobra.Command, args []string) error {
	if err := env.Initialize(); err != nil {
		return err
	}

	components, err := component.FromArgs(args)
	if err != nil {
		return err
	}

	log.Infof("Tearing down %s", components)

	if dryRun {
		log.Infof("Exiting because of enabled dry-run mode.")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	d, err := deployer.New()
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}
	defer d.Cleanup()

	if err := d.Teardown(ctx, components); err != nil {
		return fmt.Errorf("teardown failed: %w", err)
	}

	if components.IncludesCentral() {
		if err := manifest.DeleteManifestSecret(ctx); err != nil {
			log.Warningf("Failed to delete roxie manifest: %v", err)
		}
	}
	if components == component.All {
		if err := manifest.DeleteRoxieNamespace(ctx); err != nil {
			log.Warningf("Failed to delete roxie namespace: %v", err)
		}
	}

	log.Success("🎉 Teardown complete!")

	return nil
}
