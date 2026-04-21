package internal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"strings"

	genkitai "github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// genkitGenerateWithRequestFunc is the type of genkit.GenerateWithRequest for injection.
type genkitGenerateWithRequestFunc func(
	context.Context, *genkit.Genkit, *genkitai.GenerateActionOptions,
	[]genkitai.ModelMiddleware, genkitai.ModelStreamCallback,
) (*genkitai.ModelResponse, error)

// genkitLLMAdapter adapts genkit.Genkit to implement model.LLM.
//
// Tools limitation: ADK req.Tools is map[string]any (keys = tool names). Genkit opts.Tools
// expects []string (registered tool names). The adapter passes tool names only; tool definitions
// must be registered in Genkit separately. ADK tool schemas (map values) are not forwarded.
type genkitLLMAdapter struct {
	genkit              *genkit.Genkit
	name                string
	logger              *slog.Logger
	generateWithRequest genkitGenerateWithRequestFunc
}

// GenkitLLMAdapterDeps holds dependencies for constructing the adapter.
type GenkitLLMAdapterDeps struct {
	Genkit     *genkit.Genkit
	RootLogger *slog.Logger

	// Used to inject a fake implementation for testing.
	generateWithRequest genkitGenerateWithRequestFunc
}

// GenkitLLMAdapterFactory is a function that creates a GenkitLLMAdapter for a resolved model name.
type GenkitLLMAdapterFactory func(name string) model.LLM

func NewGenkitLLMAdapterFactory(deps GenkitLLMAdapterDeps) GenkitLLMAdapterFactory {
	return func(name string) model.LLM {
		fn := genkit.GenerateWithRequest
		if deps.generateWithRequest != nil {
			fn = deps.generateWithRequest
		}
		return &genkitLLMAdapter{
			genkit:              deps.Genkit,
			name:                name,
			logger:              deps.RootLogger.WithGroup("genkit-adapter"),
			generateWithRequest: fn,
		}
	}
}

// Ensure genkitLLMAdapter implements model.LLM.
var _ model.LLM = (*genkitLLMAdapter)(nil)

// generateContentStream runs streaming generation, yielding partial chunks then the final response.
func (a *genkitLLMAdapter) generateContentStream(
	ctx context.Context,
	opts *genkitai.GenerateActionOptions,
	yield func(*model.LLMResponse, error) bool,
) {
	cb := func(streamCtx context.Context, chunk *genkitai.ModelResponseChunk) error {
		if streamCtx.Err() != nil {
			return streamCtx.Err()
		}
		llmResp := convertGenkitChunkToADK(chunk, true)
		dropToolPartsFromPartialChunk(llmResp)
		if llmResp.Content == nil || len(llmResp.Content.Parts) == 0 {
			return nil
		}
		if !yield(llmResp, nil) {
			return errStreamAborted
		}
		return nil
	}
	resp, err := a.generateWithRequest(ctx, a.genkit, opts, nil, cb)
	if err != nil {
		if errors.Is(err, errStreamAborted) {
			return
		}
		a.logger.ErrorContext(ctx, "genkit streaming generation failed",
			slog.String("error", err.Error()),
		)
		yield(nil, fmt.Errorf("genkit generation failed: %w", err))
		return
	}
	finalResp := convertGenkitResponseToADK(resp, false)
	finalResp.Partial = false
	finalResp.TurnComplete = true
	yield(finalResp, nil)
}

// errStreamAborted is returned from the streaming callback when the consumer stops iterating.
var errStreamAborted = errors.New("stream aborted by consumer")

// Name returns the model name this adapter was constructed with (used when req.Model is empty).
func (a *genkitLLMAdapter) Name() string {
	return a.name
}

