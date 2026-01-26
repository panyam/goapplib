// Package gorm provides integration tests for content services using GORM/SQL backends.
package gorm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/panyam/goapplib/content/gen/go/likes/v1"
	"github.com/panyam/goapplib/content/services/likes/backends"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupTestDB creates a database connection for testing.
// If CONTENT_TEST_PGDB environment variable is set, it connects to PostgreSQL.
// Otherwise, it creates a temporary SQLite database.
func setupTestDB(t *testing.T) *gorm.DB {
	pgDB := os.Getenv("CONTENT_TEST_PGDB")
	if pgDB != "" {
		return setupPostgresDB(t, pgDB)
	}
	return setupSQLiteDB(t)
}

// setupSQLiteDB creates a temporary SQLite database for testing
func setupSQLiteDB(t *testing.T) *gorm.DB {
	f, err := os.CreateTemp(os.TempDir(), "content_test_"+t.Name()+"_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp db file: %v", err)
	}
	tmpFile := f.Name()
	f.Close()

	t.Cleanup(func() {
		os.Remove(tmpFile)
	})

	db, err := gorm.Open(sqlite.Open(tmpFile), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to open SQLite database: %v", err)
	}

	return db
}

// setupPostgresDB connects to a PostgreSQL database for testing.
func setupPostgresDB(t *testing.T, dbName string) *gorm.DB {
	host := os.Getenv("CONTENT_TEST_PGHOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("CONTENT_TEST_PGPORT")
	if port == "" {
		port = "5432"
	}
	user := os.Getenv("CONTENT_TEST_PGUSER")
	if user == "" {
		user = "postgres"
	}
	password := os.Getenv("CONTENT_TEST_PGPASSWORD")

	dsn := fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=disable",
		host, port, user, dbName)
	if password != "" {
		dsn += " password=" + password
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to connect to PostgreSQL: %v", err)
	}

	// Create a unique test schema
	schemaName := "test_" + sanitizeSchemaName(t.Name())

	if err := db.Exec(fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", schemaName)).Error; err != nil {
		t.Fatalf("Failed to create test schema: %v", err)
	}

	if err := db.Exec(fmt.Sprintf("SET search_path TO %s", schemaName)).Error; err != nil {
		t.Fatalf("Failed to set search_path: %v", err)
	}

	t.Cleanup(func() {
		db.Exec(fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schemaName))
	})

	return db
}

func sanitizeSchemaName(name string) string {
	result := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			result = append(result, c)
		} else {
			result = append(result, '_')
		}
	}
	if len(result) > 0 && result[0] >= '0' && result[0] <= '9' {
		result = append([]byte("s_"), result...)
	}
	if len(result) > 60 {
		result = result[:60]
	}
	return string(result)
}

// setupLikesService creates a likes service with auto-migration.
func setupLikesService(t *testing.T) *backends.GORMLikesService {
	db := setupTestDB(t)
	service := backends.NewGORMLikesService(db)
	if err := service.AutoMigrate(); err != nil {
		t.Fatalf("Failed to auto-migrate: %v", err)
	}
	return service
}

