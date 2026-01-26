// Package tags provides a reusable tagging and metadata service for any entity.
// The service supports both pure tags (labels) and metadata (name-value pairs)
// with denormalized counts for fast reads.
//
// # Architecture
//
// The package follows a layered architecture:
//
//   - TagsService interface: defines the contract for tag operations
//   - BaseTagsService: provides shared logic (normalization, caching, common operations)
//   - TagsStorageProvider: abstracts raw storage operations
//   - Backend implementations (gorm, gae): concrete storage providers
//
// # Usage
//
// Applications should use one of the concrete backend implementations:
//
//	// GORM backend (for PostgreSQL/MySQL/SQLite)
//	tagsService := gorm.NewTagsService(db)
//
//	// GAE Datastore backend (for Google Cloud)
//	tagsService := gae.NewTagsService(client, "namespace")
//
// # Key Features
//
//   - Pure tags: Simple labels like "favorites", "rock"
//   - Metadata: Name-value pairs like venue="Wembley", rating="5"
//   - Multi-user tagging: Same tag can be applied by multiple users
//   - Visibility control: Private, shared, and public tag applications
//   - Tag deduplication via normalization
//   - Denormalized usage counts
package tags

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	commonv1 "github.com/panyam/goapplib/content/gen/go/common/v1"
	v1 "github.com/panyam/goapplib/content/gen/go/tags/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TagsService defines the interface for tag operations.
type TagsService interface {
	// Tag CRUD
	CreateTag(ctx context.Context, req *v1.CreateTagRequest) (*v1.CreateTagResponse, error)
	GetTag(ctx context.Context, req *v1.GetTagRequest) (*v1.GetTagResponse, error)
	UpdateTag(ctx context.Context, req *v1.UpdateTagRequest) (*v1.UpdateTagResponse, error)
	DeleteTag(ctx context.Context, req *v1.DeleteTagRequest) (*v1.DeleteTagResponse, error)
	ListTags(ctx context.Context, req *v1.ListTagsRequest) (*v1.ListTagsResponse, error)

	// Entity tagging
	TagEntity(ctx context.Context, req *v1.TagEntityRequest) (*v1.TagEntityResponse, error)
	UntagEntity(ctx context.Context, req *v1.UntagEntityRequest) (*v1.UntagEntityResponse, error)
	GetEntityTags(ctx context.Context, req *v1.GetEntityTagsRequest) (*v1.GetEntityTagsResponse, error)
	GetEntitiesWithTag(ctx context.Context, req *v1.GetEntitiesWithTagRequest) (*v1.GetEntitiesWithTagResponse, error)

	// Batch operations
	BatchTagEntities(ctx context.Context, req *v1.BatchTagEntitiesRequest) (*v1.BatchTagEntitiesResponse, error)
	BatchGetEntityTags(ctx context.Context, req *v1.BatchGetEntityTagsRequest) (*v1.BatchGetEntityTagsResponse, error)

	// Discovery
	SearchTags(ctx context.Context, req *v1.SearchTagsRequest) (*v1.SearchTagsResponse, error)
	GetPopularTags(ctx context.Context, req *v1.GetPopularTagsRequest) (*v1.GetPopularTagsResponse, error)

	// Admin operations
	MergeTags(ctx context.Context, req *v1.MergeTagsRequest) (*v1.MergeTagsResponse, error)
	PromoteTag(ctx context.Context, req *v1.PromoteTagRequest) (*v1.PromoteTagResponse, error)
}

// TagsStorageProvider is implemented by concrete backends (gorm, gae)
// to provide raw storage operations for tags.
type TagsStorageProvider interface {
	// Tag operations
	SaveTag(ctx context.Context, tag *v1.Tag) error
	GetTag(ctx context.Context, id string) (*v1.Tag, error)
	DeleteTag(ctx context.Context, id string) error
	ListTags(ctx context.Context, opts ListTagsOptions) ([]*v1.Tag, int, error)
	FindTagByNormalizedValues(ctx context.Context, ownerID, normalizedName, normalizedValue string) (*v1.Tag, error)
	SearchTags(ctx context.Context, opts SearchTagsOptions) ([]*v1.Tag, error)
	GetPopularTags(ctx context.Context, opts PopularTagsOptions) ([]*v1.Tag, error)

	// EntityTag operations
	SaveEntityTag(ctx context.Context, entityTag *v1.EntityTag) error
	DeleteEntityTag(ctx context.Context, tagID, entityID, taggedBy string) error
	GetEntityTag(ctx context.Context, tagID, entityID, taggedBy string) (*v1.EntityTag, error)
	ListEntityTagsByEntity(ctx context.Context, entityID string, opts EntityTagsOptions) ([]*v1.EntityTag, error)
	ListEntityTagsByTag(ctx context.Context, tagID string, limit, offset int) ([]*v1.EntityTag, int, error)
	DeleteEntityTagsByTag(ctx context.Context, tagID string) (int64, error)

	// TagUsageCounts operations (optional - for denormalized counts)
	GetTagUsageCounts(ctx context.Context, entityID string) (*v1.TagUsageCounts, error)
	SaveTagUsageCounts(ctx context.Context, counts *v1.TagUsageCounts) error
}

