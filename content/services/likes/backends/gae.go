// Package backends provides concrete storage implementations for the likes service.
package backends

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/datastore"
	dsidx "github.com/panyam/goapplib/datastore"
	"github.com/panyam/goapplib/content/services/likes"

	v1 "github.com/panyam/goapplib/content/gen/go/likes/v1"
	dsgen "github.com/panyam/goapplib/content/gen/datastore/likes/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// DatastoreLikesService implements LikesService using Google Cloud Datastore.
type DatastoreLikesService struct {
	*likes.BaseLikesService
	client    *datastore.Client
	namespace string
}

// NewDatastoreLikesService creates a new Datastore-backed likes service.
// Options:
//   - dsidx.WithValidation(ctx): Validate indexes exist, return error with deployment instructions if not
func NewDatastoreLikesService(client *datastore.Client, namespace string, options ...dsidx.ServiceOption) (*DatastoreLikesService, error) {
	provider := &datastoreLikesStorageProvider{
		client:    client,
		namespace: namespace,
	}
	base := likes.NewBaseLikesService(provider)
	service := &DatastoreLikesService{
		BaseLikesService: base,
		client:           client,
		namespace:        namespace,
	}

	// Apply options
	opts := dsidx.ApplyOptions(options...)
	if opts.ValidateCtx != nil {
		if err := dsidx.ValidateAndWriteIndexes(opts.ValidateCtx, client, namespace, service); err != nil {
			return nil, err
		}
	}

	return service, nil
}

// datastoreLikesStorageProvider implements LikesStorageProvider using Datastore.
type datastoreLikesStorageProvider struct {
	client    *datastore.Client
	namespace string
}

const (
	likeKind         = "Like"
	likeCountsKind   = "LikeCounts"
	reactionTypeKind = "ReactionType"
)

func (p *datastoreLikesStorageProvider) newKey(kind, id string) *datastore.Key {
	key := datastore.NameKey(kind, id, nil)
	if p.namespace != "" {
		key.Namespace = p.namespace
	}
	return key
}

// SaveLike saves a like to Datastore.
func (p *datastoreLikesStorageProvider) SaveLike(ctx context.Context, like *v1.Like) error {
	dsLike := likeToDatastore(like)
	key := p.newKey(likeKind, like.Id)
	_, err := p.client.Put(ctx, key, dsLike)
	return err
}

// DeleteLike deletes a like from Datastore.
func (p *datastoreLikesStorageProvider) DeleteLike(ctx context.Context, entityType, entityID, userID string) error {
	// Find the like first
	like, err := p.GetLike(ctx, entityType, entityID, userID)
	if err != nil || like == nil {
		return err
	}

	key := p.newKey(likeKind, like.Id)
	return p.client.Delete(ctx, key)
}

// GetLike retrieves a like from Datastore.
func (p *datastoreLikesStorageProvider) GetLike(ctx context.Context, entityType, entityID, userID string) (*v1.Like, error) {
	query := datastore.NewQuery(likeKind).
		FilterField("entity_type", "=", entityType).
		FilterField("entity_id", "=", entityID).
		FilterField("user_id", "=", userID).
		Limit(1)

	if p.namespace != "" {
		query = query.Namespace(p.namespace)
	}

	var dsLikes []dsgen.LikeDatastore
	keys, err := p.client.GetAll(ctx, query, &dsLikes)
	if err != nil {
		return nil, err
	}

	if len(dsLikes) == 0 {
		return nil, nil
	}

	// Populate Id from the key since it's not persisted in the entity
	dsLikes[0].Id = keys[0].Name

	return likeFromDatastore(&dsLikes[0]), nil
}

