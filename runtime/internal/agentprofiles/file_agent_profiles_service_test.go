package agentprofiles

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileAgentProfilesService(t *testing.T) {
	fake := faker.New()

	makeService := func(t *testing.T, baseDir string) *FileAgentProfilesService {
		t.Helper()
		svc, err := NewFileAgentProfilesService(baseDir, testLogger(t))
		require.NoError(t, err)
		return svc
	}

	makeCreateParams := func() CreateAgentProfileParams {
		return CreateAgentProfileParams{
			Name:         fake.Lexify("profile-????????"),
			DisplayName:  fake.Person().Name(),
			Role:         "assistant",
			Instructions: fake.Lorem().Sentence(8),
			ToolRefs: []string{
				"tool.read",
				"tool.write",
			},
			ExecutionSettings: ExecutionSettings{
				DefaultModel: "provider/model",
			},
		}
	}

	t.Run("Create/Get/List/Delete", func(t *testing.T) {
		baseDir := t.TempDir()
		svc := makeService(t, baseDir)
		ctx := t.Context()

		created, err := svc.Create(ctx, makeCreateParams())
		require.NoError(t, err)
		require.NotNil(t, created)

		filePath := filepath.Join(baseDir, "agents", created.Name+".md")
		_, err = os.Stat(filePath)
		require.NoError(t, err)

		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Contains(t, string(content), "---\n")
		assert.Contains(t, string(content), "model: provider/model")
		assert.Contains(t, string(content), "tools:")
		assert.Contains(t, string(content), created.Instructions)

		got, err := svc.Get(ctx, created.Name)
		require.NoError(t, err)
		assert.Equal(t, created.Name, got.Name)
		assert.Equal(t, created.CreatedAt, got.CreatedAt)

		listed, err := svc.List(ctx)
		require.NoError(t, err)
		require.Len(t, listed, 1)
		assert.Equal(t, created.Name, listed[0].Name)

		err = svc.Delete(ctx, created.Name)
		require.NoError(t, err)

		_, err = svc.Get(ctx, created.Name)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrAgentProfileNotFound)
	})

	t.Run("Create returns conflict for duplicate name", func(t *testing.T) {
		svc := makeService(t, t.TempDir())
		ctx := t.Context()
		params := makeCreateParams()

		_, err := svc.Create(ctx, params)
		require.NoError(t, err)

		_, err = svc.Create(ctx, params)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrAgentProfileNameConflict)
	})

	t.Run("List returns profiles sorted by created_at", func(t *testing.T) {
		svc := makeService(t, t.TempDir())
		ctx := t.Context()

		first, err := svc.Create(ctx, makeCreateParams())
		require.NoError(t, err)
		time.Sleep(2 * time.Millisecond)
		second, err := svc.Create(ctx, makeCreateParams())
		require.NoError(t, err)

		listed, err := svc.List(ctx)
		require.NoError(t, err)
		require.Len(t, listed, 2)
		assert.Equal(t, first.Name, listed[0].Name)
		assert.Equal(t, second.Name, listed[1].Name)
	})

	t.Run("List ignores non-markdown files", func(t *testing.T) {
		baseDir := t.TempDir()
		svc := makeService(t, baseDir)

		valid, err := svc.Create(t.Context(), makeCreateParams())
		require.NoError(t, err)
		require.NoError(
			t,
			os.WriteFile(filepath.Join(baseDir, "agents", "skip.txt"), []byte("noop"), 0600),
		)

		listed, err := svc.List(t.Context())
		require.NoError(t, err)
		require.Len(t, listed, 1)
		assert.Equal(t, valid.Name, listed[0].Name)
	})

	t.Run("List ignores nested directories", func(t *testing.T) {
		baseDir := t.TempDir()
		svc := makeService(t, baseDir)
		require.NoError(t, os.Mkdir(filepath.Join(baseDir, "agents", "nested"), 0750))

		_, err := svc.Create(t.Context(), makeCreateParams())
		require.NoError(t, err)

		listed, err := svc.List(t.Context())
		require.NoError(t, err)
		require.Len(t, listed, 1)
	})

	t.Run("Update changes mutable fields and preserves immutable fields", func(t *testing.T) {
		svc := makeService(t, t.TempDir())
		ctx := t.Context()

		created, err := svc.Create(ctx, makeCreateParams())
		require.NoError(t, err)
		time.Sleep(2 * time.Millisecond)

		updated, err := svc.Update(ctx, created.Name, UpdateAgentProfileParams{
			DisplayName:  " Updated Name ",
			Role:         " reviewer ",
			Instructions: " updated instructions ",
			ToolRefs:     []string{" tool.write ", "tool.read", "tool.write"},
			ExecutionSettings: ExecutionSettings{
				DefaultModel: " provider/new-model ",
			},
		})
		require.NoError(t, err)
		assert.Equal(t, created.Name, updated.Name)
		assert.Equal(t, created.CreatedAt, updated.CreatedAt)
		assert.False(t, updated.UpdatedAt.Before(created.UpdatedAt))
		assert.Equal(t, "Updated Name", updated.DisplayName)
		assert.Equal(t, "reviewer", updated.Role)
		assert.Equal(t, "updated instructions", updated.Instructions)
		assert.Equal(t, []string{"tool.write", "tool.read"}, updated.ToolRefs)
		assert.Equal(t, "provider/new-model", updated.ExecutionSettings.DefaultModel)

		content, err := os.ReadFile(filepath.Join(svc.baseDir, "agents", created.Name+".md"))
		require.NoError(t, err)
		assert.Contains(t, string(content), "updated instructions")
		assert.Contains(t, string(content), "model: provider/new-model")
	})

	t.Run("Update/Delete return not found for unknown profile", func(t *testing.T) {
		svc := makeService(t, t.TempDir())
		ctx := t.Context()

		_, err := svc.Update(ctx, "missing-profile", UpdateAgentProfileParams{
			DisplayName:  "x",
			Role:         "assistant",
			Instructions: "x",
			ExecutionSettings: ExecutionSettings{
				DefaultModel: "provider/model",
			},
		})
		require.Error(t, err)
		require.ErrorIs(t, err, ErrAgentProfileNotFound)

		err = svc.Delete(ctx, "missing-profile")
		require.Error(t, err)
		require.ErrorIs(t, err, ErrAgentProfileNotFound)
	})

	t.Run("AutoMigrate is no-op", func(t *testing.T) {
		svc := makeService(t, t.TempDir())
		require.NoError(t, svc.AutoMigrate())
	})

	t.Run("restart-shaped reload reads unchanged profile", func(t *testing.T) {
		baseDir := t.TempDir()
		ctx := t.Context()
		params := makeCreateParams()

		svc1 := makeService(t, baseDir)
		created, err := svc1.Create(ctx, params)
		require.NoError(t, err)

		svc2 := makeService(t, baseDir)
		loaded, err := svc2.Get(ctx, created.Name)
		require.NoError(t, err)
		assert.Equal(t, *created, *loaded)
	})

	t.Run("round-trips execution settings variants", func(t *testing.T) {
		t.Run("omitted regular mode keeps legacy model frontmatter", func(t *testing.T) {
			baseDir := t.TempDir()
			svc := makeService(t, baseDir)

			created, err := svc.Create(t.Context(), makeCreateParams())
			require.NoError(t, err)

			content, err := os.ReadFile(filepath.Join(baseDir, "agents", created.Name+".md"))
			require.NoError(t, err)
			assert.Contains(t, string(content), "model: provider/model")
			assert.NotContains(t, string(content), "executionSettings:")

			reloaded, err := makeService(t, baseDir).Get(t.Context(), created.Name)
			require.NoError(t, err)
			assert.Equal(t, created.ExecutionSettings, reloaded.ExecutionSettings)
		})

		t.Run("explicit regular mode persists execution settings block", func(t *testing.T) {
			baseDir := t.TempDir()
			svc := makeService(t, baseDir)

			created, err := svc.Create(t.Context(), CreateAgentProfileParams{
				Name:         fake.Lexify("profile-????????"),
				DisplayName:  fake.Person().Name(),
				Role:         "assistant",
				Instructions: fake.Lorem().Sentence(8),
				ToolRefs:     []string{"tool.read"},
				ExecutionSettings: ExecutionSettings{
					Mode:         ExecutionModeRegular,
					DefaultModel: "provider/model",
				},
			})
			require.NoError(t, err)

			content, err := os.ReadFile(filepath.Join(baseDir, "agents", created.Name+".md"))
			require.NoError(t, err)
			assert.Contains(t, string(content), "executionSettings:")
			assert.Contains(t, string(content), "mode: regular")
			assert.Contains(t, string(content), "defaultModel: provider/model")
			assert.NotContains(t, string(content), "\nmodel:")

			reloaded, err := makeService(t, baseDir).Get(t.Context(), created.Name)
			require.NoError(t, err)
			assert.Equal(t, created.ExecutionSettings, reloaded.ExecutionSettings)
		})

		t.Run("acp-stdio mode persists execution settings block", func(t *testing.T) {
			baseDir := t.TempDir()
			svc := makeService(t, baseDir)

			created, err := svc.Create(t.Context(), CreateAgentProfileParams{
				Name:         fake.Lexify("profile-????????"),
				DisplayName:  fake.Person().Name(),
				Role:         "assistant",
				Instructions: fake.Lorem().Sentence(8),
				ToolRefs:     []string{"tool.read"},
				ExecutionSettings: ExecutionSettings{
					Mode: ExecutionModeACPStdio,
					AgentCommand: ACPStdioAgentCommand{
						Command: "opencode",
						Args:    []string{"acp", "--safe"},
					},
					Cwd: "/workspace",
				},
			})
			require.NoError(t, err)

			content, err := os.ReadFile(filepath.Join(baseDir, "agents", created.Name+".md"))
			require.NoError(t, err)
			assert.Contains(t, string(content), "executionSettings:")
			assert.Contains(t, string(content), "mode: acp-stdio")
			assert.Contains(t, string(content), "agentCommand:")
			assert.Contains(t, string(content), "command: opencode")
			assert.Contains(t, string(content), "- acp")
			assert.Contains(t, string(content), "cwd: /workspace")
			assert.NotContains(t, string(content), "\nmodel:")

			reloaded, err := makeService(t, baseDir).Get(t.Context(), created.Name)
			require.NoError(t, err)
			assert.Equal(t, created.ExecutionSettings, reloaded.ExecutionSettings)
		})
	})

	t.Run("loads legacy regular profiles with model frontmatter", func(t *testing.T) {
		baseDir := t.TempDir()
		svc := makeService(t, baseDir)
		path := filepath.Join(baseDir, "agents", "profile-legacy.md")
		content := "---\nname: profile-legacy\nrole: assistant\nmodel: provider/model\ntools: []\n---\nlegacy instructions\n"
		require.NoError(t, os.WriteFile(path, []byte(content), 0600))

		profile, err := svc.Get(t.Context(), "profile-legacy")
		require.NoError(t, err)
		assert.Equal(t, ExecutionSettings{
			DefaultModel: "provider/model",
		}, profile.ExecutionSettings)
	})

	t.Run("error paths", func(t *testing.T) {
		t.Run("NewFileAgentProfilesService rejects empty base dir", func(t *testing.T) {
			_, err := NewFileAgentProfilesService("", testLogger(t))
			require.Error(t, err)
		})

		t.Run("NewFileAgentProfilesService returns error when base path is a file", func(t *testing.T) {
			baseDir := t.TempDir()
			fileBase := filepath.Join(baseDir, "not-a-dir")
			require.NoError(t, os.WriteFile(fileBase, []byte("x"), 0600))

			_, err := NewFileAgentProfilesService(fileBase, testLogger(t))
			require.Error(t, err)
		})

		t.Run("List returns error on unreadable directory", func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("chmod permissions differ on Windows")
			}
			baseDir := t.TempDir()
			svc := makeService(t, baseDir)
			profilesDir := filepath.Join(baseDir, "agents")
			require.NoError(t, os.Chmod(profilesDir, 0000))
			t.Cleanup(func() { _ = os.Chmod(profilesDir, 0750) })

			_, err := svc.List(t.Context())
			require.Error(t, err)
		})

		t.Run("List returns empty when profiles dir is missing", func(t *testing.T) {
			baseDir := t.TempDir()
			svc := makeService(t, baseDir)
			profilesDir := filepath.Join(baseDir, "agents")
			require.NoError(t, os.RemoveAll(profilesDir))

			listed, err := svc.List(t.Context())
			require.NoError(t, err)
			assert.Empty(t, listed)
		})

		t.Run("List returns error on malformed markdown frontmatter", func(t *testing.T) {
			baseDir := t.TempDir()
			svc := makeService(t, baseDir)
			path := filepath.Join(baseDir, "agents", "bad.md")
			require.NoError(t, os.WriteFile(path, []byte("name: no-frontmatter"), 0600))

			_, err := svc.List(t.Context())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "expected markdown with YAML frontmatter")
		})

		t.Run("Get returns validation error when required frontmatter is missing", func(t *testing.T) {
			baseDir := t.TempDir()
			svc := makeService(t, baseDir)
			path := filepath.Join(baseDir, "agents", "bad.md")
			content := "---\nname: bad\nrole: assistant\ntools: []\n---\ninstructions\n"
			require.NoError(t, os.WriteFile(path, []byte(content), 0600))

			_, err := svc.Get(t.Context(), "bad")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "missing required frontmatter field `model` or `executionSettings`")
		})

		t.Run("Get returns validation error when frontmatter name mismatches file", func(t *testing.T) {
			baseDir := t.TempDir()
			svc := makeService(t, baseDir)
			path := filepath.Join(baseDir, "agents", "bad.md")
			content := "---\nname: different\nrole: assistant\nmodel: provider/model\ntools: []\n---\ninstructions\n"
			require.NoError(t, os.WriteFile(path, []byte(content), 0600))

			_, err := svc.Get(t.Context(), "bad")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "must match file name")
		})

		t.Run("Create returns validation error for invalid payload", func(t *testing.T) {
			svc := makeService(t, t.TempDir())
			_, err := svc.Create(t.Context(), CreateAgentProfileParams{
				Name:         "invalid",
				Role:         "assistant",
				Instructions: "ok",
				ToolRefs:     []string{" "},
				ExecutionSettings: ExecutionSettings{
					DefaultModel: "provider/model",
				},
			})
			require.Error(t, err)
		})

		t.Run("Create returns write error when agents dir is not writable", func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("chmod permissions differ on Windows")
			}

			baseDir := t.TempDir()
			svc := makeService(t, baseDir)
			agentsDir := filepath.Join(baseDir, "agents")
			require.NoError(t, os.Chmod(agentsDir, 0550))
			t.Cleanup(func() { _ = os.Chmod(agentsDir, 0750) })

			_, err := svc.Create(t.Context(), makeCreateParams())
			require.Error(t, err)
		})

		t.Run("Update returns validation error for invalid payload", func(t *testing.T) {
			svc := makeService(t, t.TempDir())
			created, err := svc.Create(t.Context(), makeCreateParams())
			require.NoError(t, err)

			_, err = svc.Update(t.Context(), created.Name, UpdateAgentProfileParams{
				DisplayName:  "x",
				Role:         "assistant",
				Instructions: "ok",
				ExecutionSettings: ExecutionSettings{
					DefaultModel: " ",
				},
			})
			require.Error(t, err)
		})

		t.Run("Update returns write error when profile file is read-only", func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("chmod permissions differ on Windows")
			}

			baseDir := t.TempDir()
			svc := makeService(t, baseDir)
			created, err := svc.Create(t.Context(), makeCreateParams())
			require.NoError(t, err)

			path := filepath.Join(baseDir, "agents", created.Name+".md")
			require.NoError(t, os.Chmod(path, 0400))
			t.Cleanup(func() { _ = os.Chmod(path, 0600) })

			_, err = svc.Update(t.Context(), created.Name, UpdateAgentProfileParams{
				DisplayName:  "x",
				Role:         "assistant",
				Instructions: "ok",
				ExecutionSettings: ExecutionSettings{
					DefaultModel: "provider/model",
				},
			})
			require.Error(t, err)
		})

		t.Run("Delete returns remove error when path is a directory", func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("chmod permissions differ on Windows")
			}
			baseDir := t.TempDir()
			svc := makeService(t, baseDir)
			ctx := t.Context()
			created, err := svc.Create(ctx, makeCreateParams())
			require.NoError(t, err)

			path := filepath.Join(baseDir, "agents", created.Name+".md")
			require.NoError(t, os.Remove(path))
			require.NoError(t, os.Mkdir(path, 0750))
			require.NoError(t, os.WriteFile(filepath.Join(path, "child"), []byte("x"), 0600))
			t.Cleanup(func() { _ = os.RemoveAll(path) })

			err = svc.Delete(ctx, created.Name)
			require.Error(t, err)
		})
	})
}

