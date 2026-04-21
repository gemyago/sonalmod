package workspacefs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"unicode/utf8"
)

// EditFileRequest is the JSON input for workspacefs_edit_file.
type EditFileRequest struct {
	// Path is relative to the workspace (no leading slash, no ".." escape).
	Path string `json:"path"`
	// Workspace is the configured workspace identifier (required on every call).
	Workspace string `json:"workspace"`
	// OldText is the substring to replace; must be non-empty and must occur at least once. Only the first occurrence is replaced.
	OldText string `json:"old_text"`
	// NewText replaces OldText at the first match (may be empty to delete that occurrence).
	NewText string `json:"new_text"`
}

// EditFileResponse is the JSON result for workspacefs_edit_file.
type EditFileResponse struct {
	// MatchCount is the number of non-overlapping occurrences of OldText in the file before the edit.
	MatchCount int `json:"match_count"`
	// BytesWritten is the size of the written file in bytes.
	BytesWritten int `json:"bytes_written,omitempty"`
}

// EditFile replaces the first occurrence of OldText with NewText in a UTF-8 text file under the selected workspace.
// When multiple occurrences exist, only the first is replaced; MatchCount reports how many non-overlapping matches were found.
func (s *Service) EditFile(ctx context.Context, in EditFileRequest) (EditFileResponse, error) {
	if err := ctx.Err(); err != nil {
		return EditFileResponse{}, err
	}
	if err := validateEditFileRequest(in); err != nil {
		return EditFileResponse{}, err
	}

	root, err := s.pickWorkspace(in.Workspace)
	if err != nil {
		return EditFileResponse{}, err
	}
	rel, err := sanitizeRelativePath(in.Path)
	if err != nil {
		return EditFileResponse{}, fmt.Errorf("workspacefs: %w", err)
	}

	content, err := s.readUTF8FileFromRoot(root, rel)
	if err != nil {
		return EditFileResponse{}, err
	}

	newContent, matchCount, err := computeFirstOccurrenceReplace(content, in.OldText, in.NewText)
	if err != nil {
		return EditFileResponse{}, err
	}

	wf, err := s.WriteFile(ctx, WriteFileRequest{Path: in.Path, Workspace: in.Workspace, Content: newContent})
	if err != nil {
		return EditFileResponse{}, err
	}
	return EditFileResponse{
		MatchCount:   matchCount,
		BytesWritten: wf.BytesWritten,
	}, nil
}

func validateEditFileRequest(in EditFileRequest) error {
	if strings.TrimSpace(in.Path) == "" {
		return errors.New("workspacefs: path is required")
	}
	if in.OldText == "" {
		return errors.New("workspacefs: old_text must not be empty")
	}
	if !utf8.ValidString(in.OldText) || !utf8.ValidString(in.NewText) {
		return errors.New("workspacefs: old_text and new_text must be valid UTF-8")
	}
	return nil
}

func (s *Service) readUTF8FileFromRoot(root *os.Root, rel string) (string, error) {
	f, err := root.Open(rel)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("workspacefs: %w", err)
		}
		return "", fmt.Errorf("workspacefs: open %q: %w", rel, err)
	}
	defer func() { _ = f.Close() }()

	data, err := readAllLimited(f, s.maxReadBytes)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(data) {
		return "", errors.New("workspacefs: file is not valid UTF-8 text")
	}
	return string(data), nil
}

func computeFirstOccurrenceReplace(content, oldText, newText string) (string, int, error) {
	before, after, found := strings.Cut(content, oldText)
	if !found {
		return "", 0, errors.New("workspacefs: old_text not found in file")
	}
	matchCount := strings.Count(content, oldText)
	return before + newText + after, matchCount, nil
}
