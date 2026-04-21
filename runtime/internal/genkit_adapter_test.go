//go:build !release

package internal

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"

	genkitai "github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

func makeGenkitAdapterMockDeps(t *testing.T) GenkitLLMAdapterDeps {
	t.Helper()
	return GenkitLLMAdapterDeps{
		Genkit:     &genkit.Genkit{},
		RootLogger: RootTestLogger().With("test", t.Name()),
	}
}

func TestConvertADKRequestToGenkitOptions(t *testing.T) {
	fake := faker.New()

	t.Run("single user message with text part", func(t *testing.T) {
		text := fake.Lorem().Sentence(10)
		req := &model.LLMRequest{
			Contents: []*genai.Content{
				{
					Role: string(genai.RoleUser),
					Parts: []*genai.Part{
						{Text: text},
					},
				},
			},
		}

		opts := convertADKRequestToGenkitOptions(req)

		require.NotNil(t, opts)
		require.Len(t, opts.Messages, 1)
		msg := opts.Messages[0]
		assert.Equal(t, genkitai.Role(genai.RoleUser), msg.Role)
		require.Len(t, msg.Content, 1)
		assert.Equal(t, text, msg.Content[0].Text)
	})

	t.Run("multi-turn user model user", func(t *testing.T) {
		user1 := fake.Lorem().Sentence(5)
		modelResp := fake.Lorem().Sentence(8)
		user2 := fake.Lorem().Sentence(6)
		req := &model.LLMRequest{
			Contents: []*genai.Content{
				{Role: string(genai.RoleUser), Parts: []*genai.Part{{Text: user1}}},
				{Role: string(genai.RoleModel), Parts: []*genai.Part{{Text: modelResp}}},
				{Role: string(genai.RoleUser), Parts: []*genai.Part{{Text: user2}}},
			},
		}

		opts := convertADKRequestToGenkitOptions(req)

		require.NotNil(t, opts)
		require.Len(t, opts.Messages, 3)
		assert.Equal(t, genkitai.Role(genai.RoleUser), opts.Messages[0].Role)
		assert.Equal(t, user1, opts.Messages[0].Content[0].Text)
		assert.Equal(t, genkitai.Role(genai.RoleModel), opts.Messages[1].Role)
		assert.Equal(t, modelResp, opts.Messages[1].Content[0].Text)
		assert.Equal(t, genkitai.Role(genai.RoleUser), opts.Messages[2].Role)
		assert.Equal(t, user2, opts.Messages[2].Content[0].Text)
	})

	t.Run("system instruction from config prepended before conversation", func(t *testing.T) {
		sysText := fake.Lorem().Sentence(8)
		userText := fake.Lorem().Sentence(5)
		req := &model.LLMRequest{
			Config: &genai.GenerateContentConfig{
				SystemInstruction: &genai.Content{
					Parts: []*genai.Part{{Text: sysText}},
				},
			},
			Contents: []*genai.Content{
				{Role: string(genai.RoleUser), Parts: []*genai.Part{{Text: userText}}},
			},
		}

		opts := convertADKRequestToGenkitOptions(req)

		require.Len(t, opts.Messages, 2)
		assert.Equal(t, genkitai.RoleSystem, opts.Messages[0].Role)
		require.Len(t, opts.Messages[0].Content, 1)
		assert.Equal(t, sysText, opts.Messages[0].Content[0].Text)
		assert.Equal(t, genkitai.Role(genai.RoleUser), opts.Messages[1].Role)
		assert.Equal(t, userText, opts.Messages[1].Content[0].Text)
	})

	t.Run("config present but empty system instruction unchanged message list", func(t *testing.T) {
		userText := fake.Lorem().Sentence(4)
		req := &model.LLMRequest{
			Config: &genai.GenerateContentConfig{},
			Contents: []*genai.Content{
				{Role: string(genai.RoleUser), Parts: []*genai.Part{{Text: userText}}},
			},
		}

		opts := convertADKRequestToGenkitOptions(req)

		require.Len(t, opts.Messages, 1)
		assert.Equal(t, userText, opts.Messages[0].Content[0].Text)
	})

	t.Run("empty contents", func(t *testing.T) {
		req := &model.LLMRequest{
			Contents: []*genai.Content{},
		}

		opts := convertADKRequestToGenkitOptions(req)

		require.NotNil(t, opts)
		assert.Empty(t, opts.Messages)
	})

	t.Run("model override", func(t *testing.T) {
		modelName := fake.Lorem().Word() + "-" + fake.Lorem().Word() + "-" + fake.Lorem().Word()
		req := &model.LLMRequest{
			Model: modelName,
			Contents: []*genai.Content{
				{Role: string(genai.RoleUser), Parts: []*genai.Part{{Text: fake.Lorem().Sentence(3)}}},
			},
		}

		opts := convertADKRequestToGenkitOptions(req)

		require.NotNil(t, opts)
		assert.Equal(t, modelName, opts.Model)
	})

	t.Run("config not passed through - genai type incompatible with Genkit compat_oai", func(t *testing.T) {
		temp := float32(fake.IntBetween(50, 100)) / 100
		maxTokens := int32(fake.IntBetween(50, 500))
		cfg := &genai.GenerateContentConfig{
			Temperature:     new(temp),
			MaxOutputTokens: maxTokens,
		}
		req := &model.LLMRequest{
			Config:   cfg,
			Contents: []*genai.Content{},
		}

		opts := convertADKRequestToGenkitOptions(req)

		require.NotNil(t, opts)
		assert.Nil(t, opts.Config, "genai config not passed through; compat_oai expects openai types")
	})

	t.Run("tools passthrough extracts tool names from map keys", func(t *testing.T) {
		toolNameA := fake.Lorem().Word()
		toolNameB := fake.Lorem().Word()
		req := &model.LLMRequest{
			Tools: map[string]any{
				toolNameA: map[string]any{"description": fake.Lorem().Sentence(2)},
				toolNameB: map[string]any{"description": fake.Lorem().Sentence(2)},
			},
			Contents: []*genai.Content{
				{Role: string(genai.RoleUser), Parts: []*genai.Part{{Text: fake.Lorem().Word()}}},
			},
		}

		opts := convertADKRequestToGenkitOptions(req)

		require.NotNil(t, opts)
		require.Len(t, opts.Tools, 2)
		assert.Contains(t, opts.Tools, toolNameA)
		assert.Contains(t, opts.Tools, toolNameB)
		assert.True(t, opts.ReturnToolRequests,
			"ReturnToolRequests must be true when tools present to delegate execution to ADK")
	})

	t.Run("tool response messages get role tool for OpenAI compat", func(t *testing.T) {
		funcRespName := fake.Lorem().Word()
		funcRespKey := fake.Lorem().Word()
		funcRespVal := fake.IntBetween(1, 100)
		req := &model.LLMRequest{
			Tools: map[string]any{"getCurrentTime": map[string]any{}},
			Contents: []*genai.Content{
				{Role: string(genai.RoleUser), Parts: []*genai.Part{{Text: fake.Lorem().Word()}}},
				{Role: string(genai.RoleModel), Parts: []*genai.Part{
					{FunctionCall: &genai.FunctionCall{Name: "getCurrentTime", ID: "call_1", Args: map[string]any{}}},
				}},
				{Role: string(genai.RoleUser), Parts: []*genai.Part{
					{
						FunctionResponse: &genai.FunctionResponse{
							Name: funcRespName, ID: "call_1",
							Response: map[string]any{funcRespKey: funcRespVal},
						},
					},
				}},
			},
		}

		opts := convertADKRequestToGenkitOptions(req)

		require.NotNil(t, opts)
		require.Len(t, opts.Messages, 3)
		assert.Equal(t, genkitai.RoleUser, opts.Messages[0].Role)
		assert.Equal(t, genkitai.RoleModel, opts.Messages[1].Role)
		assert.Equal(t, genkitai.RoleTool, opts.Messages[2].Role,
			"messages with only FunctionResponse parts must use role tool for OpenAI compat")
	})

	t.Run("ReturnToolRequests false when no tools", func(t *testing.T) {
		req := &model.LLMRequest{
			Contents: []*genai.Content{
				{Role: string(genai.RoleUser), Parts: []*genai.Part{{Text: fake.Lorem().Word()}}},
			},
		}

		opts := convertADKRequestToGenkitOptions(req)

		require.NotNil(t, opts)
		assert.Empty(t, opts.Tools)
		assert.False(t, opts.ReturnToolRequests)
	})

	t.Run("nil contents produces empty messages", func(t *testing.T) {
		req := &model.LLMRequest{
			Contents: nil,
		}

		opts := convertADKRequestToGenkitOptions(req)

		require.NotNil(t, opts)
		assert.Empty(t, opts.Messages)
	})

	t.Run("skips nil content entries", func(t *testing.T) {
		text := fake.Lorem().Sentence(3)
		req := &model.LLMRequest{
			Contents: []*genai.Content{
				nil,
				{Role: string(genai.RoleUser), Parts: []*genai.Part{{Text: text}}},
				nil,
			},
		}

		opts := convertADKRequestToGenkitOptions(req)

		require.NotNil(t, opts)
		require.Len(t, opts.Messages, 1)
		assert.Equal(t, text, opts.Messages[0].Content[0].Text)
	})

	t.Run("nil request returns empty options", func(t *testing.T) {
		opts := convertADKRequestToGenkitOptions(nil)

		require.NotNil(t, opts)
		assert.Empty(t, opts.Messages)
		assert.Empty(t, opts.Model)
		assert.Nil(t, opts.Config)
	})

	t.Run("maps InlineData FunctionCall FunctionResponse FileData parts", func(t *testing.T) {
		text := fake.Lorem().Sentence(2)
		inlineData := []byte(fake.Lorem().Word())
		mimeType := fake.MimeType().MimeType()
		funcCallName := fake.Lorem().Word()
		funcCallArgKey := fake.Lorem().Word()
		funcCallArgVal := fake.IntBetween(1, 100)
		funcRespName := fake.Lorem().Word()
		funcRespKey := fake.Lorem().Word()
		funcRespVal := fake.Bool()
		fileURI := "file:///" + fake.Lorem().Word() + "." + fake.File().Extension()
		req := &model.LLMRequest{
			Contents: []*genai.Content{
				{
					Role: string(genai.RoleUser),
					Parts: []*genai.Part{
						{Text: text},
						{InlineData: &genai.Blob{Data: inlineData, MIMEType: mimeType}},
						{
							FunctionCall: &genai.FunctionCall{
								Name: funcCallName,
								Args: map[string]any{funcCallArgKey: funcCallArgVal},
							},
						},
						{
							FunctionResponse: &genai.FunctionResponse{
								Name:     funcRespName,
								Response: map[string]any{funcRespKey: funcRespVal},
							},
						},
						{FileData: &genai.FileData{FileURI: fileURI}},
						nil,
					},
				},
			},
		}

		opts := convertADKRequestToGenkitOptions(req)

		require.NotNil(t, opts)
		require.Len(t, opts.Messages, 1)
		require.Len(t, opts.Messages[0].Content, 5, "text, inline, functioncall, functionresponse, filedata")
		assert.Equal(t, text, opts.Messages[0].Content[0].Text)
		// InlineData → MediaPart (base64)
		assert.True(t, opts.Messages[0].Content[1].IsMedia())
		// FunctionCall → ToolRequest
		assert.True(t, opts.Messages[0].Content[2].IsToolRequest())
		assert.Equal(t, funcCallName, opts.Messages[0].Content[2].ToolRequest.Name)
		assert.Equal(t, map[string]any{funcCallArgKey: funcCallArgVal}, opts.Messages[0].Content[2].ToolRequest.Input)
		// FunctionResponse → ToolResponse
		assert.True(t, opts.Messages[0].Content[3].IsToolResponse())
		assert.Equal(t, funcRespName, opts.Messages[0].Content[3].ToolResponse.Name)
		// FileData → ResourcePart
		assert.True(t, opts.Messages[0].Content[4].IsResource())
		assert.Equal(t, fileURI, opts.Messages[0].Content[4].Resource.Uri)
	})

	t.Run("FunctionCall with nil Args uses empty map", func(t *testing.T) {
		funcName := fake.Lorem().Word()
		req := &model.LLMRequest{
			Contents: []*genai.Content{
				{
					Role: string(genai.RoleUser),
					Parts: []*genai.Part{
						{FunctionCall: &genai.FunctionCall{Name: funcName, Args: nil}},
					},
				},
			},
		}

		opts := convertADKRequestToGenkitOptions(req)

		require.NotNil(t, opts)
		require.Len(t, opts.Messages[0].Content, 1)
		assert.True(t, opts.Messages[0].Content[0].IsToolRequest())
		assert.Equal(t, funcName, opts.Messages[0].Content[0].ToolRequest.Name)
		assert.Equal(t, map[string]any{}, opts.Messages[0].Content[0].ToolRequest.Input)
	})

	t.Run("FunctionResponse with nil Response uses empty map", func(t *testing.T) {
		funcName := fake.Lorem().Word()
		req := &model.LLMRequest{
			Contents: []*genai.Content{
				{
					Role: string(genai.RoleUser),
					Parts: []*genai.Part{
						{FunctionResponse: &genai.FunctionResponse{Name: funcName, Response: nil}},
					},
				},
			},
		}

		opts := convertADKRequestToGenkitOptions(req)

		require.NotNil(t, opts)
		require.Len(t, opts.Messages[0].Content, 1)
		assert.True(t, opts.Messages[0].Content[0].IsToolResponse())
		assert.Equal(t, funcName, opts.Messages[0].Content[0].ToolResponse.Name)
		assert.Equal(t, map[string]any{}, opts.Messages[0].Content[0].ToolResponse.Output)
	})
}