// ListLikesByEntity lists likes for an entity.
func (p *datastoreLikesStorageProvider) ListLikesByEntity(ctx context.Context, entityType, entityID string, reactionType string, limit, offset int) ([]*v1.Like, int, error) {
	query := datastore.NewQuery(likeKind).
		FilterField("entity_type", "=", entityType).
		FilterField("entity_id", "=", entityID).
		Order("-created_at")

	if p.namespace != "" {
		query = query.Namespace(p.namespace)
	}

	if reactionType != "" {
		query = query.FilterField("reaction_type", "=", reactionType)
	}

	// Get total count
	countQuery := query.KeysOnly()
	keys, err := p.client.GetAll(ctx, countQuery, nil)
	if err != nil {
		return nil, 0, err
	}
	total := len(keys)

	// Get paginated results
	query = query.Offset(offset).Limit(limit)
	var dsLikes []dsgen.LikeDatastore
	resultKeys, err := p.client.GetAll(ctx, query, &dsLikes)
	if err != nil {
		return nil, 0, err
	}

	likes := make([]*v1.Like, len(dsLikes))
	for i := range dsLikes {
		// Populate Id from the key since it's not persisted in the entity
		dsLikes[i].Id = resultKeys[i].Name
		likes[i] = likeFromDatastore(&dsLikes[i])
	}

	return likes, total, nil
}

// ListLikesByUser lists likes by a user.
func (p *datastoreLikesStorageProvider) ListLikesByUser(ctx context.Context, userID string, entityType string, limit, offset int) ([]*v1.Like, int, error) {
	query := datastore.NewQuery(likeKind).
		FilterField("user_id", "=", userID).
		Order("-created_at")

	if p.namespace != "" {
		query = query.Namespace(p.namespace)
	}

	if entityType != "" {
		query = query.FilterField("entity_type", "=", entityType)
	}

	// Get total count
	countQuery := query.KeysOnly()
	keys, err := p.client.GetAll(ctx, countQuery, nil)
	if err != nil {
		return nil, 0, err
	}
	total := len(keys)

	// Get paginated results
	query = query.Offset(offset).Limit(limit)
	var dsLikes []dsgen.LikeDatastore
	_, err = p.client.GetAll(ctx, query, &dsLikes)
	if err != nil {
		return nil, 0, err
	}

	likes := make([]*v1.Like, len(dsLikes))
	for i, dl := range dsLikes {
		likes[i] = likeFromDatastore(&dl)
	}

	return likes, total, nil
}

// GetLikeCounts retrieves like counts for an entity.
func (p *datastoreLikesStorageProvider) GetLikeCounts(ctx context.Context, entityType, entityID string) (*v1.LikeCounts, error) {
	key := p.newKey(likeCountsKind, likeCountsKey(entityType, entityID))

	var dsCounts dsgen.LikeCountsDatastore
	err := p.client.Get(ctx, key, &dsCounts)
	if err != nil {
		if err == datastore.ErrNoSuchEntity {
			return nil, nil
		}
		return nil, err
	}

	return likeCountsFromDatastore(&dsCounts), nil
}

// SaveLikeCounts saves like counts to Datastore.
func (p *datastoreLikesStorageProvider) SaveLikeCounts(ctx context.Context, counts *v1.LikeCounts) error {
	dsCounts := likeCountsToDatastore(counts)
	key := p.newKey(likeCountsKind, likeCountsKey(counts.EntityType, counts.EntityId))
	_, err := p.client.Put(ctx, key, dsCounts)
	return err
}

// SaveReactionType saves a reaction type to Datastore.
func (p *datastoreLikesStorageProvider) SaveReactionType(ctx context.Context, rt *v1.ReactionType) error {
	dsRT := reactionTypeToDatastore(rt)
	key := p.newKey(reactionTypeKind, rt.Id)
	_, err := p.client.Put(ctx, key, dsRT)
	return err
}

// GetReactionType retrieves a reaction type from Datastore.
func (p *datastoreLikesStorageProvider) GetReactionType(ctx context.Context, id string) (*v1.ReactionType, error) {
	key := p.newKey(reactionTypeKind, id)

	var dsRT dsgen.ReactionTypeDatastore
	err := p.client.Get(ctx, key, &dsRT)
	if err != nil {
		if err == datastore.ErrNoSuchEntity {
			return nil, nil
		}
		return nil, err
	}

	return reactionTypeFromDatastore(&dsRT), nil
}