// GenerateContent implements model.LLM by converting ADK requests to Genkit calls.
// Non-streaming yields one response; streaming yields partial chunks then a final response.
func (a *genkitLLMAdapter) GenerateContent(
	ctx context.Context,
	req *model.LLMRequest,
	stream bool,
) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		opts := convertADKRequestToGenkitOptions(req)
		if opts.Model == "" {
			opts.Model = a.name
		}

		a.logger.DebugContext(ctx, "GenerateContent",
			slog.Bool("stream", stream),
			slog.String("model", opts.Model),
			slog.Int("messages", len(opts.Messages)),
		)

		if stream {
			a.generateContentStream(ctx, opts, yield)
			return
		}

		resp, err := a.generateWithRequest(ctx, a.genkit, opts, nil, nil)
		if err != nil {
			a.logger.ErrorContext(ctx, "genkit generation failed",
				slog.String("error", err.Error()),
			)
			yield(nil, fmt.Errorf("genkit generation failed: %w", err))
			return
		}

		llmResp := convertGenkitResponseToADK(resp, false)
		yield(llmResp, nil)
	}
}

// hasOnlyFunctionResponseParts returns true if all non-nil parts are FunctionResponse.
func hasOnlyFunctionResponseParts(parts []*genai.Part) bool {
	if len(parts) == 0 {
		return false
	}
	for _, p := range parts {
		if p == nil {
			continue
		}
		if p.FunctionResponse == nil {
			return false
		}
	}
	return true
}

// convertADKRequestToGenkitOptions converts an ADK LLMRequest to Genkit GenerateActionOptions.
// Maps req.Contents to opts.Messages (genai.Content → genkitai.Message), req.Model to opts.Model.
// req.Config.SystemInstruction is mapped to a leading genkit message with RoleSystem (Genkit
// plugins turn that into provider-specific system prompts, e.g. Gemini's SystemInstruction).
// Other req.Config fields are still not passed through; Genkit compat_oai expects
// openai.ChatCompletionNewParams or map[string]any for full config passthrough.
func convertADKRequestToGenkitOptions(req *model.LLMRequest) *genkitai.GenerateActionOptions {
	if req == nil {
		return &genkitai.GenerateActionOptions{}
	}
	opts := &genkitai.GenerateActionOptions{}

	for _, content := range req.Contents {
		if content == nil {
			continue
		}
		role := genkitai.Role(content.Role)
		// OpenAI expects role "tool" for tool response messages. ADK may send "user";
		// normalize so compat_oai produces valid tool messages with tool_call_id.
		if hasOnlyFunctionResponseParts(content.Parts) {
			role = genkitai.RoleTool
		}
		msg := &genkitai.Message{Role: role}
		for _, part := range content.Parts {
			if part == nil {
				continue
			}
			if mapped := convertGenaiPartToGenkit(part); mapped != nil {
				msg.Content = append(msg.Content, mapped)
			}
		}
		opts.Messages = append(opts.Messages, msg)
	}

	if sysMsg := systemInstructionMessageFromGenAIConfig(req.Config); sysMsg != nil {
		opts.Messages = append([]*genkitai.Message{sysMsg}, opts.Messages...)
	}

	if req.Model != "" {
		opts.Model = req.Model
	}
	// Config: ADK passes *genai.GenerateContentConfig. Genkit's compat_oai expects
	// openai.ChatCompletionNewParams or map[string]any. Skip passthrough to avoid
	// "unexpected config type" errors; Genkit uses defaults.
	// Tools: ADK req.Tools is map[string]any (keys = tool names); Genkit opts.Tools is []string.
	// We pass tool names only. Tool definitions must be registered in Genkit separately.
	// See genkit_adapter.go doc for limitations.
	if len(req.Tools) > 0 {
		opts.Tools = make([]string, 0, len(req.Tools))
		for name := range req.Tools {
			opts.Tools = append(opts.Tools, name)
		}
		// Delegate tool execution to ADK so confirmations, callbacks, and session visibility work.
		opts.ReturnToolRequests = true
	}

	return opts
}

