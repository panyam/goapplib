package users

import (
	"context"
	"testing"

	v1 "github.com/panyam/goapplib/gen/go/goapplib/v1"
)

func TestAProfileFromLoginToPreferences(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	users := NewService(dir)

	first, err := AfterLogin(ctx, users, "u1", "Ada", "ada@example.com")
	if err != nil || first.Name != "Ada" || first.CreatedAt == nil {
		t.Fatalf("first login: %v, %+v", err, first)
	}
	// A later login without a name keeps the one already there, and refreshes the email.
	again, err := AfterLogin(ctx, users, "u1", "", "ada@lovelace.org")
	if err != nil || again.Name != "Ada" || again.Email != "ada@lovelace.org" {
		t.Fatalf("second login: %v, %+v", err, again)
	}

	if err := SetTheme(ctx, users, "u1", "dark"); err != nil {
		t.Fatal(err)
	}
	// A fresh service over the same directory reads what was saved, not this one's cache.
	got, err := NewService(dir).GetUser(ctx, &v1.GetUserRequest{Id: "u1"})
	if err != nil || got.User.GetExtras().GetFields()["theme"].GetStringValue() != "dark" {
		t.Fatalf("after SetTheme: %v, %+v", err, got)
	}
	if _, err := users.GetUser(ctx, &v1.GetUserRequest{Id: "nobody"}); err == nil {
		t.Error("GetUser of an unknown id succeeded")
	}
}