func TestParseProfileMarkdownRoundTrip(t *testing.T) {
	svc, err := NewFileAgentProfilesService(t.TempDir(), testLogger(t))
	require.NoError(t, err)

	targetPath := filepath.Join(svc.baseDir, "agents", "profile-main.md")
	original := AgentProfile{
		Name:         "profile-main",
		DisplayName:  "Main Profile",
		Role:         "assistant",
		Instructions: "  line one\nline two  ",
		ToolRefs:     []string{"tool.read", "tool.write"},
		ExecutionSettings: ExecutionSettings{
			DefaultModel: "provider/model",
		},
	}

	require.NoError(t, svc.writeProfileFile(targetPath, original))

	data, err := os.ReadFile(targetPath)
	require.NoError(t, err)
	parsed, err := parseProfileMarkdown(targetPath, data)
	require.NoError(t, err)
	assert.Equal(t, original.Name, parsed.Name)
	assert.Equal(t, original.DisplayName, parsed.DisplayName)
	assert.Equal(t, original.Role, parsed.Role)
	assert.Equal(t, "line one\nline two", parsed.Instructions)
	assert.Equal(t, original.ToolRefs, parsed.ToolRefs)
	assert.Equal(t, original.ExecutionSettings.DefaultModel, parsed.ExecutionSettings.DefaultModel)

	t.Run("preserves explicit regular mode", func(t *testing.T) {
		regularPath := filepath.Join(svc.baseDir, "agents", "profile-regular.md")
		regularProfile := AgentProfile{
			Name:         "profile-regular",
			DisplayName:  "Regular Profile",
			Role:         "assistant",
			Instructions: "regular instructions",
			ToolRefs:     []string{"tool.read"},
			ExecutionSettings: ExecutionSettings{
				Mode:         ExecutionModeRegular,
				DefaultModel: "provider/model",
			},
		}

		require.NoError(t, svc.writeProfileFile(regularPath, regularProfile))

		regularData, readErr := os.ReadFile(regularPath)
		require.NoError(t, readErr)
		regularParsed, parseErr := parseProfileMarkdown(regularPath, regularData)
		require.NoError(t, parseErr)
		assert.Equal(t, regularProfile.ExecutionSettings, regularParsed.ExecutionSettings)
	})

	t.Run("preserves acp-stdio settings", func(t *testing.T) {
		acpPath := filepath.Join(svc.baseDir, "agents", "profile-acp.md")
		acpProfile := AgentProfile{
			Name:         "profile-acp",
			DisplayName:  "ACP Profile",
			Role:         "assistant",
			Instructions: "acp instructions",
			ToolRefs:     []string{"tool.read"},
			ExecutionSettings: ExecutionSettings{
				Mode: ExecutionModeACPStdio,
				AgentCommand: ACPStdioAgentCommand{
					Command: "opencode",
					Args:    []string{"acp", "--safe"},
				},
				Cwd: "/workspace",
			},
		}

		require.NoError(t, svc.writeProfileFile(acpPath, acpProfile))

		acpData, readErr := os.ReadFile(acpPath)
		require.NoError(t, readErr)
		acpParsed, parseErr := parseProfileMarkdown(acpPath, acpData)
		require.NoError(t, parseErr)
		assert.Equal(t, acpProfile.ExecutionSettings, acpParsed.ExecutionSettings)
	})
}

