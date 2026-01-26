// Package backends provides concrete storage implementations for the tags service.
package backends

import (
	"context"
	"time"

	"github.com/panyam/goapplib/content/services/tags"

	v1 "github.com/panyam/goapplib/content/gen/go/tags/v1"
	gormgen "github.com/panyam/goapplib/content/gen/gorm/tags/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

// GORMTagsService implements TagsService using GORM.
type GORMTagsService struct {
	*tags.BaseTagsService
	db *gorm.DB
}

// NewGORMTagsService creates a new GORM-backed tags service.
func NewGORMTagsService(db *gorm.DB) *GORMTagsService {
	provider := &gormTagsStorageProvider{db: db}
	base := tags.NewBaseTagsService(provider)
	return &GORMTagsService{
		BaseTagsService: base,
		db:              db,
	}
}

// NewGORMTagsServiceWithOptions creates a new GORM-backed tags service with options.
func NewGORMTagsServiceWithOptions(db *gorm.DB, opts ...tags.ServiceOption) *GORMTagsService {
	provider := &gormTagsStorageProvider{db: db}
	base := tags.NewBaseTagsService(provider, opts...)
	return &GORMTagsService{
		BaseTagsService: base,
		db:              db,
	}
}

// Re-export hook types and options for convenience
type (
	HookContext              = tags.HookContext
	Event                    = tags.Event
	EventType                = tags.EventType
	AuthorizeHook            = tags.AuthorizeHook
	ValidateEntityHook       = tags.ValidateEntityHook
	BeforeTagSaveHook        = tags.BeforeTagSaveHook
	AfterTagSaveHook         = tags.AfterTagSaveHook
	BeforeTagDeleteHook      = tags.BeforeTagDeleteHook
	AfterTagDeleteHook       = tags.AfterTagDeleteHook
	BeforeEntityTagSaveHook  = tags.BeforeEntityTagSaveHook
	AfterEntityTagSaveHook   = tags.AfterEntityTagSaveHook
	BeforeEntityTagDeleteHook = tags.BeforeEntityTagDeleteHook
	AfterEntityTagDeleteHook  = tags.AfterEntityTagDeleteHook
	AfterTagsReadHook        = tags.AfterTagsReadHook
	OnEventHook              = tags.OnEventHook
)

// Re-export event types
const (
	EventTagCreated     = tags.EventTagCreated
	EventTagUpdated     = tags.EventTagUpdated
	EventTagDeleted     = tags.EventTagDeleted
	EventEntityTagged   = tags.EventEntityTagged
	EventEntityUntagged = tags.EventEntityUntagged
	EventTagsMerged     = tags.EventTagsMerged
	EventTagPromoted    = tags.EventTagPromoted
)

// Re-export option functions
var (
	WithHooks                 = tags.WithHooks
	WithOnAuthorize           = tags.WithOnAuthorize
	WithValidateEntity        = tags.WithValidateEntity
	WithBeforeTagSave         = tags.WithBeforeTagSave
	WithAfterTagSave          = tags.WithAfterTagSave
	WithBeforeTagDelete       = tags.WithBeforeTagDelete
	WithAfterTagDelete        = tags.WithAfterTagDelete
	WithBeforeEntityTagSave   = tags.WithBeforeEntityTagSave
	WithAfterEntityTagSave    = tags.WithAfterEntityTagSave
	WithBeforeEntityTagDelete = tags.WithBeforeEntityTagDelete
	WithAfterEntityTagDelete  = tags.WithAfterEntityTagDelete
	WithAfterTagsRead         = tags.WithAfterTagsRead
	WithOnEvent               = tags.WithOnEvent
	WithNormalizer            = tags.WithNormalizer
	WithUserIDContextKey      = tags.WithUserIDContextKey
	WithCache                 = tags.WithCache
)

// AutoMigrate creates the database tables.
func (s *GORMTagsService) AutoMigrate() error {
	return s.db.AutoMigrate(
		&gormgen.TagGORM{},
		&gormgen.EntityTagGORM{},
		&gormgen.TagUsageCountsGORM{},
	)
}

// gormTagsStorageProvider implements TagsStorageProvider using GORM.
type gormTagsStorageProvider struct {
	db *gorm.DB
}

// SaveTag saves a tag to the database.
func (p *gormTagsStorageProvider) SaveTag(ctx context.Context, tag *v1.Tag) error {
	gormTag := tagToGORM(tag)
	return p.db.WithContext(ctx).Save(gormTag).Error
}

// GetTag retrieves a tag by ID.
func (p *gormTagsStorageProvider) GetTag(ctx context.Context, id string) (*v1.Tag, error) {
	var gormTag gormgen.TagGORM
	err := p.db.WithContext(ctx).Where("id = ?", id).First(&gormTag).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return tagFromGORM(&gormTag), nil
}

// DeleteTag deletes a tag from the database.
func (p *gormTagsStorageProvider) DeleteTag(ctx context.Context, id string) error {
	return p.db.WithContext(ctx).Where("id = ?", id).Delete(&gormgen.TagGORM{}).Error
}

// ListTags lists tags with filtering.
func (p *gormTagsStorageProvider) ListTags(ctx context.Context, opts tags.ListTagsOptions) ([]*v1.Tag, int, error) {
	var gormTags []gormgen.TagGORM
	var total int64

	query := p.db.WithContext(ctx).Model(&gormgen.TagGORM{})

	// Apply filters
	if opts.OwnerID != "" {
		query = query.Where("owner_id = ?", opts.OwnerID)
	}
	if opts.Scope != v1.TagScope_TAG_SCOPE_UNSPECIFIED {
		query = query.Where("scope = ?", opts.Scope)
	}
	if opts.NameFilter != "" && opts.NameFilter != "*" {
		query = query.Where("normalized_name = ?", opts.NameFilter)
	}

	// Exclude deleted/redirect tags by default
	query = query.Where("status = ?", v1.TagStatus_TAG_STATUS_ACTIVE)

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply ordering
	switch opts.OrderBy {
	case "usage_count":
		query = query.Order("usage_count DESC")
	case "created_at":
		query = query.Order("created_at DESC")
	case "display_order":
		query = query.Order("display_order ASC")
	case "name":
		query = query.Order("normalized_name ASC, normalized_value ASC")
	default:
		query = query.Order("created_at DESC")
	}

	// Apply pagination
	if err := query.Offset(opts.Offset).Limit(opts.Limit).Find(&gormTags).Error; err != nil {
		return nil, 0, err
	}

	result := make([]*v1.Tag, len(gormTags))
	for i, gt := range gormTags {
		result[i] = tagFromGORM(&gt)
	}

	return result, int(total), nil
}

// FindTagByNormalizedValues finds a tag by its normalized name and value.
func (p *gormTagsStorageProvider) FindTagByNormalizedValues(ctx context.Context, ownerID, normalizedName, normalizedValue string) (*v1.Tag, error) {
	var gormTag gormgen.TagGORM
	err := p.db.WithContext(ctx).
		Where("owner_id = ? AND normalized_name = ? AND normalized_value = ?",
			ownerID, normalizedName, normalizedValue).
		Where("status = ?", v1.TagStatus_TAG_STATUS_ACTIVE).
		First(&gormTag).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return tagFromGORM(&gormTag), nil
}

// SearchTags searches for tags by query (prefix match).
func (p *gormTagsStorageProvider) SearchTags(ctx context.Context, opts tags.SearchTagsOptions) ([]*v1.Tag, error) {
	var gormTags []gormgen.TagGORM

	query := p.db.WithContext(ctx).Model(&gormgen.TagGORM{}).
		Where("status = ?", v1.TagStatus_TAG_STATUS_ACTIVE)

	// Search by prefix on normalized_value
	if opts.Query != "" {
		query = query.Where("normalized_value LIKE ?", opts.Query+"%")
	}

	// Apply name filter
	if opts.NameFilter != "" && opts.NameFilter != "*" {
		query = query.Where("normalized_name = ?", opts.NameFilter)
	}

	// Owner filter with optional shared tags
	if opts.OwnerID != "" {
		if opts.IncludeShared {
			query = query.Where(
				"(owner_id = ?) OR scope IN (?, ?)",
				opts.OwnerID,
				v1.TagScope_TAG_SCOPE_SHARED, v1.TagScope_TAG_SCOPE_PUBLIC,
			)
		} else {
			query = query.Where("owner_id = ?", opts.OwnerID)
		}
	}

	// Apply limit
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}

	if err := query.Order("usage_count DESC").Limit(limit).Find(&gormTags).Error; err != nil {
		return nil, err
	}

	result := make([]*v1.Tag, len(gormTags))
	for i, gt := range gormTags {
		result[i] = tagFromGORM(&gt)
	}

	return result, nil
}

