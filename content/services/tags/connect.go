// Package tags provides Connect RPC adapters for the TagsService.
package tags

import (
	"context"

	"connectrpc.com/connect"
	v1 "github.com/panyam/goapplib/content/gen/go/tags/v1"
	"github.com/panyam/goapplib/content/gen/go/tags/v1/tagsv1connect"
)

// ConnectTagsServer wraps TagsService for Connect RPC.
// Use with tagsv1connect.NewTagsServiceHandler().
type ConnectTagsServer struct {
	Service TagsService
}

// Compile-time check that ConnectTagsServer implements TagsServiceHandler.
var _ tagsv1connect.TagsServiceHandler = (*ConnectTagsServer)(nil)

// NewConnectTagsServer creates a new Connect RPC server wrapping the given TagsService.
func NewConnectTagsServer(svc TagsService) *ConnectTagsServer {
	return &ConnectTagsServer{Service: svc}
}

// CreateTag creates a new tag definition.
func (s *ConnectTagsServer) CreateTag(ctx context.Context, req *connect.Request[v1.CreateTagRequest]) (*connect.Response[v1.CreateTagResponse], error) {
	resp, err := s.Service.CreateTag(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// GetTag retrieves a tag by ID.
func (s *ConnectTagsServer) GetTag(ctx context.Context, req *connect.Request[v1.GetTagRequest]) (*connect.Response[v1.GetTagResponse], error) {
	resp, err := s.Service.GetTag(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// UpdateTag updates a tag's properties.
func (s *ConnectTagsServer) UpdateTag(ctx context.Context, req *connect.Request[v1.UpdateTagRequest]) (*connect.Response[v1.UpdateTagResponse], error) {
	resp, err := s.Service.UpdateTag(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// DeleteTag removes a tag and optionally untags all entities.
func (s *ConnectTagsServer) DeleteTag(ctx context.Context, req *connect.Request[v1.DeleteTagRequest]) (*connect.Response[v1.DeleteTagResponse], error) {
	resp, err := s.Service.DeleteTag(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ListTags lists tags for an owner with filtering.
func (s *ConnectTagsServer) ListTags(ctx context.Context, req *connect.Request[v1.ListTagsRequest]) (*connect.Response[v1.ListTagsResponse], error) {
	resp, err := s.Service.ListTags(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// TagEntity applies a tag to an entity.
func (s *ConnectTagsServer) TagEntity(ctx context.Context, req *connect.Request[v1.TagEntityRequest]) (*connect.Response[v1.TagEntityResponse], error) {
	resp, err := s.Service.TagEntity(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// UntagEntity removes a tag from an entity.
func (s *ConnectTagsServer) UntagEntity(ctx context.Context, req *connect.Request[v1.UntagEntityRequest]) (*connect.Response[v1.UntagEntityResponse], error) {
	resp, err := s.Service.UntagEntity(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// GetEntityTags returns all tags for a specific entity.
func (s *ConnectTagsServer) GetEntityTags(ctx context.Context, req *connect.Request[v1.GetEntityTagsRequest]) (*connect.Response[v1.GetEntityTagsResponse], error) {
	resp, err := s.Service.GetEntityTags(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// GetEntitiesWithTag returns all entities that have a specific tag.
func (s *ConnectTagsServer) GetEntitiesWithTag(ctx context.Context, req *connect.Request[v1.GetEntitiesWithTagRequest]) (*connect.Response[v1.GetEntitiesWithTagResponse], error) {
	resp, err := s.Service.GetEntitiesWithTag(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// BatchTagEntities applies a tag to multiple entities at once.
func (s *ConnectTagsServer) BatchTagEntities(ctx context.Context, req *connect.Request[v1.BatchTagEntitiesRequest]) (*connect.Response[v1.BatchTagEntitiesResponse], error) {
	resp, err := s.Service.BatchTagEntities(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// BatchGetEntityTags returns tags for multiple entities at once.
func (s *ConnectTagsServer) BatchGetEntityTags(ctx context.Context, req *connect.Request[v1.BatchGetEntityTagsRequest]) (*connect.Response[v1.BatchGetEntityTagsResponse], error) {
	resp, err := s.Service.BatchGetEntityTags(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// SearchTags searches for tags by name (autocomplete).
func (s *ConnectTagsServer) SearchTags(ctx context.Context, req *connect.Request[v1.SearchTagsRequest]) (*connect.Response[v1.SearchTagsResponse], error) {
	resp, err := s.Service.SearchTags(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// GetPopularTags returns the most used tags.
func (s *ConnectTagsServer) GetPopularTags(ctx context.Context, req *connect.Request[v1.GetPopularTagsRequest]) (*connect.Response[v1.GetPopularTagsResponse], error) {
	resp, err := s.Service.GetPopularTags(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// MergeTags merges two tags into one (admin operation).
func (s *ConnectTagsServer) MergeTags(ctx context.Context, req *connect.Request[v1.MergeTagsRequest]) (*connect.Response[v1.MergeTagsResponse], error) {
	resp, err := s.Service.MergeTags(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// PromoteTag promotes a private tag to public.
func (s *ConnectTagsServer) PromoteTag(ctx context.Context, req *connect.Request[v1.PromoteTagRequest]) (*connect.Response[v1.PromoteTagResponse], error) {
	resp, err := s.Service.PromoteTag(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}
