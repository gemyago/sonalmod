package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/gemyago/sonalmod/apps/sonalmod/internal/system/ident"
)

var (
	ErrUserNotFound   = errors.New("user not found")
	ErrUsernameExists = errors.New("username already exists")
)

type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"passwordHash"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type CreateUserParams struct {
	Username     string
	PasswordHash string
}

type UserStoreDeps struct {
	DataDir string `name:"config.dataDir"`
	IDGen   ident.Generator
	Logger  *slog.Logger
}

type UserStore struct {
	deps UserStoreDeps
}

func NewUserStore(deps UserStoreDeps) *UserStore {
	return &UserStore{deps: deps}
}

func (s *UserStore) usersDir() string {
	return filepath.Join(s.deps.DataDir, "auth", "users")
}

func (s *UserStore) userFilePath(id string) string {
	return filepath.Join(s.usersDir(), id+".json")
}

func (s *UserStore) ensureUsersDir() error {
	if err := os.MkdirAll(s.usersDir(), 0o700); err != nil {
		return fmt.Errorf("create users directory: %w", err)
	}
	return nil
}

func (s *UserStore) writeUser(user *User) error {
	if err := s.ensureUsersDir(); err != nil {
		return err
	}

	data, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("marshal user: %w", err)
	}

	dest := s.userFilePath(user.ID)
	tmp := dest + ".tmp"

	err = os.WriteFile(tmp, data, 0o600)
	if err != nil {
		return fmt.Errorf("write temp user file: %w", err)
	}

	err = os.Rename(tmp, dest)
	if err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename user file: %w", err)
	}

	return nil
}

func (s *UserStore) readUser(id string) (*User, error) {
	data, err := os.ReadFile(s.userFilePath(id))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("read user file: %w", err)
	}

	var user User
	err = json.Unmarshal(data, &user)
	if err != nil {
		return nil, fmt.Errorf("unmarshal user: %w", err)
	}

	return &user, nil
}

func (s *UserStore) Create(ctx context.Context, params CreateUserParams) (*User, error) {
	// Check for duplicate username by scanning existing users.
	existing, err := s.GetByUsername(ctx, params.Username)
	if err != nil && !errors.Is(err, ErrUserNotFound) {
		return nil, fmt.Errorf("check username: %w", err)
	}
	if existing != nil {
		return nil, ErrUsernameExists
	}

	now := time.Now().UTC()
	user := &User{
		ID:           s.deps.IDGen.MustNewV7().String(),
		Username:     params.Username,
		PasswordHash: params.PasswordHash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	err = s.writeUser(user)
	if err != nil {
		return nil, fmt.Errorf("write user: %w", err)
	}

	s.deps.Logger.DebugContext(ctx, "user created", slog.String("userID", user.ID))
	return user, nil
}

func (s *UserStore) GetByID(_ context.Context, id string) (*User, error) {
	user, err := s.readUser(id)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (s *UserStore) GetByUsername(ctx context.Context, username string) (*User, error) {
	users, err := s.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}

	for _, u := range users {
		if u.Username == username {
			uCopy := u
			return &uCopy, nil
		}
	}

	return nil, ErrUserNotFound
}

func (s *UserStore) List(ctx context.Context) ([]User, error) {
	dir := s.usersDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []User{}, nil
		}
		return nil, fmt.Errorf("read users directory: %w", err)
	}

	users := make([]User, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if filepath.Ext(name) != ".json" {
			continue
		}

		// Skip temp files.
		if filepath.Ext(name[:len(name)-5]) == ".tmp" {
			continue
		}

		id := name[:len(name)-5] // strip .json
		var user *User
		user, err = s.readUser(id)
		if err != nil {
			s.deps.Logger.WarnContext(ctx, "failed to read user file, skipping",
				slog.String("file", name), slog.Any("error", err))
			continue
		}
		users = append(users, *user)
	}

	return users, nil
}

func (s *UserStore) UpdatePassword(ctx context.Context, id string, newHash string) error {
	user, err := s.readUser(id)
	if err != nil {
		return err
	}

	user.PasswordHash = newHash
	user.UpdatedAt = time.Now().UTC()

	err = s.writeUser(user)
	if err != nil {
		return fmt.Errorf("write user: %w", err)
	}

	s.deps.Logger.DebugContext(ctx, "user password updated", slog.String("userID", id))
	return nil
}