func TestParseProfileMarkdownValidation(t *testing.T) {
	t.Run("rejects missing frontmatter delimiters", func(t *testing.T) {
		_, err := parseProfileMarkdown("/tmp/profile-main.md", []byte("name: nope"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected markdown with YAML frontmatter")
	})

	t.Run("rejects malformed frontmatter yaml", func(t *testing.T) {
		content := "---\nname: [bad\n---\ninstructions\n"
		_, err := parseProfileMarkdown("/tmp/profile-main.md", []byte(content))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse agent profile frontmatter")
	})

	t.Run("rejects missing name", func(t *testing.T) {
		content := "---\nrole: assistant\nmodel: provider/model\ntools: []\n---\ninstructions\n"
		_, err := parseProfileMarkdown("/tmp/profile-main.md", []byte(content))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing required frontmatter field `name`")
	})

	t.Run("rejects missing role", func(t *testing.T) {
		content := "---\nname: profile-main\nmodel: provider/model\ntools: []\n---\ninstructions\n"
		_, err := parseProfileMarkdown("/tmp/profile-main.md", []byte(content))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing required frontmatter field `role`")
	})

	t.Run("rejects empty instructions body", func(t *testing.T) {
		content := "---\nname: profile-main\nrole: assistant\nmodel: provider/model\ntools: []\n---\n \n"
		_, err := parseProfileMarkdown("/tmp/profile-main.md", []byte(content))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "markdown body must contain instructions")
	})

	t.Run("rejects invalid normalized payload", func(t *testing.T) {
		content := "---\nname: profile-main\nrole: assistant\nmodel: provider/model\ntools:\n  - tool.read\n  - \" \"\n---\ninstructions\n"
		_, err := parseProfileMarkdown("/tmp/profile-main.md", []byte(content))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tool_refs")
	})
}

func TestProfileTimestampsFromFileInfo(t *testing.T) {
	modTime := time.Date(2026, 4, 23, 12, 30, 0, 100, time.UTC)

	t.Run("falls back to mod time when creation time is unavailable", func(t *testing.T) {
		info := testFileInfo{
			modTime: modTime,
			sys:     &struct{}{},
		}
		createdAt, updatedAt := profileTimestampsFromFileInfo(info)
		assert.Equal(t, modTime, createdAt)
		assert.Equal(t, modTime, updatedAt)
	})

	t.Run("uses creation time when available", func(t *testing.T) {
		birth := modTime.Add(-time.Hour)
		info := testFileInfo{
			modTime: modTime,
			sys: &testBirthtimeStat{
				Birthtimespec: testTimespec{
					Sec:  birth.Unix(),
					Nsec: int64(birth.Nanosecond()),
				},
			},
		}
		createdAt, updatedAt := profileTimestampsFromFileInfo(info)
		assert.Equal(t, birth, createdAt)
		assert.Equal(t, modTime, updatedAt)
	})

	t.Run("clamps creation time to updated time when creation is later", func(t *testing.T) {
		birth := modTime.Add(time.Hour)
		info := testFileInfo{
			modTime: modTime,
			sys: &testBirthtimeStat{
				Birthtimespec: testTimespec{
					Sec:  birth.Unix(),
					Nsec: int64(birth.Nanosecond()),
				},
			},
		}
		createdAt, updatedAt := profileTimestampsFromFileInfo(info)
		assert.Equal(t, modTime, createdAt)
		assert.Equal(t, modTime, updatedAt)
	})
}

func TestProfileTimestampsFromPath(t *testing.T) {
	t.Run("returns not found for missing file", func(t *testing.T) {
		_, _, err := profileTimestampsFromPath(filepath.Join(t.TempDir(), "agents", "missing.md"))
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrAgentProfileNotFound)
	})

	t.Run("returns stat error for invalid path", func(t *testing.T) {
		baseDir := t.TempDir()
		path := filepath.Join(baseDir, "agents.md", "bad.md")
		require.NoError(t, os.WriteFile(filepath.Join(baseDir, "agents.md"), []byte("x"), 0600))

		_, _, err := profileTimestampsFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "stat agent profile file")
	})
}

