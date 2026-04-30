package acpstdio

import (
	"context"
	"errors"
	"fmt"
	"strings"

	rt "github.com/gemyago/sonalmod/runtime/internal"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// SessionRecorder persists ACP profile run history through the standard session storage path.
type SessionRecorder interface {
	Record(ctx context.Context, request rt.ACPRunRequest, events []*rt.SessionEvent) error
}

type sessionService interface {
	Create(ctx context.Context, req *session.CreateRequest) (*session.CreateResponse, error)
	Get(ctx context.Context, req *session.GetRequest) (*session.GetResponse, error)
	AppendEvent(ctx context.Context, sess session.Session, event *session.Event) error
}

type adkSessionRecorder struct {
	appName string
	storage sessionService
}

// NewSessionRecorder creates a recorder backed by the runtime session service.
//
//nolint:ireturn // constructor intentionally returns the recorder interface.
func NewSessionRecorder(appName string, storage sessionService) (SessionRecorder, error) {
	if strings.TrimSpace(appName) == "" {
		return nil, errors.New("app name is required")
	}
	if storage == nil {
		return nil, errors.New("session storage is required")
	}

	return &adkSessionRecorder{
		appName: appName,
		storage: storage,
	}, nil
}

func (r *adkSessionRecorder) Record(
	ctx context.Context,
	request rt.ACPRunRequest,
	events []*rt.SessionEvent,
) error {
	if strings.TrimSpace(request.UserID) == "" {
		return errors.New("userID is required")
	}
	if strings.TrimSpace(request.SessionID) == "" {
		return errors.New("sessionID is required")
	}

	resp, err := r.storage.Get(ctx, &session.GetRequest{
		AppName:   r.appName,
		UserID:    request.UserID,
		SessionID: request.SessionID,
	})
	var sess session.Session
	switch {
	case err == nil:
		if resp == nil || resp.Session == nil {
			return fmt.Errorf("get session %s: missing session", request.SessionID)
		}
		sess = resp.Session
	case !strings.Contains(err.Error(), "not found"):
		return fmt.Errorf("get session %s: %w", request.SessionID, err)
	default:
		createResp, createErr := r.storage.Create(ctx, &session.CreateRequest{
			AppName:   r.appName,
			UserID:    request.UserID,
			SessionID: request.SessionID,
			State:     make(map[string]any),
		})
		if createErr != nil {
			return fmt.Errorf("create session %s: %w", request.SessionID, createErr)
		}
		if createResp == nil || createResp.Session == nil {
			return fmt.Errorf("create session %s: missing session", request.SessionID)
		}
		sess = createResp.Session
	}

	if userEvent := newUserSessionEvent(request.Message); userEvent != nil {
		appendErr := r.storage.AppendEvent(ctx, sess, userEvent)
		if appendErr != nil {
			return fmt.Errorf("append user event: %w", appendErr)
		}
	}

	for _, event := range events {
		if event == nil {
			continue
		}

		appendErr := r.storage.AppendEvent(ctx, sess, newADKSessionEvent(event))
		if appendErr != nil {
			return fmt.Errorf("append agent event: %w", appendErr)
		}
	}

	return nil
}

func newUserSessionEvent(message *rt.MessageContent) *session.Event {
	if message == nil {
		return nil
	}

	parts := make([]*genai.Part, 0, len(message.Parts))
	for _, part := range message.Parts {
		parts = append(parts, &genai.Part{Text: part.Text})
	}
	if len(parts) == 0 {
		return nil
	}

	event := session.NewEvent("")
	event.Author = "user"
	event.TurnComplete = true
	event.Content = &genai.Content{
		Role:  "user",
		Parts: parts,
	}

	return event
}

func newADKSessionEvent(event *rt.SessionEvent) *session.Event {
	if event == nil {
		return nil
	}

	adkEvent := session.NewEvent(event.InvocationID)
	adkEvent.ErrorCode = event.ErrorCode
	adkEvent.ErrorMessage = event.ErrorMessage
	adkEvent.Partial = event.Partial
	adkEvent.TurnComplete = event.TurnComplete
	adkEvent.Interrupted = event.Interrupted
	adkEvent.Author = event.Author
	adkEvent.Branch = event.Branch
	adkEvent.InvocationID = event.InvocationID
	adkEvent.Content = newGenAIContent(event.Content)

	return adkEvent
}

func newGenAIContent(content *rt.SessionEventContent) *genai.Content {
	if content == nil {
		return nil
	}

	parts := make([]*genai.Part, 0, len(content.Parts))
	for _, part := range content.Parts {
		switch {
		case part.Text != "":
			parts = append(parts, &genai.Part{Text: part.Text})
		case part.FunctionCall != nil:
			parts = append(parts, &genai.Part{
				FunctionCall: &genai.FunctionCall{
					ID:   part.FunctionCall.ID,
					Name: part.FunctionCall.Name,
					Args: part.FunctionCall.Args,
				},
			})
		case part.FunctionResponse != nil:
			parts = append(parts, &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					ID:       part.FunctionResponse.ID,
					Name:     part.FunctionResponse.Name,
					Response: part.FunctionResponse.Response,
				},
			})
		}
	}

	if len(parts) == 0 && content.Role == "" {
		return nil
	}

	return &genai.Content{
		Role:  content.Role,
		Parts: parts,
	}
}
