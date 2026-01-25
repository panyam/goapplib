// Package backends provides concrete storage implementations for the collections service.
package backends

import (
	"context"

	"github.com/panyam/goapplib/content/services/collections"

	v1 "github.com/panyam/goapplib/content/gen/go/collections/v1"
	gormgen "github.com/panyam/goapplib/content/gen/gorm/collections/v1"
	"gorm.io/gorm"
)

// GORMCollectionsService implements CollectionsService using GORM.
type GORMCollectionsService struct {
	*collections.BaseCollectionsService
	db *gorm.DB
}

// NewGORMCollectionsService creates a new GORM-backed collections service.
func NewGORMCollectionsService(db *gorm.DB) *GORMCollectionsService {
	provider := &gormCollectionsStorageProvider{db: db}
	base := collections.NewBaseCollectionsService(provider)
	return &GORMCollectionsService{
		BaseCollectionsService: base,
		db:                     db,
	}
}

// AutoMigrate creates the database tables.
func (s *GORMCollectionsService) AutoMigrate() error {
	return s.db.AutoMigrate(
		&gormgen.CollectionGORM{},
		&gormgen.CollectionItemGORM{},
	)
}

// gormCollectionsStorageProvider implements CollectionsStorageProvider using GORM.
type gormCollectionsStorageProvider struct {
	db *gorm.DB
}

// SaveCollection saves a collection to the database.
func (p *gormCollectionsStorageProvider) SaveCollection(ctx context.Context, collection *v1.Collection) error {
	gormColl, err := gormgen.CollectionToCollectionGORM(collection, nil, nil)
	if err != nil {
		return err
	}
	return p.db.WithContext(ctx).Save(gormColl).Error
}

// GetCollection retrieves a collection by ID.
func (p *gormCollectionsStorageProvider) GetCollection(ctx context.Context, id string) (*v1.Collection, error) {
	var gormColl gormgen.CollectionGORM
	err := p.db.WithContext(ctx).Where("id = ?", id).First(&gormColl).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return gormgen.CollectionFromCollectionGORM(nil, &gormColl, nil)
}

// DeleteCollection deletes a collection from the database.
func (p *gormCollectionsStorageProvider) DeleteCollection(ctx context.Context, id string) error {
	return p.db.WithContext(ctx).Where("id = ?", id).Delete(&gormgen.CollectionGORM{}).Error
}

// ListCollections lists collections with filtering.
func (p *gormCollectionsStorageProvider) ListCollections(ctx context.Context, opts collections.ListCollectionsOptions) ([]*v1.Collection, int, error) {
	var gormColls []gormgen.CollectionGORM
	var total int64

	query := p.db.WithContext(ctx).Model(&gormgen.CollectionGORM{})

	// Apply filters
	if opts.OwnerType != "" {
		query = query.Where("owner_type = ?", opts.OwnerType)
	}
	if opts.OwnerID != "" {
		query = query.Where("owner_id = ?", opts.OwnerID)
	}
	// ParentID filter: empty string means root collections
	query = query.Where("parent_id = ?", opts.ParentID)

	if opts.Type != "" {
		query = query.Where("type = ?", opts.Type)
	}
	if opts.Visibility != v1.CollectionVisibility_COLLECTION_VISIBILITY_UNSPECIFIED {
		query = query.Where("visibility = ?", opts.Visibility)
	}

	// Exclude deleted collections
	query = query.Where("status = ?", v1.CollectionStatus_COLLECTION_STATUS_ACTIVE)

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply ordering
	switch opts.OrderBy {
	case "name":
		query = query.Order("normalized_name ASC")
	case "created_at":
		query = query.Order("created_at DESC")
	case "display_order":
		query = query.Order("display_order ASC")
	default:
		query = query.Order("display_order ASC, created_at DESC")
	}

	// Apply pagination
	if err := query.Offset(opts.Offset).Limit(opts.Limit).Find(&gormColls).Error; err != nil {
		return nil, 0, err
	}

	result := make([]*v1.Collection, len(gormColls))
	for i := range gormColls {
		coll, err := gormgen.CollectionFromCollectionGORM(nil, &gormColls[i], nil)
		if err != nil {
			return nil, 0, err
		}
		result[i] = coll
	}

	return result, int(total), nil
}

// FindCollectionByName finds a collection by its normalized name within a parent.
func (p *gormCollectionsStorageProvider) FindCollectionByName(ctx context.Context, ownerType, ownerID, parentID, normalizedName string) (*v1.Collection, error) {
	var gormColl gormgen.CollectionGORM
	err := p.db.WithContext(ctx).
		Where("owner_type = ? AND owner_id = ? AND parent_id = ? AND normalized_name = ?",
			ownerType, ownerID, parentID, normalizedName).
		Where("status = ?", v1.CollectionStatus_COLLECTION_STATUS_ACTIVE).
		First(&gormColl).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return gormgen.CollectionFromCollectionGORM(nil, &gormColl, nil)
}

// GetCollectionsByPath returns all collections that have the given ancestor in their path.
func (p *gormCollectionsStorageProvider) GetCollectionsByPath(ctx context.Context, ancestorID string) ([]*v1.Collection, error) {
	var gormColls []gormgen.CollectionGORM
	// Use JSON contains for SQLite/PostgreSQL
	err := p.db.WithContext(ctx).
		Where("status = ?", v1.CollectionStatus_COLLECTION_STATUS_ACTIVE).
		Where("path LIKE ?", "%\""+ancestorID+"\"%").
		Find(&gormColls).Error
	if err != nil {
		return nil, err
	}

	result := make([]*v1.Collection, len(gormColls))
	for i := range gormColls {
		coll, err := gormgen.CollectionFromCollectionGORM(nil, &gormColls[i], nil)
		if err != nil {
			return nil, err
		}
		result[i] = coll
	}

	return result, nil
}