func TestCreationTimeFromFileInfo(t *testing.T) {
	modTime := time.Date(2026, 4, 23, 12, 30, 0, 0, time.UTC)

	t.Run("returns zero when sys is nil", func(t *testing.T) {
		assert.True(t, creationTimeFromFileInfo(testFileInfo{modTime: modTime}).IsZero())
	})

	t.Run("returns zero when sys is nil pointer", func(t *testing.T) {
		var stat *testBirthtimeStat
		assert.True(t, creationTimeFromFileInfo(testFileInfo{modTime: modTime, sys: stat}).IsZero())
	})

	t.Run("returns zero when sys is not a struct", func(t *testing.T) {
		assert.True(t, creationTimeFromFileInfo(testFileInfo{modTime: modTime, sys: 42}).IsZero())
	})

	t.Run("reads unix birthtime field", func(t *testing.T) {
		birth := modTime.Add(-2 * time.Hour)
		got := creationTimeFromFileInfo(testFileInfo{
			modTime: modTime,
			sys: &testBirthtimeStat{
				Birthtimespec: testTimespec{Sec: birth.Unix(), Nsec: int64(birth.Nanosecond())},
			},
		})
		assert.Equal(t, birth, got)
	})

	t.Run("reads alternate unix birthtim field", func(t *testing.T) {
		birth := modTime.Add(-3 * time.Hour)
		got := creationTimeFromFileInfo(testFileInfo{
			modTime: modTime,
			sys: &testBirthtimStat{
				Birthtim: testTimespec{Sec: birth.Unix(), Nsec: int64(birth.Nanosecond())},
			},
		})
		assert.Equal(t, birth, got)
	})

	t.Run("reads windows creation time field", func(t *testing.T) {
		birth := modTime.Add(-4 * time.Hour)
		got := creationTimeFromFileInfo(testFileInfo{
			modTime: modTime,
			sys: &testWindowsCreationStat{
				CreationTime: testCreationTimeValue{nanos: birth.UnixNano()},
			},
		})
		assert.Equal(t, birth, got)
	})
}