// ListReactionTypes lists all reaction types.
func (p *datastoreLikesStorageProvider) ListReactionTypes(ctx context.Context) ([]*v1.ReactionType, error) {
	query := datastore.NewQuery(reactionTypeKind).
		Order("display_order")

	if p.namespace != "" {
		query = query.Namespace(p.namespace)
	}

	var dsTypes []dsgen.ReactionTypeDatastore
	_, err := p.client.GetAll(ctx, query, &dsTypes)
	if err != nil {
		return nil, err
	}

	types := make([]*v1.ReactionType, len(dsTypes))
	for i, dt := range dsTypes {
		types[i] = reactionTypeFromDatastore(&dt)
	}

	return types, nil
}

// Helper functions

func likeCountsKey(entityType, entityID string) string {
	return fmt.Sprintf("%s:%s", entityType, entityID)
}

// Conversion functions for Datastore

func likeToDatastore(like *v1.Like) *dsgen.LikeDatastore {
	var createdAt, updatedAt time.Time
	if like.CreatedAt != nil {
		createdAt = like.CreatedAt.AsTime()
	}
	if like.UpdatedAt != nil {
		updatedAt = like.UpdatedAt.AsTime()
	}
	return &dsgen.LikeDatastore{
		Id:           like.Id,
		EntityType:   like.EntityType,
		EntityId:     like.EntityId,
		UserId:       like.UserId,
		ReactionType: like.ReactionType,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
		CreatorId:    like.CreatorId,
	}
}

func likeFromDatastore(dsLike *dsgen.LikeDatastore) *v1.Like {
	var createdAt, updatedAt *timestamppb.Timestamp
	if !dsLike.CreatedAt.IsZero() {
		createdAt = timestamppb.New(dsLike.CreatedAt)
	}
	if !dsLike.UpdatedAt.IsZero() {
		updatedAt = timestamppb.New(dsLike.UpdatedAt)
	}
	return &v1.Like{
		Id:           dsLike.Id,
		EntityType:   dsLike.EntityType,
		EntityId:     dsLike.EntityId,
		UserId:       dsLike.UserId,
		ReactionType: dsLike.ReactionType,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
		CreatorId:    dsLike.CreatorId,
	}
}

func likeCountsToDatastore(counts *v1.LikeCounts) *dsgen.LikeCountsDatastore {
	var updatedAt time.Time
	if counts.UpdatedAt != nil {
		updatedAt = counts.UpdatedAt.AsTime()
	}
	return &dsgen.LikeCountsDatastore{
		EntityType:     counts.EntityType,
		EntityId:       counts.EntityId,
		TotalCount:     counts.TotalCount,
		ByReactionType: counts.ByReactionType,
		UpdatedAt:      updatedAt,
	}
}

func likeCountsFromDatastore(dsCounts *dsgen.LikeCountsDatastore) *v1.LikeCounts {
	byType := dsCounts.ByReactionType
	if byType == nil {
		byType = make(map[string]int64)
	}
	var updatedAt *timestamppb.Timestamp
	if !dsCounts.UpdatedAt.IsZero() {
		updatedAt = timestamppb.New(dsCounts.UpdatedAt)
	}
	return &v1.LikeCounts{
		EntityType:     dsCounts.EntityType,
		EntityId:       dsCounts.EntityId,
		TotalCount:     dsCounts.TotalCount,
		ByReactionType: byType,
		UpdatedAt:      updatedAt,
	}
}

func reactionTypeToDatastore(rt *v1.ReactionType) *dsgen.ReactionTypeDatastore {
	var createdAt, updatedAt time.Time
	if rt.CreatedAt != nil {
		createdAt = rt.CreatedAt.AsTime()
	}
	if rt.UpdatedAt != nil {
		updatedAt = rt.UpdatedAt.AsTime()
	}
	return &dsgen.ReactionTypeDatastore{
		Id:           rt.Id,
		Name:         rt.Name,
		Emoji:        rt.Emoji,
		IconUrl:      rt.IconUrl,
		DisplayOrder: rt.DisplayOrder,
		IsDefault:    rt.IsDefault,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
		CreatorId:    rt.CreatorId,
	}
}

