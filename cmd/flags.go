package main

import (
	"time"

	"github.com/spf13/cobra"
	"github.com/stackrox/roxie/internal/deployer"
	"gopkg.in/yaml.v3"
)

type cliFlag struct {
	config      *deployer.Config
	longName    string
	shortName   string
	flagType    string
	applyFn     func(config *deployer.Config, val string) error
	noOptDefVal string
	description string
	persistent  bool
}

type flagOpt func(opts *cliFlag)

func (f *cliFlag) Set(val string) error {
	return f.applyFn(f.config, val)
}

func (f *cliFlag) String() string {
	return "" // Not sure what to return here.
}

func (f *cliFlag) Type() string {
	return f.flagType
}

func withApplyFnBool(boolApplyFn func(config *deployer.Config, val bool) error) flagOpt {
	return func(opts *cliFlag) {
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

func withApplyFn(flagType string, stringApplyFn func(config *deployer.Config, val string) error) flagOpt {
	return func(opts *cliFlag) {
		opts.flagType = flagType
		opts.applyFn = func(config *deployer.Config, val string) error {
			return stringApplyFn(config, val)
		}
	}
}

func withApplyFnDuration(durationApplyFn func(config *deployer.Config, duration time.Duration) error) flagOpt {
	return func(opts *cliFlag) {
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

func withNoOptDefVal(defVal string) flagOpt {
	return func(opts *cliFlag) {
		opts.noOptDefVal = defVal
	}
}

func withShortName(shortName string) flagOpt {
	return func(opts *cliFlag) {
		opts.shortName = shortName
	}
}

func withPersistent() flagOpt {
	return func(opts *cliFlag) {
		opts.persistent = true
	}
}

func registerFlag(cmd *cobra.Command, settings *deployer.Config, longName string, description string, flagOpts ...flagOpt) {
	f := cliFlag{
		config:      settings,
		longName:    longName,
		description: description,
	}
	for _, applyOpt := range flagOpts {
		applyOpt(&f)
	}
	flagSet := cmd.Flags()
	if f.persistent {
		flagSet = cmd.PersistentFlags()
	}
	flag := flagSet.VarPF(&f, f.longName, f.shortName, f.description)
	if f.noOptDefVal != "" {
		flag.NoOptDefVal = f.noOptDefVal
	}
}
