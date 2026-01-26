package datastore

import (
	"context"
	"fmt"
	"testing"
	"time"

	v1 "github.com/panyam/goapplib/content/gen/go/tags/v1"
	"github.com/panyam/goapplib/content/services/tags/backends"
)

// tagsTestKinds are the Datastore kinds used by the tags service.
var tagsTestKinds = []string{
	"Tag",
	"EntityTag",
	"TagUsageCounts",
}

// setupTagsService creates a tags service for testing.
// Index validation is done once in TestMain, not here.
func setupTagsService(t *testing.T) *backends.DatastoreTagsService {
	client := setupTestClient(t, tagsTestKinds)
	namespace := getTestNamespace()

	service, err := backends.NewDatastoreTagsService(client, namespace)
	if err != nil {
		t.Fatalf("Failed to create tags service: %v", err)
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

	// Use unique IDs to avoid conflicts with other tests
	entityID := fmt.Sprintf("album-%d", time.Now().UnixNano())
	ownerID := fmt.Sprintf("user-%d", time.Now().UnixNano())

	// Tag an entity with multiple tags
	_, err := service.TagEntity(ctx, &v1.TagEntityRequest{
		EntityId: entityID,
		Value:    "Rock",
		OwnerId:  ownerID,
		TaggedBy: ownerID,
	})
	if err != nil {
		t.Fatalf("TagEntity failed: %v", err)
	}

	_, err = service.TagEntity(ctx, &v1.TagEntityRequest{
		EntityId: entityID,
		Value:    "Classic",
		OwnerId:  ownerID,
		TaggedBy: ownerID,
	})
	if err != nil {
		t.Fatalf("TagEntity failed: %v", err)
	}

	// Get tags for entity
	resp, err := service.GetEntityTags(ctx, &v1.GetEntityTagsRequest{
		EntityId: entityID,
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
