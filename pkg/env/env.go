package env

import (
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/stackrox/roxie-golang/pkg/containerutil"
)

var (
	RunningInContainer bool
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
	// CurrentClusterType holds the detected cluster type for the current kubectl context
	// This is automatically populated during package initialization
	CurrentClusterType ClusterType

	// CurrentContext holds the name of the current kubectl context
	// This is automatically populated during package initialization
	CurrentContext string
)

func init() {
	RunningInContainer = containerutil.IsRunningInContainer()
	if RunningInContainer {
		os.Setenv("KUBECONFIG", "/kubeconfig")
	}
	kubeConfig := fetchKubeConfig()
	CurrentContext = kubeConfig.CurrentContext
	apiResources := fetchAPIResources()
	CurrentClusterType = detectClusterType(kubeConfig, apiResources)
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

// DetectClusterType identifies the cluster type for the current kubectl context
// This is a convenience wrapper that fetches the kubeconfig and API resources,
// then delegates to detectClusterType for the actual detection logic
func DetectClusterType() ClusterType {
	kubeConfig := fetchKubeConfig()
	apiResources := fetchAPIResources()
	return detectClusterType(kubeConfig, apiResources)
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
func fetchKubeConfig() KubeConfig {
	// Get current context
	cmd := exec.Command("kubectl", "config", "current-context")
	output, err := cmd.Output()
	if err != nil {
		return KubeConfig{}
	}
	currentContext := strings.TrimSpace(string(output))

	// Get cluster info
	cmd = exec.Command("kubectl", "config", "view", "--minify", "-o", "json")
	output, err = cmd.Output()
	if err != nil {
		return KubeConfig{CurrentContext: currentContext}
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
		return KubeConfig{CurrentContext: currentContext}
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
	}
}

// fetchAPIResources retrieves the list of API resources from the cluster
func fetchAPIResources() []string {
	cmd := exec.Command("kubectl", "api-resources", "-o", "name")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	return lines
}
