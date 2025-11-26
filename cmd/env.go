package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/stackrox/roxie/internal/env"
)

func newEnvCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "env",
		Short:  "Display environment information",
		Long:   `Display detected environment information including cluster type and container status.`,
		Hidden: true, // Hidden command for debugging/inspection
		Run:    runEnv,
	}

	return cmd
}

func runEnv(cmd *cobra.Command, args []string) {
	fmt.Println("Roxie Environment Information:")
	fmt.Println("==============================")
	fmt.Printf("Running in Container: %v\n", env.RunningInContainer)
	fmt.Printf("Current Context:      %s\n", env.GetCurrentContext())
	fmt.Printf("Cluster Type:         %s\n", env.GetCurrentClusterType().String())
}
