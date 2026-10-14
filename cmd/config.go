package main

import (
	"fmt"

	"dario.cat/mergo"
	"github.com/stackrox/roxie/internal/deployer"
)

func assembleConfigForCommand(configBase *deployer.Config, configFromArgs deployer.Config, skipUserConfig bool) (deployer.Config, error) {
	// Always start from the built-in defaults so that defaults (e.g.
	// roxie.imageRegistry) are applied even when reusing an existing cluster
	// manifest as the base configuration. Otherwise a manifest written by an
	// older roxie (which predates such defaults) would leave those fields empty.
	config := deployer.DefaultConfig()
	if configBase != nil {
		// Merge the provided base (e.g. an existing cluster manifest) on top of
		// the defaults; non-empty values in the base take precedence.
		if err := mergo.Merge(&config, configBase, mergo.WithOverride, mergo.WithoutDereference); err != nil {
			return deployer.Config{}, fmt.Errorf("merging base config over defaults: %w", err)
		}
	}

	// Apply user config on top (overriding defaults).
	if !skipUserConfig {
		if err := tryApplyUserDefaults(&config); err != nil {
			return deployer.Config{}, fmt.Errorf("applying user config: %w", err)
		}
	}

	// Apply changes from arg parsing.
	if err := mergo.Merge(&config, &configFromArgs, mergo.WithOverride, mergo.WithoutDereference); err != nil {
		return deployer.Config{}, fmt.Errorf("applying config patches from command line argument: %w", err)
	}

	return config, nil
}
