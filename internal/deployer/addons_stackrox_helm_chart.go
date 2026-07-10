package deployer

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/stackrox/roxie/internal/env"
	"github.com/stackrox/roxie/internal/helm"
	"github.com/stackrox/roxie/internal/helpers"
)

func (h *StackRoxRepoHelmChartAddOn) New(
	addOnCfg AddOnConfig,
	commonProperties CommonAddOnProperties,
	name, namespace string,
) (AddOn, error) {
	if !env.IsInStackroxRepository(addOnCfg.log) {
		addOnCfg.log.Errorf("the Helm chart add-on %q uses stackroxRepoHelmChart but roxie is not running from a stackrox checkout", name)
		return nil, errors.New("not invoked in StackRox repository")
	}
	topLevelDir, err := env.GetStackRoxTopLevelDir()
	if err != nil {
		return nil, fmt.Errorf("resolving stackrox repo path for Helm chart add-on %q: %w", name, err)
	}

	chartPath := filepath.Join(topLevelDir, h.Path)
	if err := helm.BuildDependencies(addOnCfg.log, chartPath); err != nil {
		return nil, fmt.Errorf("building dependencies for Helm chart add-on %q: %w", name, err)
	}

	opts := helm.InstallOptions{
		ChartPath: chartPath,
	}
	addOnCfg.log.Infof("Add-on %q: using Helm chart add-on from stackrox repo at %s", name, opts.ChartPath)

	// Shallow copy, because we modify the ValuesFile and don't want to mutate the actual receiver:
	hCopy := *h

	if hCopy.ValuesFile != "" && !filepath.IsAbs(hCopy.ValuesFile) {
		hCopy.ValuesFile = filepath.Join(topLevelDir, hCopy.ValuesFile)
	}
	values, err := hCopy.GetValues()
	if err != nil {
		return nil, fmt.Errorf("retrieving values for Helm chart add-on %q: %w", name, err)
	}
	opts.Values = values

	if addOnCfg.verbose {
		addOnCfg.log.Dimf("values for Helm chart add-on %q evaluated:", name)
		helpers.LogMultilineYaml(addOnCfg.log, opts.Values)
	}

	return newHelmAddOn(addOnCfg, commonProperties, name, namespace, opts)
}
