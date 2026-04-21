package workspacefs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"
)

// GetFileInfoRequest is the JSON input for workspacefs_get_file_info.
type GetFileInfoRequest struct {
	// Path is relative to the workspace (no leading slash, no ".." escape).
	Path string `json:"path"`
	// Workspace is the configured workspace identifier (required on every call).
	Workspace string `json:"workspace"`
}

// GetFileInfoResponse is the JSON result for workspacefs_get_file_info.
type GetFileInfoResponse struct {
	Size        int64  `json:"size"`
	ModTime     string `json:"mod_time"` // RFC3339Nano, UTC
	Name        string `json:"name"`
	IsDirectory bool   `json:"is_directory"`
}

// GetFileInfo returns metadata for a path under the selected workspace.
func (s *Service) GetFileInfo(ctx context.Context, in GetFileInfoRequest) (GetFileInfoResponse, error) {
	if err := ctx.Err(); err != nil {
		return GetFileInfoResponse{}, err
	}
	if strings.TrimSpace(in.Path) == "" {
		return GetFileInfoResponse{}, errors.New("workspacefs: path is required")
	}

	root, err := s.pickWorkspace(in.Workspace)
	if err != nil {
		return GetFileInfoResponse{}, err
	}
	rel, err := sanitizeRelativePath(in.Path)
	if err != nil {
		return GetFileInfoResponse{}, fmt.Errorf("workspacefs: %w", err)
	}

	fi, err := root.Stat(rel)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return GetFileInfoResponse{}, fmt.Errorf("workspacefs: %w", err)
		}
		return GetFileInfoResponse{}, fmt.Errorf("workspacefs: stat %q: %w", rel, err)
	}

	return GetFileInfoResponse{
		Size:        fi.Size(),
		ModTime:     fi.ModTime().UTC().Format(time.RFC3339Nano),
		Name:        fi.Name(),
		IsDirectory: fi.IsDir(),
	}, nil
}
