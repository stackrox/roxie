package main

import (
	"fmt"

	"dario.cat/mergo"
	"github.com/stackrox/roxie/internal/deployer"
)

func assembleConfigForCommand(configBase *deployer.Config, configFromArgs deployer.Config, skipUserConfig bool) (deployer.Config, error) {
	var config deployer.Config
	if configBase == nil {
		// Start with default configuration.
		config = deployer.DefaultConfig()
	} else {
		config = *configBase
		// Neutralize operator-level values that older roxie baked into the
		// persisted manifest, so they re-resolve from the roxie section (and
		// later overrides take effect). Only meaningful for a loaded manifest,
		// and must happen before user config and args are merged on top.
		config.DropBakedOperatorDefaults()
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
