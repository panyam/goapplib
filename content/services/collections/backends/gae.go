// Package backends provides concrete storage implementations for the collections service.
package backends

import (
	"context"
	"fmt"

	"cloud.google.com/go/datastore"
	dsidx "github.com/panyam/goapplib/datastore"
	"github.com/panyam/goapplib/content/services/collections"

	v1 "github.com/panyam/goapplib/content/gen/go/collections/v1"
	dsgen "github.com/panyam/goapplib/content/gen/datastore/collections/v1"
)

// Default kind names for collections service
const (
	DefaultCollectionKind     = "Collection"
	DefaultCollectionItemKind = "CollectionItem"
)

// DatastoreCollectionsService implements CollectionsService using Google Cloud Datastore.
type DatastoreCollectionsService struct {
	*collections.BaseCollectionsService
	client           *datastore.Client
	namespace        string
	indexesValidated bool

	// Kind names (customizable via WithKindNames)
	collectionKind     string
	collectionItemKind string
}

// NewDatastoreCollectionsService creates a new Datastore-backed collections service.
// Options:
//   - dsidx.WithValidation(ctx): Validate indexes exist (warns by default, use WithValidationMode to change)
//   - dsidx.WithValidationMode(mode): Set validation mode (ValidationNone, ValidationWarn, ValidationError)
//   - dsidx.WithKindNames(map[string]string{"Collection": "MyCollection"}): Override kind names
func NewDatastoreCollectionsService(client *datastore.Client, namespace string, options ...dsidx.ServiceOption) (*DatastoreCollectionsService, error) {
	opts := dsidx.ApplyOptions(options...)

	// Resolve kind names (use defaults if not overridden)
	collectionKind := DefaultCollectionKind
	collectionItemKind := DefaultCollectionItemKind

	if opts.KindNames != nil {
		if name, ok := opts.KindNames["Collection"]; ok {
			collectionKind = name
		}
		if name, ok := opts.KindNames["CollectionItem"]; ok {
			collectionItemKind = name
		}
	}

	provider := &datastoreCollectionsStorageProvider{
		client:             client,
		namespace:          namespace,
		collectionKind:     collectionKind,
		collectionItemKind: collectionItemKind,
	}
	base := collections.NewBaseCollectionsService(provider)
	service := &DatastoreCollectionsService{
		BaseCollectionsService: base,
		client:                 client,
		namespace:              namespace,
		collectionKind:         collectionKind,
		collectionItemKind:     collectionItemKind,
	}

	if opts.ValidateCtx != nil {
		if err := service.EnsureIndexesWithMode(opts.ValidateCtx, opts.ValidationMode); err != nil {
			return nil, err
		}
	}

	return service, nil
}

// EnsureIndexes validates that required indexes exist (only runs once per instance).
// Uses ValidationWarn mode by default.
func (s *DatastoreCollectionsService) EnsureIndexes(ctx context.Context) error {
	return s.EnsureIndexesWithMode(ctx, dsidx.ValidationWarn)
}

// EnsureIndexesWithMode validates indexes with the specified mode.
// Returns nil if indexes are valid or already validated.
// With ValidationError mode, returns error with deployment instructions if indexes are missing.
// With ValidationWarn mode, prints warning but returns nil.
func (s *DatastoreCollectionsService) EnsureIndexesWithMode(ctx context.Context, mode dsidx.ValidationMode) error {
	if s.indexesValidated {
		return nil
	}

	if err := dsidx.ValidateWithMode(ctx, s.client, s.namespace, s, mode); err != nil {
		return err
	}

	s.indexesValidated = true
	return nil
}

// datastoreCollectionsStorageProvider implements CollectionsStorageProvider using Datastore.
type datastoreCollectionsStorageProvider struct {
	client             *datastore.Client
	namespace          string
	collectionKind     string
	collectionItemKind string
}

func (p *datastoreCollectionsStorageProvider) newKey(kind, id string) *datastore.Key {
	key := datastore.NameKey(kind, id, nil)
	if p.namespace != "" {
		key.Namespace = p.namespace
	}
	return key
}

