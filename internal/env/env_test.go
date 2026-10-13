package env

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stackrox/roxie/internal/types"
)

func TestDetectClusterType_InfraGKE(t *testing.T) {
	config := KubeConfig{
		CurrentContext: "gke_acs-team-temp-dev_us-central1-a_my-cluster",
		Clusters: []KubeCluster{
			{
				Name:   "gke_cluster",
				Server: "https://34.1.2.3",
			},
		},
	}
	apiResources := []string{"pods", "services", "deployments"}

	result := DetectClusterType(config, apiResources)
	if result != types.ClusterTypeInfraGKE {
		t.Errorf("DetectClusterType() = %v (%s), want %v", result, result.String(), types.ClusterTypeInfraGKE)
	}
}

func TestDetectClusterType_InfraGKE_ExactMatch(t *testing.T) {
	config := KubeConfig{
		CurrentContext: "gke_acs-team-temp-dev",
		Clusters: []KubeCluster{
			{
				Name:   "gke_cluster",
				Server: "https://34.1.2.3",
			},
		},
	}
	apiResources := []string{"pods", "services"}

	result := DetectClusterType(config, apiResources)
	if result != types.ClusterTypeInfraGKE {
		t.Errorf("DetectClusterType() = %v (%s), want %v", result, result.String(), types.ClusterTypeInfraGKE)
	}
}

func TestDetectClusterType_AKS(t *testing.T) {
	config := KubeConfig{
		CurrentContext: "my-aks-cluster",
		Clusters: []KubeCluster{
			{
				Name:   "my-aks-cluster",
				Server: "https://ABCDEF1234567890.some-region.azmk8s.io:443",
			},
		},
	}
	apiResources := []string{"pods", "services"}

	result := DetectClusterType(config, apiResources)
	assert.Equal(t, types.ClusterTypeAKS, result)
}

func TestDetectClusterType_EKS(t *testing.T) {
	config := KubeConfig{
		CurrentContext: "my-eks-cluster",
		Clusters: []KubeCluster{
			{
				Name:   "my-eks-cluster",
				Server: "https://ABCDEF1234567890.some-region.eks.amazonaws.com:443",
			},
		},
	}
	apiResources := []string{"pods", "services"}

	result := DetectClusterType(config, apiResources)
	assert.Equal(t, types.ClusterTypeEKS, result)
}

func TestDetectClusterType_InfraOpenShift4(t *testing.T) {
	config := KubeConfig{
		CurrentContext: "admin",
		Clusters: []KubeCluster{
			{
				Name:   "openshift-cluster",
				Server: "https://api.my-cluster.ocp.infra.rox.systems:6443",
			},
		},
	}
	apiResources := []string{
		"pods",
		"services",
		"clusterversions.config.openshift.io",
		"clusteroperators.config.openshift.io",
	}

	result := DetectClusterType(config, apiResources)
	assert.Equal(t, types.ClusterTypeInfraOpenShift4, result)
}

func TestDetectClusterType_OpenShift4(t *testing.T) {
	config := KubeConfig{
		CurrentContext: "some-context-name",
		Clusters: []KubeCluster{
			{
				Name:   "some-other-name",
				Server: "https://my-cluster.example.com:6443",
			},
		},
	}
	apiResources := []string{
		"pods",
		"services",
		"clusterversions.config.openshift.io",
		"clusteroperators.config.openshift.io",
	}

	result := DetectClusterType(config, apiResources)
	assert.Equal(t, types.ClusterTypeOpenShift4, result)
}

func TestDetectClusterType_CRC_OpenShiftPreset(t *testing.T) {
	// The CRC OpenShift preset is full OpenShift 4 running locally: it serves the
	// config.openshift.io API group AND is reachable at api.crc.testing. It must be
	// detected as CRC (not OpenShift4), keyed off the server host. A non-CRC server
	// host with the same API resources is plain OpenShift4.
	openShift4Resources := []string{
		"pods",
		"services",
		"clusterversions.config.openshift.io",
		"clusteroperators.config.openshift.io",
	}

	tests := []struct {
		name        string
		kubeContext string
		server      string
		want        types.ClusterType
	}{
		{name: "crc server host", kubeContext: "crc-admin", server: "https://api.crc.testing:6443", want: types.ClusterTypeCRC},
		{name: "crc server host, arbitrary context", kubeContext: "whatever", server: "https://api.crc.testing:6443", want: types.ClusterTypeCRC},
		// Hostnames are case-insensitive, so an uppercase CRC server host must
		// still be detected as CRC.
		{name: "crc server host, uppercase", kubeContext: "crc-admin", server: "https://API.CRC.TESTING:6443", want: types.ClusterTypeCRC},
		// A non-CRC server host must be detected as plain OpenShift4, even if the
		// context name happens to contain "crc".
		{name: "non-crc server, crc-like context", kubeContext: "acrc-cluster", server: "https://api.my-cluster.example.com:6443", want: types.ClusterTypeOpenShift4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := KubeConfig{
				CurrentContext: tt.kubeContext,
				Clusters: []KubeCluster{
					{
						Name:   tt.kubeContext,
						Server: tt.server,
					},
				},
			}

			result := DetectClusterType(config, openShift4Resources)
			assert.Equal(t, tt.want, result,
				"context %q / server %q with OpenShift4 API resources", tt.kubeContext, tt.server)
		})
	}
}

