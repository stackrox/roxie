package deployer

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"

	"github.com/stackrox/roxie/internal/logger"
)

var (
	validAddOnName = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)
)

// AddOn is the resolved runtime representation of an add-on that can be
// deployed alongside ACS. Implementations handle type-specific install and
// uninstall logic (e.g. Helm charts).
type AddOn interface {
	Name() string
	Priority() uint
	Deploy(ctx context.Context) error
	Teardown(ctx context.Context) error
	IsOptional() bool
}

func (d *Deployer) deployAddOns(ctx context.Context, addOns []AddOn) error {
	if len(addOns) == 0 {
		return nil
	}

	needPullSecrets := d.config.Roxie.ClusterType.NeedsPullSecrets()
	if err := d.prepareNamespace(ctx, d.config.Central.Namespace, needPullSecrets); err != nil {
		return fmt.Errorf("failed to prepare namespace: %w", err)
	}

	d.logger.Infof("Deploying %d add-on(s)...", len(addOns))

	for _, addon := range addOns {
		if err := addon.Deploy(ctx); err != nil {
			if !addon.IsOptional() {
				return fmt.Errorf("installing non-optional add-on %q: %w", addon.Name(), err)
			}
			d.logger.Warningf("Failed to install add-on %q: %v", addon.Name(), err)
		} else {
			d.logger.Successf("Add-on %q installed", addon.Name())
		}
	}

	return nil
}

func (d *Deployer) teardownAddOns(ctx context.Context, addOns []AddOn) {
	if len(addOns) == 0 {
		return
	}

	d.logger.Infof("Tearing down %d add-on(s)...", len(addOns))

	for _, addon := range addOns {
		if err := addon.Teardown(ctx); err != nil {
			if !addon.IsOptional() {
				d.logger.Errorf("Failed to tear down non-optional add-on %q: %v", addon.Name(), err)
			} else {
				d.logger.Warningf("Failed to tear down optional add-on %q: %v", addon.Name(), err)
			}
		} else {
			d.logger.Successf("Add-on %q torn down", addon.Name())
		}
	}
}

// AddOnConfig carries runtime dependencies (logger, verbosity) needed to construct add-on instances.
type AddOnConfig struct {
	log     *logger.Logger
	verbose bool
}

// ResolveEnabledAddOns returns the enabled add-ons sorted by descending priority (name for ties at zero).
func (d *Deployer) ResolveEnabledAddOns() ([]AddOn, error) {
	return resolveEnabledAddOns(d.config.Central, d.AddOnConfiguration())
}

func resolveEnabledAddOns(centralCfg CentralConfig, addOnCfg AddOnConfig) ([]AddOn, error) {
	var addOns []AddOn

	for name, enabled := range centralCfg.AddOns {
		if !enabled {
			continue
		}
		if !validAddOnName.MatchString(name) {
			return nil, fmt.Errorf("add-on name %q is invalid: must match %s", name, validAddOnName.String())
		}

		addOnDef, found := centralCfg.AvailableAddOns[name]
		if !found {
			return nil, fmt.Errorf("cannot enable unknown add-on %q", name)
		}

		addOn, err := createAddOnFromDefinition(centralCfg, addOnCfg, name, addOnDef)
		if err != nil {
			return nil, err
		}

		addOns = append(addOns, addOn)
	}

	sortAddOns(addOns)

	return addOns, nil
}

// Bigger priority means, deploy addOn first. Among the addOns with the same priority, sort by addOn name.
func sortAddOns(addOns []AddOn) {
	slices.SortFunc(addOns, func(a, b AddOn) int {
		if a.Priority() != b.Priority() {
			return cmp.Compare(b.Priority(), a.Priority()) // higher first
		}
		return cmp.Compare(a.Name(), b.Name())
	})

}

// createAddOnFromDefinition dispatches to the concrete add-on constructor based on which chart source is set.
func createAddOnFromDefinition(
	centralCfg CentralConfig,
	addOnCfg AddOnConfig,
	name string,
	addOnDef CentralAddOnDefinition,
) (AddOn, error) {
	var addOn AddOn
	var err error

	if addOnDef.HelmChart != nil && addOnDef.StackRoxRepoHelmChart != nil {
		return nil, errors.New("add-on is configured for multiple add-on backends simultaneously")
	}

	commonProperties := addOnDef.CommonAddOnProperties
	switch {
	case addOnDef.StackRoxRepoHelmChart != nil:
		addOn, err = addOnDef.StackRoxRepoHelmChart.New(addOnCfg, commonProperties, name, centralCfg.Namespace)
	case addOnDef.HelmChart != nil:
		addOn, err = addOnDef.HelmChart.New(addOnCfg, commonProperties, name, centralCfg.Namespace)
	default:
		err = fmt.Errorf("add-on %q: no recognized add-on type configured", name)
	}
	if err != nil {
		return nil, err
	}

	return addOn, nil
}

// AddOnConfiguration builds an AddOnConfig from the deployer's runtime state.
func (d *Deployer) AddOnConfiguration() AddOnConfig {
	return AddOnConfig{
		log:     d.logger,
		verbose: d.verbose,
	}
}
