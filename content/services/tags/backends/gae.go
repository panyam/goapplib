// Package backends provides concrete storage implementations for the tags service.
package backends

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/datastore"
	dsidx "github.com/panyam/goapplib/datastore"
	"github.com/panyam/goapplib/content/services/tags"

	v1 "github.com/panyam/goapplib/content/gen/go/tags/v1"
	dsgen "github.com/panyam/goapplib/content/gen/datastore/tags/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// DatastoreTagsService implements TagsService using Google Cloud Datastore.
type DatastoreTagsService struct {
	*tags.BaseTagsService
	client    *datastore.Client
	namespace string
}

// NewDatastoreTagsService creates a new Datastore-backed tags service.
func NewDatastoreTagsService(client *datastore.Client, namespace string) *DatastoreTagsService {
	provider := &datastoreTagsStorageProvider{
		client:    client,
		namespace: namespace,
	}
	base := tags.NewBaseTagsService(provider)
	return &DatastoreTagsService{
		BaseTagsService: base,
		client:          client,
		namespace:       namespace,
	}
}

// datastoreTagsStorageProvider implements TagsStorageProvider using Datastore.
type datastoreTagsStorageProvider struct {
	client    *datastore.Client
	namespace string
}

const (
	tagKind            = "Tag"
	entityTagKind      = "EntityTag"
	tagUsageCountsKind = "TagUsageCounts"
)

func (p *datastoreTagsStorageProvider) newKey(kind, id string) *datastore.Key {
	key := datastore.NameKey(kind, id, nil)
	if p.namespace != "" {
		key.Namespace = p.namespace
	}
	return key
}

// SaveTag saves a tag to Datastore.
func (p *datastoreTagsStorageProvider) SaveTag(ctx context.Context, tag *v1.Tag) error {
	dsTag := tagToDatastore(tag)
	key := p.newKey(tagKind, tag.Id)
	_, err := p.client.Put(ctx, key, dsTag)
	return err
}

// GetTag retrieves a tag from Datastore.
func (p *datastoreTagsStorageProvider) GetTag(ctx context.Context, id string) (*v1.Tag, error) {
	key := p.newKey(tagKind, id)
	var dsTag dsgen.TagDatastore
	err := p.client.Get(ctx, key, &dsTag)
	if err != nil {
		if err == datastore.ErrNoSuchEntity {
			return nil, nil
		}
		return nil, err
	}
	dsTag.Id = id // ID is stored in key, not as property
	return tagFromDatastore(&dsTag), nil
}

// DeleteTag deletes a tag from Datastore.
func (p *datastoreTagsStorageProvider) DeleteTag(ctx context.Context, id string) error {
	key := p.newKey(tagKind, id)
	return p.client.Delete(ctx, key)
}

// ListTags lists tags with filtering.
func (p *datastoreTagsStorageProvider) ListTags(ctx context.Context, opts tags.ListTagsOptions) ([]*v1.Tag, int, error) {
	query := datastore.NewQuery(tagKind)

	if p.namespace != "" {
		query = query.Namespace(p.namespace)
	}

	// Apply filters
	if opts.OwnerType != "" {
		query = query.FilterField("owner_type", "=", opts.OwnerType)
	}
	if opts.OwnerID != "" {
		query = query.FilterField("owner_id", "=", opts.OwnerID)
	}
	if opts.Scope != v1.TagScope_TAG_SCOPE_UNSPECIFIED {
		query = query.FilterField("scope", "=", int32(opts.Scope))
	}
	if opts.NameFilter != "" && opts.NameFilter != "*" {
		query = query.FilterField("normalized_name", "=", opts.NameFilter)
	}

	// Filter to active tags only
	query = query.FilterField("status", "=", int32(v1.TagStatus_TAG_STATUS_ACTIVE))

	// Get total count (Datastore doesn't support COUNT directly, so we use KeysOnly)
	countQuery := query.KeysOnly()
	keys, err := p.client.GetAll(ctx, countQuery, nil)
	if err != nil {
		return nil, 0, err
	}
	total := len(keys)

	// Apply ordering
	switch opts.OrderBy {
	case "usage_count":
		query = query.Order("-usage_count")
	case "created_at":
		query = query.Order("-created_at")
	case "display_order":
		query = query.Order("display_order")
	case "name":
		query = query.Order("normalized_name")
	default:
		query = query.Order("-created_at")
	}

	// Apply pagination
	query = query.Offset(opts.Offset).Limit(opts.Limit)

	var dsTags []dsgen.TagDatastore
	resultKeys, err := p.client.GetAll(ctx, query, &dsTags)
	if err != nil {
		return nil, 0, err
	}

	result := make([]*v1.Tag, len(dsTags))
	for i := range dsTags {
		dsTags[i].Id = resultKeys[i].Name
		result[i] = tagFromDatastore(&dsTags[i])
	}

	return result, total, nil
}

