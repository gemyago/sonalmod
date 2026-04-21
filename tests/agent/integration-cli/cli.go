package main

import (
	"context"
	"fmt"
	"io"

	"github.com/gemyago/sonalmod/runtime/agent"
	uuid "github.com/gofrs/uuid/v5"
)

// Default matches apps/sonal-ui/src/pages/Chat.svelte (import.meta.env.VITE_AGENT_USER_ID ?? 'dev-user').
const agentUserID = "dev-user"

type cliParams struct {
	Prompt    string
	SessionID string
	Model     string
}

func runCLI(
	ctx context.Context,
	runner agent.AgentRunner,
	params cliParams,
	output io.Writer,
) error {
	if params.SessionID == "" {
		params.SessionID = uuid.Must(uuid.NewV4()).String()
	}
	runParams := agent.NewRunParams(agentUserID, params.SessionID, params.Model).WithText(params.Prompt)
	result, err := runner.Run(ctx, runParams)
	if err != nil {
		return err
	}
	if streamErr := streamAgentOutput(ctx, output, result); streamErr != nil {
		return streamErr
	}
	_, err = fmt.Fprintf(output, "\n\nTo continue: -s %s\n", result.SessionID())
	return err
}