// GetPopularTags returns the most used tags.
func (p *gormTagsStorageProvider) GetPopularTags(ctx context.Context, opts tags.PopularTagsOptions) ([]*v1.Tag, error) {
	var gormTags []gormgen.TagGORM

	query := p.db.WithContext(ctx).Model(&gormgen.TagGORM{}).
		Where("status = ?", v1.TagStatus_TAG_STATUS_ACTIVE).
		Where("usage_count > 0")

	// Apply name filter
	if opts.NameFilter != "" && opts.NameFilter != "*" {
		query = query.Where("normalized_name = ?", opts.NameFilter)
	}

	// Owner filter with optional shared tags
	if opts.OwnerID != "" {
		if opts.IncludeShared {
			query = query.Where(
				"owner_id = ? OR scope IN (?, ?)",
				opts.OwnerID,
				v1.TagScope_TAG_SCOPE_SHARED, v1.TagScope_TAG_SCOPE_PUBLIC,
			)
		} else {
			query = query.Where("owner_id = ?", opts.OwnerID)
		}
	}

	// Apply limit
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}

	if err := query.Order("usage_count DESC").Limit(limit).Find(&gormTags).Error; err != nil {
		return nil, err
	}

	result := make([]*v1.Tag, len(gormTags))
	for i, gt := range gormTags {
		result[i] = tagFromGORM(&gt)
	}

	return result, nil
}