func TestTimeFromTimespecField(t *testing.T) {
	base := time.Date(2026, 4, 23, 12, 30, 0, 0, time.UTC)

	t.Run("rejects invalid value", func(t *testing.T) {
		assert.True(t, timeFromTimespecField(reflect.Value{}).IsZero())
	})

	t.Run("rejects nil pointer", func(t *testing.T) {
		var stat *testTimespec
		assert.True(t, timeFromTimespecField(reflect.ValueOf(stat)).IsZero())
	})

	t.Run("rejects non-struct value", func(t *testing.T) {
		assert.True(t, timeFromTimespecField(reflect.ValueOf(1)).IsZero())
	})

	t.Run("rejects invalid sec nsec values", func(t *testing.T) {
		got := timeFromTimespecField(reflect.ValueOf(testTimespec{Sec: 0, Nsec: -1}))
		assert.True(t, got.IsZero())
	})

	t.Run("parses valid timespec", func(t *testing.T) {
		got := timeFromTimespecField(reflect.ValueOf(testTimespec{
			Sec:  base.Unix(),
			Nsec: int64(base.Nanosecond()),
		}))
		assert.Equal(t, base, got)
	})
}

func TestTimeFromWindowsCreationField(t *testing.T) {
	base := time.Date(2026, 4, 23, 12, 30, 0, 0, time.UTC)

	t.Run("rejects invalid value", func(t *testing.T) {
		assert.True(t, timeFromWindowsCreationField(reflect.Value{}).IsZero())
	})

	t.Run("rejects nil pointer", func(t *testing.T) {
		var creation *testCreationTimeValue
		assert.True(t, timeFromWindowsCreationField(reflect.ValueOf(creation)).IsZero())
	})

	t.Run("rejects missing nanoseconds method", func(t *testing.T) {
		assert.True(t, timeFromWindowsCreationField(reflect.ValueOf(struct{}{})).IsZero())
	})

	t.Run("rejects non-positive nanoseconds", func(t *testing.T) {
		assert.True(t, timeFromWindowsCreationField(reflect.ValueOf(testCreationTimeValue{})).IsZero())
	})

	t.Run("parses nanoseconds method", func(t *testing.T) {
		got := timeFromWindowsCreationField(reflect.ValueOf(testCreationTimeValue{nanos: base.UnixNano()}))
		assert.Equal(t, base, got)
	})
}

