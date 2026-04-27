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

	"github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
)

const (
	openCodeACPProtocolVersion  = 1
	openCodeACPProcessWaitLimit = 2 * time.Second
	openCodeACPUpdateBufferSize = 4
	openCodeACPScannerBufSize   = 64 * 1024
	openCodeACPScannerMaxSize   = 4 * 1024 * 1024
)

// ACPStdioErrorKind classifies ACP stdio launch failure categories.
type ACPStdioErrorKind string

const (
	// ACPStdioErrorKindValidation indicates invalid launch input.
	ACPStdioErrorKindValidation ACPStdioErrorKind = "validation"
	// ACPStdioErrorKindSubprocess indicates subprocess startup or I/O failures.
	ACPStdioErrorKindSubprocess ACPStdioErrorKind = "subprocess"
	// ACPStdioErrorKindProtocol indicates malformed/invalid ACP protocol responses.
	ACPStdioErrorKindProtocol ACPStdioErrorKind = "protocol"
)

// ACPStdioError wraps launch failures with a stable kind.
type ACPStdioError struct {
	Kind ACPStdioErrorKind
	Op   string
	Err  error
}

func (e *ACPStdioError) Error() string {
	return fmt.Sprintf("acp stdio %s (%s): %v", e.Op, e.Kind, e.Err)
}

func (e *ACPStdioError) Unwrap() error {
	return e.Err
}

// ACPStdioLaunchRequest defines data required to launch an ACP stdio run.
type ACPStdioLaunchRequest struct {
	AgentCommand agentprofiles.ACPStdioAgentCommand
	CWD          string
	Prompt       string
	MCPServers   []any
}

// ACPStdioUpdate contains a parsed session/update notification.
type ACPStdioUpdate struct {
	SessionID string
	Type      string
	Payload   json.RawMessage
}

// ACPStdioLaunchResult contains session metadata and prompt result.
type ACPStdioLaunchResult struct {
	SessionID    string
	PromptResult json.RawMessage
	Updates      []ACPStdioUpdate
}

// OpenCodeACPClient executes the validated OpenCode ACP launch subset over stdio.
type OpenCodeACPClient struct{}

// NewOpenCodeACPClient creates a client that launches OpenCode ACP subprocesses.
func NewOpenCodeACPClient() *OpenCodeACPClient {
	return &OpenCodeACPClient{}
}

func wrapACPStdioError(kind ACPStdioErrorKind, op string, err error) error {
	if err == nil {
		return nil
	}
	return &ACPStdioError{Kind: kind, Op: op, Err: err}
}

type acpStdioResolvedLaunchRequest struct {
	Command    agentprofiles.ACPStdioAgentCommand
	CWD        string
	Prompt     string
	MCPServers []any
}

type acpStdioSubprocess struct {
	stdin io.WriteCloser
	cmd   *exec.Cmd
	out   io.Reader
}

func (p *acpStdioSubprocess) close() {
	_ = p.stdin.Close()
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- p.cmd.Wait()
	}()
	select {
	case <-waitDone:
	case <-time.After(openCodeACPProcessWaitLimit):
		_ = p.cmd.Process.Kill()
		<-waitDone
	}
}

func (c *OpenCodeACPClient) Launch(
	ctx context.Context,
	request ACPStdioLaunchRequest,
) (*ACPStdioLaunchResult, error) {
	resolved, err := resolveACPStdioLaunchRequest(request)
	if err != nil {
		return nil, err
	}

	process, err := startACPStdioSubprocess(ctx, resolved)
	if err != nil {
		return nil, err
	}
	defer process.close()

	return executeACPStdioProtocol(ctx, process, resolved)
}

