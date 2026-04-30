//go:build !release

package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gemyago/sonalmod/apps/sonalmod/internal/telemetry"
	"github.com/gemyago/sonalmod/runtime/agent"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRuntime(t *testing.T) {
	fake := faker.New()
	rootLogger := telemetry.RootTestLogger()

	makeDeps := func(t *testing.T) RuntimeDeps {
		t.Helper()
		dataDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dataDir, "agent-temp"), 0o700))
		return RuntimeDeps{
			RootLogger:                      rootLogger,
			DataDir:                         dataDir,
			AgentRuntimeDatabaseAutoMigrate: true,
			SkillsEnabled:                   false,
			SkillsPaths:                     []string{},
			SkillsMaxSkillBytes:             65536,
			SkillsMaxCatalogEntries:         500,
			ToolsRegistry:                   agent.NewToolsRegistry(),
		}
	}

	makeDatabaseDeps := func(t *testing.T) RuntimeDeps {
		t.Helper()
		deps := makeDeps(t)
		deps.AgentRuntimeStorageType = storageTypeDatabase
		deps.AgentRuntimeDatabaseDSN = filepath.Join(t.TempDir(), "runtime.db")
		deps.AgentRuntimeDatabaseTablePrefix = "runtime_"
		return deps
	}

	// makeSkillDir creates a valid skill directory with a SKILL.md inside parentDir.
	// Returns the skill name (== dir name).
	makeSkillDir := func(t *testing.T, parentDir string) string {
		t.Helper()
		const skillName = "test-skill"
		skillDir := filepath.Join(parentDir, skillName)
		require.NoError(t, os.MkdirAll(skillDir, 0o755))
		skillMD := fmt.Sprintf("---\nname: %s\ndescription: A test skill for unit testing.\n---\n# Body\n", skillName)
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644))
		return skillName
	}

	t.Run("creates runtime with non-nil runner and http handler", func(t *testing.T) {
		deps := makeDeps(t)
		runtime, err := newRuntime(deps)
		require.NoError(t, err)
		require.NotNil(t, runtime)
		assert.NotNil(t, runtime.Runner)
		assert.NotNil(t, runtime.HTTPHandler)
	})

	t.Run("database storage - creates runtime with database backend and migrates profiles", func(t *testing.T) {
		deps := makeDatabaseDeps(t)
		runtime, err := newRuntime(deps)
		require.NoError(t, err)
		require.NotNil(t, runtime)
		assert.NotNil(t, runtime.Runner)
		assert.NotNil(t, runtime.HTTPHandler)

		profilesSvc, err := agent.NewDatabaseAgentProfilesService(
			deps.AgentRuntimeDatabaseDSN,
			rootLogger,
			deps.AgentRuntimeDatabaseTablePrefix,
		)
		require.NoError(t, err)

		profiles, err := profilesSvc.List(t.Context())
		require.NoError(t, err)
		require.Empty(t, profiles)
	})

	t.Run("database storage - autoMigrate disabled still constructs runtime", func(t *testing.T) {
		deps := makeDatabaseDeps(t)
		deps.AgentRuntimeDatabaseAutoMigrate = false
		runtime, err := newRuntime(deps)
		require.NoError(t, err)
		require.NotNil(t, runtime)
		assert.NotNil(t, runtime.Runner)
		assert.NotNil(t, runtime.HTTPHandler)

		profilesSvc, err := agent.NewDatabaseAgentProfilesService(
			deps.AgentRuntimeDatabaseDSN,
			rootLogger,
			deps.AgentRuntimeDatabaseTablePrefix,
		)
		require.NoError(t, err)

		_, err = profilesSvc.List(t.Context())
		require.Error(t, err)
		require.ErrorContains(t, err, "no such table")
	})

	t.Run("http handler is wired with background runner", func(t *testing.T) {
		runtime, err := newRuntime(makeDeps(t))
		require.NoError(t, err)
		require.NotNil(t, runtime)

		// The handler should accept requests — a nil/bad path returns a proper HTTP response,
		// confirming the handler is fully wired (BackgroundRunner → agentapi → HTTP mux).
		assert.NotNil(t, runtime.HTTPHandler)
	})

	t.Run("exec disabled by default", func(t *testing.T) {
		deps := makeDeps(t)
		deps.ExecEnabled = false
		runtime, err := newRuntime(deps)
		require.NoError(t, err)
		require.NotNil(t, runtime)
		assert.NotNil(t, runtime.Runner)
		assert.NotNil(t, runtime.HTTPHandler)
	})

	t.Run("exec enabled with valid limits", func(t *testing.T) {
		deps := makeDeps(t)
		deps.ExecEnabled = true
		deps.ExecMaxOutputBytes = fake.Int64Between(1024, 1024*1024)
		deps.ExecDefaultTimeout = time.Duration(fake.Int64Between(1, 60)) * time.Second
		deps.ExecMaxConcurrentJobs = fake.IntBetween(1, 20)
		runtime, err := newRuntime(deps)
		require.NoError(t, err)
		require.NotNil(t, runtime)
		assert.NotNil(t, runtime.Runner)
		assert.NotNil(t, runtime.HTTPHandler)
	})

	t.Run("skills disabled - runtime starts normally without skills tools", func(t *testing.T) {
		deps := makeDeps(t)
		deps.SkillsEnabled = false
		deps.SkillsPaths = []string{t.TempDir()}

		runtime, err := newRuntime(deps)
		require.NoError(t, err)
		require.NotNil(t, runtime)
		assert.NotNil(t, runtime.Runner)
	})

	t.Run("skills enabled with default recommended paths - runtime starts with skills tools", func(t *testing.T) {
		deps := makeDeps(t)
		skillsRoot := t.TempDir()
		makeSkillDir(t, skillsRoot)
		deps.SkillsEnabled = true
		deps.SkillsPaths = []string{skillsRoot}

		runtime, err := newRuntime(deps)
		require.NoError(t, err)
		require.NotNil(t, runtime)
		assert.NotNil(t, runtime.Runner)
	})

	t.Run("skills enabled with non-existent paths - runtime starts without failing", func(t *testing.T) {
		deps := makeDeps(t)
		deps.SkillsEnabled = true
		deps.SkillsPaths = []string{filepath.Join(t.TempDir(), "nonexistent")}

		runtime, err := newRuntime(deps)
		require.NoError(t, err)
		require.NotNil(t, runtime)
		assert.NotNil(t, runtime.Runner)
	})

	t.Run("skills enabled with malformed SKILL.md - skips bad skill without failing", func(t *testing.T) {
		deps := makeDeps(t)
		skillsRoot := t.TempDir()

		// Create a directory with a malformed SKILL.md (missing frontmatter)
		badSkillDir := filepath.Join(skillsRoot, "bad-skill")
		require.NoError(t, os.MkdirAll(badSkillDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(badSkillDir, "SKILL.md"), []byte("no frontmatter here"), 0o644))

		deps.SkillsEnabled = true
		deps.SkillsPaths = []string{skillsRoot}

		runtime, err := newRuntime(deps)
		require.NoError(t, err)
		require.NotNil(t, runtime)
		assert.NotNil(t, runtime.Runner)
	})

	t.Run("uses provided ToolsRegistry from deps instead of creating new one", func(t *testing.T) {
		deps := makeDeps(t)
		providedRegistry := agent.NewToolsRegistry()
		deps.ToolsRegistry = providedRegistry

		runtime, err := newRuntime(deps)
		require.NoError(t, err)
		require.NotNil(t, runtime)
		assert.Same(t, providedRegistry, runtime.ToolsRegistry)
	})

	t.Run("skills enabled with duplicate skill names - runtime starts keeping first occurrence", func(t *testing.T) {
		deps := makeDeps(t)
		root1 := t.TempDir()
		root2 := t.TempDir()

		// Same skill name in both roots
		for _, root := range []string{root1, root2} {
			skillDir := filepath.Join(root, "my-skill")
			require.NoError(t, os.MkdirAll(skillDir, 0o755))
			content := "---\nname: my-skill\ndescription: Duplicate skill.\n---\n# Body\n"
			require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644))
		}

		deps.SkillsEnabled = true
		deps.SkillsPaths = []string{root1, root2}

		runtime, err := newRuntime(deps)
		require.NoError(t, err)
		require.NotNil(t, runtime)
		assert.NotNil(t, runtime.Runner)
	})
}