func TestReflectAsInt64(t *testing.T) {
	t.Run("accepts signed ints", func(t *testing.T) {
		got, ok := reflectAsInt64(reflect.ValueOf(int32(7)))
		assert.True(t, ok)
		assert.Equal(t, int64(7), got)
	})

	t.Run("accepts unsigned ints", func(t *testing.T) {
		got, ok := reflectAsInt64(reflect.ValueOf(uint16(9)))
		assert.True(t, ok)
		assert.Equal(t, int64(9), got)
	})

	t.Run("rejects unsupported kinds", func(t *testing.T) {
		_, ok := reflectAsInt64(reflect.ValueOf("7"))
		assert.False(t, ok)
	})
}

type testFileInfo struct {
	modTime time.Time
	sys     any
}

func (f testFileInfo) Name() string       { return "profile.md" }
func (f testFileInfo) Size() int64        { return 0 }
func (f testFileInfo) Mode() os.FileMode  { return 0600 }
func (f testFileInfo) ModTime() time.Time { return f.modTime }
func (f testFileInfo) IsDir() bool        { return false }
func (f testFileInfo) Sys() any           { return f.sys }

type testBirthtimeStat struct {
	Birthtimespec testTimespec
}

type testBirthtimStat struct {
	Birthtim testTimespec
}

type testTimespec struct {
	Sec  int64
	Nsec int64
}

type testWindowsCreationStat struct {
	CreationTime testCreationTimeValue
}

type testCreationTimeValue struct {
	nanos int64
}

func (v testCreationTimeValue) Nanoseconds() int64 {
	return v.nanos
}