// SaveCollection saves a collection to Datastore.
func (p *datastoreCollectionsStorageProvider) SaveCollection(ctx context.Context, collection *v1.Collection) error {
	dsColl, err := dsgen.CollectionToCollectionDatastore(collection, nil, nil)
	if err != nil {
		return err
	}
	key := p.newKey(p.collectionKind, collection.Id)
	_, err = p.client.Put(ctx, key, dsColl)
	return err
}

// GetCollection retrieves a collection from Datastore.
func (p *datastoreCollectionsStorageProvider) GetCollection(ctx context.Context, id string) (*v1.Collection, error) {
	key := p.newKey(p.collectionKind, id)
	var dsColl dsgen.CollectionDatastore
	err := p.client.Get(ctx, key, &dsColl)
	if err != nil {
		if err == datastore.ErrNoSuchEntity {
			return nil, nil
		}
		return nil, err
	}
	dsColl.Id = id // ID is stored in key, not as property
	return dsgen.CollectionFromCollectionDatastore(nil, &dsColl, nil)
}

// DeleteCollection deletes a collection from Datastore.
func (p *datastoreCollectionsStorageProvider) DeleteCollection(ctx context.Context, id string) error {
	key := p.newKey(p.collectionKind, id)
	return p.client.Delete(ctx, key)
}

// ListCollections lists collections with filtering.
func (p *datastoreCollectionsStorageProvider) ListCollections(ctx context.Context, opts collections.ListCollectionsOptions) ([]*v1.Collection, int, error) {
	query := datastore.NewQuery(p.collectionKind)

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
	// ParentID filter (empty string = root)
	query = query.FilterField("parent_id", "=", opts.ParentID)

	if opts.Type != "" {
		query = query.FilterField("type", "=", opts.Type)
	}
	if opts.Visibility != v1.CollectionVisibility_COLLECTION_VISIBILITY_UNSPECIFIED {
		query = query.FilterField("visibility", "=", int32(opts.Visibility))
	}

	// Filter to active collections only
	query = query.FilterField("status", "=", int32(v1.CollectionStatus_COLLECTION_STATUS_ACTIVE))

	// Get total count
	countQuery := query.KeysOnly()
	keys, err := p.client.GetAll(ctx, countQuery, nil)
	if err != nil {
		return nil, 0, err
	}
	total := len(keys)

	// Apply ordering
	switch opts.OrderBy {
	case "name":
		query = query.Order("normalized_name")
	case "created_at":
		query = query.Order("-created_at")
	case "display_order":
		query = query.Order("display_order")
	default:
		query = query.Order("display_order")
	}

	// Apply pagination
	query = query.Offset(opts.Offset).Limit(opts.Limit)

	var dsColls []dsgen.CollectionDatastore
	resultKeys, err := p.client.GetAll(ctx, query, &dsColls)
	if err != nil {
		return nil, 0, err
	}

	result := make([]*v1.Collection, len(dsColls))
	for i := range dsColls {
		dsColls[i].Id = resultKeys[i].Name
		coll, err := dsgen.CollectionFromCollectionDatastore(nil, &dsColls[i], nil)
		if err != nil {
			return nil, 0, err
		}
		result[i] = coll
	}

	return result, total, nil
}

// FindCollectionByName finds a collection by its normalized name within a parent.
func (p *datastoreCollectionsStorageProvider) FindCollectionByName(ctx context.Context, ownerType, ownerID, parentID, normalizedName string) (*v1.Collection, error) {
	query := datastore.NewQuery(p.collectionKind).
		FilterField("owner_type", "=", ownerType).
		FilterField("owner_id", "=", ownerID).
		FilterField("parent_id", "=", parentID).
		FilterField("normalized_name", "=", normalizedName).
		FilterField("status", "=", int32(v1.CollectionStatus_COLLECTION_STATUS_ACTIVE)).
		Limit(1)

	if p.namespace != "" {
		query = query.Namespace(p.namespace)
	}

	var dsColls []dsgen.CollectionDatastore
	keys, err := p.client.GetAll(ctx, query, &dsColls)
	if err != nil {
		return nil, err
	}

	if len(dsColls) == 0 {
		return nil, nil
	}

	dsColls[0].Id = keys[0].Name
	return dsgen.CollectionFromCollectionDatastore(nil, &dsColls[0], nil)
}

