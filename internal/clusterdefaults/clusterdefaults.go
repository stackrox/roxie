package clusterdefaults

import (
	"strings"

	"github.com/stackrox/roxie/internal/logger"
)

// ClusterType represents different types of Kubernetes clusters
type ClusterType int

const (
	// ClusterTypeUnknown represents an unidentified cluster type
	ClusterTypeUnknown ClusterType = iota
	// ClusterTypeKind represents a Kind (Kubernetes in Docker) cluster
	ClusterTypeKind
	// ClusterTypeMinikube represents a Minikube cluster
	ClusterTypeMinikube
	// ClusterTypeK3s represents a K3s cluster
	ClusterTypeK3s
	// ClusterTypeCRC represents a CRC (CodeReady Containers) cluster
	ClusterTypeCRC
)

// String returns the string representation of a ClusterType
func (ct ClusterType) String() string {
	switch ct {
	case ClusterTypeKind:
		return "kind"
	case ClusterTypeMinikube:
		return "minikube"
	case ClusterTypeK3s:
		return "k3s"
	case ClusterTypeCRC:
		return "crc"
	default:
		return "unknown"
	}
}

// TODO(#91): Maybe I'm missing something, but this manager/detector/applicator abstraction
// seems massively over-engineered since the is only one concrete implementation. AFAICT this
// could all be just a single function that the deployer calls with log, kubeconfig, resources,
// exposure and portForward.

// DeploymentDefaults holds the recommended defaults for a cluster type
type DeploymentDefaults struct {
	Resources          string // Resource preset (e.g., "small", "default")
	Exposure           string // Exposure mode (e.g., "none", "loadbalancer")
	PortForwardEnabled bool   // Whether port-forwarding should be enabled
}

// Detector interface for identifying cluster types
type Detector interface {
	// Detect returns the cluster type based on the kube context name
	Detect(kubeContext string) ClusterType
}

// Applicator interface for applying cluster-specific defaults
type Applicator interface {
	// Apply returns adjusted deployment parameters based on cluster type
	Apply(clusterType ClusterType, resources, exposure string, portForwardEnabled bool) (
		adjustedResources string,
		adjustedExposure string,
		adjustedPortForward bool,
		changed bool,
	)
}

// Manager coordinates cluster detection and default application
type Manager struct {
	detector   Detector
	applicator Applicator
	logger     *logger.Logger
}

// NewManager creates a new cluster defaults manager
func NewManager(log *logger.Logger) *Manager {
	return &Manager{
		detector:   &defaultDetector{},
		applicator: &defaultApplicator{},
		logger:     log,
	}
}

// ApplyConvenienceDefaults detects the cluster type and applies appropriate defaults
func (m *Manager) ApplyConvenienceDefaults(kubeContext, resources, exposure string, portForwardEnabled bool) (
	adjustedResources string,
	adjustedExposure string,
	adjustedPortForward bool,
) {
	// Detect cluster type
	clusterType := m.detector.Detect(kubeContext)

	// Apply defaults based on cluster type
	adjRes, adjExp, adjPF, changed := m.applicator.Apply(
		clusterType,
		resources,
		exposure,
		portForwardEnabled,
	)

	// Log if defaults were applied
	if changed {
		m.logDefaultsApplied(clusterType, adjRes, adjExp, adjPF)
	}

	return adjRes, adjExp, adjPF
}

// logDefaultsApplied logs a message when cluster-specific defaults are applied
func (m *Manager) logDefaultsApplied(clusterType ClusterType, resources, exposure string, portForward bool) {
	pfStatus := "with port-forwarding"
	if !portForward {
		pfStatus = "without port-forwarding"
	}

	m.logger.Warning(
		"Detected " + clusterType.String() + " cluster: using --resources=" +
			resources + " --exposure=" + exposure + " " + pfStatus,
	)
}

// defaultDetector implements the Detector interface
type defaultDetector struct{}

// Detect identifies the cluster type based on kube context name
func (d *defaultDetector) Detect(kubeContext string) ClusterType {
	contextLower := strings.ToLower(kubeContext)

	// Kind clusters typically have context names starting with "kind-"
	if strings.HasPrefix(contextLower, "kind") {
		return ClusterTypeKind
	}

	// Minikube clusters typically have context name "minikube"
	if contextLower == "minikube" || strings.HasPrefix(contextLower, "minikube-") {
		return ClusterTypeMinikube
	}

	// K3s clusters often have "k3s" in the context name
	if strings.Contains(contextLower, "k3s") {
		return ClusterTypeK3s
	}

	// CRC (CodeReady Containers) contexts start with "crc" or contain "-crc-"/"_crc_" as a segment
	if strings.HasPrefix(contextLower, "crc") || strings.Contains(contextLower, "-crc-") || strings.Contains(contextLower, "-crc:") {
		return ClusterTypeCRC
	}

	return ClusterTypeUnknown
}

// defaultApplicator implements the Applicator interface
type defaultApplicator struct{}

// Apply returns adjusted deployment parameters based on cluster type
func (a *defaultApplicator) Apply(
	clusterType ClusterType,
	resources, exposure string,
	portForwardEnabled bool,
) (string, string, bool, bool) {
	defaults, ok := getDefaultsForClusterType(clusterType)
	if !ok {
		// No special defaults for this cluster type
		return resources, exposure, portForwardEnabled, false
	}

	// Check if any parameter would change
	changed := resources != defaults.Resources ||
		exposure != defaults.Exposure ||
		portForwardEnabled != defaults.PortForwardEnabled

	if !changed {
		// User already specified the recommended defaults
		return resources, exposure, portForwardEnabled, false
	}

	// Apply the defaults
	return defaults.Resources, defaults.Exposure, defaults.PortForwardEnabled, true
}

// getDefaultsForClusterType returns the recommended defaults for a given cluster type
func getDefaultsForClusterType(clusterType ClusterType) (DeploymentDefaults, bool) {
	switch clusterType {
	case ClusterTypeKind:
		// Kind clusters are local, lightweight, and don't support LoadBalancer
		return DeploymentDefaults{
			Resources:          "small",
			Exposure:           "none",
			PortForwardEnabled: true,
		}, true

	case ClusterTypeMinikube:
		// Minikube is also local and benefits from small resources
		return DeploymentDefaults{
			Resources:          "small",
			Exposure:           "none",
			PortForwardEnabled: true,
		}, true

	case ClusterTypeK3s:
		// K3s can vary (local or cloud), apply conservative defaults
		return DeploymentDefaults{
			Resources:          "small",
			Exposure:           "none",
			PortForwardEnabled: true,
		}, true

	case ClusterTypeCRC:
		return DeploymentDefaults{
			Resources:          "small",
			Exposure:           "none",
			PortForwardEnabled: true,
		}, true

	default:
		return DeploymentDefaults{}, false
	}
}
