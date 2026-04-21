package gormsonal

import (
	"testing"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestNewGormConfigForSonalmodTables(t *testing.T) {
	fake := faker.New()
	t.Run("empty TablePrefix means no prefix", func(t *testing.T) {
		cfg := NewGormConfigForSonalmodTables(GormSonalmodTablesOpts{})
		ns := requireNamingStrategy(t, cfg)
		require.Empty(t, ns.TablePrefix)
		require.False(t, cfg.TranslateError)
	})

	t.Run("custom TablePrefix is preserved", func(t *testing.T) {
		prefix := fake.Lorem().Word() + "_"
		cfg := NewGormConfigForSonalmodTables(GormSonalmodTablesOpts{TablePrefix: prefix})
		ns := requireNamingStrategy(t, cfg)
		require.Equal(t, prefix, ns.TablePrefix)
	})

	t.Run("TranslateError is set when requested", func(t *testing.T) {
		cfg := NewGormConfigForSonalmodTables(GormSonalmodTablesOpts{TranslateError: true})
		require.True(t, cfg.TranslateError)
		ns := requireNamingStrategy(t, cfg)
		require.Empty(t, ns.TablePrefix)
	})
}

func requireNamingStrategy(t *testing.T, cfg *gorm.Config) schema.NamingStrategy {
	t.Helper()
	ns, ok := cfg.NamingStrategy.(schema.NamingStrategy)
	require.True(t, ok, "expected schema.NamingStrategy")
	return ns
}
