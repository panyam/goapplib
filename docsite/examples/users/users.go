// Package users is the example on the UsersService reference page: profiles kept by goapplib's
// filesystem backend, made or refreshed after a login with EnsureUser, with app-specific data in
// Extras. users_test.go runs it against a temporary directory.
package users

import (
	"context"

	v1 "github.com/panyam/goapplib/gen/go/goapplib/v1"
	"github.com/panyam/goapplib/services"
	fsusers "github.com/panyam/goapplib/services/backends/fs"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

// NewService keeps profiles as files under dir. It's the development backend; gorm and gae have
// the same constructor shape over a database.
func NewService(dir string) services.UsersService {
	return fsusers.NewUsersService(dir)
}

// AfterLogin makes sure the user who just logged in has a profile. The first login creates it;
// later ones update the name, email and image if the login supplied them, and leave them alone if
// it didn't.
func AfterLogin(ctx context.Context, users services.UsersService, id, name, email string) (*v1.User, error) {
	return users.EnsureUser(ctx, id, name, email, "")
}

// SetTheme keeps an app-specific preference in the profile's Extras, a protobuf Struct, so the app
// doesn't need its own user table for it.
func SetTheme(ctx context.Context, users services.UsersService, id, theme string) error {
	got, err := users.GetUser(ctx, &v1.GetUserRequest{Id: id})
	if err != nil {
		return err
	}
	extras, err := structpb.NewStruct(map[string]any{"theme": theme})
	if err != nil {
		return err
	}
	// Edit a copy: GetUser can return the service's cached user itself (issue 114), and changing
	// that changes the cache whether or not the update below succeeds.
	user := proto.Clone(got.User).(*v1.User)
	user.Extras = extras
	_, err = users.UpdateUser(ctx, &v1.UpdateUserRequest{User: user})
	return err
}
