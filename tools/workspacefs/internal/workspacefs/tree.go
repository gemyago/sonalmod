package workspacefs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

// DirectoryTreeRequest is the JSON input for workspacefs_directory_tree.
type DirectoryTreeRequest struct {
	// Path is relative to the workspace; use "." or omit for the workspace base.
	Path string `json:"path"`
	// Workspace is the configured workspace identifier (required on every call).
	Workspace string `json:"workspace"`
	// MaxDepth is how many levels to descend below Path (minimum 1). Omitted uses DefaultMaxTreeDepth.
	MaxDepth *int `json:"max_depth,omitempty"`
}

// DirectoryTreeNode is one node in a directory_tree response.
type DirectoryTreeNode struct {
	Name        string               `json:"name"`
	IsDirectory bool                 `json:"is_directory"`
	Children    []*DirectoryTreeNode `json:"children,omitempty"`
}

// DirectoryTreeResponse is the JSON result for workspacefs_directory_tree.
type DirectoryTreeResponse struct {
	Root *DirectoryTreeNode `json:"root"`
}

// DirectoryTree returns a recursive directory tree under the selected workspace.
func (s *Service) DirectoryTree(ctx context.Context, in DirectoryTreeRequest) (DirectoryTreeResponse, error) {
	if err := ctx.Err(); err != nil {
		return DirectoryTreeResponse{}, err
	}

	maxDepth := DefaultMaxTreeDepth
	if in.MaxDepth != nil {
		if *in.MaxDepth < 1 {
			return DirectoryTreeResponse{}, errors.New("workspacefs: max_depth must be at least 1")
		}
		maxDepth = *in.MaxDepth
	}
	maxDepth = min(maxDepth, maxTreeDepthHardCap)

	root, err := s.pickWorkspace(in.Workspace)
	if err != nil {
		return DirectoryTreeResponse{}, err
	}
	rel, err := sanitizeRelativePath(in.Path)
	if err != nil {
		return DirectoryTreeResponse{}, fmt.Errorf("workspacefs: %w", err)
	}

	fsys := root.FS()
	fi, err := fs.Stat(fsys, rel)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return DirectoryTreeResponse{}, fmt.Errorf("workspacefs: %w", err)
		}
		return DirectoryTreeResponse{}, fmt.Errorf("workspacefs: stat %q: %w", rel, err)
	}
	if !fi.IsDir() {
		return DirectoryTreeResponse{}, errors.New("workspacefs: path is not a directory")
	}

	var used int
	node, err := s.buildDirectoryTree(fsys, rel, maxDepth, &used)
	if err != nil {
		return DirectoryTreeResponse{}, err
	}
	return DirectoryTreeResponse{Root: node}, nil
}

func (s *Service) bumpTreeUsed(used *int) error {
	*used++
	if *used > s.maxListEntries {
		return fmt.Errorf(
			"workspacefs: directory tree exceeds maximum entries (%d)",
			s.maxListEntries,
		)
	}
	return nil
}

func (s *Service) buildDirectoryTree(
	fsys fs.FS,
	rel string,
	remainingDepth int,
	used *int,
) (*DirectoryTreeNode, error) {
	fi, statErr := fs.Stat(fsys, rel)
	if statErr != nil {
		return nil, fmt.Errorf("workspacefs: stat %q: %w", rel, statErr)
	}
	name := path.Base(rel)
	if rel == "." {
		name = "."
	}
	if !fi.IsDir() {
		if err := s.bumpTreeUsed(used); err != nil {
			return nil, err
		}
		return &DirectoryTreeNode{Name: name, IsDirectory: false}, nil
	}

	if err := s.bumpTreeUsed(used); err != nil {
		return nil, err
	}

	node := &DirectoryTreeNode{Name: name, IsDirectory: true}

	entries, readErr := fs.ReadDir(fsys, rel)
	if readErr != nil {
		return nil, fmt.Errorf("workspacefs: read directory %q: %w", rel, readErr)
	}
	if len(entries) > s.maxListEntries {
		return nil, fmt.Errorf(
			"workspacefs: directory tree exceeds maximum entries (%d)",
			s.maxListEntries,
		)
	}

	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})

	for _, e := range entries {
		sub := path.Join(rel, e.Name())
		if err := s.appendDirectoryTreeChild(fsys, sub, e, remainingDepth, used, node); err != nil {
			return nil, err
		}
	}

	return node, nil
}

func (s *Service) appendDirectoryTreeChild(
	fsys fs.FS,
	sub string,
	e fs.DirEntry,
	remainingDepth int,
	used *int,
	node *DirectoryTreeNode,
) error {
	if e.IsDir() {
		if remainingDepth <= 1 {
			return s.appendShallowDirChild(e, used, node)
		}
		child, cErr := s.buildDirectoryTree(fsys, sub, remainingDepth-1, used)
		if cErr != nil {
			return cErr
		}
		node.Children = append(node.Children, child)
		return nil
	}
	if err := s.bumpTreeUsed(used); err != nil {
		return err
	}
	node.Children = append(node.Children, &DirectoryTreeNode{Name: e.Name(), IsDirectory: false})
	return nil
}

func (s *Service) appendShallowDirChild(e fs.DirEntry, used *int, node *DirectoryTreeNode) error {
	if err := s.bumpTreeUsed(used); err != nil {
		return err
	}
	node.Children = append(node.Children, &DirectoryTreeNode{Name: e.Name(), IsDirectory: true})
	return nil
}