// systemInstructionMessageFromGenAIConfig builds a single Genkit system message from ADK's
// GenerateContentConfig.SystemInstruction, or nil if absent or empty.
func systemInstructionMessageFromGenAIConfig(cfg *genai.GenerateContentConfig) *genkitai.Message {
	if cfg == nil || cfg.SystemInstruction == nil {
		return nil
	}
	msg := &genkitai.Message{Role: genkitai.RoleSystem}
	for _, part := range cfg.SystemInstruction.Parts {
		if part == nil {
			continue
		}
		if mapped := convertGenaiPartToGenkit(part); mapped != nil {
			msg.Content = append(msg.Content, mapped)
		}
	}
	if len(msg.Content) == 0 {
		return nil
	}
	return msg
}

// convertGenaiPartToGenkit maps genai.Part to genkitai.Part.
// Text, InlineData, FileData, FunctionCall, and FunctionResponse are supported.
func convertGenaiPartToGenkit(part *genai.Part) *genkitai.Part {
	if part.Text != "" {
		return genkitai.NewTextPart(part.Text)
	}
	if part.InlineData != nil {
		// Genkit NewMediaPart expects (mimeType, contents string); use base64 for binary.
		mime := part.InlineData.MIMEType
		if mime == "" {
			mime = "application/octet-stream"
		}
		return genkitai.NewMediaPart(mime, base64.StdEncoding.EncodeToString(part.InlineData.Data))
	}
	if part.FunctionCall != nil {
		input := part.FunctionCall.Args
		if input == nil {
			input = map[string]any{}
		}
		return genkitai.NewToolRequestPart(&genkitai.ToolRequest{
			Name:  part.FunctionCall.Name,
			Input: input,
			Ref:   part.FunctionCall.ID,
		})
	}
	if part.FunctionResponse != nil {
		output := part.FunctionResponse.Response
		if output == nil {
			output = map[string]any{}
		}
		return genkitai.NewToolResponsePart(&genkitai.ToolResponse{
			Name:   part.FunctionResponse.Name,
			Output: output,
			Ref:    part.FunctionResponse.ID,
		})
	}
	if part.FileData != nil {
		return genkitai.NewResourcePart(part.FileData.FileURI)
	}
	return nil
}

// convertGenkitChunkToADK converts a Genkit ModelResponseChunk to ADK LLMResponse.
// When partial is true, sets Partial=true and TurnComplete=false (streaming chunk).
// When partial is false, sets Partial=false and TurnComplete=true (final chunk).
func convertGenkitChunkToADK(chunk *genkitai.ModelResponseChunk, partial bool) *model.LLMResponse {
	out := &model.LLMResponse{
		Partial:        partial,
		TurnComplete:   !partial,
		Interrupted:    false,
		CustomMetadata: make(map[string]any),
	}
	if chunk == nil {
		return out
	}
	out.Content = &genai.Content{
		Role: string(chunk.Role),
	}
	for _, part := range chunk.Content {
		if mapped := convertGenkitPartToGenai(part); mapped != nil {
			out.Content.Parts = append(out.Content.Parts, mapped)
		}
	}
	return out
}

func dropToolPartsFromPartialChunk(resp *model.LLMResponse) {
	if resp == nil || resp.Content == nil {
		return
	}

	filtered := make([]*genai.Part, 0, len(resp.Content.Parts))
	for _, part := range resp.Content.Parts {
		if part == nil {
			continue
		}
		if part.FunctionCall != nil || part.FunctionResponse != nil {
			continue
		}
		filtered = append(filtered, part)
	}

	resp.Content.Parts = filtered
}

