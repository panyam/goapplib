// Package collections provides Connect RPC adapters for the CollectionsService.
package collections

import (
	"context"

	"connectrpc.com/connect"
	v1 "github.com/panyam/goapplib/content/gen/go/collections/v1"
	"github.com/panyam/goapplib/content/gen/go/collections/v1/collectionsv1connect"
)

// ConnectCollectionsServer wraps CollectionsService for Connect RPC.
// Use with collectionsv1connect.NewCollectionsServiceHandler().
type ConnectCollectionsServer struct {
	Service CollectionsService
}

// Compile-time check that ConnectCollectionsServer implements CollectionsServiceHandler.
var _ collectionsv1connect.CollectionsServiceHandler = (*ConnectCollectionsServer)(nil)

// NewConnectCollectionsServer creates a new Connect RPC server wrapping the given CollectionsService.
func NewConnectCollectionsServer(svc CollectionsService) *ConnectCollectionsServer {
	return &ConnectCollectionsServer{Service: svc}
}

// CreateCollection creates a new collection.
func (s *ConnectCollectionsServer) CreateCollection(ctx context.Context, req *connect.Request[v1.CreateCollectionRequest]) (*connect.Response[v1.CreateCollectionResponse], error) {
	resp, err := s.Service.CreateCollection(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// GetCollection retrieves a collection by ID.
func (s *ConnectCollectionsServer) GetCollection(ctx context.Context, req *connect.Request[v1.GetCollectionRequest]) (*connect.Response[v1.GetCollectionResponse], error) {
	resp, err := s.Service.GetCollection(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// UpdateCollection updates a collection's properties.
func (s *ConnectCollectionsServer) UpdateCollection(ctx context.Context, req *connect.Request[v1.UpdateCollectionRequest]) (*connect.Response[v1.UpdateCollectionResponse], error) {
	resp, err := s.Service.UpdateCollection(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// DeleteCollection removes a collection.
func (s *ConnectCollectionsServer) DeleteCollection(ctx context.Context, req *connect.Request[v1.DeleteCollectionRequest]) (*connect.Response[v1.DeleteCollectionResponse], error) {
	resp, err := s.Service.DeleteCollection(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ListCollections lists collections for an owner with filtering.
func (s *ConnectCollectionsServer) ListCollections(ctx context.Context, req *connect.Request[v1.ListCollectionsRequest]) (*connect.Response[v1.ListCollectionsResponse], error) {
	resp, err := s.Service.ListCollections(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// GetCollectionTree returns a collection and its children as a tree structure.
func (s *ConnectCollectionsServer) GetCollectionTree(ctx context.Context, req *connect.Request[v1.GetCollectionTreeRequest]) (*connect.Response[v1.GetCollectionTreeResponse], error) {
	resp, err := s.Service.GetCollectionTree(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// MoveCollection moves a collection to a new parent.
func (s *ConnectCollectionsServer) MoveCollection(ctx context.Context, req *connect.Request[v1.MoveCollectionRequest]) (*connect.Response[v1.MoveCollectionResponse], error) {
	resp, err := s.Service.MoveCollection(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// GetCollectionPath returns the ancestors of a collection (breadcrumb).
func (s *ConnectCollectionsServer) GetCollectionPath(ctx context.Context, req *connect.Request[v1.GetCollectionPathRequest]) (*connect.Response[v1.GetCollectionPathResponse], error) {
	resp, err := s.Service.GetCollectionPath(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// AddToCollection adds an entity to a collection.
func (s *ConnectCollectionsServer) AddToCollection(ctx context.Context, req *connect.Request[v1.AddToCollectionRequest]) (*connect.Response[v1.AddToCollectionResponse], error) {
	resp, err := s.Service.AddToCollection(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// RemoveFromCollection removes an entity from a collection.
func (s *ConnectCollectionsServer) RemoveFromCollection(ctx context.Context, req *connect.Request[v1.RemoveFromCollectionRequest]) (*connect.Response[v1.RemoveFromCollectionResponse], error) {
	resp, err := s.Service.RemoveFromCollection(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// GetCollectionItems returns all items in a collection.
func (s *ConnectCollectionsServer) GetCollectionItems(ctx context.Context, req *connect.Request[v1.GetCollectionItemsRequest]) (*connect.Response[v1.GetCollectionItemsResponse], error) {
	resp, err := s.Service.GetCollectionItems(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// GetEntityCollections returns all collections that contain an entity.
func (s *ConnectCollectionsServer) GetEntityCollections(ctx context.Context, req *connect.Request[v1.GetEntityCollectionsRequest]) (*connect.Response[v1.GetEntityCollectionsResponse], error) {
	resp, err := s.Service.GetEntityCollections(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ReorderItems updates the display order of items in a collection.
func (s *ConnectCollectionsServer) ReorderItems(ctx context.Context, req *connect.Request[v1.ReorderItemsRequest]) (*connect.Response[v1.ReorderItemsResponse], error) {
	resp, err := s.Service.ReorderItems(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// BatchAddToCollection adds multiple entities to a collection at once.
func (s *ConnectCollectionsServer) BatchAddToCollection(ctx context.Context, req *connect.Request[v1.BatchAddToCollectionRequest]) (*connect.Response[v1.BatchAddToCollectionResponse], error) {
	resp, err := s.Service.BatchAddToCollection(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// BatchGetEntityCollections returns collections for multiple entities at once.
func (s *ConnectCollectionsServer) BatchGetEntityCollections(ctx context.Context, req *connect.Request[v1.BatchGetEntityCollectionsRequest]) (*connect.Response[v1.BatchGetEntityCollectionsResponse], error) {
	resp, err := s.Service.BatchGetEntityCollections(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}
