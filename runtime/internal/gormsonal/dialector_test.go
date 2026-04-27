package gormsonal

import (
	"errors"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"

	"github.com/stretchr/testify/require"
)

func TestNewGormDialector(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		dsn      string
		wantName string
	}{
		{
			name:     "sqlite_memory",
			dsn:      ":memory:",
			wantName: "sqlite",
		},
		{
			name:     "sqlite_file_prefix",
			dsn:      "file:app.db",
			wantName: "sqlite",
		},
		{
			name:     "sqlite_trimmed_memory",
			dsn:      "  :memory:  ",
			wantName: "sqlite",
		},
		{
			name:     "sqlite_dsn_contains_sqlite_token",
			dsn:      "something_with_sqlite_in_middle",
			wantName: "sqlite",
		},
		{
			name:     "sqlite_suffix_db",
			dsn:      "/tmp/data.db",
			wantName: "sqlite",
		},
		{
			name:     "sqlite_suffix_sqlite",
			dsn:      "/var/lib/app/state.sqlite",
			wantName: "sqlite",
		},
		{
			name:     "postgres_libpq",
			dsn:      "host=localhost user=u password=p dbname=test port=5432 sslmode=disable",
			wantName: "postgres",
		},
		{
			name:     "postgres_url",
			dsn:      "postgres://user:pass@localhost:5432/mydb?sslmode=disable",
			wantName: "postgres",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d := NewGormDialector(tt.dsn)
			require.NotNil(t, d)
			require.Equal(t, tt.wantName, d.Name())
		})
	}
}

func TestDialectorTranslateFallback(t *testing.T) {
	t.Parallel()

	want := errors.New("boom")

	d := Dialector{Dialector: stubDialector{}}
	require.Same(t, want, d.Translate(want))
}

type stubDialector struct{}

func (stubDialector) Name() string { return "stub" }

func (stubDialector) Initialize(*gorm.DB) error { return nil }

func (stubDialector) Migrator(*gorm.DB) gorm.Migrator { return nil }

func (stubDialector) DataTypeOf(*schema.Field) string { return "" }

func (stubDialector) DefaultValueOf(*schema.Field) clause.Expression { return nil }

func (stubDialector) BindVarTo(clause.Writer, *gorm.Statement, any) {}

func (stubDialector) QuoteTo(clause.Writer, string) {}

func (stubDialector) Explain(string, ...any) string { return "" }
