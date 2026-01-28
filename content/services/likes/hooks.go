package likes

import (
	"context"

	v1 "github.com/panyam/goapplib/content/gen/go/likes/v1"
	"github.com/panyam/goapplib/content/services/common"
)

// DefaultUserIDContextKey is the default context key for the authenticated user ID.
// Can be overridden per service instance via WithUserIDContextKey.
// Uses the common default so all goapplib services share the same key.
const DefaultUserIDContextKey = common.DefaultUserIDContextKey

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
// This is set by the mount middleware when the service is mounted at a path like /songs/{songId}/likes.
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
	// Operation being performed (e.g., "AddReaction", "RemoveReaction")
	Operation string
	// UserID of the user performing the operation (may be empty, hook can set it)
	UserID string
	// EntityID being operated on
	EntityID string
	// Request is the original request object (type assert to access specific fields).
	// The hook can modify request fields (e.g., set UserId from auth context).
	Request any
}

// AuthorizeHook is called before any operation to check authorization.
// The hook can:
// - Return an error to deny the operation
// - Modify the request via hookCtx.Request (e.g., set UserId from auth context)
// - Read/validate request fields
type AuthorizeHook func(ctx context.Context, hookCtx *HookContext) error

// ValidateEntityHook is called to validate that an entity exists and is valid.
// Return an error if the entity doesn't exist or is invalid.
type ValidateEntityHook func(ctx context.Context, entityID string) error

// BeforeSaveHook is called before saving a like.
// The hook can modify the like before it's saved.
// Return an error to abort the save.
type BeforeSaveHook func(ctx context.Context, like *v1.Like) error

// AfterSaveHook is called after successfully saving a like.
// Errors from this hook are logged but don't affect the response.
type AfterSaveHook func(ctx context.Context, like *v1.Like) error

// BeforeDeleteHook is called before deleting a like.
// Return an error to abort the delete.
type BeforeDeleteHook func(ctx context.Context, like *v1.Like) error

// AfterDeleteHook is called after successfully deleting a like.
// Errors from this hook are logged but don't affect the response.
type AfterDeleteHook func(ctx context.Context, like *v1.Like) error

// AfterReadHook is called after reading likes (for transformation/enrichment).
// Can modify the likes slice in place.
type AfterReadHook func(ctx context.Context, likes []*v1.Like) error

// EventType represents the type of event that occurred.
type EventType string

const (
	EventReactionAdded   EventType = "reaction.added"
	EventReactionRemoved EventType = "reaction.removed"
	EventReactionChanged EventType = "reaction.changed"
)

// Event represents a likes service event for notifications.
type Event struct {
	Type     EventType
	EntityID string
	UserID   string
	Like     *v1.Like
	OldType  string // For reaction changes
	NewType  string // For reaction changes
}

// OnEventHook is called when notable events occur (for notifications, analytics, etc.).
// Errors from this hook are logged but don't affect the response.
type OnEventHook func(ctx context.Context, event *Event) error

// Hooks contains all hook functions for the likes service.
type Hooks struct {
	// Authorization hook - called before operations
	OnAuthorize AuthorizeHook

	// Entity validation hook - validates entity exists
	ValidateEntity ValidateEntityHook

	// Lifecycle hooks
	BeforeSave   BeforeSaveHook
	AfterSave    AfterSaveHook
	BeforeDelete BeforeDeleteHook
	AfterDelete  AfterDeleteHook

	// Read hooks
	AfterRead AfterReadHook

	// Event hooks
	OnEvent OnEventHook
}

// ServiceOption is a functional option for configuring the likes service.
type ServiceOption func(*BaseLikesService)

// WithHooks sets all hooks at once.
func WithHooks(hooks Hooks) ServiceOption {
	return func(s *BaseLikesService) {
		s.Hooks = hooks
	}
}

// WithOnAuthorize sets the authorization hook.
func WithOnAuthorize(hook AuthorizeHook) ServiceOption {
	return func(s *BaseLikesService) {
		s.Hooks.OnAuthorize = hook
	}
}

// WithValidateEntity sets the entity validation hook.
func WithValidateEntity(hook ValidateEntityHook) ServiceOption {
	return func(s *BaseLikesService) {
		s.Hooks.ValidateEntity = hook
	}
}

// WithBeforeSave sets the before save hook.
func WithBeforeSave(hook BeforeSaveHook) ServiceOption {
	return func(s *BaseLikesService) {
		s.Hooks.BeforeSave = hook
	}
}

// WithAfterSave sets the after save hook.
func WithAfterSave(hook AfterSaveHook) ServiceOption {
	return func(s *BaseLikesService) {
		s.Hooks.AfterSave = hook
	}
}

// WithBeforeDelete sets the before delete hook.
func WithBeforeDelete(hook BeforeDeleteHook) ServiceOption {
	return func(s *BaseLikesService) {
		s.Hooks.BeforeDelete = hook
	}
}

// WithAfterDelete sets the after delete hook.
func WithAfterDelete(hook AfterDeleteHook) ServiceOption {
	return func(s *BaseLikesService) {
		s.Hooks.AfterDelete = hook
	}
}

// WithAfterRead sets the after read hook.
func WithAfterRead(hook AfterReadHook) ServiceOption {
	return func(s *BaseLikesService) {
		s.Hooks.AfterRead = hook
	}
}

// WithOnEvent sets the event notification hook.
func WithOnEvent(hook OnEventHook) ServiceOption {
	return func(s *BaseLikesService) {
		s.Hooks.OnEvent = hook
	}
}

// WithDefaultReactionType sets the default reaction type.
func WithDefaultReactionType(reactionType string) ServiceOption {
	return func(s *BaseLikesService) {
		s.DefaultReactionType = reactionType
	}
}

// WithUserIDContextKey sets the context key used to read user ID from context.
// This allows different apps to use their own context key conventions.
func WithUserIDContextKey(key any) ServiceOption {
	return func(s *BaseLikesService) {
		s.UserIDContextKey = key
	}
}

// WithCache enables the in-memory counts cache.
func WithCache() ServiceOption {
	return func(s *BaseLikesService) {
		s.InitializeCache()
	}
}
