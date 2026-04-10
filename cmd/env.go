package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/stackrox/roxie/internal/env"
	"github.com/stackrox/roxie/internal/logger"
)

func newEnvCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "env",
		Short:  "Display environment information",
		Long:   `Display detected environment information including cluster type and container status.`,
		Hidden: true, // Hidden command for debugging/inspection
		RunE:   runEnv,
	}

	return cmd
}

func runEnv(cmd *cobra.Command, args []string) error {
	log := logger.New()
	if err := env.Initialize(log); err != nil {
		return err
	}

	fmt.Println("Roxie Environment Information:")
	fmt.Println("==============================")
	fmt.Printf("Kube config:                %s\n", os.Getenv("KUBECONFIG"))
	fmt.Printf("Running in roxie container: %v\n", env.RunningInRoxieContainer)
	fmt.Printf("Current Context:            %s\n", env.GetCurrentContext())
	fmt.Printf("Cluster Type:               %s\n", env.GetCurrentClusterType().String())

	return nil
}
