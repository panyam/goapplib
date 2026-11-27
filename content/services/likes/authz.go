package likes

import (
	"context"

	v1 "github.com/panyam/goapplib/content/gen/go/likes/v1"
	"github.com/panyam/goapplib/content/services/common"
)

// authorize fills the caller fields of hc and runs the OnAuthorize hook.
// Every RPC calls it before doing any work.
func (s *BaseLikesService) authorize(ctx context.Context, hc *HookContext) error {
	hc.CallerID = GetUserIDFromContext(ctx, s.UserIDContextKey)
	if hc.UserID == "" {
		hc.UserID = hc.CallerID
	}
	if s.Hooks.OnAuthorize == nil {
		return nil
	}
	return s.Hooks.OnAuthorize(ctx, hc)
}

// resolveActingUser applies common.ResolveActingUser with this service's
// context key.
func (s *BaseLikesService) resolveActingUser(ctx context.Context, requestUserID string) (string, error) {
	return common.ResolveActingUser(ctx, GetUserIDFromContext(ctx, s.UserIDContextKey), requestUserID)
}

// OwnerOnly returns an AuthorizeHook for per-user reactions.
//
// It checks client calls only (see common.WithClientCall). In-process calls
// pass, since server code does its own checks. For a client call:
//
//   - AddReaction, RemoveReaction and ToggleReaction need a logged-in caller.
//     The service already makes the caller the reacting user.
//   - GetUserReaction, ListUserReactions and BatchGetUserReactions read the
//     caller's own reactions. Another user's id gets PermissionDenied.
//   - GetLikeCounts, BatchGetLikeCounts, ListReactors and ListReactionTypes
//     are public, and pass for anonymous callers too.
//   - CreateReactionType is an admin operation and gets PermissionDenied. So
//     does any operation it doesn't know.
func OwnerOnly() AuthorizeHook {
	return func(ctx context.Context, hc *HookContext) error {
		if !common.IsClientCall(ctx) {
			return nil
		}
		caller := hc.CallerID
		ownReactions := func(user *string) error {
			switch {
			case caller == "":
				return common.ErrUnauthenticated
			case *user == "":
				*user = caller
			case *user != caller:
				return common.PermissionDenied("can only read your own reactions")
			}
			return nil
		}

		switch req := hc.Request.(type) {
		case *v1.GetLikeCountsRequest, *v1.BatchGetLikeCountsRequest,
			*v1.ListReactorsRequest, *v1.ListReactionTypesRequest:
			return nil
		case *v1.AddReactionRequest, *v1.RemoveReactionRequest, *v1.ToggleReactionRequest:
			if caller == "" {
				return common.ErrUnauthenticated
			}
		case *v1.GetUserReactionRequest:
			return ownReactions(&req.UserId)
		case *v1.ListUserReactionsRequest:
			return ownReactions(&req.UserId)
		case *v1.BatchGetUserReactionsRequest:
			return ownReactions(&req.UserId)
		default:
			return common.PermissionDenied("operation not allowed: " + hc.Operation)
		}
		return nil
	}
}
