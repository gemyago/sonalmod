package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestACPClient(t *testing.T) {
	fake := faker.New()

	t.Run("falls back to session/new when session/load is unsupported", func(t *testing.T) {
		prompt := fake.Lorem().Word()
		loadSessionID := fake.UUID().V4()
		newSession := fake.UUID().V4()
		okMessage := fake.Lorem().Word()
		serverScript := strings.Join([]string{
			`{"jsonrpc":"2.0","id":1,"result":{"capabilities":{"session":{"load":false}}}}`,
			fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"result":{"sessionId":"%s"}}`, newSession),
			fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"result":{"message":"%s"}}`, okMessage),
		}, "\n") + "\n"

		var outbound bytes.Buffer
		client := newACPClient(strings.NewReader(serverScript), &outbound, nil)

		result, err := client.execute(t.Context(), acpExecuteParams{
			Prompt:      prompt,
			LoadSession: loadSessionID,
		})
		require.NoError(t, err)
		assert.Equal(t, newSession, result.SessionID)
		assert.False(t, result.LoadedSession)

		methods := outboundMethods(t, outbound.String())
		assert.Equal(t, []string{"initialize", "session/new", "session/prompt"}, methods)
		initializeRequest := outboundRequestAt(t, outbound.String(), 0)
		params, ok := initializeRequest["params"].(map[string]any)
		require.True(t, ok)
		protocolVersion, ok := params["protocolVersion"].(float64)
		require.True(t, ok)
		assert.Equal(t, acpProtocolVersion, int(protocolVersion))
	})

	t.Run("uses session/load when capability is advertised", func(t *testing.T) {
		prompt := fake.Lorem().Word()
		loadSessionID := fake.UUID().V4()
		loadedFlag := fake.Lorem().Word()
		serverScript := strings.Join([]string{
			`{"jsonrpc":"2.0","id":1,"result":{"capabilities":{"session":{"load":true}}}}`,
			fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"result":{"loaded":"%s"}}`, loadedFlag),
			fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"result":{"message":"%s"}}`, fake.Lorem().Word()),
		}, "\n") + "\n"

		var outbound bytes.Buffer
		client := newACPClient(strings.NewReader(serverScript), &outbound, nil)

		result, err := client.execute(t.Context(), acpExecuteParams{
			Prompt:      prompt,
			LoadSession: loadSessionID,
		})
		require.NoError(t, err)
		assert.Equal(t, loadSessionID, result.SessionID)
		assert.True(t, result.LoadedSession)

		methods := outboundMethods(t, outbound.String())
		assert.Equal(t, []string{"initialize", "session/load", "session/prompt"}, methods)
	})

	t.Run("cancel-after emits session/cancel request", func(t *testing.T) {
		prompt := fake.Lorem().Word()
		sessionID := fake.UUID().V4()
		serverScript := strings.Join([]string{
			`{"jsonrpc":"2.0","id":1,"result":{"capabilities":{}}}`,
			fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"result":{"sessionId":"%s"}}`, sessionID),
			`{"jsonrpc":"2.0","id":4,"result":{"canceled":true}}`,
			fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"result":{"message":"%s"}}`, fake.Lorem().Word()),
		}, "\n") + "\n"

		var outbound bytes.Buffer
		client := newACPClient(strings.NewReader(serverScript), &outbound, nil)
		client.sleep = func(_ time.Duration) {}

		_, err := client.execute(t.Context(), acpExecuteParams{
			Prompt:      prompt,
			CancelAfter: time.Millisecond,
		})
		require.NoError(t, err)

		methods := outboundMethods(t, outbound.String())
		assert.Equal(t, []string{"initialize", "session/new", "session/prompt", "session/cancel"}, methods)
	})

	t.Run("outbound requests are newline-delimited JSON", func(t *testing.T) {
		prompt := fake.Lorem().Word()
		sessionID := fake.UUID().V4()
		serverScript := strings.Join([]string{
			`{"jsonrpc":"2.0","id":1,"result":{"capabilities":{}}}`,
			fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"result":{"sessionId":"%s"}}`, sessionID),
			fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"result":{"message":"%s"}}`, fake.Lorem().Word()),
		}, "\n") + "\n"

		var outbound bytes.Buffer
		client := newACPClient(strings.NewReader(serverScript), &outbound, nil)
		_, err := client.execute(t.Context(), acpExecuteParams{Prompt: prompt})
		require.NoError(t, err)

		raw := outbound.String()
		require.True(t, strings.HasSuffix(raw, "\n"))

		for line := range strings.SplitSeq(strings.TrimSpace(raw), "\n") {
			var envelope map[string]any
			require.NoError(t, json.Unmarshal([]byte(line), &envelope))
			assert.Equal(t, "2.0", envelope["jsonrpc"])
		}
	})

	t.Run("waitForResponse handles notifications and out-of-order responses", func(t *testing.T) {
		serverScript := strings.Join([]string{
			`{"jsonrpc":"2.0","method":"session/updated","params":{"ok":true}}`,
			`{"jsonrpc":"2.0","id":2,"result":{"ok":2}}`,
			`{"jsonrpc":"2.0","id":1,"result":{"ok":1}}`,
		}, "\n") + "\n"
		client := newACPClient(strings.NewReader(serverScript), &bytes.Buffer{}, nil)

		first, firstErr := client.waitForResponse(t.Context(), 1)
		require.NoError(t, firstErr)
		assert.JSONEq(t, `{"ok":1}`, string(first.Result))

		second, secondErr := client.waitForResponse(t.Context(), 2)
		require.NoError(t, secondErr)
		assert.JSONEq(t, `{"ok":2}`, string(second.Result))
	})

	t.Run("waitForResponse returns ACP errors", func(t *testing.T) {
		serverScript := fmt.Sprintf(
			`{"jsonrpc":"2.0","id":1,"error":{"code":-1,"message":"%s"}}`,
			fake.Lorem().Word(),
		) + "\n"
		client := newACPClient(strings.NewReader(serverScript), &bytes.Buffer{}, nil)

		_, err := client.waitForResponse(t.Context(), 1)
		require.Error(t, err)
		require.ErrorContains(t, err, "ACP error response")
	})

	t.Run("sendRequest respects canceled context", func(t *testing.T) {
		client := newACPClient(strings.NewReader(""), &bytes.Buffer{}, nil)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err := client.sendRequest(ctx, "initialize", map[string]any{})
		require.Error(t, err)
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("sendRequest propagates writer failures", func(t *testing.T) {
		client := newACPClient(strings.NewReader(""), &errorWriter{err: errors.New(fake.Lorem().Word())}, nil)
		_, err := client.sendRequest(t.Context(), "initialize", map[string]any{})
		require.Error(t, err)
		require.ErrorContains(t, err, "write initialize request")
	})

	t.Run("sendRequest returns marshal errors", func(t *testing.T) {
		client := newACPClient(strings.NewReader(""), &bytes.Buffer{}, nil)
		_, err := client.sendRequest(t.Context(), "initialize", map[string]any{
			"bad": func() {},
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "marshal initialize request")
	})

	t.Run("sendRequest returns transcript write errors", func(t *testing.T) {
		client := newACPClient(strings.NewReader(""), &bytes.Buffer{}, newACPTranscript(&errorWriter{
			err: errors.New(fake.Lorem().Word()),
		}))
		_, err := client.sendRequest(t.Context(), "initialize", map[string]any{})
		require.Error(t, err)
		require.ErrorContains(t, err, "record outgoing initialize request")
	})

	t.Run("waitForResponse handles EOF and decode errors", func(t *testing.T) {
		eofClient := newACPClient(strings.NewReader(""), &bytes.Buffer{}, nil)
		_, eofErr := eofClient.waitForResponse(t.Context(), 1)
		require.Error(t, eofErr)
		require.ErrorContains(t, eofErr, "read ACP response")

		badClient := newACPClient(strings.NewReader("not-json\n"), &bytes.Buffer{}, nil)
		_, badErr := badClient.waitForResponse(t.Context(), 1)
		require.Error(t, badErr)
		require.ErrorContains(t, badErr, "decode ACP message")
	})

	t.Run("waitForResponse retries on empty lines", func(t *testing.T) {
		client := newACPClient(
			strings.NewReader("\n\n{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"ok\":true}}\n"),
			&bytes.Buffer{},
			nil,
		)
		resp, err := client.waitForResponse(t.Context(), 1)
		require.NoError(t, err)
		assert.JSONEq(t, `{"ok":true}`, string(resp.Result))
	})

	t.Run("waitForResponse can stop on canceled context", func(t *testing.T) {
		client := newACPClient(strings.NewReader(""), &bytes.Buffer{}, nil)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err := client.waitForResponse(ctx, 1)
		require.Error(t, err)
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("readIncomingEnvelope returns transcript errors", func(t *testing.T) {
		client := newACPClient(
			strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}\n"),
			&bytes.Buffer{},
			newACPTranscript(&errorWriter{err: errors.New(fake.Lorem().Word())}),
		)
		_, err := client.readIncomingEnvelope()
		require.Error(t, err)
		require.ErrorContains(t, err, "record incoming ACP message")
	})

	t.Run("execute returns load-session errors", func(t *testing.T) {
		prompt := fake.Lorem().Word()
		loadSessionID := fake.UUID().V4()
		serverScript := strings.Join([]string{
			`{"jsonrpc":"2.0","id":1,"result":{"capabilities":{"session":{"load":true}}}}`,
			fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"error":{"code":-1,"message":"%s"}}`, fake.Lorem().Word()),
		}, "\n") + "\n"
		client := newACPClient(strings.NewReader(serverScript), &bytes.Buffer{}, nil)
		_, err := client.execute(t.Context(), acpExecuteParams{
			Prompt:      prompt,
			LoadSession: loadSessionID,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "load ACP session")
	})

	t.Run("execute returns create-session errors", func(t *testing.T) {
		prompt := fake.Lorem().Word()
		serverScript := strings.Join([]string{
			`{"jsonrpc":"2.0","id":1,"result":{"capabilities":{}}}`,
			fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"error":{"code":-1,"message":"%s"}}`, fake.Lorem().Word()),
		}, "\n") + "\n"
		client := newACPClient(strings.NewReader(serverScript), &bytes.Buffer{}, nil)
		_, err := client.execute(t.Context(), acpExecuteParams{Prompt: prompt})
		require.Error(t, err)
		require.ErrorContains(t, err, "create ACP session")
	})

	t.Run("execute returns prompt-send errors", func(t *testing.T) {
		prompt := fake.Lorem().Word()
		sessionID := fake.UUID().V4()
		serverScript := strings.Join([]string{
			`{"jsonrpc":"2.0","id":1,"result":{"capabilities":{}}}`,
			fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"result":{"sessionId":"%s"}}`, sessionID),
		}, "\n") + "\n"
		client := newACPClient(strings.NewReader(serverScript), &failAfterWriter{
			remaining: 2,
			err:       errors.New(fake.Lorem().Word()),
		}, nil)
		_, err := client.execute(t.Context(), acpExecuteParams{Prompt: prompt})
		require.Error(t, err)
		require.ErrorContains(t, err, "send session/prompt")
	})

	t.Run("execute returns prompt-wait errors", func(t *testing.T) {
		prompt := fake.Lorem().Word()
		sessionID := fake.UUID().V4()
		serverScript := strings.Join([]string{
			`{"jsonrpc":"2.0","id":1,"result":{"capabilities":{}}}`,
			fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"result":{"sessionId":"%s"}}`, sessionID),
		}, "\n") + "\n"
		client := newACPClient(strings.NewReader(serverScript), &bytes.Buffer{}, nil)
		_, err := client.execute(t.Context(), acpExecuteParams{Prompt: prompt})
		require.Error(t, err)
		require.ErrorContains(t, err, "wait for session/prompt response")
	})

	t.Run("execute returns cancel errors", func(t *testing.T) {
		prompt := fake.Lorem().Word()
		sessionID := fake.UUID().V4()
		serverScript := strings.Join([]string{
			`{"jsonrpc":"2.0","id":1,"result":{"capabilities":{}}}`,
			fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"result":{"sessionId":"%s"}}`, sessionID),
			fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"result":{"message":"%s"}}`, fake.Lorem().Word()),
		}, "\n") + "\n"
		client := newACPClient(strings.NewReader(serverScript), &failAfterWriter{
			remaining: 3,
			err:       errors.New(fake.Lorem().Word()),
		}, nil)
		client.sleep = func(_ time.Duration) {}
		_, err := client.execute(t.Context(), acpExecuteParams{
			Prompt:      prompt,
			CancelAfter: time.Millisecond,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "send session/cancel")
	})

	t.Run("normalizeJSONRPCID supports numbers strings and passthrough", func(t *testing.T) {
		assert.Equal(t, "3", normalizeJSONRPCID(json.RawMessage(`3`)))
		assert.Equal(t, "abc", normalizeJSONRPCID(json.RawMessage(`"abc"`)))
		assert.Equal(t, "{}", normalizeJSONRPCID(json.RawMessage(`{}`)))
	})

	t.Run("extractCapabilities returns empty on malformed payload", func(t *testing.T) {
		assert.Empty(t, extractCapabilities(json.RawMessage(`{`)))
		assert.Empty(t, extractCapabilities(json.RawMessage(`{"x":1}`)))
	})

	t.Run("supportsSessionLoad supports direct flag", func(t *testing.T) {
		assert.True(t, supportsSessionLoad(map[string]any{"sessionLoad": true}))
		assert.False(t, supportsSessionLoad(map[string]any{"sessionLoad": false}))
		assert.False(t, supportsSessionLoad(map[string]any{}))
		assert.False(t, supportsSessionLoad(map[string]any{"session": "bad"}))
		assert.False(t, supportsSessionLoad(map[string]any{"session": map[string]any{}}))
	})

	t.Run("extractSessionID supports alternative keys and errors when missing", func(t *testing.T) {
		sessionID := fake.UUID().V4()
		fallbackID := fake.UUID().V4()
		id, err := extractSessionID(json.RawMessage(fmt.Sprintf(`{"session_id":"%s"}`, sessionID)))
		require.NoError(t, err)
		assert.Equal(t, sessionID, id)

		id, err = extractSessionID(json.RawMessage(fmt.Sprintf(`{"id":"%s"}`, fallbackID)))
		require.NoError(t, err)
		assert.Equal(t, fallbackID, id)

		_, err = extractSessionID(json.RawMessage(`{"nope":1}`))
		require.Error(t, err)

		_, err = extractSessionID(json.RawMessage(`{`))
		require.Error(t, err)
	})

	t.Run("acpRPCError implements error string", func(t *testing.T) {
		code := fake.IntBetween(1, 99)
		message := fake.Lorem().Word()
		assert.Equal(
			t,
			fmt.Sprintf("code=%d message=%s", code, message),
			(&acpRPCError{Code: code, Message: message}).Error(),
		)
	})
}

