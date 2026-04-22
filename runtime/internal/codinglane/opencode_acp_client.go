package codinglane

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	openCodeACPProtocolVersion  = 1
	openCodeACPProcessWaitLimit = 2 * time.Second
)

// OpenCodeACPErrorKind classifies ACP launch failure categories.
type OpenCodeACPErrorKind string

const (
	// OpenCodeACPErrorKindValidation indicates invalid launch input.
	OpenCodeACPErrorKindValidation OpenCodeACPErrorKind = "validation"
	// OpenCodeACPErrorKindSubprocess indicates subprocess startup or I/O failures.
	OpenCodeACPErrorKindSubprocess OpenCodeACPErrorKind = "subprocess"
	// OpenCodeACPErrorKindProtocol indicates malformed/invalid ACP protocol responses.
	OpenCodeACPErrorKindProtocol OpenCodeACPErrorKind = "protocol"
)

// OpenCodeACPError wraps launch failures with a stable kind.
type OpenCodeACPError struct {
	Kind OpenCodeACPErrorKind
	Op   string
	Err  error
}

func (e *OpenCodeACPError) Error() string {
	return fmt.Sprintf("opencode acp %s (%s): %v", e.Op, e.Kind, e.Err)
}

func (e *OpenCodeACPError) Unwrap() error {
	return e.Err
}

// OpenCodeACPLaunchRequest defines data required to launch an ACP coding run.
type OpenCodeACPLaunchRequest struct {
	AgentCommand OpenCodeAgentCommand
	CWD          string
	Prompt       string
	MCPServers   []any
}

// OpenCodeACPUpdate contains a parsed session/update notification.
type OpenCodeACPUpdate struct {
	SessionID string
	Type      string
	Payload   json.RawMessage
}

// OpenCodeACPLaunchResult contains session metadata and prompt result.
type OpenCodeACPLaunchResult struct {
	SessionID    string
	PromptResult json.RawMessage
	Updates      []OpenCodeACPUpdate
}

// OpenCodeACPClient executes the validated OpenCode ACP launch subset over stdio.
type OpenCodeACPClient struct{}

// NewOpenCodeACPClient creates a client that launches OpenCode ACP subprocesses.
func NewOpenCodeACPClient() *OpenCodeACPClient {
	return &OpenCodeACPClient{}
}

func wrapOpenCodeACPError(kind OpenCodeACPErrorKind, op string, err error) error {
	if err == nil {
		return nil
	}
	return &OpenCodeACPError{
		Kind: kind,
		Op:   op,
		Err:  err,
	}
}

