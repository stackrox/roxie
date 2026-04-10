package env

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/stackrox/roxie/internal/containerutil"
	"github.com/stackrox/roxie/internal/logger"
	"golang.org/x/term"
)

var (
	RunningInRoxieContainer bool
	RunningInteractively    bool
	initializationMutex     sync.Mutex
)

// ClusterType represents different types of Kubernetes clusters
type ClusterType int

const (
	// ClusterTypeUnknown represents an unidentified cluster type
	ClusterTypeUnknown ClusterType = iota
	// InfraGKE represents a GKE (Google Kubernetes Engine) cluster
	InfraGKE
	// InfraOpenShift4 represents an OpenShift 4 cluster
	InfraOpenShift4
	// LocalKind represents a Kind (Kubernetes in Docker) cluster
	LocalKind
)

var (
	// currentClusterType holds the detected cluster type for the current kubectl context
	// This is lazily populated on first access via GetCurrentClusterType()
	currentClusterType ClusterType

	// currentContext holds the name of the current kubectl context
	// This is lazily populated on first access via GetCurrentContext()
	currentContext string

	// initialized tracks whether we've performed the lazy initialization
	initialized bool
)

func init() {
	RunningInRoxieContainer = containerutil.IsRunningInRoxieContainer()
	if RunningInRoxieContainer {
		os.Setenv("KUBECONFIG", "/kubeconfig")
	}
	RunningInteractively = isRunningInteractively()
}

// isRunningInteractively detects if roxie is running interactively
// by checking if stdin, stdout, and stderr are all connected to a terminal.
func isRunningInteractively() bool {
	for _, f := range []*os.File{os.Stdin, os.Stdout, os.Stderr} {
		if !term.IsTerminal(int(f.Fd())) {
			return false
		}
	}
	return true
}

// ensureInitialized performs lazy initialization of cluster information
// This avoids contacting the cluster on package import
func ensureInitialized(log *logger.Logger) error {
	initializationMutex.Lock()
	defer initializationMutex.Unlock()

	if !initialized {
		kubeConfig, err := fetchKubeConfig(log)
		if err != nil {
			return err
		}
		currentContext = kubeConfig.CurrentContext
		apiResources, err := fetchAPIResources()
		if err != nil {
			return err
		}
		currentClusterType = detectClusterType(kubeConfig, apiResources)
		initialized = true
	}
	return nil
}

// GetCurrentClusterType returns the current cluster type, initializing if needed
func GetCurrentClusterType() ClusterType {
	panicIfNotInitialized()
	return currentClusterType
}

// GetCurrentContext returns the current kubectl context, initializing if needed
func GetCurrentContext() string {
	panicIfNotInitialized()
	return currentContext
}

func panicIfNotInitialized() {
	if !initialized {
		panic("environment information not initialized")
	}
}

// String returns the string representation of a ClusterType
func (ct ClusterType) String() string {
	switch ct {
	case InfraGKE:
		return "GKE"
	case InfraOpenShift4:
		return "OpenShift4"
	case LocalKind:
		return "Kind"
	default:
		return "Unknown"
	}
}

// KubeConfig represents a simplified kubectl configuration
type KubeConfig struct {
	CurrentContext string
	Clusters       []KubeCluster
}

// KubeCluster represents a cluster in the kubectl configuration
type KubeCluster struct {
	Name   string
	Server string
}

// Initialize performs environment initialization and sets the global variables.
// Retries on failure to handle race conditions during container startup, which I have
// observed in relation with podman :U mounts: the container was starting before the gcloud config
// was writable by the container user, hence GKE authentication failed immediately.
func Initialize(log *logger.Logger) error {
	if log == nil {
		log = logger.New()
	}
	if RunningInRoxieContainer {
		log.Dim("Running containerized.")
	}

	const maxRetries = 3
	const delay = 500 * time.Millisecond
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := ensureInitialized(log)
		if err == nil {
			return nil
		}

		lastErr = err

		if attempt < maxRetries {
			log.Dimf("Attempt %d/%d failed: %v, retrying...", attempt, maxRetries, err)
			time.Sleep(delay)
		}
	}

	return fmt.Errorf("failed to initialize environment after %d attempts: %w", maxRetries, lastErr)
}

// detectClusterType implements the cluster type detection logic
// This function is pure and testable - it doesn't invoke kubectl itself
func detectClusterType(config KubeConfig, apiResources []string) ClusterType {
	if config.CurrentContext == "" {
		return ClusterTypeUnknown
	}

	contextLower := strings.ToLower(config.CurrentContext)

	// Check for GKE clusters
	// GKE contexts have format: gke_PROJECT_ZONE_CLUSTER
	if strings.HasPrefix(config.CurrentContext, "gke_acs-team-temp-dev") {
		return InfraGKE
	}

	// Check for OpenShift 4 clusters by examining the server hostname
	if serverURL := getServerURL(config); serverURL != "" {
		if parsedURL, err := url.Parse(serverURL); err == nil {
			hostname := parsedURL.Hostname()
			if strings.HasSuffix(hostname, ".ocp.infra.rox.systems") {
				// Further verify it's OpenShift 4 by checking the API resources
				if isOpenShift4(apiResources) {
					return InfraOpenShift4
				}
			}
		}
	}

	// Check for Kind clusters
	// Kind clusters typically have context names starting with "kind-" or just "kind"
	if strings.HasPrefix(contextLower, "kind") {
		return LocalKind
	}

	return ClusterTypeUnknown
}