// UpdateCollectionPaths updates the paths of all descendants when a collection is moved.
func (p *gormCollectionsStorageProvider) UpdateCollectionPaths(ctx context.Context, collectionID string, oldPath, newPath []string) (int64, error) {
	// Get all descendants
	descendants, err := p.GetCollectionsByPath(ctx, collectionID)
	if err != nil {
		return 0, err
	}

	var updated int64
	for _, desc := range descendants {
		// Replace old path prefix with new path prefix
		// Old path: [...oldPath, collectionID, ...]
		// New path: [...newPath, collectionID, ...]
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

// SaveCollectionItem saves a collection item to the database.
func (p *gormCollectionsStorageProvider) SaveCollectionItem(ctx context.Context, item *v1.CollectionItem) error {
	gormItem, err := gormgen.CollectionItemToCollectionItemGORM(item, nil, nil)
	if err != nil {
		return err
	}
	return p.db.WithContext(ctx).Save(gormItem).Error
}

// GetCollectionItem retrieves a collection item.
func (p *gormCollectionsStorageProvider) GetCollectionItem(ctx context.Context, collectionID, entityType, entityID string) (*v1.CollectionItem, error) {
	var gormItem gormgen.CollectionItemGORM
	err := p.db.WithContext(ctx).
		Where("collection_id = ? AND entity_type = ? AND entity_id = ?",
			collectionID, entityType, entityID).
		First(&gormItem).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return gormgen.CollectionItemFromCollectionItemGORM(nil, &gormItem, nil)
}

// DeleteCollectionItem deletes a collection item from the database.
func (p *gormCollectionsStorageProvider) DeleteCollectionItem(ctx context.Context, collectionID, entityType, entityID string) error {
	return p.db.WithContext(ctx).
		Where("collection_id = ? AND entity_type = ? AND entity_id = ?",
			collectionID, entityType, entityID).
		Delete(&gormgen.CollectionItemGORM{}).Error
}

// ListCollectionItems lists items in a collection.
func (p *gormCollectionsStorageProvider) ListCollectionItems(ctx context.Context, collectionID string, opts collections.ListItemsOptions) ([]*v1.CollectionItem, int, error) {
	var gormItems []gormgen.CollectionItemGORM
	var total int64

	query := p.db.WithContext(ctx).Model(&gormgen.CollectionItemGORM{}).
		Where("collection_id = ?", collectionID)

	if opts.EntityTypeFilter != "" {
		query = query.Where("entity_type = ?", opts.EntityTypeFilter)
	}

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply sorting
	switch opts.SortBy {
	case v1.SortField_SORT_FIELD_ADDED_AT:
		if opts.SortDescending {
			query = query.Order("added_at DESC")
		} else {
			query = query.Order("added_at ASC")
		}
	default: // SORT_FIELD_DISPLAY_ORDER
		if opts.SortDescending {
			query = query.Order("display_order DESC")
		} else {
			query = query.Order("display_order ASC")
		}
	}

	// Apply pagination
	if err := query.Offset(opts.Offset).Limit(opts.Limit).Find(&gormItems).Error; err != nil {
		return nil, 0, err
	}

	result := make([]*v1.CollectionItem, len(gormItems))
	for i := range gormItems {
		item, err := gormgen.CollectionItemFromCollectionItemGORM(nil, &gormItems[i], nil)
		if err != nil {
			return nil, 0, err
		}
		result[i] = item
	}

	return result, int(total), nil
}

// ListEntityCollections returns all collections that contain an entity.
func (p *gormCollectionsStorageProvider) ListEntityCollections(ctx context.Context, entityType, entityID string, opts collections.EntityCollectionsOptions) ([]*v1.Collection, error) {
	var gormItems []gormgen.CollectionItemGORM

	query := p.db.WithContext(ctx).
		Where("entity_type = ? AND entity_id = ?", entityType, entityID)

	if err := query.Find(&gormItems).Error; err != nil {
		return nil, err
	}

	// Fetch collections for each item
	result := make([]*v1.Collection, 0, len(gormItems))
	for _, item := range gormItems {
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
func (p *gormCollectionsStorageProvider) DeleteItemsByCollection(ctx context.Context, collectionID string) (int64, error) {
	result := p.db.WithContext(ctx).Where("collection_id = ?", collectionID).Delete(&gormgen.CollectionItemGORM{})
	return result.RowsAffected, result.Error
}

// UpdateItemOrders updates the display order of multiple items.
func (p *gormCollectionsStorageProvider) UpdateItemOrders(ctx context.Context, collectionID string, orders []*v1.ItemOrder) (int64, error) {
	var updated int64

	for _, order := range orders {
		result := p.db.WithContext(ctx).
			Model(&gormgen.CollectionItemGORM{}).
			Where("collection_id = ? AND entity_type = ? AND entity_id = ?",
				collectionID, order.EntityType, order.EntityId).
			Update("display_order", order.DisplayOrder)

		if result.Error != nil {
			continue
		}
		updated += result.RowsAffected
	}

	return updated, nil
}

// GetMaxDisplayOrder returns the maximum display order in a collection.
func (p *gormCollectionsStorageProvider) GetMaxDisplayOrder(ctx context.Context, collectionID string) (int32, error) {
	var maxOrder int32
	err := p.db.WithContext(ctx).
		Model(&gormgen.CollectionItemGORM{}).
		Where("collection_id = ?", collectionID).
		Select("COALESCE(MAX(display_order), 0)").
		Scan(&maxOrder).Error
	return maxOrder, err
}
