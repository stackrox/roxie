package main

import (
	"time"

	"github.com/spf13/cobra"
	"github.com/stackrox/roxie/internal/deployer"
	"gopkg.in/yaml.v3"
)

type CliFlag struct {
	config      *deployer.Config
	longName    string
	shortName   string
	flagType    string
	applyFn     func(config *deployer.Config, val string) error
	noOptDefVal string
	description string
}

type FlagOpt func(opts *CliFlag)

func (f *CliFlag) Set(val string) error {
	return f.applyFn(f.config, val)
}

func (f *CliFlag) String() string {
	return "" // Not sure what to return here.
}

func (f *CliFlag) Type() string {
	return f.flagType
}

func WithApplyFnBool(boolApplyFn func(config *deployer.Config, val bool) error) FlagOpt {
	return func(opts *CliFlag) {
		opts.flagType = "bool"
		opts.applyFn = func(config *deployer.Config, val string) error {
			var valParsed bool
			if err := yaml.Unmarshal([]byte(val), &valParsed); err != nil {
				return err
			}
			return boolApplyFn(config, valParsed)
		}
	}
}

func WithApplyFn(flagType string, stringApplyFn func(config *deployer.Config, val string) error) FlagOpt {
	return func(opts *CliFlag) {
		opts.flagType = flagType
		opts.applyFn = func(config *deployer.Config, val string) error {
			return stringApplyFn(config, val)
		}
	}
}

func WithApplyFnDuration(durationApplyFn func(config *deployer.Config, duration time.Duration) error) FlagOpt {
	return func(opts *CliFlag) {
		opts.flagType = "duration"
		opts.applyFn = func(config *deployer.Config, val string) error {
			var duration time.Duration
			duration, err := time.ParseDuration(val)
			if err != nil {
				return err
			}
			return durationApplyFn(config, duration)
		}
	}
}

func WithNoOptDefVal(defVal string) FlagOpt {
	return func(opts *CliFlag) {
		opts.noOptDefVal = defVal
	}
}

func WithShortName(shortName string) FlagOpt {
	return func(opts *CliFlag) {
		opts.shortName = shortName
	}
}

func registerFlag(cmd *cobra.Command, settings *deployer.Config, longName string, description string, flagOpts ...FlagOpt) {
	cliFlag := CliFlag{
		config:      settings,
		longName:    longName,
		description: description,
	}
	for _, applyOpt := range flagOpts {
		applyOpt(&cliFlag)

	}
	flag := cmd.Flags().VarPF(&cliFlag, cliFlag.longName, cliFlag.shortName, cliFlag.description)
	if cliFlag.noOptDefVal != "" {
		flag.NoOptDefVal = cliFlag.noOptDefVal
	}
}