func outboundMethods(t *testing.T, outbound string) []string {
	t.Helper()
	methods := make([]string, 0)
	for line := range strings.SplitSeq(strings.TrimSpace(outbound), "\n") {
		var envelope map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &envelope))
		method, ok := envelope["method"].(string)
		require.True(t, ok)
		methods = append(methods, method)
	}
	return methods
}

func outboundRequestAt(t *testing.T, outbound string, index int) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(outbound), "\n")
	require.Greater(t, len(lines), index)
	var envelope map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[index]), &envelope))
	return envelope
}

func TestNewACPClientForCommand(t *testing.T) {
	fake := faker.New()
	randomMissingCommand := "missing-" + fake.Lorem().Word()

	t.Run("empty command is rejected", func(t *testing.T) {
		_, _, err := newACPClientForCommand(t.Context(), "   ", nil, "", nil)
		require.Error(t, err)
		require.ErrorContains(t, err, "agent command is required")
	})

	t.Run("unknown command returns start error", func(t *testing.T) {
		_, _, err := newACPClientForCommand(t.Context(), randomMissingCommand, nil, "", nil)
		require.Error(t, err)
	})

	t.Run("command can start and close", func(t *testing.T) {
		ctx := t.Context()
		if _, lookErr := exec.LookPath("cat"); lookErr != nil {
			t.Skip("cat not available on this platform")
		}

		client, closeFn, err := newACPClientForCommand(ctx, "cat", nil, "", nil)
		require.NoError(t, err)
		require.NotNil(t, client)
		require.NotNil(t, closeFn)

		_, sendErr := client.sendRequest(ctx, "initialize", map[string]any{})
		require.NoError(t, sendErr)
		closeFn()
	})

	t.Run("invalid cwd is returned by command startup", func(t *testing.T) {
		ctx := t.Context()
		if _, lookErr := exec.LookPath("cat"); lookErr != nil {
			t.Skip("cat not available on this platform")
		}

		_, _, err := newACPClientForCommand(ctx, "cat", nil, filepath.Join(t.TempDir(), "missing"), nil)
		require.Error(t, err)
	})

	t.Run("closeFn kills long-running process after timeout", func(t *testing.T) {
		if _, lookErr := exec.LookPath("sleep"); lookErr != nil {
			t.Skip("sleep not available on this platform")
		}

		client, closeFn, err := newACPClientForCommand(t.Context(), "sleep", []string{"60"}, "", nil)
		require.NoError(t, err)
		require.NotNil(t, client)

		start := time.Now()
		closeFn()
		assert.Less(t, time.Since(start), 5*time.Second)
	})
}

func TestValidateEnvelope(t *testing.T) {
	okEnvelope := acpIncomingEnvelope{Result: json.RawMessage(`{"ok":true}`)}
	received, err := validateEnvelope(1, okEnvelope)
	require.NoError(t, err)
	assert.Equal(t, okEnvelope, received)

	_, err = validateEnvelope(1, acpIncomingEnvelope{
		Error: &acpRPCError{Code: -1, Message: "boom"},
	})
	require.Error(t, err)
}

func TestACPReadRetryError(t *testing.T) {
	assert.Equal(t, "retry read", (acpReadRetryError{}).Error())
}

type errorWriter struct {
	err error
}

func (w *errorWriter) Write(_ []byte) (int, error) {
	if w.err == nil {
		return 0, io.ErrClosedPipe
	}
	return 0, w.err
}

type failAfterWriter struct {
	remaining int
	err       error
}

func (w *failAfterWriter) Write(p []byte) (int, error) {
	if w.remaining <= 0 {
		return 0, w.err
	}
	w.remaining--
	return len(p), nil
}
