package codinglane

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenCodeACPLauncher(t *testing.T) {
	fake := faker.New()

	makeProfile := func(name string) agentprofiles.AgentProfile {
		return agentprofiles.AgentProfile{
			Name:         name,
			DisplayName:  fake.Lorem().Word(),
			Role:         "coding-agent",
			Instructions: "Follow repository rules and keep changes focused.",
			ToolRefs:     []string{"workspacefs", "skills"},
			ExecutionSettings: agentprofiles.ExecutionSettings{
				DefaultModel: "openai/gpt-5",
			},
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
	}

	makeBinding := func(name string, profileName string) OpenCodeBinding {
		return OpenCodeBinding{
			Name:        name,
			ProfileName: profileName,
			CWD:         "/workspace/project",
			AgentCommand: OpenCodeAgentCommand{
				Command: "opencode",
				Args:    []string{"acp"},
			},
			LaunchOptions: OpenCodeLaunchOptions{Transport: "stdio"},
			CreatedAt:     time.Now().UTC(),
			UpdatedAt:     time.Now().UTC(),
		}
	}

	t.Run("launch succeeds when profile and binding exist", func(t *testing.T) {
		profile := makeProfile("profile-main")
		binding := makeBinding("binding-main", profile.Name)

		acpClient := &fakeOpenCodeACPClient{
			launchFn: func(_ context.Context, req OpenCodeACPLaunchRequest) (*OpenCodeACPLaunchResult, error) {
				assert.Equal(t, binding.AgentCommand.Command, req.AgentCommand.Command)
				assert.Equal(t, binding.CWD, req.CWD)
				assert.Contains(t, req.Prompt, profile.Instructions)
				assert.Contains(t, req.Prompt, "fix failing launch tests")
				return &OpenCodeACPLaunchResult{
					SessionID:    "session-42",
					PromptResult: json.RawMessage(`{"ok":true}`),
					Updates: []OpenCodeACPUpdate{{
						SessionID: "session-42",
						Type:      "final",
						Payload:   json.RawMessage(`{"type":"final"}`),
					}},
				}, nil
			},
		}

		launcher, err := NewOpenCodeACPLauncher(&fakeProfilesService{
			profiles: map[string]agentprofiles.AgentProfile{profile.Name: profile},
		}, &fakeBindingsService{
			bindings: map[string]OpenCodeBinding{binding.Name: binding},
		}, acpClient)
		require.NoError(t, err)

		result, launchErr := launcher.Launch(t.Context(), OpenCodeLaunchRequest{
			ProfileName: profile.Name,
			Prompt:      "fix failing launch tests",
		})
		require.NoError(t, launchErr)
		require.NotNil(t, result)
		assert.Equal(t, profile.Name, result.ProfileName)
		assert.Equal(t, binding.Name, result.BindingName)
		assert.Equal(t, "session-42", result.SessionID)
		assert.JSONEq(t, `{"ok":true}`, string(result.PromptResult))
	})

	t.Run("missing profile or binding returns not-found", func(t *testing.T) {
		profile := makeProfile("profile-main")

		launcherMissingProfile, err := NewOpenCodeACPLauncher(&fakeProfilesService{
			getErr: errors.New("wrapper: " + agentprofiles.ErrAgentProfileNotFound.Error()),
		}, &fakeBindingsService{}, &fakeOpenCodeACPClient{})
		require.NoError(t, err)

		_, launchErr := launcherMissingProfile.Launch(t.Context(), OpenCodeLaunchRequest{
			ProfileName: profile.Name,
			Prompt:      fake.Lorem().Sentence(4),
		})
		require.Error(t, launchErr)
		assertOpenCodeLaunchErrorKind(t, launchErr, OpenCodeLaunchErrorKindNotFound)

		launcherMissingBinding, err := NewOpenCodeACPLauncher(&fakeProfilesService{
			profiles: map[string]agentprofiles.AgentProfile{profile.Name: profile},
		}, &fakeBindingsService{bindings: map[string]OpenCodeBinding{}}, &fakeOpenCodeACPClient{})
		require.NoError(t, err)

		_, launchErr = launcherMissingBinding.Launch(t.Context(), OpenCodeLaunchRequest{
			ProfileName: profile.Name,
			Prompt:      fake.Lorem().Sentence(4),
		})
		require.Error(t, launchErr)
		assertOpenCodeLaunchErrorKind(t, launchErr, OpenCodeLaunchErrorKindNotFound)
	})

	t.Run("subprocess or protocol failures map to launch-failed with context", func(t *testing.T) {
		profile := makeProfile("profile-main")
		binding := makeBinding("binding-main", profile.Name)

		launcher, err := NewOpenCodeACPLauncher(&fakeProfilesService{
			profiles: map[string]agentprofiles.AgentProfile{profile.Name: profile},
		}, &fakeBindingsService{
			bindings: map[string]OpenCodeBinding{binding.Name: binding},
		}, &fakeOpenCodeACPClient{
			launchFn: func(_ context.Context, _ OpenCodeACPLaunchRequest) (*OpenCodeACPLaunchResult, error) {
				return nil, &OpenCodeACPError{
					Kind: OpenCodeACPErrorKindSubprocess,
					Op:   "start-subprocess",
					Err:  errors.New("binary missing"),
				}
			},
		})
		require.NoError(t, err)

		_, launchErr := launcher.Launch(t.Context(), OpenCodeLaunchRequest{
			ProfileName: profile.Name,
			Prompt:      fake.Lorem().Sentence(4),
		})
		require.Error(t, launchErr)
		assertOpenCodeLaunchErrorKind(t, launchErr, OpenCodeLaunchErrorKindLaunchFailed)
		assert.ErrorContains(t, launchErr, binding.Name)
		assert.ErrorContains(t, launchErr, profile.Name)
	})
}

func assertOpenCodeLaunchErrorKind(t *testing.T, err error, kind OpenCodeLaunchErrorKind) {
	t.Helper()

	var launchErr *OpenCodeLaunchError
	require.ErrorAs(t, err, &launchErr)
	assert.Equal(t, kind, launchErr.Kind)
}

type fakeOpenCodeACPClient struct {
	launchFn func(context.Context, OpenCodeACPLaunchRequest) (*OpenCodeACPLaunchResult, error)
}

func (f *fakeOpenCodeACPClient) Launch(
	ctx context.Context,
	request OpenCodeACPLaunchRequest,
) (*OpenCodeACPLaunchResult, error) {
	if f.launchFn != nil {
		return f.launchFn(ctx, request)
	}
	return &OpenCodeACPLaunchResult{SessionID: faker.New().UUID().V4()}, nil
}

type fakeProfilesService struct {
	profiles map[string]agentprofiles.AgentProfile
	getErr   error
}

func (f *fakeProfilesService) List(context.Context) ([]agentprofiles.AgentProfile, error) {
	out := make([]agentprofiles.AgentProfile, 0, len(f.profiles))
	for _, profile := range f.profiles {
		out = append(out, profile)
	}
	return out, nil
}

func (f *fakeProfilesService) Get(_ context.Context, name string) (*agentprofiles.AgentProfile, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	profile, ok := f.profiles[name]
	if !ok {
		return nil, agentprofiles.ErrAgentProfileNotFound
	}
	return &profile, nil
}

func (f *fakeProfilesService) Create(context.Context, agentprofiles.CreateAgentProfileParams) (*agentprofiles.AgentProfile, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeProfilesService) Update(context.Context, string, agentprofiles.UpdateAgentProfileParams) (*agentprofiles.AgentProfile, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeProfilesService) Delete(context.Context, string) error {
	return errors.New("not implemented")
}

func (f *fakeProfilesService) AutoMigrate() error {
	return nil
}

type fakeBindingsService struct {
	bindings map[string]OpenCodeBinding
	getErr   error
	listErr  error
}

func (f *fakeBindingsService) List(context.Context) ([]OpenCodeBinding, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]OpenCodeBinding, 0, len(f.bindings))
	for _, binding := range f.bindings {
		out = append(out, binding)
	}
	return out, nil
}

func (f *fakeBindingsService) Get(_ context.Context, name string) (*OpenCodeBinding, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	binding, ok := f.bindings[name]
	if !ok {
		return nil, ErrOpenCodeBindingNotFound
	}
	return &binding, nil
}

func (f *fakeBindingsService) Create(context.Context, CreateOpenCodeBindingParams) (*OpenCodeBinding, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeBindingsService) Update(context.Context, string, UpdateOpenCodeBindingParams) (*OpenCodeBinding, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeBindingsService) Delete(context.Context, string) error {
	return errors.New("not implemented")
}

func (f *fakeBindingsService) AutoMigrate() error {
	return nil
}