func TestConvertGenkitResponseToADK(t *testing.T) {
	fake := faker.New()

	t.Run("simple text response", func(t *testing.T) {
		text := fake.Lorem().Sentence(8)
		resp := &genkitai.ModelResponse{
			Message: &genkitai.Message{
				Role: genkitai.RoleModel,
				Content: []*genkitai.Part{
					genkitai.NewTextPart(text),
				},
			},
		}

		got := convertGenkitResponseToADK(resp, false)

		require.NotNil(t, got)
		require.NotNil(t, got.Content)
		assert.Equal(t, string(genkitai.RoleModel), got.Content.Role)
		require.Len(t, got.Content.Parts, 1)
		assert.Equal(t, text, got.Content.Parts[0].Text)
		assert.False(t, got.Partial)
		assert.True(t, got.TurnComplete)
		assert.False(t, got.Interrupted)
		assert.NotNil(t, got.CustomMetadata)
	})

	t.Run("response with finish reason", func(t *testing.T) {
		text := fake.Lorem().Sentence(3)
		resp := &genkitai.ModelResponse{
			Message: &genkitai.Message{
				Role: genkitai.RoleModel,
				Content: []*genkitai.Part{
					genkitai.NewTextPart(text),
				},
			},
			FinishReason: genkitai.FinishReasonStop,
		}

		got := convertGenkitResponseToADK(resp, false)

		require.NotNil(t, got)
		assert.Equal(t, genai.FinishReasonStop, got.FinishReason)
	})

	t.Run("convertGenkitChunkToADK sets Partial and TurnComplete", func(t *testing.T) {
		text := fake.Lorem().Sentence(2)
		chunk := &genkitai.ModelResponseChunk{
			Content: []*genkitai.Part{genkitai.NewTextPart(text)},
			Role:    genkitai.RoleModel,
			Index:   0,
		}

		partial := convertGenkitChunkToADK(chunk, true)
		require.NotNil(t, partial)
		assert.True(t, partial.Partial)
		assert.False(t, partial.TurnComplete)
		assert.Equal(t, text, partial.Content.Parts[0].Text)

		final := convertGenkitChunkToADK(chunk, false)
		require.NotNil(t, final)
		assert.False(t, final.Partial)
		assert.True(t, final.TurnComplete)
		assert.Equal(t, text, final.Content.Parts[0].Text)
	})

	t.Run("convertGenkitChunkToADK handles nil chunk", func(t *testing.T) {
		got := convertGenkitChunkToADK(nil, true)

		require.NotNil(t, got)
		assert.True(t, got.Partial)
		assert.False(t, got.TurnComplete)
		assert.Nil(t, got.Content)
	})

	t.Run("dropToolPartsFromPartialChunk removes function parts and keeps text", func(t *testing.T) {
		toolName := fake.Lorem().Word()
		toolID := fake.UUID().V4()
		text := fake.Lorem().Sentence(2)
		resp := &model.LLMResponse{
			Content: &genai.Content{
				Role: string(genkitai.RoleModel),
				Parts: []*genai.Part{
					{Text: text},
					{FunctionCall: &genai.FunctionCall{Name: toolName, ID: toolID, Args: map[string]any{}}},
					{FunctionResponse: &genai.FunctionResponse{Name: toolName, ID: toolID, Response: map[string]any{}}},
				},
			},
		}

		dropToolPartsFromPartialChunk(resp)

		require.NotNil(t, resp.Content)
		require.Len(t, resp.Content.Parts, 1)
		assert.Equal(t, text, resp.Content.Parts[0].Text)
	})

	t.Run("streaming vs non-streaming", func(t *testing.T) {
		text := fake.Lorem().Sentence(2)
		resp := &genkitai.ModelResponse{
			Message: &genkitai.Message{
				Role:    genkitai.RoleModel,
				Content: []*genkitai.Part{genkitai.NewTextPart(text)},
			},
		}

		nonStream := convertGenkitResponseToADK(resp, false)
		stream := convertGenkitResponseToADK(resp, true)

		require.NotNil(t, nonStream)
		require.NotNil(t, stream)
		assert.False(t, nonStream.Partial)
		assert.True(t, nonStream.TurnComplete)
		assert.False(t, stream.Partial)
		assert.True(t, stream.TurnComplete)
	})

	t.Run("usage metadata when available", func(t *testing.T) {
		text := fake.Lorem().Word()
		inputTokens := fake.IntBetween(1, 100)
		outputTokens := fake.IntBetween(1, 100)
		totalTokens := inputTokens + outputTokens
		resp := &genkitai.ModelResponse{
			Message: &genkitai.Message{
				Role:    genkitai.RoleModel,
				Content: []*genkitai.Part{genkitai.NewTextPart(text)},
			},
			Usage: &genkitai.GenerationUsage{
				InputTokens:  inputTokens,
				OutputTokens: outputTokens,
				TotalTokens:  totalTokens,
			},
		}

		got := convertGenkitResponseToADK(resp, false)

		require.NotNil(t, got)
		require.NotNil(t, got.UsageMetadata)
		assert.Equal(t, int32(inputTokens), got.UsageMetadata.PromptTokenCount)
		assert.Equal(t, int32(outputTokens), got.UsageMetadata.CandidatesTokenCount)
		assert.Equal(t, int32(totalTokens), got.UsageMetadata.TotalTokenCount)
	})

	t.Run("nil response returns empty response", func(t *testing.T) {
		got := convertGenkitResponseToADK(nil, false)

		require.NotNil(t, got)
		assert.Nil(t, got.Content)
		assert.NotNil(t, got.CustomMetadata)
	})

	t.Run("nil message produces empty content", func(t *testing.T) {
		resp := &genkitai.ModelResponse{
			Message: nil,
		}

		got := convertGenkitResponseToADK(resp, false)

		require.NotNil(t, got)
		assert.Nil(t, got.Content)
	})

	t.Run("interrupted when finish reason is interrupted", func(t *testing.T) {
		text := fake.Lorem().Word()
		resp := &genkitai.ModelResponse{
			Message: &genkitai.Message{
				Role:    genkitai.RoleModel,
				Content: []*genkitai.Part{genkitai.NewTextPart(text)},
			},
			FinishReason: genkitai.FinishReasonInterrupted,
		}

		got := convertGenkitResponseToADK(resp, false)

		require.NotNil(t, got)
		assert.True(t, got.Interrupted)
	})

	t.Run("maps ToolRequest and ToolResponse parts to FunctionCall and FunctionResponse", func(t *testing.T) {
		text := fake.Lorem().Sentence(2)
		toolName := fake.Lorem().Word()
		argKey := fake.Lorem().Word()
		argVal := fake.IntBetween(1, 100)
		respKey := fake.Lorem().Word()
		respVal := fake.Lorem().Word()
		resp := &genkitai.ModelResponse{
			Message: &genkitai.Message{
				Role: genkitai.RoleModel,
				Content: []*genkitai.Part{
					genkitai.NewTextPart(text),
					genkitai.NewToolRequestPart(&genkitai.ToolRequest{
						Name: toolName, Input: map[string]any{argKey: argVal},
					}),
					genkitai.NewToolResponsePart(&genkitai.ToolResponse{
						Name: toolName, Output: map[string]any{respKey: respVal},
					}),
				},
			},
		}

		got := convertGenkitResponseToADK(resp, false)

		require.NotNil(t, got)
		require.NotNil(t, got.Content)
		require.Len(t, got.Content.Parts, 3)
		assert.Equal(t, text, got.Content.Parts[0].Text)
		require.NotNil(t, got.Content.Parts[1].FunctionCall)
		assert.Equal(t, toolName, got.Content.Parts[1].FunctionCall.Name)
		assert.EqualValues(t, argVal, got.Content.Parts[1].FunctionCall.Args[argKey])
		require.NotNil(t, got.Content.Parts[2].FunctionResponse)
		assert.Equal(t, toolName, got.Content.Parts[2].FunctionResponse.Name)
		assert.Equal(t, map[string]any{respKey: respVal}, got.Content.Parts[2].FunctionResponse.Response)
	})

	t.Run("ToolRequest with JSON string input parses into args map", func(t *testing.T) {
		toolName := fake.Lorem().Word()
		argKey := fake.Lorem().Word()
		argVal := fake.IntBetween(1, 100)
		jsonInput := `{"` + argKey + `":` + strconv.Itoa(argVal) + `}`
		resp := &genkitai.ModelResponse{
			Message: &genkitai.Message{
				Role: genkitai.RoleModel,
				Content: []*genkitai.Part{
					genkitai.NewToolRequestPart(&genkitai.ToolRequest{Name: toolName, Input: jsonInput}),
				},
			},
		}

		got := convertGenkitResponseToADK(resp, false)

		require.NotNil(t, got)
		require.Len(t, got.Content.Parts, 1)
		require.NotNil(t, got.Content.Parts[0].FunctionCall)
		assert.Equal(t, toolName, got.Content.Parts[0].FunctionCall.Name)
		assert.EqualValues(t, argVal, got.Content.Parts[0].FunctionCall.Args[argKey])
	})

	t.Run("ToolRequest with invalid non-object input falls back to empty args map", func(t *testing.T) {
		toolName := fake.Lorem().Word()
		resp := &genkitai.ModelResponse{
			Message: &genkitai.Message{
				Role: genkitai.RoleModel,
				Content: []*genkitai.Part{
					genkitai.NewToolRequestPart(&genkitai.ToolRequest{Name: toolName, Input: "not-json"}),
				},
			},
		}

		got := convertGenkitResponseToADK(resp, false)

		require.NotNil(t, got)
		require.Len(t, got.Content.Parts, 1)
		require.NotNil(t, got.Content.Parts[0].FunctionCall)
		assert.Equal(t, toolName, got.Content.Parts[0].FunctionCall.Name)
		assert.Equal(t, map[string]any{}, got.Content.Parts[0].FunctionCall.Args)
	})

	t.Run("ToolResponse with non-map Output wraps in output key", func(t *testing.T) {
		toolName := fake.Lorem().Word()
		outputVal := fake.IntBetween(1, 100)
		resp := &genkitai.ModelResponse{
			Message: &genkitai.Message{
				Role: genkitai.RoleModel,
				Content: []*genkitai.Part{
					genkitai.NewToolResponsePart(&genkitai.ToolResponse{Name: toolName, Output: outputVal}),
				},
			},
		}

		got := convertGenkitResponseToADK(resp, false)

		require.NotNil(t, got)
		require.Len(t, got.Content.Parts, 1)
		require.NotNil(t, got.Content.Parts[0].FunctionResponse)
		assert.Equal(t, toolName, got.Content.Parts[0].FunctionResponse.Name)
		assert.Equal(t, map[string]any{"output": outputVal}, got.Content.Parts[0].FunctionResponse.Response)
	})

	t.Run("convertGenkitPartToGenai nil and unsupported parts return nil", func(t *testing.T) {
		require.Nil(t, convertGenkitPartToGenai(nil))
		require.Nil(t, convertGenkitPartToGenai(&genkitai.Part{}))
	})

	t.Run("normalizeToolRequestInputToArgsMap supports bytes raw message and empty values", func(t *testing.T) {
		argKey := fake.Lorem().Word()
		argVal := fake.IntBetween(1, 100)
		jsonInput := []byte(`{"` + argKey + `":` + strconv.Itoa(argVal) + `}`)

		assert.EqualValues(t, argVal, normalizeToolRequestInputToArgsMap(jsonInput)[argKey])
		assert.EqualValues(t, argVal, normalizeToolRequestInputToArgsMap(json.RawMessage(jsonInput))[argKey])
		assert.Equal(t, map[string]any{}, normalizeToolRequestInputToArgsMap("   "))
		assert.Equal(t, map[string]any{}, normalizeToolRequestInputToArgsMap(42))
	})

	t.Run("GenkitLLMAdapter constructor and Name", func(t *testing.T) {
		modelName := fake.Lorem().Word() + "-" + fake.Lorem().Word()
		deps := makeGenkitAdapterMockDeps(t)

		factory := NewGenkitLLMAdapterFactory(deps)
		adapter := factory(modelName)

		require.NotNil(t, adapter)
		assert.Equal(t, modelName, adapter.Name())
	})

	t.Run("GenkitLLMAdapter GenerateContent non-streaming", func(t *testing.T) {
		f := faker.New()
		text := f.Lorem().Sentence(5)
		modelName := f.Lorem().Word() + "-" + f.Lorem().Word()
		userMsg := f.Lorem().Word()
		fakeResp := &genkitai.ModelResponse{
			Message: &genkitai.Message{
				Role:    genkitai.RoleModel,
				Content: []*genkitai.Part{genkitai.NewTextPart(text)},
			},
			FinishReason: genkitai.FinishReasonStop,
		}

		deps := makeGenkitAdapterMockDeps(t)
		deps.generateWithRequest = func(
			_ context.Context, _ *genkit.Genkit, _ *genkitai.GenerateActionOptions,
			_ []genkitai.ModelMiddleware, _ genkitai.ModelStreamCallback,
		) (*genkitai.ModelResponse, error) {
			return fakeResp, nil
		}
		factory := NewGenkitLLMAdapterFactory(deps)
		adapter := factory(modelName)

		req := &model.LLMRequest{
			Contents: []*genai.Content{
				{Role: string(genai.RoleUser), Parts: []*genai.Part{{Text: userMsg}}},
			},
		}

		var got *model.LLMResponse
		for resp, err := range adapter.GenerateContent(t.Context(), req, false) {
			require.NoError(t, err)
			got = resp
		}

		require.NotNil(t, got)
		require.NotNil(t, got.Content)
		assert.Equal(t, text, got.Content.Parts[0].Text)
		assert.Equal(t, genai.FinishReasonStop, got.FinishReason)
		assert.True(t, got.TurnComplete)
	})

	t.Run("GenkitLLMAdapter GenerateContent with tools sets ReturnToolRequests", func(t *testing.T) {
		f := faker.New()
		toolName := "getCurrentTime"
		userMsg := f.Lorem().Word()
		fakeResp := &genkitai.ModelResponse{
			Message: &genkitai.Message{
				Role:    genkitai.RoleModel,
				Content: []*genkitai.Part{genkitai.NewTextPart("response")},
			},
			FinishReason: genkitai.FinishReasonStop,
		}

		var capturedOpts *genkitai.GenerateActionOptions
		deps := makeGenkitAdapterMockDeps(t)
		deps.generateWithRequest = func(
			_ context.Context, _ *genkit.Genkit, opts *genkitai.GenerateActionOptions,
			_ []genkitai.ModelMiddleware, _ genkitai.ModelStreamCallback,
		) (*genkitai.ModelResponse, error) {
			capturedOpts = opts
			return fakeResp, nil
		}
		modelName := f.Lorem().Word() + "-" + f.Lorem().Word()
		factory := NewGenkitLLMAdapterFactory(deps)
		adapter := factory(modelName)

		req := &model.LLMRequest{
			Tools: map[string]any{toolName: map[string]any{"description": "returns current time"}},
			Contents: []*genai.Content{
				{Role: string(genai.RoleUser), Parts: []*genai.Part{{Text: userMsg}}},
			},
		}

		for resp, err := range adapter.GenerateContent(t.Context(), req, false) {
			require.NoError(t, err)
			require.NotNil(t, resp)
		}

		require.NotNil(t, capturedOpts)
		assert.Contains(t, capturedOpts.Tools, toolName)
		assert.True(t, capturedOpts.ReturnToolRequests,
			"ReturnToolRequests must be true when tools present for ADK delegation")
	})

	t.Run("GenkitLLMAdapter GenerateContent streaming yields chunks then final", func(t *testing.T) {
		f := faker.New()
		chunk1Text := f.Lorem().Word()
		chunk2Text := f.Lorem().Word()
		finalText := chunk1Text + chunk2Text
		modelName := f.Lorem().Word() + "-" + f.Lorem().Word()
		userMsg := f.Lorem().Word()
		fakeResp := &genkitai.ModelResponse{
			Message: &genkitai.Message{
				Role:    genkitai.RoleModel,
				Content: []*genkitai.Part{genkitai.NewTextPart(finalText)},
			},
			FinishReason: genkitai.FinishReasonStop,
		}

		var callbackInvoked bool
		deps := makeGenkitAdapterMockDeps(t)
		deps.generateWithRequest = func(
			ctx context.Context, _ *genkit.Genkit, _ *genkitai.GenerateActionOptions,
			_ []genkitai.ModelMiddleware, cb genkitai.ModelStreamCallback,
		) (*genkitai.ModelResponse, error) {
			if cb != nil {
				callbackInvoked = true
				// Simulate streaming: yield two chunks then return final
				_ = cb(ctx, &genkitai.ModelResponseChunk{
					Content: []*genkitai.Part{genkitai.NewTextPart(chunk1Text)},
					Role:    genkitai.RoleModel,
					Index:   0,
				})
				_ = cb(ctx, &genkitai.ModelResponseChunk{
					Content: []*genkitai.Part{genkitai.NewTextPart(chunk2Text)},
					Role:    genkitai.RoleModel,
					Index:   0,
				})
			}
			return fakeResp, nil
		}
		factory := NewGenkitLLMAdapterFactory(deps)
		adapter := factory(modelName)
		req := &model.LLMRequest{
			Contents: []*genai.Content{
				{Role: string(genai.RoleUser), Parts: []*genai.Part{{Text: userMsg}}},
			},
		}

		var responses []*model.LLMResponse
		for resp, err := range adapter.GenerateContent(t.Context(), req, true) {
			require.NoError(t, err)
			require.NotNil(t, resp)
			responses = append(responses, resp)
		}

		require.True(t, callbackInvoked, "streaming callback should have been invoked")
		require.GreaterOrEqual(t, len(responses), 3, "expected at least 2 chunks + 1 final")
		// First two are partial chunks
		assert.True(t, responses[0].Partial, "first chunk should be partial")
		assert.False(t, responses[0].TurnComplete, "first chunk should not be turn complete")
		assert.Equal(t, chunk1Text, responses[0].Content.Parts[0].Text)
		assert.True(t, responses[1].Partial, "second chunk should be partial")
		assert.False(t, responses[1].TurnComplete, "second chunk should not be turn complete")
		assert.Equal(t, chunk2Text, responses[1].Content.Parts[0].Text)
		// Final response
		assert.False(t, responses[2].Partial, "final response should not be partial")
		assert.True(t, responses[2].TurnComplete, "final response should be turn complete")
		assert.Equal(t, finalText, responses[2].Content.Parts[0].Text)
		assert.Equal(t, genai.FinishReasonStop, responses[2].FinishReason)
	})

	t.Run("GenerateContent streaming suppresses tool delta chunks", func(t *testing.T) {
		modelName := fake.Lorem().Word() + "-" + fake.Lorem().Word()
		toolName := "getCurrentTime"
		toolID := "call_stream_tool_1"
		finalResp := &genkitai.ModelResponse{
			Message: &genkitai.Message{
				Role: genkitai.RoleModel,
				Content: []*genkitai.Part{
					genkitai.NewToolRequestPart(&genkitai.ToolRequest{
						Name:  toolName,
						Ref:   toolID,
						Input: map[string]any{},
					}),
				},
			},
			FinishReason: genkitai.FinishReasonStop,
		}

		deps := makeGenkitAdapterMockDeps(t)
		deps.generateWithRequest = func(
			ctx context.Context, _ *genkit.Genkit, _ *genkitai.GenerateActionOptions,
			_ []genkitai.ModelMiddleware, cb genkitai.ModelStreamCallback,
		) (*genkitai.ModelResponse, error) {
			if cb != nil {
				err := cb(ctx, &genkitai.ModelResponseChunk{
					Content: []*genkitai.Part{
						genkitai.NewToolRequestPart(&genkitai.ToolRequest{
							Name:  toolName,
							Ref:   "",
							Input: "{}",
						}),
					},
					Role:  genkitai.RoleModel,
					Index: 0,
				})
				require.NoError(t, err)
			}
			return finalResp, nil
		}

		factory := NewGenkitLLMAdapterFactory(deps)
		adapter := factory(modelName)
		req := &model.LLMRequest{
			Contents: []*genai.Content{
				{Role: string(genai.RoleUser), Parts: []*genai.Part{{Text: "What time is it?"}}},
			},
		}

		var responses []*model.LLMResponse
		for resp, err := range adapter.GenerateContent(t.Context(), req, true) {
			require.NoError(t, err)
			require.NotNil(t, resp)
			responses = append(responses, resp)
		}

		require.Len(t, responses, 1, "partial tool delta should not be emitted")
		require.NotNil(t, responses[0].Content)
		require.Len(t, responses[0].Content.Parts, 1)
		require.NotNil(t, responses[0].Content.Parts[0].FunctionCall)
		assert.Equal(t, toolName, responses[0].Content.Parts[0].FunctionCall.Name)
		assert.Equal(t, toolID, responses[0].Content.Parts[0].FunctionCall.ID)
		assert.Equal(t, map[string]any{}, responses[0].Content.Parts[0].FunctionCall.Args)
		assert.False(t, responses[0].Partial)
		assert.True(t, responses[0].TurnComplete)
	})

	t.Run("GenerateContent streaming stops cleanly when consumer aborts", func(t *testing.T) {
		modelName := fake.Lorem().Word() + "-" + fake.Lorem().Word()
		chunk1Text := fake.Lorem().Word()
		chunk2Text := fake.Lorem().Word()
		finalText := fake.Lorem().Sentence(3)
		finalResp := &genkitai.ModelResponse{
			Message: &genkitai.Message{
				Role:    genkitai.RoleModel,
				Content: []*genkitai.Part{genkitai.NewTextPart(finalText)},
			},
		}

		deps := makeGenkitAdapterMockDeps(t)
		deps.generateWithRequest = func(
			ctx context.Context, _ *genkit.Genkit, _ *genkitai.GenerateActionOptions,
			_ []genkitai.ModelMiddleware, cb genkitai.ModelStreamCallback,
		) (*genkitai.ModelResponse, error) {
			if cb != nil {
				if err := cb(ctx, &genkitai.ModelResponseChunk{
					Content: []*genkitai.Part{genkitai.NewTextPart(chunk1Text)},
					Role:    genkitai.RoleModel,
					Index:   0,
				}); err != nil {
					return nil, err
				}
				if err := cb(ctx, &genkitai.ModelResponseChunk{
					Content: []*genkitai.Part{genkitai.NewTextPart(chunk2Text)},
					Role:    genkitai.RoleModel,
					Index:   0,
				}); err != nil {
					return nil, err
				}
			}
			return finalResp, nil
		}

		factory := NewGenkitLLMAdapterFactory(deps)
		adapter := factory(modelName)
		req := &model.LLMRequest{
			Contents: []*genai.Content{
				{Role: string(genai.RoleUser), Parts: []*genai.Part{{Text: fake.Lorem().Word()}}},
			},
		}

		collected := make([]*model.LLMResponse, 0, 1)
		for resp, err := range adapter.GenerateContent(t.Context(), req, true) {
			require.NoError(t, err)
			collected = append(collected, resp)
			break
		}

		require.Len(t, collected, 1)
		require.NotNil(t, collected[0].Content)
		assert.Equal(t, chunk1Text, collected[0].Content.Parts[0].Text)
	})

	t.Run("GenerateContent streaming propagates callback context cancellation", func(t *testing.T) {
		modelName := fake.Lorem().Word() + "-" + fake.Lorem().Word()
		deps := makeGenkitAdapterMockDeps(t)
		deps.generateWithRequest = func(
			ctx context.Context, _ *genkit.Genkit, _ *genkitai.GenerateActionOptions,
			_ []genkitai.ModelMiddleware, cb genkitai.ModelStreamCallback,
		) (*genkitai.ModelResponse, error) {
			streamCtx, cancel := context.WithCancel(ctx)
			cancel()

			if cb != nil {
				err := cb(streamCtx, &genkitai.ModelResponseChunk{
					Content: []*genkitai.Part{genkitai.NewTextPart("chunk")},
					Role:    genkitai.RoleModel,
					Index:   0,
				})
				require.ErrorIs(t, err, context.Canceled)
				return nil, err
			}

			return nil, errors.New("callback is required")
		}

		factory := NewGenkitLLMAdapterFactory(deps)
		adapter := factory(modelName)
		req := &model.LLMRequest{
			Contents: []*genai.Content{
				{Role: string(genai.RoleUser), Parts: []*genai.Part{{Text: fake.Lorem().Word()}}},
			},
		}

		var gotErr error
		for _, err := range adapter.GenerateContent(t.Context(), req, true) {
			gotErr = err
		}

		require.Error(t, gotErr)
		require.ErrorContains(t, gotErr, "genkit generation failed")
		require.ErrorIs(t, gotErr, context.Canceled)
	})

	t.Run("GenkitLLMAdapter GenerateContent propagates genkit error", func(t *testing.T) {
		modelName := fake.Lorem().Word() + "-" + fake.Lorem().Word()
		userMsg := fake.Lorem().Word()
		deps := makeGenkitAdapterMockDeps(t)
		deps.generateWithRequest = func(
			_ context.Context, _ *genkit.Genkit, _ *genkitai.GenerateActionOptions,
			_ []genkitai.ModelMiddleware, _ genkitai.ModelStreamCallback,
		) (*genkitai.ModelResponse, error) {
			return nil, errors.New("genkit api error")
		}
		factory := NewGenkitLLMAdapterFactory(deps)
		adapter := factory(modelName)
		req := &model.LLMRequest{
			Contents: []*genai.Content{
				{Role: string(genai.RoleUser), Parts: []*genai.Part{{Text: userMsg}}},
			},
		}

		var gotErr error
		for _, e := range adapter.GenerateContent(t.Context(), req, false) {
			gotErr = e
		}
		require.Error(t, gotErr)
		assert.Contains(t, gotErr.Error(), "genkit generation failed")
		assert.ErrorContains(t, gotErr, "genkit api error")
	})

	t.Run("finish reason mapping", func(t *testing.T) {
		cases := []struct {
			genkit genkitai.FinishReason
			genai  genai.FinishReason
		}{
			{genkitai.FinishReasonStop, genai.FinishReasonStop},
			{genkitai.FinishReasonLength, genai.FinishReasonMaxTokens},
			{genkitai.FinishReasonBlocked, genai.FinishReasonSafety},
			{genkitai.FinishReasonOther, genai.FinishReasonOther},
			{genkitai.FinishReasonUnknown, genai.FinishReasonUnspecified},
		}
		for _, tc := range cases {
			t.Run(string(tc.genkit), func(t *testing.T) {
				text := fake.Letter()
				resp := &genkitai.ModelResponse{
					Message: &genkitai.Message{
						Role:    genkitai.RoleModel,
						Content: []*genkitai.Part{genkitai.NewTextPart(text)},
					},
					FinishReason: tc.genkit,
				}
				got := convertGenkitResponseToADK(resp, false)
				require.NotNil(t, got)
				assert.Equal(t, tc.genai, got.FinishReason)
			})
		}
	})
}
