package tags

import (
	"context"

	v1 "github.com/panyam/goapplib/content/gen/go/tags/v1"
	"github.com/panyam/goapplib/content/services/common"
)

// DefaultUserIDContextKey is the default context key for the authenticated user ID.
// Can be overridden per service instance via WithUserIDContextKey.
const DefaultUserIDContextKey = "tags:user_id"

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

// GetMountedEntityID retrieves the entity ID from mount context.
// This is set by the mount middleware when the service is mounted at a path like /songs/{songId}/tags.
func GetMountedEntityID(ctx context.Context) string {
	return common.GetMountedEntityID(ctx)
}

// WithMountedEntityID stores the entity ID in context.
// This is typically called by mount middleware after extracting the path parameter.
func WithMountedEntityID(ctx context.Context, entityID string) context.Context {
	return common.WithMountedEntityID(ctx, entityID)
}

// HookContext provides context information for hook invocations.
type HookContext struct {
	// Operation being performed (e.g., "CreateTag", "TagEntity")
	Operation string
	// UserID of the user performing the operation (may be empty, hook can set it)
	UserID string
	// EntityID being operated on (for entity tagging operations)
	EntityID string
	// TagID being operated on
	TagID string
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

// ValidateEntityHook is called to validate that an entity exists and is valid.
// Return an error if the entity doesn't exist or is invalid.
type ValidateEntityHook func(ctx context.Context, entityID string) error

// BeforeTagSaveHook is called before saving a tag.
// The hook can modify the tag before it's saved.
// Return an error to abort the save.
type BeforeTagSaveHook func(ctx context.Context, tag *v1.Tag) error

// AfterTagSaveHook is called after successfully saving a tag.
// Errors from this hook are logged but don't affect the response.
type AfterTagSaveHook func(ctx context.Context, tag *v1.Tag) error

// BeforeTagDeleteHook is called before deleting a tag.
// Return an error to abort the delete.
type BeforeTagDeleteHook func(ctx context.Context, tag *v1.Tag) error

// AfterTagDeleteHook is called after successfully deleting a tag.
// Errors from this hook are logged but don't affect the response.
type AfterTagDeleteHook func(ctx context.Context, tag *v1.Tag) error

// BeforeEntityTagSaveHook is called before saving an entity tag association.
// Return an error to abort the save.
type BeforeEntityTagSaveHook func(ctx context.Context, entityTag *v1.EntityTag, tag *v1.Tag) error

// AfterEntityTagSaveHook is called after successfully saving an entity tag association.
// Errors from this hook are logged but don't affect the response.
type AfterEntityTagSaveHook func(ctx context.Context, entityTag *v1.EntityTag, tag *v1.Tag) error

// BeforeEntityTagDeleteHook is called before deleting an entity tag association.
// Return an error to abort the delete.
type BeforeEntityTagDeleteHook func(ctx context.Context, entityTag *v1.EntityTag) error

// AfterEntityTagDeleteHook is called after successfully deleting an entity tag association.
// Errors from this hook are logged but don't affect the response.
type AfterEntityTagDeleteHook func(ctx context.Context, entityTag *v1.EntityTag) error

// AfterTagsReadHook is called after reading tags (for transformation/enrichment).
// Can modify the tags slice in place.
type AfterTagsReadHook func(ctx context.Context, tags []*v1.Tag) error

// EventType represents the type of event that occurred.
type EventType string

const (
	EventTagCreated       EventType = "tag.created"
	EventTagUpdated       EventType = "tag.updated"
	EventTagDeleted       EventType = "tag.deleted"
	EventEntityTagged     EventType = "entity.tagged"
	EventEntityUntagged   EventType = "entity.untagged"
	EventTagsMerged       EventType = "tags.merged"
	EventTagPromoted      EventType = "tag.promoted"
)

// Event represents a tags service event for notifications.
type Event struct {
	Type     EventType
	TagID    string
	EntityID string
	UserID   string
	Tag      *v1.Tag
	// For merge operations
	SourceTagID string
	TargetTagID string
	// Count of affected entities
	EntitiesAffected int64
}

// OnEventHook is called when notable events occur (for notifications, analytics, etc.).
// Errors from this hook are logged but don't affect the response.
type OnEventHook func(ctx context.Context, event *Event) error

// Hooks contains all hook functions for the tags service.
type Hooks struct {
	// Authorization hook - called before operations
	OnAuthorize AuthorizeHook

	// Entity validation hook - validates entity exists
	ValidateEntity ValidateEntityHook

	// Tag lifecycle hooks
	BeforeTagSave   BeforeTagSaveHook
	AfterTagSave    AfterTagSaveHook
	BeforeTagDelete BeforeTagDeleteHook
	AfterTagDelete  AfterTagDeleteHook

	// EntityTag lifecycle hooks
	BeforeEntityTagSave   BeforeEntityTagSaveHook
	AfterEntityTagSave    AfterEntityTagSaveHook
	BeforeEntityTagDelete BeforeEntityTagDeleteHook
	AfterEntityTagDelete  AfterEntityTagDeleteHook

	// Read hooks
	AfterTagsRead AfterTagsReadHook

	// Event hooks
	OnEvent OnEventHook
}

// ServiceOption is a functional option for configuring the tags service.
type ServiceOption func(*BaseTagsService)

// WithHooks sets all hooks at once.
func WithHooks(hooks Hooks) ServiceOption {
	return func(s *BaseTagsService) {
		s.Hooks = hooks
	}
}

// WithOnAuthorize sets the authorization hook.
func WithOnAuthorize(hook AuthorizeHook) ServiceOption {
	return func(s *BaseTagsService) {
		s.Hooks.OnAuthorize = hook
	}
}

// WithValidateEntity sets the entity validation hook.
func WithValidateEntity(hook ValidateEntityHook) ServiceOption {
	return func(s *BaseTagsService) {
		s.Hooks.ValidateEntity = hook
	}
}

// WithBeforeTagSave sets the before tag save hook.
func WithBeforeTagSave(hook BeforeTagSaveHook) ServiceOption {
	return func(s *BaseTagsService) {
		s.Hooks.BeforeTagSave = hook
	}
}

// WithAfterTagSave sets the after tag save hook.
func WithAfterTagSave(hook AfterTagSaveHook) ServiceOption {
	return func(s *BaseTagsService) {
		s.Hooks.AfterTagSave = hook
	}
}

// WithBeforeTagDelete sets the before tag delete hook.
func WithBeforeTagDelete(hook BeforeTagDeleteHook) ServiceOption {
	return func(s *BaseTagsService) {
		s.Hooks.BeforeTagDelete = hook
	}
}

// WithAfterTagDelete sets the after tag delete hook.
func WithAfterTagDelete(hook AfterTagDeleteHook) ServiceOption {
	return func(s *BaseTagsService) {
		s.Hooks.AfterTagDelete = hook
	}
}

// WithBeforeEntityTagSave sets the before entity tag save hook.
func WithBeforeEntityTagSave(hook BeforeEntityTagSaveHook) ServiceOption {
	return func(s *BaseTagsService) {
		s.Hooks.BeforeEntityTagSave = hook
	}
}

// WithAfterEntityTagSave sets the after entity tag save hook.
func WithAfterEntityTagSave(hook AfterEntityTagSaveHook) ServiceOption {
	return func(s *BaseTagsService) {
		s.Hooks.AfterEntityTagSave = hook
	}
}

// WithBeforeEntityTagDelete sets the before entity tag delete hook.
func WithBeforeEntityTagDelete(hook BeforeEntityTagDeleteHook) ServiceOption {
	return func(s *BaseTagsService) {
		s.Hooks.BeforeEntityTagDelete = hook
	}
}

// WithAfterEntityTagDelete sets the after entity tag delete hook.
func WithAfterEntityTagDelete(hook AfterEntityTagDeleteHook) ServiceOption {
	return func(s *BaseTagsService) {
		s.Hooks.AfterEntityTagDelete = hook
	}
}

// WithAfterTagsRead sets the after tags read hook.
func WithAfterTagsRead(hook AfterTagsReadHook) ServiceOption {
	return func(s *BaseTagsService) {
		s.Hooks.AfterTagsRead = hook
	}
}

// WithOnEvent sets the event notification hook.
func WithOnEvent(hook OnEventHook) ServiceOption {
	return func(s *BaseTagsService) {
		s.Hooks.OnEvent = hook
	}
}

// WithNormalizer sets a custom normalizer function.
func WithNormalizer(normalizer func(string) string) ServiceOption {
	return func(s *BaseTagsService) {
		s.Normalizer = normalizer
	}
}

// WithUserIDContextKey sets the context key used to read user ID from context.
// This allows different apps to use their own context key conventions.
func WithUserIDContextKey(key any) ServiceOption {
	return func(s *BaseTagsService) {
		s.UserIDContextKey = key
	}
}

// WithCache enables the in-memory tag cache.
func WithCache() ServiceOption {
	return func(s *BaseTagsService) {
		s.InitializeCache()
	}
}
