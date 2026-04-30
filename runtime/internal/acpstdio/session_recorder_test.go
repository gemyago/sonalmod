package acpstdio

import (
	"context"
	"errors"
	"testing"

	rt "github.com/gemyago/sonalmod/runtime/internal"
	"github.com/gemyago/sonalmod/runtime/internal/sessions"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/adk/session"
)

type recorderSessionServiceStub struct {
	createFn func(ctx context.Context, req *session.CreateRequest) (*session.CreateResponse, error)
	getFn    func(ctx context.Context, req *session.GetRequest) (*session.GetResponse, error)
	appendFn func(ctx context.Context, sess session.Session, event *session.Event) error
}

func (s recorderSessionServiceStub) Create(
	ctx context.Context,
	req *session.CreateRequest,
) (*session.CreateResponse, error) {
	return s.createFn(ctx, req)
}

func (s recorderSessionServiceStub) Get(
	ctx context.Context,
	req *session.GetRequest,
) (*session.GetResponse, error) {
	return s.getFn(ctx, req)
}

func (s recorderSessionServiceStub) AppendEvent(
	ctx context.Context,
	sess session.Session,
	event *session.Event,
) error {
	return s.appendFn(ctx, sess, event)
}

func TestNewSessionRecorder(t *testing.T) {
	t.Parallel()

	fake := faker.New()

	t.Run("rejects blank app name", func(t *testing.T) {
		t.Parallel()

		recorder, err := NewSessionRecorder("   ", sessions.NewMemorySessionsStorage())

		require.Error(t, err)
		assert.Nil(t, recorder)
		require.ErrorContains(t, err, "app name is required")
	})

	t.Run("rejects nil storage", func(t *testing.T) {
		t.Parallel()

		recorder, err := NewSessionRecorder(fake.Lorem().Word(), nil)

		require.Error(t, err)
		assert.Nil(t, recorder)
		require.ErrorContains(t, err, "session storage is required")
	})
}

