package deployer

import (
	"strings"
	"testing"

	"github.com/stackrox/roxie/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func profileValue(t *testing.T, obj map[string]any, path string) any {
	t.Helper()
	var value any = obj
	for _, key := range strings.Split(path, ".") {
		m, ok := value.(map[string]any)
		require.True(t, ok, "expected map at %s", key)
		value = m[key]
	}
	return value
}

func TestTinyResources(t *testing.T) {
	central, err := (&CentralConfig{ResourceProfile: types.ResourceProfileTiny}).CustomResource()
	require.NoError(t, err)
	secured, err := (&SecuredClusterConfig{ResourceProfile: types.ResourceProfileTiny}).CustomResource()
	require.NoError(t, err)
	for name, tc := range map[string]struct {
		cr                                       map[string]any
		path, cpu, memory, limitCPU, limitMemory string
	}{
		"central":    {central, "central", "90m", "300Mi", "1", "2Gi"},
		"central db": {central, "central.db", "80m", "400Mi", "1", "2Gi"},
		"worker":     {central, "centralWorker", "10m", "128Mi", "", "128Mi"},
		"scanner db": {central, "scannerV4.db", "80m", "400Mi", "1000m", "2000Mi"},
		"indexer":    {central, "scannerV4.indexer", "80m", "300Mi", "2000m", "2Gi"},
		"matcher":    {central, "scannerV4.matcher", "80m", "300Mi", "1000m", "2Gi"},
		"sensor":     {secured, "sensor", "80m", "300Mi", "1000m", "2Gi"},
		"admission":  {secured, "admissionControl", "10m", "", "", ""},
		"collector":  {secured, "perNode.collector", "10m", "", "", ""},
		"compliance": {secured, "perNode.compliance", "10m", "", "", ""},
		"inventory":  {secured, "perNode.nodeInventory", "10m", "", "", ""},
	} {
		t.Run(name, func(t *testing.T) {
			resources := profileValue(t, tc.cr, "spec."+tc.path+".resources").(map[string]any)
			want := map[string]any{"requests": map[string]any{"cpu": tc.cpu}}
			if tc.memory != "" {
				want["requests"].(map[string]any)["memory"] = tc.memory
			}
			if tc.limitMemory != "" {
				want["limits"] = map[string]any{"memory": tc.limitMemory}
			}
			if tc.limitCPU != "" {
				want["limits"].(map[string]any)["cpu"] = tc.limitCPU
			}
			assert.Equal(t, want, resources)
		})
	}
	assert.Equal(t, 1, profileValue(t, secured, "spec.admissionControl.replicas"))
	for name, path := range map[string]string{"indexer": "spec.scannerV4.indexer.scaling", "matcher": "spec.scannerV4.matcher.scaling"} {
		t.Run(name+" replicas", func(t *testing.T) {
			assert.Equal(t, map[string]any{"autoScaling": "Disabled", "replicas": 1}, profileValue(t, central, path))
		})
	}
	assert.Equal(t, map[string]any{"resources": map[string]any{"requests": map[string]any{"cpu": "10m", "memory": "128Mi"}, "limits": map[string]any{"memory": "128Mi"}}}, profileValue(t, central, "spec.centralWorker"))
}

func TestProfilePostgresArguments(t *testing.T) {
	for name, profile := range map[string]types.ResourceProfile{
		"tiny": types.ResourceProfileTiny, "small": types.ResourceProfileSmall, "medium": types.ResourceProfileMedium, "ci": types.ResourceProfileCI, "defaults": types.ResourceProfileAcsDefaults,
	} {
		t.Run(name, func(t *testing.T) {
			cr, err := (&CentralConfig{ResourceProfile: profile}).CustomResource()
			require.NoError(t, err)
			overlays := profileValue(t, cr, "spec.overlays")
			if profile != types.ResourceProfileTiny {
				assert.Nil(t, overlays)
				return
			}
			list := overlays.([]any)
			require.Len(t, list, 1)
			overlay := list[0].(map[string]any)
			assert.Equal(t, "central-db", overlay["name"])
			assert.Equal(t, "apps/v1", overlay["apiVersion"])
			assert.Equal(t, "Deployment", overlay["kind"])
			assert.Equal(t, true, overlay["optional"])
			patches := overlay["patches"].([]any)
			require.Len(t, patches, 1) // No command patch: preserve the image entrypoint.
			patch := patches[0].(map[string]any)
			assert.Equal(t, "spec.template.spec.containers[name:central-db].args", patch["path"])
			var args []string
			require.NoError(t, yaml.Unmarshal([]byte(patch["value"].(string)), &args))
			assert.Equal(t, []string{"postgres", "-c", "config_file=/etc/stackrox.d/config/postgresql.conf", "-c", "shared_buffers=128MB", "-c", "work_mem=4MB", "-c", "maintenance_work_mem=64MB", "-c", "autovacuum_work_mem=32MB"}, args)
		})
	}
}

