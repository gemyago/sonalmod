package codinglane

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenCodeACPClient(t *testing.T) {
	fake := faker.New()

	makeRequest := func() OpenCodeACPLaunchRequest {
		return OpenCodeACPLaunchRequest{
			AgentCommand: OpenCodeAgentCommand{
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
		assertOpenCodeACPErrorKind(t, err, OpenCodeACPErrorKindProtocol)
	})

	t.Run("missing session id response returns protocol errors", func(t *testing.T) {
		t.Setenv("SONALMOD_ACP_HELPER_MODE", "missing-session-id")
		t.Setenv("SONALMOD_ACP_HELPER_METHODS_LOG", filepath.Join(t.TempDir(), "methods.log"))

		client := NewOpenCodeACPClient()
		_, err := client.Launch(t.Context(), makeRequest())
		require.Error(t, err)
		assertOpenCodeACPErrorKind(t, err, OpenCodeACPErrorKindProtocol)
	})
}

func TestOpenCodeACPClientHelperProcess(t *testing.T) {
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
			fmt.Fprintf(os.Stdout, "{\"jsonrpc\":\"2.0\",\"error\":{\"code\":-32700,\"message\":\"%s\"}}\n", err.Error())
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
					"update": map[string]any{"type": "progress", "message": "thinking"},
				})
				writeNotification("session/update", map[string]any{
					"sessionId": "session-1",
					"update": map[string]any{"type": "final", "message": "done"},
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

func assertOpenCodeACPErrorKind(t *testing.T, err error, kind OpenCodeACPErrorKind) {
	t.Helper()

	var acpErr *OpenCodeACPError
	require.ErrorAs(t, err, &acpErr)
	assert.Equal(t, kind, acpErr.Kind)
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
