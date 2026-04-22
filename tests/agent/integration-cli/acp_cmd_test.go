package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/jaswdr/faker/v2"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestACPCmd(t *testing.T) {
	fake := faker.New()

	makeClient := func(script string) *acpClient {
		return newACPClient(bytes.NewBufferString(script), &bytes.Buffer{}, nil)
	}
	noTranscript := func(_ string) (*acpTranscript, io.Closer, error) {
		return nil, nil, nil
	}
	makeServerScript := func(sessionID string, promptResult int) string {
		return `{"jsonrpc":"2.0","id":1,"result":{"capabilities":{}}}` + "\n" +
			fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"result":{"sessionId":"%s"}}`, sessionID) + "\n" +
			fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"result":{"value":%d}}`, promptResult) + "\n"
	}

	t.Run("buildACPFunc delegates to runACP", func(t *testing.T) {
		args := &acpCmdArgs{
			AgentCommand: "missing-" + fake.Lorem().Word(),
			Prompt:       fake.Lorem().Word(),
		}
		runE := buildACPFunc(args)
		cmd := &cobra.Command{}
		cmd.SetContext(t.Context())

		err := runE(cmd, nil)
		require.Error(t, err)
	})

	t.Run("runACPWithDeps success writes session and prompt result", func(t *testing.T) {
		sessionID := fake.UUID().V4()
		prompt := fake.Lorem().Word()
		promptResult := fake.IntBetween(1, 999)
		clientFactory := func(
			_ context.Context,
			_ string,
			_ []string,
			_ string,
			_ *acpTranscript,
		) (*acpClient, func(), error) {
			return makeClient(makeServerScript(sessionID, promptResult)), func() {}, nil
		}

		var out bytes.Buffer
		err := runACPWithDeps(t.Context(), &out, acpExecuteParams{
			AgentCommand: "ignored-" + fake.Lorem().Word(),
			Prompt:       prompt,
		}, "", noTranscript, clientFactory)
		require.NoError(t, err)
		assert.Contains(t, out.String(), "session_id="+sessionID)
		assert.Contains(t, out.String(), fmt.Sprintf(`prompt_result={"value":%d}`, promptResult))
	})

	t.Run("runACPWithDeps transcript factory errors are returned", func(t *testing.T) {
		expectedErr := fake.Lorem().Word()
		transcriptFactory := func(_ string) (*acpTranscript, io.Closer, error) {
			return nil, nil, errors.New(expectedErr)
		}
		err := runACPWithDeps(t.Context(), &bytes.Buffer{}, acpExecuteParams{}, "", transcriptFactory, nil)
		require.Error(t, err)
		require.ErrorContains(t, err, expectedErr)
	})

	t.Run("runACPWithDeps client factory errors are returned", func(t *testing.T) {
		expectedErr := fake.Lorem().Word()
		clientFactory := func(
			_ context.Context,
			_ string,
			_ []string,
			_ string,
			_ *acpTranscript,
		) (*acpClient, func(), error) {
			return nil, nil, errors.New(expectedErr)
		}

		err := runACPWithDeps(t.Context(), &bytes.Buffer{}, acpExecuteParams{}, "", noTranscript, clientFactory)
		require.Error(t, err)
		require.ErrorContains(t, err, expectedErr)
	})

	t.Run("runACPWithDeps execute failures are returned", func(t *testing.T) {
		clientFactory := func(
			_ context.Context,
			_ string,
			_ []string,
			_ string,
			_ *acpTranscript,
		) (*acpClient, func(), error) {
			return makeClient(""), func() {}, nil
		}

		err := runACPWithDeps(t.Context(), &bytes.Buffer{}, acpExecuteParams{}, "", noTranscript, clientFactory)
		require.Error(t, err)
	})

	t.Run("runACPWithDeps writer errors are surfaced", func(t *testing.T) {
		sessionID := fake.UUID().V4()
		promptResult := fake.IntBetween(1, 999)
		clientFactory := func(
			_ context.Context,
			_ string,
			_ []string,
			_ string,
			_ *acpTranscript,
		) (*acpClient, func(), error) {
			return makeClient(makeServerScript(sessionID, promptResult)), func() {}, nil
		}

		err := runACPWithDeps(t.Context(), &errorWriter{err: errors.New(fake.Lorem().Word())}, acpExecuteParams{
			AgentCommand: "ignored-" + fake.Lorem().Word(),
			Prompt:       fake.Lorem().Word(),
		}, "", noTranscript, clientFactory)
		require.Error(t, err)
		require.ErrorContains(t, err, "write session id")
	})

	t.Run("runACPWithDeps prompt write errors are surfaced", func(t *testing.T) {
		sessionID := fake.UUID().V4()
		promptResult := fake.IntBetween(1, 999)
		clientFactory := func(
			_ context.Context,
			_ string,
			_ []string,
			_ string,
			_ *acpTranscript,
		) (*acpClient, func(), error) {
			return makeClient(makeServerScript(sessionID, promptResult)), func() {}, nil
		}

		writer := &failAfterWriterCmd{remaining: 1, err: errors.New(fake.Lorem().Word())}
		err := runACPWithDeps(t.Context(), writer, acpExecuteParams{
			AgentCommand: "ignored-" + fake.Lorem().Word(),
			Prompt:       fake.Lorem().Word(),
		}, "", noTranscript, clientFactory)
		require.Error(t, err)
		require.ErrorContains(t, err, "write prompt result")
	})

	t.Run("runACPWithDeps transcript closer is invoked", func(t *testing.T) {
		closed := false
		transcriptFactory := func(_ string) (*acpTranscript, io.Closer, error) {
			return nil, closerFunc(func() error {
				closed = true
				return nil
			}), nil
		}
		sessionID := fake.UUID().V4()
		promptResult := fake.IntBetween(1, 999)
		clientFactory := func(
			_ context.Context,
			_ string,
			_ []string,
			_ string,
			_ *acpTranscript,
		) (*acpClient, func(), error) {
			return makeClient(makeServerScript(sessionID, promptResult)), func() {}, nil
		}

		err := runACPWithDeps(t.Context(), &bytes.Buffer{}, acpExecuteParams{
			AgentCommand: "ignored-" + fake.Lorem().Word(),
			Prompt:       fake.Lorem().Word(),
		}, fake.Lorem().Word()+".jsonl", transcriptFactory, clientFactory)
		require.NoError(t, err)
		assert.True(t, closed)
	})

	t.Run("runACP uses default deps", func(t *testing.T) {
		err := runACP(t.Context(), &bytes.Buffer{}, acpExecuteParams{}, "")
		require.Error(t, err)
	})

	t.Run("normalizePromptResult compacts valid JSON and preserves invalid", func(t *testing.T) {
		raw := json.RawMessage(fmt.Sprintf(`{ "a": %d }`, fake.IntBetween(1, 999)))
		var payload map[string]int
		require.NoError(t, json.Unmarshal(raw, &payload))
		assert.Equal(t, fmt.Sprintf(`{"a":%d}`, payload["a"]), string(normalizePromptResult(raw)))

		invalid := json.RawMessage(`{`)
		assert.Equal(t, `{`, string(normalizePromptResult(invalid)))
	})
}

type closerFunc func() error

func (f closerFunc) Close() error {
	return f()
}

type failAfterWriterCmd struct {
	remaining int
	err       error
}

func (w *failAfterWriterCmd) Write(p []byte) (int, error) {
	if w.remaining <= 0 {
		return 0, w.err
	}
	w.remaining--
	return len(p), nil
}
