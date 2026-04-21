package skills

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/gemyago/sonalmod/runtime/agent"
	iskills "github.com/gemyago/sonalmod/tools/skills/internal/skills"
)

func skillsAgentTools(cat *iskills.Catalog, _ *slog.Logger) []agent.DefinedTool {
	return []agent.DefinedTool{
		skillsListTool(cat),
		skillsReadTool(cat),
	}
}

type skillsListRequest struct{}

type skillsListResponse struct {
	Skills []skillMetadata `json:"skills"`
}

type skillMetadata struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func skillsListTool(cat *iskills.Catalog) agent.ToolDef[skillsListRequest, skillsListResponse] {
	return agent.NewToolDef[skillsListRequest, skillsListResponse](
		"skills_list",
		"List available skills by name and short description. Use skill names as input to skills_read to load full instructions.",
		func(_ *agent.ToolContext, _ skillsListRequest) (skillsListResponse, error) {
			entries := cat.List()
			skills := make([]skillMetadata, 0, len(entries))
			for _, e := range entries {
				skills = append(skills, skillMetadata{
					Name:        e.Name,
					Description: e.Description,
				})
			}
			return skillsListResponse{Skills: skills}, nil
		},
	)
}

type skillsReadRequest struct {
	Name string `json:"name"`
}

type skillsReadResponse struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Body        string `json:"body"`
}

func skillsReadTool(cat *iskills.Catalog) agent.ToolDef[skillsReadRequest, skillsReadResponse] {
	return agent.NewToolDef[skillsReadRequest, skillsReadResponse](
		"skills_read",
		"Load the full instructions for a skill by name. Returns skill metadata and the complete SKILL.md body. Use skills_list to discover available skill names.",
		func(_ *agent.ToolContext, req skillsReadRequest) (skillsReadResponse, error) {
			if req.Name == "" {
				return skillsReadResponse{}, errors.New("skills_read: name is required")
			}
			entry, ok := cat.Get(req.Name)
			if !ok {
				return skillsReadResponse{}, fmt.Errorf("skills_read: skill %q not found", req.Name)
			}
			return skillsReadResponse{
				Name:        entry.Name,
				Description: entry.Description,
				Body:        entry.Body,
			}, nil
		},
	)
}
