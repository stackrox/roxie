package deployer

import (
	"strings"
	"testing"

	"github.com/stackrox/roxie/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveAddOns(t *testing.T) {
	longName := strings.Repeat("a", 53-len(addOnReleasePrefix)+1)

	tests := []struct {
		name   string
		cfg    CentralConfig
		assert func(t *testing.T, resolved []AddOn, err error)
	}{
		{
			name: "no add-ons configured returns empty",
			cfg:  NewCentralConfig(),
			assert: func(t *testing.T, resolved []AddOn, err error) {
				require.NoError(t, err)
				assert.Empty(t, resolved)
			},
		},
		{
			name: "nil AddOns map returns empty",
			cfg: CentralConfig{
				AvailableAddOns: map[string]CentralAddOnDefinition{
					"my-addon": {
						HelmChart: &HelmChartRepoAddOn{
							Repo:  "https://example.com/charts",
							Chart: "example",
						},
					},
				},
			},
			assert: func(t *testing.T, resolved []AddOn, err error) {
				require.NoError(t, err)
				assert.Empty(t, resolved)
			},
		},
		{
			name: "disabled add-on is not resolved",
			cfg: CentralConfig{
				AddOns: map[string]bool{"my-addon": false},
				AvailableAddOns: map[string]CentralAddOnDefinition{
					"my-addon": {
						HelmChart: &HelmChartRepoAddOn{
							Repo:  "https://example.com/charts",
							Chart: "example",
						},
					},
				},
			},
			assert: func(t *testing.T, resolved []AddOn, err error) {
				require.NoError(t, err)
				assert.Empty(t, resolved)
			},
		},
		{
			name: "enabled repo-based add-on is resolved",
			cfg: CentralConfig{
				Namespace: "acs-central",
				AddOns:    map[string]bool{"my-addon": true},
				AvailableAddOns: map[string]CentralAddOnDefinition{
					"my-addon": {
						HelmChart: &HelmChartRepoAddOn{
							Repo:    "https://example.com/charts",
							Chart:   "example",
							Version: "1.0.0",
						},
					},
				},
			},
			assert: func(t *testing.T, resolved []AddOn, err error) {
				require.NoError(t, err)
				require.Len(t, resolved, 1)
				assert.Equal(t, "my-addon", resolved[0].Name())

				ha, ok := resolved[0].(*helmAddOn)
				require.True(t, ok, "expected *helmAddOn")
				assert.Equal(t, "roxie-addon-my-addon", ha.releaseName)
				assert.Equal(t, "https://example.com/charts", ha.installOpts.RepoURL)
				assert.Equal(t, "example", ha.installOpts.ChartName)
				assert.Equal(t, "1.0.0", ha.installOpts.ChartVersion)
				assert.Equal(t, "acs-central", ha.installOpts.Namespace)
			},
		},
		{
			name: "missing chart source returns error",
			cfg: CentralConfig{
				AddOns: map[string]bool{"broken": true},
				AvailableAddOns: map[string]CentralAddOnDefinition{
					"broken": {},
				},
			},
			assert: func(t *testing.T, _ []AddOn, err error) {
				require.ErrorContains(t, err, "no recognized add-on type configured")
			},
		},
		{
			name: "switch key mismatch returns error",
			cfg: CentralConfig{
				AddOns: map[string]bool{"other-key": true},
				AvailableAddOns: map[string]CentralAddOnDefinition{
					"my-addon": {
						HelmChart: &HelmChartRepoAddOn{
							Repo:  "https://example.com/charts",
							Chart: "example",
						},
					},
				},
			},
			assert: func(t *testing.T, _ []AddOn, err error) {
				require.ErrorContains(t, err, "unknown add-on")
			},
		},
		{
			name: "invalid add-on name returns error",
			cfg: CentralConfig{
				AddOns: map[string]bool{"Bad_Name": true},
				AvailableAddOns: map[string]CentralAddOnDefinition{
					"Bad_Name": {
						HelmChart: &HelmChartRepoAddOn{
							Repo:  "https://example.com/charts",
							Chart: "example",
						},
					},
				},
			},
			assert: func(t *testing.T, _ []AddOn, err error) {
				require.ErrorContains(t, err, "is invalid")
			},
		},
		{
			name: "add-on name starting with hyphen is rejected",
			cfg: CentralConfig{
				AddOns: map[string]bool{"-bad": true},
				AvailableAddOns: map[string]CentralAddOnDefinition{
					"-bad": {
						HelmChart: &HelmChartRepoAddOn{
							Repo:  "https://example.com/charts",
							Chart: "example",
						},
					},
				},
			},
			assert: func(t *testing.T, _ []AddOn, err error) {
				require.ErrorContains(t, err, "is invalid")
			},
		},
		{
			name: "release name exceeding 53 chars is rejected",
			cfg: CentralConfig{
				AddOns: map[string]bool{longName: true},
				AvailableAddOns: map[string]CentralAddOnDefinition{
					longName: {
						HelmChart: &HelmChartRepoAddOn{
							Repo:  "https://example.com/charts",
							Chart: "example",
						},
					},
				},
			},
			assert: func(t *testing.T, _ []AddOn, err error) {
				require.ErrorContains(t, err, "exceeds 53 characters")
			},
		},
		{
			name: "resolved add-ons are sorted by priority descending then by name",
			cfg: CentralConfig{
				Namespace: "test-ns",
				AddOns: map[string]bool{
					"zebra":     true,
					"alpha":     true,
					"mid-prio":  true,
					"hi-prio":   true,
					"hi-prio-2": true,
					"beta":      true,
				},
				AvailableAddOns: map[string]CentralAddOnDefinition{
					"zebra": {
						HelmChart: &HelmChartRepoAddOn{Repo: "https://example.com", Chart: "z"},
					},
					"alpha": {
						HelmChart: &HelmChartRepoAddOn{Repo: "https://example.com", Chart: "a"},
					},
					"mid-prio": {
						CommonAddOnProperties: CommonAddOnProperties{Priority: 5},
						HelmChart:             &HelmChartRepoAddOn{Repo: "https://example.com", Chart: "m"},
					},
					"hi-prio": {
						CommonAddOnProperties: CommonAddOnProperties{Priority: 10},
						HelmChart:             &HelmChartRepoAddOn{Repo: "https://example.com", Chart: "h"},
					},
					"hi-prio-2": {
						CommonAddOnProperties: CommonAddOnProperties{Priority: 10},
						HelmChart:             &HelmChartRepoAddOn{Repo: "https://example.com", Chart: "h"},
					},
					"beta": {
						HelmChart: &HelmChartRepoAddOn{Repo: "https://example.com", Chart: "b"},
					},
				},
			},
			assert: func(t *testing.T, resolved []AddOn, err error) {
				require.NoError(t, err)
				require.Len(t, resolved, 6)

				names := make([]string, len(resolved))
				for i, a := range resolved {
					names[i] = a.Name()
				}
				assert.Equal(t, []string{"hi-prio", "hi-prio-2", "mid-prio", "alpha", "beta", "zebra"}, names)
			},
		},
	}

	addOnCfg := AddOnConfig{
		log: logger.New(),
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, err := resolveEnabledAddOns(tt.cfg, addOnCfg)
			tt.assert(t, resolved, err)
		})
	}
}