// convertGenkitResponseToADK converts a Genkit ModelResponse to ADK LLMResponse.
// Maps resp.Message to genai.Content, genkitai.Part to genai.Part (Text supported; ToolRequest,
// ToolResponse stubbed), resp.FinishReason to genai.FinishReason, and sets UsageMetadata,
// CitationMetadata, GroundingMetadata when available. Partial, TurnComplete, and
// Interrupted are set based on stream mode and response state.
func convertGenkitResponseToADK(resp *genkitai.ModelResponse, stream bool) *model.LLMResponse {
	_ = stream // API parity with ADK; non-streaming path does not vary by this flag today
	out := &model.LLMResponse{
		Partial:        false,
		TurnComplete:   true,
		Interrupted:    false,
		CustomMetadata: make(map[string]any),
	}
	if resp == nil {
		return out
	}

	if resp.Message != nil {
		out.Content = &genai.Content{
			Role: string(resp.Message.Role),
		}
		for _, part := range resp.Message.Content {
			if mapped := convertGenkitPartToGenai(part); mapped != nil {
				out.Content.Parts = append(out.Content.Parts, mapped)
			}
		}
	}

	out.FinishReason = mapGenkitFinishReasonToGenai(resp.FinishReason)

	if resp.Usage != nil {
		//nolint:gosec // G115: token counts from Genkit are bounded in practice
		out.UsageMetadata = &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:     int32(resp.Usage.InputTokens),
			CandidatesTokenCount: int32(resp.Usage.OutputTokens),
			TotalTokenCount:      int32(resp.Usage.TotalTokens),
		}
	}

	if len(resp.Interrupts()) > 0 || resp.FinishReason == genkitai.FinishReasonInterrupted {
		out.Interrupted = true
	}

	return out
}

// convertGenkitPartToGenai maps genkitai.Part to genai.Part.
// Text, ToolRequest (→ FunctionCall), and ToolResponse (→ FunctionResponse) are supported.
func convertGenkitPartToGenai(part *genkitai.Part) *genai.Part {
	if part == nil {
		return nil
	}
	if part.Text != "" {
		return &genai.Part{Text: part.Text}
	}
	if part.IsToolRequest() && part.ToolRequest != nil {
		return &genai.Part{
			FunctionCall: &genai.FunctionCall{
				Name: part.ToolRequest.Name,
				ID:   part.ToolRequest.Ref,
				Args: normalizeToolRequestInputToArgsMap(part.ToolRequest.Input),
			},
		}
	}
	if part.IsToolResponse() && part.ToolResponse != nil {
		resp := part.ToolResponse.Output
		respMap, ok := resp.(map[string]any)
		if !ok || respMap == nil {
			respMap = map[string]any{"output": resp}
		}
		return &genai.Part{
			FunctionResponse: &genai.FunctionResponse{
				Name:     part.ToolResponse.Name,
				ID:       part.ToolResponse.Ref,
				Response: respMap,
			},
		}
	}
	return nil
}

func normalizeToolRequestInputToArgsMap(input any) map[string]any {
	switch v := input.(type) {
	case nil:
		return map[string]any{}
	case map[string]any:
		if v == nil {
			return map[string]any{}
		}
		return v
	case string:
		return parseJSONObjectString(v)
	case []byte:
		return parseJSONObjectBytes(v)
	case json.RawMessage:
		return parseJSONObjectBytes(v)
	default:
		return map[string]any{}
	}
}

func parseJSONObjectString(raw string) map[string]any {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return map[string]any{}
	}

	return parseJSONObjectBytes([]byte(trimmed))
}

func parseJSONObjectBytes(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}

	var argsMap map[string]any
	if err := json.Unmarshal(raw, &argsMap); err != nil || argsMap == nil {
		return map[string]any{}
	}

	return argsMap
}

// mapGenkitFinishReasonToGenai maps Genkit FinishReason to genai.FinishReason.
func mapGenkitFinishReasonToGenai(r genkitai.FinishReason) genai.FinishReason {
	switch r {
	case genkitai.FinishReasonStop:
		return genai.FinishReasonStop
	case genkitai.FinishReasonLength:
		return genai.FinishReasonMaxTokens
	case genkitai.FinishReasonBlocked:
		return genai.FinishReasonSafety
	case genkitai.FinishReasonInterrupted:
		return genai.FinishReasonOther
	case genkitai.FinishReasonOther:
		return genai.FinishReasonOther
	case genkitai.FinishReasonUnknown:
		return genai.FinishReasonUnspecified
	default:
		return genai.FinishReasonUnspecified
	}
}
