package collections

import (
	"context"

	v1 "github.com/panyam/goapplib/content/gen/go/collections/v1"
)

// DefaultUserIDContextKey is the default context key for the authenticated user ID.
// Can be overridden per service instance via WithUserIDContextKey.
const DefaultUserIDContextKey = "collections:user_id"

// GetUserIDFromContext returns the user ID from context using the given key.
// Returns empty string if not set.
func GetUserIDFromContext(ctx context.Context, key any) string {
	if key == nil {
		key = DefaultUserIDContextKey
	}
	if v := ctx.Value(key); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// WithUserID returns a new context with the user ID set.
// Typically called by interceptors/middleware.
func WithUserID(ctx context.Context, key any, userID string) context.Context {
	if key == nil {
		key = DefaultUserIDContextKey
	}
	return context.WithValue(ctx, key, userID)
}

// HookContext provides context information for hook invocations.
type HookContext struct {
	// Operation being performed (e.g., "CreateCollection", "AddToCollection")
	Operation string
	// UserID of the user performing the operation (may be empty, hook can set it)
	UserID string
	// CollectionID being operated on
	CollectionID string
	// EntityID being operated on (for item operations)
	EntityID string
	// Request is the original request object (type assert to access specific fields).
	// The hook can modify request fields (e.g., set OwnerId from auth context).
	Request any
}

// AuthorizeHook is called before any operation to check authorization.
// The hook can:
// - Return an error to deny the operation
// - Modify the request via hookCtx.Request (e.g., set OwnerId from auth context)
// - Read/validate request fields
type AuthorizeHook func(ctx context.Context, hookCtx *HookContext) error

// ValidateEntityHook is called to validate that an entity exists and is valid
// before adding it to a collection.
// Return an error if the entity doesn't exist or is invalid.
type ValidateEntityHook func(ctx context.Context, entityID string) error

// BeforeCollectionSaveHook is called before saving a collection.
// The hook can modify the collection before it's saved.
// Return an error to abort the save.
type BeforeCollectionSaveHook func(ctx context.Context, collection *v1.Collection) error

// AfterCollectionSaveHook is called after successfully saving a collection.
// Errors from this hook are logged but don't affect the response.
type AfterCollectionSaveHook func(ctx context.Context, collection *v1.Collection) error

// BeforeCollectionDeleteHook is called before deleting a collection.
// Return an error to abort the delete.
type BeforeCollectionDeleteHook func(ctx context.Context, collection *v1.Collection) error

// AfterCollectionDeleteHook is called after successfully deleting a collection.
// Errors from this hook are logged but don't affect the response.
type AfterCollectionDeleteHook func(ctx context.Context, collection *v1.Collection) error

// BeforeItemSaveHook is called before saving a collection item.
// Return an error to abort the save.
type BeforeItemSaveHook func(ctx context.Context, item *v1.CollectionItem, collection *v1.Collection) error

// AfterItemSaveHook is called after successfully saving a collection item.
// Errors from this hook are logged but don't affect the response.
type AfterItemSaveHook func(ctx context.Context, item *v1.CollectionItem, collection *v1.Collection) error

// BeforeItemDeleteHook is called before deleting a collection item.
// Return an error to abort the delete.
type BeforeItemDeleteHook func(ctx context.Context, item *v1.CollectionItem) error

// AfterItemDeleteHook is called after successfully deleting a collection item.
// Errors from this hook are logged but don't affect the response.
type AfterItemDeleteHook func(ctx context.Context, item *v1.CollectionItem) error

// AfterCollectionsReadHook is called after reading collections (for transformation/enrichment).
// Can modify the collections slice in place.
type AfterCollectionsReadHook func(ctx context.Context, collections []*v1.Collection) error

// AfterItemsReadHook is called after reading collection items (for transformation/enrichment).
// Can modify the items slice in place.
type AfterItemsReadHook func(ctx context.Context, items []*v1.CollectionItem) error

// EventType represents the type of event that occurred.
type EventType string

const (
	EventCollectionCreated EventType = "collection.created"
	EventCollectionUpdated EventType = "collection.updated"
	EventCollectionDeleted EventType = "collection.deleted"
	EventCollectionMoved   EventType = "collection.moved"
	EventItemAdded         EventType = "item.added"
	EventItemRemoved       EventType = "item.removed"
)

// Event represents a collections service event for notifications.
type Event struct {
	Type         EventType
	CollectionID string
	EntityID     string
	UserID       string
	Collection   *v1.Collection
	Item         *v1.CollectionItem
	// For move operations
	OldParentID string
	NewParentID string
	// Count of affected items/children
	ChildrenAffected int64
	ItemsAffected    int64
}

// OnEventHook is called when notable events occur (for notifications, analytics, etc.).
// Errors from this hook are logged but don't affect the response.
type OnEventHook func(ctx context.Context, event *Event) error

// Hooks contains all hook functions for the collections service.
type Hooks struct {
	// Authorization hook - called before operations
	OnAuthorize AuthorizeHook

	// Entity validation hook - validates entity exists before adding to collection
	ValidateEntity ValidateEntityHook

	// Collection lifecycle hooks
	BeforeCollectionSave   BeforeCollectionSaveHook
	AfterCollectionSave    AfterCollectionSaveHook
	BeforeCollectionDelete BeforeCollectionDeleteHook
	AfterCollectionDelete  AfterCollectionDeleteHook

	// Item lifecycle hooks
	BeforeItemSave   BeforeItemSaveHook
	AfterItemSave    AfterItemSaveHook
	BeforeItemDelete BeforeItemDeleteHook
	AfterItemDelete  AfterItemDeleteHook

	// Read hooks
	AfterCollectionsRead AfterCollectionsReadHook
	AfterItemsRead       AfterItemsReadHook

	// Event hooks
	OnEvent OnEventHook
}

// ServiceOption is a functional option for configuring the collections service.
type ServiceOption func(*BaseCollectionsService)

// WithHooks sets all hooks at once.
func WithHooks(hooks Hooks) ServiceOption {
	return func(s *BaseCollectionsService) {
		s.Hooks = hooks
	}
}

// WithOnAuthorize sets the authorization hook.
func WithOnAuthorize(hook AuthorizeHook) ServiceOption {
	return func(s *BaseCollectionsService) {
		s.Hooks.OnAuthorize = hook
	}
}

// WithValidateEntity sets the entity validation hook.
func WithValidateEntity(hook ValidateEntityHook) ServiceOption {
	return func(s *BaseCollectionsService) {
		s.Hooks.ValidateEntity = hook
	}
}

// WithBeforeCollectionSave sets the before collection save hook.
func WithBeforeCollectionSave(hook BeforeCollectionSaveHook) ServiceOption {
	return func(s *BaseCollectionsService) {
		s.Hooks.BeforeCollectionSave = hook
	}
}

// WithAfterCollectionSave sets the after collection save hook.
func WithAfterCollectionSave(hook AfterCollectionSaveHook) ServiceOption {
	return func(s *BaseCollectionsService) {
		s.Hooks.AfterCollectionSave = hook
	}
}

// WithBeforeCollectionDelete sets the before collection delete hook.
func WithBeforeCollectionDelete(hook BeforeCollectionDeleteHook) ServiceOption {
	return func(s *BaseCollectionsService) {
		s.Hooks.BeforeCollectionDelete = hook
	}
}

// WithAfterCollectionDelete sets the after collection delete hook.
func WithAfterCollectionDelete(hook AfterCollectionDeleteHook) ServiceOption {
	return func(s *BaseCollectionsService) {
		s.Hooks.AfterCollectionDelete = hook
	}
}

// WithBeforeItemSave sets the before item save hook.
func WithBeforeItemSave(hook BeforeItemSaveHook) ServiceOption {
	return func(s *BaseCollectionsService) {
		s.Hooks.BeforeItemSave = hook
	}
}

// WithAfterItemSave sets the after item save hook.
func WithAfterItemSave(hook AfterItemSaveHook) ServiceOption {
	return func(s *BaseCollectionsService) {
		s.Hooks.AfterItemSave = hook
	}
}

// WithBeforeItemDelete sets the before item delete hook.
func WithBeforeItemDelete(hook BeforeItemDeleteHook) ServiceOption {
	return func(s *BaseCollectionsService) {
		s.Hooks.BeforeItemDelete = hook
	}
}

// WithAfterItemDelete sets the after item delete hook.
func WithAfterItemDelete(hook AfterItemDeleteHook) ServiceOption {
	return func(s *BaseCollectionsService) {
		s.Hooks.AfterItemDelete = hook
	}
}

// WithAfterCollectionsRead sets the after collections read hook.
func WithAfterCollectionsRead(hook AfterCollectionsReadHook) ServiceOption {
	return func(s *BaseCollectionsService) {
		s.Hooks.AfterCollectionsRead = hook
	}
}

// WithAfterItemsRead sets the after items read hook.
func WithAfterItemsRead(hook AfterItemsReadHook) ServiceOption {
	return func(s *BaseCollectionsService) {
		s.Hooks.AfterItemsRead = hook
	}
}

// WithOnEvent sets the event notification hook.
func WithOnEvent(hook OnEventHook) ServiceOption {
	return func(s *BaseCollectionsService) {
		s.Hooks.OnEvent = hook
	}
}

// WithNormalizer sets a custom normalizer function.
func WithNormalizer(normalizer func(string) string) ServiceOption {
	return func(s *BaseCollectionsService) {
		s.Normalizer = normalizer
	}
}

// WithMaxDepth sets the maximum allowed nesting depth.
func WithMaxDepth(maxDepth int32) ServiceOption {
	return func(s *BaseCollectionsService) {
		s.MaxDepth = maxDepth
	}
}

// WithUserIDContextKey sets the context key used to read user ID from context.
// This allows different apps to use their own context key conventions.
func WithUserIDContextKey(key any) ServiceOption {
	return func(s *BaseCollectionsService) {
		s.UserIDContextKey = key
	}
}

// WithCache enables the in-memory collections cache.
func WithCache() ServiceOption {
	return func(s *BaseCollectionsService) {
		s.InitializeCache()
	}
}
