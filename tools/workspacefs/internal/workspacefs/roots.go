package workspacefs

// WorkspaceDescriptor is model-visible workspace metadata (no filesystem paths).
type WorkspaceDescriptor struct {
	Identifier  string `json:"identifier"`
	Description string `json:"description"`
}

// ListWorkspacesResponse is the JSON result for workspacefs_list_workspaces.
type ListWorkspacesResponse struct {
	Workspaces []WorkspaceDescriptor `json:"workspaces"`
}

// ListWorkspaces returns configured workspaces with identifier and description in configuration order.
func (s *Service) ListWorkspaces() []WorkspaceDescriptor {
	if s == nil {
		return nil
	}
	out := make([]WorkspaceDescriptor, len(s.entries))
	for i := range s.entries {
		out[i] = WorkspaceDescriptor{
			Identifier:  s.entries[i].identifier,
			Description: s.entries[i].description,
		}
	}
	return out
}