func resolveACPStdioLaunchRequest(
	request ACPStdioLaunchRequest,
) (acpStdioResolvedLaunchRequest, error) {
	command, err := normalizeACPStdioAgentCommand(request.AgentCommand)
	if err != nil {
		return acpStdioResolvedLaunchRequest{}, wrapACPStdioError(
			ACPStdioErrorKindValidation,
			"validate-agent-command",
			err,
		)
	}

	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" {
		return acpStdioResolvedLaunchRequest{}, wrapACPStdioError(
			ACPStdioErrorKindValidation,
			"validate-prompt",
			errors.New("prompt is required"),
		)
	}

	cwd := strings.TrimSpace(request.CWD)
	if cwd == "" {
		cwd, err = os.Getwd()
		if err != nil {
			return acpStdioResolvedLaunchRequest{}, wrapACPStdioError(
				ACPStdioErrorKindSubprocess,
				"resolve-working-directory",
				fmt.Errorf("determine working directory: %w", err),
			)
		}
	}

	mcpServers := request.MCPServers
	if mcpServers == nil {
		mcpServers = []any{}
	}

	return acpStdioResolvedLaunchRequest{
		Command:    command,
		CWD:        cwd,
		Prompt:     prompt,
		MCPServers: mcpServers,
	}, nil
}

func startACPStdioSubprocess(
	ctx context.Context,
	request acpStdioResolvedLaunchRequest,
) (*acpStdioSubprocess, error) {
	// #nosec G204 -- command/args are validated persisted defaults from trusted runtime config.
	cmd := exec.CommandContext(ctx, request.Command.Command, request.Command.Args...)
	cmd.Dir = request.CWD
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, wrapACPStdioError(
			ACPStdioErrorKindSubprocess,
			"open-stdin",
			fmt.Errorf("open ACP stdin: %w", err),
		)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, wrapACPStdioError(
			ACPStdioErrorKindSubprocess,
			"open-stdout",
			fmt.Errorf("open ACP stdout: %w", err),
		)
	}

	if err = cmd.Start(); err != nil {
		return nil, wrapACPStdioError(
			ACPStdioErrorKindSubprocess,
			"start-subprocess",
			fmt.Errorf("start ACP subprocess: %w", err),
		)
	}

	return &acpStdioSubprocess{stdin: stdin, out: stdout, cmd: cmd}, nil
}

func executeACPStdioProtocol(
	ctx context.Context,
	process *acpStdioSubprocess,
	request acpStdioResolvedLaunchRequest,
) (*ACPStdioLaunchResult, error) {
	client := newOpenCodeACPWireClient(process.out, process.stdin)

	if err := initializeACPStdio(ctx, client); err != nil {
		return nil, err
	}

	sessionID, err := createACPStdioSession(ctx, client, request)
	if err != nil {
		return nil, err
	}

	promptResult, updates, err := promptACPStdioSession(ctx, client, sessionID, request.Prompt)
	if err != nil {
		return nil, err
	}

	return &ACPStdioLaunchResult{
		SessionID:    sessionID,
		PromptResult: promptResult,
		Updates:      updates,
	}, nil
}

func initializeACPStdio(ctx context.Context, client *openCodeACPWireClient) error {
	initializeResp, err := client.call(ctx, "initialize", map[string]any{
		"protocolVersion": openCodeACPProtocolVersion,
	}, nil)
	if err != nil {
		return wrapACPStdioError(ACPStdioErrorKindProtocol, "initialize", err)
	}

	if _, err = jsonRawObject(initializeResp.Result, "initialize result"); err != nil {
		return wrapACPStdioError(ACPStdioErrorKindProtocol, "initialize", err)
	}

	return nil
}

func createACPStdioSession(
	ctx context.Context,
	client *openCodeACPWireClient,
	request acpStdioResolvedLaunchRequest,
) (string, error) {
	newSessionResp, err := client.call(ctx, "session/new", map[string]any{
		"cwd":        request.CWD,
		"mcpServers": request.MCPServers,
	}, nil)
	if err != nil {
		return "", wrapACPStdioError(ACPStdioErrorKindProtocol, "session/new", err)
	}

	sessionID, err := extractOpenCodeSessionID(newSessionResp.Result)
	if err != nil {
		return "", wrapACPStdioError(ACPStdioErrorKindProtocol, "session/new", err)
	}

	return sessionID, nil
}

