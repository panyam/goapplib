package collections

import (
	"context"

	v1 "github.com/panyam/goapplib/content/gen/go/collections/v1"
	"github.com/panyam/goapplib/content/services/common"
)

// authorize fills the caller fields of hc and runs the OnAuthorize hook.
// Every RPC calls it before doing any work.
func (s *BaseCollectionsService) authorize(ctx context.Context, hc *HookContext) error {
	hc.CallerID = GetUserIDFromContext(ctx, s.UserIDContextKey)
	if hc.UserID == "" {
		hc.UserID = hc.CallerID
	}
	if s.Hooks.OnAuthorize == nil {
		return nil
	}
	return s.Hooks.OnAuthorize(ctx, hc)
}

// loadForAuth loads a collection for the hook context. It skips the read when
// no hook is set, and returns nil for an empty id or a missing collection.
func (s *BaseCollectionsService) loadForAuth(ctx context.Context, id string) *v1.Collection {
	if id == "" || s.Hooks.OnAuthorize == nil {
		return nil
	}
	c, _ := s.StorageProvider.GetCollection(ctx, id)
	return c
}

// resolveActingUser applies common.ResolveActingUser with this service's
// context key.
func (s *BaseCollectionsService) resolveActingUser(ctx context.Context, requestUserID string) (string, error) {
	return common.ResolveActingUser(ctx, GetUserIDFromContext(ctx, s.UserIDContextKey), requestUserID)
}

// resolveActingUsers resolves each request user field in place with
// common.ResolveActingUser, and stops at the first error.
func (s *BaseCollectionsService) resolveActingUsers(ctx context.Context, fields ...*string) error {
	for _, f := range fields {
		u, err := s.resolveActingUser(ctx, *f)
		if err != nil {
			return err
		}
		*f = u
	}
	return nil
}

// OwnerOnly returns an AuthorizeHook for user-owned collections.
//
// It checks client calls only (see common.WithClientCall). In-process calls
// pass, since server code does its own checks. For a client call:
//
//   - Writes need a logged-in caller who owns the collection. A caller who can
//     read but not write gets PermissionDenied. Creating or moving under a
//     parent needs the caller to own the parent too.
//   - Reads of one collection (Get, Path, Items, Tree) pass for the owner, and
//     for anyone when the collection is PUBLIC. Other reads get NotFound, so
//     private collections don't leak.
//   - ListCollections with no owner_id lists the caller's own collections.
//     With another owner's id it lists only their PUBLIC collections.
//   - GetEntityCollections and BatchGetEntityCollections are scoped to the
//     caller. Another owner's id gets PermissionDenied.
//   - GetCollectionTree with no id gets PermissionDenied, since it would list
//     every owner's root collections.
//   - Any operation it doesn't know gets PermissionDenied.
func OwnerOnly() AuthorizeHook {
	return func(ctx context.Context, hc *HookContext) error {
		if !common.IsClientCall(ctx) {
			return nil
		}
		caller := hc.CallerID
		owns := func(c *v1.Collection) bool { return c != nil && caller != "" && c.OwnerId == caller }
		canRead := func(c *v1.Collection) bool {
			return owns(c) || (c != nil && c.Visibility == v1.CollectionVisibility_COLLECTION_VISIBILITY_PUBLIC)
		}
		write := func(c *v1.Collection) error {
			switch {
			case caller == "":
				return common.ErrUnauthenticated
			case owns(c):
				return nil
			case canRead(c):
				return common.PermissionDenied("not the collection owner")
			default:
				return common.NotFound("collection not found")
			}
		}

		switch hc.Operation {
		case "GetCollection", "GetCollectionPath", "GetCollectionItems":
			if !canRead(hc.Collection) {
				return common.NotFound("collection not found")
			}
		case "GetCollectionTree":
			if req, ok := hc.Request.(*v1.GetCollectionTreeRequest); ok && req.Id == "" {
				return common.PermissionDenied("a collection tree needs a collection id")
			}
			if !canRead(hc.Collection) {
				return common.NotFound("collection not found")
			}
		case "CreateCollection":
			if caller == "" {
				return common.ErrUnauthenticated
			}
			if req, ok := hc.Request.(*v1.CreateCollectionRequest); ok && req.ParentId != "" {
				return write(hc.Parent)
			}
		case "UpdateCollection", "DeleteCollection", "AddToCollection",
			"RemoveFromCollection", "ReorderItems", "BatchAddToCollection":
			return write(hc.Collection)
		case "MoveCollection":
			if err := write(hc.Collection); err != nil {
				return err
			}
			if req, ok := hc.Request.(*v1.MoveCollectionRequest); ok && req.NewParentId != "" {
				return write(hc.Parent)
			}
		case "ListCollections":
			req, _ := hc.Request.(*v1.ListCollectionsRequest)
			if req == nil {
				return common.PermissionDenied("unexpected request")
			}
			switch {
			case req.OwnerId == "" && caller == "":
				return common.ErrUnauthenticated
			case req.OwnerId == "":
				req.OwnerId = caller
			case req.OwnerId != caller:
				req.Visibility = v1.CollectionVisibility_COLLECTION_VISIBILITY_PUBLIC
			}
		case "GetEntityCollections", "BatchGetEntityCollections":
			if caller == "" {
				return common.ErrUnauthenticated
			}
			var owner *string
			switch req := hc.Request.(type) {
			case *v1.GetEntityCollectionsRequest:
				owner = &req.OwnerId
			case *v1.BatchGetEntityCollectionsRequest:
				owner = &req.OwnerId
			default:
				return common.PermissionDenied("unexpected request")
			}
			if *owner == "" {
				*owner = caller
			} else if *owner != caller {
				return common.PermissionDenied("can only list your own collections")
			}
		default:
			return common.PermissionDenied("operation not allowed: " + hc.Operation)
		}
		return nil
	}
}