func (c *OpenCodeACPClient) Launch(
	ctx context.Context,
	request OpenCodeACPLaunchRequest,
) (*OpenCodeACPLaunchResult, error) {
	normalizedCommand, err := normalizeAgentCommand(request.AgentCommand)
	if err != nil {
		return nil, wrapOpenCodeACPError(OpenCodeACPErrorKindValidation, "validate-agent-command", err)
	}
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" {
		return nil, wrapOpenCodeACPError(
			OpenCodeACPErrorKindValidation,
			"validate-prompt",
			errors.New("prompt is required"),
		)
	}

	cwd := strings.TrimSpace(request.CWD)
	if cwd == "" {
		cwd, err = os.Getwd()
		if err != nil {
			return nil, wrapOpenCodeACPError(
				OpenCodeACPErrorKindSubprocess,
				"resolve-working-directory",
				fmt.Errorf("determine working directory: %w", err),
			)
		}
	}

	mcpServers := request.MCPServers
	if mcpServers == nil {
		mcpServers = []any{}
	}

	cmd := exec.CommandContext(ctx, normalizedCommand.Command, normalizedCommand.Args...)
	cmd.Dir = cwd
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, wrapOpenCodeACPError(
			OpenCodeACPErrorKindSubprocess,
			"open-stdin",
			fmt.Errorf("open ACP stdin: %w", err),
		)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, wrapOpenCodeACPError(
			OpenCodeACPErrorKindSubprocess,
			"open-stdout",
			fmt.Errorf("open ACP stdout: %w", err),
		)
	}
	if err = cmd.Start(); err != nil {
		return nil, wrapOpenCodeACPError(
			OpenCodeACPErrorKindSubprocess,
			"start-subprocess",
			fmt.Errorf("start ACP subprocess: %w", err),
		)
	}

	defer func() {
		_ = stdin.Close()
		waitDone := make(chan error, 1)
		go func() {
			waitDone <- cmd.Wait()
		}()
		select {
		case <-waitDone:
		case <-time.After(openCodeACPProcessWaitLimit):
			_ = cmd.Process.Kill()
			<-waitDone
		}
	}()

	client := newOpenCodeACPWireClient(stdout, stdin)

	initializeResp, err := client.call(ctx, "initialize", map[string]any{
		"protocolVersion": openCodeACPProtocolVersion,
	}, nil)
	if err != nil {
		return nil, wrapOpenCodeACPError(OpenCodeACPErrorKindProtocol, "initialize", err)
	}
	if _, err = jsonRawObject(initializeResp.Result, "initialize result"); err != nil {
		return nil, wrapOpenCodeACPError(OpenCodeACPErrorKindProtocol, "initialize", err)
	}

	newSessionResp, err := client.call(ctx, "session/new", map[string]any{
		"cwd":        cwd,
		"mcpServers": mcpServers,
	}, nil)
	if err != nil {
		return nil, wrapOpenCodeACPError(OpenCodeACPErrorKindProtocol, "session/new", err)
	}
	sessionID, err := extractOpenCodeSessionID(newSessionResp.Result)
	if err != nil {
		return nil, wrapOpenCodeACPError(OpenCodeACPErrorKindProtocol, "session/new", err)
	}

	updates := make([]OpenCodeACPUpdate, 0, 4)
	promptResp, err := client.call(ctx, "session/prompt", map[string]any{
		"sessionId": sessionID,
		"prompt": []map[string]string{
			{
				"type": "text",
				"text": prompt,
			},
		},
	}, func(env openCodeACPEnvelope) error {
		if env.Method != "session/update" {
			return nil
		}
		update, parseErr := parseOpenCodeSessionUpdate(env.Params)
		if parseErr != nil {
			return parseErr
		}
		updates = append(updates, update)
		return nil
	})
	if err != nil {
		return nil, wrapOpenCodeACPError(OpenCodeACPErrorKindProtocol, "session/prompt", err)
	}

	return &OpenCodeACPLaunchResult{
		SessionID:    sessionID,
		PromptResult: promptResp.Result,
		Updates:      updates,
	}, nil
}

type openCodeACPWireClient struct {
	scanner *bufio.Scanner
	writer  io.Writer
	nextID  int64
}

func newOpenCodeACPWireClient(reader io.Reader, writer io.Writer) *openCodeACPWireClient {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	return &openCodeACPWireClient{
		scanner: scanner,
		writer:  writer,
		nextID:  1,
	}
}

type openCodeACPEnvelope struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *openCodeACPAPIError
}

type openCodeACPAPIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *openCodeACPAPIError) Error() string {
	return fmt.Sprintf("code=%d message=%s", e.Code, e.Message)
}

func (c *openCodeACPWireClient) call(
	ctx context.Context,
	method string,
	params any,
	onNotification func(openCodeACPEnvelope) error,
) (openCodeACPEnvelope, error) {
	requestID := c.nextID
	c.nextID++

	if err := c.writeRequest(ctx, requestID, method, params); err != nil {
		return openCodeACPEnvelope{}, err
	}

	for {
		envelope, err := c.readEnvelope(ctx)
		if err != nil {
			return openCodeACPEnvelope{}, err
		}

		if len(envelope.ID) == 0 {
			if onNotification != nil {
				if err = onNotification(envelope); err != nil {
					return openCodeACPEnvelope{}, err
				}
			}
			continue
		}

		if normalizeOpenCodeRPCID(envelope.ID) != strconv.FormatInt(requestID, 10) {
			return openCodeACPEnvelope{}, fmt.Errorf(
				"unexpected response id %s for method %s",
				normalizeOpenCodeRPCID(envelope.ID),
				method,
			)
		}
		if envelope.Error != nil {
			return openCodeACPEnvelope{}, fmt.Errorf("ACP error response: %w", envelope.Error)
		}
		if len(envelope.Result) == 0 {
			return openCodeACPEnvelope{}, fmt.Errorf("method %s missing result payload", method)
		}

		return envelope, nil
	}
}