// ListTagsOptions contains options for listing tags.
type ListTagsOptions struct {
	OwnerID    string
	Scope      v1.TagScope
	NameFilter string // "" = all, empty string = pure tags, "venue" = specific name
	OrderBy    string // "name", "usage_count", "created_at", "display_order"
	Limit      int
	Offset     int
}

// SearchTagsOptions contains options for searching tags.
type SearchTagsOptions struct {
	Query         string
	NameFilter    string
	OwnerID       string
	IncludeShared bool
	Limit         int
}

// PopularTagsOptions contains options for getting popular tags.
type PopularTagsOptions struct {
	NameFilter    string
	OwnerID       string
	IncludeShared bool
	Limit         int
}

// EntityTagsOptions contains options for listing entity tags.
type EntityTagsOptions struct {
	NameFilter string
	OwnerID    string
}

// BaseTagsService provides shared logic for tags services.
// Embeds UnimplementedTagsServiceServer for gRPC forward compatibility.
type BaseTagsService struct {
	v1.UnimplementedTagsServiceServer
	StorageProvider TagsStorageProvider

	// Optional in-memory cache for tag lookups
	CacheEnabled bool
	TagCache     map[string]*v1.Tag // key: tag ID
	CacheMu      sync.RWMutex

	// Normalizer function (defaults to lowercase + trim)
	Normalizer func(string) string

	// UserIDContextKey is the context key for reading user ID.
	// Defaults to DefaultUserIDContextKey if not set.
	UserIDContextKey any

	// Hooks for customization
	Hooks Hooks
}

// Compile-time check that BaseTagsService implements TagsServiceServer.
var _ v1.TagsServiceServer = (*BaseTagsService)(nil)

