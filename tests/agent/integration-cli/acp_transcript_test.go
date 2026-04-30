package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestACPTranscriptSuite(t *testing.T) {
	fake := faker.New()

	t.Run("records outbound and inbound envelopes as JSONL", func(t *testing.T) {
		method := fake.Lorem().Word()
		sessionID := fake.UUID().V4()
		resultKey := fake.Lorem().Word()
		resultValue := fake.IntBetween(1, 9999)

		var out bytes.Buffer
		transcript := newACPTranscript(&out)

		outgoing := []byte(
			`{"jsonrpc":"2.0","id":1,"method":"` +
				method +
				`","params":{"sessionId":"` +
				sessionID +
				`"}}`,
		)
		incoming := []byte(`{"jsonrpc":"2.0","id":1,"result":{"` + resultKey + `":` + jsonNumber(resultValue) + `}}`)

		require.NoError(t, transcript.recordOutgoing(outgoing))
		require.NoError(t, transcript.recordIncoming(incoming))

		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		require.Len(t, lines, 2)

		var first acpTranscriptEntry
		require.NoError(t, json.Unmarshal([]byte(lines[0]), &first))
		assert.Equal(t, "out", first.Direction)
		firstEnvelope := first.Envelope.(map[string]any)
		assert.Equal(t, method, firstEnvelope["method"])

		var second acpTranscriptEntry
		require.NoError(t, json.Unmarshal([]byte(lines[1]), &second))
		assert.Equal(t, "in", second.Direction)
		secondEnvelope := second.Envelope.(map[string]any)
		result, ok := secondEnvelope["result"].(map[string]any)
		require.True(t, ok)
		value, isFloat := result[resultKey].(float64)
		require.True(t, isFloat)
		assert.InDelta(t, float64(resultValue), value, 0)
	})

	t.Run("nil transcript is a no-op", func(t *testing.T) {
		var transcript *acpTranscript
		require.NoError(t, transcript.recordOutgoing([]byte(`{"ok":"`+fake.Lorem().Word()+`"}`)))
		require.NoError(t, transcript.recordIncoming([]byte(`{"ok":"`+fake.Lorem().Word()+`"}`)))
	})

	t.Run("invalid JSON envelope falls back to raw field", func(t *testing.T) {
		var out bytes.Buffer
		transcript := newACPTranscript(&out)
		raw := fake.Lorem().Sentence(3)

		require.NoError(t, transcript.recordIncoming([]byte(raw)))
		var entry acpTranscriptEntry
		require.NoError(t, json.Unmarshal(bytes.TrimSpace(out.Bytes()), &entry))
		envelope, ok := entry.Envelope.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, raw, envelope["raw"])
	})

	t.Run("file helper creates writable transcript", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), fake.Lorem().Word()+".jsonl")
		transcript, closer, err := newACPTranscriptFile(path)
		require.NoError(t, err)
		require.NotNil(t, transcript)
		require.NotNil(t, closer)
		defer func() {
			_ = closer.Close()
		}()

		require.NoError(
			t,
			transcript.recordOutgoing(
				[]byte(`{"jsonrpc":"2.0","id":1,"method":"`+fake.Lorem().Word()+`"}`),
			),
		)
		require.NoError(t, closer.Close())

		data, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		assert.NotEmpty(t, strings.TrimSpace(string(data)))
	})

	t.Run("empty path returns nil transcript and nil closer", func(t *testing.T) {
		transcript, closer, err := newACPTranscriptFile("")
		require.NoError(t, err)
		assert.Nil(t, transcript)
		assert.Nil(t, closer)
	})

	t.Run("create file failure is returned", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), fake.Lorem().Word())
		require.NoError(t, os.WriteFile(file, []byte(fake.Lorem().Word()), 0o644))
		_, _, err := newACPTranscriptFile(filepath.Join(file, fake.Lorem().Word()+".jsonl"))
		require.Error(t, err)
	})

	t.Run("newACPTranscript nil writer returns nil transcript", func(t *testing.T) {
		assert.Nil(t, newACPTranscript(nil))
	})

	t.Run("newACPTranscript non nil writer returns transcript", func(t *testing.T) {
		assert.NotNil(t, newACPTranscript(io.Discard))
	})
}

func jsonNumber(v int) string {
	return strconv.Itoa(v)
}