func TestDetectClusterType_OpenShift4_NoAPIResources(t *testing.T) {
	config := KubeConfig{
		CurrentContext: "admin",
		Clusters: []KubeCluster{
			{
				Name:   "openshift-cluster",
				Server: "https://api.my-cluster.ocp.infra.rox.systems:6443",
			},
		},
	}
	apiResources := []string{"pods", "services"}

	result := DetectClusterType(config, apiResources)
	if result != types.ClusterTypeUnknown {
		t.Errorf("DetectClusterType() = %v (%s), want %v", result, result.String(), types.ClusterTypeUnknown)
	}
}

func TestDetectClusterType_Kind(t *testing.T) {
	config := KubeConfig{
		CurrentContext: "kind-dev-cluster",
		Clusters: []KubeCluster{
			{
				Name:   "kind-dev-cluster",
				Server: "https://127.0.0.1:55193",
			},
		},
	}
	apiResources := []string{"pods", "services"}

	result := DetectClusterType(config, apiResources)
	if result != types.ClusterTypeKind {
		t.Errorf("DetectClusterType() = %v (%s), want %v", result, result.String(), types.ClusterTypeKind)
	}
}

func TestDetectClusterType_Kind_CaseInsensitive(t *testing.T) {
	config := KubeConfig{
		CurrentContext: "KIND-test",
		Clusters: []KubeCluster{
			{
				Name:   "KIND-test",
				Server: "https://127.0.0.1:12345",
			},
		},
	}
	apiResources := []string{"pods"}

	result := DetectClusterType(config, apiResources)
	if result != types.ClusterTypeKind {
		t.Errorf("DetectClusterType() = %v (%s), want %v", result, result.String(), types.ClusterTypeKind)
	}
}

func TestDetectClusterType_EmptyContext(t *testing.T) {
	config := KubeConfig{
		CurrentContext: "",
		Clusters:       []KubeCluster{},
	}
	apiResources := []string{}

	result := DetectClusterType(config, apiResources)
	if result != types.ClusterTypeUnknown {
		t.Errorf("DetectClusterType() = %v (%s), want %v", result, result.String(), types.ClusterTypeUnknown)
	}
}

func TestDetectClusterType_Minikube(t *testing.T) {
	config := KubeConfig{
		CurrentContext: "minikube",
		Clusters: []KubeCluster{
			{
				Name:   "minikube",
				Server: "https://192.168.49.2:8443",
			},
		},
	}
	apiResources := []string{"pods", "services"}

	result := DetectClusterType(config, apiResources)
	if result != types.ClusterTypeMinikube {
		t.Errorf("DetectClusterType() = %v (%s), want %v", result, result.String(), types.ClusterTypeMinikube)
	}
}

func TestDetectClusterType_GKE_DifferentProject(t *testing.T) {
	tests := []struct {
		name    string
		context string
	}{
		{
			name:    "arbitrary project",
			context: "gke_other-project_us-west1_cluster",
		},
		{
			name:    "CI direct-gke via gke.sh (acs-san-stackroxci)",
			context: "gke_acs-san-stackroxci_us-east4-b_rox-ci-qa-e2e-12345678",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := KubeConfig{
				CurrentContext: tt.context,
				Clusters: []KubeCluster{
					{
						Name:   "gke_cluster",
						Server: "https://34.1.2.3",
					},
				},
			}
			result := DetectClusterType(config, []string{"pods"})
			assert.Equal(t, types.ClusterTypeGKE, result)
		})
	}
}

