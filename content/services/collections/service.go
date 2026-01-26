// Package collections provides a reusable collections/folders service for organizing entities.
// The service supports hierarchical collections with nesting, multi-parent support for items,
// and denormalized counts for fast reads.
//
// # Architecture
//
// The package follows a layered architecture:
//
//   - CollectionsService interface: defines the contract for collection operations
//   - BaseCollectionsService: provides shared logic (normalization, path management, counts)
//   - CollectionsStorageProvider: abstracts raw storage operations
//   - Backend implementations (gorm, gae): concrete storage providers
//
// # Usage
//
// Applications should use one of the concrete backend implementations:
//
//	// GORM backend (for PostgreSQL/MySQL/SQLite)
//	collectionsService := gorm.NewCollectionsService(db)
//
//	// GAE Datastore backend (for Google Cloud)
//	collectionsService := gae.NewCollectionsService(client, "namespace")
//
// # Key Features
//
//   - Hierarchical collections (folders within folders)
//   - Path array for efficient subtree queries
//   - Multi-parent items (entity can be in multiple collections)
//   - Denormalized item and child counts
//   - Flexible type field for UI customization
package collections

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	v1 "github.com/panyam/goapplib/content/gen/go/collections/v1"
	commonv1 "github.com/panyam/goapplib/content/gen/go/common/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// CollectionsService defines the interface for collection operations.
type CollectionsService interface {
	// Collection CRUD
	CreateCollection(ctx context.Context, req *v1.CreateCollectionRequest) (*v1.CreateCollectionResponse, error)
	GetCollection(ctx context.Context, req *v1.GetCollectionRequest) (*v1.GetCollectionResponse, error)
	UpdateCollection(ctx context.Context, req *v1.UpdateCollectionRequest) (*v1.UpdateCollectionResponse, error)
	DeleteCollection(ctx context.Context, req *v1.DeleteCollectionRequest) (*v1.DeleteCollectionResponse, error)
	ListCollections(ctx context.Context, req *v1.ListCollectionsRequest) (*v1.ListCollectionsResponse, error)

	// Hierarchy
	GetCollectionTree(ctx context.Context, req *v1.GetCollectionTreeRequest) (*v1.GetCollectionTreeResponse, error)
	MoveCollection(ctx context.Context, req *v1.MoveCollectionRequest) (*v1.MoveCollectionResponse, error)
	GetCollectionPath(ctx context.Context, req *v1.GetCollectionPathRequest) (*v1.GetCollectionPathResponse, error)

	// Items
	AddToCollection(ctx context.Context, req *v1.AddToCollectionRequest) (*v1.AddToCollectionResponse, error)
	RemoveFromCollection(ctx context.Context, req *v1.RemoveFromCollectionRequest) (*v1.RemoveFromCollectionResponse, error)
	GetCollectionItems(ctx context.Context, req *v1.GetCollectionItemsRequest) (*v1.GetCollectionItemsResponse, error)
	GetEntityCollections(ctx context.Context, req *v1.GetEntityCollectionsRequest) (*v1.GetEntityCollectionsResponse, error)
	ReorderItems(ctx context.Context, req *v1.ReorderItemsRequest) (*v1.ReorderItemsResponse, error)

	// Batch
	BatchAddToCollection(ctx context.Context, req *v1.BatchAddToCollectionRequest) (*v1.BatchAddToCollectionResponse, error)
	BatchGetEntityCollections(ctx context.Context, req *v1.BatchGetEntityCollectionsRequest) (*v1.BatchGetEntityCollectionsResponse, error)
}

// CollectionsStorageProvider is implemented by concrete backends (gorm, gae)
// to provide raw storage operations for collections.
type CollectionsStorageProvider interface {
	// Collection operations
	SaveCollection(ctx context.Context, collection *v1.Collection) error
	GetCollection(ctx context.Context, id string) (*v1.Collection, error)
	DeleteCollection(ctx context.Context, id string) error
	ListCollections(ctx context.Context, opts ListCollectionsOptions) ([]*v1.Collection, int, error)
	FindCollectionByName(ctx context.Context, ownerID, parentID, normalizedName string) (*v1.Collection, error)
	GetCollectionsByPath(ctx context.Context, ancestorID string) ([]*v1.Collection, error)
	UpdateCollectionPaths(ctx context.Context, collectionID string, oldPath, newPath []string) (int64, error)

	// Item operations
	SaveCollectionItem(ctx context.Context, item *v1.CollectionItem) error
	GetCollectionItem(ctx context.Context, collectionID, entityID string) (*v1.CollectionItem, error)
	DeleteCollectionItem(ctx context.Context, collectionID, entityID string) error
	ListCollectionItems(ctx context.Context, collectionID string, opts ListItemsOptions) ([]*v1.CollectionItem, int, error)
	ListEntityCollections(ctx context.Context, entityID string, opts EntityCollectionsOptions) ([]*v1.Collection, error)
	DeleteItemsByCollection(ctx context.Context, collectionID string) (int64, error)
	UpdateItemOrders(ctx context.Context, collectionID string, orders []*v1.ItemOrder) (int64, error)
	GetMaxDisplayOrder(ctx context.Context, collectionID string) (int32, error)
}