func reactionTypeFromDatastore(dsRT *dsgen.ReactionTypeDatastore) *v1.ReactionType {
	var createdAt, updatedAt *timestamppb.Timestamp
	if !dsRT.CreatedAt.IsZero() {
		createdAt = timestamppb.New(dsRT.CreatedAt)
	}
	if !dsRT.UpdatedAt.IsZero() {
		updatedAt = timestamppb.New(dsRT.UpdatedAt)
	}
	return &v1.ReactionType{
		Id:           dsRT.Id,
		Name:         dsRT.Name,
		Emoji:        dsRT.Emoji,
		IconUrl:      dsRT.IconUrl,
		DisplayOrder: dsRT.DisplayOrder,
		IsDefault:    dsRT.IsDefault,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
		CreatorId:    dsRT.CreatorId,
	}
}

// IndexProvider implementation for DatastoreLikesService

// ServiceName returns the service name for index file naming.
func (s *DatastoreLikesService) ServiceName() string {
	return "likes"
}

// RequiredIndexes returns the composite indexes required by the likes service.
func (s *DatastoreLikesService) RequiredIndexes() []dsidx.DatastoreIndex {
	return []dsidx.DatastoreIndex{
		// For GetLike - find a user's reaction on an entity
		{
			Kind: likeKind,
			Properties: []dsidx.IndexProperty{
				{Name: "entity_type"},
				{Name: "entity_id"},
				{Name: "user_id"},
			},
		},
		// For ListLikesByEntity - list all reactions on an entity
		{
			Kind: likeKind,
			Properties: []dsidx.IndexProperty{
				{Name: "entity_type"},
				{Name: "entity_id"},
				{Name: "created_at", Direction: "desc"},
			},
		},
		// For ListLikesByEntity with reaction type filter
		{
			Kind: likeKind,
			Properties: []dsidx.IndexProperty{
				{Name: "entity_type"},
				{Name: "entity_id"},
				{Name: "reaction_type"},
				{Name: "created_at", Direction: "desc"},
			},
		},
		// For ListLikesByUser - list all reactions by a user
		{
			Kind: likeKind,
			Properties: []dsidx.IndexProperty{
				{Name: "user_id"},
				{Name: "created_at", Direction: "desc"},
			},
		},
		// For ListLikesByUser with entity type filter
		{
			Kind: likeKind,
			Properties: []dsidx.IndexProperty{
				{Name: "user_id"},
				{Name: "entity_type"},
				{Name: "created_at", Direction: "desc"},
			},
		},
	}
}

// TestQueries returns queries that exercise each required index.
func (s *DatastoreLikesService) TestQueries() []*datastore.Query {
	return []*datastore.Query{
		// GetLike
		datastore.NewQuery(likeKind).
			FilterField("entity_type", "=", "__test__").
			FilterField("entity_id", "=", "__test__").
			FilterField("user_id", "=", "__test__"),

		// ListLikesByEntity
		datastore.NewQuery(likeKind).
			FilterField("entity_type", "=", "__test__").
			FilterField("entity_id", "=", "__test__").
			Order("-created_at"),

		// ListLikesByEntity with reaction type
		datastore.NewQuery(likeKind).
			FilterField("entity_type", "=", "__test__").
			FilterField("entity_id", "=", "__test__").
			FilterField("reaction_type", "=", "__test__").
			Order("-created_at"),

		// ListLikesByUser
		datastore.NewQuery(likeKind).
			FilterField("user_id", "=", "__test__").
			Order("-created_at"),

		// ListLikesByUser with entity type
		datastore.NewQuery(likeKind).
			FilterField("user_id", "=", "__test__").
			FilterField("entity_type", "=", "__test__").
			Order("-created_at"),
	}
}

// ValidateIndexes checks if all required indexes exist.
func (s *DatastoreLikesService) ValidateIndexes(ctx context.Context) error {
	return dsidx.ValidateIndexes(ctx, s.client, s.namespace, s)
}

// WriteIndexFile writes the required indexes to a YAML file.
func (s *DatastoreLikesService) WriteIndexFile(path string) error {
	return dsidx.WriteIndexFile(path, s.ServiceName(), s.RequiredIndexes())
}

// IndexesYAML returns the indexes as a YAML string.
func (s *DatastoreLikesService) IndexesYAML() string {
	return dsidx.IndexesToYAML(s.ServiceName(), s.RequiredIndexes())
}
