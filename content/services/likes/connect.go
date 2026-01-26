// Package likes provides Connect RPC adapters for the LikesService.
package likes

import (
	"context"

	"connectrpc.com/connect"
	v1 "github.com/panyam/goapplib/content/gen/go/likes/v1"
	"github.com/panyam/goapplib/content/gen/go/likes/v1/likesv1connect"
)

// ConnectLikesServer wraps LikesService for Connect RPC.
// Use with likesv1connect.NewLikesServiceHandler().
type ConnectLikesServer struct {
	Service LikesService
}

// Compile-time check that ConnectLikesServer implements LikesServiceHandler.
var _ likesv1connect.LikesServiceHandler = (*ConnectLikesServer)(nil)

// NewConnectLikesServer creates a new Connect RPC server wrapping the given LikesService.
func NewConnectLikesServer(svc LikesService) *ConnectLikesServer {
	return &ConnectLikesServer{Service: svc}
}

// AddReaction adds or updates a user's reaction to an entity.
func (s *ConnectLikesServer) AddReaction(ctx context.Context, req *connect.Request[v1.AddReactionRequest]) (*connect.Response[v1.AddReactionResponse], error) {
	resp, err := s.Service.AddReaction(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// RemoveReaction removes a user's reaction from an entity.
func (s *ConnectLikesServer) RemoveReaction(ctx context.Context, req *connect.Request[v1.RemoveReactionRequest]) (*connect.Response[v1.RemoveReactionResponse], error) {
	resp, err := s.Service.RemoveReaction(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ToggleReaction toggles a reaction on/off.
func (s *ConnectLikesServer) ToggleReaction(ctx context.Context, req *connect.Request[v1.ToggleReactionRequest]) (*connect.Response[v1.ToggleReactionResponse], error) {
	resp, err := s.Service.ToggleReaction(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// GetUserReaction returns a user's current reaction on an entity.
func (s *ConnectLikesServer) GetUserReaction(ctx context.Context, req *connect.Request[v1.GetUserReactionRequest]) (*connect.Response[v1.GetUserReactionResponse], error) {
	resp, err := s.Service.GetUserReaction(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// GetLikeCounts returns aggregated reaction counts for an entity.
func (s *ConnectLikesServer) GetLikeCounts(ctx context.Context, req *connect.Request[v1.GetLikeCountsRequest]) (*connect.Response[v1.GetLikeCountsResponse], error) {
	resp, err := s.Service.GetLikeCounts(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ListReactors returns users who reacted to an entity.
func (s *ConnectLikesServer) ListReactors(ctx context.Context, req *connect.Request[v1.ListReactorsRequest]) (*connect.Response[v1.ListReactorsResponse], error) {
	resp, err := s.Service.ListReactors(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ListUserReactions returns all reactions by a specific user.
func (s *ConnectLikesServer) ListUserReactions(ctx context.Context, req *connect.Request[v1.ListUserReactionsRequest]) (*connect.Response[v1.ListUserReactionsResponse], error) {
	resp, err := s.Service.ListUserReactions(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// BatchGetUserReactions returns a user's reactions for multiple entities.
func (s *ConnectLikesServer) BatchGetUserReactions(ctx context.Context, req *connect.Request[v1.BatchGetUserReactionsRequest]) (*connect.Response[v1.BatchGetUserReactionsResponse], error) {
	resp, err := s.Service.BatchGetUserReactions(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// BatchGetLikeCounts returns reaction counts for multiple entities.
func (s *ConnectLikesServer) BatchGetLikeCounts(ctx context.Context, req *connect.Request[v1.BatchGetLikeCountsRequest]) (*connect.Response[v1.BatchGetLikeCountsResponse], error) {
	resp, err := s.Service.BatchGetLikeCounts(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// CreateReactionType creates a new reaction type (admin operation).
func (s *ConnectLikesServer) CreateReactionType(ctx context.Context, req *connect.Request[v1.CreateReactionTypeRequest]) (*connect.Response[v1.CreateReactionTypeResponse], error) {
	resp, err := s.Service.CreateReactionType(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ListReactionTypes returns all available reaction types.
func (s *ConnectLikesServer) ListReactionTypes(ctx context.Context, req *connect.Request[v1.ListReactionTypesRequest]) (*connect.Response[v1.ListReactionTypesResponse], error) {
	resp, err := s.Service.ListReactionTypes(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}