// GetCollectionsByPath returns all collections that have the given ancestor in their path.
func (p *datastoreCollectionsStorageProvider) GetCollectionsByPath(ctx context.Context, ancestorID string) ([]*v1.Collection, error) {
	// Datastore supports array contains queries
	query := datastore.NewQuery(p.collectionKind).
		FilterField("status", "=", int32(v1.CollectionStatus_COLLECTION_STATUS_ACTIVE)).
		FilterField("path", "=", ancestorID)

	if p.namespace != "" {
		query = query.Namespace(p.namespace)
	}

	var dsColls []dsgen.CollectionDatastore
	keys, err := p.client.GetAll(ctx, query, &dsColls)
	if err != nil {
		return nil, err
	}

	result := make([]*v1.Collection, len(dsColls))
	for i := range dsColls {
		dsColls[i].Id = keys[i].Name
		coll, err := dsgen.CollectionFromCollectionDatastore(nil, &dsColls[i], nil)
		if err != nil {
			return nil, err
		}
		result[i] = coll
	}

	return result, nil
}

// UpdateCollectionPaths updates the paths of all descendants when a collection is moved.
func (p *datastoreCollectionsStorageProvider) UpdateCollectionPaths(ctx context.Context, collectionID string, oldPath, newPath []string) (int64, error) {
	// Get all descendants
	descendants, err := p.GetCollectionsByPath(ctx, collectionID)
	if err != nil {
		return 0, err
	}

	var updated int64
	for _, desc := range descendants {
		// Replace old path prefix with new path prefix
		newDescPath := make([]string, 0, len(newPath)+1+len(desc.Path)-len(oldPath))
		newDescPath = append(newDescPath, newPath...)
		newDescPath = append(newDescPath, collectionID)

		// Find where collectionID is in desc.Path and keep everything after
		for i, id := range desc.Path {
			if id == collectionID && i+1 < len(desc.Path) {
				newDescPath = append(newDescPath, desc.Path[i+1:]...)
				break
			}
		}

		desc.Path = newDescPath
		desc.Depth = int32(len(newDescPath))

		if err := p.SaveCollection(ctx, desc); err != nil {
			continue
		}
		updated++
	}

	return updated, nil
}

// collectionItemKey creates a deterministic key string for CollectionItem.
func collectionItemKey(collectionID, entityType, entityID string) string {
	return fmt.Sprintf("%s:%s:%s", collectionID, entityType, entityID)
}

// SaveCollectionItem saves a collection item to Datastore.
func (p *datastoreCollectionsStorageProvider) SaveCollectionItem(ctx context.Context, item *v1.CollectionItem) error {
	dsItem, err := dsgen.CollectionItemToCollectionItemDatastore(item, nil, nil)
	if err != nil {
		return err
	}
	keyStr := collectionItemKey(item.CollectionId, item.EntityType, item.EntityId)
	key := p.newKey(p.collectionItemKind, keyStr)
	_, err = p.client.Put(ctx, key, dsItem)
	return err
}

// GetCollectionItem retrieves a collection item.
func (p *datastoreCollectionsStorageProvider) GetCollectionItem(ctx context.Context, collectionID, entityType, entityID string) (*v1.CollectionItem, error) {
	keyStr := collectionItemKey(collectionID, entityType, entityID)
	key := p.newKey(p.collectionItemKind, keyStr)

	var dsItem dsgen.CollectionItemDatastore
	err := p.client.Get(ctx, key, &dsItem)
	if err != nil {
		if err == datastore.ErrNoSuchEntity {
			return nil, nil
		}
		return nil, err
	}

	return dsgen.CollectionItemFromCollectionItemDatastore(nil, &dsItem, nil)
}

