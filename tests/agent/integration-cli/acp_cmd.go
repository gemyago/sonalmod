package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
)

type acpCmdArgs struct {
	AgentCommand   string
	AgentArgs      []string
	CWD            string
	Prompt         string
	TranscriptPath string
	LoadSessionID  string
	CancelAfter    time.Duration
}

func buildACPFunc(args *acpCmdArgs) func(cmd *cobra.Command, _ []string) error {
	return func(cmd *cobra.Command, _ []string) error { // coverage-ignore
		return runACP(
			cmd.Context(),
			os.Stdout,
			acpExecuteParams{
				Prompt:       args.Prompt,
				LoadSession:  args.LoadSessionID,
				CancelAfter:  args.CancelAfter,
				AgentCommand: args.AgentCommand,
				AgentArgs:    args.AgentArgs,
				CWD:          args.CWD,
			},
			args.TranscriptPath,
		)
	}
}

func runACP(
	ctx context.Context,
	output io.Writer,
	params acpExecuteParams,
	transcriptPath string,
) error {
	return runACPWithDeps(
		ctx,
		output,
		params,
		transcriptPath,
		newACPTranscriptFile,
		newACPClientForCommand,
	)
}

func runACPWithDeps(
	ctx context.Context,
	output io.Writer,
	params acpExecuteParams,
	transcriptPath string,
	transcriptFactory func(string) (*acpTranscript, io.Closer, error),
	clientFactory func(context.Context, string, []string, string, *acpTranscript) (*acpClient, func(), error),
) error {
	transcript, closer, transcriptErr := transcriptFactory(transcriptPath)
	if transcriptErr != nil {
		return transcriptErr
	}
	if closer != nil {
		defer func() {
			_ = closer.Close()
		}()
	}

	client, closeFn, clientErr := clientFactory(
		ctx,
		params.AgentCommand,
		params.AgentArgs,
		params.CWD,
		transcript,
	)
	if clientErr != nil {
		return clientErr
	}
	defer closeFn()

	result, executeErr := client.execute(ctx, params)
	if executeErr != nil {
		return executeErr
	}

	_, writeSessionErr := fmt.Fprintf(output, "session_id=%s\n", result.SessionID)
	if writeSessionErr != nil {
		return fmt.Errorf("write session id: %w", writeSessionErr)
	}

	if len(result.PromptResult) > 0 {
		compact := normalizePromptResult(result.PromptResult)
		_, writePromptErr := fmt.Fprintf(output, "prompt_result=%s\n", compact)
		if writePromptErr != nil {
			return fmt.Errorf("write prompt result: %w", writePromptErr)
		}
	}

	return nil
}

func normalizePromptResult(raw json.RawMessage) []byte {
	compact := raw
	var asAny any
	unmarshalErr := json.Unmarshal(raw, &asAny)
	if unmarshalErr != nil {
		return compact
	}
	normalized, _ := json.Marshal(asAny)
	return normalized
}
