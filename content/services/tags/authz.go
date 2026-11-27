package tags

import (
	"context"

	v1 "github.com/panyam/goapplib/content/gen/go/tags/v1"
	"github.com/panyam/goapplib/content/services/common"
)

// authorize fills the caller fields of hc and runs the OnAuthorize hook.
// Every RPC calls it before doing any work.
func (s *BaseTagsService) authorize(ctx context.Context, hc *HookContext) error {
	hc.CallerID = GetUserIDFromContext(ctx, s.UserIDContextKey)
	if hc.UserID == "" {
		hc.UserID = hc.CallerID
	}
	if s.Hooks.OnAuthorize == nil {
		return nil
	}
	return s.Hooks.OnAuthorize(ctx, hc)
}

// loadForAuth loads a tag for the hook context. It skips the read when no
// hook is set, and returns nil for an empty id or a missing tag.
func (s *BaseTagsService) loadForAuth(ctx context.Context, id string) *v1.Tag {
	if id == "" || s.Hooks.OnAuthorize == nil {
		return nil
	}
	t, _ := s.StorageProvider.GetTag(ctx, id)
	return t
}

// resolveActingUser applies common.ResolveActingUser with this service's
// context key.
func (s *BaseTagsService) resolveActingUser(ctx context.Context, requestUserID string) (string, error) {
	return common.ResolveActingUser(ctx, GetUserIDFromContext(ctx, s.UserIDContextKey), requestUserID)
}

// resolveActingUsers resolves each request user field in place with
// common.ResolveActingUser, and stops at the first error.
func (s *BaseTagsService) resolveActingUsers(ctx context.Context, fields ...*string) error {
	for _, f := range fields {
		u, err := s.resolveActingUser(ctx, *f)
		if err != nil {
			return err
		}
		*f = u
	}
	return nil
}

// OwnerOnly returns an AuthorizeHook for user-owned tags.
//
// It checks client calls only (see common.WithClientCall). In-process calls
// pass, since server code does its own checks. For a client call:
//
//   - Every operation except GetTag needs a logged-in caller.
//   - GetTag passes for the tag's owner, and for anyone when the tag is
//     PUBLIC. Other reads get NotFound, so private tags don't leak.
//   - UpdateTag and DeleteTag need the caller to own the tag. MergeTags needs
//     the caller to own both tags.
//   - TagEntity and BatchTagEntities may apply the caller's own tags and
//     PUBLIC tags. Someone else's private tag gets NotFound.
//   - ListTags with no owner_id lists the caller's tags. With another owner's
//     id it lists only their PUBLIC tags.
//   - GetEntityTags, BatchGetEntityTags, SearchTags, GetPopularTags, and
//     GetEntitiesWithTag by name and value are scoped to the caller. Another
//     owner's id gets PermissionDenied. GetEntitiesWithTag by tag_id follows
//     the GetTag rule.
//   - PromoteTag publishes a tag under another owner, so it is an admin
//     operation and gets PermissionDenied. So does any operation it doesn't
//     know.
func OwnerOnly() AuthorizeHook {
	return func(ctx context.Context, hc *HookContext) error {
		if !common.IsClientCall(ctx) {
			return nil
		}
		caller := hc.CallerID
		owns := func(t *v1.Tag) bool { return t != nil && caller != "" && t.OwnerId == caller }
		canRead := func(t *v1.Tag) bool {
			return owns(t) || (t != nil && t.Scope == v1.TagScope_TAG_SCOPE_PUBLIC)
		}
		write := func(t *v1.Tag) error {
			switch {
			case owns(t):
				return nil
			case canRead(t):
				return common.PermissionDenied("not the tag owner")
			default:
				return common.NotFound("tag not found")
			}
		}
		scopeToCaller := func(owner *string) error {
			if *owner == "" {
				*owner = caller
			} else if *owner != caller {
				return common.PermissionDenied("can only read your own tags")
			}
			return nil
		}

		if hc.Operation == "GetTag" {
			if !canRead(hc.Tag) {
				return common.NotFound("tag not found")
			}
			return nil
		}
		if caller == "" {
			return common.ErrUnauthenticated
		}

		switch req := hc.Request.(type) {
		case *v1.CreateTagRequest, *v1.UntagEntityRequest:
			// The service already made the caller the owner or tagger.
		case *v1.UpdateTagRequest, *v1.DeleteTagRequest:
			return write(hc.Tag)
		case *v1.MergeTagsRequest:
			if err := write(hc.Tag); err != nil {
				return err
			}
			return write(hc.TargetTag)
		case *v1.TagEntityRequest:
			if req.TagId != "" && !canRead(hc.Tag) {
				return common.NotFound("tag not found")
			}
		case *v1.BatchTagEntitiesRequest:
			if req.TagId != "" && !canRead(hc.Tag) {
				return common.NotFound("tag not found")
			}
		case *v1.ListTagsRequest:
			if req.OwnerId == "" {
				req.OwnerId = caller
			} else if req.OwnerId != caller {
				req.Scope = v1.TagScope_TAG_SCOPE_PUBLIC
			}
		case *v1.GetEntityTagsRequest:
			return scopeToCaller(&req.OwnerId)
		case *v1.BatchGetEntityTagsRequest:
			return scopeToCaller(&req.OwnerId)
		case *v1.SearchTagsRequest:
			return scopeToCaller(&req.OwnerId)
		case *v1.GetPopularTagsRequest:
			return scopeToCaller(&req.OwnerId)
		case *v1.GetEntitiesWithTagRequest:
			if req.TagId != "" {
				if !canRead(hc.Tag) {
					return common.NotFound("tag not found")
				}
				return nil
			}
			return scopeToCaller(&req.OwnerId)
		default:
			return common.PermissionDenied("operation not allowed: " + hc.Operation)
		}
		return nil
	}
}
