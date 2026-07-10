package deployer

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// StackRoxRepoHelmChartAddOn locates a Helm chart within the stackrox repository.
// All paths are relative to the repository root.
type StackRoxRepoHelmChartAddOn struct {
	Path            string `yaml:"path,omitempty"`
	HelmChartValues `yaml:",inline"`
}

// HelmChartRepoAddOn references a chart in a public Helm repository.
type HelmChartRepoAddOn struct {
	Repo            string `yaml:"repo,omitempty"`
	Chart           string `yaml:"chart,omitempty"`
	Version         string `yaml:"version,omitempty"`
	HelmChartValues `yaml:",inline"`
}

type HelmChartValues struct {
	Values     string `yaml:"values,omitempty"`
	ValuesFile string `yaml:"valuesFile,omitempty"`
	EnvSubst   bool   `yaml:"envSubst,omitempty"`
}

// CentralAddOnDefinition describes an add-on that can be deployed alongside Central.
type CentralAddOnDefinition struct {
	CommonAddOnProperties `yaml:",inline"`

	StackRoxRepoHelmChart *StackRoxRepoHelmChartAddOn `yaml:"stackroxRepoHelmChart,omitempty"`
	HelmChart             *HelmChartRepoAddOn         `yaml:"helmChart,omitempty"`
}

type CommonAddOnProperties struct {
	Optional bool `yaml:"optional,omitempty"`
	Priority uint `yaml:"priority,omitempty"`
}

func (h *HelmChartValues) GetValues() (map[string]any, error) {
	if h == nil {
		return nil, errors.New("cannot retrieve Helm chart values from nil receiver")
	}
	if h.Values != "" && h.ValuesFile != "" {
		return nil, errors.New("a Helm chart definition must not specify values and valuesFile at the same time")
	}

	values := h.Values
	if h.ValuesFile != "" {
		content, err := os.ReadFile(h.ValuesFile)
		if err != nil {
			return nil, fmt.Errorf("reading file %q: %w", h.ValuesFile, err)
		}
		values = string(content)
	}

	if h.EnvSubst {
		values = os.ExpandEnv(values)
	}
	valuesBytes := []byte(values)

	var valuesMap map[string]any
	if err := yaml.Unmarshal(valuesBytes, &valuesMap); err != nil {
		return nil, fmt.Errorf("unmarshaling values: %w", err)
	}
	return valuesMap, nil
}
