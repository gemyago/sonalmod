package workspacefs

import (
	"path/filepath"
	"testing"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewService(t *testing.T) {
	t.Parallel()
	fake := faker.New()

	t.Run("single_workspace_ListWorkspaces_returns_identifier_and_description", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		wantPath := filepath.Clean(abs)
		id := fake.Lorem().Word()
		desc := fake.Lorem().Sentence(6)

		svc, err := NewService([]WorkspaceMount{{Identifier: id, Description: desc, Path: wantPath}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		require.Equal(t, []WorkspaceDescriptor{{Identifier: id, Description: desc}}, svc.ListWorkspaces())
	})

	t.Run("multiple_workspaces_ListWorkspaces_order_preserved", func(t *testing.T) {
		t.Parallel()
		a := t.TempDir()
		b := t.TempDir()
		absA, err := filepath.Abs(a)
		require.NoError(t, err)
		absB, err := filepath.Abs(b)
		require.NoError(t, err)
		cleanA := filepath.Clean(absA)
		cleanB := filepath.Clean(absB)
		idA := fake.Lorem().Word() + "-a"
		idB := fake.Lorem().Word() + "-b"
		descA := fake.Lorem().Sentence(6)
		descB := fake.Lorem().Sentence(6)

		svc, err := NewService([]WorkspaceMount{
			{Identifier: idA, Description: descA, Path: cleanA},
			{Identifier: idB, Description: descB, Path: cleanB},
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		require.Equal(t, []WorkspaceDescriptor{
			{Identifier: idA, Description: descA},
			{Identifier: idB, Description: descB},
		}, svc.ListWorkspaces())
	})

	t.Run("invalid_path_returns_error", func(t *testing.T) {
		t.Parallel()
		missing := filepath.Join(t.TempDir(), "nonexistent-subdir")
		clean := filepath.Clean(missing)
		_, err := NewService([]WorkspaceMount{{
			Identifier:  fake.Lorem().Word(),
			Description: fake.Lorem().Sentence(6),
			Path:        missing,
		}})
		require.Error(t, err)
		require.ErrorContains(t, err, "cannot open workspace")
		assert.NotContains(t, err.Error(), clean, "error must not include configured absolute path")
	})

	t.Run("second_workspace_invalid_closes_first_root", func(t *testing.T) {
		t.Parallel()
		first := t.TempDir()
		absFirst, err := filepath.Abs(first)
		require.NoError(t, err)
		cleanFirst := filepath.Clean(absFirst)
		missing := filepath.Join(t.TempDir(), "missing-second-root")
		cleanMissing := filepath.Clean(missing)

		_, err = NewService([]WorkspaceMount{
			{Identifier: fake.Lorem().Word() + "-1", Description: fake.Lorem().Sentence(6), Path: cleanFirst},
			{Identifier: fake.Lorem().Word() + "-2", Description: fake.Lorem().Sentence(6), Path: missing},
		})
		require.Error(t, err)
		assert.NotContains(t, err.Error(), cleanFirst)
		assert.NotContains(t, err.Error(), cleanMissing)
	})

	t.Run("empty_mounts_returns_error", func(t *testing.T) {
		t.Parallel()
		_, err := NewService(nil)
		require.Error(t, err)
		_, err = NewService([]WorkspaceMount{})
		require.Error(t, err)
	})
}

func TestService(t *testing.T) {
	t.Parallel()
	fake := faker.New()

	t.Run("ListWorkspaces_returns_identifiers_and_descriptions_in_config_order", func(t *testing.T) {
		t.Parallel()
		a := t.TempDir()
		b := t.TempDir()
		absA, err := filepath.Abs(a)
		require.NoError(t, err)
		absB, err := filepath.Abs(b)
		require.NoError(t, err)
		idA := fake.Lorem().Word() + "-alpha"
		idB := fake.Lorem().Word() + "-beta"
		descA := fake.Lorem().Sentence(8)
		descB := fake.Lorem().Sentence(8)

		svc, err := NewService([]WorkspaceMount{
			{Identifier: idA, Description: descA, Path: filepath.Clean(absA)},
			{Identifier: idB, Description: descB, Path: filepath.Clean(absB)},
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		got := svc.ListWorkspaces()
		require.Equal(t, []WorkspaceDescriptor{
			{Identifier: idA, Description: descA},
			{Identifier: idB, Description: descB},
		}, got)
	})

	t.Run("unknown_workspace_returns_not_configured", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		wid := fake.Lorem().Word()
		svc, err := NewService([]WorkspaceMount{{
			Identifier:  wid,
			Description: fake.Lorem().Sentence(6),
			Path:        clean,
		}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.ReadTextFile(t.Context(), ReadTextFileRequest{Path: "x", Workspace: wid + "-nope"})
		require.Error(t, err)
		require.ErrorContains(t, err, `not configured`)
		assert.NotContains(t, err.Error(), clean, "unknown workspace error must not leak host root path")
	})

	t.Run("workspace_required_even_with_single_configured_workspace", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		wid := fake.Lorem().Word()
		svc, err := NewService([]WorkspaceMount{{
			Identifier:  wid,
			Description: fake.Lorem().Sentence(6),
			Path:        filepath.Clean(abs),
		}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.ReadTextFile(t.Context(), ReadTextFileRequest{Path: fake.UUID().V4() + ".txt"})
		require.Error(t, err)
		require.ErrorContains(t, err, "workspace is required")
	})
}

func TestListWorkspaces_nil_service(t *testing.T) {
	t.Parallel()
	var svc *Service
	require.Nil(t, svc.ListWorkspaces())
}

func TestService_Close_nil_receiver(t *testing.T) {
	t.Parallel()
	var svc *Service
	require.NoError(t, svc.Close())
}

func TestService_Close_releases_roots(t *testing.T) {
	t.Parallel()
	fake := faker.New()
	root := t.TempDir()
	abs, err := filepath.Abs(root)
	require.NoError(t, err)
	svc, err := NewService([]WorkspaceMount{{
		Identifier:  fake.Lorem().Word(),
		Description: fake.Lorem().Sentence(6),
		Path:        filepath.Clean(abs),
	}})
	require.NoError(t, err)
	require.NoError(t, svc.Close())
	require.NoError(t, svc.Close())
}
