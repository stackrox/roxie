package main

import (
	"fmt"
	"os"

	"github.com/creativeprojects/go-selfupdate"
	"github.com/spf13/cobra"
	log "github.com/stackrox/roxie/internal/logger"
)

const (
	updateRepoOwner = "stackrox"
	updateRepoName  = "roxie"
)

func newUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update roxie to the latest version",
		Long:  `Download and install the latest roxie release from GitHub, replacing the current binary in-place.`,
		RunE:  runUpdate,
	}
}

func runUpdate(cmd *cobra.Command, _ []string) error {
	if version == "dev" {
		return fmt.Errorf("cannot self-update a development build (version %q); install a released binary first", version)
	}

	updater, err := selfupdate.NewUpdater(selfupdate.Config{
		Validator: &selfupdate.ChecksumValidator{UniqueFilename: "checksums.txt"},
	})
	if err != nil {
		return fmt.Errorf("initializing updater: %w", err)
	}

	log.Infof("Checking for updates (current version: %s)...", version)

	latest, found, err := updater.DetectLatest(cmd.Context(), selfupdate.ParseSlug(updateRepoOwner+"/"+updateRepoName))
	if err != nil {
		return fmt.Errorf("detecting latest version: %w", err)
	}
	if !found {
		return fmt.Errorf("no releases found for %s/%s", updateRepoOwner, updateRepoName)
	}

	if latest.LessOrEqual(version) {
		log.Infof("Already up to date (latest: %s).", latest.Version())
		return nil
	}

	log.Infof("Updating to %s...", latest.Version())

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locating current executable: %w", err)
	}

	if err := updater.UpdateTo(cmd.Context(), latest, exe); err != nil {
		return fmt.Errorf("updating binary: %w", err)
	}

	log.Infof("Updated roxie: %s → %s", version, latest.Version())
	return nil
}
