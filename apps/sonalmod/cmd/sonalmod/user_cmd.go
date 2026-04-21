package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/gemyago/sonalmod/apps/sonalmod/internal/auth"
	"github.com/spf13/cobra"
	"go.uber.org/dig"
)

type userAddParams struct {
	Username string
	Password string //nolint:gosec // intentional: this is a CLI param for password input
}

type userChangePasswordParams struct {
	Username string
	Password string //nolint:gosec // intentional: this is a CLI param for password input
}

type userAddCmdDeps struct {
	dig.In

	Store  *auth.UserStore
	Hasher *auth.Argon2idHasher
}

type userListCmdDeps struct {
	dig.In

	Store *auth.UserStore
}

type userChangePasswordCmdDeps struct {
	dig.In

	Store  *auth.UserStore
	Hasher *auth.Argon2idHasher
}

func runUserAdd(
	ctx context.Context,
	deps userAddCmdDeps,
	params userAddParams,
	out io.Writer,
) error {
	hash, err := deps.Hasher.Hash(params.Password)
	if err != nil { // coverage-ignore // crypto/rand failure is not practically testable
		return fmt.Errorf("hash password: %w", err)
	}

	user, err := deps.Store.Create(ctx, auth.CreateUserParams{
		Username:     params.Username,
		PasswordHash: hash,
	})
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	_, err = fmt.Fprintf(out, "User created: id=%s username=%s\n", user.ID, user.Username)
	return err
}

func runUserList(ctx context.Context, deps userListCmdDeps, out io.Writer) error {
	users, err := deps.Store.List(ctx)
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tUsername\tCreatedAt")
	for _, u := range users {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", u.ID, u.Username, u.CreatedAt.Format("2006-01-02 15:04:05"))
	}
	return w.Flush()
}

func runUserChangePassword(
	ctx context.Context,
	deps userChangePasswordCmdDeps,
	params userChangePasswordParams,
	out io.Writer,
) error {
	user, err := deps.Store.GetByUsername(ctx, params.Username)
	if err != nil {
		return fmt.Errorf("find user: %w", err)
	}

	hash, err := deps.Hasher.Hash(params.Password)
	if err != nil { // coverage-ignore // crypto/rand failure is not practically testable
		return fmt.Errorf("hash password: %w", err)
	}

	if err = deps.Store.UpdatePassword(ctx, user.ID, hash); err != nil {
		return fmt.Errorf("update password: %w", err)
	}

	_, err = fmt.Fprintf(out, "Password updated for user: username=%s\n", user.Username)
	return err
}

func newUserAddCmd(container *dig.Container) *cobra.Command { // coverage-ignore
	var params userAddParams

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new user",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := newEngineFromRoot(cmd.Root(), container); err != nil {
				return err
			}
			return container.Invoke(func(deps userAddCmdDeps) error {
				return runUserAdd(cmd.Context(), deps, params, cmd.OutOrStdout())
			})
		},
	}
	cmd.Flags().StringVar(&params.Username, "username", "", "Username for the new user")
	cmd.Flags().StringVar(&params.Password, "password", "", "Password for the new user")
	_ = cmd.MarkFlagRequired("username")
	_ = cmd.MarkFlagRequired("password")
	return cmd
}

func newUserListCmd(container *dig.Container) *cobra.Command { // coverage-ignore
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all users",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := newEngineFromRoot(cmd.Root(), container); err != nil {
				return err
			}
			return container.Invoke(func(deps userListCmdDeps) error {
				return runUserList(cmd.Context(), deps, cmd.OutOrStdout())
			})
		},
	}
	return cmd
}

func newUserChangePasswordCmd(container *dig.Container) *cobra.Command { // coverage-ignore
	var params userChangePasswordParams

	cmd := &cobra.Command{
		Use:   "change-password",
		Short: "Change a user's password",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := newEngineFromRoot(cmd.Root(), container); err != nil {
				return err
			}
			return container.Invoke(func(deps userChangePasswordCmdDeps) error {
				return runUserChangePassword(cmd.Context(), deps, params, cmd.OutOrStdout())
			})
		},
	}
	cmd.Flags().StringVar(&params.Username, "username", "", "Username of the user")
	cmd.Flags().StringVar(&params.Password, "password", "", "New password")
	_ = cmd.MarkFlagRequired("username")
	_ = cmd.MarkFlagRequired("password")
	return cmd
}

func newUserCmd(container *dig.Container) *cobra.Command { // coverage-ignore
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage users",
	}
	cmd.AddCommand(
		newUserAddCmd(container),
		newUserListCmd(container),
		newUserChangePasswordCmd(container),
	)
	return cmd
}
