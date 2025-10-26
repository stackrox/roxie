package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	// Version information set at build time via -ldflags
	version   = "dev"
	gitCommit = "unknown"
	buildDate = "unknown"
)

func newVersionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long:  `Print the version of roxie along with build information.`,
		Run:   runVersion,
	}

	return cmd
}

func runVersion(cmd *cobra.Command, args []string) {
	if gitCommit != "unknown" && gitCommit != "" {
		fmt.Printf("roxie version %s-%s", version, gitCommit)
	} else {
		fmt.Printf("roxie version %s", version)
	}

	if buildDate != "unknown" && buildDate != "" {
		fmt.Printf(" (%s)", buildDate)
	}

	fmt.Println()
}
