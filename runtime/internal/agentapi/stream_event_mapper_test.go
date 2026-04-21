package agentapi

import (
	"testing"

	rt "github.com/gemyago/sonalmod/runtime/internal"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

func TestAgentAPIStreamEventMapper(t *testing.T) {
	t.Run("ToStreamEvent", func(t *testing.T) {
		mapper := NewAgentAPIStreamEventMapper()

		t.Run("table", func(t *testing.T) {
			fake := faker.New()
			roleModel := Model

			invPartial := fake.UUID().V4()
			authorName := fake.Lorem().Word()
			branchPath := "root." + fake.Lorem().Word()
			textHello := fake.Lorem().Word()

			invFinal := fake.UUID().V4()
			textFull := fake.Lorem().Sentence(fake.IntBetween(4, 12))

			invErr := fake.UUID().V4()
			errCode := fake.Lorem().Word()
			errMsg := fake.Lorem().Sentence(4)
			ignoredWhenError := fake.Lorem().Word()

			tests := []struct {
				name    string
				event   *session.Event
				wantErr require.ErrorAssertionFunc
				check   func(t *testing.T, got StreamEvent)
			}{
				{
					name:    "nil_event",
					event:   nil,
					wantErr: func(t require.TestingT, err error, _ ...any) { require.ErrorIs(t, err, ErrNilSessionEvent) },
					check:   nil,
				},
				{
					name: "partial_agent_chunk",
					event: func() *session.Event {
						ev := session.NewEvent(invPartial)
						ev.Author = authorName
						ev.Branch = branchPath
						ev.Content = &genai.Content{
							Role:  string(roleModel),
							Parts: []*genai.Part{{Text: textHello}},
						}
						ev.Partial = true
						ev.TurnComplete = false
						return ev
					}(),
					wantErr: require.NoError,
					check: func(t *testing.T, got StreamEvent) {
						t.Helper()
						d, err := got.Discriminator()
						require.NoError(t, err)
						assert.Equal(t, "agent", d)
						agent, err := got.AsAgentStreamEvent()
						require.NoError(t, err)
						assert.Equal(t, "agent", agent.Event)
						require.NotNil(t, agent.Partial)
						assert.True(t, *agent.Partial)
						if agent.TurnComplete != nil {
							assert.False(t, *agent.TurnComplete)
						}
						require.NotNil(t, agent.Content)
						assert.Equal(t, &roleModel, agent.Content.Role)
						require.Len(t, agent.Content.Parts, 1)
						assert.Equal(t, &textHello, agent.Content.Parts[0].Text)
						require.NotNil(t, agent.Author)
						assert.Equal(t, authorName, *agent.Author)
						require.NotNil(t, agent.Branch)
						assert.Equal(t, branchPath, *agent.Branch)
						require.NotNil(t, agent.InvocationId)
						assert.Equal(t, invPartial, *agent.InvocationId)
					},
				},
				{
					name: "final_turn_complete",
					event: func() *session.Event {
						ev := session.NewEvent(invFinal)
						ev.Content = &genai.Content{
							Parts: []*genai.Part{{Text: textFull}},
						}
						ev.Partial = false
						ev.TurnComplete = true
						return ev
					}(),
					wantErr: require.NoError,
					check: func(t *testing.T, got StreamEvent) {
						t.Helper()
						agent, err := got.AsAgentStreamEvent()
						require.NoError(t, err)
						require.NotNil(t, agent.Partial)
						assert.False(t, *agent.Partial)
						require.NotNil(t, agent.TurnComplete)
						assert.True(t, *agent.TurnComplete)
						require.NotNil(t, agent.Content)
						require.Len(t, agent.Content.Parts, 1)
						assert.Equal(t, &textFull, agent.Content.Parts[0].Text)
					},
				},
				{
					name: "error_like_adk_fields",
					event: func() *session.Event {
						ev := session.NewEvent(invErr)
						ev.ErrorCode = errCode
						ev.ErrorMessage = errMsg
						ev.Content = &genai.Content{
							Parts: []*genai.Part{{Text: ignoredWhenError}},
						}
						return ev
					}(),
					wantErr: require.NoError,
					check: func(t *testing.T, got StreamEvent) {
						t.Helper()
						d, err := got.Discriminator()
						require.NoError(t, err)
						assert.Equal(t, "error", d)
						se, err := got.AsStreamErrorEvent()
						require.NoError(t, err)
						assert.Equal(t, "error", se.Event)
						assert.Equal(t, errMsg, se.Message)
						require.NotNil(t, se.Code)
						assert.Equal(t, errCode, *se.Code)
					},
				},
				{
					name: "error_code_only_message_empty",
					event: func() *session.Event {
						ev := session.NewEvent(fake.UUID().V4())
						ev.ErrorCode = errCode
						ev.ErrorMessage = ""
						return ev
					}(),
					wantErr: require.NoError,
					check: func(t *testing.T, got StreamEvent) {
						t.Helper()
						se, err := got.AsStreamErrorEvent()
						require.NoError(t, err)
						assert.Equal(t, errCode, se.Message)
						require.NotNil(t, se.Code)
						assert.Equal(t, errCode, *se.Code)
					},
				},
				{
					name: "interrupted_flag",
					event: func() *session.Event {
						ev := session.NewEvent(invPartial)
						ev.Interrupted = true
						ev.Content = &genai.Content{
							Parts: []*genai.Part{{Text: textHello}},
						}
						return ev
					}(),
					wantErr: require.NoError,
					check: func(t *testing.T, got StreamEvent) {
						t.Helper()
						agent, err := got.AsAgentStreamEvent()
						require.NoError(t, err)
						require.NotNil(t, agent.Interrupted)
						assert.True(t, *agent.Interrupted)
					},
				},
				{
					name: "no_genai_content",
					event: func() *session.Event {
						ev := session.NewEvent(invPartial)
						ev.Content = nil
						return ev
					}(),
					wantErr: require.NoError,
					check: func(t *testing.T, got StreamEvent) {
						t.Helper()
						agent, err := got.AsAgentStreamEvent()
						require.NoError(t, err)
						assert.Nil(t, agent.Content)
					},
				},
				{
					name: "genai_content_role_only_no_parts",
					event: func() *session.Event {
						ev := session.NewEvent(invPartial)
						ev.Content = &genai.Content{
							Role: string(roleModel),
						}
						return ev
					}(),
					wantErr: require.NoError,
					check: func(t *testing.T, got StreamEvent) {
						t.Helper()
						agent, err := got.AsAgentStreamEvent()
						require.NoError(t, err)
						require.NotNil(t, agent.Content)
						assert.Equal(t, &roleModel, agent.Content.Role)
						assert.Empty(t, agent.Content.Parts)
					},
				},
				{
					name: "genai_content_skips_nil_and_empty_text_parts",
					event: func() *session.Event {
						ev := session.NewEvent(invPartial)
						ev.Content = &genai.Content{
							Parts: []*genai.Part{
								nil,
								{Text: ""},
								{Text: textHello},
							},
						}
						return ev
					}(),
					wantErr: require.NoError,
					check: func(t *testing.T, got StreamEvent) {
						t.Helper()
						agent, err := got.AsAgentStreamEvent()
						require.NoError(t, err)
						require.NotNil(t, agent.Content)
						require.Len(t, agent.Content.Parts, 1)
						assert.Equal(t, &textHello, agent.Content.Parts[0].Text)
					},
				},
				{
					name: "function_call_part_maps_to_tool_call",
					event: func() *session.Event {
						ev := session.NewEvent(fake.UUID().V4())
						callID := fake.UUID().V4()
						toolName := fake.Lorem().Word()
						ev.Content = &genai.Content{
							Role: string(roleModel),
							Parts: []*genai.Part{
								{FunctionCall: &genai.FunctionCall{
									ID:   callID,
									Name: toolName,
									Args: map[string]any{"key": fake.Lorem().Word()},
								}},
							},
						}
						return ev
					}(),
					wantErr: require.NoError,
					check: func(t *testing.T, got StreamEvent) {
						t.Helper()
						agent, err := got.AsAgentStreamEvent()
						require.NoError(t, err)
						require.NotNil(t, agent.Content)
						require.Len(t, agent.Content.Parts, 1)
						part := agent.Content.Parts[0]
						assert.Nil(t, part.Text)
						require.NotNil(t, part.ToolCall)
						assert.NotEmpty(t, part.ToolCall.Id)
						assert.NotEmpty(t, part.ToolCall.Name)
						require.NotNil(t, part.ToolCall.Args)
						assert.Nil(t, part.ToolResult)
					},
				},
				{
					name: "function_response_part_maps_to_tool_result",
					event: func() *session.Event {
						ev := session.NewEvent(fake.UUID().V4())
						callID := fake.UUID().V4()
						toolName := fake.Lorem().Word()
						ev.Content = &genai.Content{
							Role: "user",
							Parts: []*genai.Part{
								{FunctionResponse: &genai.FunctionResponse{
									ID:       callID,
									Name:     toolName,
									Response: map[string]any{"result": fake.Lorem().Word()},
								}},
							},
						}
						return ev
					}(),
					wantErr: require.NoError,
					check: func(t *testing.T, got StreamEvent) {
						t.Helper()
						agent, err := got.AsAgentStreamEvent()
						require.NoError(t, err)
						require.NotNil(t, agent.Content)
						require.Len(t, agent.Content.Parts, 1)
						part := agent.Content.Parts[0]
						assert.Nil(t, part.Text)
						assert.Nil(t, part.ToolCall)
						require.NotNil(t, part.ToolResult)
						assert.NotEmpty(t, part.ToolResult.Id)
						assert.NotEmpty(t, part.ToolResult.Name)
						require.NotNil(t, part.ToolResult.Response)
					},
				},
				{
					name: "mixed_text_and_function_call_parts",
					event: func() *session.Event {
						ev := session.NewEvent(fake.UUID().V4())
						ev.Content = &genai.Content{
							Role: string(roleModel),
							Parts: []*genai.Part{
								{Text: textHello},
								{FunctionCall: &genai.FunctionCall{
									ID:   fake.UUID().V4(),
									Name: fake.Lorem().Word(),
									Args: map[string]any{"x": fake.IntBetween(1, 100)},
								}},
							},
						}
						return ev
					}(),
					wantErr: require.NoError,
					check: func(t *testing.T, got StreamEvent) {
						t.Helper()
						agent, err := got.AsAgentStreamEvent()
						require.NoError(t, err)
						require.NotNil(t, agent.Content)
						require.Len(t, agent.Content.Parts, 2)
						assert.NotNil(t, agent.Content.Parts[0].Text)
						assert.Nil(t, agent.Content.Parts[0].ToolCall)
						assert.Nil(t, agent.Content.Parts[1].Text)
						assert.NotNil(t, agent.Content.Parts[1].ToolCall)
					},
				},
				{
					name: "multiple_parallel_function_calls_all_mapped",
					event: func() *session.Event {
						ev := session.NewEvent(fake.UUID().V4())
						ev.Content = &genai.Content{
							Role: string(roleModel),
							Parts: []*genai.Part{
								{FunctionCall: &genai.FunctionCall{
									ID:   fake.UUID().V4(),
									Name: fake.Lorem().Word(),
									Args: map[string]any{"a": fake.Lorem().Word()},
								}},
								{FunctionCall: &genai.FunctionCall{
									ID:   fake.UUID().V4(),
									Name: fake.Lorem().Word(),
									Args: map[string]any{"b": fake.Lorem().Word()},
								}},
								{FunctionCall: &genai.FunctionCall{
									ID:   fake.UUID().V4(),
									Name: fake.Lorem().Word(),
									Args: map[string]any{"c": fake.Lorem().Word()},
								}},
							},
						}
						return ev
					}(),
					wantErr: require.NoError,
					check: func(t *testing.T, got StreamEvent) {
						t.Helper()
						agent, err := got.AsAgentStreamEvent()
						require.NoError(t, err)
						require.NotNil(t, agent.Content)
						require.Len(t, agent.Content.Parts, 3)
						for i, part := range agent.Content.Parts {
							assert.Nil(t, part.Text, "part %d should have nil text", i)
							assert.NotNil(t, part.ToolCall, "part %d should have ToolCall set", i)
							assert.Nil(t, part.ToolResult, "part %d should have nil ToolResult", i)
						}
					},
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					got, err := mapper.ToStreamEvent(rt.MapADKSessionEvent(tt.event))
					tt.wantErr(t, err)
					if tt.check != nil {
						tt.check(t, got)
					}
				})
			}
		})
	})
}