// SaveEntityTag saves an entity tag to the database.
func (p *gormTagsStorageProvider) SaveEntityTag(ctx context.Context, entityTag *v1.EntityTag) error {
	gormET := entityTagToGORM(entityTag)
	return p.db.WithContext(ctx).Save(gormET).Error
}

// DeleteEntityTag deletes an entity tag from the database.
func (p *gormTagsStorageProvider) DeleteEntityTag(ctx context.Context, tagID, entityID, taggedBy string) error {
	return p.db.WithContext(ctx).
		Where("tag_id = ? AND entity_id = ? AND tagged_by = ?",
			tagID, entityID, taggedBy).
		Delete(&gormgen.EntityTagGORM{}).Error
}

// GetEntityTag retrieves an entity tag.
func (p *gormTagsStorageProvider) GetEntityTag(ctx context.Context, tagID, entityID, taggedBy string) (*v1.EntityTag, error) {
	var gormET gormgen.EntityTagGORM
	err := p.db.WithContext(ctx).
		Where("tag_id = ? AND entity_id = ? AND tagged_by = ?",
			tagID, entityID, taggedBy).
		First(&gormET).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return entityTagFromGORM(&gormET), nil
}

// ListEntityTagsByEntity lists entity tags for an entity.
func (p *gormTagsStorageProvider) ListEntityTagsByEntity(ctx context.Context, entityID string, opts tags.EntityTagsOptions) ([]*v1.EntityTag, error) {
	var gormETs []gormgen.EntityTagGORM

	query := p.db.WithContext(ctx).Model(&gormgen.EntityTagGORM{}).
		Where("entity_id = ?", entityID)

	// We need to join with tags table to filter by name and owner
	if opts.NameFilter != "" || opts.OwnerID != "" {
		query = query.Joins("JOIN tags ON tags.id = entity_tags.tag_id")

		if opts.NameFilter != "" && opts.NameFilter != "*" {
			query = query.Where("tags.normalized_name = ?", opts.NameFilter)
		}
		if opts.OwnerID != "" {
			query = query.Where("tags.owner_id = ?", opts.OwnerID)
		}
	}

	if err := query.Order("entity_tags.created_at DESC").Find(&gormETs).Error; err != nil {
		return nil, err
	}

	result := make([]*v1.EntityTag, len(gormETs))
	for i, et := range gormETs {
		result[i] = entityTagFromGORM(&et)
	}

	return result, nil
}

// ListEntityTagsByTag lists entity tags for a tag.
func (p *gormTagsStorageProvider) ListEntityTagsByTag(ctx context.Context, tagID string, limit, offset int) ([]*v1.EntityTag, int, error) {
	var gormETs []gormgen.EntityTagGORM
	var total int64

	query := p.db.WithContext(ctx).Model(&gormgen.EntityTagGORM{}).
		Where("tag_id = ?", tagID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&gormETs).Error; err != nil {
		return nil, 0, err
	}

	result := make([]*v1.EntityTag, len(gormETs))
	for i, et := range gormETs {
		result[i] = entityTagFromGORM(&et)
	}

	return result, int(total), nil
}

// DeleteEntityTagsByTag deletes all entity tags for a tag.
func (p *gormTagsStorageProvider) DeleteEntityTagsByTag(ctx context.Context, tagID string) (int64, error) {
	result := p.db.WithContext(ctx).Where("tag_id = ?", tagID).Delete(&gormgen.EntityTagGORM{})
	return result.RowsAffected, result.Error
}

