package sessions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"maps"

	"github.com/google/uuid"
	"google.golang.org/adk/session"
)

// FileSessionsStorage unifies file-backed ADK session persistence and session metadata.
type FileSessionsStorage struct {
	*FileSessionService

	meta *FileSessionMetadataStore
}

// NewFileSessionsStorage returns concrete *FileSessionsStorage (accept interface, return struct).
func NewFileSessionsStorage(baseDir string, logger *slog.Logger) (*FileSessionsStorage, error) {
	svc, err := NewFileSessionService(baseDir, logger)
	if err != nil {
		return nil, err
	}
	meta, err := NewFileSessionMetadataStore(baseDir)
	if err != nil {
		return nil, err
	}
	return &FileSessionsStorage{
		FileSessionService: svc,
		meta:               meta,
	}, nil
}

func (s *FileSessionsStorage) SaveMetadata(ctx context.Context, m SessionMetadata) error {
	return s.meta.Save(ctx, m)
}

func (s *FileSessionsStorage) ListMetadata(
	ctx context.Context,
	p ListSessionMetadataParams,
) (*ListSessionMetadataResult, error) {
	return s.meta.List(ctx, p)
}

func (s *FileSessionsStorage) DeleteMetadata(ctx context.Context, appName, userID, sessionID string) error {
	return s.meta.Delete(ctx, appName, userID, sessionID)
}

func (s *FileSessionsStorage) AutoMigrate() error {
	return nil
}

var _ SessionsStorage = (*FileSessionsStorage)(nil)
var _ session.Service = (*FileSessionsStorage)(nil)

// FileSessionMetadataStore persists session metadata in a JSON index file per app and user.
type FileSessionMetadataStore struct {
	baseDir string
	mu      sync.RWMutex
}

// NewFileSessionMetadataStore returns file-backed session metadata storage under baseDir.
// Layout: {baseDir}/{appName}/{userID}/_sessions_index.json
// baseDir must be non-empty; it is created if missing.
func NewFileSessionMetadataStore(
	baseDir string,
) (
	*FileSessionMetadataStore,
	error,
) {
	if baseDir == "" {
		return nil, errors.New("base_dir is required")
	}
	if err := os.MkdirAll(baseDir, 0750); err != nil {
		return nil, fmt.Errorf("create base dir: %w", err)
	}
	return &FileSessionMetadataStore{baseDir: baseDir}, nil
}

func sessionMetadataIndexPath(baseDir, appName, userID string) string {
	return filepath.Join(baseDir, appName, userID, "_sessions_index.json")
}

