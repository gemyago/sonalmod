package sessions

import (
	"encoding/json"
	"maps"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/adk/session"
)

func TestNewFileSessionService(t *testing.T) {
	t.Run("empty baseDir returns error", func(t *testing.T) {
		svc, err := NewFileSessionService("", nil)
		require.Error(t, err)
		assert.Nil(t, svc)
		assert.Contains(t, err.Error(), "base_dir")
	})

	t.Run("creates service with valid baseDir", func(t *testing.T) {
		baseDir := t.TempDir()
		svc, err := NewFileSessionService(baseDir, nil)
		require.NoError(t, err)
		require.NotNil(t, svc)
		// Verify it implements session.Service by using it
		ctx := t.Context()
		_, err = svc.Create(ctx, &session.CreateRequest{
			AppName:   "test",
			UserID:    "user-1",
			SessionID: "sess-1",
		})
		require.NoError(t, err)
	})

	t.Run("creates baseDir if it does not exist", func(t *testing.T) {
		baseDir := filepath.Join(t.TempDir(), "nested", "dir")
		svc, err := NewFileSessionService(baseDir, nil)
		require.NoError(t, err)
		require.NotNil(t, svc)
		require.DirExists(t, baseDir)
	})

	t.Run("fails when base path cannot be created", func(t *testing.T) {
		tmp := t.TempDir()
		blocked := filepath.Join(tmp, "not-a-dir")
		require.NoError(t, os.WriteFile(blocked, []byte("x"), 0600))

		_, err := NewFileSessionService(filepath.Join(blocked, "nested"), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "create base dir")
	})
}

func TestExtractStateDeltas(t *testing.T) {
	fake := faker.New()

	t.Run("nil or empty delta returns empty maps", func(t *testing.T) {
		cases := []struct {
			name  string
			delta map[string]any
		}{
			{"nil", nil},
			{"empty", map[string]any{}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				app, user, session := extractStateDeltas(tc.delta)

				require.NotNil(t, app)
				require.NotNil(t, user)
				require.NotNil(t, session)
				assert.Empty(t, app)
				assert.Empty(t, user)
				assert.Empty(t, session)
			})
		}
	})

	t.Run("app prefix keys go to app map without prefix", func(t *testing.T) {
		key := fake.Lorem().Word()
		val := fake.Lorem().Sentence(3)
		delta := map[string]any{"app:" + key: val}

		app, user, session := extractStateDeltas(delta)

		assert.Equal(t, map[string]any{key: val}, app)
		assert.Empty(t, user)
		assert.Empty(t, session)
	})

	t.Run("user prefix keys go to user map without prefix", func(t *testing.T) {
		key := fake.Lorem().Word()
		val := fake.Lorem().Sentence(3)
		delta := map[string]any{"user:" + key: val}

		app, user, session := extractStateDeltas(delta)

		assert.Empty(t, app)
		assert.Equal(t, map[string]any{key: val}, user)
		assert.Empty(t, session)
	})

	t.Run("unprefixed keys go to session map", func(t *testing.T) {
		key := fake.Lorem().Word()
		val := fake.Lorem().Sentence(3)
		delta := map[string]any{key: val}

		app, user, session := extractStateDeltas(delta)

		assert.Empty(t, app)
		assert.Empty(t, user)
		assert.Equal(t, map[string]any{key: val}, session)
	})

	t.Run("temp prefix keys are excluded", func(t *testing.T) {
		key := fake.Lorem().Word()
		val := fake.Lorem().Sentence(3)
		delta := map[string]any{"temp:" + key: val}

		app, user, session := extractStateDeltas(delta)

		assert.Empty(t, app)
		assert.Empty(t, user)
		assert.Empty(t, session)
	})

	t.Run("mixed prefixes split correctly", func(t *testing.T) {
		appKey := fake.Lorem().Word()
		userKey := fake.Lorem().Word()
		sessionKey := fake.Lorem().Word()
		tempKey := fake.Lorem().Word()
		delta := map[string]any{
			"app:" + appKey:   "app-val",
			"user:" + userKey: "user-val",
			sessionKey:        "session-val",
			"temp:" + tempKey: "temp-val",
		}

		app, user, session := extractStateDeltas(delta)

		assert.Equal(t, map[string]any{appKey: "app-val"}, app)
		assert.Equal(t, map[string]any{userKey: "user-val"}, user)
		assert.Equal(t, map[string]any{sessionKey: "session-val"}, session)
	})
}

