package services

import (
	v1 "github.com/panyam/goapplib/gen/go/goapplib/v1"
)

// UserBridge wraps a goapplib.v1.User to implement the oneauth accounts.User
// interface. This allows seamless integration between goapplib's User proto and
// oneauth's authentication system.
//
// # Usage
//
// After authenticating with AuthService, create a UserBridge to pass to
// systems that expect a accounts.User:
//
//	user := &v1.User{Id: "123", Name: "John"}
//	bridge := services.NewUserBridge(user)
//	// bridge now implements accounts.User interface
//	fmt.Println(bridge.Id())       // "123"
//	fmt.Println(bridge.Profile())  // map[name:John ...]
//
// # accounts.User Interface
//
// The accounts.User interface requires:
//   - Id() string: returns the user's unique identifier
//   - Profile() map[string]any: returns user profile data as a map
//
// UserBridge satisfies this interface by extracting data from the wrapped
// goapplib.v1.User proto message.
type UserBridge struct {
	user *v1.User
}

// NewUserBridge creates a UserBridge wrapping the given User proto.
func NewUserBridge(user *v1.User) *UserBridge {
	return &UserBridge{user: user}
}

// Id implements accounts.User interface.
// Returns the user's unique identifier.
func (u *UserBridge) Id() string {
	if u.user == nil {
		return ""
	}
	return u.user.GetId()
}

// Profile implements accounts.User interface.
// Returns user profile data as a map, including name, email, image_url,
// description, and tags. The extras field is also included if present.
func (u *UserBridge) Profile() map[string]any {
	if u.user == nil {
		return nil
	}

	profile := map[string]any{
		"name":        u.user.GetName(),
		"email":       u.user.GetEmail(),
		"image_url":   u.user.GetImageUrl(),
		"description": u.user.GetDescription(),
		"tags":        u.user.GetTags(),
	}

	// Include extras if present
	if u.user.Extras != nil {
		profile["extras"] = u.user.Extras.AsMap()
	}

	return profile
}

// User returns the underlying User proto.
// Useful when you need to access the full proto message.
func (u *UserBridge) User() *v1.User {
	return u.user
}

// Verify that UserBridge implements the required interface at compile time.
// This uses a pattern that ensures interface compliance without runtime overhead.
var _ interface {
	Id() string
	Profile() map[string]any
} = (*UserBridge)(nil)