// DeleteCollectionItem deletes a collection item from Datastore.
func (p *datastoreCollectionsStorageProvider) DeleteCollectionItem(ctx context.Context, collectionID, entityType, entityID string) error {
	keyStr := collectionItemKey(collectionID, entityType, entityID)
	key := p.newKey(p.collectionItemKind, keyStr)
	return p.client.Delete(ctx, key)
}

// ListCollectionItems lists items in a collection.
func (p *datastoreCollectionsStorageProvider) ListCollectionItems(ctx context.Context, collectionID string, opts collections.ListItemsOptions) ([]*v1.CollectionItem, int, error) {
	query := datastore.NewQuery(p.collectionItemKind).
		FilterField("collection_id", "=", collectionID)

	if p.namespace != "" {
		query = query.Namespace(p.namespace)
	}

	if opts.EntityTypeFilter != "" {
		query = query.FilterField("entity_type", "=", opts.EntityTypeFilter)
	}

	// Get total count
	countQuery := query.KeysOnly()
	keys, err := p.client.GetAll(ctx, countQuery, nil)
	if err != nil {
		return nil, 0, err
	}
	total := len(keys)

	// Apply sorting
	switch opts.SortBy {
	case v1.SortField_SORT_FIELD_ADDED_AT:
		if opts.SortDescending {
			query = query.Order("-added_at")
		} else {
			query = query.Order("added_at")
		}
	default: // SORT_FIELD_DISPLAY_ORDER
		if opts.SortDescending {
			query = query.Order("-display_order")
		} else {
			query = query.Order("display_order")
		}
	}

	// Apply pagination
	query = query.Offset(opts.Offset).Limit(opts.Limit)

	var dsItems []dsgen.CollectionItemDatastore
	_, err = p.client.GetAll(ctx, query, &dsItems)
	if err != nil {
		return nil, 0, err
	}

	result := make([]*v1.CollectionItem, len(dsItems))
	for i := range dsItems {
		item, err := dsgen.CollectionItemFromCollectionItemDatastore(nil, &dsItems[i], nil)
		if err != nil {
			return nil, 0, err
		}
		result[i] = item
	}

	return result, total, nil
}

// ListEntityCollections returns all collections that contain an entity.
func (p *datastoreCollectionsStorageProvider) ListEntityCollections(ctx context.Context, entityType, entityID string, opts collections.EntityCollectionsOptions) ([]*v1.Collection, error) {
	query := datastore.NewQuery(p.collectionItemKind).
		FilterField("entity_type", "=", entityType).
		FilterField("entity_id", "=", entityID)

	if p.namespace != "" {
		query = query.Namespace(p.namespace)
	}

	var dsItems []dsgen.CollectionItemDatastore
	_, err := p.client.GetAll(ctx, query, &dsItems)
	if err != nil {
		return nil, err
	}

	// Fetch collections for each item
	result := make([]*v1.Collection, 0, len(dsItems))
	for _, item := range dsItems {
		collection, err := p.GetCollection(ctx, item.CollectionId)
		if err != nil || collection == nil {
			continue
		}

		// Filter by owner if specified
		if opts.OwnerType != "" && collection.OwnerType != opts.OwnerType {
			continue
		}
		if opts.OwnerID != "" && collection.OwnerId != opts.OwnerID {
			continue
		}

		result = append(result, collection)
	}

	return result, nil
}