func TestMergeStates(t *testing.T) {
	fake := faker.New()

	t.Run("nil or empty inputs return empty map", func(t *testing.T) {
		cases := []struct {
			name               string
			app, user, session map[string]any
		}{
			{"all nil", nil, nil, nil},
			{"all empty", map[string]any{}, map[string]any{}, map[string]any{}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				merged := mergeStates(tc.app, tc.user, tc.session)

				require.NotNil(t, merged)
				assert.Empty(t, merged)
			})
		}
	})

	t.Run("session keys copied as-is", func(t *testing.T) {
		key := fake.Lorem().Word()
		val := fake.Lorem().Sentence(3)
		session := map[string]any{key: val}

		merged := mergeStates(nil, nil, session)

		assert.Equal(t, map[string]any{key: val}, merged)
	})

	t.Run("app keys get app prefix", func(t *testing.T) {
		key := fake.Lorem().Word()
		val := fake.Lorem().Sentence(3)
		app := map[string]any{key: val}

		merged := mergeStates(app, nil, nil)

		assert.Equal(t, map[string]any{"app:" + key: val}, merged)
	})

	t.Run("user keys get user prefix", func(t *testing.T) {
		key := fake.Lorem().Word()
		val := fake.Lorem().Sentence(3)
		user := map[string]any{key: val}

		merged := mergeStates(nil, user, nil)

		assert.Equal(t, map[string]any{"user:" + key: val}, merged)
	})

	t.Run("all scopes merged correctly", func(t *testing.T) {
		appKey := fake.Lorem().Word()
		userKey := fake.Lorem().Word()
		sessionKey := fake.Lorem().Word()
		app := map[string]any{appKey: "app-val"}
		user := map[string]any{userKey: "user-val"}
		session := map[string]any{sessionKey: "session-val"}

		merged := mergeStates(app, user, session)

		expected := map[string]any{
			"app:" + appKey:   "app-val",
			"user:" + userKey: "user-val",
			sessionKey:        "session-val",
		}
		assert.Equal(t, expected, merged)
	})
}

