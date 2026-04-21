package internal

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"go.uber.org/dig"
)

func TestEngineCfg(t *testing.T) {
	t.Run("WithEngineContainer sets container", func(t *testing.T) {
		c := dig.New()
		cfg := &EngineCfg{}
		WithEngineContainer(c).apply(cfg)
		assert.Same(t, c, cfg.Container)
	})

	t.Run("WithEngineConfig sets viper", func(t *testing.T) {
		v := viper.New()
		cfg := &EngineCfg{}
		WithEngineConfig(v).apply(cfg)
		assert.Same(t, v, cfg.Config)
	})

	t.Run("Apply chains opts", func(t *testing.T) {
		c := dig.New()
		v := viper.New()
		cfg := &EngineCfg{}
		cfg.Apply(WithEngineContainer(c), WithEngineConfig(v))
		assert.Same(t, c, cfg.Container)
		assert.Same(t, v, cfg.Config)
	})
}
