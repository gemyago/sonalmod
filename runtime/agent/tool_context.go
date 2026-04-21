package agent

import (
	"context"

	"google.golang.org/adk/tool"
)

// ToolContext is the invocation context passed to agent tool handlers. It is
// created only inside this package (ADK bridge or test-only helpers). It embeds
// [context.Context] so a *ToolContext may be passed where a [context.Context] is
// required. ADK-specific state is not exposed on the public type.
type ToolContext struct {
	context.Context

	adk tool.Context // set by the ADK bridge; zero in test-only constructions
}

func newToolContextFromADK(adkCtx tool.Context) *ToolContext {
	if adkCtx == nil {
		panic("agent: internal error: nil tool.Context from ADK bridge")
	}
	return &ToolContext{Context: adkCtx, adk: adkCtx}
}