func TestFileSessionService(t *testing.T) {
	fake := faker.New()

	makeService := func(t *testing.T) *FileSessionService {
		t.Helper()
		svc, err := NewFileSessionService(t.TempDir(), nil)
		require.NoError(t, err)
		return svc
	}

	t.Run("Create session Get returns same session", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()
		appName := "app-" + fake.Lorem().Word()
		userID := "user-" + fake.UUID().V4()
		sessionID := uuid.New().String()
		state := map[string]any{"k1": "v1"}

		createResp, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
			State:     state,
		})
		require.NoError(t, err)
		require.NotNil(t, createResp)
		assert.Equal(t, sessionID, createResp.Session.ID())
		assert.Equal(t, appName, createResp.Session.AppName())
		assert.Equal(t, userID, createResp.Session.UserID())

		getResp, err := svc.Get(ctx, &session.GetRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		require.NoError(t, err)
		require.NotNil(t, getResp)
		assert.Equal(t, sessionID, getResp.Session.ID())
		assert.Equal(t, appName, getResp.Session.AppName())
		assert.Equal(t, userID, getResp.Session.UserID())
		gotState := collectState(getResp.Session.State())
		assert.Equal(t, state, gotState)
	})

	t.Run("Create with duplicate session ID returns error", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()
		appName := "app-" + fake.Lorem().Word()
		userID := "user-" + fake.UUID().V4()
		sessionID := uuid.New().String()

		_, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		require.NoError(t, err)

		_, err = svc.Create(ctx, &session.CreateRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("Get with NumRecentEvents filters correctly", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()
		appName := "app-" + fake.Lorem().Word()
		userID := "user-" + fake.UUID().V4()
		sessionID := uuid.New().String()

		_, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		require.NoError(t, err)

		// Append 5 events via direct file write so Get can exercise event slicing without
		// depending on event ordering from multiple AppendEvent calls.
		appendSessionEventsViaFile(t, svc.baseDir, appName, userID, sessionID, 5)

		getResp, err := svc.Get(ctx, &session.GetRequest{
			AppName:         appName,
			UserID:          userID,
			SessionID:       sessionID,
			NumRecentEvents: 3,
		})
		require.NoError(t, err)
		require.NotNil(t, getResp)
		assert.Equal(t, 3, getResp.Session.Events().Len())
		// Should be last 3: events 3, 4, 5
		assert.Equal(t, "3", getResp.Session.Events().At(0).ID)
		assert.Equal(t, "4", getResp.Session.Events().At(1).ID)
		assert.Equal(t, "5", getResp.Session.Events().At(2).ID)
	})

	t.Run("Get with After filters correctly", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()
		appName := "app-" + fake.Lorem().Word()
		userID := "user-" + fake.UUID().V4()
		sessionID := uuid.New().String()

		_, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		require.NoError(t, err)

		appendSessionEventsViaFile(t, svc.baseDir, appName, userID, sessionID, 5)

		getResp, err := svc.Get(ctx, &session.GetRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
			After:     time.Time{}.Add(4),
		})
		require.NoError(t, err)
		require.NotNil(t, getResp)
		assert.Equal(t, 2, getResp.Session.Events().Len())
		assert.Equal(t, "4", getResp.Session.Events().At(0).ID)
		assert.Equal(t, "5", getResp.Session.Events().At(1).ID)
	})

	t.Run("Get non-existent session returns error", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()
		appName := "app-" + fake.Lorem().Word()
		userID := "user-" + fake.UUID().V4()
		sessionID := uuid.New().String()

		_, err := svc.Get(ctx, &session.GetRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("List returns sessions for app/user", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()
		appName := "app-" + fake.Lorem().Word()
		userID := "user-" + fake.UUID().V4()
		sessionID1 := uuid.New().String()
		sessionID2 := uuid.New().String()

		_, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID1,
			State:     map[string]any{"k1": "v1"},
		})
		require.NoError(t, err)
		_, err = svc.Create(ctx, &session.CreateRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID2,
			State:     map[string]any{"k2": "v2"},
		})
		require.NoError(t, err)

		listResp, err := svc.List(ctx, &session.ListRequest{
			AppName: appName,
			UserID:  userID,
		})
		require.NoError(t, err)
		require.NotNil(t, listResp)
		require.Len(t, listResp.Sessions, 2)
		ids := []string{listResp.Sessions[0].ID(), listResp.Sessions[1].ID()}
		assert.Contains(t, ids, sessionID1)
		assert.Contains(t, ids, sessionID2)
	})

	t.Run("List with userID filters correctly", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()
		appName := "app-" + fake.Lorem().Word()
		userID1 := "user-" + fake.UUID().V4()
		userID2 := "user-" + fake.UUID().V4()

		_, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   appName,
			UserID:    userID1,
			SessionID: uuid.New().String(),
		})
		require.NoError(t, err)
		_, err = svc.Create(ctx, &session.CreateRequest{
			AppName:   appName,
			UserID:    userID2,
			SessionID: uuid.New().String(),
		})
		require.NoError(t, err)

		listResp, err := svc.List(ctx, &session.ListRequest{
			AppName: appName,
			UserID:  userID1,
		})
		require.NoError(t, err)
		require.NotNil(t, listResp)
		require.Len(t, listResp.Sessions, 1)
		assert.Equal(t, userID1, listResp.Sessions[0].UserID())
	})

	t.Run("Delete removes session Get afterward returns error", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()
		appName := "app-" + fake.Lorem().Word()
		userID := "user-" + fake.UUID().V4()
		sessionID := uuid.New().String()

		_, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		require.NoError(t, err)

		err = svc.Delete(ctx, &session.DeleteRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		require.NoError(t, err)

		_, err = svc.Get(ctx, &session.GetRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("AppendEvent adds event Get returns updated events", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()
		appName := "app-" + fake.Lorem().Word()
		userID := "user-" + fake.UUID().V4()
		sessionID := uuid.New().String()

		createResp, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		require.NoError(t, err)
		sess := createResp.Session

		ev := &session.Event{
			ID:        "ev-1",
			Author:    "user",
			Timestamp: time.Now(),
			Actions:   session.EventActions{StateDelta: map[string]any{"sk": "sv"}},
		}

		err = svc.AppendEvent(ctx, sess, ev)
		require.NoError(t, err)

		// In-memory session must reflect the new event (ADK runner reads sess.Events() after AppendEvent).
		require.Equal(t, 1, sess.Events().Len())
		assert.Equal(t, "ev-1", sess.Events().At(0).ID)
		assert.Equal(t, "user", sess.Events().At(0).Author)
		gotState := collectState(sess.State())
		assert.Equal(t, map[string]any{"sk": "sv"}, gotState)

		// Persistence: Get returns same events.
		getResp, err := svc.Get(ctx, &session.GetRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		require.NoError(t, err)
		require.Equal(t, 1, getResp.Session.Events().Len())
		assert.Equal(t, "ev-1", getResp.Session.Events().At(0).ID)
	})

	t.Run("AppendEvent with partial event does not persist it", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()
		appName := "app-" + fake.Lorem().Word()
		userID := "user-" + fake.UUID().V4()
		sessionID := uuid.New().String()

		createResp, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		require.NoError(t, err)
		sess := createResp.Session

		ev := &session.Event{
			ID:        "ev-partial",
			Author:    "model",
			Timestamp: time.Now(),
		}
		ev.Partial = true

		err = svc.AppendEvent(ctx, sess, ev)
		require.NoError(t, err)

		getResp, err := svc.Get(ctx, &session.GetRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		require.NoError(t, err)
		assert.Equal(t, 0, getResp.Session.Events().Len())
	})

	t.Run("AppendEvent updates app/user state files", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()
		appName := "app-" + fake.Lorem().Word()
		userID := "user-" + fake.UUID().V4()
		sessionID := uuid.New().String()

		createResp, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		require.NoError(t, err)
		sess := createResp.Session

		ev := &session.Event{
			ID:        "ev-2",
			Author:    "model",
			Timestamp: time.Now(),
			Actions: session.EventActions{
				StateDelta: map[string]any{
					"app:ak":  "av",
					"user:uk": "uv",
					"sk":      "sv",
				},
			},
		}

		err = svc.AppendEvent(ctx, sess, ev)
		require.NoError(t, err)

		getResp, err := svc.Get(ctx, &session.GetRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		require.NoError(t, err)
		gotState := collectState(getResp.Session.State())
		assert.Equal(t, "av", gotState["app:ak"])
		assert.Equal(t, "uv", gotState["user:uk"])
		assert.Equal(t, "sv", gotState["sk"])
	})

	t.Run("fileSession LastUpdateTime State Get Set and iterators", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()
		appName := "app-" + fake.Lorem().Word()
		userID := "user-" + fake.UUID().V4()
		sessionID := uuid.New().String()

		createResp, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
			State:     map[string]any{"k": "v"},
		})
		require.NoError(t, err)
		sess := createResp.Session.(*fileSession)
		assert.False(t, sess.LastUpdateTime().IsZero())

		st := sess.State()
		_, err = st.Get("missing")
		require.ErrorIs(t, err, session.ErrStateKeyNotExist)

		v, err := st.Get("k")
		require.NoError(t, err)
		assert.Equal(t, "v", v)

		require.NoError(t, st.Set("k2", "v2"))
		v, err = st.Get("k2")
		require.NoError(t, err)
		assert.Equal(t, "v2", v)

		pairs := maps.Collect(st.All())
		assert.Equal(t, "v", pairs["k"])
		assert.Equal(t, "v2", pairs["k2"])

		// Early stop in State.All iterator (yield false branch).
		n := 0
		for k, v := range st.All() {
			_, _ = k, v
			n++
			break
		}
		assert.Equal(t, 1, n)

		evs := sess.Events()
		for range evs.All() {
			break
		}
		assert.Nil(t, evs.At(-1))
		assert.Nil(t, evs.At(99))
	})

	t.Run("Create rejects empty app_name or user_id", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()

		_, err := svc.Create(ctx, &session.CreateRequest{AppName: "", UserID: "u"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "app_name")

		_, err = svc.Create(ctx, &session.CreateRequest{AppName: "a", UserID: ""})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "user_id")
	})

	t.Run("Create generates SessionID when empty", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()
		appName := "app-" + fake.Lorem().Word()
		userID := "user-" + fake.UUID().V4()

		createResp, err := svc.Create(ctx, &session.CreateRequest{
			AppName: appName,
			UserID:  userID,
			State:   map[string]any{"x": 1},
		})
		require.NoError(t, err)
		id := createResp.Session.ID()
		require.NotEmpty(t, id)
		_, parseErr := uuid.Parse(id)
		require.NoError(t, parseErr)
	})

	t.Run("Create with app and user prefixed state writes scope files", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()
		appName := "app-" + fake.Lorem().Word()
		userID := "user-" + fake.UUID().V4()
		sessionID := uuid.New().String()

		_, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
			State: map[string]any{
				"app:shared":   "from-app",
				"user:pref":    "from-user",
				"session-only": "s",
			},
		})
		require.NoError(t, err)

		getResp, err := svc.Get(ctx, &session.GetRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		require.NoError(t, err)
		got := collectState(getResp.Session.State())
		assert.Equal(t, "from-app", got["app:shared"])
		assert.Equal(t, "from-user", got["user:pref"])
		assert.Equal(t, "s", got["session-only"])
	})

	t.Run("Get rejects missing identifiers", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()

		_, err := svc.Get(ctx, &session.GetRequest{AppName: "", UserID: "u", SessionID: "s"})
		require.Error(t, err)

		_, err = svc.Get(ctx, &session.GetRequest{AppName: "a", UserID: "", SessionID: "s"})
		require.Error(t, err)

		_, err = svc.Get(ctx, &session.GetRequest{AppName: "a", UserID: "u", SessionID: ""})
		require.Error(t, err)
	})

	t.Run("Get and List return parse error when session file is invalid JSON", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()
		appName := "app-" + fake.Lorem().Word()
		userID := "user-" + fake.UUID().V4()
		sessionID := uuid.New().String()

		_, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		require.NoError(t, err)

		path := sessionPath(svc.baseDir, appName, userID, sessionID)
		require.NoError(t, os.WriteFile(path, []byte("{"), 0600))

		_, err = svc.Get(ctx, &session.GetRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse")

		_, err = svc.List(ctx, &session.ListRequest{AppName: appName})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse")
	})

	t.Run("List requires app_name", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()

		_, err := svc.List(ctx, &session.ListRequest{AppName: ""})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "app_name")
	})

	t.Run("List without userID aggregates sessions across users", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()
		appName := "app-" + fake.Lorem().Word()
		u1 := "user-" + fake.UUID().V4()
		u2 := "user-" + fake.UUID().V4()
		s1 := uuid.New().String()
		s2 := uuid.New().String()

		_, err := svc.Create(ctx, &session.CreateRequest{AppName: appName, UserID: u1, SessionID: s1})
		require.NoError(t, err)
		_, err = svc.Create(ctx, &session.CreateRequest{AppName: appName, UserID: u2, SessionID: s2})
		require.NoError(t, err)

		listResp, err := svc.List(ctx, &session.ListRequest{AppName: appName})
		require.NoError(t, err)
		require.Len(t, listResp.Sessions, 2)
		ids := []string{listResp.Sessions[0].ID(), listResp.Sessions[1].ID()}
		assert.ElementsMatch(t, []string{s1, s2}, ids)
	})

	t.Run("List returns empty when app directory does not exist", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()

		listResp, err := svc.List(ctx, &session.ListRequest{AppName: "missing-app-" + fake.Lorem().Word()})
		require.NoError(t, err)
		require.Empty(t, listResp.Sessions)
	})

	t.Run("Delete rejects missing identifiers", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()

		err := svc.Delete(ctx, &session.DeleteRequest{AppName: "", UserID: "u", SessionID: "s"})
		require.Error(t, err)
	})

	t.Run("Delete returns not found when session file missing", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()

		err := svc.Delete(ctx, &session.DeleteRequest{
			AppName:   "a",
			UserID:    "u",
			SessionID: uuid.New().String(),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("AppendEvent rejects nil event", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()
		createResp, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   "a",
			UserID:    "u",
			SessionID: uuid.New().String(),
		})
		require.NoError(t, err)

		err = svc.AppendEvent(ctx, createResp.Session, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nil")
	})

	t.Run("AppendEvent rejects non-fileSession", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()
		ev := &session.Event{ID: "e", Timestamp: time.Now()}

		err := svc.AppendEvent(ctx, appendEventWrongTypeSession{}, ev)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected session type")
	})

	t.Run("AppendEvent returns not found when session file was removed", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()
		appName := "app-" + fake.Lorem().Word()
		userID := "user-" + fake.UUID().V4()
		sessionID := uuid.New().String()

		createResp, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		require.NoError(t, err)
		sess := createResp.Session.(*fileSession)

		path := sessionPath(svc.baseDir, appName, userID, sessionID)
		require.NoError(t, os.Remove(path))

		err = svc.AppendEvent(ctx, sess, &session.Event{ID: "e", Timestamp: time.Now()})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("AppendEvent with only temp state deltas does not change merged session state", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()
		appName := "app-" + fake.Lorem().Word()
		userID := "user-" + fake.UUID().V4()
		sessionID := uuid.New().String()

		createResp, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
			State:     map[string]any{"keep": "yes"},
		})
		require.NoError(t, err)
		sess := createResp.Session.(*fileSession)

		err = svc.AppendEvent(ctx, sess, &session.Event{
			ID:        "ev",
			Timestamp: time.Now(),
			Actions: session.EventActions{
				StateDelta: map[string]any{"temp:scratch": "gone"},
			},
		})
		require.NoError(t, err)

		getResp, err := svc.Get(ctx, &session.GetRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		require.NoError(t, err)
		got := collectState(getResp.Session.State())
		assert.Equal(t, "yes", got["keep"])
		_, hasTemp := got["temp:scratch"]
		assert.False(t, hasTemp)
	})

	t.Run("AppendEvent fails when app state cannot be serialized", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()
		createResp, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   "a",
			UserID:    "u",
			SessionID: uuid.New().String(),
		})
		require.NoError(t, err)

		ch := make(chan int)
		ev := &session.Event{
			ID:        "ev",
			Timestamp: time.Now(),
			Actions: session.EventActions{
				StateDelta: map[string]any{"app:bad": ch},
			},
		}
		err = svc.AppendEvent(ctx, createResp.Session, ev)
		require.Error(t, err)
	})

	t.Run("readAppState ignores corrupt app JSON file", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()
		appName := "app-" + fake.Lorem().Word()
		userID := "user-" + fake.UUID().V4()
		sessionID := uuid.New().String()

		appPath := appStatePath(svc.baseDir, appName)
		require.NoError(t, os.MkdirAll(filepath.Dir(appPath), 0750))
		require.NoError(t, os.WriteFile(appPath, []byte("not-json"), 0600))

		_, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
			State:     map[string]any{"k": "session"},
		})
		require.NoError(t, err)

		getResp, err := svc.Get(ctx, &session.GetRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		require.NoError(t, err)
		got := collectState(getResp.Session.State())
		assert.Equal(t, "session", got["k"])
	})

	t.Run("Get returns read error when session file is not readable", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("file mode not used for permissions the same way")
		}
		svc := makeService(t)
		ctx := t.Context()
		appName := "app-" + fake.Lorem().Word()
		userID := "user-" + fake.UUID().V4()
		sessionID := uuid.New().String()

		_, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		require.NoError(t, err)

		path := sessionPath(svc.baseDir, appName, userID, sessionID)
		require.NoError(t, os.Chmod(path, 0000))
		t.Cleanup(func() { _ = os.Chmod(path, 0600) })

		_, err = svc.Get(ctx, &session.GetRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read session file")
	})

	t.Run("fileSessionState Set initializes nil state map", func(t *testing.T) {
		var mu sync.RWMutex
		st := &fileSessionState{mu: &mu, state: nil}
		require.NoError(t, st.Set("a", 1))
		v, err := st.Get("a")
		require.NoError(t, err)
		assert.Equal(t, 1, v)
	})

	t.Run("Create fails when session JSON cannot be marshaled", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()

		_, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   "a",
			UserID:    "u",
			SessionID: uuid.New().String(),
			State:     map[string]any{"k": math.NaN()},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "marshal session")
	})

	t.Run("List returns error when app directory cannot be read", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("chmod permissions differ from Unix")
		}
		svc := makeService(t)
		ctx := t.Context()
		appName := "app-" + fake.Lorem().Word()
		userID := "user-" + fake.UUID().V4()

		_, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: uuid.New().String(),
		})
		require.NoError(t, err)

		appDir := filepath.Join(svc.baseDir, appName)
		require.NoError(t, os.Chmod(appDir, 0000))
		t.Cleanup(func() { _ = os.Chmod(appDir, 0750) })

		_, err = svc.List(ctx, &session.ListRequest{AppName: appName})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read dir")
	})

	t.Run("List without userID returns error when user subdir cannot be read", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("chmod permissions differ from Unix")
		}
		svc := makeService(t)
		ctx := t.Context()
		appName := "app-" + fake.Lorem().Word()
		userID := "user-" + fake.UUID().V4()
		sessionID := uuid.New().String()

		_, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		require.NoError(t, err)

		userDir := filepath.Join(svc.baseDir, appName, userID)
		require.NoError(t, os.Chmod(userDir, 0000))
		t.Cleanup(func() { _ = os.Chmod(userDir, 0750) })

		_, err = svc.List(ctx, &session.ListRequest{AppName: appName})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read dir")
	})

	t.Run("List without userID skips non-json entries and subdirs in user dir", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()
		appName := "app-" + fake.Lorem().Word()
		userID := "user-" + fake.UUID().V4()
		sessionID := uuid.New().String()

		_, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		require.NoError(t, err)

		userDir := filepath.Join(svc.baseDir, appName, userID)
		require.NoError(t, os.WriteFile(filepath.Join(userDir, "notes.txt"), []byte("x"), 0600))
		require.NoError(t, os.Mkdir(filepath.Join(userDir, "nested"), 0750))

		listResp, err := svc.List(ctx, &session.ListRequest{AppName: appName, UserID: userID})
		require.NoError(t, err)
		require.Len(t, listResp.Sessions, 1)
		assert.Equal(t, sessionID, listResp.Sessions[0].ID())
	})

	t.Run("Delete returns error when session path cannot be removed", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()
		appName := "app-" + fake.Lorem().Word()
		userID := "user-" + fake.UUID().V4()
		sessionID := uuid.New().String()

		_, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		require.NoError(t, err)

		path := sessionPath(svc.baseDir, appName, userID, sessionID)
		require.NoError(t, os.Remove(path))
		require.NoError(t, os.Mkdir(path, 0750))
		require.NoError(t, os.WriteFile(filepath.Join(path, "child"), []byte("x"), 0600))

		err = svc.Delete(ctx, &session.DeleteRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "remove session file")
	})

	t.Run("Delete prunes empty dirs but stops when sibling session exists", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()
		appName := "app-" + fake.Lorem().Word()
		userID := "user-" + fake.UUID().V4()
		s1 := uuid.New().String()
		s2 := uuid.New().String()

		_, err := svc.Create(ctx, &session.CreateRequest{AppName: appName, UserID: userID, SessionID: s1})
		require.NoError(t, err)
		_, err = svc.Create(ctx, &session.CreateRequest{AppName: appName, UserID: userID, SessionID: s2})
		require.NoError(t, err)

		require.NoError(t, svc.Delete(ctx, &session.DeleteRequest{
			AppName: appName, UserID: userID, SessionID: s1,
		}))

		userDir := filepath.Join(svc.baseDir, appName, userID)
		require.DirExists(t, userDir)
		_, err = svc.Get(ctx, &session.GetRequest{AppName: appName, UserID: userID, SessionID: s2})
		require.NoError(t, err)
	})

	t.Run("AppendEvent returns parse error when file is corrupt", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()
		createResp, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   "a",
			UserID:    "u",
			SessionID: uuid.New().String(),
		})
		require.NoError(t, err)
		fs := createResp.Session.(*fileSession)
		path := sessionPath(svc.baseDir, fs.appName, fs.userID, fs.id)
		require.NoError(t, os.WriteFile(path, []byte("{"), 0600))

		err = svc.AppendEvent(ctx, fs, &session.Event{ID: "e", Timestamp: time.Now()})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse session file")
	})

	t.Run("AppendEvent returns read error when file is not readable", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("chmod permissions differ from Unix")
		}
		svc := makeService(t)
		ctx := t.Context()
		createResp, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   "a",
			UserID:    "u",
			SessionID: uuid.New().String(),
		})
		require.NoError(t, err)
		fs := createResp.Session.(*fileSession)
		path := sessionPath(svc.baseDir, fs.appName, fs.userID, fs.id)
		require.NoError(t, os.Chmod(path, 0000))
		t.Cleanup(func() { _ = os.Chmod(path, 0600) })

		err = svc.AppendEvent(ctx, fs, &session.Event{ID: "e", Timestamp: time.Now()})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read session file")
	})

	t.Run("AppendEvent updates in-memory state when fs.state was nil", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()
		createResp, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   "a",
			UserID:    "u",
			SessionID: uuid.New().String(),
		})
		require.NoError(t, err)
		fs := createResp.Session.(*fileSession)
		fs.state = nil

		err = svc.AppendEvent(ctx, fs, &session.Event{
			ID:        "e",
			Timestamp: time.Now(),
			Actions:   session.EventActions{StateDelta: map[string]any{"sk": "sv"}},
		})
		require.NoError(t, err)
		got := collectState(fs.State())
		assert.Equal(t, "sv", got["sk"])
	})

	t.Run("AppendEvent fails when session cannot be marshaled after state delta", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()
		createResp, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   "a",
			UserID:    "u",
			SessionID: uuid.New().String(),
		})
		require.NoError(t, err)

		err = svc.AppendEvent(ctx, createResp.Session, &session.Event{
			ID:        "e",
			Timestamp: time.Now(),
			Actions:   session.EventActions{StateDelta: map[string]any{"sk": math.NaN()}},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "marshal session")
	})

	t.Run("AppendEvent fails when session file cannot be written", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("chmod permissions differ from Unix")
		}
		svc := makeService(t)
		ctx := t.Context()
		createResp, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   "a",
			UserID:    "u",
			SessionID: uuid.New().String(),
		})
		require.NoError(t, err)
		fs := createResp.Session.(*fileSession)
		path := sessionPath(svc.baseDir, fs.appName, fs.userID, fs.id)
		require.NoError(t, os.Chmod(path, 0400))
		t.Cleanup(func() { _ = os.Chmod(path, 0600) })

		err = svc.AppendEvent(ctx, fs, &session.Event{ID: "e", Timestamp: time.Now()})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "write session file")
	})

	t.Run("AppendEvent fails when user state delta cannot be serialized", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()
		createResp, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   "a",
			UserID:    "u",
			SessionID: uuid.New().String(),
		})
		require.NoError(t, err)

		ch := make(chan int)
		err = svc.AppendEvent(ctx, createResp.Session, &session.Event{
			ID:        "e",
			Timestamp: time.Now(),
			Actions: session.EventActions{
				StateDelta: map[string]any{"user:bad": ch},
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "write user state")
	})

	t.Run("readAppState returns empty map when JSON state field is null", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()
		appName := "app-" + fake.Lorem().Word()
		userID := "user-" + fake.UUID().V4()
		sessionID := uuid.New().String()

		appPath := appStatePath(svc.baseDir, appName)
		require.NoError(t, os.MkdirAll(filepath.Dir(appPath), 0750))
		require.NoError(t, os.WriteFile(appPath, []byte(`{"state":null,"updatedAt":"2020-01-01T00:00:00Z"}`), 0600))

		_, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
			State:     map[string]any{"k": "v"},
		})
		require.NoError(t, err)

		getResp, err := svc.Get(ctx, &session.GetRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		require.NoError(t, err)
		got := collectState(getResp.Session.State())
		assert.Equal(t, "v", got["k"])
	})

	t.Run("trimTempDeltaState returns early when StateDelta is empty", func(t *testing.T) {
		ev := &session.Event{}
		out := trimTempDeltaState(ev)
		assert.Same(t, ev, out)
		assert.Empty(t, ev.Actions.StateDelta)
	})
}

