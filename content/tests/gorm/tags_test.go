// Package gorm provides integration tests for content services using GORM/SQL backends.
package gorm

import (
	"context"
	"fmt"
	"testing"

	v1 "github.com/panyam/goapplib/content/gen/go/tags/v1"
	"github.com/panyam/goapplib/content/services/tags"
	"github.com/panyam/goapplib/content/services/tags/backends"
)

// setupTagsService creates a tags service with auto-migration.
func setupTagsService(t *testing.T) *backends.GORMTagsService {
	db := setupTestDB(t)
	service := backends.NewGORMTagsService(db)
	if err := service.AutoMigrate(); err != nil {
		t.Fatalf("Failed to auto-migrate: %v", err)
	}
	return service
}

// TestTagsService_CreateTag tests creating tags.
func TestTagsService_CreateTag(t *testing.T) {
	service := setupTagsService(t)
	ctx := context.Background()

	// Create a pure tag (no name, just value)
	resp, err := service.CreateTag(ctx, &v1.CreateTagRequest{
		Value:   "Favorites",
		OwnerId: "user-1",
		Scope:   v1.TagScope_TAG_SCOPE_PRIVATE,
	})
	if err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}

	if resp.Tag == nil {
		t.Fatal("Expected tag to be returned")
	}
	if resp.Tag.Value != "Favorites" {
		t.Errorf("Expected value=Favorites, got %s", resp.Tag.Value)
	}
	if resp.Tag.NormalizedValue != "favorites" {
		t.Errorf("Expected normalized_value=favorites, got %s", resp.Tag.NormalizedValue)
	}
	if resp.AlreadyExisted {
		t.Error("Expected AlreadyExisted=false for new tag")
	}

	// Create same tag again - should return existing
	resp2, err := service.CreateTag(ctx, &v1.CreateTagRequest{
		Value:   "FAVORITES", // Different case
		OwnerId: "user-1",
	})
	if err != nil {
		t.Fatalf("CreateTag (duplicate) failed: %v", err)
	}

	if !resp2.AlreadyExisted {
		t.Error("Expected AlreadyExisted=true for duplicate tag")
	}
	if resp2.Tag.Id != resp.Tag.Id {
		t.Error("Expected same tag ID for duplicate")
	}
}

// TestTagsService_CreateMetadataTag tests creating metadata tags (name-value pairs).
func TestTagsService_CreateMetadataTag(t *testing.T) {
	service := setupTagsService(t)
	ctx := context.Background()

	// Create a metadata tag (name="venue", value="Wembley Stadium")
	resp, err := service.CreateTag(ctx, &v1.CreateTagRequest{
		Name:    "venue",
		Value:   "Wembley Stadium",
		OwnerId: "user-1",
		Scope:   v1.TagScope_TAG_SCOPE_PRIVATE,
	})
	if err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}

	if resp.Tag.Name != "venue" {
		t.Errorf("Expected name=venue, got %s", resp.Tag.Name)
	}
	if resp.Tag.NormalizedName != "venue" {
		t.Errorf("Expected normalized_name=venue, got %s", resp.Tag.NormalizedName)
	}
	if resp.Tag.Value != "Wembley Stadium" {
		t.Errorf("Expected value=Wembley Stadium, got %s", resp.Tag.Value)
	}
}

