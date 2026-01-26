// Package backends provides concrete storage implementations for the likes service.
package backends

import (
	"context"
	"time"

	"github.com/panyam/goapplib/content/services/likes"

	v1 "github.com/panyam/goapplib/content/gen/go/likes/v1"
	gormgen "github.com/panyam/goapplib/content/gen/gorm/likes/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

// GORMLikesService implements LikesService using GORM.
type GORMLikesService struct {
	*likes.BaseLikesService
	db *gorm.DB
}

// NewGORMLikesService creates a new GORM-backed likes service.
func NewGORMLikesService(db *gorm.DB) *GORMLikesService {
	provider := &gormLikesStorageProvider{db: db}
	base := likes.NewBaseLikesService(provider)
	return &GORMLikesService{
		BaseLikesService: base,
		db:               db,
	}
}

// NewGORMLikesServiceWithOptions creates a new GORM-backed likes service with options.
func NewGORMLikesServiceWithOptions(db *gorm.DB, opts ...likes.ServiceOption) *GORMLikesService {
	provider := &gormLikesStorageProvider{db: db}
	base := likes.NewBaseLikesService(provider, opts...)
	return &GORMLikesService{
		BaseLikesService: base,
		db:               db,
	}
}

// Re-export hook types and options for convenience
type (
	HookContext        = likes.HookContext
	Event              = likes.Event
	EventType          = likes.EventType
	AuthorizeHook      = likes.AuthorizeHook
	ValidateEntityHook = likes.ValidateEntityHook
	BeforeSaveHook     = likes.BeforeSaveHook
	AfterSaveHook      = likes.AfterSaveHook
	BeforeDeleteHook   = likes.BeforeDeleteHook
	AfterDeleteHook    = likes.AfterDeleteHook
	AfterReadHook      = likes.AfterReadHook
	OnEventHook        = likes.OnEventHook
)

// Re-export event types
const (
	EventReactionAdded   = likes.EventReactionAdded
	EventReactionRemoved = likes.EventReactionRemoved
	EventReactionChanged = likes.EventReactionChanged
)

// Re-export option functions
var (
	WithHooks              = likes.WithHooks
	WithOnAuthorize        = likes.WithOnAuthorize
	WithValidateEntity     = likes.WithValidateEntity
	WithBeforeSave         = likes.WithBeforeSave
	WithAfterSave          = likes.WithAfterSave
	WithBeforeDelete       = likes.WithBeforeDelete
	WithAfterDelete        = likes.WithAfterDelete
	WithAfterRead          = likes.WithAfterRead
	WithOnEvent            = likes.WithOnEvent
	WithDefaultReactionType = likes.WithDefaultReactionType
	WithUserIDContextKey   = likes.WithUserIDContextKey
	WithCache              = likes.WithCache
)

// AutoMigrate creates the database tables.
func (s *GORMLikesService) AutoMigrate() error {
	return s.db.AutoMigrate(
		&gormgen.LikeGORM{},
		&gormgen.LikeCountsGORM{},
		&gormgen.ReactionTypeGORM{},
	)
}

// gormLikesStorageProvider implements LikesStorageProvider using GORM.
type gormLikesStorageProvider struct {
	db *gorm.DB
}

// SaveLike saves a like to the database.
func (p *gormLikesStorageProvider) SaveLike(ctx context.Context, like *v1.Like) error {
	gormLike := likeToGORM(like)
	return p.db.WithContext(ctx).Save(gormLike).Error
}

// DeleteLike deletes a like from the database.
func (p *gormLikesStorageProvider) DeleteLike(ctx context.Context, entityID, userID string) error {
	return p.db.WithContext(ctx).
		Where("entity_id = ? AND user_id = ?", entityID, userID).
		Delete(&gormgen.LikeGORM{}).Error
}

// GetLike retrieves a like from the database.
func (p *gormLikesStorageProvider) GetLike(ctx context.Context, entityID, userID string) (*v1.Like, error) {
	var gormLike gormgen.LikeGORM
	err := p.db.WithContext(ctx).
		Where("entity_id = ? AND user_id = ?", entityID, userID).
		First(&gormLike).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return likeFromGORM(&gormLike), nil
}

// ListLikesByEntity lists likes for an entity.
func (p *gormLikesStorageProvider) ListLikesByEntity(ctx context.Context, entityID string, reactionType string, limit, offset int) ([]*v1.Like, int, error) {
	var gormLikes []gormgen.LikeGORM
	var total int64

	query := p.db.WithContext(ctx).Model(&gormgen.LikeGORM{}).
		Where("entity_id = ?", entityID)

	if reactionType != "" {
		query = query.Where("reaction_type = ?", reactionType)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&gormLikes).Error; err != nil {
		return nil, 0, err
	}

	likes := make([]*v1.Like, len(gormLikes))
	for i, gl := range gormLikes {
		likes[i] = likeFromGORM(&gl)
	}

	return likes, int(total), nil
}

// ListLikesByUser lists likes by a user.
func (p *gormLikesStorageProvider) ListLikesByUser(ctx context.Context, userID string, limit, offset int) ([]*v1.Like, int, error) {
	var gormLikes []gormgen.LikeGORM
	var total int64

	query := p.db.WithContext(ctx).Model(&gormgen.LikeGORM{}).
		Where("user_id = ?", userID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&gormLikes).Error; err != nil {
		return nil, 0, err
	}

	likes := make([]*v1.Like, len(gormLikes))
	for i, gl := range gormLikes {
		likes[i] = likeFromGORM(&gl)
	}

	return likes, int(total), nil
}

// GetLikeCounts retrieves like counts for an entity.
func (p *gormLikesStorageProvider) GetLikeCounts(ctx context.Context, entityID string) (*v1.LikeCounts, error) {
	var gormCounts gormgen.LikeCountsGORM
	err := p.db.WithContext(ctx).
		Where("entity_id = ?", entityID).
		First(&gormCounts).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return likeCountsFromGORM(&gormCounts), nil
}

// SaveLikeCounts saves like counts to the database.
func (p *gormLikesStorageProvider) SaveLikeCounts(ctx context.Context, counts *v1.LikeCounts) error {
	gormCounts := likeCountsToGORM(counts)
	return p.db.WithContext(ctx).Save(gormCounts).Error
}

// SaveReactionType saves a reaction type to the database.
func (p *gormLikesStorageProvider) SaveReactionType(ctx context.Context, rt *v1.ReactionType) error {
	gormRT := reactionTypeToGORM(rt)
	return p.db.WithContext(ctx).Save(gormRT).Error
}

// GetReactionType retrieves a reaction type from the database.
func (p *gormLikesStorageProvider) GetReactionType(ctx context.Context, id string) (*v1.ReactionType, error) {
	var gormRT gormgen.ReactionTypeGORM
	err := p.db.WithContext(ctx).Where("id = ?", id).First(&gormRT).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return reactionTypeFromGORM(&gormRT), nil
}

// ListReactionTypes lists all reaction types.
func (p *gormLikesStorageProvider) ListReactionTypes(ctx context.Context) ([]*v1.ReactionType, error) {
	var gormTypes []gormgen.ReactionTypeGORM
	if err := p.db.WithContext(ctx).Order("display_order ASC").Find(&gormTypes).Error; err != nil {
		return nil, err
	}

	types := make([]*v1.ReactionType, len(gormTypes))
	for i, gt := range gormTypes {
		types[i] = reactionTypeFromGORM(&gt)
	}

	return types, nil
}

// Conversion functions

func likeToGORM(like *v1.Like) *gormgen.LikeGORM {
	return &gormgen.LikeGORM{
		Id:           like.Id,
		EntityId:     like.EntityId,
		UserId:       like.UserId,
		ReactionType: like.ReactionType,
		CreatedAt:    like.CreatedAt.AsTime(),
		UpdatedAt:    like.UpdatedAt.AsTime(),
		CreatorId:    like.CreatorId,
	}
}

func likeFromGORM(gormLike *gormgen.LikeGORM) *v1.Like {
	return &v1.Like{
		Id:           gormLike.Id,
		EntityId:     gormLike.EntityId,
		UserId:       gormLike.UserId,
		ReactionType: gormLike.ReactionType,
		CreatedAt:    timestamppb.New(gormLike.CreatedAt),
		UpdatedAt:    timestamppb.New(gormLike.UpdatedAt),
		CreatorId:    gormLike.CreatorId,
	}
}

func likeCountsToGORM(counts *v1.LikeCounts) *gormgen.LikeCountsGORM {
	return &gormgen.LikeCountsGORM{
		EntityId:       counts.EntityId,
		TotalCount:     counts.TotalCount,
		ByReactionType: counts.ByReactionType,
		UpdatedAt:      counts.UpdatedAt.AsTime(),
	}
}

func likeCountsFromGORM(gormCounts *gormgen.LikeCountsGORM) *v1.LikeCounts {
	byType := gormCounts.ByReactionType
	if byType == nil {
		byType = make(map[string]int64)
	}
	var updatedAt *timestamppb.Timestamp
	if !gormCounts.UpdatedAt.IsZero() {
		updatedAt = timestamppb.New(gormCounts.UpdatedAt)
	}
	return &v1.LikeCounts{
		EntityId:       gormCounts.EntityId,
		TotalCount:     gormCounts.TotalCount,
		ByReactionType: byType,
		UpdatedAt:      updatedAt,
	}
}

func reactionTypeToGORM(rt *v1.ReactionType) *gormgen.ReactionTypeGORM {
	var createdAt, updatedAt time.Time
	if rt.CreatedAt != nil {
		createdAt = rt.CreatedAt.AsTime()
	}
	if rt.UpdatedAt != nil {
		updatedAt = rt.UpdatedAt.AsTime()
	}
	return &gormgen.ReactionTypeGORM{
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

func reactionTypeFromGORM(gormRT *gormgen.ReactionTypeGORM) *v1.ReactionType {
	var createdAt, updatedAt *timestamppb.Timestamp
	if !gormRT.CreatedAt.IsZero() {
		createdAt = timestamppb.New(gormRT.CreatedAt)
	}
	if !gormRT.UpdatedAt.IsZero() {
		updatedAt = timestamppb.New(gormRT.UpdatedAt)
	}
	return &v1.ReactionType{
		Id:           gormRT.Id,
		Name:         gormRT.Name,
		Emoji:        gormRT.Emoji,
		IconUrl:      gormRT.IconUrl,
		DisplayOrder: gormRT.DisplayOrder,
		IsDefault:    gormRT.IsDefault,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
		CreatorId:    gormRT.CreatorId,
	}
}