func TestADKSessionRecorderRecord(t *testing.T) {
	t.Parallel()

	fake := faker.New()

	makeRequest := func() rt.ACPRunRequest {
		return rt.ACPRunRequest{
			UserID:    fake.Internet().User(),
			SessionID: fake.UUID().V4(),
			Message: &rt.MessageContent{
				Parts: []rt.MessagePart{{Text: fake.Lorem().Sentence(4)}},
			},
		}
	}

	makeEvent := func() *rt.SessionEvent {
		return &rt.SessionEvent{
			TurnComplete: true,
			Content: &rt.SessionEventContent{
				Role:  "model",
				Parts: []rt.SessionEventPart{{Text: fake.Lorem().Sentence(3)}},
			},
		}
	}

	t.Run("validates required request fields", func(t *testing.T) {
		t.Parallel()

		recorder, err := NewSessionRecorder(fake.Lorem().Word(), sessions.NewMemorySessionsStorage())
		require.NoError(t, err)

		err = recorder.Record(t.Context(), rt.ACPRunRequest{SessionID: fake.UUID().V4()}, nil)
		require.Error(t, err)
		require.ErrorContains(t, err, "userID is required")

		err = recorder.Record(t.Context(), rt.ACPRunRequest{UserID: fake.Internet().User()}, nil)
		require.Error(t, err)
		require.ErrorContains(t, err, "sessionID is required")
	})

	t.Run("returns error when existing session lookup is malformed", func(t *testing.T) {
		t.Parallel()

		recorder, err := NewSessionRecorder(fake.Lorem().Word(), recorderSessionServiceStub{
			createFn: func(context.Context, *session.CreateRequest) (*session.CreateResponse, error) {
				panic("Create should not be called")
			},
			getFn: func(context.Context, *session.GetRequest) (*session.GetResponse, error) {
				return &session.GetResponse{}, nil
			},
			appendFn: func(context.Context, session.Session, *session.Event) error {
				panic("AppendEvent should not be called")
			},
		})
		require.NoError(t, err)

		err = recorder.Record(t.Context(), makeRequest(), []*rt.SessionEvent{makeEvent()})
		require.Error(t, err)
		require.ErrorContains(t, err, "missing session")
	})

	t.Run("returns error when lookup fails for reasons other than not found", func(t *testing.T) {
		t.Parallel()

		recorder, err := NewSessionRecorder(fake.Lorem().Word(), recorderSessionServiceStub{
			createFn: func(context.Context, *session.CreateRequest) (*session.CreateResponse, error) {
				panic("Create should not be called")
			},
			getFn: func(context.Context, *session.GetRequest) (*session.GetResponse, error) {
				return nil, errors.New(fake.Lorem().Sentence(3))
			},
			appendFn: func(context.Context, session.Session, *session.Event) error {
				panic("AppendEvent should not be called")
			},
		})
		require.NoError(t, err)

		err = recorder.Record(t.Context(), makeRequest(), []*rt.SessionEvent{makeEvent()})
		require.Error(t, err)
		require.ErrorContains(t, err, "get session")
	})

	t.Run("returns error when create fails", func(t *testing.T) {
		t.Parallel()

		recorder, err := NewSessionRecorder(fake.Lorem().Word(), recorderSessionServiceStub{
			createFn: func(context.Context, *session.CreateRequest) (*session.CreateResponse, error) {
				return nil, errors.New(fake.Lorem().Sentence(3))
			},
			getFn: func(context.Context, *session.GetRequest) (*session.GetResponse, error) {
				return nil, errors.New("session not found")
			},
			appendFn: func(context.Context, session.Session, *session.Event) error {
				panic("AppendEvent should not be called")
			},
		})
		require.NoError(t, err)

		err = recorder.Record(t.Context(), makeRequest(), []*rt.SessionEvent{makeEvent()})
		require.Error(t, err)
		require.ErrorContains(t, err, "create session")
	})

	t.Run("returns error when create response has no session", func(t *testing.T) {
		t.Parallel()

		recorder, err := NewSessionRecorder(fake.Lorem().Word(), recorderSessionServiceStub{
			createFn: func(context.Context, *session.CreateRequest) (*session.CreateResponse, error) {
				return &session.CreateResponse{}, nil
			},
			getFn: func(context.Context, *session.GetRequest) (*session.GetResponse, error) {
				return nil, errors.New("session not found")
			},
			appendFn: func(context.Context, session.Session, *session.Event) error {
				panic("AppendEvent should not be called")
			},
		})
		require.NoError(t, err)

		err = recorder.Record(t.Context(), makeRequest(), []*rt.SessionEvent{makeEvent()})
		require.Error(t, err)
		require.ErrorContains(t, err, "missing session")
	})

	t.Run("returns error when appending user event fails", func(t *testing.T) {
		t.Parallel()

		mem := sessions.NewMemorySessionsStorage()
		request := makeRequest()
		appName := fake.Lorem().Word()
		createResp, err := mem.Create(t.Context(), &session.CreateRequest{
			AppName:   appName,
			UserID:    request.UserID,
			SessionID: request.SessionID,
			State:     make(map[string]any),
		})
		require.NoError(t, err)

		recorder, err := NewSessionRecorder(appName, recorderSessionServiceStub{
			createFn: func(context.Context, *session.CreateRequest) (*session.CreateResponse, error) {
				panic("Create should not be called")
			},
			getFn: func(context.Context, *session.GetRequest) (*session.GetResponse, error) {
				return &session.GetResponse{Session: createResp.Session}, nil
			},
			appendFn: func(context.Context, session.Session, *session.Event) error {
				return errors.New("boom")
			},
		})
		require.NoError(t, err)

		err = recorder.Record(t.Context(), request, []*rt.SessionEvent{makeEvent()})
		require.Error(t, err)
		require.ErrorContains(t, err, "append user event")
	})

	t.Run("returns error when appending agent event fails", func(t *testing.T) {
		t.Parallel()

		mem := sessions.NewMemorySessionsStorage()
		request := makeRequest()
		appName := fake.Lorem().Word()
		createResp, err := mem.Create(t.Context(), &session.CreateRequest{
			AppName:   appName,
			UserID:    request.UserID,
			SessionID: request.SessionID,
			State:     make(map[string]any),
		})
		require.NoError(t, err)

		appendCount := 0
		recorder, err := NewSessionRecorder(appName, recorderSessionServiceStub{
			createFn: func(context.Context, *session.CreateRequest) (*session.CreateResponse, error) {
				panic("Create should not be called")
			},
			getFn: func(context.Context, *session.GetRequest) (*session.GetResponse, error) {
				return &session.GetResponse{Session: createResp.Session}, nil
			},
			appendFn: func(context.Context, session.Session, *session.Event) error {
				appendCount++
				if appendCount == 2 {
					return errors.New("boom")
				}
				return nil
			},
		})
		require.NoError(t, err)

		err = recorder.Record(t.Context(), request, []*rt.SessionEvent{makeEvent()})
		require.Error(t, err)
		require.ErrorContains(t, err, "append agent event")
	})

	t.Run("records existing sessions and maps tool payloads", func(t *testing.T) {
		t.Parallel()

		request := makeRequest()
		appName := fake.Lorem().Word()
		storage := sessions.NewMemorySessionsStorage()
		createResp, err := storage.Create(t.Context(), &session.CreateRequest{
			AppName:   appName,
			UserID:    request.UserID,
			SessionID: request.SessionID,
			State:     make(map[string]any),
		})
		require.NoError(t, err)
		require.NotNil(t, createResp.Session)

		recorder, err := NewSessionRecorder(appName, storage)
		require.NoError(t, err)

		callID := fake.UUID().V4()
		callName := fake.Lorem().Word()
		responseName := fake.Lorem().Word()
		err = recorder.Record(t.Context(), request, []*rt.SessionEvent{
			nil,
			{
				Content: &rt.SessionEventContent{
					Role: "model",
					Parts: []rt.SessionEventPart{
						{Text: fake.Lorem().Sentence(2)},
						{FunctionCall: &rt.SessionEventFunctionCall{
							ID:   callID,
							Name: callName,
							Args: map[string]any{"k": fake.Lorem().Word()},
						}},
						{FunctionResponse: &rt.SessionEventFunctionResponse{
							ID:       callID,
							Name:     responseName,
							Response: map[string]any{"ok": true},
						}},
					},
				},
				TurnComplete: true,
				Author:       "model",
				InvocationID: fake.UUID().V4(),
			},
		})
		require.NoError(t, err)

		stored, err := storage.Get(t.Context(), &session.GetRequest{
			AppName:   appName,
			UserID:    request.UserID,
			SessionID: request.SessionID,
		})
		require.NoError(t, err)
		require.Equal(t, 2, stored.Session.Events().Len())

		agentEvent := rt.MapADKSessionEvent(stored.Session.Events().At(1))
		require.NotNil(t, agentEvent)
		require.NotNil(t, agentEvent.Content)
		require.Len(t, agentEvent.Content.Parts, 3)
		require.NotNil(t, agentEvent.Content.Parts[1].FunctionCall)
		assert.Equal(t, callName, agentEvent.Content.Parts[1].FunctionCall.Name)
		require.NotNil(t, agentEvent.Content.Parts[2].FunctionResponse)
		assert.Equal(t, responseName, agentEvent.Content.Parts[2].FunctionResponse.Name)
	})
}

