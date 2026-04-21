package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/toolconfirmation"
	"google.golang.org/genai"
)

// adkToolCtxStub is a minimal tool.Context for tests (pattern: ADK tool/tool_test.go).
type adkToolCtxStub struct {
	context.Context
}

func (c *adkToolCtxStub) ToolConfirmation() *toolconfirmation.ToolConfirmation { return nil }
func (c *adkToolCtxStub) RequestConfirmation(string, any) error                { return nil }
func (c *adkToolCtxStub) Actions() *session.EventActions                       { return nil }
func (c *adkToolCtxStub) FunctionCallID() string                               { return "test-fc-id" }
func (c *adkToolCtxStub) SearchMemory(context.Context, string) (*memory.SearchResponse, error) {
	return &memory.SearchResponse{}, nil
}
func (c *adkToolCtxStub) AgentName() string                    { return "test-agent" }
func (c *adkToolCtxStub) ReadonlyState() session.ReadonlyState { return nil }
func (c *adkToolCtxStub) State() session.State                 { return nil }
func (c *adkToolCtxStub) Artifacts() agent.Artifacts           { return nil }
func (c *adkToolCtxStub) InvocationID() string                 { return "test-invocation-id" }
func (c *adkToolCtxStub) UserContent() *genai.Content          { return nil }
func (c *adkToolCtxStub) AppName() string                      { return "test-app" }
func (c *adkToolCtxStub) Branch() string                       { return "test-branch" }
func (c *adkToolCtxStub) SessionID() string                    { return "test-session-id" }
func (c *adkToolCtxStub) UserID() string                       { return "test-user-id" }

var _ tool.Context = (*adkToolCtxStub)(nil)

func TestToolContext(t *testing.T) {
	t.Run("embeds_context", func(t *testing.T) {
		ctx := t.Context()
		tc := &ToolContext{Context: ctx}
		require.NotNil(t, tc)
		assert.Equal(t, ctx, tc.Context)
	})

	t.Run("newFromADK_panics_on_nil", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic")
			}
		}()
		_ = newToolContextFromADK(nil)
	})
}

// recursiveTestNode is a self-referential type used to verify that WithOutputSchema
// allows bypassing cycle-detection in the schema inference.
type recursiveTestNode struct {
	Name     string               `json:"name"`
	Children []*recursiveTestNode `json:"children,omitempty"`
}

type recursiveTestResponse struct {
	Root *recursiveTestNode `json:"root"`
}

func TestToolDef(t *testing.T) {
	t.Run("handleADK_invokes_handler_with_tool_context", func(t *testing.T) {
		ctx := t.Context()
		adkCtx := &adkToolCtxStub{Context: ctx}
		td := NewToolDef[struct{}, string](
			"test_tool",
			"desc",
			func(tc *ToolContext, _ struct{}) (string, error) {
				require.NotNil(t, tc)
				assert.Equal(t, adkCtx, tc.Context)
				return "ok", nil
			},
		)
		out, err := td.handleADK(adkCtx, struct{}{})
		require.NoError(t, err)
		assert.Equal(t, "ok", out)
	})

	t.Run("handler_accepts_test_tool_context", func(t *testing.T) {
		ctx := t.Context()
		td := NewToolDef[struct{}, string](
			"test_tool",
			"desc",
			func(tc *ToolContext, _ struct{}) (string, error) {
				require.NotNil(t, tc)
				assert.Equal(t, ctx, tc.Context)
				return "ok", nil
			},
		)
		out, err := td.Handler(&ToolContext{Context: ctx}, struct{}{})
		require.NoError(t, err)
		assert.Equal(t, "ok", out)
	})

	t.Run("newADKTool_fails_for_recursive_output_type_without_schema", func(t *testing.T) {
		td := NewToolDef[struct{}, recursiveTestResponse](
			"recursive_tool",
			"desc",
			func(_ *ToolContext, _ struct{}) (recursiveTestResponse, error) {
				return recursiveTestResponse{}, nil
			},
		)
		_, err := td.newADKTool()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cycle")
	})

	t.Run("WithOutputSchema_allows_recursive_output_type", func(t *testing.T) {
		schema := []byte(`{
			"type": "object",
			"properties": {
				"root": {"$ref": "#/$defs/node"}
			},
			"$defs": {
				"node": {
					"type": "object",
					"properties": {
						"name": {"type": "string"},
						"children": {"type": "array", "items": {"$ref": "#/$defs/node"}}
					},
					"required": ["name"]
				}
			}
		}`)
		td := NewToolDef[struct{}, recursiveTestResponse](
			"recursive_tool",
			"desc",
			func(_ *ToolContext, _ struct{}) (recursiveTestResponse, error) {
				return recursiveTestResponse{}, nil
			},
		).WithOutputSchema(schema)
		_, err := td.newADKTool()
		require.NoError(t, err)
	})

	t.Run("WithOutputSchema_returns_error_for_invalid_json", func(t *testing.T) {
		td := NewToolDef[struct{}, recursiveTestResponse](
			"recursive_tool",
			"desc",
			func(_ *ToolContext, _ struct{}) (recursiveTestResponse, error) {
				return recursiveTestResponse{}, nil
			},
		).WithOutputSchema([]byte(`not valid json`))
		_, err := td.newADKTool()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid output schema JSON")
	})
}