// TestLikesService_AddReaction tests adding reactions.
func TestLikesService_AddReaction(t *testing.T) {
	service := setupLikesService(t)
	ctx := context.Background()

	// Add a reaction
	resp, err := service.AddReaction(ctx, &v1.AddReactionRequest{
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
	if resp.Like.ReactionType != "like" {
		t.Errorf("Expected reaction_type=like, got %s", resp.Like.ReactionType)
	}

	// Verify counts updated
	if resp.Counts == nil {
		t.Fatal("Expected counts to be returned")
	}
	if resp.Counts.TotalCount != 1 {
		t.Errorf("Expected total_count=1, got %d", resp.Counts.TotalCount)
	}
	if resp.Counts.ByReactionType["like"] != 1 {
		t.Errorf("Expected by_reaction_type[like]=1, got %d", resp.Counts.ByReactionType["like"])
	}
}

// TestLikesService_ChangeReaction tests changing a reaction type.
func TestLikesService_ChangeReaction(t *testing.T) {
	service := setupLikesService(t)
	ctx := context.Background()

	// Add initial reaction
	_, err := service.AddReaction(ctx, &v1.AddReactionRequest{
		EntityId:     "post-1",
		UserId:       "user-1",
		ReactionType: "like",
	})
	if err != nil {
		t.Fatalf("AddReaction failed: %v", err)
	}

	// Change reaction type
	resp, err := service.AddReaction(ctx, &v1.AddReactionRequest{
		EntityId:     "post-1",
		UserId:       "user-1",
		ReactionType: "love",
	})
	if err != nil {
		t.Fatalf("AddReaction (change) failed: %v", err)
	}

	// Verify reaction changed
	if resp.Like.ReactionType != "love" {
		t.Errorf("Expected reaction_type=love, got %s", resp.Like.ReactionType)
	}

	// Verify counts (should still be 1 total)
	if resp.Counts.TotalCount != 1 {
		t.Errorf("Expected total_count=1, got %d", resp.Counts.TotalCount)
	}
	if resp.Counts.ByReactionType["love"] != 1 {
		t.Errorf("Expected by_reaction_type[love]=1, got %d", resp.Counts.ByReactionType["love"])
	}
	if _, exists := resp.Counts.ByReactionType["like"]; exists {
		t.Error("Expected like count to be removed")
	}
}

// TestLikesService_RemoveReaction tests removing reactions.
func TestLikesService_RemoveReaction(t *testing.T) {
	service := setupLikesService(t)
	ctx := context.Background()

	// Add a reaction
	_, err := service.AddReaction(ctx, &v1.AddReactionRequest{
		EntityId:     "post-1",
		UserId:       "user-1",
		ReactionType: "like",
	})
	if err != nil {
		t.Fatalf("AddReaction failed: %v", err)
	}

	// Remove reaction
	resp, err := service.RemoveReaction(ctx, &v1.RemoveReactionRequest{
		EntityId: "post-1",
		UserId:   "user-1",
	})
	if err != nil {
		t.Fatalf("RemoveReaction failed: %v", err)
	}

	if !resp.Removed {
		t.Error("Expected removed=true")
	}

	// Verify counts
	if resp.Counts.TotalCount != 0 {
		t.Errorf("Expected total_count=0, got %d", resp.Counts.TotalCount)
	}
}

// TestLikesService_ToggleReaction tests toggling reactions.
func TestLikesService_ToggleReaction(t *testing.T) {
	service := setupLikesService(t)
	ctx := context.Background()

	// Toggle on
	resp, err := service.ToggleReaction(ctx, &v1.ToggleReactionRequest{
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
	if resp.Counts.TotalCount != 1 {
		t.Errorf("Expected total_count=1, got %d", resp.Counts.TotalCount)
	}

	// Toggle off
	resp, err = service.ToggleReaction(ctx, &v1.ToggleReactionRequest{
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
	if resp.Counts.TotalCount != 0 {
		t.Errorf("Expected total_count=0, got %d", resp.Counts.TotalCount)
	}
}

// TestLikesService_GetUserReaction tests getting a user's reaction.
func TestLikesService_GetUserReaction(t *testing.T) {
	service := setupLikesService(t)
	ctx := context.Background()

	// Check before adding
	resp, err := service.GetUserReaction(ctx, &v1.GetUserReactionRequest{
		EntityId: "post-1",
		UserId:   "user-1",
	})
	if err != nil {
		t.Fatalf("GetUserReaction failed: %v", err)
	}
	if resp.Like != nil {
		t.Error("Expected no reaction before adding")
	}

	// Add reaction
	_, err = service.AddReaction(ctx, &v1.AddReactionRequest{
		EntityId:     "post-1",
		UserId:       "user-1",
		ReactionType: "love",
	})
	if err != nil {
		t.Fatalf("AddReaction failed: %v", err)
	}

	// Check after adding
	resp, err = service.GetUserReaction(ctx, &v1.GetUserReactionRequest{
		EntityId: "post-1",
		UserId:   "user-1",
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

// TestLikesService_ListReactors tests listing reactors.
func TestLikesService_ListReactors(t *testing.T) {
	service := setupLikesService(t)
	ctx := context.Background()

	// Add multiple reactions
	for i := 1; i <= 5; i++ {
		_, err := service.AddReaction(ctx, &v1.AddReactionRequest{
			EntityId:     "post-1",
			UserId:       fmt.Sprintf("user-%d", i),
			ReactionType: "like",
		})
		if err != nil {
			t.Fatalf("AddReaction failed: %v", err)
		}
	}

	// List reactors
	resp, err := service.ListReactors(ctx, &v1.ListReactorsRequest{
		EntityId: "post-1",
	})
	if err != nil {
		t.Fatalf("ListReactors failed: %v", err)
	}

	if len(resp.Likes) != 5 {
		t.Errorf("Expected 5 likes, got %d", len(resp.Likes))
	}
	if resp.Pagination.TotalCount != 5 {
		t.Errorf("Expected total_count=5, got %d", resp.Pagination.TotalCount)
	}
}

// TestLikesService_BatchGetLikeCounts tests batch getting counts.
func TestLikesService_BatchGetLikeCounts(t *testing.T) {
	service := setupLikesService(t)
	ctx := context.Background()

	// Add reactions to multiple entities
	entities := []struct {
		entityID string
		count    int
	}{
		{"post-1", 3},
		{"post-2", 5},
		{"comment-1", 2},
	}

	for _, e := range entities {
		for i := 0; i < e.count; i++ {
			_, err := service.AddReaction(ctx, &v1.AddReactionRequest{
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
		EntityIds: []string{
			"post-1",
			"post-2",
			"comment-1",
			"nonexistent",
		},
	})
	if err != nil {
		t.Fatalf("BatchGetLikeCounts failed: %v", err)
	}

	// Verify counts
	if counts, ok := resp.Counts["post-1"]; ok {
		if counts.TotalCount != 3 {
			t.Errorf("Expected post-1 count=3, got %d", counts.TotalCount)
		}
	} else {
		t.Error("Missing counts for post-1")
	}

	if counts, ok := resp.Counts["post-2"]; ok {
		if counts.TotalCount != 5 {
			t.Errorf("Expected post-2 count=5, got %d", counts.TotalCount)
		}
	} else {
		t.Error("Missing counts for post-2")
	}
}

// TestLikesService_ReactionTypes tests reaction type management.
func TestLikesService_ReactionTypes(t *testing.T) {
	service := setupLikesService(t)
	ctx := context.Background()

	// Create reaction types
	types := []*v1.ReactionType{
		{Id: "like", Name: "Like", Emoji: "\U0001F44D", DisplayOrder: 1, IsDefault: true},
		{Id: "love", Name: "Love", Emoji: "\u2764\uFE0F", DisplayOrder: 2},
		{Id: "celebrate", Name: "Celebrate", Emoji: "\U0001F389", DisplayOrder: 3},
	}

	for _, rt := range types {
		_, err := service.CreateReactionType(ctx, &v1.CreateReactionTypeRequest{
			ReactionType: rt,
		})
		if err != nil {
			t.Fatalf("CreateReactionType failed: %v", err)
		}
	}

	// List reaction types
	resp, err := service.ListReactionTypes(ctx, &v1.ListReactionTypesRequest{})
	if err != nil {
		t.Fatalf("ListReactionTypes failed: %v", err)
	}

	if len(resp.ReactionTypes) != 3 {
		t.Errorf("Expected 3 reaction types, got %d", len(resp.ReactionTypes))
	}

	// Verify order
	if resp.ReactionTypes[0].Id != "like" {
		t.Errorf("Expected first type=like, got %s", resp.ReactionTypes[0].Id)
	}
}

// Placeholder for filepath import (used by setupSQLiteDB)
var _ = filepath.Join

// ========== Hooks and Context Tests ==========

// TestLikesService_UserIDFromContext tests that user ID is resolved from context.
func TestLikesService_UserIDFromContext(t *testing.T) {
	db := setupTestDB(t)

	// Use a custom context key
	type myCtxKey string
	const userKey myCtxKey = "my_user_id"

	service := backends.NewGORMLikesServiceWithOptions(db,
		backends.WithUserIDContextKey(userKey),
	)
	if err := service.AutoMigrate(); err != nil {
		t.Fatalf("Failed to auto-migrate: %v", err)
	}

	// Set user ID in context
	ctx := context.WithValue(context.Background(), userKey, "user-from-context")

	// Add reaction without specifying UserId in request
	resp, err := service.AddReaction(ctx, &v1.AddReactionRequest{
		EntityId:     "post-1",
		ReactionType: "like",
		// UserId intentionally omitted - should come from context
	})
	if err != nil {
		t.Fatalf("AddReaction failed: %v", err)
	}

	if resp.Like.UserId != "user-from-context" {
		t.Errorf("Expected UserId=user-from-context, got %s", resp.Like.UserId)
	}

	// Verify we can get the reaction using context-based user ID
	getResp, err := service.GetUserReaction(ctx, &v1.GetUserReactionRequest{
		EntityId: "post-1",
		// UserId intentionally omitted
	})
	if err != nil {
		t.Fatalf("GetUserReaction failed: %v", err)
	}
	if getResp.Like == nil {
		t.Error("Expected to find the reaction")
	}
}

// TestLikesService_OnAuthorizeHook tests the authorization hook.
func TestLikesService_OnAuthorizeHook(t *testing.T) {
	db := setupTestDB(t)

	authCalled := false
	authDenied := false

	service := backends.NewGORMLikesServiceWithOptions(db,
		backends.WithOnAuthorize(func(ctx context.Context, hookCtx *backends.HookContext) error {
			authCalled = true
			if hookCtx.Operation != "AddReaction" {
				t.Errorf("Expected operation=AddReaction, got %s", hookCtx.Operation)
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
	_, err := service.AddReaction(ctx, &v1.AddReactionRequest{
		EntityId:     "post-1",
		UserId:       "user-1",
		ReactionType: "like",
	})
	if err != nil {
		t.Fatalf("AddReaction failed: %v", err)
	}
	if !authCalled {
		t.Error("Expected OnAuthorize hook to be called")
	}

	// Test authorization denial
	authCalled = false
	authDenied = true
	_, err = service.AddReaction(ctx, &v1.AddReactionRequest{
		EntityId:     "post-2",
		UserId:       "user-1",
		ReactionType: "like",
	})
	if err == nil {
		t.Error("Expected AddReaction to fail when authorization denied")
	}
	if !authCalled {
		t.Error("Expected OnAuthorize hook to be called")
	}
}

// TestLikesService_ValidateEntityHook tests the entity validation hook.
func TestLikesService_ValidateEntityHook(t *testing.T) {
	db := setupTestDB(t)

	validEntities := map[string]bool{
		"post-1": true,
		"post-2": true,
	}

	service := backends.NewGORMLikesServiceWithOptions(db,
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
	_, err := service.AddReaction(ctx, &v1.AddReactionRequest{
		EntityId:     "post-1",
		UserId:       "user-1",
		ReactionType: "like",
	})
	if err != nil {
		t.Fatalf("AddReaction failed for valid entity: %v", err)
	}

	// Test invalid entity
	_, err = service.AddReaction(ctx, &v1.AddReactionRequest{
		EntityId:     "invalid-post",
		UserId:       "user-1",
		ReactionType: "like",
	})
	if err == nil {
		t.Error("Expected AddReaction to fail for invalid entity")
	}
}

// TestLikesService_BeforeAfterSaveHooks tests the save lifecycle hooks.
func TestLikesService_BeforeAfterSaveHooks(t *testing.T) {
	db := setupTestDB(t)

	beforeSaveCalled := false
	afterSaveCalled := false
	var savedLikeID string

	service := backends.NewGORMLikesServiceWithOptions(db,
		backends.WithBeforeSave(func(ctx context.Context, like *v1.Like) error {
			beforeSaveCalled = true
			// Verify we can inspect the like before save
			if like.EntityId == "" {
				return fmt.Errorf("entity_id should be set")
			}
			return nil
		}),
		backends.WithAfterSave(func(ctx context.Context, like *v1.Like) error {
			afterSaveCalled = true
			savedLikeID = like.Id
			return nil
		}),
	)
	if err := service.AutoMigrate(); err != nil {
		t.Fatalf("Failed to auto-migrate: %v", err)
	}

	ctx := context.Background()

	resp, err := service.AddReaction(ctx, &v1.AddReactionRequest{
		EntityId:     "post-1",
		UserId:       "user-1",
		ReactionType: "like",
	})
	if err != nil {
		t.Fatalf("AddReaction failed: %v", err)
	}

	if !beforeSaveCalled {
		t.Error("Expected BeforeSave hook to be called")
	}
	if !afterSaveCalled {
		t.Error("Expected AfterSave hook to be called")
	}
	if savedLikeID != resp.Like.Id {
		t.Errorf("Expected savedLikeID=%s, got %s", resp.Like.Id, savedLikeID)
	}
}

// TestLikesService_BeforeAfterDeleteHooks tests the delete lifecycle hooks.
func TestLikesService_BeforeAfterDeleteHooks(t *testing.T) {
	db := setupTestDB(t)

	beforeDeleteCalled := false
	afterDeleteCalled := false
	var deletedEntityID string

	service := backends.NewGORMLikesServiceWithOptions(db,
		backends.WithBeforeDelete(func(ctx context.Context, like *v1.Like) error {
			beforeDeleteCalled = true
			return nil
		}),
		backends.WithAfterDelete(func(ctx context.Context, like *v1.Like) error {
			afterDeleteCalled = true
			deletedEntityID = like.EntityId
			return nil
		}),
	)
	if err := service.AutoMigrate(); err != nil {
		t.Fatalf("Failed to auto-migrate: %v", err)
	}

	ctx := context.Background()

	// First add a reaction
	_, err := service.AddReaction(ctx, &v1.AddReactionRequest{
		EntityId:     "post-1",
		UserId:       "user-1",
		ReactionType: "like",
	})
	if err != nil {
		t.Fatalf("AddReaction failed: %v", err)
	}

	// Now remove it
	_, err = service.RemoveReaction(ctx, &v1.RemoveReactionRequest{
		EntityId: "post-1",
		UserId:   "user-1",
	})
	if err != nil {
		t.Fatalf("RemoveReaction failed: %v", err)
	}

	if !beforeDeleteCalled {
		t.Error("Expected BeforeDelete hook to be called")
	}
	if !afterDeleteCalled {
		t.Error("Expected AfterDelete hook to be called")
	}
	if deletedEntityID != "post-1" {
		t.Errorf("Expected deletedEntityID=post-1, got %s", deletedEntityID)
	}
}

// TestLikesService_OnEventHook tests the event notification hook.
func TestLikesService_OnEventHook(t *testing.T) {
	db := setupTestDB(t)

	var events []*backends.Event

	service := backends.NewGORMLikesServiceWithOptions(db,
		backends.WithOnEvent(func(ctx context.Context, event *backends.Event) error {
			events = append(events, event)
			return nil
		}),
	)
	if err := service.AutoMigrate(); err != nil {
		t.Fatalf("Failed to auto-migrate: %v", err)
	}

	ctx := context.Background()

	// Add reaction - should trigger EventReactionAdded
	_, err := service.AddReaction(ctx, &v1.AddReactionRequest{
		EntityId:     "post-1",
		UserId:       "user-1",
		ReactionType: "like",
	})
	if err != nil {
		t.Fatalf("AddReaction failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}
	if events[0].Type != backends.EventReactionAdded {
		t.Errorf("Expected event type=%s, got %s", backends.EventReactionAdded, events[0].Type)
	}

	// Change reaction - should trigger EventReactionChanged
	_, err = service.AddReaction(ctx, &v1.AddReactionRequest{
		EntityId:     "post-1",
		UserId:       "user-1",
		ReactionType: "love",
	})
	if err != nil {
		t.Fatalf("AddReaction (change) failed: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("Expected 2 events, got %d", len(events))
	}
	if events[1].Type != backends.EventReactionChanged {
		t.Errorf("Expected event type=%s, got %s", backends.EventReactionChanged, events[1].Type)
	}
	if events[1].OldType != "like" || events[1].NewType != "love" {
		t.Errorf("Expected change from like to love, got %s to %s", events[1].OldType, events[1].NewType)
	}

	// Remove reaction - should trigger EventReactionRemoved
	_, err = service.RemoveReaction(ctx, &v1.RemoveReactionRequest{
		EntityId: "post-1",
		UserId:   "user-1",
	})
	if err != nil {
		t.Fatalf("RemoveReaction failed: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("Expected 3 events, got %d", len(events))
	}
	if events[2].Type != backends.EventReactionRemoved {
		t.Errorf("Expected event type=%s, got %s", backends.EventReactionRemoved, events[2].Type)
	}
}

// TestLikesService_AfterReadHook tests the after read hook for data enrichment.
func TestLikesService_AfterReadHook(t *testing.T) {
	db := setupTestDB(t)

	enrichmentCalled := false

	service := backends.NewGORMLikesServiceWithOptions(db,
		backends.WithAfterRead(func(ctx context.Context, likes []*v1.Like) error {
			enrichmentCalled = true
			// Could enrich likes here (e.g., add user display names)
			return nil
		}),
	)
	if err := service.AutoMigrate(); err != nil {
		t.Fatalf("Failed to auto-migrate: %v", err)
	}

	ctx := context.Background()

	// Add a reaction
	_, err := service.AddReaction(ctx, &v1.AddReactionRequest{
		EntityId:     "post-1",
		UserId:       "user-1",
		ReactionType: "like",
	})
	if err != nil {
		t.Fatalf("AddReaction failed: %v", err)
	}

	// Get reaction - should trigger AfterRead
	_, err = service.GetUserReaction(ctx, &v1.GetUserReactionRequest{
		EntityId: "post-1",
		UserId:   "user-1",
	})
	if err != nil {
		t.Fatalf("GetUserReaction failed: %v", err)
	}

	if !enrichmentCalled {
		t.Error("Expected AfterRead hook to be called")
	}
}

// TestLikesService_HookCanModifyRequest tests that auth hook can modify request.
func TestLikesService_HookCanModifyRequest(t *testing.T) {
	db := setupTestDB(t)

	service := backends.NewGORMLikesServiceWithOptions(db,
		backends.WithOnAuthorize(func(ctx context.Context, hookCtx *backends.HookContext) error {
			// Modify the request to set user ID from "authenticated" context
			if req, ok := hookCtx.Request.(*v1.AddReactionRequest); ok {
				if req.UserId == "" {
					req.UserId = "auth-user-123"
				}
			}
			return nil
		}),
	)
	if err := service.AutoMigrate(); err != nil {
		t.Fatalf("Failed to auto-migrate: %v", err)
	}

	ctx := context.Background()

	// Add reaction without UserId - hook should set it
	resp, err := service.AddReaction(ctx, &v1.AddReactionRequest{
		EntityId:     "post-1",
		ReactionType: "like",
		// UserId intentionally empty
	})
	if err != nil {
		t.Fatalf("AddReaction failed: %v", err)
	}

	if resp.Like.UserId != "auth-user-123" {
		t.Errorf("Expected UserId=auth-user-123, got %s", resp.Like.UserId)
	}
}
