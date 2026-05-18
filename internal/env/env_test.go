package env

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stackrox/roxie/internal/types"
)

func TestDetectClusterType_GKE(t *testing.T) {
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

func TestDetectClusterType_GKE_ExactMatch(t *testing.T) {
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
	config := KubeConfig{
		CurrentContext: "gke_other-project_us-west1_cluster",
		Clusters: []KubeCluster{
			{
				Name:   "gke_cluster",
				Server: "https://34.1.2.3",
			},
		},
	}
	apiResources := []string{"pods"}

	result := DetectClusterType(config, apiResources)
	if result != types.ClusterTypeUnknown {
		t.Errorf("DetectClusterType() = %v (%s), want %v", result, result.String(), types.ClusterTypeUnknown)
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
			name:        "types.ClusterTypeInfraGKE",
			clusterType: types.ClusterTypeInfraGKE,
			want:        "GKE",
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
			name:        "crc cluster with admin context",
			kubeContext: "crc-admin",
			want:        types.ClusterTypeCRC,
		},
		{
			name:        "crc cluster with api prefix",
			kubeContext: "api-crc-testing:6443",
			want:        types.ClusterTypeCRC,
		},
		{
			name:        "crc cluster with uppercase",
			kubeContext: "CRC-admin",
			want:        types.ClusterTypeCRC,
		},
		{
			name:        "crc cluster bare name",
			kubeContext: "crc",
			want:        types.ClusterTypeCRC,
		},
		{
			name:        "not crc - incidental substring",
			kubeContext: "acrc-cluster",
			want:        types.ClusterTypeUnknown,
		},
		{
			name:        "not crc - encrypted in name",
			kubeContext: "my-encrypted-cluster",
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
