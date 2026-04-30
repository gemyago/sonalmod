package acpstdio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenCodeACPClient(t *testing.T) {
	fake := faker.New()

	makeRequest := func() LaunchRequest {
		return LaunchRequest{
			AgentCommand: agentprofiles.ACPStdioAgentCommand{
				Command: os.Args[0],
				Args: []string{
					"-test.run=TestOpenCodeACPClientHelperProcess",
					"--",
				},
			},
			CWD:    t.TempDir(),
			Prompt: fake.Lorem().Sentence(8),
		}
	}

	t.Run("performs initialize new prompt and consumes session update messages", func(t *testing.T) {
		methodsLog := filepath.Join(t.TempDir(), "methods.log")
		t.Setenv("SONALMOD_ACP_HELPER_MODE", "success")
		t.Setenv("SONALMOD_ACP_HELPER_METHODS_LOG", methodsLog)

		client := NewOpenCodeACPClient()
		result, err := client.Launch(t.Context(), makeRequest())
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.NotEmpty(t, result.SessionID)
		assert.JSONEq(t, `{"ok":true}`, string(result.PromptResult))
		require.Len(t, result.Updates, 2)
		assert.Equal(t, "progress", result.Updates[0].Type)
		assert.Equal(t, "final", result.Updates[1].Type)
	})

	t.Run("does not call unsupported session methods", func(t *testing.T) {
		methodsLog := filepath.Join(t.TempDir(), "methods.log")
		t.Setenv("SONALMOD_ACP_HELPER_MODE", "success")
		t.Setenv("SONALMOD_ACP_HELPER_METHODS_LOG", methodsLog)

		client := NewOpenCodeACPClient()
		_, err := client.Launch(t.Context(), makeRequest())
		require.NoError(t, err)

		raw, err := os.ReadFile(methodsLog)
		require.NoError(t, err)
		methods := strings.Fields(string(raw))
		assert.Equal(t, []string{"initialize", "session/new", "session/prompt"}, methods)
		forbidden := map[string]struct{}{
			"session/cancel": {},
			"session/close":  {},
			"session/load":   {},
			"session/list":   {},
		}
		for _, method := range methods {
			_, bad := forbidden[method]
			assert.False(t, bad, "unexpected method %s", method)
		}
	})

	t.Run("malformed responses return protocol errors", func(t *testing.T) {
		t.Setenv("SONALMOD_ACP_HELPER_MODE", "bad-initialize")
		t.Setenv("SONALMOD_ACP_HELPER_METHODS_LOG", filepath.Join(t.TempDir(), "methods.log"))

		client := NewOpenCodeACPClient()
		_, err := client.Launch(t.Context(), makeRequest())
		require.Error(t, err)
		assertLaunchErrorKind(t, err, LaunchErrorKindProtocol)
	})

	t.Run("missing session id response returns protocol errors", func(t *testing.T) {
		t.Setenv("SONALMOD_ACP_HELPER_MODE", "missing-session-id")
		t.Setenv("SONALMOD_ACP_HELPER_METHODS_LOG", filepath.Join(t.TempDir(), "methods.log"))

		client := NewOpenCodeACPClient()
		_, err := client.Launch(t.Context(), makeRequest())
		require.Error(t, err)
		assertLaunchErrorKind(t, err, LaunchErrorKindProtocol)
	})

	t.Run("validation and subprocess startup errors return typed kinds", func(t *testing.T) {
		client := NewOpenCodeACPClient()

		_, err := client.Launch(t.Context(), LaunchRequest{
			AgentCommand: agentprofiles.ACPStdioAgentCommand{},
			Prompt:       "run",
		})
		require.Error(t, err)
		assertLaunchErrorKind(t, err, LaunchErrorKindValidation)

		_, err = client.Launch(t.Context(), LaunchRequest{
			AgentCommand: agentprofiles.ACPStdioAgentCommand{Command: os.Args[0], Args: []string{"-test.run=Nope"}},
			Prompt:       " ",
		})
		require.Error(t, err)
		assertLaunchErrorKind(t, err, LaunchErrorKindValidation)

		_, err = client.Launch(t.Context(), LaunchRequest{
			AgentCommand: agentprofiles.ACPStdioAgentCommand{Command: "/no/such/opencode-binary"},
			Prompt:       "run",
		})
		require.Error(t, err)
		assertLaunchErrorKind(t, err, LaunchErrorKindSubprocess)
	})
}

