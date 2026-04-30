package main

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
	"sync"
	"time"
)

const acpProcessWaitTimeout = 2 * time.Second
const acpProtocolVersion = 1

type acpExecuteParams struct {
	Prompt       string
	LoadSession  string
	CancelAfter  time.Duration
	AgentCommand string
	AgentArgs    []string
	CWD          string
}

type acpExecuteResult struct {
	SessionID     string
	PromptResult  json.RawMessage
	Capabilities  map[string]any
	LoadedSession bool
}

type acpClient struct {
	reader     *bufio.Reader
	writer     io.Writer
	transcript *acpTranscript

	writeMu   sync.Mutex
	nextID    int64
	pendingBy map[string]acpIncomingEnvelope
	sleep     func(time.Duration)
}

type acpIncomingEnvelope struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Result json.RawMessage `json:"result"`
	Error  *acpRPCError    `json:"error"`
}

type acpRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func newACPClient(reader io.Reader, writer io.Writer, transcript *acpTranscript) *acpClient {
	return &acpClient{
		reader:     bufio.NewReader(reader),
		writer:     writer,
		transcript: transcript,
		nextID:     1,
		pendingBy:  map[string]acpIncomingEnvelope{},
		sleep:      time.Sleep,
	}
}

func newACPClientForCommand(
	ctx context.Context,
	command string,
	args []string,
	cwd string,
	transcript *acpTranscript,
) (*acpClient, func(), error) {
	if strings.TrimSpace(command) == "" {
		return nil, nil, errors.New("agent command is required")
	}

	cmd := exec.CommandContext(ctx, command, args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	cmd.Stderr = os.Stderr

	stdin, openStdinErr := cmd.StdinPipe()
	if openStdinErr != nil {
		return nil, nil, fmt.Errorf("open ACP agent stdin: %w", openStdinErr)
	}
	stdout, openStdoutErr := cmd.StdoutPipe()
	if openStdoutErr != nil {
		return nil, nil, fmt.Errorf("open ACP agent stdout: %w", openStdoutErr)
	}
	startErr := cmd.Start()
	if startErr != nil {
		return nil, nil, fmt.Errorf("start ACP agent command: %w", startErr)
	}

	closeFn := func() {
		_ = stdin.Close()
		waitDone := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(waitDone)
		}()
		select {
		case <-waitDone:
		case <-time.After(acpProcessWaitTimeout):
			_ = cmd.Process.Kill()
			<-waitDone
		}
	}

	return newACPClient(stdout, stdin, transcript), closeFn, nil
}

func (c *acpClient) execute(ctx context.Context, params acpExecuteParams) (acpExecuteResult, error) {
	result := acpExecuteResult{}

	initializeResp, initializeErr := c.call(ctx, "initialize", map[string]any{
		"protocolVersion": acpProtocolVersion,
	})
	if initializeErr != nil {
		return result, fmt.Errorf("initialize ACP session: %w", initializeErr)
	}
	capabilities := extractCapabilities(initializeResp.Result)
	result.Capabilities = capabilities

	if params.LoadSession != "" && supportsSessionLoad(capabilities) {
		loadRespErr := c.loadSession(ctx, params.LoadSession)
		if loadRespErr != nil {
			return result, fmt.Errorf("load ACP session: %w", loadRespErr)
		}
		result.SessionID = params.LoadSession
		result.LoadedSession = true
	} else {
		newSessionID, createErr := c.createSession(ctx, params.CWD)
		if createErr != nil {
			return result, fmt.Errorf("create ACP session: %w", createErr)
		}
		result.SessionID = newSessionID
	}

	promptID, sendPromptErr := c.sendRequest(ctx, "session/prompt", map[string]any{
		"sessionId": result.SessionID,
		"prompt": []map[string]string{
			{
				"type": "text",
				"text": params.Prompt,
			},
		},
	})
	if sendPromptErr != nil {
		return result, fmt.Errorf("send session/prompt: %w", sendPromptErr)
	}

	if params.CancelAfter > 0 {
		cancelErr := c.cancelSession(ctx, result.SessionID, params.CancelAfter)
		if cancelErr != nil {
			return result, fmt.Errorf("send session/cancel: %w", cancelErr)
		}
	}

	promptResp, waitPromptErr := c.waitForResponse(ctx, promptID)
	if waitPromptErr != nil {
		return result, fmt.Errorf("wait for session/prompt response: %w", waitPromptErr)
	}
	result.PromptResult = promptResp.Result

	return result, nil
}

func (c *acpClient) loadSession(ctx context.Context, sessionID string) error {
	_, loadErr := c.call(ctx, "session/load", map[string]any{
		"sessionId": sessionID,
	})
	return loadErr
}

func (c *acpClient) createSession(ctx context.Context, cwd string) (string, error) {
	sessionCWD := strings.TrimSpace(cwd)
	if sessionCWD == "" {
		currentDir, wdErr := os.Getwd()
		if wdErr != nil {
			return "", fmt.Errorf("determine working directory: %w", wdErr)
		}
		sessionCWD = currentDir
	}

	newResp, callErr := c.call(ctx, "session/new", map[string]any{
		"cwd":        sessionCWD,
		"mcpServers": []any{},
	})
	if callErr != nil {
		return "", callErr
	}
	return extractSessionID(newResp.Result)
}