func TestIsOpenShift4(t *testing.T) {
	tests := []struct {
		name         string
		apiResources []string
		want         bool
	}{
		{
			name: "OpenShift 4 with clusterversions",
			apiResources: []string{
				"pods",
				"clusterversions.config.openshift.io",
				"services",
			},
			want: true,
		},
		{
			name: "OpenShift 4 with other config resources",
			apiResources: []string{
				"pods",
				"clusteroperators.config.openshift.io",
			},
			want: true,
		},
		{
			name: "No OpenShift resources",
			apiResources: []string{
				"pods",
				"services",
				"deployments",
			},
			want: false,
		},
		{
			name:         "Empty list",
			apiResources: []string{},
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isOpenShift4(tt.apiResources)
			if got != tt.want {
				t.Errorf("isOpenShift4() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetServerURL(t *testing.T) {
	tests := []struct {
		name   string
		config KubeConfig
		want   string
	}{
		{
			name: "Single cluster",
			config: KubeConfig{
				Clusters: []KubeCluster{
					{Server: "https://example.com:6443"},
				},
			},
			want: "https://example.com:6443",
		},
		{
			name: "Multiple clusters - returns first",
			config: KubeConfig{
				Clusters: []KubeCluster{
					{Server: "https://first.com:6443"},
					{Server: "https://second.com:6443"},
				},
			},
			want: "https://first.com:6443",
		},
		{
			name: "No clusters",
			config: KubeConfig{
				Clusters: []KubeCluster{},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getServerURL(tt.config)
			if got != tt.want {
				t.Errorf("getServerURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClusterTypeString(t *testing.T) {
	tests := []struct {
		name        string
		clusterType types.ClusterType
		want        string
	}{
		{
			name:        "types.ClusterTypeGKE",
			clusterType: types.ClusterTypeGKE,
			want:        "GKE",
		},
		{
			name:        "types.ClusterTypeInfraGKE",
			clusterType: types.ClusterTypeInfraGKE,
			want:        "GKE (infra)",
		},
		{
			name:        "InfraOpenShift4",
			clusterType: types.ClusterTypeInfraOpenShift4,
			want:        "OpenShift4 (infra)",
		},
		{
			name:        "OpenShift4",
			clusterType: types.ClusterTypeOpenShift4,
			want:        "OpenShift4",
		},
		{
			name:        "LocalKind",
			clusterType: types.ClusterTypeKind,
			want:        "Kind",
		},
		{
			name:        "AKS",
			clusterType: types.ClusterTypeAKS,
			want:        "AKS",
		},
		{
			name:        "EKS",
			clusterType: types.ClusterTypeEKS,
			want:        "EKS",
		},
		{
			name:        "ClusterTypeUnknown",
			clusterType: types.ClusterTypeUnknown,
			want:        "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.clusterType.String()
			if got != tt.want {
				t.Errorf("ClusterType.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsStackRoxRepositoryRemote(t *testing.T) {
	tests := []struct {
		remote string
		want   bool
	}{
		{"git@github.com:stackrox/stackrox.git", true},
		{"git@github.com:stackrox/stackrox", true},
		{"https://github.com/stackrox/stackrox.git", true},
		{"https://github.com/stackrox/stackrox", true},
		{"ssh://git@github.com/stackrox/stackrox.git", true},
		{"ssh://git@github.com/stackrox/stackrox", true},
		{"git@github.com:someone/stackrox.git", false},
		{"git@github.com:stackrox/other.git", false},
		{"git@github.com:foo-stackrox/stackrox-bar.git", false},
		{"git@github.com:foo-stackrox/stackrox.git", false},
		{"git@github.com:stackrox/stackrox-bar.git", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.remote, func(t *testing.T) {
			got := isStackRoxRepositoryRemote(tt.remote)
			assert.Equal(t, tt.want, got, "isStackRoxRepositoryRemote(%q)", tt.remote)
		})
	}
}

func TestDefaultDetector_Detect(t *testing.T) {
	tests := []struct {
		name        string
		kubeContext string
		want        types.ClusterType
	}{
		{
			name:        "kind cluster with standard prefix",
			kubeContext: "kind-dev-cluster",
			want:        types.ClusterTypeKind,
		},
		{
			name:        "kind cluster simple name",
			kubeContext: "kind",
			want:        types.ClusterTypeKind,
		},
		{
			name:        "kind cluster with uppercase",
			kubeContext: "KIND-test",
			want:        types.ClusterTypeKind,
		},
		{
			name:        "crc-like context without server host or api resources is unknown",
			kubeContext: "crc-admin",
			want:        types.ClusterTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kubeConfig := KubeConfig{
				CurrentContext: tt.kubeContext,
			}
			got := DetectClusterType(kubeConfig, nil)
			if got != tt.want {
				t.Errorf("Detect(%q) = %v, want %v", tt.kubeContext, got, tt.want)
			}
		})
	}
}