// GetTagUsageCounts retrieves usage counts for an entity.
func (p *gormTagsStorageProvider) GetTagUsageCounts(ctx context.Context, entityID string) (*v1.TagUsageCounts, error) {
	var gormCounts gormgen.TagUsageCountsGORM
	err := p.db.WithContext(ctx).
		Where("entity_id = ?", entityID).
		First(&gormCounts).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return tagUsageCountsFromGORM(&gormCounts), nil
}

// SaveTagUsageCounts saves usage counts to the database.
func (p *gormTagsStorageProvider) SaveTagUsageCounts(ctx context.Context, counts *v1.TagUsageCounts) error {
	gormCounts := tagUsageCountsToGORM(counts)
	return p.db.WithContext(ctx).Save(gormCounts).Error
}

// Conversion functions

func tagToGORM(tag *v1.Tag) *gormgen.TagGORM {
	var createdAt, updatedAt time.Time
	if tag.CreatedAt != nil {
		createdAt = tag.CreatedAt.AsTime()
	}
	if tag.UpdatedAt != nil {
		updatedAt = tag.UpdatedAt.AsTime()
	}
	return &gormgen.TagGORM{
		Id:              tag.Id,
		Name:            tag.Name,
		NormalizedName:  tag.NormalizedName,
		Value:           tag.Value,
		NormalizedValue: tag.NormalizedValue,
		Type:            tag.Type,
		Color:           tag.Color,
		Description:     tag.Description,
		DisplayOrder:    tag.DisplayOrder,
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

func tagFromGORM(gormTag *gormgen.TagGORM) *v1.Tag {
	var createdAt, updatedAt *timestamppb.Timestamp
	if !gormTag.CreatedAt.IsZero() {
		createdAt = timestamppb.New(gormTag.CreatedAt)
	}
	if !gormTag.UpdatedAt.IsZero() {
		updatedAt = timestamppb.New(gormTag.UpdatedAt)
	}
	return &v1.Tag{
		Id:              gormTag.Id,
		Name:            gormTag.Name,
		NormalizedName:  gormTag.NormalizedName,
		Value:           gormTag.Value,
		NormalizedValue: gormTag.NormalizedValue,
		Type:            gormTag.Type,
		Color:           gormTag.Color,
		Description:     gormTag.Description,
		DisplayOrder:    gormTag.DisplayOrder,
		OwnerId:         gormTag.OwnerId,
		Scope:           gormTag.Scope,
		UsageCount:      gormTag.UsageCount,
		Status:          gormTag.Status,
		RedirectToTagId: gormTag.RedirectToTagId,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
		CreatorId:       gormTag.CreatorId,
	}
}

func entityTagToGORM(et *v1.EntityTag) *gormgen.EntityTagGORM {
	var createdAt time.Time
	if et.CreatedAt != nil {
		createdAt = et.CreatedAt.AsTime()
	}
	return &gormgen.EntityTagGORM{
		TagId:      et.TagId,
		EntityId:   et.EntityId,
		TaggedBy:   et.TaggedBy,
		Visibility: et.Visibility,
		CreatedAt:  createdAt,
	}
}

func entityTagFromGORM(gormET *gormgen.EntityTagGORM) *v1.EntityTag {
	var createdAt *timestamppb.Timestamp
	if !gormET.CreatedAt.IsZero() {
		createdAt = timestamppb.New(gormET.CreatedAt)
	}
	return &v1.EntityTag{
		TagId:      gormET.TagId,
		EntityId:   gormET.EntityId,
		TaggedBy:   gormET.TaggedBy,
		Visibility: gormET.Visibility,
		CreatedAt:  createdAt,
	}
}

func tagUsageCountsToGORM(counts *v1.TagUsageCounts) *gormgen.TagUsageCountsGORM {
	var updatedAt time.Time
	if counts.UpdatedAt != nil {
		updatedAt = counts.UpdatedAt.AsTime()
	}
	return &gormgen.TagUsageCountsGORM{
		EntityId:   counts.EntityId,
		TotalCount: counts.TotalCount,
		ByName:     counts.ByName,
		UpdatedAt:  updatedAt,
	}
}

func tagUsageCountsFromGORM(gormCounts *gormgen.TagUsageCountsGORM) *v1.TagUsageCounts {
	byName := gormCounts.ByName
	if byName == nil {
		byName = make(map[string]int64)
	}
	var updatedAt *timestamppb.Timestamp
	if !gormCounts.UpdatedAt.IsZero() {
		updatedAt = timestamppb.New(gormCounts.UpdatedAt)
	}
	return &v1.TagUsageCounts{
		EntityId:   gormCounts.EntityId,
		TotalCount: gormCounts.TotalCount,
		ByName:     byName,
		UpdatedAt:  updatedAt,
	}
}
