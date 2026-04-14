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
	"github.com/stackrox/roxie/internal/logger"
)

func spawnSubshell(d *deployer.Deployer, log *logger.Logger) error {
	shellPath := shell
	if shellPath == "" {
		shellPath = os.Getenv("ROXIE_USER_SHELL")
	}
	if shellPath == "" {
		shellPath = os.Getenv("SHELL")
	}
	if shellPath == "" {
		shellPath = "/bin/bash"
	}

	log.Infof("Spawning sub-shell: %s", shellPath)

	env := os.Environ()

	endpoint, password, caCertFile, kubeContext, exposure := d.GetDeploymentInfo()

	if endpoint != "" {
		env = append(env, fmt.Sprintf("API_ENDPOINT=%s", endpoint))
		env = append(env, fmt.Sprintf("ROX_ENDPOINT=%s", endpoint))
		env = append(env, fmt.Sprintf("ROX_BASE_URL=https://%s", endpoint))
	}

	if password != "" {
		env = append(env, fmt.Sprintf("ROX_ADMIN_PASSWORD=%s", password))
	}

	if caCertFile != "" {
		env = append(env, fmt.Sprintf("ROX_CA_CERT_FILE=%s", caCertFile))
	}

	env = append(env, "ROXIE_SHELL=1")
	env = append(env, fmt.Sprintf("name=acs@%s", kubeContext))

	haproxyAvailable := isHAProxyAvailable()

	var haproxyCmd *exec.Cmd
	var haproxyConfigPath string
	var haproxyStarted bool

	if haproxyAvailable && endpoint != "" && caCertFile != "" {
		var err error
		haproxyCmd, haproxyConfigPath, err = startHAProxy(endpoint, caCertFile, log)
		if err != nil {
			log.Warningf("Failed to start HAProxy: %v", err)
		} else {
			env = append(env, fmt.Sprintf("ROXIE_HAPROXY_CFG_FILE=%s", haproxyConfigPath))
			haproxyStarted = true
			defer cleanupHAProxy(haproxyCmd, haproxyConfigPath)
		}
	}

	printBanner(endpoint, exposure, haproxyAvailable, haproxyStarted)

	shellCmd := exec.Command(shellPath, "-i")
	shellCmd.Env = env
	shellCmd.Stdin = os.Stdin
	shellCmd.Stdout = os.Stdout
	shellCmd.Stderr = os.Stderr

	err := shellCmd.Run()

	// Print exit message
	cyan := color.New(color.FgCyan, color.Bold)
	cyan.Println("\n[roxie] Exited subshell. You are now back in your original shell.")
	cyan.Println("")

	// Don't treat shell exit as an error - shells can exit with non-zero status
	// for various reasons (like the last command failing) which is normal behavior
	if err != nil {
		// Check if it's a normal exit (exit code from the shell)
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Shell exited (could be normal exit or last command failed)
			// This is not an error condition for roxie - the subshell worked fine
			_ = exitErr // Acknowledge we handled this
			return nil
		}
		// Only return error if we couldn't even start the shell
		return fmt.Errorf("failed to run subshell: %w", err)
	}

	return nil
}

func startHAProxy(endpoint, caCertFile string, log *logger.Logger) (*exec.Cmd, string, error) {
	configFile, err := os.CreateTemp("", "roxie-haproxy-*.cfg")
	if err != nil {
		return nil, "", fmt.Errorf("failed to create temp config: %w", err)
	}
	configPath := configFile.Name()

	haproxyConfig := fmt.Sprintf(`global
    log /dev/null local0

defaults
    log     global
    mode    http
    timeout connect 5s
    timeout client  30s
    timeout server  30s

frontend http_front
    bind *:8080 // this should probably be configurable?
    default_backend https_back

backend https_back
    server srv1 %s ssl verify required ca-file %s
`, endpoint, caCertFile)

	if _, err := configFile.WriteString(haproxyConfig); err != nil {
		configFile.Close()
		os.Remove(configPath)
		return nil, "", fmt.Errorf("failed to write haproxy config: %w", err)
	}
	configFile.Close()

	cmd := exec.Command("haproxy", "-f", configPath)
	// What about stdin? We probably should prevent haproxy from reading from the terminal just in case...
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		os.Remove(configPath)
		return nil, "", fmt.Errorf("failed to start haproxy: %w", err)
	}

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

func printBanner(endpoint, exposure string, haproxyAvailable, haproxyStarted bool) {
	cyan := color.New(color.FgCyan, color.Bold)
	cyan.Println("\n[roxie] Entering a subshell with ACS environment variables set.")
	cyan.Println("[roxie]")
	cyan.Println("[roxie] Environment is set up for talking to ACS Central. Examples:")
	cyan.Println("[roxie]")
	cyan.Println("[roxie]   * roxctl central whoami")
	cyan.Println("[roxie]   * roxcurl /v1/clusters")
	cyan.Println("[roxie]")

	if haproxyStarted {
		cyan.Println("[roxie] Central UI: http://localhost:8080 (username: admin, password: see $ROX_ADMIN_PASSWORD)")
	} else if exposure != "none" && exposure != "" {
		cyan.Printf("[roxie] Central UI: https://%s", endpoint)
	} else if !env.RunningInRoxieContainer {
		cyan.Println("[roxie] Note: Installing haproxy enables automatic HTTP access to Central at http://localhost:8080")
	}

	cyan.Println("")
}