// DeleteItemsByCollection deletes all items in a collection.
func (p *datastoreCollectionsStorageProvider) DeleteItemsByCollection(ctx context.Context, collectionID string) (int64, error) {
	query := datastore.NewQuery(p.collectionItemKind).
		FilterField("collection_id", "=", collectionID).
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

// UpdateItemOrders updates the display order of multiple items.
func (p *datastoreCollectionsStorageProvider) UpdateItemOrders(ctx context.Context, collectionID string, orders []*v1.ItemOrder) (int64, error) {
	var updated int64

	for _, order := range orders {
		item, err := p.GetCollectionItem(ctx, collectionID, order.EntityType, order.EntityId)
		if err != nil || item == nil {
			continue
		}

		item.DisplayOrder = order.DisplayOrder
		if err := p.SaveCollectionItem(ctx, item); err != nil {
			continue
		}
		updated++
	}

	return updated, nil
}

// GetMaxDisplayOrder returns the maximum display order in a collection.
func (p *datastoreCollectionsStorageProvider) GetMaxDisplayOrder(ctx context.Context, collectionID string) (int32, error) {
	query := datastore.NewQuery(p.collectionItemKind).
		FilterField("collection_id", "=", collectionID).
		Order("-display_order").
		Limit(1)

	if p.namespace != "" {
		query = query.Namespace(p.namespace)
	}

	var dsItems []dsgen.CollectionItemDatastore
	_, err := p.client.GetAll(ctx, query, &dsItems)
	if err != nil {
		return 0, err
	}

	if len(dsItems) == 0 {
		return 0, nil
	}

	return dsItems[0].DisplayOrder, nil
}

// IndexProvider implementation for DatastoreCollectionsService

// ServiceName returns the service name for index file naming.
func (s *DatastoreCollectionsService) ServiceName() string {
	return "collections"
}

// RequiredIndexes returns the composite indexes required by the collections service.
func (s *DatastoreCollectionsService) RequiredIndexes() []dsidx.DatastoreIndex {
	return []dsidx.DatastoreIndex{
		// For ListCollections - list by owner and parent (default order by display_order)
		{
			Kind: s.collectionKind,
			Properties: []dsidx.IndexProperty{
				{Name: "owner_type"},
				{Name: "owner_id"},
				{Name: "parent_id"},
				{Name: "status"},
				{Name: "display_order"},
			},
		},
		// For ListCollections - list by owner and parent (order by created_at)
		{
			Kind: s.collectionKind,
			Properties: []dsidx.IndexProperty{
				{Name: "owner_type"},
				{Name: "owner_id"},
				{Name: "parent_id"},
				{Name: "status"},
				{Name: "created_at", Direction: "desc"},
			},
		},
		// For ListCollections - list by owner and parent (order by name)
		{
			Kind: s.collectionKind,
			Properties: []dsidx.IndexProperty{
				{Name: "owner_type"},
				{Name: "owner_id"},
				{Name: "parent_id"},
				{Name: "status"},
				{Name: "normalized_name"},
			},
		},
		// For FindCollectionByName - find collection by name within parent
		{
			Kind: s.collectionKind,
			Properties: []dsidx.IndexProperty{
				{Name: "owner_type"},
				{Name: "owner_id"},
				{Name: "parent_id"},
				{Name: "normalized_name"},
				{Name: "status"},
			},
		},
		// For GetCollectionsByPath - find descendants
		{
			Kind: s.collectionKind,
			Properties: []dsidx.IndexProperty{
				{Name: "status"},
				{Name: "path"},
			},
		},
		// For ListCollectionItems - list items by collection (order by display_order)
		{
			Kind: s.collectionItemKind,
			Properties: []dsidx.IndexProperty{
				{Name: "collection_id"},
				{Name: "display_order"},
			},
		},
		// For ListCollectionItems - list items by collection (order by added_at desc)
		{
			Kind: s.collectionItemKind,
			Properties: []dsidx.IndexProperty{
				{Name: "collection_id"},
				{Name: "added_at", Direction: "desc"},
			},
		},
		// For ListCollectionItems with entity type filter
		{
			Kind: s.collectionItemKind,
			Properties: []dsidx.IndexProperty{
				{Name: "collection_id"},
				{Name: "entity_type"},
				{Name: "display_order"},
			},
		},
		// For ListEntityCollections - find collections containing entity
		{
			Kind: s.collectionItemKind,
			Properties: []dsidx.IndexProperty{
				{Name: "entity_type"},
				{Name: "entity_id"},
			},
		},
		// For GetMaxDisplayOrder - get max order in collection
		{
			Kind: s.collectionItemKind,
			Properties: []dsidx.IndexProperty{
				{Name: "collection_id"},
				{Name: "display_order", Direction: "desc"},
			},
		},
	}
}

// TestQueries returns queries that exercise each required index.
func (s *DatastoreCollectionsService) TestQueries() []*datastore.Query {
	return []*datastore.Query{
		// ListCollections by display_order
		datastore.NewQuery(s.collectionKind).
			FilterField("owner_type", "=", "__test__").
			FilterField("owner_id", "=", "__test__").
			FilterField("parent_id", "=", "").
			FilterField("status", "=", int32(v1.CollectionStatus_COLLECTION_STATUS_ACTIVE)).
			Order("display_order"),

		// ListCollections by created_at
		datastore.NewQuery(s.collectionKind).
			FilterField("owner_type", "=", "__test__").
			FilterField("owner_id", "=", "__test__").
			FilterField("parent_id", "=", "").
			FilterField("status", "=", int32(v1.CollectionStatus_COLLECTION_STATUS_ACTIVE)).
			Order("-created_at"),

		// ListCollections by name
		datastore.NewQuery(s.collectionKind).
			FilterField("owner_type", "=", "__test__").
			FilterField("owner_id", "=", "__test__").
			FilterField("parent_id", "=", "").
			FilterField("status", "=", int32(v1.CollectionStatus_COLLECTION_STATUS_ACTIVE)).
			Order("normalized_name"),

		// FindCollectionByName
		datastore.NewQuery(s.collectionKind).
			FilterField("owner_type", "=", "__test__").
			FilterField("owner_id", "=", "__test__").
			FilterField("parent_id", "=", "").
			FilterField("normalized_name", "=", "__test__").
			FilterField("status", "=", int32(v1.CollectionStatus_COLLECTION_STATUS_ACTIVE)),

		// GetCollectionsByPath
		datastore.NewQuery(s.collectionKind).
			FilterField("status", "=", int32(v1.CollectionStatus_COLLECTION_STATUS_ACTIVE)).
			FilterField("path", "=", "__test__"),

		// ListCollectionItems by display_order
		datastore.NewQuery(s.collectionItemKind).
			FilterField("collection_id", "=", "__test__").
			Order("display_order"),

		// ListCollectionItems by added_at
		datastore.NewQuery(s.collectionItemKind).
			FilterField("collection_id", "=", "__test__").
			Order("-added_at"),

		// ListCollectionItems with entity type filter
		datastore.NewQuery(s.collectionItemKind).
			FilterField("collection_id", "=", "__test__").
			FilterField("entity_type", "=", "__test__").
			Order("display_order"),

		// ListEntityCollections
		datastore.NewQuery(s.collectionItemKind).
			FilterField("entity_type", "=", "__test__").
			FilterField("entity_id", "=", "__test__"),

		// GetMaxDisplayOrder
		datastore.NewQuery(s.collectionItemKind).
			FilterField("collection_id", "=", "__test__").
			Order("-display_order"),
	}
}

// ValidateIndexes checks if all required indexes exist.
func (s *DatastoreCollectionsService) ValidateIndexes(ctx context.Context) error {
	return dsidx.ValidateIndexes(ctx, s.client, s.namespace, s)
}

// WriteIndexFile writes the required indexes to a YAML file.
func (s *DatastoreCollectionsService) WriteIndexFile(path string) error {
	return dsidx.WriteIndexFile(path, s.ServiceName(), s.RequiredIndexes())
}

// IndexesYAML returns the indexes as a YAML string.
func (s *DatastoreCollectionsService) IndexesYAML() string {
	return dsidx.IndexesToYAML(s.ServiceName(), s.RequiredIndexes())
}