func TestOpenCodeACPClientHelperProcess(_ *testing.T) {
	if os.Getenv("SONALMOD_ACP_HELPER_MODE") == "" {
		return
	}

	mode := os.Getenv("SONALMOD_ACP_HELPER_MODE")
	methodsLog := os.Getenv("SONALMOD_ACP_HELPER_METHODS_LOG")
	_ = os.WriteFile(methodsLog, []byte(""), 0600)

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var req map[string]any
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			fmt.Fprintf(
				os.Stdout,
				"{\"jsonrpc\":\"2.0\",\"error\":{\"code\":-32700,\"message\":\"%s\"}}\n",
				err.Error(),
			)
			continue
		}

		id := req["id"]
		method, _ := req["method"].(string)
		if method != "" {
			f, err := os.OpenFile(methodsLog, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
			if err == nil {
				_, _ = f.WriteString(method + "\n")
				_ = f.Close()
			}
		}

		switch mode {
		case "success":
			switch method {
			case "initialize":
				writeResult(id, map[string]any{"capabilities": map[string]any{}})
			case "session/new":
				writeResult(id, map[string]any{"sessionId": "session-1"})
			case "session/prompt":
				writeNotification("session/update", map[string]any{
					"sessionId": "session-1",
					"update":    map[string]any{"type": "progress", "message": "thinking"},
				})
				writeNotification("session/update", map[string]any{
					"sessionId": "session-1",
					"update":    map[string]any{"type": "final", "message": "done"},
				})
				writeResult(id, map[string]any{"ok": true})
			default:
				writeError(id, -32601, "Method not found")
			}
		case "bad-initialize":
			if method == "initialize" {
				writeResult(id, "bad-result")
				continue
			}
			writeError(id, -32601, "Method not found")
		case "missing-session-id":
			switch method {
			case "initialize":
				writeResult(id, map[string]any{"capabilities": map[string]any{}})
			case "session/new":
				writeResult(id, map[string]any{"ok": true})
			default:
				writeError(id, -32601, "Method not found")
			}
		default:
			writeError(id, -32603, "Unknown helper mode")
		}
	}

	os.Exit(0)
}

func assertLaunchErrorKind(t *testing.T, err error, kind LaunchErrorKind) {
	t.Helper()

	var acpErr *LaunchError
	require.ErrorAs(t, err, &acpErr)
	assert.Equal(t, kind, acpErr.Kind)
}