func TestTinyUserOverrides(t *testing.T) {
	for name, external := range map[string]bool{"managed db": false, "external db": true} {
		t.Run(name, func(t *testing.T) {
			userOverlay := map[string]any{"apiVersion": "apps/v1", "kind": "Deployment", "name": "central-db", "optional": true, "patches": []any{map[string]any{"path": "spec.template.spec.containers[name:central-db].args", "value": "[postgres, -c, shared_buffers=64MB]"}}}
			spec := map[string]any{
				"central":  map[string]any{"db": map[string]any{"resources": map[string]any{"requests": map[string]any{"memory": "1Gi"}, "limits": map[string]any{"memory": "1Gi"}}}},
				"overlays": []any{userOverlay},
			}
			if external {
				spec["central"].(map[string]any)["db"].(map[string]any)["connectionString"] = "host=external-db port=5432"
			}
			cr, err := (&CentralConfig{ResourceProfile: types.ResourceProfileTiny, Spec: spec}).CustomResource()
			require.NoError(t, err)
			assert.Equal(t, "1Gi", profileValue(t, cr, "spec.central.db.resources.requests.memory"))
			assert.Equal(t, "1Gi", profileValue(t, cr, "spec.central.db.resources.limits.memory"))
			assert.Equal(t, "80m", profileValue(t, cr, "spec.central.db.resources.requests.cpu"))
			overlays := profileValue(t, cr, "spec.overlays").([]any)
			require.Len(t, overlays, 2)
			assert.Equal(t, true, overlays[0].(map[string]any)["optional"])
			assert.Equal(t, userOverlay, overlays[1]) // User patches run after profile patches.
			if external {
				assert.Equal(t, "host=external-db port=5432", profileValue(t, cr, "spec.central.db.connectionString"))
			}
			secured, err := (&SecuredClusterConfig{ResourceProfile: types.ResourceProfileTiny, Spec: map[string]any{
				"admissionControl": map[string]any{"replicas": 2, "resources": map[string]any{"requests": map[string]any{"cpu": "20m"}}},
				"overlays":         []any{userOverlay},
			}}).CustomResource()
			require.NoError(t, err)
			assert.Equal(t, 2, profileValue(t, secured, "spec.admissionControl.replicas"))
			assert.Equal(t, "20m", profileValue(t, secured, "spec.admissionControl.resources.requests.cpu"))
			securedOverlays := profileValue(t, secured, "spec.overlays").([]any)
			require.Len(t, securedOverlays, 2)
			crs := securedOverlays[0].(map[string]any)
			assert.Equal(t, "sensor", crs["name"])
			assert.Equal(t, []any{map[string]any{"path": "spec.template.spec.initContainers[name:crs].resources.requests", "value": `{"cpu":"80m", "memory": "150Mi"}`}}, crs["patches"])
			assert.Equal(t, userOverlay, securedOverlays[1])
			fresh, err := (&CentralConfig{ResourceProfile: types.ResourceProfileTiny}).CustomResource()
			require.NoError(t, err)
			assert.Equal(t, "400Mi", profileValue(t, fresh, "spec.central.db.resources.requests.memory"))
			assert.Len(t, profileValue(t, fresh, "spec.overlays"), 1)
		})
	}
}
