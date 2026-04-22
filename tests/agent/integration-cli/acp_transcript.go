package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

type acpTranscript struct {
	out io.Writer
	mu  sync.Mutex
}

type acpTranscriptEntry struct {
	Timestamp string `json:"timestamp"`
	Direction string `json:"direction"`
	Envelope  any    `json:"envelope"`
}

func newACPTranscript(out io.Writer) *acpTranscript {
	if out == nil {
		return nil
	}
	return &acpTranscript{out: out}
}

func newACPTranscriptFile(path string) (*acpTranscript, io.Closer, error) {
	if path == "" {
		return nil, nil, nil
	}

	file, err := os.Create(path)
	if err != nil {
		return nil, nil, fmt.Errorf("create transcript file: %w", err)
	}

	return newACPTranscript(file), file, nil
}

func (t *acpTranscript) recordOutgoing(raw []byte) error {
	return t.record("out", raw)
}

func (t *acpTranscript) recordIncoming(raw []byte) error {
	return t.record("in", raw)
}

func (t *acpTranscript) record(direction string, raw []byte) error {
	if t == nil {
		return nil
	}

	var envelope any
	unmarshalErr := json.Unmarshal(raw, &envelope)
	if unmarshalErr != nil {
		envelope = map[string]any{
			"raw": string(raw),
		}
	}

	entry := acpTranscriptEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Direction: direction,
		Envelope:  envelope,
	}

	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal transcript entry: %w", err)
	}
	line = append(line, '\n')

	t.mu.Lock()
	defer t.mu.Unlock()
	_, writeErr := t.out.Write(line)
	if writeErr != nil {
		return fmt.Errorf("write transcript entry: %w", writeErr)
	}
	return nil
}
