package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCLIParams(t *testing.T) {
	t.Run("session ID field has empty default value", func(t *testing.T) {
		params := cliParams{}
		assert.Empty(t, params.SessionID)
	})
	t.Run("prompt field has empty default value", func(t *testing.T) {
		params := cliParams{}
		assert.Empty(t, params.Prompt)
	})
}