func promptACPStdioSession(
	ctx context.Context,
	client *openCodeACPWireClient,
	sessionID string,
	prompt string,
) (json.RawMessage, []ACPStdioUpdate, error) {
	updates := make([]ACPStdioUpdate, 0, openCodeACPUpdateBufferSize)
	promptResp, err := client.call(ctx, "session/prompt", map[string]any{
		"sessionId": sessionID,
		"prompt": []map[string]string{{
			"type": "text",
			"text": prompt,
		}},
	}, func(env openCodeACPEnvelope) error {
		if env.Method != "session/update" {
			return nil
		}

		update, parseErr := parseACPStdioSessionUpdate(env.Params)
		if parseErr != nil {
			return parseErr
		}
		updates = append(updates, update)
		return nil
	})
	if err != nil {
		return nil, nil, wrapACPStdioError(ACPStdioErrorKindProtocol, "session/prompt", err)
	}

	return promptResp.Result, updates, nil
}

type openCodeACPWireClient struct {
	scanner *bufio.Scanner
	writer  io.Writer
	nextID  int64
}

func newOpenCodeACPWireClient(reader io.Reader, writer io.Writer) *openCodeACPWireClient {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, openCodeACPScannerBufSize), openCodeACPScannerMaxSize)
	return &openCodeACPWireClient{
		scanner: scanner,
		writer:  writer,
		nextID:  1,
	}
}

type openCodeACPEnvelope struct {
	ID     json.RawMessage      `json:"id"`
	Method string               `json:"method"`
	Params json.RawMessage      `json:"params"`
	Result json.RawMessage      `json:"result"`
	Error  *openCodeACPAPIError `json:"error"`
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

func parseACPStdioSessionUpdate(params json.RawMessage) (ACPStdioUpdate, error) {
	paramsObj, err := jsonRawObject(params, "session/update params")
	if err != nil {
		return ACPStdioUpdate{}, err
	}

	sessionID, _ := paramsObj["sessionId"].(string)
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ACPStdioUpdate{}, errors.New("session/update params missing sessionId")
	}

	updateValue, ok := paramsObj["update"]
	if !ok {
		return ACPStdioUpdate{}, errors.New("session/update params missing update")
	}
	updateRaw, err := json.Marshal(updateValue)
	if err != nil {
		return ACPStdioUpdate{}, fmt.Errorf("marshal session/update payload: %w", err)
	}
	updateObj, err := jsonRawObject(updateRaw, "session/update payload")
	if err != nil {
		return ACPStdioUpdate{}, err
	}

	updateType, _ := updateObj["type"].(string)
	updateType = strings.TrimSpace(updateType)
	if updateType == "" {
		return ACPStdioUpdate{}, errors.New("session/update payload missing type")
	}

	return ACPStdioUpdate{
		SessionID: sessionID,
		Type:      updateType,
		Payload:   updateRaw,
	}, nil
}

func normalizeACPStdioAgentCommand(
	command agentprofiles.ACPStdioAgentCommand,
) (agentprofiles.ACPStdioAgentCommand, error) {
	command.Command = strings.TrimSpace(command.Command)
	if command.Command == "" {
		return agentprofiles.ACPStdioAgentCommand{}, errors.New("agent_command.command is required")
	}
	if strings.ContainsAny(command.Command, "\n\r\t") {
		return agentprofiles.ACPStdioAgentCommand{}, errors.New(
			"agent_command.command contains control characters",
		)
	}

	if command.Args == nil {
		command.Args = []string{}
	}
	for idx := range command.Args {
		command.Args[idx] = strings.TrimSpace(command.Args[idx])
		if command.Args[idx] == "" {
			return agentprofiles.ACPStdioAgentCommand{}, errors.New(
				"agent_command.args must not contain empty values",
			)
		}
		if strings.ContainsAny(command.Args[idx], "\n\r\t") {
			return agentprofiles.ACPStdioAgentCommand{}, errors.New(
				"agent_command.args contain control characters",
			)
		}
	}
	if hasDuplicates(command.Args) {
		return agentprofiles.ACPStdioAgentCommand{}, errors.New("agent_command.args must be unique")
	}

	return command, nil
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