// FindTagByNormalizedValues finds a tag by its normalized name and value.
func (p *datastoreTagsStorageProvider) FindTagByNormalizedValues(ctx context.Context, ownerType, ownerID, normalizedName, normalizedValue string) (*v1.Tag, error) {
	query := datastore.NewQuery(tagKind).
		FilterField("owner_type", "=", ownerType).
		FilterField("owner_id", "=", ownerID).
		FilterField("normalized_name", "=", normalizedName).
		FilterField("normalized_value", "=", normalizedValue).
		FilterField("status", "=", int32(v1.TagStatus_TAG_STATUS_ACTIVE)).
		Limit(1)

	if p.namespace != "" {
		query = query.Namespace(p.namespace)
	}

	var dsTags []dsgen.TagDatastore
	keys, err := p.client.GetAll(ctx, query, &dsTags)
	if err != nil {
		return nil, err
	}

	if len(dsTags) == 0 {
		return nil, nil
	}

	dsTags[0].Id = keys[0].Name
	return tagFromDatastore(&dsTags[0]), nil
}

// SearchTags searches for tags by query (prefix match).
func (p *datastoreTagsStorageProvider) SearchTags(ctx context.Context, opts tags.SearchTagsOptions) ([]*v1.Tag, error) {
	// Datastore doesn't support LIKE queries, so we use range queries for prefix matching
	query := datastore.NewQuery(tagKind).
		FilterField("status", "=", int32(v1.TagStatus_TAG_STATUS_ACTIVE))

	if p.namespace != "" {
		query = query.Namespace(p.namespace)
	}

	// For prefix matching, we use >= and < with the prefix
	if opts.Query != "" {
		query = query.FilterField("normalized_value", ">=", opts.Query)
		// Add upper bound for prefix search
		upperBound := opts.Query + "\uffff"
		query = query.FilterField("normalized_value", "<", upperBound)
	}

	// Apply name filter
	if opts.NameFilter != "" && opts.NameFilter != "*" {
		query = query.FilterField("normalized_name", "=", opts.NameFilter)
	}

	// Owner filter - Note: Datastore OR queries are complex, simplified here
	if opts.OwnerType != "" && opts.OwnerID != "" && !opts.IncludeShared {
		query = query.FilterField("owner_type", "=", opts.OwnerType)
		query = query.FilterField("owner_id", "=", opts.OwnerID)
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	query = query.Order("-usage_count").Limit(limit)

	var dsTags []dsgen.TagDatastore
	keys, err := p.client.GetAll(ctx, query, &dsTags)
	if err != nil {
		return nil, err
	}

	result := make([]*v1.Tag, len(dsTags))
	for i := range dsTags {
		dsTags[i].Id = keys[i].Name
		result[i] = tagFromDatastore(&dsTags[i])
	}

	return result, nil
}

// GetPopularTags returns the most used tags.
func (p *datastoreTagsStorageProvider) GetPopularTags(ctx context.Context, opts tags.PopularTagsOptions) ([]*v1.Tag, error) {
	query := datastore.NewQuery(tagKind).
		FilterField("status", "=", int32(v1.TagStatus_TAG_STATUS_ACTIVE)).
		FilterField("usage_count", ">", 0)

	if p.namespace != "" {
		query = query.Namespace(p.namespace)
	}

	// Apply name filter
	if opts.NameFilter != "" && opts.NameFilter != "*" {
		query = query.FilterField("normalized_name", "=", opts.NameFilter)
	}

	// Owner filter
	if opts.OwnerType != "" && opts.OwnerID != "" && !opts.IncludeShared {
		query = query.FilterField("owner_type", "=", opts.OwnerType)
		query = query.FilterField("owner_id", "=", opts.OwnerID)
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	query = query.Order("-usage_count").Limit(limit)

	var dsTags []dsgen.TagDatastore
	keys, err := p.client.GetAll(ctx, query, &dsTags)
	if err != nil {
		return nil, err
	}

	result := make([]*v1.Tag, len(dsTags))
	for i := range dsTags {
		dsTags[i].Id = keys[i].Name
		result[i] = tagFromDatastore(&dsTags[i])
	}

	return result, nil
}

// SaveEntityTag saves an entity tag to Datastore.
func (p *datastoreTagsStorageProvider) SaveEntityTag(ctx context.Context, entityTag *v1.EntityTag) error {
	dsET := entityTagToDatastore(entityTag)
	// Composite key: tag_id + entity_type + entity_id + tagged_by
	keyStr := entityTagKey(entityTag.TagId, entityTag.EntityType, entityTag.EntityId, entityTag.TaggedBy)
	key := p.newKey(entityTagKind, keyStr)
	_, err := p.client.Put(ctx, key, dsET)
	return err
}

// DeleteEntityTag deletes an entity tag from Datastore.
func (p *datastoreTagsStorageProvider) DeleteEntityTag(ctx context.Context, tagID, entityType, entityID, taggedBy string) error {
	keyStr := entityTagKey(tagID, entityType, entityID, taggedBy)
	key := p.newKey(entityTagKind, keyStr)
	return p.client.Delete(ctx, key)
}

// GetEntityTag retrieves an entity tag.
func (p *datastoreTagsStorageProvider) GetEntityTag(ctx context.Context, tagID, entityType, entityID, taggedBy string) (*v1.EntityTag, error) {
	keyStr := entityTagKey(tagID, entityType, entityID, taggedBy)
	key := p.newKey(entityTagKind, keyStr)

	var dsET dsgen.EntityTagDatastore
	err := p.client.Get(ctx, key, &dsET)
	if err != nil {
		if err == datastore.ErrNoSuchEntity {
			return nil, nil
		}
		return nil, err
	}

	return entityTagFromDatastore(&dsET), nil
}

// ListEntityTagsByEntity lists entity tags for an entity.
func (p *datastoreTagsStorageProvider) ListEntityTagsByEntity(ctx context.Context, entityType, entityID string, opts tags.EntityTagsOptions) ([]*v1.EntityTag, error) {
	query := datastore.NewQuery(entityTagKind).
		FilterField("entity_type", "=", entityType).
		FilterField("entity_id", "=", entityID)

	if p.namespace != "" {
		query = query.Namespace(p.namespace)
	}

	// Note: Filtering by tag properties requires additional queries/joins
	// For simplicity, we fetch all and filter in memory if needed

	query = query.Order("-created_at")

	var dsETs []dsgen.EntityTagDatastore
	_, err := p.client.GetAll(ctx, query, &dsETs)
	if err != nil {
		return nil, err
	}

	// Filter by tag properties if needed
	result := make([]*v1.EntityTag, 0, len(dsETs))
	for i := range dsETs {
		et := entityTagFromDatastore(&dsETs[i])

		// If filtering by owner/name, we need to check the tag
		if opts.OwnerType != "" || opts.OwnerID != "" || opts.NameFilter != "" {
			tag, _ := p.GetTag(ctx, et.TagId)
			if tag == nil {
				continue
			}
			if opts.OwnerType != "" && tag.OwnerType != opts.OwnerType {
				continue
			}
			if opts.OwnerID != "" && tag.OwnerId != opts.OwnerID {
				continue
			}
			if opts.NameFilter != "" && opts.NameFilter != "*" && tag.NormalizedName != opts.NameFilter {
				continue
			}
		}

		result = append(result, et)
	}

	return result, nil
}

// ListEntityTagsByTag lists entity tags for a tag.
func (p *datastoreTagsStorageProvider) ListEntityTagsByTag(ctx context.Context, tagID string, entityTypeFilter string, limit, offset int) ([]*v1.EntityTag, int, error) {
	query := datastore.NewQuery(entityTagKind).
		FilterField("tag_id", "=", tagID)

	if p.namespace != "" {
		query = query.Namespace(p.namespace)
	}

	if entityTypeFilter != "" {
		query = query.FilterField("entity_type", "=", entityTypeFilter)
	}

	// Get total count
	countQuery := query.KeysOnly()
	keys, err := p.client.GetAll(ctx, countQuery, nil)
	if err != nil {
		return nil, 0, err
	}
	total := len(keys)

	// Apply pagination
	query = query.Offset(offset).Limit(limit).Order("-created_at")

	var dsETs []dsgen.EntityTagDatastore
	_, err = p.client.GetAll(ctx, query, &dsETs)
	if err != nil {
		return nil, 0, err
	}

	result := make([]*v1.EntityTag, len(dsETs))
	for i := range dsETs {
		result[i] = entityTagFromDatastore(&dsETs[i])
	}

	return result, total, nil
}

// DeleteEntityTagsByTag deletes all entity tags for a tag.
func (p *datastoreTagsStorageProvider) DeleteEntityTagsByTag(ctx context.Context, tagID string) (int64, error) {
	query := datastore.NewQuery(entityTagKind).
		FilterField("tag_id", "=", tagID).
		KeysOnly()

	if p.namespace != "" {
		query = query.Namespace(p.namespace)
	}

	keys, err := p.client.GetAll(ctx, query, nil)
	if err != nil {
		return 0, err
	}

	if len(keys) == 0 {
		return 0, nil
	}

	err = p.client.DeleteMulti(ctx, keys)
	if err != nil {
		return 0, err
	}

	return int64(len(keys)), nil
}

// GetTagUsageCounts retrieves usage counts for an entity.
func (p *datastoreTagsStorageProvider) GetTagUsageCounts(ctx context.Context, entityType, entityID string) (*v1.TagUsageCounts, error) {
	keyStr := fmt.Sprintf("%s:%s", entityType, entityID)
	key := p.newKey(tagUsageCountsKind, keyStr)

	var dsCounts dsgen.TagUsageCountsDatastore
	err := p.client.Get(ctx, key, &dsCounts)
	if err != nil {
		if err == datastore.ErrNoSuchEntity {
			return nil, nil
		}
		return nil, err
	}

	return tagUsageCountsFromDatastore(&dsCounts), nil
}

// SaveTagUsageCounts saves usage counts to Datastore.
func (p *datastoreTagsStorageProvider) SaveTagUsageCounts(ctx context.Context, counts *v1.TagUsageCounts) error {
	dsCounts := tagUsageCountsToDatastore(counts)
	keyStr := fmt.Sprintf("%s:%s", counts.EntityType, counts.EntityId)
	key := p.newKey(tagUsageCountsKind, keyStr)
	_, err := p.client.Put(ctx, key, dsCounts)
	return err
}

// Helper functions

func entityTagKey(tagID, entityType, entityID, taggedBy string) string {
	return fmt.Sprintf("%s:%s:%s:%s", tagID, entityType, entityID, taggedBy)
}

// Conversion functions

func tagToDatastore(tag *v1.Tag) *dsgen.TagDatastore {
	var createdAt, updatedAt time.Time
	if tag.CreatedAt != nil {
		createdAt = tag.CreatedAt.AsTime()
	}
	if tag.UpdatedAt != nil {
		updatedAt = tag.UpdatedAt.AsTime()
	}
	return &dsgen.TagDatastore{
		Id:              tag.Id,
		Name:            tag.Name,
		NormalizedName:  tag.NormalizedName,
		Value:           tag.Value,
		NormalizedValue: tag.NormalizedValue,
		Type:            tag.Type,
		Color:           tag.Color,
		Description:     tag.Description,
		DisplayOrder:    tag.DisplayOrder,
		OwnerType:       tag.OwnerType,
		OwnerId:         tag.OwnerId,
		Scope:           tag.Scope,
		UsageCount:      tag.UsageCount,
		Status:          tag.Status,
		RedirectToTagId: tag.RedirectToTagId,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
		CreatorId:       tag.CreatorId,
	}
}

func tagFromDatastore(dsTag *dsgen.TagDatastore) *v1.Tag {
	var createdAt, updatedAt *timestamppb.Timestamp
	if !dsTag.CreatedAt.IsZero() {
		createdAt = timestamppb.New(dsTag.CreatedAt)
	}
	if !dsTag.UpdatedAt.IsZero() {
		updatedAt = timestamppb.New(dsTag.UpdatedAt)
	}
	return &v1.Tag{
		Id:              dsTag.Id,
		Name:            dsTag.Name,
		NormalizedName:  dsTag.NormalizedName,
		Value:           dsTag.Value,
		NormalizedValue: dsTag.NormalizedValue,
		Type:            dsTag.Type,
		Color:           dsTag.Color,
		Description:     dsTag.Description,
		DisplayOrder:    dsTag.DisplayOrder,
		OwnerType:       dsTag.OwnerType,
		OwnerId:         dsTag.OwnerId,
		Scope:           dsTag.Scope,
		UsageCount:      dsTag.UsageCount,
		Status:          dsTag.Status,
		RedirectToTagId: dsTag.RedirectToTagId,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
		CreatorId:       dsTag.CreatorId,
	}
}

func entityTagToDatastore(et *v1.EntityTag) *dsgen.EntityTagDatastore {
	var createdAt time.Time
	if et.CreatedAt != nil {
		createdAt = et.CreatedAt.AsTime()
	}
	return &dsgen.EntityTagDatastore{
		TagId:      et.TagId,
		EntityType: et.EntityType,
		EntityId:   et.EntityId,
		TaggedBy:   et.TaggedBy,
		Visibility: et.Visibility,
		CreatedAt:  createdAt,
	}
}

func entityTagFromDatastore(dsET *dsgen.EntityTagDatastore) *v1.EntityTag {
	var createdAt *timestamppb.Timestamp
	if !dsET.CreatedAt.IsZero() {
		createdAt = timestamppb.New(dsET.CreatedAt)
	}
	return &v1.EntityTag{
		TagId:      dsET.TagId,
		EntityType: dsET.EntityType,
		EntityId:   dsET.EntityId,
		TaggedBy:   dsET.TaggedBy,
		Visibility: dsET.Visibility,
		CreatedAt:  createdAt,
	}
}

func tagUsageCountsToDatastore(counts *v1.TagUsageCounts) *dsgen.TagUsageCountsDatastore {
	var updatedAt time.Time
	if counts.UpdatedAt != nil {
		updatedAt = counts.UpdatedAt.AsTime()
	}
	return &dsgen.TagUsageCountsDatastore{
		EntityType: counts.EntityType,
		EntityId:   counts.EntityId,
		TotalCount: counts.TotalCount,
		ByName:     counts.ByName,
		UpdatedAt:  updatedAt,
	}
}

func tagUsageCountsFromDatastore(dsCounts *dsgen.TagUsageCountsDatastore) *v1.TagUsageCounts {
	byName := dsCounts.ByName
	if byName == nil {
		byName = make(map[string]int64)
	}
	var updatedAt *timestamppb.Timestamp
	if !dsCounts.UpdatedAt.IsZero() {
		updatedAt = timestamppb.New(dsCounts.UpdatedAt)
	}
	return &v1.TagUsageCounts{
		EntityType: dsCounts.EntityType,
		EntityId:   dsCounts.EntityId,
		TotalCount: dsCounts.TotalCount,
		ByName:     byName,
		UpdatedAt:  updatedAt,
	}
}

// IndexProvider implementation for DatastoreTagsService

// ServiceName returns the service name for index file naming.
func (s *DatastoreTagsService) ServiceName() string {
	return "tags"
}

// RequiredIndexes returns the composite indexes required by the tags service.
func (s *DatastoreTagsService) RequiredIndexes() []dsidx.DatastoreIndex {
	return []dsidx.DatastoreIndex{
		// For ListEntityTagsByEntity - get all tags for an entity
		{
			Kind: entityTagKind,
			Properties: []dsidx.IndexProperty{
				{Name: "entity_type"},
				{Name: "entity_id"},
				{Name: "created_at", Direction: "desc"},
			},
		},
		// For ListEntityTagsByTag - get all entities with a tag
		{
			Kind: entityTagKind,
			Properties: []dsidx.IndexProperty{
				{Name: "tag_id"},
				{Name: "created_at", Direction: "desc"},
			},
		},
		// For ListEntityTagsByTag with entity type filter
		{
			Kind: entityTagKind,
			Properties: []dsidx.IndexProperty{
				{Name: "tag_id"},
				{Name: "entity_type"},
				{Name: "created_at", Direction: "desc"},
			},
		},
		// For ListTags - list tags for an owner (default order by created_at)
		{
			Kind: tagKind,
			Properties: []dsidx.IndexProperty{
				{Name: "owner_type"},
				{Name: "owner_id"},
				{Name: "status"},
				{Name: "created_at", Direction: "desc"},
			},
		},
		// For ListTags - list tags by usage count
		{
			Kind: tagKind,
			Properties: []dsidx.IndexProperty{
				{Name: "owner_type"},
				{Name: "owner_id"},
				{Name: "status"},
				{Name: "usage_count", Direction: "desc"},
			},
		},
		// For ListTags with name filter
		{
			Kind: tagKind,
			Properties: []dsidx.IndexProperty{
				{Name: "owner_type"},
				{Name: "owner_id"},
				{Name: "normalized_name"},
				{Name: "status"},
				{Name: "created_at", Direction: "desc"},
			},
		},
		// For FindTagByNormalizedValues - find a specific tag
		{
			Kind: tagKind,
			Properties: []dsidx.IndexProperty{
				{Name: "owner_type"},
				{Name: "owner_id"},
				{Name: "normalized_name"},
				{Name: "normalized_value"},
				{Name: "status"},
			},
		},
		// For SearchTags - prefix search (note: ordered by normalized_value for range query)
		{
			Kind: tagKind,
			Properties: []dsidx.IndexProperty{
				{Name: "owner_type"},
				{Name: "owner_id"},
				{Name: "status"},
				{Name: "normalized_value"},
			},
		},
		// For GetPopularTags - popular tags with usage > 0
		{
			Kind: tagKind,
			Properties: []dsidx.IndexProperty{
				{Name: "status"},
				{Name: "usage_count", Direction: "desc"},
			},
		},
	}
}

// TestQueries returns queries that exercise each required index.
func (s *DatastoreTagsService) TestQueries() []*datastore.Query {
	return []*datastore.Query{
		// ListEntityTagsByEntity
		datastore.NewQuery(entityTagKind).
			FilterField("entity_type", "=", "__test__").
			FilterField("entity_id", "=", "__test__").
			Order("-created_at"),

		// ListEntityTagsByTag
		datastore.NewQuery(entityTagKind).
			FilterField("tag_id", "=", "__test__").
			Order("-created_at"),

		// ListEntityTagsByTag with entity type filter
		datastore.NewQuery(entityTagKind).
			FilterField("tag_id", "=", "__test__").
			FilterField("entity_type", "=", "__test__").
			Order("-created_at"),

		// ListTags by created_at
		datastore.NewQuery(tagKind).
			FilterField("owner_type", "=", "__test__").
			FilterField("owner_id", "=", "__test__").
			FilterField("status", "=", int32(v1.TagStatus_TAG_STATUS_ACTIVE)).
			Order("-created_at"),

		// ListTags by usage_count
		datastore.NewQuery(tagKind).
			FilterField("owner_type", "=", "__test__").
			FilterField("owner_id", "=", "__test__").
			FilterField("status", "=", int32(v1.TagStatus_TAG_STATUS_ACTIVE)).
			Order("-usage_count"),

		// ListTags with name filter
		datastore.NewQuery(tagKind).
			FilterField("owner_type", "=", "__test__").
			FilterField("owner_id", "=", "__test__").
			FilterField("normalized_name", "=", "__test__").
			FilterField("status", "=", int32(v1.TagStatus_TAG_STATUS_ACTIVE)).
			Order("-created_at"),

		// FindTagByNormalizedValues
		datastore.NewQuery(tagKind).
			FilterField("owner_type", "=", "__test__").
			FilterField("owner_id", "=", "__test__").
			FilterField("normalized_name", "=", "__test__").
			FilterField("normalized_value", "=", "__test__").
			FilterField("status", "=", int32(v1.TagStatus_TAG_STATUS_ACTIVE)),

		// SearchTags (prefix search)
		datastore.NewQuery(tagKind).
			FilterField("owner_type", "=", "__test__").
			FilterField("owner_id", "=", "__test__").
			FilterField("status", "=", int32(v1.TagStatus_TAG_STATUS_ACTIVE)).
			FilterField("normalized_value", ">=", "a").
			FilterField("normalized_value", "<", "b"),

		// GetPopularTags
		datastore.NewQuery(tagKind).
			FilterField("status", "=", int32(v1.TagStatus_TAG_STATUS_ACTIVE)).
			FilterField("usage_count", ">", int64(0)).
			Order("-usage_count"),
	}
}

// ValidateIndexes checks if all required indexes exist.
func (s *DatastoreTagsService) ValidateIndexes(ctx context.Context) error {
	return dsidx.ValidateIndexes(ctx, s.client, s.namespace, s)
}

// WriteIndexFile writes the required indexes to a YAML file.
func (s *DatastoreTagsService) WriteIndexFile(path string) error {
	return dsidx.WriteIndexFile(path, s.ServiceName(), s.RequiredIndexes())
}

// IndexesYAML returns the indexes as a YAML string.
func (s *DatastoreTagsService) IndexesYAML() string {
	return dsidx.IndexesToYAML(s.ServiceName(), s.RequiredIndexes())
}
