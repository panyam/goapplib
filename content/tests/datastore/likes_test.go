package datastore

import (
	"context"
	"fmt"
	"testing"

	commonv1 "github.com/panyam/goapplib/content/gen/go/common/v1"
	v1 "github.com/panyam/goapplib/content/gen/go/likes/v1"
	"github.com/panyam/goapplib/content/services/likes/backends"
)

// likesTestKinds are the Datastore kinds used by the likes service.
var likesTestKinds = []string{
	"Like",
	"LikeCounts",
	"ReactionType",
}

// setupLikesService creates a likes service for testing.
// Index validation is done once in TestMain, not here.
func setupLikesService(t *testing.T) *backends.DatastoreLikesService {
	client := setupTestClient(t, likesTestKinds)
	namespace := getTestNamespace()

	service, err := backends.NewDatastoreLikesService(client, namespace)
	if err != nil {
		t.Fatalf("Failed to create likes service: %v", err)
	}

	return service
}

// TestLikesService_AddReaction tests adding reactions.
func TestLikesService_AddReaction(t *testing.T) {
	service := setupLikesService(t)
	ctx := context.Background()

	resp, err := service.AddReaction(ctx, &v1.AddReactionRequest{
		EntityType:   "post",
		EntityId:     "post-1",
		UserId:       "user-1",
		ReactionType: "like",
	})
	if err != nil {
		t.Fatalf("AddReaction failed: %v", err)
	}

	if resp.Like == nil {
		t.Fatal("Expected like to be returned")
	}
	if resp.Like.EntityType != "post" {
		t.Errorf("Expected entity_type=post, got %s", resp.Like.EntityType)
	}
	if resp.Like.ReactionType != "like" {
		t.Errorf("Expected reaction_type=like, got %s", resp.Like.ReactionType)
	}

	if resp.Counts == nil {
		t.Fatal("Expected counts to be returned")
	}
	if resp.Counts.TotalCount != 1 {
		t.Errorf("Expected total_count=1, got %d", resp.Counts.TotalCount)
	}
}

// TestLikesService_ToggleReaction tests toggling reactions.
func TestLikesService_ToggleReaction(t *testing.T) {
	service := setupLikesService(t)
	ctx := context.Background()

	// Toggle on
	resp, err := service.ToggleReaction(ctx, &v1.ToggleReactionRequest{
		EntityType:   "post",
		EntityId:     "post-1",
		UserId:       "user-1",
		ReactionType: "like",
	})
	if err != nil {
		t.Fatalf("ToggleReaction failed: %v", err)
	}
	if !resp.Added {
		t.Error("Expected added=true")
	}

	// Toggle off
	resp, err = service.ToggleReaction(ctx, &v1.ToggleReactionRequest{
		EntityType:   "post",
		EntityId:     "post-1",
		UserId:       "user-1",
		ReactionType: "like",
	})
	if err != nil {
		t.Fatalf("ToggleReaction failed: %v", err)
	}
	if resp.Added {
		t.Error("Expected added=false")
	}
}

// TestLikesService_GetUserReaction tests getting a user's reaction.
func TestLikesService_GetUserReaction(t *testing.T) {
	service := setupLikesService(t)
	ctx := context.Background()

	// Check before adding
	resp, err := service.GetUserReaction(ctx, &v1.GetUserReactionRequest{
		EntityType: "post",
		EntityId:   "post-1",
		UserId:     "user-1",
	})
	if err != nil {
		t.Fatalf("GetUserReaction failed: %v", err)
	}
	if resp.Like != nil {
		t.Error("Expected no reaction before adding")
	}

	// Add reaction
	_, err = service.AddReaction(ctx, &v1.AddReactionRequest{
		EntityType:   "post",
		EntityId:     "post-1",
		UserId:       "user-1",
		ReactionType: "love",
	})
	if err != nil {
		t.Fatalf("AddReaction failed: %v", err)
	}

	// Check after adding
	resp, err = service.GetUserReaction(ctx, &v1.GetUserReactionRequest{
		EntityType: "post",
		EntityId:   "post-1",
		UserId:     "user-1",
	})
	if err != nil {
		t.Fatalf("GetUserReaction failed: %v", err)
	}
	if resp.Like == nil {
		t.Fatal("Expected reaction after adding")
	}
	if resp.Like.ReactionType != "love" {
		t.Errorf("Expected reaction_type=love, got %s", resp.Like.ReactionType)
	}
}

// TestLikesService_BatchGetLikeCounts tests batch getting counts.
func TestLikesService_BatchGetLikeCounts(t *testing.T) {
	service := setupLikesService(t)
	ctx := context.Background()

	// Add reactions to multiple entities
	entities := []struct {
		entityType string
		entityID   string
		count      int
	}{
		{"post", "post-1", 3},
		{"post", "post-2", 2},
	}

	for _, e := range entities {
		for i := 0; i < e.count; i++ {
			_, err := service.AddReaction(ctx, &v1.AddReactionRequest{
				EntityType:   e.entityType,
				EntityId:     e.entityID,
				UserId:       fmt.Sprintf("user-%s-%d", e.entityID, i),
				ReactionType: "like",
			})
			if err != nil {
				t.Fatalf("AddReaction failed: %v", err)
			}
		}
	}

	// Batch get counts
	resp, err := service.BatchGetLikeCounts(ctx, &v1.BatchGetLikeCountsRequest{
		Entities: []*commonv1.EntityRef{
			{EntityType: "post", EntityId: "post-1"},
			{EntityType: "post", EntityId: "post-2"},
		},
	})
	if err != nil {
		t.Fatalf("BatchGetLikeCounts failed: %v", err)
	}

	if counts, ok := resp.Counts["post:post-1"]; ok {
		if counts.TotalCount != 3 {
			t.Errorf("Expected post:post-1 count=3, got %d", counts.TotalCount)
		}
	} else {
		t.Error("Missing counts for post:post-1")
	}

	if counts, ok := resp.Counts["post:post-2"]; ok {
		if counts.TotalCount != 2 {
			t.Errorf("Expected post:post-2 count=2, got %d", counts.TotalCount)
		}
	} else {
		t.Error("Missing counts for post:post-2")
	}
}
