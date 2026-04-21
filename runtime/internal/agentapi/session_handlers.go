package agentapi

import (
	"encoding/json"
	"net/http"

	"github.com/gemyago/sonalmod/runtime/agent"
	"github.com/gemyago/sonalmod/runtime/internal/callerid"
)

// ListSessions implements [ServerInterface].
func (s *AgentAPIServer) ListSessions(w http.ResponseWriter, r *http.Request, params ListSessionsParams) {
	ctx := r.Context()

	id := callerid.FromContext(ctx)
	if id == nil {
		writeProblemDetails(w, http.StatusUnauthorized, "Unauthorized", "authentication required")
		return
	}

	offset := 0
	if params.Offset != nil {
		offset = *params.Offset
	}

	result, err := s.runner.ListSessions(ctx, agent.ListSessionsParams{
		AppName: listSessionsAppName,
		UserID:  id.UserID(),
		Limit:   params.Limit,
		Offset:  offset,
	})
	if err != nil {
		s.logger.ErrorContext(ctx, "ListSessions: runner", "err", err)
		writeProblemDetails(w, http.StatusInternalServerError, "Internal Server Error", "failed to list sessions")
		return
	}

	apiSessions := make([]SessionMetadata, len(result.Sessions))
	for i := range result.Sessions {
		apiSessions[i] = mapListedSessionMetadata(result.Sessions[i])
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(SessionListResponse{
		Sessions: apiSessions,
		Total:    result.Total,
	})
}