// ListCollectionsOptions contains options for listing collections.
type ListCollectionsOptions struct {
	OwnerID    string // "type:id" format (e.g., "user:123")
	ParentID   string
	Type       string
	Visibility v1.CollectionVisibility
	OrderBy    string // "name", "created_at", "display_order"
	Limit      int
	Offset     int
}

// ListItemsOptions contains options for listing collection items.
type ListItemsOptions struct {
	SortBy         v1.SortField
	SortDescending bool
	Limit          int
	Offset         int
}

// EntityCollectionsOptions contains options for listing collections containing an entity.
type EntityCollectionsOptions struct {
	OwnerID string // "type:id" format (e.g., "user:123")
}

// BaseCollectionsService provides shared logic for collections services.
// Embeds UnimplementedCollectionsServiceServer for gRPC forward compatibility.
type BaseCollectionsService struct {
	v1.UnimplementedCollectionsServiceServer
	StorageProvider CollectionsStorageProvider

	// Optional in-memory cache for collection lookups
	CacheEnabled    bool
	CollectionCache map[string]*v1.Collection // key: collection ID
	CacheMu         sync.RWMutex

	// Normalizer function (defaults to lowercase + trim)
	Normalizer func(string) string

	// Maximum allowed depth (0 = unlimited)
	MaxDepth int32

	// UserIDContextKey is the context key for reading user ID.
	// Defaults to DefaultUserIDContextKey if not set.
	UserIDContextKey any

	// Hooks for customization
	Hooks Hooks
}

// Compile-time check that BaseCollectionsService implements CollectionsServiceServer.
var _ v1.CollectionsServiceServer = (*BaseCollectionsService)(nil)