// getServerURL retrieves the server URL from the KubeConfig
func getServerURL(config KubeConfig) string {
	if len(config.Clusters) == 0 {
		return ""
	}
	return config.Clusters[0].Server
}

// isOpenShift4 checks if the cluster is running OpenShift 4.x by examining the API resources list
func isOpenShift4(apiResources []string) bool {
	// Check for the presence of the config.openshift.io API group
	// OpenShift 4 clusters have resources in this API group
	for _, resource := range apiResources {
		if strings.Contains(resource, "config.openshift.io") {
			return true
		}
	}
	return false
}

// fetchKubeConfig retrieves the current kubectl configuration
func fetchKubeConfig(log *logger.Logger) (KubeConfig, error) {
	if err := kubeconfigChecks(log); err != nil {
		return KubeConfig{}, err
	}
	// Get current context
	cmd := exec.Command("kubectl", "config", "current-context")
	output, err := cmd.Output()
	if err != nil {
		return KubeConfig{}, errors.New("failed to get current context")
	}
	currentContext := strings.TrimSpace(string(output))

	// Get cluster info
	cmd = exec.Command("kubectl", "config", "view", "--minify", "-o", "json")
	output, err = cmd.Output()
	if err != nil {
		return KubeConfig{}, fmt.Errorf("failed to obtain minified kubeconfig: %w", err)
	}

	var rawConfig struct {
		CurrentContext string `json:"current-context"`
		Clusters       []struct {
			Name    string `json:"name"`
			Cluster struct {
				Server string `json:"server"`
			} `json:"cluster"`
		} `json:"clusters"`
	}

	if err := json.Unmarshal(output, &rawConfig); err != nil {
		return KubeConfig{}, fmt.Errorf("failed to unmarshal kubeconfig: %w", err)
	}

	clusters := make([]KubeCluster, len(rawConfig.Clusters))
	for i, c := range rawConfig.Clusters {
		clusters[i] = KubeCluster{
			Name:   c.Name,
			Server: c.Cluster.Server,
		}
	}

	return KubeConfig{
		CurrentContext: currentContext,
		Clusters:       clusters,
	}, nil
}

func kubeconfigChecks(log *logger.Logger) error {
	kubeConfigPath, err := getKubeConfigPath()
	if err != nil {
		return fmt.Errorf("getting kubeconfig path: %w", err)
	}
	log.Infof("Using kubeconfig %s", kubeConfigPath)

	file, err := os.Open(kubeConfigPath)
	if err != nil {
		log.Warningf("Kubeconfig %s cannot be opened for reading.", kubeConfigPath)
		if errors.Is(err, os.ErrNotExist) {
			if RunningInRoxieContainer {
				log.Warningf("Make sure that your kubeconfig is mounted into the container, as in: -v $KUBECONFIG:/kubeconfig:U")
			}
		} else {
			if RunningInRoxieContainer {
				log.Warningf("Make sure that your kubeconfig is mounted with the 'U' option, as in: -v $KUBECONFIG:/kubeconfig:U")
			}
		}
		return fmt.Errorf("failed to open kubeconfig %q for reading: %w", kubeConfigPath, err)
	}
	_ = file.Close()
	return nil
}

func getKubeConfigPath() (string, error) {
	kubeConfigPath := os.Getenv("KUBECONFIG")
	if kubeConfigPath == "" {
		home := os.Getenv("HOME")
		if home == "" {
			return "", errors.New("HOME environment variable is not set")
		}
		kubeConfigPath = filepath.Join(home, ".kube", "config")
	}
	return kubeConfigPath, nil
}

// fetchAPIResources retrieves the list of API resources from the cluster
func fetchAPIResources() ([]string, error) {
	cmd := exec.Command("kubectl", "api-resources", "-o", "name")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve API resources: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	return lines, nil
}

func IsInStackroxRepository() bool {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	outputBytes, err := cmd.Output()
	if err != nil {
		return false
	}
	outputLines := strings.Split(string(outputBytes), "\n")
	if len(outputLines) == 0 {
		return false
	}
	return outputLines[0] == "git@github.com:stackrox/stackrox.git"
}

func GetStackroxRepositoryTag() (string, error) {
	topLevelDir, err := getStackRoxTopLevelDir()
	if err != nil {
		return "", fmt.Errorf("getting stackrox top level directory: %w", err)
	}
	cmd := exec.Command("make", "-s", "-C", topLevelDir, "tag")
	outputBytes, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("retrieving stackrox repository tag: %w", err)
	}
	tag := strings.TrimSpace(string(outputBytes))
	if strings.HasSuffix(tag, "-dirty") {
		return "", fmt.Errorf("stackrox repository is dirty")
	}
	return tag, nil
}

func getStackRoxTopLevelDir() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	outputBytes, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("getting stackrox top level directory: %w", err)
	}
	topLevelDir := strings.TrimSpace(string(outputBytes))
	if len(topLevelDir) == 0 {
		return "", fmt.Errorf("stackrox top level directory is empty")
	}
	return topLevelDir, nil
}
