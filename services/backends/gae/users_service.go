//go:build !wasm
// +build !wasm

// Package gae provides a Google Cloud Datastore implementation of the UsersService.
// It is designed for deployment on Google Cloud Platform and supports multi-tenancy
// through Datastore namespaces.
//
// # Datastore Kind
//
// Users are stored with the kind "User" and the following properties:
//   - id: string (used as key name, not stored as property)
//   - created_at, updated_at: timestamps
//   - name, description, email, image_url: strings
//   - tags: []string (noindex)
//   - extras: Struct (noindex, for app-specific data)
//
// # Namespacing
//
// The service supports Datastore namespaces for multi-tenant applications.
// Pass a namespace to NewUsersService to isolate data between tenants:
//
//	userService := gae.NewUsersService(client, "tenant-123")
//
// # Generated Code
//
// This package uses protoc-gen-dal-datastore generated code:
//   - UserDatastore: Datastore entity model
//   - UserDatastoreToUser/UserToUserDatastore: conversion functions
//   - UserDatastoreDAL: data access layer with CRUD operations
//
// # Usage
//
//	client, _ := datastore.NewClient(ctx, projectID)
//	userService := gae.NewUsersService(client, "")  // default namespace
//	resp, err := userService.CreateUser(ctx, &v1.CreateUserRequest{
//	    User: &v1.User{Name: "John Doe", Email: "john@example.com"},
//	})
package gae

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"time"

	"cloud.google.com/go/datastore"
	v1 "github.com/panyam/goapplib/gen/go/goapplib/v1"
	v1ds "github.com/panyam/goapplib/gen/datastore"
	v1dal "github.com/panyam/goapplib/gen/datastore/dal"
	"github.com/panyam/goapplib/services"
	tspb "google.golang.org/protobuf/types/known/timestamppb"
)

// UsersService implements services.UsersService using Google Cloud Datastore.
// It embeds BaseUsersService to inherit caching and shared operations.
type UsersService struct {
	services.BaseUsersService
	client      *datastore.Client
	namespace   string
	MaxPageSize int
	UserDAL     v1dal.UserDatastoreDAL
}

// NewUsersService creates a new Datastore-backed UsersService.
// The namespace parameter supports multi-tenancy; pass empty string for default namespace.
func NewUsersService(client *datastore.Client, namespace string) *UsersService {
	service := &UsersService{
		client:      client,
		namespace:   namespace,
		MaxPageSize: 1000,
	}
	service.Self = service
	service.StorageProvider = service
	service.InitializeCache()

	return service
}

// NamespacedKey creates a Datastore key with the configured namespace
func (s *UsersService) NamespacedKey(kind, name string) *datastore.Key {
	key := datastore.NameKey(kind, name, nil)
	key.Namespace = s.namespace
	return key
}

// LoadUser implements UserStorageProvider.
func (s *UsersService) LoadUser(ctx context.Context, id string) (*v1.User, error) {
	key := s.NamespacedKey("User", id)
	userDs, err := s.UserDAL.Get(ctx, s.client, key)
	if err != nil {
		if err == datastore.ErrNoSuchEntity {
			return nil, services.ErrUserNotFound
		}
		return nil, err
	}
	return v1ds.UserDatastoreToUser(userDs, nil, nil)
}

// ListAllUsers implements UserStorageProvider.
func (s *UsersService) ListAllUsers(ctx context.Context) ([]*v1.User, error) {
	userDss, err := s.UserDAL.List(ctx, s.client, s.namespace, s.MaxPageSize, 0)
	if err != nil {
		return nil, err
	}

	users := make([]*v1.User, 0, len(userDss))
	for _, uds := range userDss {
		user, err := v1ds.UserDatastoreToUser(uds, nil, nil)
		if err != nil {
			continue
		}
		users = append(users, user)
	}
	return users, nil
}

// SaveUser implements UserStorageProvider.
func (s *UsersService) SaveUser(ctx context.Context, id string, user *v1.User) error {
	key := s.NamespacedKey("User", id)
	userDs, err := v1ds.UserToUserDatastore(user, nil, nil)
	if err != nil {
		return err
	}
	return s.UserDAL.Save(ctx, s.client, key, userDs)
}

// DeleteFromStorage implements UserStorageProvider.
func (s *UsersService) DeleteFromStorage(ctx context.Context, id string) error {
	key := s.NamespacedKey("User", id)
	return s.UserDAL.Delete(ctx, s.client, key)
}

// UserExists implements UserStorageProvider.
func (s *UsersService) UserExists(ctx context.Context, id string) bool {
	_, err := s.LoadUser(ctx, id)
	return err == nil
}

// CreateUser creates a new user profile.
func (s *UsersService) CreateUser(ctx context.Context, req *v1.CreateUserRequest) (*v1.CreateUserResponse, error) {
	resp := &v1.CreateUserResponse{}
	if req.User == nil {
		return nil, fmt.Errorf("user data is required")
	}

	// Generate ID if not provided
	if req.User.Id == "" {
		req.User.Id = shortRandId()
	}

	// Check if user already exists
	if s.UserExists(ctx, req.User.Id) {
		suggestedId := req.User.Id + "-" + shortRandSuffix()
		resp.FieldErrors = map[string]string{
			"id": suggestedId,
		}
		return resp, nil
	}

	now := time.Now()
	req.User.CreatedAt = tspb.New(now)
	req.User.UpdatedAt = tspb.New(now)

	if err := s.SaveUser(ctx, req.User.Id, req.User); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	resp.User = req.User
	return resp, nil
}

// UpdateUser updates an existing user profile.
func (s *UsersService) UpdateUser(ctx context.Context, req *v1.UpdateUserRequest) (*v1.UpdateUserResponse, error) {
	if req.User == nil || req.User.Id == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	// Load existing user
	existingUser, err := s.LoadUser(ctx, req.User.Id)
	if err != nil {
		return nil, err
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

// shortRandId generates a short random ID
func shortRandId() string {
	maxId := int64(math.Pow(36, 8))
	randval := rand.Int63() % maxId
	return strconv.FormatInt(randval, 36)
}

// shortRandSuffix generates a 4-character random suffix for ID suggestions
func shortRandSuffix() string {
	maxId := int64(math.Pow(36, 4))
	randval := rand.Int63() % maxId
	return strconv.FormatInt(randval, 36)
}
