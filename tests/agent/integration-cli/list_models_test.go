package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gemyago/sonalmod/runtime/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubModelsLister struct {
	models []agent.ModelInfo
	err    error
}

func (s stubModelsLister) ListModels(_ context.Context) ([]agent.ModelInfo, error) {
	return s.models, s.err
}

func TestRunListModels(t *testing.T) {
	ctx := t.Context()

	t.Run("nil lister returns error", func(t *testing.T) {
		var buf bytes.Buffer
		err := runListModels(ctx, nil, &buf)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "model listing is not available")
		assert.Empty(t, buf.String())
	})

	t.Run("ListModels error is wrapped", func(t *testing.T) {
		lister := stubModelsLister{err: errors.New("upstream failure")}
		var buf bytes.Buffer
		err := runListModels(ctx, lister, &buf)
		require.Error(t, err)
		require.ErrorContains(t, err, "list models")
		require.ErrorContains(t, err, "upstream failure")
	})

	t.Run("success writes sorted lines", func(t *testing.T) {
		lister := stubModelsLister{
			models: []agent.ModelInfo{
				{Provider: "b", Name: "m1", DisplayName: "ignored"},
				{Provider: "a", Name: "z"},
				{Provider: "a", Name: "a", DisplayName: "ignored"},
			},
		}
		var buf bytes.Buffer
		require.NoError(t, runListModels(ctx, lister, &buf))
		lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
		require.Len(t, lines, 3)
		assert.Equal(t, "* a/a", lines[0])
		assert.Equal(t, "* a/z", lines[1])
		assert.Equal(t, "* b/m1", lines[2])
	})
}

func TestWriteListModels(t *testing.T) {
	t.Run("sorts by provider then name", func(t *testing.T) {
		models := []agent.ModelInfo{
			{Provider: "p2", Name: "n1"},
			{Provider: "p1", Name: "n2"},
			{Provider: "p1", Name: "n1"},
		}
		var buf bytes.Buffer
		require.NoError(t, writeListModels(&buf, models))
		assert.Equal(t, "* p1/n1\n* p1/n2\n* p2/n1\n", buf.String())
	})
}
