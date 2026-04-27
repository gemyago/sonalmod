//go:build !release

package agentapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gemyago/sonalmod/runtime/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenCodeBindingHandlers(t *testing.T) {
	t.Parallel()

	newLogger := func() *slog.Logger {
		return slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	newServer := func(bindings agent.OpenCodeBindingService) *AgentAPIServer {
		return NewAgentAPIServer(ServerParams{
			Logger:                 newLogger(),
			OpenCodeBindingService: bindings,
		})
	}
	newBindingService := func(t *testing.T) agent.OpenCodeBindingService {
		t.Helper()
		svc, err := agent.NewFileOpenCodeBindingService(t.TempDir(), newLogger())
		require.NoError(t, err)
		return svc
	}

	t.Run("binding routes are not exposed", func(t *testing.T) {
		t.Parallel()

		h := HandlerFromMux(newServer(nil), http.NewServeMux())
		testCases := []struct {
			name   string
			method string
			path   string
		}{
			{name: "list", method: http.MethodGet, path: "/opencode-bindings"},
			{name: "create", method: http.MethodPost, path: "/opencode-bindings"},
			{name: "get", method: http.MethodGet, path: "/opencode-bindings/binding-main"},
			{name: "update", method: http.MethodPut, path: "/opencode-bindings/binding-main"},
			{name: "delete", method: http.MethodDelete, path: "/opencode-bindings/binding-main"},
		}

		for _, tc := range testCases {
			req := httptest.NewRequestWithContext(
				t.Context(),
				tc.method,
				tc.path,
				strings.NewReader(`{}`),
			)
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusNotFound, rec.Code, tc.name)
		}
	})

	t.Run("direct CRUD methods still work until later cleanup", func(t *testing.T) {
		t.Parallel()

		srv := newServer(newBindingService(t))

		createReq := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			"/opencode-bindings",
			strings.NewReader(
				`{"name":"binding-main","profileName":"profile-main","cwd":"/tmp","agentCommand":{"command":"opencode","args":["--fast"]},"launchOptions":{"transport":"stdio"}}`,
			),
		)
		createRec := httptest.NewRecorder()
		srv.CreateOpenCodeBinding(createRec, createReq)
		require.Equal(t, http.StatusCreated, createRec.Code)

		getReq := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodGet,
			"/opencode-bindings/binding-main",
			nil,
		)
		getRec := httptest.NewRecorder()
		srv.GetOpenCodeBinding(getRec, getReq, "binding-main")
		require.Equal(t, http.StatusOK, getRec.Code)

		updateReq := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPut,
			"/opencode-bindings/binding-main",
			strings.NewReader(
				`{"cwd":"/tmp/new","agentCommand":{"command":"opencode","args":["--safe"]},"launchOptions":{"transport":"stdio"}}`,
			),
		)
		updateRec := httptest.NewRecorder()
		srv.UpdateOpenCodeBinding(updateRec, updateReq, "binding-main")
		require.Equal(t, http.StatusOK, updateRec.Code)

		var updated OpenCodeBindingResponse
		require.NoError(t, json.NewDecoder(updateRec.Body).Decode(&updated))
		assert.Equal(t, "/tmp/new", updated.Cwd)
		assert.Equal(t, []string{"--safe"}, updated.AgentCommand.Args)

		listReq := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodGet,
			"/opencode-bindings",
			nil,
		)
		listRec := httptest.NewRecorder()
		srv.ListOpenCodeBindings(listRec, listReq)
		require.Equal(t, http.StatusOK, listRec.Code)

		var listed OpenCodeBindingListResponse
		require.NoError(t, json.NewDecoder(listRec.Body).Decode(&listed))
		require.Len(t, listed.Bindings, 1)

		deleteReq := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodDelete,
			"/opencode-bindings/binding-main",
			nil,
		)
		deleteRec := httptest.NewRecorder()
		srv.DeleteOpenCodeBinding(deleteRec, deleteReq, "binding-main")
		require.Equal(t, http.StatusNoContent, deleteRec.Code)
	})

	t.Run("create handles malformed conflict and validation errors", func(t *testing.T) {
		t.Parallel()

		srv := newServer(newBindingService(t))

		malformedReq := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			"/opencode-bindings",
			strings.NewReader(`{`),
		)
		malformedRec := httptest.NewRecorder()
		srv.CreateOpenCodeBinding(malformedRec, malformedReq)
		require.Equal(t, http.StatusBadRequest, malformedRec.Code)

		validBody := `{"name":"binding-conflict","profileName":"profile-main","cwd":"/tmp","agentCommand":{"command":"opencode","args":["--fast"]},"launchOptions":{"transport":"stdio"}}`
		createReq := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			"/opencode-bindings",
			strings.NewReader(validBody),
		)
		createRec := httptest.NewRecorder()
		srv.CreateOpenCodeBinding(createRec, createReq)
		require.Equal(t, http.StatusCreated, createRec.Code)

		conflictReq := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			"/opencode-bindings",
			strings.NewReader(validBody),
		)
		conflictRec := httptest.NewRecorder()
		srv.CreateOpenCodeBinding(conflictRec, conflictReq)
		require.Equal(t, http.StatusConflict, conflictRec.Code)

		invalidReq := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			"/opencode-bindings",
			strings.NewReader(
				`{"name":"bad binding","profileName":"profile-main","agentCommand":{"command":"","args":[]},"launchOptions":{"transport":"stdio"}}`,
			),
		)
		invalidRec := httptest.NewRecorder()
		srv.CreateOpenCodeBinding(invalidRec, invalidReq)
		require.Equal(t, http.StatusBadRequest, invalidRec.Code)
	})

	t.Run("list get update and delete map service failures", func(t *testing.T) {
		t.Parallel()

		srv := newServer(&stubOpenCodeBindingService{
			listFn: func(context.Context) ([]agent.OpenCodeBinding, error) {
				return nil, errors.New("boom")
			},
			getFn: func(context.Context, string) (*agent.OpenCodeBinding, error) {
				return nil, errors.New("boom")
			},
			updateFn: func(context.Context, string, agent.UpdateOpenCodeBindingParams) (*agent.OpenCodeBinding, error) {
				return nil, errors.New("boom")
			},
			deleteFn: func(context.Context, string) error {
				return errors.New("boom")
			},
		})

		listReq := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodGet,
			"/opencode-bindings",
			nil,
		)
		listRec := httptest.NewRecorder()
		srv.ListOpenCodeBindings(listRec, listReq)
		require.Equal(t, http.StatusInternalServerError, listRec.Code)

		getReq := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodGet,
			"/opencode-bindings/fail",
			nil,
		)
		getRec := httptest.NewRecorder()
		srv.GetOpenCodeBinding(getRec, getReq, "fail")
		require.Equal(t, http.StatusInternalServerError, getRec.Code)

		updateReq := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPut,
			"/opencode-bindings/fail",
			strings.NewReader(
				`{"cwd":"/tmp","agentCommand":{"command":"opencode","args":["--safe"]},"launchOptions":{"transport":"stdio"}}`,
			),
		)
		updateRec := httptest.NewRecorder()
		srv.UpdateOpenCodeBinding(updateRec, updateReq, "fail")
		require.Equal(t, http.StatusBadRequest, updateRec.Code)

		deleteReq := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodDelete,
			"/opencode-bindings/fail",
			nil,
		)
		deleteRec := httptest.NewRecorder()
		srv.DeleteOpenCodeBinding(deleteRec, deleteReq, "fail")
		require.Equal(t, http.StatusInternalServerError, deleteRec.Code)
	})

	t.Run("get update and delete map not found errors", func(t *testing.T) {
		t.Parallel()

		srv := newServer(&stubOpenCodeBindingService{
			getFn: func(context.Context, string) (*agent.OpenCodeBinding, error) {
				return nil, agent.ErrOpenCodeBindingNotFound
			},
			updateFn: func(context.Context, string, agent.UpdateOpenCodeBindingParams) (*agent.OpenCodeBinding, error) {
				return nil, agent.ErrOpenCodeBindingNotFound
			},
			deleteFn: func(context.Context, string) error {
				return agent.ErrOpenCodeBindingNotFound
			},
		})

		getReq := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodGet,
			"/opencode-bindings/missing",
			nil,
		)
		getRec := httptest.NewRecorder()
		srv.GetOpenCodeBinding(getRec, getReq, "missing")
		require.Equal(t, http.StatusNotFound, getRec.Code)

		updateReq := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPut,
			"/opencode-bindings/missing",
			strings.NewReader(
				`{"cwd":"/tmp","agentCommand":{"command":"opencode","args":["--safe"]},"launchOptions":{"transport":"stdio"}}`,
			),
		)
		updateRec := httptest.NewRecorder()
		srv.UpdateOpenCodeBinding(updateRec, updateReq, "missing")
		require.Equal(t, http.StatusNotFound, updateRec.Code)

		deleteReq := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodDelete,
			"/opencode-bindings/missing",
			nil,
		)
		deleteRec := httptest.NewRecorder()
		srv.DeleteOpenCodeBinding(deleteRec, deleteReq, "missing")
		require.Equal(t, http.StatusNotFound, deleteRec.Code)
	})

	t.Run("missing service returns internal errors", func(t *testing.T) {
		t.Parallel()

		srv := newServer(nil)
		testCases := []struct {
			name string
			call func(*httptest.ResponseRecorder)
		}{
			{
				name: "list",
				call: func(rec *httptest.ResponseRecorder) {
					req := httptest.NewRequestWithContext(
						t.Context(),
						http.MethodGet,
						"/opencode-bindings",
						nil,
					)
					srv.ListOpenCodeBindings(rec, req)
				},
			},
			{
				name: "create",
				call: func(rec *httptest.ResponseRecorder) {
					req := httptest.NewRequestWithContext(
						t.Context(),
						http.MethodPost,
						"/opencode-bindings",
						strings.NewReader(`{}`),
					)
					srv.CreateOpenCodeBinding(rec, req)
				},
			},
			{
				name: "get",
				call: func(rec *httptest.ResponseRecorder) {
					req := httptest.NewRequestWithContext(
						t.Context(),
						http.MethodGet,
						"/opencode-bindings/missing",
						nil,
					)
					srv.GetOpenCodeBinding(rec, req, "missing")
				},
			},
			{
				name: "update",
				call: func(rec *httptest.ResponseRecorder) {
					req := httptest.NewRequestWithContext(
						t.Context(),
						http.MethodPut,
						"/opencode-bindings/missing",
						strings.NewReader(`{}`),
					)
					srv.UpdateOpenCodeBinding(rec, req, "missing")
				},
			},
			{
				name: "delete",
				call: func(rec *httptest.ResponseRecorder) {
					req := httptest.NewRequestWithContext(
						t.Context(),
						http.MethodDelete,
						"/opencode-bindings/missing",
						nil,
					)
					srv.DeleteOpenCodeBinding(rec, req, "missing")
				},
			},
		}

		for _, tc := range testCases {
			rec := httptest.NewRecorder()
			tc.call(rec)
			require.Equal(t, http.StatusInternalServerError, rec.Code, tc.name)
		}
	})

	t.Run("update handles malformed JSON", func(t *testing.T) {
		t.Parallel()

		srv := newServer(newBindingService(t))
		req := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPut,
			"/opencode-bindings/binding-main",
			strings.NewReader(`{`),
		)
		rec := httptest.NewRecorder()

		srv.UpdateOpenCodeBinding(rec, req, "binding-main")

		require.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

type stubOpenCodeBindingService struct {
	listFn   func(ctx context.Context) ([]agent.OpenCodeBinding, error)
	getFn    func(ctx context.Context, name string) (*agent.OpenCodeBinding, error)
	createFn func(ctx context.Context, params agent.CreateOpenCodeBindingParams) (*agent.OpenCodeBinding, error)
	updateFn func(ctx context.Context, name string, params agent.UpdateOpenCodeBindingParams) (*agent.OpenCodeBinding, error)
	deleteFn func(ctx context.Context, name string) error
}

func (s *stubOpenCodeBindingService) List(ctx context.Context) ([]agent.OpenCodeBinding, error) {
	if s.listFn != nil {
		return s.listFn(ctx)
	}
	return []agent.OpenCodeBinding{}, nil
}

func (s *stubOpenCodeBindingService) Get(ctx context.Context, name string) (*agent.OpenCodeBinding, error) {
	if s.getFn != nil {
		return s.getFn(ctx, name)
	}
	return &agent.OpenCodeBinding{
		Name:        name,
		ProfileName: "profile-main",
		CWD:         "/tmp",
		AgentCommand: agent.OpenCodeAgentCommand{
			Command: "opencode",
			Args:    []string{"--safe"},
		},
		LaunchOptions: agent.OpenCodeLaunchOptions{Transport: "stdio"},
	}, nil
}

func (s *stubOpenCodeBindingService) Create(
	ctx context.Context,
	params agent.CreateOpenCodeBindingParams,
) (*agent.OpenCodeBinding, error) {
	if s.createFn != nil {
		return s.createFn(ctx, params)
	}
	return &agent.OpenCodeBinding{
		Name:          params.Name,
		ProfileName:   params.ProfileName,
		CWD:           params.CWD,
		AgentCommand:  params.AgentCommand,
		LaunchOptions: params.LaunchOptions,
	}, nil
}

func (s *stubOpenCodeBindingService) Update(
	ctx context.Context,
	name string,
	params agent.UpdateOpenCodeBindingParams,
) (*agent.OpenCodeBinding, error) {
	if s.updateFn != nil {
		return s.updateFn(ctx, name, params)
	}
	return &agent.OpenCodeBinding{
		Name:        name,
		ProfileName: "profile-main",
		CWD:         params.CWD,
		AgentCommand: agent.OpenCodeAgentCommand{
			Command: params.AgentCommand.Command,
			Args:    append([]string(nil), params.AgentCommand.Args...),
		},
		LaunchOptions: params.LaunchOptions,
	}, nil
}

func (s *stubOpenCodeBindingService) Delete(ctx context.Context, name string) error {
	if s.deleteFn != nil {
		return s.deleteFn(ctx, name)
	}
	return nil
}

func (s *stubOpenCodeBindingService) AutoMigrate() error {
	return nil
}