// sessionMetadataRecord is the on-disk JSON shape (camelCase keys).
type sessionMetadataRecord struct {
	SessionID string    `json:"sessionId"`
	AppName   string    `json:"appName"`
	UserID    string    `json:"userID"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func toRecord(m SessionMetadata) sessionMetadataRecord {
	return sessionMetadataRecord(m)
}

func fromRecord(r sessionMetadataRecord) SessionMetadata {
	return SessionMetadata(r)
}

var _ SessionMetadataStore = (*FileSessionMetadataStore)(nil)

func (s *FileSessionMetadataStore) Save(ctx context.Context, metadata SessionMetadata) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidateMetadataForSave(metadata); err != nil {
		return err
	}
	path := sessionMetadataIndexPath(s.baseDir, metadata.AppName, metadata.UserID)
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := readIndex(path)
	if err != nil {
		return fmt.Errorf("read index: %w", err)
	}
	var found bool
	for i := range entries {
		if entries[i].SessionID == metadata.SessionID {
			entries[i] = metadata
			found = true
			break
		}
	}
	if !found {
		entries = append(entries, metadata)
	}
	if werr := writeIndexAtomic(path, entries); werr != nil {
		return fmt.Errorf("write index: %w", werr)
	}
	return nil
}

func (s *FileSessionMetadataStore) List(
	ctx context.Context,
	params ListSessionMetadataParams,
) (*ListSessionMetadataResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := ValidateListParams(params); err != nil {
		return nil, err
	}
	path := sessionMetadataIndexPath(s.baseDir, params.AppName, params.UserID)
	s.mu.RLock()
	defer s.mu.RUnlock()
	all, err := readIndex(path)
	if err != nil {
		return nil, fmt.Errorf("read index: %w", err)
	}
	var filtered []SessionMetadata
	for _, m := range all {
		if m.AppName == params.AppName && m.UserID == params.UserID {
			filtered = append(filtered, m)
		}
	}
	sortByUpdatedAtDesc(filtered)
	total := len(filtered)
	start := min(params.Offset, total)
	end := min(start+params.Limit, total)
	page := filtered[start:end]
	out := make([]SessionMetadata, len(page))
	copy(out, page)
	return &ListSessionMetadataResult{Sessions: out, Total: total}, nil
}

func (s *FileSessionMetadataStore) Delete(ctx context.Context, appName, userID, sessionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if appName == "" || userID == "" {
		return errors.New("app_name and user_id are required")
	}
	if sessionID == "" {
		return errors.New("session_id is required")
	}
	path := sessionMetadataIndexPath(s.baseDir, appName, userID)
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := readIndex(path)
	if err != nil {
		return fmt.Errorf("read index: %w", err)
	}
	before := len(entries)
	entries = slices.DeleteFunc(entries, func(m SessionMetadata) bool {
		return m.SessionID == sessionID
	})
	if len(entries) == before {
		return nil
	}
	if werr := writeIndexAtomic(path, entries); werr != nil {
		return fmt.Errorf("write index: %w", werr)
	}
	return nil
}

func readIndex(path string) ([]SessionMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var records []sessionMetadataRecord
	if uerr := json.Unmarshal(data, &records); uerr != nil {
		return nil, fmt.Errorf("parse session metadata index: %w", uerr)
	}
	out := make([]SessionMetadata, len(records))
	for i := range records {
		out[i] = fromRecord(records[i])
	}
	return out, nil
}

func writeIndexAtomic(path string, sessions []SessionMetadata) error {
	records := make([]sessionMetadataRecord, len(sessions))
	for i := range sessions {
		records[i] = toRecord(sessions[i])
	}
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session metadata index: %w", err)
	}
	dir := filepath.Dir(path)
	if err = os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("ensure metadata dir: %w", err)
	}
	f, err := os.CreateTemp(dir, "_sessions_index.*.tmp")
	if err != nil {
		return fmt.Errorf("create temp index: %w", err)
	}
	tmpPath := f.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, werr := f.Write(data); werr != nil {
		_ = f.Close()
		return fmt.Errorf("write temp index: %w", werr)
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync temp index: %w", err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("close temp index: %w", err)
	}
	// tmpPath is from CreateTemp under dir; path is the index under the same tree.
	//nolint:gosec // G703: paths are confined to baseDir/session index layout (no user-controlled traversal).
	if err = os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename index file: %w", err)
	}
	ok = true
	return nil
}

// extractStateDeltas splits a state delta map into app, user, and session maps
// based on key prefixes. Keys with "app:" go to app, "user:" to user,
// unprefixed (and not "temp:") go to session. Temp keys are excluded.
func extractStateDeltas(delta map[string]any) (map[string]any, map[string]any, map[string]any) {
	app := make(map[string]any)
	user := make(map[string]any)
	sess := make(map[string]any)

	if delta == nil {
		return app, user, sess
	}

	for key, value := range delta {
		var stripped string
		var hasPrefix bool
		if stripped, hasPrefix = strings.CutPrefix(key, session.KeyPrefixApp); hasPrefix {
			app[stripped] = value
		} else if stripped, hasPrefix = strings.CutPrefix(key, session.KeyPrefixUser); hasPrefix {
			user[stripped] = value
		} else if !strings.HasPrefix(key, session.KeyPrefixTemp) {
			sess[key] = value
		}
	}
	return app, user, sess
}

// mergeStates combines app, user, and session state maps into a single map
// with appropriate prefixes (app:, user:) for app and user keys.
func mergeStates(appState, userState, sessionState map[string]any) map[string]any {
	totalSize := len(appState) + len(userState) + len(sessionState)
	merged := make(map[string]any, totalSize)

	maps.Copy(merged, sessionState)

	for key, value := range appState {
		merged[session.KeyPrefixApp+key] = value
	}

	for key, value := range userState {
		merged[session.KeyPrefixUser+key] = value
	}

	return merged
}

// fileSession implements session.Session for file-based persistence.
type fileSession struct {
	id        string
	appName   string
	userID    string
	state     map[string]any
	events    []*session.Event
	updatedAt time.Time
	mu        sync.RWMutex
}

// Ensure fileSession implements session.Session.
var _ session.Session = (*fileSession)(nil)

func (s *fileSession) ID() string {
	return s.id
}

func (s *fileSession) AppName() string {
	return s.appName
}

func (s *fileSession) UserID() string {
	return s.userID
}

//nolint:ireturn // ADK session contract requires interface return types.
func (s *fileSession) State() session.State {
	return &fileSessionState{
		mu:    &s.mu,
		state: s.state,
	}
}

//nolint:ireturn // ADK session contract requires interface return types.
func (s *fileSession) Events() session.Events {
	return fileSessionEvents(s.events)
}

func (s *fileSession) LastUpdateTime() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.updatedAt
}

// fileSessionState implements session.State for fileSession.
type fileSessionState struct {
	mu    *sync.RWMutex
	state map[string]any
}

func (st *fileSessionState) Get(key string) (any, error) {
	st.mu.RLock()
	defer st.mu.RUnlock()
	val, ok := st.state[key]
	if !ok {
		return nil, session.ErrStateKeyNotExist
	}
	return val, nil
}

func (st *fileSessionState) Set(key string, value any) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.state == nil {
		st.state = make(map[string]any)
	}
	st.state[key] = value
	return nil
}

func (st *fileSessionState) All() iter.Seq2[string, any] {
	return func(yield func(key string, val any) bool) {
		st.mu.RLock()
		for k, v := range st.state {
			st.mu.RUnlock()
			if !yield(k, v) {
				return
			}
			st.mu.RLock()
		}
		st.mu.RUnlock()
	}
}

// fileSessionEvents implements session.Events for fileSession.
type fileSessionEvents []*session.Event

func (e fileSessionEvents) All() iter.Seq[*session.Event] {
	return func(yield func(*session.Event) bool) {
		for _, event := range e {
			if !yield(event) {
				return
			}
		}
	}
}

func (e fileSessionEvents) Len() int {
	return len(e)
}

func (e fileSessionEvents) At(i int) *session.Event {
	if i >= 0 && i < len(e) {
		return e[i]
	}
	return nil
}

// fileSessionStorage is the JSON persistence struct for session files.
// Used when reading/writing session JSON in Create, Get, AppendEvent.
type fileSessionStorage struct {
	AppName   string           `json:"appName"`
	UserID    string           `json:"userID"`
	SessionID string           `json:"sessionID"`
	State     map[string]any   `json:"state"`
	UpdatedAt time.Time        `json:"updatedAt"`
	Events    []*session.Event `json:"events"`
}

// FileSessionService implements session.Service with file system persistence.
type FileSessionService struct {
	baseDir string
	logger  *slog.Logger
	mu      sync.RWMutex
}

// Ensure FileSessionService implements session.Service.
var _ session.Service = (*FileSessionService)(nil)

// NewFileSessionService creates a file-backed [session.Service] implementation.
// baseDir must be non-empty; it will be created if it does not exist.
// logger may be nil.
func NewFileSessionService(
	baseDir string,
	logger *slog.Logger,
) (
	*FileSessionService,
	error,
) {
	if baseDir == "" {
		return nil, errors.New("base_dir is required")
	}
	if err := os.MkdirAll(baseDir, 0750); err != nil {
		return nil, fmt.Errorf("create base dir: %w", err)
	}
	return &FileSessionService{
		baseDir: baseDir,
		logger:  logger,
	}, nil
}

func sessionPath(baseDir, appName, userID, sessionID string) string {
	return filepath.Join(baseDir, appName, userID, sessionID+".json")
}

func appStatePath(baseDir, appName string) string {
	return filepath.Join(baseDir, "_app", appName+".json")
}

func userStatePath(baseDir, appName, userID string) string {
	return filepath.Join(baseDir, "_user", appName, userID+".json")
}

func (s *FileSessionService) Create(ctx context.Context, req *session.CreateRequest) (*session.CreateResponse, error) {
	_ = ctx // required by session.Service interface
	if req.AppName == "" || req.UserID == "" {
		return nil, fmt.Errorf(
			"app_name and user_id are required, got app_name: %q, user_id: %q",
			req.AppName, req.UserID,
		)
	}

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = uuid.NewString()
	}

	path := sessionPath(s.baseDir, req.AppName, req.UserID, sessionID)

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("session %s already exists", sessionID)
	}

	state := req.State
	if state == nil {
		state = make(map[string]any)
	}

	appDelta, userDelta, sessionDelta := extractStateDeltas(state)

	// Update app state file
	if len(appDelta) > 0 {
		appPath := appStatePath(s.baseDir, req.AppName)
		if err := s.ensureDir(filepath.Dir(appPath)); err != nil {
			return nil, fmt.Errorf("ensure app state dir: %w", err)
		}
		appState := s.readAppState(appPath)
		maps.Copy(appState, appDelta)
		if err := s.writeStateFile(appPath, appState); err != nil {
			return nil, fmt.Errorf("write app state: %w", err)
		}
	}

	// Update user state file
	if len(userDelta) > 0 {
		userPath := userStatePath(s.baseDir, req.AppName, req.UserID)
		if err := s.ensureDir(filepath.Dir(userPath)); err != nil {
			return nil, fmt.Errorf("ensure user state dir: %w", err)
		}
		userState := s.readUserState(userPath)
		maps.Copy(userState, userDelta)
		if err := s.writeStateFile(userPath, userState); err != nil {
			return nil, fmt.Errorf("write user state: %w", err)
		}
	}

	mergedState := mergeStates(
		s.readAppState(appStatePath(s.baseDir, req.AppName)),
		s.readUserState(userStatePath(s.baseDir, req.AppName, req.UserID)),
		sessionDelta,
	)

	now := time.Now()
	stored := fileSessionStorage{
		AppName:   req.AppName,
		UserID:    req.UserID,
		SessionID: sessionID,
		State:     mergedState,
		UpdatedAt: now,
		Events:    nil,
	}

	if err := s.ensureDir(filepath.Dir(path)); err != nil {
		return nil, fmt.Errorf("ensure session dir: %w", err)
	}
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal session: %w", err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		return nil, fmt.Errorf("write session file: %w", err)
	}

	sess := &fileSession{
		id:        sessionID,
		appName:   req.AppName,
		userID:    req.UserID,
		state:     maps.Clone(mergedState),
		events:    nil,
		updatedAt: now,
	}
	return &session.CreateResponse{Session: sess}, nil
}

func (s *FileSessionService) Get(ctx context.Context, req *session.GetRequest) (*session.GetResponse, error) {
	_ = ctx // required by session.Service interface
	appName, userID, sessionID := req.AppName, req.UserID, req.SessionID
	if appName == "" || userID == "" || sessionID == "" {
		return nil, fmt.Errorf(
			"app_name, user_id, session_id are required, got app_name: %q, user_id: %q, session_id: %q",
			appName, userID, sessionID,
		)
	}

	path := sessionPath(s.baseDir, appName, userID, sessionID)

	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("session %s not found", sessionID)
		}
		return nil, fmt.Errorf("read session file: %w", err)
	}

	var stored fileSessionStorage
	if err = json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("parse session file: %w", err)
	}

	appState := s.readAppState(appStatePath(s.baseDir, appName))
	userState := s.readUserState(userStatePath(s.baseDir, appName, userID))
	_, _, sessionState := extractStateDeltas(stored.State)
	mergedState := mergeStates(appState, userState, sessionState)

	filteredEvents := stored.Events
	if filteredEvents == nil {
		filteredEvents = []*session.Event{}
	}

	if req.NumRecentEvents > 0 {
		start := max(len(filteredEvents)-req.NumRecentEvents, 0)
		filteredEvents = filteredEvents[start:]
	}
	if !req.After.IsZero() && len(filteredEvents) > 0 {
		firstIndexToKeep := sort.Search(len(filteredEvents), func(i int) bool {
			return !filteredEvents[i].Timestamp.Before(req.After)
		})
		filteredEvents = filteredEvents[firstIndexToKeep:]
	}

	sess := &fileSession{
		id:        sessionID,
		appName:   appName,
		userID:    userID,
		state:     mergedState,
		events:    filteredEvents,
		updatedAt: stored.UpdatedAt,
	}
	return &session.GetResponse{Session: sess}, nil
}

func (s *FileSessionService) List(ctx context.Context, req *session.ListRequest) (*session.ListResponse, error) {
	_ = ctx
	appName, userID := req.AppName, req.UserID
	if appName == "" {
		return nil, fmt.Errorf("app_name is required, got app_name: %q", appName)
	}

	scanDir := filepath.Join(s.baseDir, appName)
	if userID != "" {
		scanDir = filepath.Join(scanDir, userID)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(scanDir)
	if err != nil {
		if os.IsNotExist(err) {
			return &session.ListResponse{Sessions: []session.Session{}}, nil
		}
		return nil, fmt.Errorf("read dir %s: %w", scanDir, err)
	}

	var sessions []session.Session
	for _, entry := range entries {
		if entry.IsDir() {
			if userID != "" {
				continue
			}
			userDir := filepath.Join(scanDir, entry.Name())
			userSessions, listErr := s.listSessionsInDir(appName, entry.Name(), userDir)
			if listErr != nil {
				return nil, listErr
			}
			sessions = append(sessions, userSessions...)
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		sessionID := strings.TrimSuffix(entry.Name(), ".json")
		sess, loadErr := s.loadSessionMetadata(appName, userID, sessionID)
		if loadErr != nil {
			return nil, loadErr
		}
		sessions = append(sessions, sess)
	}

	return &session.ListResponse{Sessions: sessions}, nil
}

func (s *FileSessionService) listSessionsInDir(appName, userID, dir string) ([]session.Session, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}
	var sessions []session.Session
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		sessionID := strings.TrimSuffix(entry.Name(), ".json")
		sess, loadErr := s.loadSessionMetadata(appName, userID, sessionID)
		if loadErr != nil {
			return nil, loadErr
		}
		sessions = append(sessions, sess)
	}
	return sessions, nil
}

func (s *FileSessionService) loadSessionMetadata(
	appName, userID, sessionID string,
) (*fileSession, error) {
	path := sessionPath(s.baseDir, appName, userID, sessionID)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read session file %s: %w", path, err)
	}
	var stored fileSessionStorage
	if err = json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("parse session file %s: %w", path, err)
	}
	appState := s.readAppState(appStatePath(s.baseDir, appName))
	userState := s.readUserState(userStatePath(s.baseDir, appName, userID))
	_, _, sessionState := extractStateDeltas(stored.State)
	mergedState := mergeStates(appState, userState, sessionState)
	events := stored.Events
	if events == nil {
		events = []*session.Event{}
	}
	return &fileSession{
		id:        sessionID,
		appName:   appName,
		userID:    userID,
		state:     mergedState,
		events:    events,
		updatedAt: stored.UpdatedAt,
	}, nil
}

func (s *FileSessionService) Delete(ctx context.Context, req *session.DeleteRequest) error {
	_ = ctx
	appName, userID, sessionID := req.AppName, req.UserID, req.SessionID
	if appName == "" || userID == "" || sessionID == "" {
		return fmt.Errorf(
			"app_name, user_id, session_id are required, got app_name: %q, user_id: %q, session_id: %q",
			appName, userID, sessionID,
		)
	}

	path := sessionPath(s.baseDir, appName, userID, sessionID)

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("session %s not found", sessionID)
		}
		return fmt.Errorf("remove session file: %w", err)
	}

	s.pruneEmptyDirs(filepath.Dir(path), filepath.Join(s.baseDir, appName))
	return nil
}

func (s *FileSessionService) pruneEmptyDirs(dir, stopAt string) {
	for dir != stopAt && dir != "." {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		parent := filepath.Dir(dir)
		_ = os.Remove(dir)
		dir = parent
	}
}

func (s *FileSessionService) AppendEvent(ctx context.Context, sess session.Session, event *session.Event) error {
	_ = ctx
	if event == nil {
		return errors.New("event is nil")
	}
	if event.Partial {
		return nil
	}

	fs, ok := sess.(*fileSession)
	if !ok {
		return fmt.Errorf("unexpected session type %T", sess)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := sessionPath(s.baseDir, fs.appName, fs.userID, fs.id)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("session %s not found", fs.id)
		}
		return fmt.Errorf("read session file: %w", err)
	}

	var stored fileSessionStorage
	if err = json.Unmarshal(data, &stored); err != nil {
		return fmt.Errorf("parse session file: %w", err)
	}

	event = trimTempDeltaState(event)
	if applyErr := s.applyEventStateDeltas(fs, event, &stored); applyErr != nil {
		return applyErr
	}

	if stored.Events == nil {
		stored.Events = []*session.Event{}
	}
	stored.Events = append(stored.Events, event)
	stored.UpdatedAt = event.Timestamp

	// Update in-memory session so callers (e.g. ADK runner) see the new event.
	// ADK InMemoryService does this; file-backed sessions must match for agents
	// that build LLM requests from session.Events().
	if fs.events == nil {
		fs.events = []*session.Event{}
	}
	fs.events = append(fs.events, event)
	fs.updatedAt = event.Timestamp
	if len(event.Actions.StateDelta) > 0 {
		_, _, sessionDelta := extractStateDeltas(event.Actions.StateDelta)
		if len(sessionDelta) > 0 {
			if fs.state == nil {
				fs.state = make(map[string]any)
			}
			maps.Copy(fs.state, sessionDelta)
		}
	}

	out, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	if err = os.WriteFile(path, out, 0600); err != nil {
		return fmt.Errorf("write session file: %w", err)
	}

	return nil
}

func (s *FileSessionService) applyEventStateDeltas(
	fs *fileSession, event *session.Event, stored *fileSessionStorage,
) error {
	if len(event.Actions.StateDelta) == 0 {
		return nil
	}
	appDelta, userDelta, sessionDelta := extractStateDeltas(event.Actions.StateDelta)

	if len(appDelta) > 0 {
		appPath := appStatePath(s.baseDir, fs.appName)
		if dirErr := s.ensureDir(filepath.Dir(appPath)); dirErr != nil {
			return fmt.Errorf("ensure app state dir: %w", dirErr)
		}
		appState := s.readAppState(appPath)
		maps.Copy(appState, appDelta)
		if writeErr := s.writeStateFile(appPath, appState); writeErr != nil {
			return fmt.Errorf("write app state: %w", writeErr)
		}
	}

	if len(userDelta) > 0 {
		userPath := userStatePath(s.baseDir, fs.appName, fs.userID)
		if dirErr := s.ensureDir(filepath.Dir(userPath)); dirErr != nil {
			return fmt.Errorf("ensure user state dir: %w", dirErr)
		}
		userState := s.readUserState(userPath)
		maps.Copy(userState, userDelta)
		if writeErr := s.writeStateFile(userPath, userState); writeErr != nil {
			return fmt.Errorf("write user state: %w", writeErr)
		}
	}

	if len(sessionDelta) > 0 {
		if stored.State == nil {
			stored.State = make(map[string]any)
		}
		maps.Copy(stored.State, sessionDelta)
	}

	return nil
}

// trimTempDeltaState removes temporary state delta keys from the event.
func trimTempDeltaState(event *session.Event) *session.Event {
	if len(event.Actions.StateDelta) == 0 {
		return event
	}
	filtered := make(map[string]any)
	for key, value := range event.Actions.StateDelta {
		if !strings.HasPrefix(key, session.KeyPrefixTemp) {
			filtered[key] = value
		}
	}
	event.Actions.StateDelta = filtered
	return event
}

func (s *FileSessionService) ensureDir(dir string) error {
	return os.MkdirAll(dir, 0750)
}

type stateFile struct {
	State     map[string]any `json:"state"`
	UpdatedAt time.Time      `json:"updatedAt"`
}

func (s *FileSessionService) readAppState(path string) map[string]any {
	data, err := os.ReadFile(path)
	if err != nil {
		return make(map[string]any)
	}
	var f stateFile
	if err = json.Unmarshal(data, &f); err != nil {
		return make(map[string]any)
	}
	if f.State == nil {
		return make(map[string]any)
	}
	return f.State
}

func (s *FileSessionService) readUserState(path string) map[string]any {
	return s.readAppState(path)
}

func (s *FileSessionService) writeStateFile(path string, state map[string]any) error {
	f := stateFile{State: state, UpdatedAt: time.Now()}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
