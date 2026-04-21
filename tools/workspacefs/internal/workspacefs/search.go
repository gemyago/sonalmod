package workspacefs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// SearchFilesRequest is the JSON input for workspacefs_search_files.
type SearchFilesRequest struct {
	// Path is the directory to search relative to the workspace; use "." or omit for the workspace base.
	Path string `json:"path"`
	// Glob is a path.Match-style pattern relative to Path (e.g. "*.txt", "foo/*.go"). Not path.Match "**" recursion.
	Glob string `json:"glob"`
	// Workspace is the configured workspace identifier (required on every call).
	Workspace string `json:"workspace"`
}

// SearchFilesResponse is the JSON result for workspacefs_search_files.
type SearchFilesResponse struct {
	// Paths are relative to the workspace, sorted case-insensitively by name.
	Paths []string `json:"paths"`
}

// SearchFiles walks under Path (relative to the workspace) and returns file paths matching Glob
// using [path.Match] semantics (forward slashes). Only regular files are included.
func (s *Service) SearchFiles(ctx context.Context, in SearchFilesRequest) (SearchFilesResponse, error) {
	if err := ctx.Err(); err != nil {
		return SearchFilesResponse{}, err
	}

	root, err := s.pickWorkspace(in.Workspace)
	if err != nil {
		return SearchFilesResponse{}, err
	}
	basePath, err := sanitizeRelativePath(in.Path)
	if err != nil {
		return SearchFilesResponse{}, fmt.Errorf("workspacefs: %w", err)
	}
	globPat, err := sanitizeGlobPattern(in.Glob)
	if err != nil {
		return SearchFilesResponse{}, fmt.Errorf("workspacefs: %w", err)
	}

	fsys := root.FS()
	if vErr := validateSearchDirectory(fsys, basePath); vErr != nil {
		return SearchFilesResponse{}, vErr
	}

	matches, err := s.globWalkFiles(ctx, fsys, basePath, globPat)
	if err != nil {
		return SearchFilesResponse{}, err
	}
	sort.Slice(matches, func(i, j int) bool {
		return strings.ToLower(matches[i]) < strings.ToLower(matches[j])
	})
	return SearchFilesResponse{Paths: matches}, nil
}

func validateSearchDirectory(fsys fs.FS, basePath string) error {
	fi, err := fs.Stat(fsys, basePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("workspacefs: %w", err)
		}
		return fmt.Errorf("workspacefs: stat %q: %w", basePath, err)
	}
	if !fi.IsDir() {
		return errors.New("workspacefs: path is not a directory")
	}
	return nil
}

func (s *Service) globWalkFiles(ctx context.Context, fsys fs.FS, basePath, globPat string) ([]string, error) {
	var matches []string
	var n int
	walkErr := fs.WalkDir(fsys, basePath, func(p string, d fs.DirEntry, entryErr error) error {
		if entryErr != nil {
			return entryErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, ok := relPathUnderBase(basePath, p)
		if !ok {
			return nil
		}
		matched, matchErr := path.Match(globPat, rel)
		if matchErr != nil {
			return fmt.Errorf("workspacefs: match %q: %w", globPat, matchErr)
		}
		if !matched {
			return nil
		}
		n++
		if n > s.maxListEntries {
			return fmt.Errorf(
				"workspacefs: search exceeds maximum entries (%d)",
				s.maxListEntries,
			)
		}
		matches = append(matches, p)
		return nil
	})
	return matches, walkErr
}

func sanitizeGlobPattern(g string) (string, error) {
	g = strings.TrimSpace(g)
	if g == "" {
		return "", errors.New("glob is required")
	}
	g = path.Clean(filepath.ToSlash(g))
	if strings.HasPrefix(g, "/") {
		return "", errors.New("glob must be relative")
	}
	if g == ".." || strings.HasPrefix(g, "../") {
		return "", errors.New("glob escapes workspace")
	}
	return g, nil
}

func relPathUnderBase(base, full string) (string, bool) {
	base = path.Clean(filepath.ToSlash(base))
	full = path.Clean(filepath.ToSlash(full))
	if base == "." {
		return full, true
	}
	if full == base {
		return ".", true
	}
	prefix := base + "/"
	rest, ok := strings.CutPrefix(full, prefix)
	return rest, ok
}