func TestOpenCodeACPClientInternalHelpers(t *testing.T) {
	t.Run("error wrapper and unwrapping behavior", func(t *testing.T) {
		require.NoError(t, wrapLaunchError(LaunchErrorKindProtocol, "x", nil))

		sourceErr := errors.New("source")
		wrapped := wrapLaunchError(LaunchErrorKindProtocol, "initialize", sourceErr)
		require.Error(t, wrapped)

		var acpErr *LaunchError
		require.ErrorAs(t, wrapped, &acpErr)
		assert.Equal(t, LaunchErrorKindProtocol, acpErr.Kind)
		require.ErrorIs(t, wrapped, sourceErr)
		assert.Contains(t, acpErr.Error(), "initialize")
	})

	t.Run("resolve request applies cwd and mcp defaults", func(t *testing.T) {
		resolved, err := resolveACPLaunchRequest(LaunchRequest{
			AgentCommand: agentprofiles.ACPStdioAgentCommand{Command: "opencode", Args: []string{"acp"}},
			Prompt:       "run tests",
		})
		require.NoError(t, err)
		assert.NotEmpty(t, resolved.CWD)
		assert.NotNil(t, resolved.MCPServers)
		assert.Equal(t, "run tests", resolved.Prompt)
	})

	t.Run("wire write request error branches", func(t *testing.T) {
		client := newOpenCodeACPWireClient(strings.NewReader(""), &bytes.Buffer{})

		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		err := client.writeRequest(ctx, 1, "initialize", map[string]any{})
		require.ErrorIs(t, err, context.Canceled)

		err = client.writeRequest(t.Context(), 1, "initialize", map[string]any{"bad": func() {}})
		require.Error(t, err)
		require.ErrorContains(t, err, "marshal initialize request")

		client = newOpenCodeACPWireClient(strings.NewReader(""), &errorWriter{err: errors.New("write failed")})
		err = client.writeRequest(t.Context(), 1, "initialize", map[string]any{})
		require.Error(t, err)
		assert.ErrorContains(t, err, "write initialize request")
	})

	t.Run("wire read envelope branches", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		client := newOpenCodeACPWireClient(strings.NewReader(""), &bytes.Buffer{})
		_, err := client.readEnvelope(ctx)
		require.ErrorIs(t, err, context.Canceled)

		client = newOpenCodeACPWireClient(strings.NewReader(""), &bytes.Buffer{})
		_, err = client.readEnvelope(t.Context())
		require.Error(t, err)
		require.ErrorContains(t, err, "EOF")

		client = newOpenCodeACPWireClient(strings.NewReader("not-json\n"), &bytes.Buffer{})
		_, err = client.readEnvelope(t.Context())
		require.Error(t, err)
		require.ErrorContains(t, err, "decode ACP message")

		oversized := strings.Repeat("a", openCodeACPScannerMaxSize+1) + "\n"
		client = newOpenCodeACPWireClient(strings.NewReader(oversized), &bytes.Buffer{})
		_, err = client.readEnvelope(t.Context())
		require.Error(t, err)
		require.ErrorContains(t, err, "read ACP message")

		client = newOpenCodeACPWireClient(
			strings.NewReader("\n\n{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"ok\":true}}\n"),
			&bytes.Buffer{},
		)
		env, err := client.readEnvelope(t.Context())
		require.NoError(t, err)
		assert.JSONEq(t, `{"ok":true}`, string(env.Result))
	})

	t.Run("call validates response ids and payloads", func(t *testing.T) {
		reader := strings.NewReader(
			"{\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{\"ok\":true}}\n",
		)
		client := newOpenCodeACPWireClient(reader, &bytes.Buffer{})
		_, err := client.call(t.Context(), "initialize", map[string]any{}, nil)
		require.Error(t, err)
		require.ErrorContains(t, err, "unexpected response id")

		reader = strings.NewReader(
			"{\"jsonrpc\":\"2.0\",\"id\":1,\"error\":{\"code\":-1,\"message\":\"boom\"}}\n",
		)
		client = newOpenCodeACPWireClient(reader, &bytes.Buffer{})
		_, err = client.call(t.Context(), "initialize", map[string]any{}, nil)
		require.Error(t, err)
		require.ErrorContains(t, err, "ACP error response")

		reader = strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":1}\n")
		client = newOpenCodeACPWireClient(reader, &bytes.Buffer{})
		_, err = client.call(t.Context(), "initialize", map[string]any{}, nil)
		require.Error(t, err)
		require.ErrorContains(t, err, "missing result payload")

		reader = strings.NewReader(
			"{\"jsonrpc\":\"2.0\",\"method\":\"session/update\",\"params\":{\"sessionId\":\"s\",\"update\":{\"type\":\"progress\"}}}\n" +
				"{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"ok\":true}}\n",
		)
		client = newOpenCodeACPWireClient(reader, &bytes.Buffer{})
		notificationCount := 0
		_, err = client.call(t.Context(), "initialize", map[string]any{}, func(_ openCodeACPEnvelope) error {
			notificationCount++
			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, 1, notificationCount)

		reader = strings.NewReader(
			"{\"jsonrpc\":\"2.0\",\"method\":\"session/update\",\"params\":{\"sessionId\":\"s\",\"update\":{\"type\":\"progress\"}}}\n",
		)
		client = newOpenCodeACPWireClient(reader, &bytes.Buffer{})
		_, err = client.call(t.Context(), "initialize", map[string]any{}, func(_ openCodeACPEnvelope) error {
			return errors.New("stop")
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "stop")
	})

	t.Run("json and update parsing helper branches", func(t *testing.T) {
		assert.Equal(t, "3", normalizeOpenCodeRPCID(json.RawMessage(`3`)))
		assert.Equal(t, "abc", normalizeOpenCodeRPCID(json.RawMessage(`"abc"`)))
		assert.Equal(t, "{}", normalizeOpenCodeRPCID(json.RawMessage(`{}`)))

		_, err := jsonRawObject(json.RawMessage(`\"x\"`), "payload")
		require.Error(t, err)

		_, err = extractOpenCodeSessionID(json.RawMessage(`{"ok":true}`))
		require.Error(t, err)
		_, err = extractOpenCodeSessionID(json.RawMessage(`"x"`))
		require.Error(t, err)

		_, err = parseACPSessionUpdate(json.RawMessage(`{"update":{"type":"progress"}}`))
		require.Error(t, err)
		_, err = parseACPSessionUpdate(json.RawMessage(`{"sessionId":"s"}`))
		require.Error(t, err)
		_, err = parseACPSessionUpdate(json.RawMessage(`{"sessionId":"s","update":{"x":"y"}}`))
		require.Error(t, err)

		_, err = jsonRawObject(json.RawMessage(`null`), "payload")
		require.Error(t, err)
	})

	t.Run("normalize ACP stdio agent command trims and validates arguments", func(t *testing.T) {
		normalized, err := normalizeACPAgentCommand(agentprofiles.ACPStdioAgentCommand{
			Command: "  opencode  ",
			Args:    nil,
		})
		require.NoError(t, err)
		assert.Equal(t, "opencode", normalized.Command)
		assert.Equal(t, []string{}, normalized.Args)

		_, err = normalizeACPAgentCommand(agentprofiles.ACPStdioAgentCommand{
			Command: "opencode",
			Args:    []string{"dup", "dup"},
		})
		require.ErrorContains(t, err, "must be unique")

		_, err = normalizeACPAgentCommand(agentprofiles.ACPStdioAgentCommand{
			Command: "opencode",
			Args:    []string{"bad\targ"},
		})
		require.ErrorContains(t, err, "contain control characters")
	})
}

func writeResult(id any, result any) {
	resp := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}
	raw, _ := json.Marshal(resp)
	_, _ = os.Stdout.Write(append(raw, '\n'))
}

func writeError(id any, code int, message string) {
	resp := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	}
	raw, _ := json.Marshal(resp)
	_, _ = os.Stdout.Write(append(raw, '\n'))
}

func writeNotification(method string, params any) {
	resp := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	raw, _ := json.Marshal(resp)
	_, _ = os.Stdout.Write(append(raw, '\n'))
}

type errorWriter struct {
	err error
}

func (w *errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

var _ io.Writer = (*errorWriter)(nil)
