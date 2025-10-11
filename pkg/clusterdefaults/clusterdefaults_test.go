package clusterdefaults

import (
	"testing"

	"github.com/stackrox/roxie-golang/pkg/logger"
)

func TestDefaultDetector_Detect(t *testing.T) {
	tests := []struct {
		name        string
		kubeContext string
		want        ClusterType
	}{
		{
			name:        "kind cluster with standard prefix",
			kubeContext: "kind-dev-cluster",
			want:        ClusterTypeKind,
		},
		{
			name:        "kind cluster simple name",
			kubeContext: "kind",
			want:        ClusterTypeKind,
		},
		{
			name:        "kind cluster with uppercase",
			kubeContext: "KIND-test",
			want:        ClusterTypeKind,
		},
	}

	detector := &defaultDetector{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detector.Detect(tt.kubeContext)
			if got != tt.want {
				t.Errorf("Detect(%q) = %v, want %v", tt.kubeContext, got, tt.want)
			}
		})
	}
}

func TestDefaultApplicator_Apply(t *testing.T) {
	tests := []struct {
		name               string
		clusterType        ClusterType
		resources          string
		exposure           string
		portForwardEnabled bool
		wantResources      string
		wantExposure       string
		wantPortForward    bool
		wantChanged        bool
	}{
		{
			name:               "kind cluster with default params",
			clusterType:        ClusterTypeKind,
			resources:          "default",
			exposure:           "loadbalancer",
			portForwardEnabled: false,
			wantResources:      "small",
			wantExposure:       "none",
			wantPortForward:    true,
			wantChanged:        true,
		},
		{
			name:               "kind cluster with already correct params",
			clusterType:        ClusterTypeKind,
			resources:          "small",
			exposure:           "none",
			portForwardEnabled: true,
			wantResources:      "small",
			wantExposure:       "none",
			wantPortForward:    true,
			wantChanged:        false,
		},
		{
			name:               "kind cluster with partial match",
			clusterType:        ClusterTypeKind,
			resources:          "small",
			exposure:           "loadbalancer",
			portForwardEnabled: false,
			wantResources:      "small",
			wantExposure:       "none",
			wantPortForward:    true,
			wantChanged:        true,
		},
		{
			name:               "unknown cluster type",
			clusterType:        ClusterTypeUnknown,
			resources:          "default",
			exposure:           "loadbalancer",
			portForwardEnabled: false,
			wantResources:      "default",
			wantExposure:       "loadbalancer",
			wantPortForward:    false,
			wantChanged:        false,
		},
		{
			name:               "minikube cluster",
			clusterType:        ClusterTypeMinikube,
			resources:          "default",
			exposure:           "loadbalancer",
			portForwardEnabled: false,
			wantResources:      "small",
			wantExposure:       "none",
			wantPortForward:    true,
			wantChanged:        true,
		},
	}

	applicator := &defaultApplicator{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes, gotExp, gotPF, gotChanged := applicator.Apply(
				tt.clusterType,
				tt.resources,
				tt.exposure,
				tt.portForwardEnabled,
			)

			if gotRes != tt.wantResources {
				t.Errorf("Apply() resources = %v, want %v", gotRes, tt.wantResources)
			}
			if gotExp != tt.wantExposure {
				t.Errorf("Apply() exposure = %v, want %v", gotExp, tt.wantExposure)
			}
			if gotPF != tt.wantPortForward {
				t.Errorf("Apply() portForward = %v, want %v", gotPF, tt.wantPortForward)
			}
			if gotChanged != tt.wantChanged {
				t.Errorf("Apply() changed = %v, want %v", gotChanged, tt.wantChanged)
			}
		})
	}
}

func TestManager_ApplyConvenienceDefaults(t *testing.T) {
	tests := []struct {
		name               string
		kubeContext        string
		resources          string
		exposure           string
		portForwardEnabled bool
		wantResources      string
		wantExposure       string
		wantPortForward    bool
	}{
		{
			name:               "kind cluster detection and defaults",
			kubeContext:        "kind-local",
			resources:          "default",
			exposure:           "loadbalancer",
			portForwardEnabled: false,
			wantResources:      "small",
			wantExposure:       "none",
			wantPortForward:    true,
		},
		{
			name:               "gke cluster no changes",
			kubeContext:        "gke_project_zone_cluster",
			resources:          "default",
			exposure:           "loadbalancer",
			portForwardEnabled: false,
			wantResources:      "default",
			wantExposure:       "loadbalancer",
			wantPortForward:    false,
		},
	}

	log := logger.New()
	manager := NewManager(log)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes, gotExp, gotPF := manager.ApplyConvenienceDefaults(
				tt.kubeContext,
				tt.resources,
				tt.exposure,
				tt.portForwardEnabled,
			)

			if gotRes != tt.wantResources {
				t.Errorf("ApplyConvenienceDefaults() resources = %v, want %v", gotRes, tt.wantResources)
			}
			if gotExp != tt.wantExposure {
				t.Errorf("ApplyConvenienceDefaults() exposure = %v, want %v", gotExp, tt.wantExposure)
			}
			if gotPF != tt.wantPortForward {
				t.Errorf("ApplyConvenienceDefaults() portForward = %v, want %v", gotPF, tt.wantPortForward)
			}
		})
	}
}

func TestClusterType_String(t *testing.T) {
	tests := []struct {
		clusterType ClusterType
		want        string
	}{
		{ClusterTypeKind, "kind"},
		{ClusterTypeMinikube, "minikube"},
		{ClusterTypeK3s, "k3s"},
		{ClusterTypeUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.clusterType.String(); got != tt.want {
				t.Errorf("ClusterType.String() = %v, want %v", got, tt.want)
			}
		})
	}
}