func (c *openCodeACPWireClient) writeRequest(
	ctx context.Context,
	requestID int64,
	method string,
	params any,
) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	request := map[string]any{
		"jsonrpc": "2.0",
		"id":      requestID,
		"method":  method,
		"params":  params,
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("marshal %s request: %w", method, err)
	}

	if _, err = c.writer.Write(append(raw, '\n')); err != nil {
		return fmt.Errorf("write %s request: %w", method, err)
	}
	return nil
}

func (c *openCodeACPWireClient) readEnvelope(ctx context.Context) (openCodeACPEnvelope, error) {
	for {
		select {
		case <-ctx.Done():
			return openCodeACPEnvelope{}, ctx.Err()
		default:
		}

		if !c.scanner.Scan() {
			if err := c.scanner.Err(); err != nil {
				return openCodeACPEnvelope{}, fmt.Errorf("read ACP message: %w", err)
			}
			return openCodeACPEnvelope{}, errors.New("read ACP message: EOF")
		}

		line := bytes.TrimSpace(c.scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var envelope openCodeACPEnvelope
		if err := json.Unmarshal(line, &envelope); err != nil {
			return openCodeACPEnvelope{}, fmt.Errorf("decode ACP message: %w", err)
		}
		return envelope, nil
	}
}

func normalizeOpenCodeRPCID(raw json.RawMessage) string {
	var asInt int64
	if err := json.Unmarshal(raw, &asInt); err == nil {
		return strconv.FormatInt(asInt, 10)
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString
	}
	return string(raw)
}

func extractOpenCodeSessionID(result json.RawMessage) (string, error) {
	obj, err := jsonRawObject(result, "session/new result")
	if err != nil {
		return "", err
	}
	sessionID, _ := obj["sessionId"].(string)
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", errors.New("session/new result missing sessionId")
	}
	return sessionID, nil
}

func parseOpenCodeSessionUpdate(params json.RawMessage) (OpenCodeACPUpdate, error) {
	paramsObj, err := jsonRawObject(params, "session/update params")
	if err != nil {
		return OpenCodeACPUpdate{}, err
	}

	sessionID, _ := paramsObj["sessionId"].(string)
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return OpenCodeACPUpdate{}, errors.New("session/update params missing sessionId")
	}

	updateValue, ok := paramsObj["update"]
	if !ok {
		return OpenCodeACPUpdate{}, errors.New("session/update params missing update")
	}
	updateRaw, err := json.Marshal(updateValue)
	if err != nil {
		return OpenCodeACPUpdate{}, fmt.Errorf("marshal session/update payload: %w", err)
	}
	updateObj, err := jsonRawObject(updateRaw, "session/update payload")
	if err != nil {
		return OpenCodeACPUpdate{}, err
	}

	updateType, _ := updateObj["type"].(string)
	updateType = strings.TrimSpace(updateType)
	if updateType == "" {
		return OpenCodeACPUpdate{}, errors.New("session/update payload missing type")
	}

	return OpenCodeACPUpdate{
		SessionID: sessionID,
		Type:      updateType,
		Payload:   updateRaw,
	}, nil
}

func jsonRawObject(raw json.RawMessage, description string) (map[string]any, error) {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("%s is not an object: %w", description, err)
	}
	if obj == nil {
		return nil, fmt.Errorf("%s is null", description)
	}
	return obj, nil
}
