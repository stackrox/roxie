package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/stackrox/roxie/internal/deployer"
	"github.com/stackrox/roxie/internal/env"
	"github.com/stackrox/roxie/internal/haproxy"
	"github.com/stackrox/roxie/internal/logger"
	"github.com/stackrox/roxie/internal/roxieenv"
	"github.com/stackrox/roxie/internal/types"
)

// spawnSubshellForDeployerEnv assembles the roxie environment from a Deployer and invokes an interactive subshell.
func spawnSubshellForDeployerEnv(roxieConfig deployer.RoxieConfig, d *deployer.Deployer, log *logger.Logger) error {
	return runCommandOrSubshell(roxieConfig, d.GetCentralDeploymentInfo(), log, nil)
}

// runCommandOrSubshell spawns an interactive subshell or runs the provided command using the given
// central deployment info.
// It handles HAProxy setup, prints the connection banner, and manages shell lifecycle.
func runCommandOrSubshell(roxieConfig deployer.RoxieConfig, centralDeploymentInfo types.CentralDeploymentInfo, log *logger.Logger, args []string) error {
	cmdEnv := os.Environ()
	for name, val := range roxieenv.AssembleRoxieEnvironment(centralDeploymentInfo).Export() {
		cmdEnv = append(cmdEnv, fmt.Sprintf("%s=%s", name, val))
	}
	cmdEnv = append(cmdEnv, "ROXIE_SHELL=1")
	cmdEnv = append(cmdEnv, fmt.Sprintf("name=acs@%s", centralDeploymentInfo.KubeContext))

	cleanupFunc, err := tryStartHAProxy(log, roxieConfig, &centralDeploymentInfo)
	if err != nil {
		log.Warningf("Failed to start HAProxy: %v", err)
	}
	if cleanupFunc != nil {
		defer cleanupFunc()
	}

	var cmd *exec.Cmd

	if subShellMode(args) {
		shellPath := resolveShellPath()
		log.Infof("Spawning sub-shell: %s", shellPath)
		printBanner(centralDeploymentInfo)
		cmd = exec.Command(shellPath, "-i")
	} else {
		// args is non-empty.
		cmd = exec.Command(args[0], args[1:]...)
	}
	cmd.Env = cmdEnv
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()

	if subShellMode(args) {
		cyan := color.New(color.FgCyan, color.Bold)
		cyan.Println("")
		cyan.Println("[roxie] Exited subshell. You are now back in your original shell.")
		cyan.Println("[roxie] If you accidentally closed the roxie subshell, you can use `roxie shell` to re-open it.")
		cyan.Println("")

		// Don't treat shell exit as an error - shells can exit with non-zero status
		// for various reasons (like the last command failing) which is normal behavior
		if err != nil {
			// Check if it's a normal exit (exit code from the shell)
			if _, ok := err.(*exec.ExitError); ok {
				return nil
			}
			// Only return error if we couldn't even start the shell
			return fmt.Errorf("failed to run subshell: %w", err)
		}
	} else {
		if err != nil {
			return fmt.Errorf("failed to execute command: %w", err)
		}
	}

	return nil
}

func tryStartHAProxy(
	log *logger.Logger,
	roxieConfig deployer.RoxieConfig,
	centralDeploymentInfo *types.CentralDeploymentInfo) (func(), error) {

	if !isHAProxyAvailable() {
		log.Dim("No HAProxy available, skipping")
		return nil, nil
	}

	if centralDeploymentInfo.Endpoint == "" {
		log.Warning("No Central endpoint available, skipping")
		return nil, nil
	}
	if centralDeploymentInfo.CACertFile == "" {
		log.Warning("No Central CA Cert available, skipping")
		return nil, nil
	}

	haproxyCmd, haproxyConfigPath, err := startHAProxy(roxieConfig, centralDeploymentInfo)
	if err != nil {
		return nil, err
	}

	log.Infof("Started HAProxy at localhost:%v", centralDeploymentInfo.HAProxyPort)

	return func() {
		cleanupHAProxy(haproxyCmd, haproxyConfigPath)
	}, nil
}

func subShellMode(args []string) bool {
	return len(args) == 0
}

func resolveShellPath() string {
	if shell != "" {
		return shell
	}
	if s := os.Getenv("ROXIE_USER_SHELL"); s != "" {
		return s
	}
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	return "/bin/bash"
}

func startHAProxy(roxieConfig deployer.RoxieConfig, centralDeploymentInfo *types.CentralDeploymentInfo) (*exec.Cmd, string, error) {
	configFile, err := os.CreateTemp("", "roxie-haproxy-*.cfg")
	if err != nil {
		return nil, "", fmt.Errorf("failed to create temp config: %w", err)
	}
	configPath := configFile.Name()

	haproxyCfg := haproxy.RenderConfig(roxieConfig.HAProxy, centralDeploymentInfo.Endpoint, centralDeploymentInfo.CACertFile)

	if _, err := configFile.WriteString(haproxyCfg); err != nil {
		configFile.Close()
		os.Remove(configPath)
		return nil, "", fmt.Errorf("failed to write haproxy config: %w", err)
	}
	configFile.Close()

	cmd := exec.Command("haproxy", "-f", configPath)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		os.Remove(configPath)
		return nil, "", fmt.Errorf("failed to start haproxy: %w", err)
	}

	centralDeploymentInfo.HAProxyPort = roxieConfig.HAProxy.BindPort

	return cmd, configPath, nil
}

func cleanupHAProxy(cmd *exec.Cmd, configPath string) {
	if cmd != nil && cmd.Process != nil {
		cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
		}()

		select {
		case <-done:
		case <-time.After(3 * time.Second):
			cmd.Process.Kill()
		}
	}

	if configPath != "" {
		os.Remove(configPath)
	}
}

// isHAProxyAvailable checks if haproxy is available in PATH
func isHAProxyAvailable() bool {
	_, err := exec.LookPath("haproxy")
	return err == nil
}

func printBanner(centralDeploymentInfo types.CentralDeploymentInfo) {
	cyan := color.New(color.FgCyan, color.Bold)
	cyan.Println("\n[roxie] Entering a subshell with ACS environment variables set.")
	cyan.Println("[roxie]")
	cyan.Println("[roxie] Environment is set up for talking to ACS Central. Examples:")
	cyan.Println("[roxie]")
	cyan.Println("[roxie]   * roxctl central whoami")
	cyan.Println("[roxie]   * roxcurl /v1/clusters")
	cyan.Println("[roxie]")

	if centralDeploymentInfo.HAProxyStarted {
		cyan.Println("[roxie] Central UI: http://localhost:8080 (username: admin, password: see $ROX_ADMIN_PASSWORD)")
	} else if centralDeploymentInfo.Exposure != types.ExposureNone {
		cyan.Printf("[roxie] Central UI: https://%s", centralDeploymentInfo.Endpoint)
	} else if !env.RunningInRoxieContainer {
		cyan.Println("[roxie] Note: Installing haproxy enables automatic HTTP access to Central at http://localhost:8080")
	}

	cyan.Println("")
}
