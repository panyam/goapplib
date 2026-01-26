// Package likes provides a reusable reactions/likes service for any entity.
// The service supports multiple reaction types (like, love, celebrate, etc.)
// with denormalized counts for fast reads.
//
// # Architecture
//
// The package follows a layered architecture:
//
//   - LikesService interface: defines the contract for like/reaction operations
//   - BaseLikesService: provides shared logic (caching, common operations)
//   - LikesStorageProvider: abstracts raw storage operations
//   - Backend implementations (fs, gorm, gae): concrete storage providers
//
// # Usage
//
// Applications should use one of the concrete backend implementations:
//
//	// GORM backend (for PostgreSQL/MySQL/SQLite)
//	likesService := gorm.NewLikesService(db)
//
//	// GAE Datastore backend (for Google Cloud)
//	likesService := gae.NewLikesService(client, "namespace")
//
// # Key Features
//
//   - One reaction per user per entity (changing reaction replaces previous)
//   - Denormalized counts updated on every add/remove
//   - Configurable reaction types (apps define their own palette)
//   - Batch operations for efficient bulk queries
package likes

import (
	"context"
	"fmt"
	"sync"
	"time"

	v1 "github.com/panyam/goapplib/content/gen/go/likes/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// LikesService defines the interface for reaction/like operations.
type LikesService interface {
	// Core operations
	AddReaction(ctx context.Context, req *v1.AddReactionRequest) (*v1.AddReactionResponse, error)
	RemoveReaction(ctx context.Context, req *v1.RemoveReactionRequest) (*v1.RemoveReactionResponse, error)
	ToggleReaction(ctx context.Context, req *v1.ToggleReactionRequest) (*v1.ToggleReactionResponse, error)

	// Query operations
	GetUserReaction(ctx context.Context, req *v1.GetUserReactionRequest) (*v1.GetUserReactionResponse, error)
	GetLikeCounts(ctx context.Context, req *v1.GetLikeCountsRequest) (*v1.GetLikeCountsResponse, error)
	ListReactors(ctx context.Context, req *v1.ListReactorsRequest) (*v1.ListReactorsResponse, error)
	ListUserReactions(ctx context.Context, req *v1.ListUserReactionsRequest) (*v1.ListUserReactionsResponse, error)

	// Batch operations
	BatchGetUserReactions(ctx context.Context, req *v1.BatchGetUserReactionsRequest) (*v1.BatchGetUserReactionsResponse, error)
	BatchGetLikeCounts(ctx context.Context, req *v1.BatchGetLikeCountsRequest) (*v1.BatchGetLikeCountsResponse, error)

	// Reaction type management
	CreateReactionType(ctx context.Context, req *v1.CreateReactionTypeRequest) (*v1.CreateReactionTypeResponse, error)
	ListReactionTypes(ctx context.Context, req *v1.ListReactionTypesRequest) (*v1.ListReactionTypesResponse, error)
}

// LikesStorageProvider is implemented by concrete backends (gorm, gae)
// to provide raw storage operations for likes.
// Note: Entity type is determined by which service/table instance is used.
type LikesStorageProvider interface {
	// Like operations
	SaveLike(ctx context.Context, like *v1.Like) error
	DeleteLike(ctx context.Context, entityID, userID string) error
	GetLike(ctx context.Context, entityID, userID string) (*v1.Like, error)
	ListLikesByEntity(ctx context.Context, entityID string, reactionType string, limit, offset int) ([]*v1.Like, int, error)
	ListLikesByUser(ctx context.Context, userID string, limit, offset int) ([]*v1.Like, int, error)

	// Counts operations
	GetLikeCounts(ctx context.Context, entityID string) (*v1.LikeCounts, error)
	SaveLikeCounts(ctx context.Context, counts *v1.LikeCounts) error

	// Reaction type operations
	SaveReactionType(ctx context.Context, rt *v1.ReactionType) error
	GetReactionType(ctx context.Context, id string) (*v1.ReactionType, error)
	ListReactionTypes(ctx context.Context) ([]*v1.ReactionType, error)
}

// BaseLikesService provides shared logic for likes services.
type BaseLikesService struct {
	StorageProvider LikesStorageProvider

	// Optional in-memory cache for counts
	CacheEnabled bool
	CountsCache  map[string]*v1.LikeCounts // key: entityID
	CacheMu      sync.RWMutex

	// Default reaction type if not specified
	DefaultReactionType string

	// UserIDContextKey is the context key for reading user ID.
	// Defaults to DefaultUserIDContextKey if not set.
	UserIDContextKey any

	// Hooks for customization
	Hooks Hooks
}

// NewBaseLikesService creates a new BaseLikesService with the given storage provider.
func NewBaseLikesService(provider LikesStorageProvider, opts ...ServiceOption) *BaseLikesService {
	s := &BaseLikesService{
		StorageProvider:     provider,
		DefaultReactionType: "like",
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// InitializeCache sets up the in-memory cache for counts.
func (s *BaseLikesService) InitializeCache() {
	s.CacheEnabled = true
	s.CountsCache = make(map[string]*v1.LikeCounts)
}

// AddReaction adds or updates a user's reaction to an entity.
func (s *BaseLikesService) AddReaction(ctx context.Context, req *v1.AddReactionRequest) (*v1.AddReactionResponse, error) {
	// Resolve user ID from request or context (interceptor/middleware sets context)
	req.UserId = s.resolveUserID(ctx, req.UserId)

	// Authorization hook (can validate, deny, or further modify request)
	if s.Hooks.OnAuthorize != nil {
		hookCtx := &HookContext{
			Operation: "AddReaction",
			UserID:    req.UserId,
			EntityID:  req.EntityId,
			Request:   req,
		}
		if err := s.Hooks.OnAuthorize(ctx, hookCtx); err != nil {
			return nil, err
		}
	}

	// Validate required fields
	if req.EntityId == "" {
		return nil, ErrEntityRequired
	}
	if req.UserId == "" {
		return nil, ErrUserIDRequired
	}

	// Validate entity hook
	if s.Hooks.ValidateEntity != nil {
		if err := s.Hooks.ValidateEntity(ctx, req.EntityId); err != nil {
			return nil, err
		}
	}

	reactionType := req.ReactionType
	if reactionType == "" {
		reactionType = s.DefaultReactionType
	}

	now := time.Now()

	// Check if user already has a reaction
	existing, _ := s.StorageProvider.GetLike(ctx, req.EntityId, req.UserId)

	// Get current counts
	counts, err := s.getOrCreateCounts(ctx, req.EntityId)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		// User already has a reaction - update it
		oldType := existing.ReactionType

		// Update reaction type
		existing.ReactionType = reactionType
		existing.UpdatedAt = timestamppb.New(now)

		// BeforeSave hook
		if s.Hooks.BeforeSave != nil {
			if err := s.Hooks.BeforeSave(ctx, existing); err != nil {
				return nil, err
			}
		}

		if err := s.StorageProvider.SaveLike(ctx, existing); err != nil {
			return nil, err
		}

		// AfterSave hook (errors logged, don't fail operation)
		if s.Hooks.AfterSave != nil {
			_ = s.Hooks.AfterSave(ctx, existing)
		}

		// Update counts if reaction type changed
		if oldType != reactionType {
			counts.ByReactionType[oldType]--
			if counts.ByReactionType[oldType] <= 0 {
				delete(counts.ByReactionType, oldType)
			}
			counts.ByReactionType[reactionType]++
			counts.UpdatedAt = timestamppb.New(now)

			if err := s.StorageProvider.SaveLikeCounts(ctx, counts); err != nil {
				return nil, err
			}
			s.invalidateCountsCache(req.EntityId)

			// OnEvent hook for reaction change
			if s.Hooks.OnEvent != nil {
				_ = s.Hooks.OnEvent(ctx, &Event{
					Type:     EventReactionChanged,
					EntityID: req.EntityId,
					UserID:   req.UserId,
					Like:     existing,
					OldType:  oldType,
					NewType:  reactionType,
				})
			}
		}

		return &v1.AddReactionResponse{
			Like:   existing,
			Counts: counts,
		}, nil
	}

	// Create new reaction
	like := &v1.Like{
		Id:           generateID(),
		EntityId:     req.EntityId,
		UserId:       req.UserId,
		ReactionType: reactionType,
		CreatedAt:    timestamppb.New(now),
		UpdatedAt:    timestamppb.New(now),
		CreatorId:    req.UserId,
	}

	// BeforeSave hook
	if s.Hooks.BeforeSave != nil {
		if err := s.Hooks.BeforeSave(ctx, like); err != nil {
			return nil, err
		}
	}

	if err := s.StorageProvider.SaveLike(ctx, like); err != nil {
		return nil, err
	}

	// AfterSave hook (errors logged, don't fail operation)
	if s.Hooks.AfterSave != nil {
		_ = s.Hooks.AfterSave(ctx, like)
	}

	// Update counts
	counts.TotalCount++
	counts.ByReactionType[reactionType]++
	counts.UpdatedAt = timestamppb.New(now)

	if err := s.StorageProvider.SaveLikeCounts(ctx, counts); err != nil {
		return nil, err
	}
	s.invalidateCountsCache(req.EntityId)

	// OnEvent hook for new reaction
	if s.Hooks.OnEvent != nil {
		_ = s.Hooks.OnEvent(ctx, &Event{
			Type:     EventReactionAdded,
			EntityID: req.EntityId,
			UserID:   req.UserId,
			Like:     like,
		})
	}

	return &v1.AddReactionResponse{
		Like:   like,
		Counts: counts,
	}, nil
}

// RemoveReaction removes a user's reaction from an entity.
func (s *BaseLikesService) RemoveReaction(ctx context.Context, req *v1.RemoveReactionRequest) (*v1.RemoveReactionResponse, error) {
	// Resolve user ID from request or context
	req.UserId = s.resolveUserID(ctx, req.UserId)

	// Authorization hook
	if s.Hooks.OnAuthorize != nil {
		hookCtx := &HookContext{
			Operation: "RemoveReaction",
			UserID:    req.UserId,
			EntityID:  req.EntityId,
			Request:   req,
		}
		if err := s.Hooks.OnAuthorize(ctx, hookCtx); err != nil {
			return nil, err
		}
	}

	// Validate required fields
	if req.EntityId == "" {
		return nil, ErrEntityRequired
	}
	if req.UserId == "" {
		return nil, ErrUserIDRequired
	}

	// Get existing reaction
	existing, _ := s.StorageProvider.GetLike(ctx, req.EntityId, req.UserId)
	if existing == nil {
		// No reaction to remove
		counts, _ := s.GetLikeCounts(ctx, &v1.GetLikeCountsRequest{
			EntityId: req.EntityId,
		})
		return &v1.RemoveReactionResponse{
			Removed: false,
			Counts:  counts.Counts,
		}, nil
	}

	// BeforeDelete hook
	if s.Hooks.BeforeDelete != nil {
		if err := s.Hooks.BeforeDelete(ctx, existing); err != nil {
			return nil, err
		}
	}

	// Delete the reaction
	if err := s.StorageProvider.DeleteLike(ctx, req.EntityId, req.UserId); err != nil {
		return nil, err
	}

	// AfterDelete hook (errors logged, don't fail operation)
	if s.Hooks.AfterDelete != nil {
		_ = s.Hooks.AfterDelete(ctx, existing)
	}

	// Update counts
	counts, err := s.getOrCreateCounts(ctx, req.EntityId)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	counts.TotalCount--
	if counts.TotalCount < 0 {
		counts.TotalCount = 0
	}
	counts.ByReactionType[existing.ReactionType]--
	if counts.ByReactionType[existing.ReactionType] <= 0 {
		delete(counts.ByReactionType, existing.ReactionType)
	}
	counts.UpdatedAt = timestamppb.New(now)

	if err := s.StorageProvider.SaveLikeCounts(ctx, counts); err != nil {
		return nil, err
	}
	s.invalidateCountsCache(req.EntityId)

	// OnEvent hook for reaction removed
	if s.Hooks.OnEvent != nil {
		_ = s.Hooks.OnEvent(ctx, &Event{
			Type:     EventReactionRemoved,
			EntityID: req.EntityId,
			UserID:   req.UserId,
			Like:     existing,
		})
	}

	return &v1.RemoveReactionResponse{
		Removed: true,
		Counts:  counts,
	}, nil
}

// ToggleReaction toggles a reaction on/off.
func (s *BaseLikesService) ToggleReaction(ctx context.Context, req *v1.ToggleReactionRequest) (*v1.ToggleReactionResponse, error) {
	if req.EntityId == "" {
		return nil, ErrEntityRequired
	}
	if req.UserId == "" {
		return nil, ErrUserIDRequired
	}

	reactionType := req.ReactionType
	if reactionType == "" {
		reactionType = s.DefaultReactionType
	}

	// Check if user already has this exact reaction
	existing, _ := s.StorageProvider.GetLike(ctx, req.EntityId, req.UserId)

	if existing != nil && existing.ReactionType == reactionType {
		// Same reaction - remove it
		resp, err := s.RemoveReaction(ctx, &v1.RemoveReactionRequest{
			EntityId: req.EntityId,
			UserId:   req.UserId,
		})
		if err != nil {
			return nil, err
		}
		return &v1.ToggleReactionResponse{
			Like:   nil,
			Added:  false,
			Counts: resp.Counts,
		}, nil
	}

	// Different or no reaction - add/update it
	resp, err := s.AddReaction(ctx, &v1.AddReactionRequest{
		EntityId:     req.EntityId,
		UserId:       req.UserId,
		ReactionType: reactionType,
	})
	if err != nil {
		return nil, err
	}

	return &v1.ToggleReactionResponse{
		Like:   resp.Like,
		Added:  true,
		Counts: resp.Counts,
	}, nil
}

// GetUserReaction returns a user's current reaction on an entity.
func (s *BaseLikesService) GetUserReaction(ctx context.Context, req *v1.GetUserReactionRequest) (*v1.GetUserReactionResponse, error) {
	// Resolve user ID from request or context
	req.UserId = s.resolveUserID(ctx, req.UserId)

	// Authorization hook
	if s.Hooks.OnAuthorize != nil {
		hookCtx := &HookContext{
			Operation: "GetUserReaction",
			UserID:    req.UserId,
			EntityID:  req.EntityId,
			Request:   req,
		}
		if err := s.Hooks.OnAuthorize(ctx, hookCtx); err != nil {
			return nil, err
		}
	}

	// Validate required fields
	if req.EntityId == "" {
		return nil, ErrEntityRequired
	}
	if req.UserId == "" {
		return nil, ErrUserIDRequired
	}

	like, err := s.StorageProvider.GetLike(ctx, req.EntityId, req.UserId)
	if err != nil {
		return nil, err
	}

	// AfterRead hook
	if like != nil && s.Hooks.AfterRead != nil {
		_ = s.Hooks.AfterRead(ctx, []*v1.Like{like})
	}

	return &v1.GetUserReactionResponse{
		Like: like,
	}, nil
}

// GetLikeCounts returns aggregated reaction counts for an entity.
func (s *BaseLikesService) GetLikeCounts(ctx context.Context, req *v1.GetLikeCountsRequest) (*v1.GetLikeCountsResponse, error) {
	if req.EntityId == "" {
		return nil, ErrEntityRequired
	}

	// Check cache first
	if s.CacheEnabled {
		s.CacheMu.RLock()
		if counts, ok := s.CountsCache[req.EntityId]; ok {
			s.CacheMu.RUnlock()
			return &v1.GetLikeCountsResponse{Counts: counts}, nil
		}
		s.CacheMu.RUnlock()
	}

	counts, err := s.StorageProvider.GetLikeCounts(ctx, req.EntityId)
	if err != nil {
		return nil, err
	}

	if counts == nil {
		counts = &v1.LikeCounts{
			EntityId:       req.EntityId,
			TotalCount:     0,
			ByReactionType: make(map[string]int64),
		}
	}

	// Update cache
	if s.CacheEnabled {
		s.CacheMu.Lock()
		s.CountsCache[req.EntityId] = counts
		s.CacheMu.Unlock()
	}

	return &v1.GetLikeCountsResponse{Counts: counts}, nil
}

// ListReactors returns users who reacted to an entity.
func (s *BaseLikesService) ListReactors(ctx context.Context, req *v1.ListReactorsRequest) (*v1.ListReactorsResponse, error) {
	// Authorization hook
	if s.Hooks.OnAuthorize != nil {
		hookCtx := &HookContext{
			Operation: "ListReactors",
			UserID:    GetUserIDFromContext(ctx, s.UserIDContextKey), // For authorization context
			EntityID:  req.EntityId,
			Request:   req,
		}
		if err := s.Hooks.OnAuthorize(ctx, hookCtx); err != nil {
			return nil, err
		}
	}

	// Validate required fields
	if req.EntityId == "" {
		return nil, ErrEntityRequired
	}

	pageSize := int(req.Pagination.GetPageSize())
	if pageSize <= 0 {
		pageSize = 50
	}

	offset := 0
	if req.Pagination.GetPageToken() != "" {
		fmt.Sscanf(req.Pagination.GetPageToken(), "%d", &offset)
	}

	likes, total, err := s.StorageProvider.ListLikesByEntity(ctx, req.EntityId, req.ReactionType, pageSize, offset)
	if err != nil {
		return nil, err
	}

	// AfterRead hook
	if len(likes) > 0 && s.Hooks.AfterRead != nil {
		_ = s.Hooks.AfterRead(ctx, likes)
	}

	var nextToken string
	if offset+len(likes) < total {
		nextToken = fmt.Sprintf("%d", offset+len(likes))
	}

	return &v1.ListReactorsResponse{
		Likes: likes,
		Pagination: &v1.PaginationResponse{
			NextPageToken: nextToken,
			TotalCount:    int32(total),
		},
	}, nil
}

// ListUserReactions returns all reactions by a specific user.
func (s *BaseLikesService) ListUserReactions(ctx context.Context, req *v1.ListUserReactionsRequest) (*v1.ListUserReactionsResponse, error) {
	// Resolve user ID from request or context
	req.UserId = s.resolveUserID(ctx, req.UserId)

	// Authorization hook
	if s.Hooks.OnAuthorize != nil {
		hookCtx := &HookContext{
			Operation: "ListUserReactions",
			UserID:    req.UserId,
			Request:   req,
		}
		if err := s.Hooks.OnAuthorize(ctx, hookCtx); err != nil {
			return nil, err
		}
	}

	// Validate required fields
	if req.UserId == "" {
		return nil, ErrUserIDRequired
	}

	pageSize := int(req.Pagination.GetPageSize())
	if pageSize <= 0 {
		pageSize = 50
	}

	offset := 0
	if req.Pagination.GetPageToken() != "" {
		fmt.Sscanf(req.Pagination.GetPageToken(), "%d", &offset)
	}

	likes, total, err := s.StorageProvider.ListLikesByUser(ctx, req.UserId, pageSize, offset)
	if err != nil {
		return nil, err
	}

	// AfterRead hook
	if len(likes) > 0 && s.Hooks.AfterRead != nil {
		_ = s.Hooks.AfterRead(ctx, likes)
	}

	var nextToken string
	if offset+len(likes) < total {
		nextToken = fmt.Sprintf("%d", offset+len(likes))
	}

	return &v1.ListUserReactionsResponse{
		Likes: likes,
		Pagination: &v1.PaginationResponse{
			NextPageToken: nextToken,
			TotalCount:    int32(total),
		},
	}, nil
}

// BatchGetUserReactions returns a user's reactions for multiple entities.
func (s *BaseLikesService) BatchGetUserReactions(ctx context.Context, req *v1.BatchGetUserReactionsRequest) (*v1.BatchGetUserReactionsResponse, error) {
	if req.UserId == "" {
		return nil, ErrUserIDRequired
	}

	reactions := make(map[string]*v1.Like)
	for _, entityID := range req.EntityIds {
		like, _ := s.StorageProvider.GetLike(ctx, entityID, req.UserId)
		if like != nil {
			reactions[entityID] = like
		}
	}

	return &v1.BatchGetUserReactionsResponse{
		Reactions: reactions,
	}, nil
}

// BatchGetLikeCounts returns counts for multiple entities.
func (s *BaseLikesService) BatchGetLikeCounts(ctx context.Context, req *v1.BatchGetLikeCountsRequest) (*v1.BatchGetLikeCountsResponse, error) {
	counts := make(map[string]*v1.LikeCounts)

	for _, entityID := range req.EntityIds {
		resp, err := s.GetLikeCounts(ctx, &v1.GetLikeCountsRequest{
			EntityId: entityID,
		})
		if err == nil && resp.Counts != nil {
			counts[entityID] = resp.Counts
		}
	}

	return &v1.BatchGetLikeCountsResponse{
		Counts: counts,
	}, nil
}

// CreateReactionType creates a new reaction type.
func (s *BaseLikesService) CreateReactionType(ctx context.Context, req *v1.CreateReactionTypeRequest) (*v1.CreateReactionTypeResponse, error) {
	if req.ReactionType == nil || req.ReactionType.Id == "" {
		return nil, ErrReactionTypeRequired
	}

	now := time.Now()
	rt := req.ReactionType
	rt.CreatedAt = timestamppb.New(now)
	rt.UpdatedAt = timestamppb.New(now)

	if err := s.StorageProvider.SaveReactionType(ctx, rt); err != nil {
		return nil, err
	}

	return &v1.CreateReactionTypeResponse{
		ReactionType: rt,
	}, nil
}

// ListReactionTypes returns all available reaction types.
func (s *BaseLikesService) ListReactionTypes(ctx context.Context, req *v1.ListReactionTypesRequest) (*v1.ListReactionTypesResponse, error) {
	types, err := s.StorageProvider.ListReactionTypes(ctx)
	if err != nil {
		return nil, err
	}

	return &v1.ListReactionTypesResponse{
		ReactionTypes: types,
	}, nil
}

// Helper methods

// resolveUserID returns the user ID from the request, falling back to context if empty.
// This allows interceptors/middleware to set user ID in context.
func (s *BaseLikesService) resolveUserID(ctx context.Context, requestUserID string) string {
	if requestUserID != "" {
		return requestUserID
	}
	return GetUserIDFromContext(ctx, s.UserIDContextKey)
}

func (s *BaseLikesService) getOrCreateCounts(ctx context.Context, entityID string) (*v1.LikeCounts, error) {
	counts, err := s.StorageProvider.GetLikeCounts(ctx, entityID)
	if err != nil || counts == nil {
		counts = &v1.LikeCounts{
			EntityId:       entityID,
			TotalCount:     0,
			ByReactionType: make(map[string]int64),
		}
	}
	if counts.ByReactionType == nil {
		counts.ByReactionType = make(map[string]int64)
	}
	return counts, nil
}

func (s *BaseLikesService) invalidateCountsCache(entityID string) {
	if s.CacheEnabled {
		s.CacheMu.Lock()
		delete(s.CountsCache, entityID)
		s.CacheMu.Unlock()
	}
}

// generateID generates a unique ID for a like.
func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// Error types
var (
	ErrEntityRequired       = &LikesError{Message: "entity_id is required"}
	ErrUserIDRequired       = &LikesError{Message: "user_id is required"}
	ErrReactionTypeRequired = &LikesError{Message: "reaction_type with id is required"}
	ErrLikeNotFound         = &LikesError{Message: "like not found"}
)

// LikesError represents an error in the likes service.
type LikesError struct {
	Message string
}

func (e *LikesError) Error() string {
	return e.Message
}
