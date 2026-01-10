//go:build !wasm
// +build !wasm

// Package gorm provides a GORM-based implementation of the UsersService.
// It supports any database that GORM supports (PostgreSQL, MySQL, SQLite, etc.)
// and is suitable for production deployments requiring relational database storage.
//
// # Database Schema
//
// The service uses auto-migration to create/update the users table:
//
//	CREATE TABLE users (
//	    id VARCHAR PRIMARY KEY,
//	    created_at TIMESTAMP,
//	    updated_at TIMESTAMP,
//	    name VARCHAR,
//	    description TEXT,
//	    tags JSON,           -- Stored as JSON array
//	    image_url VARCHAR,
//	    email VARCHAR,
//	    extras JSON          -- App-specific data as JSON
//	);
//
// # Generated Code
//
// This package uses protoc-gen-dal-gorm generated code:
//   - UserGORM: GORM model with proper tags
//   - UserGORMToUser/UserToUserGORM: conversion functions
//   - UserGORMDAL: data access layer with CRUD operations
//
// # Usage
//
//	db, _ := gorm.Open(postgres.Open(dsn), &gorm.Config{})
//	userService := gormbe.NewUsersService(db)
//	resp, err := userService.CreateUser(ctx, &v1.CreateUserRequest{
//	    User: &v1.User{Name: "John Doe", Email: "john@example.com"},
//	})
package gorm

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"time"

	v1 "github.com/panyam/goapplib/gen/go/goapplib/v1"
	v1gorm "github.com/panyam/goapplib/gen/gorm"
	v1dal "github.com/panyam/goapplib/gen/gorm/dal"
	"github.com/panyam/goapplib/services"
	"gorm.io/gorm"
)

// UsersService implements services.UsersService using GORM.
// It embeds BaseUsersService to inherit caching and shared operations.
type UsersService struct {
	services.BaseUsersService
	storage     *gorm.DB
	MaxPageSize int
	UserDAL     v1dal.UserGORMDAL
}

// NewUsersService creates a new GORM-backed UsersService.
// It automatically runs database migrations to ensure the schema is up to date.
func NewUsersService(db *gorm.DB) *UsersService {
	db.AutoMigrate(&v1gorm.UserGORM{})

	service := &UsersService{
		storage:     db,
		MaxPageSize: 1000,
	}
	service.UserDAL.WillCreate = func(ctx context.Context, user *v1gorm.UserGORM) error {
		user.UpdatedAt = time.Now()
		user.CreatedAt = time.Now()
		return nil
	}
	service.Self = service
	service.StorageProvider = service
	service.InitializeCache()

	return service
}

// LoadUser implements UserStorageProvider.
func (s *UsersService) LoadUser(ctx context.Context, id string) (*v1.User, error) {
	userGorm, err := s.UserDAL.Get(ctx, s.storage, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, services.ErrUserNotFound
		}
		return nil, err
	}
	return v1gorm.UserGORMToUser(userGorm, nil, nil)
}

// ListAllUsers implements UserStorageProvider.
func (s *UsersService) ListAllUsers(ctx context.Context) ([]*v1.User, error) {
	userGorms, err := s.UserDAL.List(ctx, s.storage, s.MaxPageSize, 0)
	if err != nil {
		return nil, err
	}

	users := make([]*v1.User, 0, len(userGorms))
	for _, ug := range userGorms {
		user, err := v1gorm.UserGORMToUser(ug, nil, nil)
		if err != nil {
			continue
		}
		users = append(users, user)
	}
	return users, nil
}

// SaveUser implements UserStorageProvider.
func (s *UsersService) SaveUser(ctx context.Context, id string, user *v1.User) error {
	userGorm, err := v1gorm.UserToUserGORM(user, nil, nil)
	if err != nil {
		return err
	}
	return s.UserDAL.Save(ctx, s.storage, userGorm)
}

// DeleteFromStorage implements UserStorageProvider.
func (s *UsersService) DeleteFromStorage(ctx context.Context, id string) error {
	return s.UserDAL.Delete(ctx, s.storage, id)
}

// UserExists implements UserStorageProvider.
func (s *UsersService) UserExists(ctx context.Context, id string) bool {
	_, err := s.UserDAL.Get(ctx, s.storage, id)
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

	userGorm, err := v1gorm.UserToUserGORM(req.User, nil, nil)
	if err != nil {
		return nil, err
	}

	if err := s.UserDAL.Create(ctx, s.storage, userGorm); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Convert back to get timestamps
	user, err := v1gorm.UserGORMToUser(userGorm, nil, nil)
	if err != nil {
		return nil, err
	}

	resp.User = user
	return resp, nil
}

// UpdateUser updates an existing user profile.
func (s *UsersService) UpdateUser(ctx context.Context, req *v1.UpdateUserRequest) (*v1.UpdateUserResponse, error) {
	if req.User == nil || req.User.Id == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	// Load existing user
	existingGorm, err := s.UserDAL.Get(ctx, s.storage, req.User.Id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, services.ErrUserNotFound
		}
		return nil, err
	}

	existingUser, err := v1gorm.UserGORMToUser(existingGorm, nil, nil)
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

	// Convert back and save
	updatedGorm, err := v1gorm.UserToUserGORM(existingUser, nil, nil)
	if err != nil {
		return nil, err
	}
	updatedGorm.UpdatedAt = time.Now()

	if err := s.UserDAL.Save(ctx, s.storage, updatedGorm); err != nil {
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