func (c *acpClient) cancelSession(ctx context.Context, sessionID string, delay time.Duration) error {
	c.sleep(delay)
	cancelID, cancelSendErr := c.sendRequest(ctx, "session/cancel", map[string]any{
		"sessionId": sessionID,
	})
	if cancelSendErr != nil {
		return cancelSendErr
	}
	_, cancelWaitErr := c.waitForResponse(ctx, cancelID)
	return cancelWaitErr
}

func (c *acpClient) call(ctx context.Context, method string, params any) (acpIncomingEnvelope, error) {
	requestID, sendErr := c.sendRequest(ctx, method, params)
	if sendErr != nil {
		return acpIncomingEnvelope{}, sendErr
	}
	return c.waitForResponse(ctx, requestID)
}

func (c *acpClient) sendRequest(ctx context.Context, method string, params any) (int64, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	requestID := c.nextID
	c.nextID++

	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      requestID,
		"method":  method,
		"params":  params,
	}
	raw, marshalErr := json.Marshal(req)
	if marshalErr != nil {
		return 0, fmt.Errorf("marshal %s request: %w", method, marshalErr)
	}

	recordErr := c.transcript.recordOutgoing(raw)
	if recordErr != nil {
		return 0, fmt.Errorf("record outgoing %s request: %w", method, recordErr)
	}

	raw = append(raw, '\n')
	_, writeErr := c.writer.Write(raw)
	if writeErr != nil {
		return 0, fmt.Errorf("write %s request: %w", method, writeErr)
	}

	return requestID, nil
}

func (c *acpClient) waitForResponse(ctx context.Context, requestID int64) (acpIncomingEnvelope, error) {
	key := strconv.FormatInt(requestID, 10)

	pending, pendingExists := c.pendingBy[key]
	if pendingExists {
		delete(c.pendingBy, key)
		return validateEnvelope(requestID, pending)
	}

	for {
		select {
		case <-ctx.Done():
			return acpIncomingEnvelope{}, ctx.Err()
		default:
		}

		envelope, readErr := c.readIncomingEnvelope()
		if readErr != nil {
			var retryErr acpReadRetryError
			if errors.As(readErr, &retryErr) {
				continue
			}
			return acpIncomingEnvelope{}, readErr
		}
		if len(envelope.ID) == 0 {
			continue
		}

		receivedID := normalizeJSONRPCID(envelope.ID)
		if receivedID == key {
			return validateEnvelope(requestID, envelope)
		}
		c.pendingBy[receivedID] = envelope
	}
}

func (c *acpClient) readIncomingEnvelope() (acpIncomingEnvelope, error) {
	line, readErr := c.reader.ReadBytes('\n')
	if readErr != nil {
		return acpIncomingEnvelope{}, fmt.Errorf("read ACP response: %w", readErr)
	}

	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return acpIncomingEnvelope{}, acpReadRetryError{}
	}

	recordErr := c.transcript.recordIncoming(line)
	if recordErr != nil {
		return acpIncomingEnvelope{}, fmt.Errorf("record incoming ACP message: %w", recordErr)
	}

	var envelope acpIncomingEnvelope
	unmarshalErr := json.Unmarshal(line, &envelope)
	if unmarshalErr != nil {
		return acpIncomingEnvelope{}, fmt.Errorf("decode ACP message: %w", unmarshalErr)
	}

	return envelope, nil
}

type acpReadRetryError struct{}

func (acpReadRetryError) Error() string {
	return "retry read"
}

func validateEnvelope(requestID int64, envelope acpIncomingEnvelope) (acpIncomingEnvelope, error) {
	if envelope.Error != nil {
		return acpIncomingEnvelope{}, fmt.Errorf("ACP error response for %d: %w", requestID, envelope.Error)
	}
	return envelope, nil
}

func normalizeJSONRPCID(raw json.RawMessage) string {
	var asInt int64
	intErr := json.Unmarshal(raw, &asInt)
	if intErr == nil {
		return strconv.FormatInt(asInt, 10)
	}
	var asString string
	stringErr := json.Unmarshal(raw, &asString)
	if stringErr == nil {
		return asString
	}
	return string(raw)
}

func (e *acpRPCError) Error() string {
	return fmt.Sprintf("code=%d message=%s", e.Code, e.Message)
}

func extractCapabilities(raw json.RawMessage) map[string]any {
	var payload map[string]any
	unmarshalErr := json.Unmarshal(raw, &payload)
	if unmarshalErr != nil {
		return map[string]any{}
	}
	caps, ok := payload["capabilities"].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return caps
}

func supportsSessionLoad(capabilities map[string]any) bool {
	if len(capabilities) == 0 {
		return false
	}

	direct, hasDirect := capabilities["sessionLoad"].(bool)
	if hasDirect {
		return direct
	}
	sessionCaps, hasSessionCaps := capabilities["session"].(map[string]any)
	if !hasSessionCaps {
		return false
	}

	loadEnabled, hasLoad := sessionCaps["load"].(bool)
	if hasLoad {
		return loadEnabled
	}
	sessionLoadEnabled, hasSessionLoad := sessionCaps["session/load"].(bool)
	if hasSessionLoad {
		return sessionLoadEnabled
	}
	return false
}

func extractSessionID(raw json.RawMessage) (string, error) {
	var payload map[string]any
	unmarshalErr := json.Unmarshal(raw, &payload)
	if unmarshalErr != nil {
		return "", fmt.Errorf("decode result: %w", unmarshalErr)
	}
	for _, key := range []string{"sessionId", "session_id", "id"} {
		value, ok := payload[key].(string)
		if ok && value != "" {
			return value, nil
		}
	}
	return "", errors.New("session/new result missing sessionId")
}
