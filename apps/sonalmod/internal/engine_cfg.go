package internal

import (
	"github.com/spf13/viper"
	"go.uber.org/dig"
)

// EngineCfg is internal only configuration surface for the engine.
type EngineCfg struct {
	LogsFormatJSON  bool
	LogsOutputFile  string
	DefaultLogLevel *string
	Env             string
	Container       *dig.Container
	Config          *viper.Viper
}

func (cfg *EngineCfg) Apply(opts ...EngineOpt) {
	for _, opt := range opts {
		opt.apply(cfg)
	}
}

type EngineOpt interface {
	apply(opts *EngineCfg)
}

type EngineCfgOptFunc func(opts *EngineCfg)

func (f EngineCfgOptFunc) apply(opts *EngineCfg) {
	f(opts)
}

// WithEngineContainer sets the container for the engine.
// Used for internal only purposes.
func WithEngineContainer(container *dig.Container) EngineOpt { //nolint:ireturn
	return EngineCfgOptFunc(func(opts *EngineCfg) {
		opts.Container = container
	})
}

// WithEngineConfig sets the config for the engine.
// Used for internal only purposes.
func WithEngineConfig(config *viper.Viper) EngineOpt { //nolint:ireturn
	return EngineCfgOptFunc(func(opts *EngineCfg) {
		opts.Config = config
	})
}
