package gormsonal

import (
	"testing"

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