// NewBaseTagsService creates a new BaseTagsService with the given storage provider.
func NewBaseTagsService(provider TagsStorageProvider, opts ...ServiceOption) *BaseTagsService {
	s := &BaseTagsService{
		StorageProvider: provider,
		Normalizer:      DefaultNormalizer,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// resolveUserID returns the user ID from the request, falling back to context if empty.
// This allows interceptors/middleware to set user ID in context.
func (s *BaseTagsService) resolveUserID(ctx context.Context, requestUserID string) string {
	if requestUserID != "" {
		return requestUserID
	}
	return GetUserIDFromContext(ctx, s.UserIDContextKey)
}

// resolveEntityID returns the entity ID from the request, falling back to mounted context if empty.
// This allows the service to be mounted at arbitrary paths with the entity ID extracted from URL params.
func (s *BaseTagsService) resolveEntityID(ctx context.Context, requestEntityID string) string {
	if requestEntityID != "" {
		return requestEntityID
	}
	return GetMountedEntityID(ctx)
}

// DefaultNormalizer is the default normalization function.
// It lowercases and trims whitespace.
func DefaultNormalizer(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// InitializeCache sets up the in-memory cache.
func (s *BaseTagsService) InitializeCache() {
	s.CacheEnabled = true
	s.TagCache = make(map[string]*v1.Tag)
}

// CreateTag creates a new tag or returns existing if duplicate.
func (s *BaseTagsService) CreateTag(ctx context.Context, req *v1.CreateTagRequest) (*v1.CreateTagResponse, error) {
	// Resolve owner ID from request or context
	req.OwnerId = s.resolveUserID(ctx, req.OwnerId)

	// Authorization hook
	if s.Hooks.OnAuthorize != nil {
		hookCtx := &HookContext{
			Operation: "CreateTag",
			UserID:    req.OwnerId,
			Request:   req,
		}
		if err := s.Hooks.OnAuthorize(ctx, hookCtx); err != nil {
			return nil, err
		}
	}

	// Validate required fields
	if req.Value == "" {
		return nil, ErrValueRequired
	}

	// Normalize name and value
	normalizedName := s.Normalizer(req.Name)
	normalizedValue := s.Normalizer(req.Value)

	// Check for existing tag with same normalized values for this owner
	existing, _ := s.StorageProvider.FindTagByNormalizedValues(
		ctx, req.OwnerId, normalizedName, normalizedValue)
	if existing != nil {
		return &v1.CreateTagResponse{
			Tag:            existing,
			AlreadyExisted: true,
		}, nil
	}

	// Create new tag
	now := time.Now()
	tag := &v1.Tag{
		Id:              generateID(),
		Name:            req.Name,
		NormalizedName:  normalizedName,
		Value:           req.Value,
		NormalizedValue: normalizedValue,
		Type:            req.Type,
		Color:           req.Color,
		Description:     req.Description,
		DisplayOrder:    req.DisplayOrder,
		OwnerId:         req.OwnerId,
		Scope:           req.Scope,
		Status:          v1.TagStatus_TAG_STATUS_ACTIVE,
		UsageCount:      0,
		CreatedAt:       timestamppb.New(now),
		UpdatedAt:       timestamppb.New(now),
		CreatorId:       req.OwnerId, // Default creator to owner
	}

	// Default scope to private if not specified
	if tag.Scope == v1.TagScope_TAG_SCOPE_UNSPECIFIED {
		tag.Scope = v1.TagScope_TAG_SCOPE_PRIVATE
	}

	// BeforeTagSave hook
	if s.Hooks.BeforeTagSave != nil {
		if err := s.Hooks.BeforeTagSave(ctx, tag); err != nil {
			return nil, err
		}
	}

	if err := s.StorageProvider.SaveTag(ctx, tag); err != nil {
		return nil, err
	}

	// AfterTagSave hook (errors logged, don't fail operation)
	if s.Hooks.AfterTagSave != nil {
		_ = s.Hooks.AfterTagSave(ctx, tag)
	}

	// OnEvent hook
	if s.Hooks.OnEvent != nil {
		_ = s.Hooks.OnEvent(ctx, &Event{
			Type:   EventTagCreated,
			TagID:  tag.Id,
			UserID: req.OwnerId,
			Tag:    tag,
		})
	}

	s.cacheTag(tag)

	return &v1.CreateTagResponse{
		Tag:            tag,
		AlreadyExisted: false,
	}, nil
}

// GetTag retrieves a tag by ID.
func (s *BaseTagsService) GetTag(ctx context.Context, req *v1.GetTagRequest) (*v1.GetTagResponse, error) {
	if req.Id == "" {
		return nil, ErrTagIDRequired
	}

	// Check cache first
	if tag := s.getCachedTag(req.Id); tag != nil {
		return &v1.GetTagResponse{Tag: tag}, nil
	}

	tag, err := s.StorageProvider.GetTag(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if tag == nil {
		return nil, ErrTagNotFound
	}

	// Follow redirect if needed
	if tag.Status == v1.TagStatus_TAG_STATUS_REDIRECT && tag.RedirectToTagId != "" {
		tag, err = s.StorageProvider.GetTag(ctx, tag.RedirectToTagId)
		if err != nil {
			return nil, err
		}
	}

	s.cacheTag(tag)

	return &v1.GetTagResponse{Tag: tag}, nil
}

// UpdateTag updates a tag's properties.
func (s *BaseTagsService) UpdateTag(ctx context.Context, req *v1.UpdateTagRequest) (*v1.UpdateTagResponse, error) {
	if req.Tag == nil || req.Tag.Id == "" {
		return nil, ErrTagIDRequired
	}

	existing, err := s.StorageProvider.GetTag(ctx, req.Tag.Id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrTagNotFound
	}

	// Update fields based on update_mask or all non-empty fields
	updateMask := make(map[string]bool)
	for _, field := range req.UpdateMask {
		updateMask[field] = true
	}
	useAllFields := len(updateMask) == 0

	if useAllFields || updateMask["color"] {
		if req.Tag.Color != "" || updateMask["color"] {
			existing.Color = req.Tag.Color
		}
	}
	if useAllFields || updateMask["description"] {
		if req.Tag.Description != "" || updateMask["description"] {
			existing.Description = req.Tag.Description
		}
	}
	if useAllFields || updateMask["display_order"] {
		existing.DisplayOrder = req.Tag.DisplayOrder
	}
	if useAllFields || updateMask["scope"] {
		if req.Tag.Scope != v1.TagScope_TAG_SCOPE_UNSPECIFIED {
			existing.Scope = req.Tag.Scope
		}
	}

	existing.UpdatedAt = timestamppb.New(time.Now())

	if err := s.StorageProvider.SaveTag(ctx, existing); err != nil {
		return nil, err
	}

	s.invalidateTagCache(existing.Id)

	return &v1.UpdateTagResponse{Tag: existing}, nil
}

// DeleteTag removes a tag and optionally untags all entities.
func (s *BaseTagsService) DeleteTag(ctx context.Context, req *v1.DeleteTagRequest) (*v1.DeleteTagResponse, error) {
	// Authorization hook
	if s.Hooks.OnAuthorize != nil {
		hookCtx := &HookContext{
			Operation: "DeleteTag",
			UserID:    GetUserIDFromContext(ctx, s.UserIDContextKey),
			TagID:     req.Id,
			Request:   req,
		}
		if err := s.Hooks.OnAuthorize(ctx, hookCtx); err != nil {
			return nil, err
		}
	}

	if req.Id == "" {
		return nil, ErrTagIDRequired
	}

	existing, err := s.StorageProvider.GetTag(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return &v1.DeleteTagResponse{Deleted: false}, nil
	}

	// BeforeTagDelete hook
	if s.Hooks.BeforeTagDelete != nil {
		if err := s.Hooks.BeforeTagDelete(ctx, existing); err != nil {
			return nil, err
		}
	}

	var entitiesUntagged int64

	if req.RemoveFromEntities {
		// Delete all entity tags for this tag
		entitiesUntagged, err = s.StorageProvider.DeleteEntityTagsByTag(ctx, req.Id)
		if err != nil {
			return nil, err
		}
	}

	// Delete the tag
	if err := s.StorageProvider.DeleteTag(ctx, req.Id); err != nil {
		return nil, err
	}

	// AfterTagDelete hook (errors logged, don't fail operation)
	if s.Hooks.AfterTagDelete != nil {
		_ = s.Hooks.AfterTagDelete(ctx, existing)
	}

	// OnEvent hook
	if s.Hooks.OnEvent != nil {
		_ = s.Hooks.OnEvent(ctx, &Event{
			Type:             EventTagDeleted,
			TagID:            req.Id,
			Tag:              existing,
			EntitiesAffected: entitiesUntagged,
		})
	}

	s.invalidateTagCache(req.Id)

	return &v1.DeleteTagResponse{
		Deleted:          true,
		EntitiesUntagged: entitiesUntagged,
	}, nil
}

// ListTags lists tags for an owner with filtering.
func (s *BaseTagsService) ListTags(ctx context.Context, req *v1.ListTagsRequest) (*v1.ListTagsResponse, error) {
	pageSize := int(req.Pagination.GetPageSize())
	if pageSize <= 0 {
		pageSize = 50
	}

	offset := 0
	if req.Pagination.GetPageToken() != "" {
		fmt.Sscanf(req.Pagination.GetPageToken(), "%d", &offset)
	}

	opts := ListTagsOptions{
		OwnerID:    req.OwnerId,
		Scope:      req.Scope,
		NameFilter: req.NameFilter,
		OrderBy:    req.OrderBy,
		Limit:      pageSize,
		Offset:     offset,
	}

	tags, total, err := s.StorageProvider.ListTags(ctx, opts)
	if err != nil {
		return nil, err
	}

	var nextToken string
	if offset+len(tags) < total {
		nextToken = fmt.Sprintf("%d", offset+len(tags))
	}

	return &v1.ListTagsResponse{
		Tags: tags,
		Pagination: &commonv1.PaginationResponse{
			NextPageToken: nextToken,
			TotalCount:    int32(total),
		},
	}, nil
}

// TagEntity applies a tag to an entity.
func (s *BaseTagsService) TagEntity(ctx context.Context, req *v1.TagEntityRequest) (*v1.TagEntityResponse, error) {
	// Resolve owner, tagger, and entity ID from request or context
	req.OwnerId = s.resolveUserID(ctx, req.OwnerId)
	req.EntityId = s.resolveEntityID(ctx, req.EntityId)
	if req.TaggedBy == "" {
		req.TaggedBy = s.resolveUserID(ctx, req.TaggedBy)
	}

	// Authorization hook
	if s.Hooks.OnAuthorize != nil {
		hookCtx := &HookContext{
			Operation: "TagEntity",
			UserID:    req.TaggedBy,
			EntityID:  req.EntityId,
			TagID:     req.TagId,
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

	// Validate entity hook
	if s.Hooks.ValidateEntity != nil {
		if err := s.Hooks.ValidateEntity(ctx, req.EntityId); err != nil {
			return nil, err
		}
	}

	var tag *v1.Tag
	var err error

	if req.TagId != "" {
		// Use existing tag by ID
		tag, err = s.StorageProvider.GetTag(ctx, req.TagId)
		if err != nil {
			return nil, err
		}
		if tag == nil {
			return nil, ErrTagNotFound
		}
	} else {
		// Find or create tag inline
		if req.Value == "" {
			return nil, ErrValueRequired
		}

		normalizedName := s.Normalizer(req.Name)
		normalizedValue := s.Normalizer(req.Value)

		// Try to find existing tag
		tag, _ = s.StorageProvider.FindTagByNormalizedValues(
			ctx, req.OwnerId, normalizedName, normalizedValue)

		if tag == nil {
			// Create new tag
			createResp, err := s.CreateTag(ctx, &v1.CreateTagRequest{
				Name:    req.Name,
				Value:   req.Value,
				Type:    req.Type,
				Color:   req.Color,
				OwnerId: req.OwnerId,
				Scope:   v1.TagScope_TAG_SCOPE_PRIVATE, // Default to private
			})
			if err != nil {
				return nil, err
			}
			tag = createResp.Tag
		}
	}

	// Check if tag is in a state that allows new entity tags
	if tag.Status == v1.TagStatus_TAG_STATUS_MIGRATING {
		return nil, ErrTagMigrating
	}

	// Determine who is applying the tag
	taggedBy := req.TaggedBy
	if taggedBy == "" {
		taggedBy = req.OwnerId
	}

	// Check if already tagged by this user
	existing, _ := s.StorageProvider.GetEntityTag(ctx, tag.Id, req.EntityId, taggedBy)
	if existing != nil {
		return &v1.TagEntityResponse{
			Tag:         tag,
			EntityTag:   existing,
			NewlyTagged: false,
		}, nil
	}

	// Determine visibility
	visibility := req.Visibility
	if visibility == v1.EntityTagVisibility_ENTITY_TAG_VISIBILITY_UNSPECIFIED {
		// Default to match tag's scope
		switch tag.Scope {
		case v1.TagScope_TAG_SCOPE_PRIVATE:
			visibility = v1.EntityTagVisibility_ENTITY_TAG_VISIBILITY_PRIVATE
		case v1.TagScope_TAG_SCOPE_SHARED:
			visibility = v1.EntityTagVisibility_ENTITY_TAG_VISIBILITY_SHARED
		case v1.TagScope_TAG_SCOPE_PUBLIC:
			visibility = v1.EntityTagVisibility_ENTITY_TAG_VISIBILITY_PUBLIC
		default:
			visibility = v1.EntityTagVisibility_ENTITY_TAG_VISIBILITY_PRIVATE
		}
	}

	// Create entity tag
	now := time.Now()
	entityTag := &v1.EntityTag{
		TagId:      tag.Id,
		EntityId:   req.EntityId,
		TaggedBy:   taggedBy,
		Visibility: visibility,
		CreatedAt:  timestamppb.New(now),
	}

	// BeforeEntityTagSave hook
	if s.Hooks.BeforeEntityTagSave != nil {
		if err := s.Hooks.BeforeEntityTagSave(ctx, entityTag, tag); err != nil {
			return nil, err
		}
	}

	if err := s.StorageProvider.SaveEntityTag(ctx, entityTag); err != nil {
		return nil, err
	}

	// AfterEntityTagSave hook (errors logged, don't fail operation)
	if s.Hooks.AfterEntityTagSave != nil {
		_ = s.Hooks.AfterEntityTagSave(ctx, entityTag, tag)
	}

	// Update tag usage count
	tag.UsageCount++
	tag.UpdatedAt = timestamppb.New(now)
	if err := s.StorageProvider.SaveTag(ctx, tag); err != nil {
		// Log but don't fail - count is denormalized
	}

	// OnEvent hook
	if s.Hooks.OnEvent != nil {
		_ = s.Hooks.OnEvent(ctx, &Event{
			Type:     EventEntityTagged,
			TagID:    tag.Id,
			EntityID: req.EntityId,
			UserID:   taggedBy,
			Tag:      tag,
		})
	}

	s.invalidateTagCache(tag.Id)

	return &v1.TagEntityResponse{
		Tag:         tag,
		EntityTag:   entityTag,
		NewlyTagged: true,
	}, nil
}

// UntagEntity removes a tag from an entity.
func (s *BaseTagsService) UntagEntity(ctx context.Context, req *v1.UntagEntityRequest) (*v1.UntagEntityResponse, error) {
	// Resolve tagged_by and entity ID from request or context
	req.TaggedBy = s.resolveUserID(ctx, req.TaggedBy)
	req.EntityId = s.resolveEntityID(ctx, req.EntityId)

	// Authorization hook
	if s.Hooks.OnAuthorize != nil {
		hookCtx := &HookContext{
			Operation: "UntagEntity",
			UserID:    req.TaggedBy,
			EntityID:  req.EntityId,
			TagID:     req.TagId,
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
	if req.TaggedBy == "" {
		return nil, ErrTaggedByRequired
	}

	var tagID string

	if req.TagId != "" {
		tagID = req.TagId
	} else {
		// Find tag by name/value
		if req.Value == "" {
			return nil, ErrValueRequired
		}

		normalizedName := s.Normalizer(req.Name)
		normalizedValue := s.Normalizer(req.Value)

		tag, _ := s.StorageProvider.FindTagByNormalizedValues(
			ctx, req.OwnerId, normalizedName, normalizedValue)
		if tag == nil {
			return &v1.UntagEntityResponse{Removed: false}, nil
		}
		tagID = tag.Id
	}

	// Check if entity tag exists
	existing, _ := s.StorageProvider.GetEntityTag(ctx, tagID, req.EntityId, req.TaggedBy)
	if existing == nil {
		return &v1.UntagEntityResponse{Removed: false}, nil
	}

	// BeforeEntityTagDelete hook
	if s.Hooks.BeforeEntityTagDelete != nil {
		if err := s.Hooks.BeforeEntityTagDelete(ctx, existing); err != nil {
			return nil, err
		}
	}

	// Delete entity tag
	if err := s.StorageProvider.DeleteEntityTag(ctx, tagID, req.EntityId, req.TaggedBy); err != nil {
		return nil, err
	}

	// AfterEntityTagDelete hook (errors logged, don't fail operation)
	if s.Hooks.AfterEntityTagDelete != nil {
		_ = s.Hooks.AfterEntityTagDelete(ctx, existing)
	}

	// Update tag usage count
	tag, _ := s.StorageProvider.GetTag(ctx, tagID)
	if tag != nil {
		tag.UsageCount--
		if tag.UsageCount < 0 {
			tag.UsageCount = 0
		}
		tag.UpdatedAt = timestamppb.New(time.Now())
		s.StorageProvider.SaveTag(ctx, tag)
		s.invalidateTagCache(tag.Id)
	}

	// OnEvent hook
	if s.Hooks.OnEvent != nil {
		_ = s.Hooks.OnEvent(ctx, &Event{
			Type:     EventEntityUntagged,
			TagID:    tagID,
			EntityID: req.EntityId,
			UserID:   req.TaggedBy,
			Tag:      tag,
		})
	}

	return &v1.UntagEntityResponse{Removed: true}, nil
}

// GetEntityTags returns all tags for a specific entity.
func (s *BaseTagsService) GetEntityTags(ctx context.Context, req *v1.GetEntityTagsRequest) (*v1.GetEntityTagsResponse, error) {
	// Resolve entity ID from request or context
	req.EntityId = s.resolveEntityID(ctx, req.EntityId)

	// Authorization hook
	if s.Hooks.OnAuthorize != nil {
		hookCtx := &HookContext{
			Operation: "GetEntityTags",
			UserID:    GetUserIDFromContext(ctx, s.UserIDContextKey),
			EntityID:  req.EntityId,
			Request:   req,
		}
		if err := s.Hooks.OnAuthorize(ctx, hookCtx); err != nil {
			return nil, err
		}
	}

	if req.EntityId == "" {
		return nil, ErrEntityRequired
	}

	opts := EntityTagsOptions{
		NameFilter: req.NameFilter,
		OwnerID:    req.OwnerId,
	}

	entityTags, err := s.StorageProvider.ListEntityTagsByEntity(ctx, req.EntityId, opts)
	if err != nil {
		return nil, err
	}

	// Fetch full tag info for each entity tag
	tags := make([]*v1.Tag, 0, len(entityTags))
	for _, et := range entityTags {
		tag, _ := s.StorageProvider.GetTag(ctx, et.TagId)
		if tag != nil {
			tags = append(tags, tag)
		}
	}

	// AfterTagsRead hook
	if len(tags) > 0 && s.Hooks.AfterTagsRead != nil {
		_ = s.Hooks.AfterTagsRead(ctx, tags)
	}

	return &v1.GetEntityTagsResponse{Tags: tags}, nil
}

// GetEntitiesWithTag returns all entities that have a specific tag.
func (s *BaseTagsService) GetEntitiesWithTag(ctx context.Context, req *v1.GetEntitiesWithTagRequest) (*v1.GetEntitiesWithTagResponse, error) {
	var tagID string

	if req.TagId != "" {
		tagID = req.TagId
	} else {
		// Find tag by name/value
		if req.Value == "" {
			return nil, ErrValueRequired
		}

		normalizedName := s.Normalizer(req.Name)
		normalizedValue := s.Normalizer(req.Value)

		tag, _ := s.StorageProvider.FindTagByNormalizedValues(
			ctx, req.OwnerId, normalizedName, normalizedValue)
		if tag == nil {
			return &v1.GetEntitiesWithTagResponse{
				EntityIds:  []string{},
				Pagination: &commonv1.PaginationResponse{},
			}, nil
		}
		tagID = tag.Id
	}

	pageSize := int(req.Pagination.GetPageSize())
	if pageSize <= 0 {
		pageSize = 50
	}

	offset := 0
	if req.Pagination.GetPageToken() != "" {
		fmt.Sscanf(req.Pagination.GetPageToken(), "%d", &offset)
	}

	entityTags, total, err := s.StorageProvider.ListEntityTagsByTag(ctx, tagID, pageSize, offset)
	if err != nil {
		return nil, err
	}

	entityIds := make([]string, len(entityTags))
	for i, et := range entityTags {
		entityIds[i] = et.EntityId
	}

	var nextToken string
	if offset+len(entityTags) < total {
		nextToken = fmt.Sprintf("%d", offset+len(entityTags))
	}

	return &v1.GetEntitiesWithTagResponse{
		EntityIds: entityIds,
		Pagination: &commonv1.PaginationResponse{
			NextPageToken: nextToken,
			TotalCount:    int32(total),
		},
	}, nil
}

// BatchTagEntities applies a tag to multiple entities at once.
func (s *BaseTagsService) BatchTagEntities(ctx context.Context, req *v1.BatchTagEntitiesRequest) (*v1.BatchTagEntitiesResponse, error) {
	var tag *v1.Tag
	var err error

	if req.TagId != "" {
		tag, err = s.StorageProvider.GetTag(ctx, req.TagId)
		if err != nil {
			return nil, err
		}
		if tag == nil {
			return nil, ErrTagNotFound
		}
	} else {
		// Find or create tag
		if req.Value == "" {
			return nil, ErrValueRequired
		}

		normalizedName := s.Normalizer(req.Name)
		normalizedValue := s.Normalizer(req.Value)

		tag, _ = s.StorageProvider.FindTagByNormalizedValues(
			ctx, req.OwnerId, normalizedName, normalizedValue)

		if tag == nil {
			createResp, err := s.CreateTag(ctx, &v1.CreateTagRequest{
				Name:    req.Name,
				Value:   req.Value,
				Type:    req.Type,
				OwnerId: req.OwnerId,
				Scope:   req.Scope,
			})
			if err != nil {
				return nil, err
			}
			tag = createResp.Tag
		}
	}

	var entitiesTagged, alreadyTagged int64

	for _, entityId := range req.EntityIds {
		resp, err := s.TagEntity(ctx, &v1.TagEntityRequest{
			EntityId: entityId,
			TagId:    tag.Id,
			TaggedBy: req.TaggedBy,
		})
		if err != nil {
			continue // Skip failed ones
		}
		if resp.NewlyTagged {
			entitiesTagged++
		} else {
			alreadyTagged++
		}
	}

	return &v1.BatchTagEntitiesResponse{
		Tag:            tag,
		EntitiesTagged: entitiesTagged,
		AlreadyTagged:  alreadyTagged,
	}, nil
}

// BatchGetEntityTags returns tags for multiple entities at once.
func (s *BaseTagsService) BatchGetEntityTags(ctx context.Context, req *v1.BatchGetEntityTagsRequest) (*v1.BatchGetEntityTagsResponse, error) {
	result := make(map[string]*v1.EntityTagList)

	for _, entityId := range req.EntityIds {
		resp, err := s.GetEntityTags(ctx, &v1.GetEntityTagsRequest{
			EntityId: entityId,
			OwnerId:  req.OwnerId,
		})
		if err != nil {
			continue
		}

		key := entityId
		result[key] = &v1.EntityTagList{Tags: resp.Tags}
	}

	return &v1.BatchGetEntityTagsResponse{EntityTags: result}, nil
}

// SearchTags searches for tags by query (prefix match on value).
func (s *BaseTagsService) SearchTags(ctx context.Context, req *v1.SearchTagsRequest) (*v1.SearchTagsResponse, error) {
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	}

	opts := SearchTagsOptions{
		Query:         req.Query,
		NameFilter:    req.NameFilter,
		OwnerID:       req.OwnerId,
		IncludeShared: req.IncludeShared,
		Limit:         limit,
	}

	tags, err := s.StorageProvider.SearchTags(ctx, opts)
	if err != nil {
		return nil, err
	}

	return &v1.SearchTagsResponse{Tags: tags}, nil
}

// GetPopularTags returns the most used tags.
func (s *BaseTagsService) GetPopularTags(ctx context.Context, req *v1.GetPopularTagsRequest) (*v1.GetPopularTagsResponse, error) {
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	}

	opts := PopularTagsOptions{
		NameFilter:    req.NameFilter,
		OwnerID:       req.OwnerId,
		IncludeShared: req.IncludeShared,
		Limit:         limit,
	}

	tags, err := s.StorageProvider.GetPopularTags(ctx, opts)
	if err != nil {
		return nil, err
	}

	return &v1.GetPopularTagsResponse{Tags: tags}, nil
}

// MergeTags merges two tags into one (admin operation).
func (s *BaseTagsService) MergeTags(ctx context.Context, req *v1.MergeTagsRequest) (*v1.MergeTagsResponse, error) {
	if req.SourceTagId == "" || req.TargetTagId == "" {
		return nil, ErrTagIDRequired
	}

	sourceTag, err := s.StorageProvider.GetTag(ctx, req.SourceTagId)
	if err != nil || sourceTag == nil {
		return nil, ErrTagNotFound
	}

	targetTag, err := s.StorageProvider.GetTag(ctx, req.TargetTagId)
	if err != nil || targetTag == nil {
		return nil, ErrTagNotFound
	}

	// Get all entity tags for source
	entityTags, _, err := s.StorageProvider.ListEntityTagsByTag(ctx, req.SourceTagId, 10000, 0)
	if err != nil {
		return nil, err
	}

	var entitiesRetagged int64

	// Re-tag each entity with target tag
	for _, et := range entityTags {
		// Check if already tagged with target
		existing, _ := s.StorageProvider.GetEntityTag(ctx, req.TargetTagId, et.EntityId, et.TaggedBy)
		if existing != nil {
			continue // Skip if already tagged
		}

		// Create new entity tag for target
		newET := &v1.EntityTag{
			TagId:      req.TargetTagId,
			EntityId:   et.EntityId,
			TaggedBy:   et.TaggedBy,
			Visibility: et.Visibility,
			CreatedAt:  et.CreatedAt,
		}
		if err := s.StorageProvider.SaveEntityTag(ctx, newET); err != nil {
			continue
		}
		entitiesRetagged++
	}

	// Delete source tag entity tags
	s.StorageProvider.DeleteEntityTagsByTag(ctx, req.SourceTagId)

	// Update source tag to redirect
	sourceTag.Status = v1.TagStatus_TAG_STATUS_REDIRECT
	sourceTag.RedirectToTagId = req.TargetTagId
	sourceTag.UpdatedAt = timestamppb.New(time.Now())
	s.StorageProvider.SaveTag(ctx, sourceTag)

	// Update target tag usage count
	targetTag.UsageCount += entitiesRetagged
	targetTag.UpdatedAt = timestamppb.New(time.Now())
	s.StorageProvider.SaveTag(ctx, targetTag)

	s.invalidateTagCache(req.SourceTagId)
	s.invalidateTagCache(req.TargetTagId)

	return &v1.MergeTagsResponse{
		Tag:              targetTag,
		EntitiesRetagged: entitiesRetagged,
	}, nil
}

// PromoteTag promotes a private tag to public scope.
func (s *BaseTagsService) PromoteTag(ctx context.Context, req *v1.PromoteTagRequest) (*v1.PromoteTagResponse, error) {
	if req.SourceTagId == "" {
		return nil, ErrTagIDRequired
	}

	sourceTag, err := s.StorageProvider.GetTag(ctx, req.SourceTagId)
	if err != nil || sourceTag == nil {
		return nil, ErrTagNotFound
	}

	// Mark source as migrating to block new entity tags
	sourceTag.Status = v1.TagStatus_TAG_STATUS_MIGRATING
	sourceTag.UpdatedAt = timestamppb.New(time.Now())
	if err := s.StorageProvider.SaveTag(ctx, sourceTag); err != nil {
		return nil, err
	}

	// Find or create target tag
	normalizedName := sourceTag.NormalizedName
	normalizedValue := sourceTag.NormalizedValue

	targetTag, _ := s.StorageProvider.FindTagByNormalizedValues(
		ctx, req.TargetOwnerId, normalizedName, normalizedValue)

	mergedIntoExisting := false
	if targetTag != nil {
		if !req.MergeIfExists {
			// Restore source tag status
			sourceTag.Status = v1.TagStatus_TAG_STATUS_ACTIVE
			s.StorageProvider.SaveTag(ctx, sourceTag)
			return nil, ErrTargetTagExists
		}
		mergedIntoExisting = true
	} else {
		// Create target tag
		createResp, err := s.CreateTag(ctx, &v1.CreateTagRequest{
			Name:        sourceTag.Name,
			Value:       sourceTag.Value,
			Type:        sourceTag.Type,
			Color:       sourceTag.Color,
			Description: sourceTag.Description,
			OwnerId:     req.TargetOwnerId,
			Scope:       v1.TagScope_TAG_SCOPE_PUBLIC,
		})
		if err != nil {
			// Restore source tag status
			sourceTag.Status = v1.TagStatus_TAG_STATUS_ACTIVE
			s.StorageProvider.SaveTag(ctx, sourceTag)
			return nil, err
		}
		targetTag = createResp.Tag
	}

	// Migrate entity tags
	entityTags, _, _ := s.StorageProvider.ListEntityTagsByTag(ctx, req.SourceTagId, 10000, 0)

	var entityTagsMigrated int64
	newVisibility := req.NewVisibility
	if newVisibility == v1.EntityTagVisibility_ENTITY_TAG_VISIBILITY_UNSPECIFIED {
		newVisibility = v1.EntityTagVisibility_ENTITY_TAG_VISIBILITY_PUBLIC
	}

	for _, et := range entityTags {
		// Check if already exists on target
		existing, _ := s.StorageProvider.GetEntityTag(ctx, targetTag.Id, et.EntityId, et.TaggedBy)
		if existing != nil {
			continue
		}

		newET := &v1.EntityTag{
			TagId:      targetTag.Id,
			EntityId:   et.EntityId,
			TaggedBy:   et.TaggedBy,
			Visibility: newVisibility,
			CreatedAt:  et.CreatedAt,
		}
		if err := s.StorageProvider.SaveEntityTag(ctx, newET); err != nil {
			continue
		}
		entityTagsMigrated++
	}

	// Delete source entity tags
	s.StorageProvider.DeleteEntityTagsByTag(ctx, req.SourceTagId)

	// Update source to redirect
	sourceTag.Status = v1.TagStatus_TAG_STATUS_REDIRECT
	sourceTag.RedirectToTagId = targetTag.Id
	sourceTag.UpdatedAt = timestamppb.New(time.Now())
	s.StorageProvider.SaveTag(ctx, sourceTag)

	// Update target usage count
	targetTag.UsageCount += entityTagsMigrated
	targetTag.UpdatedAt = timestamppb.New(time.Now())
	s.StorageProvider.SaveTag(ctx, targetTag)

	s.invalidateTagCache(req.SourceTagId)
	s.invalidateTagCache(targetTag.Id)

	return &v1.PromoteTagResponse{
		Tag:                targetTag,
		SourceTag:          sourceTag,
		EntityTagsMigrated: entityTagsMigrated,
		MergedIntoExisting: mergedIntoExisting,
	}, nil
}

// Helper methods

func (s *BaseTagsService) cacheTag(tag *v1.Tag) {
	if s.CacheEnabled && tag != nil {
		s.CacheMu.Lock()
		s.TagCache[tag.Id] = tag
		s.CacheMu.Unlock()
	}
}

func (s *BaseTagsService) getCachedTag(id string) *v1.Tag {
	if !s.CacheEnabled {
		return nil
	}
	s.CacheMu.RLock()
	defer s.CacheMu.RUnlock()
	return s.TagCache[id]
}

func (s *BaseTagsService) invalidateTagCache(id string) {
	if s.CacheEnabled {
		s.CacheMu.Lock()
		delete(s.TagCache, id)
		s.CacheMu.Unlock()
	}
}

// generateID generates a unique ID for a tag.
func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// Error types
var (
	ErrTagIDRequired    = &TagsError{Message: "tag id is required"}
	ErrValueRequired    = &TagsError{Message: "value is required"}
	ErrEntityRequired   = &TagsError{Message: "entity_id is required"}
	ErrTaggedByRequired = &TagsError{Message: "tagged_by is required"}
	ErrTagNotFound      = &TagsError{Message: "tag not found"}
	ErrTagMigrating     = &TagsError{Message: "tag is currently being migrated, new applications not allowed"}
	ErrTargetTagExists  = &TagsError{Message: "target tag already exists, set merge_if_exists=true to merge"}
)

// TagsError represents an error in the tags service.
type TagsError struct {
	Message string
}

func (e *TagsError) Error() string {
	return e.Message
}