// NewBaseCollectionsService creates a new BaseCollectionsService with the given storage provider.
func NewBaseCollectionsService(provider CollectionsStorageProvider, opts ...ServiceOption) *BaseCollectionsService {
	s := &BaseCollectionsService{
		StorageProvider: provider,
		Normalizer:      DefaultNormalizer,
		MaxDepth:        0, // Unlimited by default
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// resolveUserID returns the user ID from the request, falling back to context if empty.
// This allows interceptors/middleware to set user ID in context.
func (s *BaseCollectionsService) resolveUserID(ctx context.Context, requestUserID string) string {
	if requestUserID != "" {
		return requestUserID
	}
	return GetUserIDFromContext(ctx, s.UserIDContextKey)
}

// DefaultNormalizer is the default normalization function.
// It lowercases and trims whitespace.
func DefaultNormalizer(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}


// InitializeCache sets up the in-memory cache.
func (s *BaseCollectionsService) InitializeCache() {
	s.CacheEnabled = true
	s.CollectionCache = make(map[string]*v1.Collection)
}

// CreateCollection creates a new collection or returns existing if duplicate.
func (s *BaseCollectionsService) CreateCollection(ctx context.Context, req *v1.CreateCollectionRequest) (*v1.CreateCollectionResponse, error) {
	// Resolve owner ID from request or context
	req.OwnerId = s.resolveUserID(ctx, req.OwnerId)

	// Authorization hook
	if s.Hooks.OnAuthorize != nil {
		hookCtx := &HookContext{
			Operation: "CreateCollection",
			UserID:    req.OwnerId,
			Request:   req,
		}
		if err := s.Hooks.OnAuthorize(ctx, hookCtx); err != nil {
			return nil, err
		}
	}

	if req.Name == "" {
		return nil, ErrNameRequired
	}

	// Normalize name
	normalizedName := s.Normalizer(req.Name)

	// Check for existing collection with same normalized name in same parent
	existing, _ := s.StorageProvider.FindCollectionByName(
		ctx, req.OwnerId, req.ParentId, normalizedName)
	if existing != nil {
		return &v1.CreateCollectionResponse{
			Collection:     existing,
			AlreadyExisted: true,
		}, nil
	}

	// Build path and depth from parent
	var path []string
	var depth int32

	if req.ParentId != "" {
		parent, err := s.StorageProvider.GetCollection(ctx, req.ParentId)
		if err != nil {
			return nil, err
		}
		if parent == nil {
			return nil, ErrParentNotFound
		}

		// Build path: parent's path + parent's ID
		path = append(path, parent.Path...)
		path = append(path, parent.Id)
		depth = parent.Depth + 1

		// Check max depth
		if s.MaxDepth > 0 && depth > s.MaxDepth {
			return nil, ErrMaxDepthExceeded
		}
	}

	// Create new collection
	now := time.Now()
	collection := &v1.Collection{
		Id:             generateID(),
		Name:           req.Name,
		NormalizedName: normalizedName,
		Description:    req.Description,
		OwnerId:        req.OwnerId,
		ParentId:       req.ParentId,
		Path:           path,
		Depth:          depth,
		DisplayOrder:   req.DisplayOrder,
		Type:           req.Type,
		Icon:           req.Icon,
		Color:          req.Color,
		ItemCount:      0,
		ChildCount:     0,
		Visibility:     req.Visibility,
		Status:         v1.CollectionStatus_COLLECTION_STATUS_ACTIVE,
		CreatedAt:      timestamppb.New(now),
		UpdatedAt:      timestamppb.New(now),
		CreatorId:      req.OwnerId,
	}

	// Default visibility to private if not specified
	if collection.Visibility == v1.CollectionVisibility_COLLECTION_VISIBILITY_UNSPECIFIED {
		collection.Visibility = v1.CollectionVisibility_COLLECTION_VISIBILITY_PRIVATE
	}

	// BeforeCollectionSave hook
	if s.Hooks.BeforeCollectionSave != nil {
		if err := s.Hooks.BeforeCollectionSave(ctx, collection); err != nil {
			return nil, err
		}
	}

	if err := s.StorageProvider.SaveCollection(ctx, collection); err != nil {
		return nil, err
	}

	// AfterCollectionSave hook (errors logged, don't fail operation)
	if s.Hooks.AfterCollectionSave != nil {
		_ = s.Hooks.AfterCollectionSave(ctx, collection)
	}

	// Update parent's child_count
	if req.ParentId != "" {
		s.incrementChildCount(ctx, req.ParentId, 1)
	}

	// OnEvent hook
	if s.Hooks.OnEvent != nil {
		_ = s.Hooks.OnEvent(ctx, &Event{
			Type:         EventCollectionCreated,
			CollectionID: collection.Id,
			UserID:       req.OwnerId,
			Collection:   collection,
		})
	}

	s.cacheCollection(collection)

	return &v1.CreateCollectionResponse{
		Collection:     collection,
		AlreadyExisted: false,
	}, nil
}

// GetCollection retrieves a collection by ID.
func (s *BaseCollectionsService) GetCollection(ctx context.Context, req *v1.GetCollectionRequest) (*v1.GetCollectionResponse, error) {
	if req.Id == "" {
		return nil, ErrCollectionIDRequired
	}

	// Check cache first
	if collection := s.getCachedCollection(req.Id); collection != nil {
		return &v1.GetCollectionResponse{Collection: collection}, nil
	}

	collection, err := s.StorageProvider.GetCollection(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if collection == nil {
		return nil, ErrCollectionNotFound
	}

	s.cacheCollection(collection)

	return &v1.GetCollectionResponse{Collection: collection}, nil
}

// UpdateCollection updates a collection's properties.
func (s *BaseCollectionsService) UpdateCollection(ctx context.Context, req *v1.UpdateCollectionRequest) (*v1.UpdateCollectionResponse, error) {
	// Authorization hook
	if s.Hooks.OnAuthorize != nil {
		hookCtx := &HookContext{
			Operation:    "UpdateCollection",
			UserID:       GetUserIDFromContext(ctx, s.UserIDContextKey),
			CollectionID: req.Collection.GetId(),
			Request:      req,
		}
		if err := s.Hooks.OnAuthorize(ctx, hookCtx); err != nil {
			return nil, err
		}
	}

	if req.Collection == nil || req.Collection.Id == "" {
		return nil, ErrCollectionIDRequired
	}

	existing, err := s.StorageProvider.GetCollection(ctx, req.Collection.Id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrCollectionNotFound
	}

	// Update fields based on update_mask or all non-empty fields
	updateMask := make(map[string]bool)
	for _, field := range req.UpdateMask {
		updateMask[field] = true
	}
	useAllFields := len(updateMask) == 0

	if useAllFields || updateMask["name"] {
		if req.Collection.Name != "" {
			existing.Name = req.Collection.Name
			existing.NormalizedName = s.Normalizer(req.Collection.Name)
		}
	}
	if useAllFields || updateMask["description"] {
		existing.Description = req.Collection.Description
	}
	if useAllFields || updateMask["type"] {
		existing.Type = req.Collection.Type
	}
	if useAllFields || updateMask["icon"] {
		existing.Icon = req.Collection.Icon
	}
	if useAllFields || updateMask["color"] {
		existing.Color = req.Collection.Color
	}
	if useAllFields || updateMask["display_order"] {
		existing.DisplayOrder = req.Collection.DisplayOrder
	}
	if useAllFields || updateMask["visibility"] {
		if req.Collection.Visibility != v1.CollectionVisibility_COLLECTION_VISIBILITY_UNSPECIFIED {
			existing.Visibility = req.Collection.Visibility
		}
	}

	existing.UpdatedAt = timestamppb.New(time.Now())

	// BeforeCollectionSave hook
	if s.Hooks.BeforeCollectionSave != nil {
		if err := s.Hooks.BeforeCollectionSave(ctx, existing); err != nil {
			return nil, err
		}
	}

	if err := s.StorageProvider.SaveCollection(ctx, existing); err != nil {
		return nil, err
	}

	// AfterCollectionSave hook (errors logged, don't fail operation)
	if s.Hooks.AfterCollectionSave != nil {
		_ = s.Hooks.AfterCollectionSave(ctx, existing)
	}

	// OnEvent hook
	if s.Hooks.OnEvent != nil {
		_ = s.Hooks.OnEvent(ctx, &Event{
			Type:         EventCollectionUpdated,
			CollectionID: existing.Id,
			UserID:       GetUserIDFromContext(ctx, s.UserIDContextKey),
			Collection:   existing,
		})
	}

	s.invalidateCollectionCache(existing.Id)

	return &v1.UpdateCollectionResponse{Collection: existing}, nil
}

// DeleteCollection removes a collection.
func (s *BaseCollectionsService) DeleteCollection(ctx context.Context, req *v1.DeleteCollectionRequest) (*v1.DeleteCollectionResponse, error) {
	// Authorization hook
	if s.Hooks.OnAuthorize != nil {
		hookCtx := &HookContext{
			Operation:    "DeleteCollection",
			UserID:       GetUserIDFromContext(ctx, s.UserIDContextKey),
			CollectionID: req.Id,
			Request:      req,
		}
		if err := s.Hooks.OnAuthorize(ctx, hookCtx); err != nil {
			return nil, err
		}
	}

	if req.Id == "" {
		return nil, ErrCollectionIDRequired
	}

	existing, err := s.StorageProvider.GetCollection(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return &v1.DeleteCollectionResponse{Deleted: false}, nil
	}

	// BeforeCollectionDelete hook
	if s.Hooks.BeforeCollectionDelete != nil {
		if err := s.Hooks.BeforeCollectionDelete(ctx, existing); err != nil {
			return nil, err
		}
	}

	// Check if collection has children or items
	if !req.Recursive && !req.ForceDelete {
		if existing.ChildCount > 0 {
			return nil, ErrCollectionHasChildren
		}
		if existing.ItemCount > 0 {
			return nil, ErrCollectionHasItems
		}
	}

	var childrenDeleted, itemsRemoved int64

	if req.Recursive || req.ForceDelete {
		// Delete items in this collection
		itemsRemoved, _ = s.StorageProvider.DeleteItemsByCollection(ctx, req.Id)

		if req.Recursive {
			// Get all descendants and delete them
			descendants, _ := s.StorageProvider.GetCollectionsByPath(ctx, req.Id)
			for _, desc := range descendants {
				s.StorageProvider.DeleteItemsByCollection(ctx, desc.Id)
				s.StorageProvider.DeleteCollection(ctx, desc.Id)
				childrenDeleted++
			}
		}
	}

	// Delete the collection
	if err := s.StorageProvider.DeleteCollection(ctx, req.Id); err != nil {
		return nil, err
	}

	// AfterCollectionDelete hook (errors logged, don't fail operation)
	if s.Hooks.AfterCollectionDelete != nil {
		_ = s.Hooks.AfterCollectionDelete(ctx, existing)
	}

	// Update parent's child_count
	if existing.ParentId != "" {
		s.incrementChildCount(ctx, existing.ParentId, -1)
	}

	// OnEvent hook
	if s.Hooks.OnEvent != nil {
		_ = s.Hooks.OnEvent(ctx, &Event{
			Type:             EventCollectionDeleted,
			CollectionID:     req.Id,
			UserID:           GetUserIDFromContext(ctx, s.UserIDContextKey),
			Collection:       existing,
			ChildrenAffected: childrenDeleted,
			ItemsAffected:    itemsRemoved,
		})
	}

	s.invalidateCollectionCache(req.Id)

	return &v1.DeleteCollectionResponse{
		Deleted:         true,
		ChildrenDeleted: childrenDeleted,
		ItemsRemoved:    itemsRemoved,
	}, nil
}

// ListCollections lists collections for an owner with filtering.
func (s *BaseCollectionsService) ListCollections(ctx context.Context, req *v1.ListCollectionsRequest) (*v1.ListCollectionsResponse, error) {
	pageSize := int(req.Pagination.GetPageSize())
	if pageSize <= 0 {
		pageSize = 50
	}

	offset := 0
	if req.Pagination.GetPageToken() != "" {
		fmt.Sscanf(req.Pagination.GetPageToken(), "%d", &offset)
	}

	opts := ListCollectionsOptions{
		OwnerID:    req.OwnerId,
		ParentID:   req.ParentId,
		Type:       req.Type,
		Visibility: req.Visibility,
		OrderBy:    req.OrderBy,
		Limit:      pageSize,
		Offset:     offset,
	}

	collections, total, err := s.StorageProvider.ListCollections(ctx, opts)
	if err != nil {
		return nil, err
	}

	// AfterCollectionsRead hook
	if len(collections) > 0 && s.Hooks.AfterCollectionsRead != nil {
		_ = s.Hooks.AfterCollectionsRead(ctx, collections)
	}

	var nextToken string
	if offset+len(collections) < total {
		nextToken = fmt.Sprintf("%d", offset+len(collections))
	}

	return &v1.ListCollectionsResponse{
		Collections: collections,
		Pagination: &commonv1.PaginationResponse{
			NextPageToken: nextToken,
			TotalCount:    int32(total),
		},
	}, nil
}

// GetCollectionTree returns a collection and its children as a tree structure.
func (s *BaseCollectionsService) GetCollectionTree(ctx context.Context, req *v1.GetCollectionTreeRequest) (*v1.GetCollectionTreeResponse, error) {
	// Get root collections or specific collection's children
	opts := ListCollectionsOptions{
		ParentID: req.Id,
		Limit:    1000, // Reasonable limit for tree
	}

	collections, _, err := s.StorageProvider.ListCollections(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Build tree nodes
	nodes := make([]*v1.CollectionTreeNode, 0, len(collections))
	for _, coll := range collections {
		node := &v1.CollectionTreeNode{
			Collection: coll,
		}

		// Recursively get children if not at max depth
		if req.MaxDepth == 0 || coll.Depth < req.MaxDepth {
			childReq := &v1.GetCollectionTreeRequest{
				Id:       coll.Id,
				MaxDepth: req.MaxDepth,
			}
			childResp, err := s.GetCollectionTree(ctx, childReq)
			if err == nil {
				node.Children = childResp.Nodes
			}
		}

		nodes = append(nodes, node)
	}

	return &v1.GetCollectionTreeResponse{Nodes: nodes}, nil
}

// MoveCollection moves a collection to a new parent.
func (s *BaseCollectionsService) MoveCollection(ctx context.Context, req *v1.MoveCollectionRequest) (*v1.MoveCollectionResponse, error) {
	// Authorization hook
	if s.Hooks.OnAuthorize != nil {
		hookCtx := &HookContext{
			Operation:    "MoveCollection",
			UserID:       GetUserIDFromContext(ctx, s.UserIDContextKey),
			CollectionID: req.Id,
			Request:      req,
		}
		if err := s.Hooks.OnAuthorize(ctx, hookCtx); err != nil {
			return nil, err
		}
	}

	if req.Id == "" {
		return nil, ErrCollectionIDRequired
	}

	collection, err := s.StorageProvider.GetCollection(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if collection == nil {
		return nil, ErrCollectionNotFound
	}

	// Build new path and depth
	var newPath []string
	var newDepth int32

	if req.NewParentId != "" {
		newParent, err := s.StorageProvider.GetCollection(ctx, req.NewParentId)
		if err != nil {
			return nil, err
		}
		if newParent == nil {
			return nil, ErrParentNotFound
		}

		// Check for circular reference
		for _, ancestorID := range newParent.Path {
			if ancestorID == req.Id {
				return nil, ErrCircularReference
			}
		}
		if req.NewParentId == req.Id {
			return nil, ErrCircularReference
		}

		newPath = append(newPath, newParent.Path...)
		newPath = append(newPath, newParent.Id)
		newDepth = newParent.Depth + 1

		// Check max depth
		if s.MaxDepth > 0 && newDepth > s.MaxDepth {
			return nil, ErrMaxDepthExceeded
		}
	}

	oldPath := collection.Path
	oldParentID := collection.ParentId

	// Update collection's path and parent
	collection.Path = newPath
	collection.Depth = newDepth
	collection.ParentId = req.NewParentId
	if req.NewDisplayOrder > 0 {
		collection.DisplayOrder = req.NewDisplayOrder
	}
	collection.UpdatedAt = timestamppb.New(time.Now())

	if err := s.StorageProvider.SaveCollection(ctx, collection); err != nil {
		return nil, err
	}

	// Update all descendants' paths
	descendantsUpdated, _ := s.StorageProvider.UpdateCollectionPaths(ctx, req.Id, oldPath, newPath)

	// Update parent child counts
	if oldParentID != "" {
		s.incrementChildCount(ctx, oldParentID, -1)
	}
	if req.NewParentId != "" {
		s.incrementChildCount(ctx, req.NewParentId, 1)
	}

	// OnEvent hook
	if s.Hooks.OnEvent != nil {
		_ = s.Hooks.OnEvent(ctx, &Event{
			Type:             EventCollectionMoved,
			CollectionID:     req.Id,
			UserID:           GetUserIDFromContext(ctx, s.UserIDContextKey),
			Collection:       collection,
			OldParentID:      oldParentID,
			NewParentID:      req.NewParentId,
			ChildrenAffected: descendantsUpdated,
		})
	}

	s.invalidateCollectionCache(req.Id)

	return &v1.MoveCollectionResponse{
		Collection:         collection,
		DescendantsUpdated: descendantsUpdated,
	}, nil
}

// GetCollectionPath returns the ancestors of a collection (breadcrumb).
func (s *BaseCollectionsService) GetCollectionPath(ctx context.Context, req *v1.GetCollectionPathRequest) (*v1.GetCollectionPathResponse, error) {
	if req.Id == "" {
		return nil, ErrCollectionIDRequired
	}

	collection, err := s.StorageProvider.GetCollection(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if collection == nil {
		return nil, ErrCollectionNotFound
	}

	// Fetch all ancestors from path
	ancestors := make([]*v1.Collection, 0, len(collection.Path)+1)
	for _, ancestorID := range collection.Path {
		ancestor, _ := s.StorageProvider.GetCollection(ctx, ancestorID)
		if ancestor != nil {
			ancestors = append(ancestors, ancestor)
		}
	}

	// Add the collection itself
	ancestors = append(ancestors, collection)

	return &v1.GetCollectionPathResponse{Ancestors: ancestors}, nil
}

// AddToCollection adds an entity to a collection.
func (s *BaseCollectionsService) AddToCollection(ctx context.Context, req *v1.AddToCollectionRequest) (*v1.AddToCollectionResponse, error) {
	// Resolve AddedBy from request or context
	req.AddedBy = s.resolveUserID(ctx, req.AddedBy)

	// Authorization hook
	if s.Hooks.OnAuthorize != nil {
		hookCtx := &HookContext{
			Operation:    "AddToCollection",
			UserID:       req.AddedBy,
			CollectionID: req.CollectionId,
			EntityID:     req.EntityId,
			Request:      req,
		}
		if err := s.Hooks.OnAuthorize(ctx, hookCtx); err != nil {
			return nil, err
		}
	}

	if req.CollectionId == "" {
		return nil, ErrCollectionIDRequired
	}
	if req.EntityId == "" {
		return nil, ErrEntityRequired
	}

	// ValidateEntity hook
	if s.Hooks.ValidateEntity != nil {
		if err := s.Hooks.ValidateEntity(ctx, req.EntityId); err != nil {
			return nil, err
		}
	}

	// Verify collection exists
	collection, err := s.StorageProvider.GetCollection(ctx, req.CollectionId)
	if err != nil {
		return nil, err
	}
	if collection == nil {
		return nil, ErrCollectionNotFound
	}

	// Check if already in collection
	existing, _ := s.StorageProvider.GetCollectionItem(ctx, req.CollectionId, req.EntityId)
	if existing != nil {
		return &v1.AddToCollectionResponse{
			Item:       existing,
			NewlyAdded: false,
		}, nil
	}

	// Determine display order
	displayOrder := req.DisplayOrder
	if displayOrder == 0 {
		// Add to end
		maxOrder, _ := s.StorageProvider.GetMaxDisplayOrder(ctx, req.CollectionId)
		displayOrder = maxOrder + 1
	}

	// Create item
	now := time.Now()
	item := &v1.CollectionItem{
		CollectionId: req.CollectionId,
		EntityId:     req.EntityId,
		DisplayOrder: displayOrder,
		AddedBy:      req.AddedBy,
		AddedAt:      timestamppb.New(now),
		Metadata:     req.Metadata,
	}

	// BeforeItemSave hook
	if s.Hooks.BeforeItemSave != nil {
		if err := s.Hooks.BeforeItemSave(ctx, item, collection); err != nil {
			return nil, err
		}
	}

	if err := s.StorageProvider.SaveCollectionItem(ctx, item); err != nil {
		return nil, err
	}

	// AfterItemSave hook (errors logged, don't fail operation)
	if s.Hooks.AfterItemSave != nil {
		_ = s.Hooks.AfterItemSave(ctx, item, collection)
	}

	// Update collection's item_count
	s.incrementItemCount(ctx, req.CollectionId, 1)

	// OnEvent hook
	if s.Hooks.OnEvent != nil {
		_ = s.Hooks.OnEvent(ctx, &Event{
			Type:         EventItemAdded,
			CollectionID: req.CollectionId,
			EntityID:     req.EntityId,
			UserID:       req.AddedBy,
			Collection:   collection,
			Item:         item,
		})
	}

	return &v1.AddToCollectionResponse{
		Item:       item,
		NewlyAdded: true,
	}, nil
}

// RemoveFromCollection removes an entity from a collection.
func (s *BaseCollectionsService) RemoveFromCollection(ctx context.Context, req *v1.RemoveFromCollectionRequest) (*v1.RemoveFromCollectionResponse, error) {
	// Authorization hook
	if s.Hooks.OnAuthorize != nil {
		hookCtx := &HookContext{
			Operation:    "RemoveFromCollection",
			UserID:       GetUserIDFromContext(ctx, s.UserIDContextKey),
			CollectionID: req.CollectionId,
			EntityID:     req.EntityId,
			Request:      req,
		}
		if err := s.Hooks.OnAuthorize(ctx, hookCtx); err != nil {
			return nil, err
		}
	}

	if req.CollectionId == "" {
		return nil, ErrCollectionIDRequired
	}
	if req.EntityId == "" {
		return nil, ErrEntityRequired
	}

	// Check if item exists
	existing, _ := s.StorageProvider.GetCollectionItem(ctx, req.CollectionId, req.EntityId)
	if existing == nil {
		return &v1.RemoveFromCollectionResponse{Removed: false}, nil
	}

	// BeforeItemDelete hook
	if s.Hooks.BeforeItemDelete != nil {
		if err := s.Hooks.BeforeItemDelete(ctx, existing); err != nil {
			return nil, err
		}
	}

	// Delete item
	if err := s.StorageProvider.DeleteCollectionItem(ctx, req.CollectionId, req.EntityId); err != nil {
		return nil, err
	}

	// AfterItemDelete hook (errors logged, don't fail operation)
	if s.Hooks.AfterItemDelete != nil {
		_ = s.Hooks.AfterItemDelete(ctx, existing)
	}

	// Update collection's item_count
	s.incrementItemCount(ctx, req.CollectionId, -1)

	// OnEvent hook
	if s.Hooks.OnEvent != nil {
		_ = s.Hooks.OnEvent(ctx, &Event{
			Type:         EventItemRemoved,
			CollectionID: req.CollectionId,
			EntityID:     req.EntityId,
			UserID:       GetUserIDFromContext(ctx, s.UserIDContextKey),
			Item:         existing,
		})
	}

	return &v1.RemoveFromCollectionResponse{Removed: true}, nil
}

// GetCollectionItems returns all items in a collection.
func (s *BaseCollectionsService) GetCollectionItems(ctx context.Context, req *v1.GetCollectionItemsRequest) (*v1.GetCollectionItemsResponse, error) {
	if req.CollectionId == "" {
		return nil, ErrCollectionIDRequired
	}

	pageSize := int(req.Pagination.GetPageSize())
	if pageSize <= 0 {
		pageSize = 50
	}

	offset := 0
	if req.Pagination.GetPageToken() != "" {
		fmt.Sscanf(req.Pagination.GetPageToken(), "%d", &offset)
	}

	opts := ListItemsOptions{
		SortBy:         req.SortBy,
		SortDescending: req.SortDescending,
		Limit:          pageSize,
		Offset:         offset,
	}

	items, total, err := s.StorageProvider.ListCollectionItems(ctx, req.CollectionId, opts)
	if err != nil {
		return nil, err
	}

	// AfterItemsRead hook
	if len(items) > 0 && s.Hooks.AfterItemsRead != nil {
		_ = s.Hooks.AfterItemsRead(ctx, items)
	}

	var nextToken string
	if offset+len(items) < total {
		nextToken = fmt.Sprintf("%d", offset+len(items))
	}

	return &v1.GetCollectionItemsResponse{
		Items: items,
		Pagination: &commonv1.PaginationResponse{
			NextPageToken: nextToken,
			TotalCount:    int32(total),
		},
	}, nil
}

// GetEntityCollections returns all collections that contain an entity.
func (s *BaseCollectionsService) GetEntityCollections(ctx context.Context, req *v1.GetEntityCollectionsRequest) (*v1.GetEntityCollectionsResponse, error) {
	if req.EntityId == "" {
		return nil, ErrEntityRequired
	}

	opts := EntityCollectionsOptions{
		OwnerID: req.OwnerId,
	}

	collections, err := s.StorageProvider.ListEntityCollections(ctx, req.EntityId, opts)
	if err != nil {
		return nil, err
	}

	// AfterCollectionsRead hook
	if len(collections) > 0 && s.Hooks.AfterCollectionsRead != nil {
		_ = s.Hooks.AfterCollectionsRead(ctx, collections)
	}

	return &v1.GetEntityCollectionsResponse{Collections: collections}, nil
}

// ReorderItems updates the display order of items in a collection.
func (s *BaseCollectionsService) ReorderItems(ctx context.Context, req *v1.ReorderItemsRequest) (*v1.ReorderItemsResponse, error) {
	if req.CollectionId == "" {
		return nil, ErrCollectionIDRequired
	}

	updated, err := s.StorageProvider.UpdateItemOrders(ctx, req.CollectionId, req.ItemOrders)
	if err != nil {
		return nil, err
	}

	return &v1.ReorderItemsResponse{ItemsUpdated: updated}, nil
}

// BatchAddToCollection adds multiple entities to a collection at once.
func (s *BaseCollectionsService) BatchAddToCollection(ctx context.Context, req *v1.BatchAddToCollectionRequest) (*v1.BatchAddToCollectionResponse, error) {
	if req.CollectionId == "" {
		return nil, ErrCollectionIDRequired
	}

	var added, alreadyExisted int64

	for _, entityId := range req.EntityIds {
		resp, err := s.AddToCollection(ctx, &v1.AddToCollectionRequest{
			CollectionId: req.CollectionId,
			EntityId:     entityId,
			AddedBy:      req.AddedBy,
		})
		if err != nil {
			continue
		}
		if resp.NewlyAdded {
			added++
		} else {
			alreadyExisted++
		}
	}

	return &v1.BatchAddToCollectionResponse{
		ItemsAdded:     added,
		AlreadyExisted: alreadyExisted,
	}, nil
}

// BatchGetEntityCollections returns collections for multiple entities at once.
func (s *BaseCollectionsService) BatchGetEntityCollections(ctx context.Context, req *v1.BatchGetEntityCollectionsRequest) (*v1.BatchGetEntityCollectionsResponse, error) {
	result := make(map[string]*v1.CollectionList)

	for _, entityId := range req.EntityIds {
		resp, err := s.GetEntityCollections(ctx, &v1.GetEntityCollectionsRequest{
			EntityId: entityId,
			OwnerId:  req.OwnerId,
		})
		if err != nil {
			continue
		}

		result[entityId] = &v1.CollectionList{Collections: resp.Collections}
	}

	return &v1.BatchGetEntityCollectionsResponse{EntityCollections: result}, nil
}

// Helper methods

func (s *BaseCollectionsService) cacheCollection(collection *v1.Collection) {
	if s.CacheEnabled && collection != nil {
		s.CacheMu.Lock()
		s.CollectionCache[collection.Id] = collection
		s.CacheMu.Unlock()
	}
}

func (s *BaseCollectionsService) getCachedCollection(id string) *v1.Collection {
	if !s.CacheEnabled {
		return nil
	}
	s.CacheMu.RLock()
	defer s.CacheMu.RUnlock()
	return s.CollectionCache[id]
}

func (s *BaseCollectionsService) invalidateCollectionCache(id string) {
	if s.CacheEnabled {
		s.CacheMu.Lock()
		delete(s.CollectionCache, id)
		s.CacheMu.Unlock()
	}
}

func (s *BaseCollectionsService) incrementItemCount(ctx context.Context, collectionID string, delta int64) {
	collection, _ := s.StorageProvider.GetCollection(ctx, collectionID)
	if collection != nil {
		collection.ItemCount += delta
		if collection.ItemCount < 0 {
			collection.ItemCount = 0
		}
		collection.UpdatedAt = timestamppb.New(time.Now())
		s.StorageProvider.SaveCollection(ctx, collection)
		s.invalidateCollectionCache(collectionID)
	}
}

func (s *BaseCollectionsService) incrementChildCount(ctx context.Context, collectionID string, delta int64) {
	collection, _ := s.StorageProvider.GetCollection(ctx, collectionID)
	if collection != nil {
		collection.ChildCount += delta
		if collection.ChildCount < 0 {
			collection.ChildCount = 0
		}
		collection.UpdatedAt = timestamppb.New(time.Now())
		s.StorageProvider.SaveCollection(ctx, collection)
		s.invalidateCollectionCache(collectionID)
	}
}

// generateID generates a unique ID for a collection.
func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// Error types
var (
	ErrCollectionIDRequired = &CollectionsError{Message: "collection id is required"}
	ErrNameRequired         = &CollectionsError{Message: "name is required"}
	ErrEntityRequired       = &CollectionsError{Message: "entity_id is required"}
	ErrCollectionNotFound   = &CollectionsError{Message: "collection not found"}
	ErrParentNotFound       = &CollectionsError{Message: "parent collection not found"}
	ErrCollectionHasChildren = &CollectionsError{Message: "collection has children, use recursive=true to delete"}
	ErrCollectionHasItems   = &CollectionsError{Message: "collection has items, use force_delete=true to delete"}
	ErrCircularReference    = &CollectionsError{Message: "cannot move collection into its own subtree"}
	ErrMaxDepthExceeded     = &CollectionsError{Message: "maximum nesting depth exceeded"}
)

// CollectionsError represents an error in the collections service.
type CollectionsError struct {
	Message string
}

func (e *CollectionsError) Error() string {
	return e.Message
}
