package portforward

import (
	"fmt"
	"net"
	"os/exec"
	"syscall"
	"time"

	"github.com/stackrox/roxie-golang/pkg/logger"
)

// Manager manages a kubectl port-forward subprocess and exposes a localhost endpoint
type Manager struct {
	kubectl   string
	logger    *logger.Logger
	proc      *exec.Cmd
	localPort int
}

// New creates a new PortForwardManager
func New(kubectl string, log *logger.Logger) *Manager {
	return &Manager{
		kubectl:   kubectl,
		logger:    log,
		localPort: 0,
	}
}

// findFreeLocalPort finds an available local port, preferring the given port
func (m *Manager) findFreeLocalPort(preferredPort int) (int, error) {
	// Try preferred port first
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", preferredPort))
	if err == nil {
		port := ln.Addr().(*net.TCPAddr).Port
		ln.Close()
		return port, nil
	}

	// Fall back to any available port
	ln, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port, nil
}

// waitTCPReady waits for a TCP port to become ready
func (m *Manager) waitTCPReady(host string, port int, timeoutSeconds float64) bool {
	timeout := time.Duration(timeoutSeconds * float64(time.Second))
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), 300*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}

	return false
}

// Start starts port-forward to the given service; returns "127.0.0.1:<port>"
// If already running, this is a no-op and returns the existing endpoint
func (m *Manager) Start(namespace, serviceName string, remotePort, preferredLocalPort int) (string, error) {
	// If already running, return existing endpoint
	if m.proc != nil && m.proc.Process != nil && m.localPort != 0 {
		return fmt.Sprintf("127.0.0.1:%d", m.localPort), nil
	}

	localPort, err := m.findFreeLocalPort(preferredLocalPort)
	if err != nil {
		return "", fmt.Errorf("failed to find free port: %w", err)
	}

	cmd := exec.Command(
		m.kubectl,
		"-n", namespace,
		"port-forward",
		fmt.Sprintf("svc/%s", serviceName),
		fmt.Sprintf("%d:%d", localPort, remotePort),
		"--address", "127.0.0.1",
	)

	// Set up process group for clean termination
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	cmd.Stdout = nil // Suppress output
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start port-forward: %w", err)
	}

	// Wait for port to be ready
	if !m.waitTCPReady("127.0.0.1", localPort, 20.0) {
		// Kill the process group
		if cmd.Process != nil {
			pgid, err := syscall.Getpgid(cmd.Process.Pid)
			if err == nil {
				syscall.Kill(-pgid, syscall.SIGTERM)
			}
			cmd.Wait()
		}
		return "", fmt.Errorf("port-forward did not become ready")
	}

	m.proc = cmd
	m.localPort = localPort
	endpoint := fmt.Sprintf("127.0.0.1:%d", localPort)
	m.logger.Successf("✓ Port-forward active at https://%s", endpoint)

	return endpoint, nil
}

// Stop stops the active port-forward if running
func (m *Manager) Stop() {
	if m.proc == nil || m.proc.Process == nil {
		return
	}

	// Try graceful termination first
	pgid, err := syscall.Getpgid(m.proc.Process.Pid)
	if err == nil {
		syscall.Kill(-pgid, syscall.SIGTERM)

		// Wait up to 2 seconds for graceful shutdown
		done := make(chan error, 1)
		go func() {
			done <- m.proc.Wait()
		}()

		select {
		case <-done:
			// Process terminated gracefully
		case <-time.After(2 * time.Second):
			// Force kill
			syscall.Kill(-pgid, syscall.SIGKILL)
		}
	}

	m.proc = nil
	m.localPort = 0
}
