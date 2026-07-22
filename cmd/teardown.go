package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/stackrox/roxie/internal/component"
	"github.com/stackrox/roxie/internal/deployer"
	"github.com/stackrox/roxie/internal/env"
	"github.com/stackrox/roxie/internal/manifest"
)

func newTeardownCmd(settings *deployer.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:       "teardown [component]",
		Short:     "Teardown ACS components",
		Long:      `Teardown ACS components (central, secured-cluster, or both).`,
		ValidArgs: []string{"central", "secured-cluster", "both"},
		Args:      cobra.MaximumNArgs(1),
		RunE:      runTeardown,
	}

	registerFlag(cmd, settings, "single-namespace", "Deploy all components in a single namespace ('stackrox')",
		withNoOptDefVal("true"),
		withApplyFnBool(func(config *deployer.Config, val bool) error {
			// We do not support --single-namespace=false as of now.
			if val {
				config.Central.Namespace = sharedNamespace
				config.SecuredCluster.Namespace = sharedNamespace
			}
			return nil
		}),
	)

	return cmd
}

func runTeardown(cmd *cobra.Command, args []string) error {
	log := globalLogger
	if err := env.Initialize(log); err != nil {
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

	deploySettings, err := assembleConfigForCommand(nil, deploySettingsFromArgs, skipUserConfig)
	if err != nil {
		return err
	}

	d, err := deployer.New(log)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}
	defer d.Cleanup()

	d.SetConfig(deploySettings)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	if err := d.Teardown(ctx, components); err != nil {
		return fmt.Errorf("teardown failed: %w", err)
	}

	if components.IncludesCentral() {
		if err := manifest.DeleteManifestSecret(ctx, log); err != nil {
			log.Warningf("Failed to delete roxie manifest: %v", err)
		}
	}
	if components == component.All {
		if err := manifest.DeleteRoxieNamespace(ctx, log); err != nil {
			log.Warningf("Failed to delete roxie namespace: %v", err)
		}
	}

	log.Success("🎉 Teardown complete!")

	return nil
}