// appendEventWrongTypeSession implements session.Session but is not *fileSession.
type appendEventWrongTypeSession struct{}

func (appendEventWrongTypeSession) ID() string { return "id" }

func (appendEventWrongTypeSession) AppName() string { return "app" }

func (appendEventWrongTypeSession) UserID() string { return "u" }

func (appendEventWrongTypeSession) State() session.State { return nil }

func (appendEventWrongTypeSession) Events() session.Events { return nil }

func (appendEventWrongTypeSession) LastUpdateTime() time.Time { return time.Time{} }

func collectState(st session.State) map[string]any {
	out := maps.Collect(st.All())
	return out
}

func appendSessionEventsViaFile(t *testing.T, baseDir, appName, userID, sessionID string, count int) {
	t.Helper()
	path := sessionPath(baseDir, appName, userID, sessionID)
	data, err := readSessionFile(path)
	require.NoError(t, err)
	for i := 1; i <= count; i++ {
		data.Events = append(data.Events, &session.Event{
			ID:        strconv.Itoa(i),
			Author:    "user",
			Timestamp: time.Time{}.Add(time.Duration(i)),
		})
	}
	require.NoError(t, writeSessionFile(path, data))
}

func readSessionFile(path string) (*fileSessionStorage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var stored fileSessionStorage
	if err = json.Unmarshal(data, &stored); err != nil {
		return nil, err
	}
	return &stored, nil
}

func writeSessionFile(path string, data *fileSessionStorage) error {
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, bytes, 0600)
}
