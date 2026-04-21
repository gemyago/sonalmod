//go:build !release

package internal

import (
	"context"
	"testing"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// systemPromptReadonlyCtxStub is a minimal agent.ReadonlyContext for system prompt tests.
type systemPromptReadonlyCtxStub struct {
	context.Context

	appName string
}

func (s *systemPromptReadonlyCtxStub) UserContent() *genai.Content          { return nil }
func (s *systemPromptReadonlyCtxStub) InvocationID() string                 { return "" }
func (s *systemPromptReadonlyCtxStub) AgentName() string                    { return "" }
func (s *systemPromptReadonlyCtxStub) ReadonlyState() session.ReadonlyState { return nil }
func (s *systemPromptReadonlyCtxStub) UserID() string                       { return "" }
func (s *systemPromptReadonlyCtxStub) AppName() string                      { return s.appName }
func (s *systemPromptReadonlyCtxStub) SessionID() string                    { return "" }
func (s *systemPromptReadonlyCtxStub) Branch() string                       { return "" }

var _ agent.ReadonlyContext = (*systemPromptReadonlyCtxStub)(nil)

func TestSystemPromptInstructionProvider(t *testing.T) {
	fake := faker.New()

	// testSystemPromptBaseTemplate only exercises .AppName interpolation; it is not coupled to
	// system_prompt_base.tmpl so that file can change without updating these tests.
	const testSystemPromptBaseTemplate = "{{.AppName}}"
	testOpts := []SystemPromptInstructionProviderOption{
		WithSystemPromptBaseTemplate(testSystemPromptBaseTemplate),
	}

	t.Run("embedded prompt renders successfully", func(t *testing.T) {
		appName := fake.Lorem().Word()
		provider := newSystemPromptInstructionProvider(nil)
		rc := &systemPromptReadonlyCtxStub{Context: t.Context(), appName: appName}
		got, err := provider(rc)
		require.NoError(t, err)
		assert.NotEmpty(t, got)
	})

	t.Run("single fragment prepends base template then fragment sections", func(t *testing.T) {
		appName := fake.Lorem().Word()
		section := fake.Lorem().Word()
		content := fake.Lorem().Sentence(8)
		provider := newSystemPromptInstructionProvider([]SystemPromptFragment{
			{Section: section, Content: content},
		}, testOpts...)
		rc := &systemPromptReadonlyCtxStub{Context: t.Context(), appName: appName}
		got, err := provider(rc)
		require.NoError(t, err)
		want := appName + "\n\n## " + section + "\n\n" + content + "\n\n## Closing notes"
		assert.Equal(t, want, got)
	})

	t.Run("multiple fragments are joined with blank line separator after base", func(t *testing.T) {
		appName := fake.Lorem().Word()
		s1 := fake.Lorem().Word()
		c1 := fake.Lorem().Sentence(8)
		s2 := fake.Lorem().Word()
		c2 := fake.Lorem().Sentence(8)
		provider := newSystemPromptInstructionProvider([]SystemPromptFragment{
			{Section: s1, Content: c1},
			{Section: s2, Content: c2},
		}, testOpts...)
		rc := &systemPromptReadonlyCtxStub{Context: t.Context(), appName: appName}
		got, err := provider(rc)
		require.NoError(t, err)
		want := appName + "\n\n## " + s1 + "\n\n" + c1 + "\n\n## " + s2 + "\n\n" + c2 + "\n\n## Closing notes"
		assert.Equal(t, want, got)
	})
}