// TestTagsService_TagEntity tests tagging entities.
func TestTagsService_TagEntity(t *testing.T) {
	service := setupTagsService(t)
	ctx := context.Background()

	// Create a tag first
	createResp, err := service.CreateTag(ctx, &v1.CreateTagRequest{
		Value:   "Rock",
		OwnerId: "user-1",
	})
	if err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}
	tagID := createResp.Tag.Id

	// Tag an entity
	resp, err := service.TagEntity(ctx, &v1.TagEntityRequest{
		EntityId: "song-1",
		TagId:    tagID,
		TaggedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("TagEntity failed: %v", err)
	}

	if !resp.NewlyTagged {
		t.Error("Expected NewlyTagged=true")
	}
	if resp.EntityTag == nil {
		t.Fatal("Expected EntityTag to be returned")
	}
	if resp.EntityTag.TagId != tagID {
		t.Errorf("Expected TagId=%s, got %s", tagID, resp.EntityTag.TagId)
	}

	// Tag same entity again - should not be newly tagged
	resp2, err := service.TagEntity(ctx, &v1.TagEntityRequest{
		EntityId: "song-1",
		TagId:    tagID,
		TaggedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("TagEntity (duplicate) failed: %v", err)
	}

	if resp2.NewlyTagged {
		t.Error("Expected NewlyTagged=false for duplicate")
	}
}

// TestTagsService_TagEntityInline tests inline tag creation during tagging.
func TestTagsService_TagEntityInline(t *testing.T) {
	service := setupTagsService(t)
	ctx := context.Background()

	// Tag an entity with inline tag creation
	resp, err := service.TagEntity(ctx, &v1.TagEntityRequest{
		EntityId: "book-1",
		Value:    "To Read",
		OwnerId:  "user-1",
		TaggedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("TagEntity failed: %v", err)
	}

	if !resp.NewlyTagged {
		t.Error("Expected NewlyTagged=true")
	}
	if resp.Tag == nil {
		t.Fatal("Expected Tag to be returned")
	}
	if resp.Tag.Value != "To Read" {
		t.Errorf("Expected tag value='To Read', got %s", resp.Tag.Value)
	}
}

// TestTagsService_UntagEntity tests removing tags from entities.
func TestTagsService_UntagEntity(t *testing.T) {
	service := setupTagsService(t)
	ctx := context.Background()

	// Create and apply a tag
	tagResp, err := service.TagEntity(ctx, &v1.TagEntityRequest{
		EntityId: "post-1",
		Value:    "Important",
		OwnerId:  "user-1",
		TaggedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("TagEntity failed: %v", err)
	}

	// Remove the tag
	resp, err := service.UntagEntity(ctx, &v1.UntagEntityRequest{
		EntityId: "post-1",
		TagId:    tagResp.Tag.Id,
		TaggedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("UntagEntity failed: %v", err)
	}

	if !resp.Removed {
		t.Error("Expected Removed=true")
	}

	// Try to remove again - should return false
	resp2, err := service.UntagEntity(ctx, &v1.UntagEntityRequest{
		EntityId: "post-1",
		TagId:    tagResp.Tag.Id,
		TaggedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("UntagEntity (second time) failed: %v", err)
	}

	if resp2.Removed {
		t.Error("Expected Removed=false for non-existent entity tag")
	}
}

// TestTagsService_GetEntityTags tests getting all tags for an entity.
func TestTagsService_GetEntityTags(t *testing.T) {
	service := setupTagsService(t)
	ctx := context.Background()

	// Tag an entity with multiple tags
	_, err := service.TagEntity(ctx, &v1.TagEntityRequest{
		EntityId: "album-1",
		Value:    "Rock",
		OwnerId:  "user-1",
		TaggedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("TagEntity failed: %v", err)
	}

	_, err = service.TagEntity(ctx, &v1.TagEntityRequest{
		EntityId: "album-1",
		Value:    "Classic",
		OwnerId:  "user-1",
		TaggedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("TagEntity failed: %v", err)
	}

	// Get tags for entity
	resp, err := service.GetEntityTags(ctx, &v1.GetEntityTagsRequest{
		EntityId: "album-1",
	})
	if err != nil {
		t.Fatalf("GetEntityTags failed: %v", err)
	}

	if len(resp.Tags) != 2 {
		t.Errorf("Expected 2 tags, got %d", len(resp.Tags))
	}
}

// TestTagsService_MultiUserTagging tests that multiple users can tag with the same tag.
func TestTagsService_MultiUserTagging(t *testing.T) {
	service := setupTagsService(t)
	ctx := context.Background()

	// Create a shared tag
	createResp, err := service.CreateTag(ctx, &v1.CreateTagRequest{
		Value:   "Great",
		OwnerId: "org-1",
		Scope:   v1.TagScope_TAG_SCOPE_SHARED,
	})
	if err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}
	tagID := createResp.Tag.Id

	// User 1 tags a book
	resp1, err := service.TagEntity(ctx, &v1.TagEntityRequest{
		EntityId: "book-1",
		TagId:    tagID,
		TaggedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("TagEntity (user-1) failed: %v", err)
	}
	if !resp1.NewlyTagged {
		t.Error("Expected NewlyTagged=true for user-1")
	}

	// User 2 tags the same book with the same tag
	resp2, err := service.TagEntity(ctx, &v1.TagEntityRequest{
		EntityId: "book-1",
		TagId:    tagID,
		TaggedBy: "user-2",
	})
	if err != nil {
		t.Fatalf("TagEntity (user-2) failed: %v", err)
	}
	if !resp2.NewlyTagged {
		t.Error("Expected NewlyTagged=true for user-2")
	}

	// User 1 removes their tag
	_, err = service.UntagEntity(ctx, &v1.UntagEntityRequest{
		EntityId: "book-1",
		TagId:    tagID,
		TaggedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("UntagEntity (user-1) failed: %v", err)
	}

	// User 2's tag should still exist
	tagsResp, err := service.GetEntityTags(ctx, &v1.GetEntityTagsRequest{
		EntityId: "book-1",
	})
	if err != nil {
		t.Fatalf("GetEntityTags failed: %v", err)
	}

	if len(tagsResp.Tags) != 1 {
		t.Errorf("Expected 1 tag (from user-2), got %d", len(tagsResp.Tags))
	}
}

// TestTagsService_UsageCount tests that usage counts are updated correctly.
func TestTagsService_UsageCount(t *testing.T) {
	service := setupTagsService(t)
	ctx := context.Background()

	// Create a tag
	createResp, err := service.CreateTag(ctx, &v1.CreateTagRequest{
		Value:   "Popular",
		OwnerId: "user-1",
	})
	if err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}
	tagID := createResp.Tag.Id

	// Initial usage count should be 0
	if createResp.Tag.UsageCount != 0 {
		t.Errorf("Expected initial usage_count=0, got %d", createResp.Tag.UsageCount)
	}

	// Tag multiple entities
	for i := 1; i <= 3; i++ {
		_, err := service.TagEntity(ctx, &v1.TagEntityRequest{
			EntityId: fmt.Sprintf("item-%d", i),
			TagId:    tagID,
			TaggedBy: "user-1",
		})
		if err != nil {
			t.Fatalf("TagEntity failed: %v", err)
		}
	}

	// Get tag and verify usage count
	getResp, err := service.GetTag(ctx, &v1.GetTagRequest{Id: tagID})
	if err != nil {
		t.Fatalf("GetTag failed: %v", err)
	}

	if getResp.Tag.UsageCount != 3 {
		t.Errorf("Expected usage_count=3, got %d", getResp.Tag.UsageCount)
	}

	// Untag one entity
	_, err = service.UntagEntity(ctx, &v1.UntagEntityRequest{
		EntityId: "item-1",
		TagId:    tagID,
		TaggedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("UntagEntity failed: %v", err)
	}

	// Verify usage count decreased
	getResp2, err := service.GetTag(ctx, &v1.GetTagRequest{Id: tagID})
	if err != nil {
		t.Fatalf("GetTag failed: %v", err)
	}

	if getResp2.Tag.UsageCount != 2 {
		t.Errorf("Expected usage_count=2, got %d", getResp2.Tag.UsageCount)
	}
}

// TestTagsService_SearchTags tests tag search functionality.
func TestTagsService_SearchTags(t *testing.T) {
	service := setupTagsService(t)
	ctx := context.Background()

	// Create several tags
	tags := []string{"Rock", "Rockabilly", "Roll", "Pop"}
	for _, tag := range tags {
		_, err := service.CreateTag(ctx, &v1.CreateTagRequest{
			Value:   tag,
			OwnerId: "user-1",
		})
		if err != nil {
			t.Fatalf("CreateTag failed: %v", err)
		}
	}

	// Search for "rock" prefix
	resp, err := service.SearchTags(ctx, &v1.SearchTagsRequest{
		Query:   "rock",
		OwnerId: "user-1",
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("SearchTags failed: %v", err)
	}

	// Should find "Rock" and "Rockabilly"
	if len(resp.Tags) < 2 {
		t.Errorf("Expected at least 2 results, got %d", len(resp.Tags))
	}
}

// TestTagsService_ListTags tests listing tags with filtering.
func TestTagsService_ListTags(t *testing.T) {
	service := setupTagsService(t)
	ctx := context.Background()

	// Create tags for different owners
	_, err := service.CreateTag(ctx, &v1.CreateTagRequest{
		Value:   "User1Tag",
		OwnerId: "user-1",
	})
	if err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}

	_, err = service.CreateTag(ctx, &v1.CreateTagRequest{
		Value:   "User2Tag",
		OwnerId: "user-2",
	})
	if err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}

	// List tags for user-1 only
	resp, err := service.ListTags(ctx, &v1.ListTagsRequest{
		OwnerId: "user-1",
	})
	if err != nil {
		t.Fatalf("ListTags failed: %v", err)
	}

	if len(resp.Tags) != 1 {
		t.Errorf("Expected 1 tag for user-1, got %d", len(resp.Tags))
	}
	if resp.Tags[0].Value != "User1Tag" {
		t.Errorf("Expected User1Tag, got %s", resp.Tags[0].Value)
	}
}

// ========== Hooks and Context Tests ==========

// TestTagsService_UserIDFromContext tests that user ID is resolved from context.
func TestTagsService_UserIDFromContext(t *testing.T) {
	db := setupTestDB(t)

	// Use a custom context key
	type myCtxKey string
	const userKey myCtxKey = "my_user_id"

	service := backends.NewGORMTagsServiceWithOptions(db,
		backends.WithUserIDContextKey(userKey),
	)
	if err := service.AutoMigrate(); err != nil {
		t.Fatalf("Failed to auto-migrate: %v", err)
	}

	// Set user ID in context
	ctx := context.WithValue(context.Background(), userKey, "user-from-context")

	// Create tag without specifying OwnerId in request
	resp, err := service.CreateTag(ctx, &v1.CreateTagRequest{
		Value: "ContextTag",
		// OwnerId intentionally omitted - should come from context
	})
	if err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}

	if resp.Tag.OwnerId != "user-from-context" {
		t.Errorf("Expected OwnerId=user-from-context, got %s", resp.Tag.OwnerId)
	}
}

// TestTagsService_OnAuthorizeHook tests the authorization hook.
func TestTagsService_OnAuthorizeHook(t *testing.T) {
	db := setupTestDB(t)

	authCalled := false
	authDenied := false

	service := backends.NewGORMTagsServiceWithOptions(db,
		backends.WithOnAuthorize(func(ctx context.Context, hookCtx *backends.HookContext) error {
			authCalled = true
			if hookCtx.Operation != "CreateTag" {
				t.Errorf("Expected operation=CreateTag, got %s", hookCtx.Operation)
			}
			if authDenied {
				return fmt.Errorf("access denied")
			}
			return nil
		}),
	)
	if err := service.AutoMigrate(); err != nil {
		t.Fatalf("Failed to auto-migrate: %v", err)
	}

	ctx := context.Background()

	// Test successful authorization
	_, err := service.CreateTag(ctx, &v1.CreateTagRequest{
		Value:   "AuthTest",
		OwnerId: "user-1",
	})
	if err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}
	if !authCalled {
		t.Error("Expected OnAuthorize hook to be called")
	}

	// Test authorization denial
	authCalled = false
	authDenied = true
	_, err = service.CreateTag(ctx, &v1.CreateTagRequest{
		Value:   "Denied",
		OwnerId: "user-1",
	})
	if err == nil {
		t.Error("Expected CreateTag to fail when authorization denied")
	}
	if !authCalled {
		t.Error("Expected OnAuthorize hook to be called")
	}
}

// TestTagsService_ValidateEntityHook tests the entity validation hook.
func TestTagsService_ValidateEntityHook(t *testing.T) {
	db := setupTestDB(t)

	validEntities := map[string]bool{
		"book-1": true,
		"book-2": true,
	}

	service := backends.NewGORMTagsServiceWithOptions(db,
		backends.WithValidateEntity(func(ctx context.Context, entityID string) error {
			if !validEntities[entityID] {
				return fmt.Errorf("entity not found: %s", entityID)
			}
			return nil
		}),
	)
	if err := service.AutoMigrate(); err != nil {
		t.Fatalf("Failed to auto-migrate: %v", err)
	}

	ctx := context.Background()

	// Test valid entity
	_, err := service.TagEntity(ctx, &v1.TagEntityRequest{
		EntityId: "book-1",
		Value:    "Fiction",
		OwnerId:  "user-1",
		TaggedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("TagEntity failed for valid entity: %v", err)
	}

	// Test invalid entity
	_, err = service.TagEntity(ctx, &v1.TagEntityRequest{
		EntityId: "invalid-book",
		Value:    "Fiction",
		OwnerId:  "user-1",
		TaggedBy: "user-1",
	})
	if err == nil {
		t.Error("Expected TagEntity to fail for invalid entity")
	}
}

// TestTagsService_BeforeAfterTagSaveHooks tests the tag save lifecycle hooks.
func TestTagsService_BeforeAfterTagSaveHooks(t *testing.T) {
	db := setupTestDB(t)

	beforeSaveCalled := false
	afterSaveCalled := false
	var savedTagID string

	service := backends.NewGORMTagsServiceWithOptions(db,
		backends.WithBeforeTagSave(func(ctx context.Context, tag *v1.Tag) error {
			beforeSaveCalled = true
			if tag.Value == "" {
				return fmt.Errorf("value should be set")
			}
			return nil
		}),
		backends.WithAfterTagSave(func(ctx context.Context, tag *v1.Tag) error {
			afterSaveCalled = true
			savedTagID = tag.Id
			return nil
		}),
	)
	if err := service.AutoMigrate(); err != nil {
		t.Fatalf("Failed to auto-migrate: %v", err)
	}

	ctx := context.Background()

	resp, err := service.CreateTag(ctx, &v1.CreateTagRequest{
		Value:   "HookTest",
		OwnerId: "user-1",
	})
	if err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}

	if !beforeSaveCalled {
		t.Error("Expected BeforeTagSave hook to be called")
	}
	if !afterSaveCalled {
		t.Error("Expected AfterTagSave hook to be called")
	}
	if savedTagID != resp.Tag.Id {
		t.Errorf("Expected savedTagID=%s, got %s", resp.Tag.Id, savedTagID)
	}
}

// TestTagsService_BeforeAfterEntityTagSaveHooks tests the entity tag save lifecycle hooks.
func TestTagsService_BeforeAfterEntityTagSaveHooks(t *testing.T) {
	db := setupTestDB(t)

	beforeSaveCalled := false
	afterSaveCalled := false
	var savedEntityID string

	service := backends.NewGORMTagsServiceWithOptions(db,
		backends.WithBeforeEntityTagSave(func(ctx context.Context, entityTag *v1.EntityTag, tag *v1.Tag) error {
			beforeSaveCalled = true
			if entityTag.EntityId == "" {
				return fmt.Errorf("entity_id should be set")
			}
			return nil
		}),
		backends.WithAfterEntityTagSave(func(ctx context.Context, entityTag *v1.EntityTag, tag *v1.Tag) error {
			afterSaveCalled = true
			savedEntityID = entityTag.EntityId
			return nil
		}),
	)
	if err := service.AutoMigrate(); err != nil {
		t.Fatalf("Failed to auto-migrate: %v", err)
	}

	ctx := context.Background()

	resp, err := service.TagEntity(ctx, &v1.TagEntityRequest{
		EntityId: "song-1",
		Value:    "Jazz",
		OwnerId:  "user-1",
		TaggedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("TagEntity failed: %v", err)
	}

	if !beforeSaveCalled {
		t.Error("Expected BeforeEntityTagSave hook to be called")
	}
	if !afterSaveCalled {
		t.Error("Expected AfterEntityTagSave hook to be called")
	}
	if savedEntityID != resp.EntityTag.EntityId {
		t.Errorf("Expected savedEntityID=%s, got %s", resp.EntityTag.EntityId, savedEntityID)
	}
}

// TestTagsService_BeforeAfterDeleteHooks tests the delete lifecycle hooks.
func TestTagsService_BeforeAfterDeleteHooks(t *testing.T) {
	db := setupTestDB(t)

	beforeDeleteCalled := false
	afterDeleteCalled := false
	var deletedTagValue string

	service := backends.NewGORMTagsServiceWithOptions(db,
		backends.WithBeforeTagDelete(func(ctx context.Context, tag *v1.Tag) error {
			beforeDeleteCalled = true
			return nil
		}),
		backends.WithAfterTagDelete(func(ctx context.Context, tag *v1.Tag) error {
			afterDeleteCalled = true
			deletedTagValue = tag.Value
			return nil
		}),
	)
	if err := service.AutoMigrate(); err != nil {
		t.Fatalf("Failed to auto-migrate: %v", err)
	}

	ctx := context.Background()

	// First create a tag
	createResp, err := service.CreateTag(ctx, &v1.CreateTagRequest{
		Value:   "ToDelete",
		OwnerId: "user-1",
	})
	if err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}

	// Now delete it
	_, err = service.DeleteTag(ctx, &v1.DeleteTagRequest{
		Id: createResp.Tag.Id,
	})
	if err != nil {
		t.Fatalf("DeleteTag failed: %v", err)
	}

	if !beforeDeleteCalled {
		t.Error("Expected BeforeTagDelete hook to be called")
	}
	if !afterDeleteCalled {
		t.Error("Expected AfterTagDelete hook to be called")
	}
	if deletedTagValue != "ToDelete" {
		t.Errorf("Expected deletedTagValue=ToDelete, got %s", deletedTagValue)
	}
}

// TestTagsService_OnEventHook tests the event notification hook.
func TestTagsService_OnEventHook(t *testing.T) {
	db := setupTestDB(t)

	var events []*backends.Event

	service := backends.NewGORMTagsServiceWithOptions(db,
		backends.WithOnEvent(func(ctx context.Context, event *backends.Event) error {
			events = append(events, event)
			return nil
		}),
	)
	if err := service.AutoMigrate(); err != nil {
		t.Fatalf("Failed to auto-migrate: %v", err)
	}

	ctx := context.Background()

	// Create tag - should trigger EventTagCreated
	createResp, err := service.CreateTag(ctx, &v1.CreateTagRequest{
		Value:   "EventTest",
		OwnerId: "user-1",
	})
	if err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}
	if events[0].Type != backends.EventTagCreated {
		t.Errorf("Expected event type=%s, got %s", backends.EventTagCreated, events[0].Type)
	}

	// Tag entity - should trigger EventEntityTagged
	_, err = service.TagEntity(ctx, &v1.TagEntityRequest{
		EntityId: "item-1",
		TagId:    createResp.Tag.Id,
		TaggedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("TagEntity failed: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("Expected 2 events, got %d", len(events))
	}
	if events[1].Type != backends.EventEntityTagged {
		t.Errorf("Expected event type=%s, got %s", backends.EventEntityTagged, events[1].Type)
	}

	// Untag entity - should trigger EventEntityUntagged
	_, err = service.UntagEntity(ctx, &v1.UntagEntityRequest{
		EntityId: "item-1",
		TagId:    createResp.Tag.Id,
		TaggedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("UntagEntity failed: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("Expected 3 events, got %d", len(events))
	}
	if events[2].Type != backends.EventEntityUntagged {
		t.Errorf("Expected event type=%s, got %s", backends.EventEntityUntagged, events[2].Type)
	}

	// Delete tag - should trigger EventTagDeleted
	_, err = service.DeleteTag(ctx, &v1.DeleteTagRequest{
		Id: createResp.Tag.Id,
	})
	if err != nil {
		t.Fatalf("DeleteTag failed: %v", err)
	}

	if len(events) != 4 {
		t.Fatalf("Expected 4 events, got %d", len(events))
	}
	if events[3].Type != backends.EventTagDeleted {
		t.Errorf("Expected event type=%s, got %s", backends.EventTagDeleted, events[3].Type)
	}
}

// TestTagsService_AfterTagsReadHook tests the after read hook for data enrichment.
func TestTagsService_AfterTagsReadHook(t *testing.T) {
	db := setupTestDB(t)

	enrichmentCalled := false

	service := backends.NewGORMTagsServiceWithOptions(db,
		backends.WithAfterTagsRead(func(ctx context.Context, tagsRead []*v1.Tag) error {
			enrichmentCalled = true
			return nil
		}),
	)
	if err := service.AutoMigrate(); err != nil {
		t.Fatalf("Failed to auto-migrate: %v", err)
	}

	ctx := context.Background()

	// Create and tag
	_, err := service.TagEntity(ctx, &v1.TagEntityRequest{
		EntityId: "item-1",
		Value:    "TestRead",
		OwnerId:  "user-1",
		TaggedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("TagEntity failed: %v", err)
	}

	// Get entity tags - should trigger AfterTagsRead
	_, err = service.GetEntityTags(ctx, &v1.GetEntityTagsRequest{
		EntityId: "item-1",
	})
	if err != nil {
		t.Fatalf("GetEntityTags failed: %v", err)
	}

	if !enrichmentCalled {
		t.Error("Expected AfterTagsRead hook to be called")
	}
}

// TestTagsService_HookCanModifyRequest tests that auth hook can modify request.
func TestTagsService_HookCanModifyRequest(t *testing.T) {
	db := setupTestDB(t)

	service := backends.NewGORMTagsServiceWithOptions(db,
		backends.WithOnAuthorize(func(ctx context.Context, hookCtx *tags.HookContext) error {
			// Modify the request to set owner ID from "authenticated" context
			if req, ok := hookCtx.Request.(*v1.CreateTagRequest); ok {
				if req.OwnerId == "" {
					req.OwnerId = "auth-user-456"
				}
			}
			return nil
		}),
	)
	if err := service.AutoMigrate(); err != nil {
		t.Fatalf("Failed to auto-migrate: %v", err)
	}

	ctx := context.Background()

	// Create tag without OwnerId - hook should set it
	resp, err := service.CreateTag(ctx, &v1.CreateTagRequest{
		Value: "AuthModified",
		// OwnerId intentionally empty
	})
	if err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}

	if resp.Tag.OwnerId != "auth-user-456" {
		t.Errorf("Expected OwnerId=auth-user-456, got %s", resp.Tag.OwnerId)
	}
}