func TestSessionRecorderHelpers(t *testing.T) {
	t.Parallel()

	fake := faker.New()

	t.Run("newUserSessionEvent handles nil and empty messages", func(t *testing.T) {
		t.Parallel()

		assert.Nil(t, newUserSessionEvent(nil))
		assert.Nil(t, newUserSessionEvent(&rt.MessageContent{}))
	})

	t.Run("newADKSessionEvent returns nil for nil input", func(t *testing.T) {
		t.Parallel()

		assert.Nil(t, newADKSessionEvent(nil))
	})

	t.Run("newGenAIContent returns nil for nil and empty content", func(t *testing.T) {
		t.Parallel()

		assert.Nil(t, newGenAIContent(nil))
		assert.Nil(t, newGenAIContent(&rt.SessionEventContent{}))
	})

	t.Run("newGenAIContent maps text function call and function response parts", func(t *testing.T) {
		t.Parallel()

		callID := fake.UUID().V4()
		got := newGenAIContent(&rt.SessionEventContent{
			Role: "model",
			Parts: []rt.SessionEventPart{
				{Text: fake.Lorem().Sentence(2)},
				{FunctionCall: &rt.SessionEventFunctionCall{
					ID:   callID,
					Name: fake.Lorem().Word(),
					Args: map[string]any{"arg": fake.Lorem().Word()},
				}},
				{FunctionResponse: &rt.SessionEventFunctionResponse{
					ID:       callID,
					Name:     fake.Lorem().Word(),
					Response: map[string]any{"ok": true},
				}},
			},
		})

		require.NotNil(t, got)
		require.Len(t, got.Parts, 3)
		assert.NotNil(t, got.Parts[1].FunctionCall)
		assert.NotNil(t, got.Parts[2].FunctionResponse)
	})
}
