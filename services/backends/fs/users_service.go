//go:build !wasm
// +build !wasm

// Package fs provides a filesystem-based implementation of the UsersService.
// It stores user data as JSON files on the local filesystem, making it ideal
// for development, testing, and single-instance deployments.
//
// # Storage Layout
//
// Users are stored in the following directory structure:
//
//	{storageDir}/
//	  {userId}/
//	    user.json    # User profile data
//
// # Thread Safety
//
// The service includes optional in-memory caching via BaseUsersService.
// File operations are atomic via the goutils/storage package.
//
// # Usage
//
//	userService := fs.NewUsersService("/path/to/users")
//	resp, err := userService.CreateUser(ctx, &v1.CreateUserRequest{
//	    User: &v1.User{Name: "John Doe", Email: "john@example.com"},
//	})
package fs

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"time"

	v1 "github.com/panyam/goapplib/gen/go/goapplib/v1"
	"github.com/panyam/goapplib/services"
	"github.com/panyam/goutils/storage"
	tspb "google.golang.org/protobuf/types/known/timestamppb"
)

// UsersService implements services.UsersService using filesystem storage.
// It embeds BaseUsersService to inherit caching and shared operations.
type UsersService struct {
	services.BaseUsersService
	storage *storage.FileStorage
}

// NewUsersService creates a new filesystem-backed UsersService
func NewUsersService(storageDir string) *UsersService {
	service := &UsersService{storage: storage.NewFileStorage(storageDir)}
	service.Self = service
	service.StorageProvider = service
	service.InitializeCache()
	return service
}

// LoadUser implements UserStorageProvider
func (s *UsersService) LoadUser(ctx context.Context, id string) (*v1.User, error) {
	return storage.LoadFSArtifact[*v1.User](s.storage, id, "user")
}

// ListAllUsers implements UserStorageProvider
func (s *UsersService) ListAllUsers(ctx context.Context) ([]*v1.User, error) {
	return storage.ListFSEntities[*v1.User](s.storage, nil)
}

// SaveUser implements UserStorageProvider
func (s *UsersService) SaveUser(ctx context.Context, id string, user *v1.User) error {
	return s.storage.SaveArtifact(id, "user", user)
}

// DeleteFromStorage implements UserStorageProvider
func (s *UsersService) DeleteFromStorage(ctx context.Context, id string) error {
	return s.storage.DeleteEntity(id)
}

// UserExists implements UserStorageProvider
func (s *UsersService) UserExists(ctx context.Context, id string) bool {
	_, err := s.LoadUser(ctx, id)
	return err == nil
}

// CreateUser creates a new user profile
func (s *UsersService) CreateUser(ctx context.Context, req *v1.CreateUserRequest) (*v1.CreateUserResponse, error) {
	resp := &v1.CreateUserResponse{}
	if req.User == nil {
		return nil, fmt.Errorf("user data is required")
	}

	userId, err := s.storage.CreateEntity(req.User.Id)
	if err != nil {
		// Check if this is an ID conflict
		if req.User.Id != "" {
			suggestedId := req.User.Id + "-" + shortRandSuffix()
			resp.FieldErrors = map[string]string{
				"id": suggestedId,
			}
			return resp, nil
		}
		return nil, err
	}
	req.User.Id = userId

	now := time.Now()
	req.User.CreatedAt = tspb.New(now)
	req.User.UpdatedAt = tspb.New(now)

	if err := s.SaveUser(ctx, userId, req.User); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	resp.User = req.User
	return resp, nil
}

// UpdateUser updates an existing user profile
func (s *UsersService) UpdateUser(ctx context.Context, req *v1.UpdateUserRequest) (*v1.UpdateUserResponse, error) {
	if req.User == nil || req.User.Id == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	// Load existing user
	existingUser, err := s.LoadUser(ctx, req.User.Id)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	// Update fields if provided
	if req.User.Name != "" {
		existingUser.Name = req.User.Name
	}
	if req.User.Description != "" {
		existingUser.Description = req.User.Description
	}
	if req.User.Email != "" {
		existingUser.Email = req.User.Email
	}
	if req.User.ImageUrl != "" {
		existingUser.ImageUrl = req.User.ImageUrl
	}
	if req.User.Tags != nil {
		existingUser.Tags = req.User.Tags
	}
	if req.User.Extras != nil {
		existingUser.Extras = req.User.Extras
	}
	existingUser.UpdatedAt = tspb.New(time.Now())

	if err := s.SaveUser(ctx, req.User.Id, existingUser); err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	// Update cache
	if s.CacheEnabled {
		s.CacheMu.Lock()
		s.UserCache[req.User.Id] = existingUser
		s.CacheMu.Unlock()
	}

	return &v1.UpdateUserResponse{User: existingUser}, nil
}

// shortRandSuffix generates a 4-character random suffix for ID suggestions
func shortRandSuffix() string {
	maxId := int64(math.Pow(36, 4))
	randval := rand.Int63() % maxId
	return strconv.FormatInt(randval, 36)
}
