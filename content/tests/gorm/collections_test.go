// Package gorm provides integration tests for content services using GORM/SQL backends.
package gorm

import (
	"context"
	"fmt"
	"testing"

	v1 "github.com/panyam/goapplib/content/gen/go/collections/v1"
	"github.com/panyam/goapplib/content/services/collections"
	"github.com/panyam/goapplib/content/services/collections/backends"
)

// setupCollectionsServiceWithOptions creates a collections service with options.
func setupCollectionsServiceWithOptions(t *testing.T, opts ...collections.ServiceOption) *backends.GORMCollectionsService {
	db := setupTestDB(t)
	service := backends.NewGORMCollectionsService(db, opts...)
	if err := service.AutoMigrate(); err != nil {
		t.Fatalf("Failed to auto-migrate: %v", err)
	}
	return service
}

// setupCollectionsService creates a collections service with auto-migration.
func setupCollectionsService(t *testing.T) *backends.GORMCollectionsService {
	db := setupTestDB(t)
	service := backends.NewGORMCollectionsService(db)
	if err := service.AutoMigrate(); err != nil {
		t.Fatalf("Failed to auto-migrate: %v", err)
	}
	return service
}

// TestCollectionsService_CreateCollection tests creating collections.
func TestCollectionsService_CreateCollection(t *testing.T) {
	service := setupCollectionsService(t)
	ctx := context.Background()

	// Create a root collection
	resp, err := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:       "My Folder",
		OwnerId:    "user-1",
		Visibility: v1.CollectionVisibility_COLLECTION_VISIBILITY_PRIVATE,
	})
	if err != nil {
		t.Fatalf("CreateCollection failed: %v", err)
	}

	if resp.Collection == nil {
		t.Fatal("Expected collection to be returned")
	}
	if resp.Collection.Name != "My Folder" {
		t.Errorf("Expected name='My Folder', got %s", resp.Collection.Name)
	}
	if resp.Collection.NormalizedName != "my folder" {
		t.Errorf("Expected normalized_name='my folder', got %s", resp.Collection.NormalizedName)
	}
	if resp.AlreadyExisted {
		t.Error("Expected AlreadyExisted=false for new collection")
	}
	if resp.Collection.Depth != 0 {
		t.Errorf("Expected depth=0 for root collection, got %d", resp.Collection.Depth)
	}
	if len(resp.Collection.Path) != 0 {
		t.Errorf("Expected empty path for root collection, got %v", resp.Collection.Path)
	}

	// Create same collection again - should return existing
	resp2, err := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:    "MY FOLDER", // Different case
		OwnerId: "user-1",
	})
	if err != nil {
		t.Fatalf("CreateCollection (duplicate) failed: %v", err)
	}

	if !resp2.AlreadyExisted {
		t.Error("Expected AlreadyExisted=true for duplicate collection")
	}
	if resp2.Collection.Id != resp.Collection.Id {
		t.Error("Expected same collection ID for duplicate")
	}
}

// TestCollectionsService_NestedCollections tests creating nested collections.
func TestCollectionsService_NestedCollections(t *testing.T) {
	service := setupCollectionsService(t)
	ctx := context.Background()

	// Create root collection
	rootResp, err := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:    "Root",
		OwnerId: "user-1",
	})
	if err != nil {
		t.Fatalf("CreateCollection (root) failed: %v", err)
	}
	rootID := rootResp.Collection.Id

	// Create child collection
	childResp, err := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:     "Child",
		OwnerId:  "user-1",
		ParentId: rootID,
	})
	if err != nil {
		t.Fatalf("CreateCollection (child) failed: %v", err)
	}

	if childResp.Collection.ParentId != rootID {
		t.Errorf("Expected parent_id=%s, got %s", rootID, childResp.Collection.ParentId)
	}
	if childResp.Collection.Depth != 1 {
		t.Errorf("Expected depth=1, got %d", childResp.Collection.Depth)
	}
	if len(childResp.Collection.Path) != 1 || childResp.Collection.Path[0] != rootID {
		t.Errorf("Expected path=[%s], got %v", rootID, childResp.Collection.Path)
	}

	// Create grandchild collection
	grandchildResp, err := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:     "Grandchild",
		OwnerId:  "user-1",
		ParentId: childResp.Collection.Id,
	})
	if err != nil {
		t.Fatalf("CreateCollection (grandchild) failed: %v", err)
	}

	if grandchildResp.Collection.Depth != 2 {
		t.Errorf("Expected depth=2, got %d", grandchildResp.Collection.Depth)
	}
	expectedPath := []string{rootID, childResp.Collection.Id}
	if len(grandchildResp.Collection.Path) != 2 {
		t.Errorf("Expected path length=2, got %d", len(grandchildResp.Collection.Path))
	}
	for i, id := range expectedPath {
		if grandchildResp.Collection.Path[i] != id {
			t.Errorf("Expected path[%d]=%s, got %s", i, id, grandchildResp.Collection.Path[i])
		}
	}

	// Verify parent's child_count was updated
	parentResp, err := service.GetCollection(ctx, &v1.GetCollectionRequest{Id: rootID})
	if err != nil {
		t.Fatalf("GetCollection failed: %v", err)
	}
	if parentResp.Collection.ChildCount != 1 {
		t.Errorf("Expected child_count=1, got %d", parentResp.Collection.ChildCount)
	}
}

// TestCollectionsService_GetCollectionPath tests getting the breadcrumb path.
func TestCollectionsService_GetCollectionPath(t *testing.T) {
	service := setupCollectionsService(t)
	ctx := context.Background()

	// Create hierarchy: Root -> Child -> Grandchild
	rootResp, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:    "Root",
		OwnerId: "user-1",
	})
	childResp, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:     "Child",
		OwnerId:  "user-1",
		ParentId: rootResp.Collection.Id,
	})
	grandchildResp, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:     "Grandchild",
		OwnerId:  "user-1",
		ParentId: childResp.Collection.Id,
	})

	// Get path for grandchild
	pathResp, err := service.GetCollectionPath(ctx, &v1.GetCollectionPathRequest{
		Id: grandchildResp.Collection.Id,
	})
	if err != nil {
		t.Fatalf("GetCollectionPath failed: %v", err)
	}

	if len(pathResp.Ancestors) != 3 {
		t.Errorf("Expected 3 ancestors (Root, Child, Grandchild), got %d", len(pathResp.Ancestors))
	}
	if pathResp.Ancestors[0].Name != "Root" {
		t.Errorf("Expected first ancestor to be Root, got %s", pathResp.Ancestors[0].Name)
	}
	if pathResp.Ancestors[1].Name != "Child" {
		t.Errorf("Expected second ancestor to be Child, got %s", pathResp.Ancestors[1].Name)
	}
	if pathResp.Ancestors[2].Name != "Grandchild" {
		t.Errorf("Expected third ancestor to be Grandchild, got %s", pathResp.Ancestors[2].Name)
	}
}

// TestCollectionsService_AddRemoveItems tests adding and removing items from collections.
func TestCollectionsService_AddRemoveItems(t *testing.T) {
	service := setupCollectionsService(t)
	ctx := context.Background()

	// Create collection
	collResp, err := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:    "My Playlist",
		OwnerId: "user-1",
		Type:    "playlist",
	})
	if err != nil {
		t.Fatalf("CreateCollection failed: %v", err)
	}
	collID := collResp.Collection.Id

	// Add items
	addResp, err := service.AddToCollection(ctx, &v1.AddToCollectionRequest{
		CollectionId: collID,
		EntityId:     "song-1",
		AddedBy:      "user-1",
	})
	if err != nil {
		t.Fatalf("AddToCollection failed: %v", err)
	}
	if !addResp.NewlyAdded {
		t.Error("Expected NewlyAdded=true")
	}
	if addResp.Item.DisplayOrder != 1 {
		t.Errorf("Expected display_order=1, got %d", addResp.Item.DisplayOrder)
	}

	// Add same item again - should not be newly added
	addResp2, err := service.AddToCollection(ctx, &v1.AddToCollectionRequest{
		CollectionId: collID,
		EntityId:     "song-1",
		AddedBy:      "user-1",
	})
	if err != nil {
		t.Fatalf("AddToCollection (duplicate) failed: %v", err)
	}
	if addResp2.NewlyAdded {
		t.Error("Expected NewlyAdded=false for duplicate")
	}

	// Add another item
	addResp3, err := service.AddToCollection(ctx, &v1.AddToCollectionRequest{
		CollectionId: collID,
		EntityId:     "song-2",
		AddedBy:      "user-1",
	})
	if err != nil {
		t.Fatalf("AddToCollection failed: %v", err)
	}
	if addResp3.Item.DisplayOrder != 2 {
		t.Errorf("Expected display_order=2, got %d", addResp3.Item.DisplayOrder)
	}

	// Verify item_count
	getResp, err := service.GetCollection(ctx, &v1.GetCollectionRequest{Id: collID})
	if err != nil {
		t.Fatalf("GetCollection failed: %v", err)
	}
	if getResp.Collection.ItemCount != 2 {
		t.Errorf("Expected item_count=2, got %d", getResp.Collection.ItemCount)
	}

	// Remove item
	removeResp, err := service.RemoveFromCollection(ctx, &v1.RemoveFromCollectionRequest{
		CollectionId: collID,
		EntityId:     "song-1",
	})
	if err != nil {
		t.Fatalf("RemoveFromCollection failed: %v", err)
	}
	if !removeResp.Removed {
		t.Error("Expected Removed=true")
	}

	// Verify item_count decreased
	getResp2, err := service.GetCollection(ctx, &v1.GetCollectionRequest{Id: collID})
	if err != nil {
		t.Fatalf("GetCollection failed: %v", err)
	}
	if getResp2.Collection.ItemCount != 1 {
		t.Errorf("Expected item_count=1, got %d", getResp2.Collection.ItemCount)
	}
}

// TestCollectionsService_GetCollectionItems tests listing items in a collection.
func TestCollectionsService_GetCollectionItems(t *testing.T) {
	service := setupCollectionsService(t)
	ctx := context.Background()

	// Create collection and add items
	collResp, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:    "My Album",
		OwnerId: "user-1",
	})
	collID := collResp.Collection.Id

	for i := 1; i <= 5; i++ {
		_, err := service.AddToCollection(ctx, &v1.AddToCollectionRequest{
			CollectionId: collID,
			EntityId:     fmt.Sprintf("photo-%d", i),
			AddedBy:      "user-1",
		})
		if err != nil {
			t.Fatalf("AddToCollection failed: %v", err)
		}
	}

	// Get items
	itemsResp, err := service.GetCollectionItems(ctx, &v1.GetCollectionItemsRequest{
		CollectionId: collID,
	})
	if err != nil {
		t.Fatalf("GetCollectionItems failed: %v", err)
	}

	if len(itemsResp.Items) != 5 {
		t.Errorf("Expected 5 items, got %d", len(itemsResp.Items))
	}

	// Verify ordering by display_order
	for i, item := range itemsResp.Items {
		expectedOrder := int32(i + 1)
		if item.DisplayOrder != expectedOrder {
			t.Errorf("Expected item[%d].display_order=%d, got %d", i, expectedOrder, item.DisplayOrder)
		}
	}
}

// TestCollectionsService_GetEntityCollections tests finding collections containing an entity.
func TestCollectionsService_GetEntityCollections(t *testing.T) {
	service := setupCollectionsService(t)
	ctx := context.Background()

	// Create multiple collections
	coll1, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:    "Favorites",
		OwnerId: "user-1",
	})
	coll2, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:    "To Read",
		OwnerId: "user-1",
	})
	coll3, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:    "Other User's Collection",
		OwnerId: "user-2",
	})

	// Add same entity to multiple collections
	service.AddToCollection(ctx, &v1.AddToCollectionRequest{
		CollectionId: coll1.Collection.Id,
		EntityId:     "book-1",
		AddedBy:      "user-1",
	})
	service.AddToCollection(ctx, &v1.AddToCollectionRequest{
		CollectionId: coll2.Collection.Id,
		EntityId:     "book-1",
		AddedBy:      "user-1",
	})
	service.AddToCollection(ctx, &v1.AddToCollectionRequest{
		CollectionId: coll3.Collection.Id,
		EntityId:     "book-1",
		AddedBy:      "user-2",
	})

	// Get all collections containing the book
	resp, err := service.GetEntityCollections(ctx, &v1.GetEntityCollectionsRequest{
		EntityId: "book-1",
	})
	if err != nil {
		t.Fatalf("GetEntityCollections failed: %v", err)
	}

	if len(resp.Collections) != 3 {
		t.Errorf("Expected 3 collections, got %d", len(resp.Collections))
	}

	// Get collections filtered by owner
	respFiltered, err := service.GetEntityCollections(ctx, &v1.GetEntityCollectionsRequest{
		EntityId: "book-1",
		OwnerId:  "user-1",
	})
	if err != nil {
		t.Fatalf("GetEntityCollections (filtered) failed: %v", err)
	}

	if len(respFiltered.Collections) != 2 {
		t.Errorf("Expected 2 collections for user-1, got %d", len(respFiltered.Collections))
	}
}

// TestCollectionsService_ListCollections tests listing collections with filtering.
func TestCollectionsService_ListCollections(t *testing.T) {
	service := setupCollectionsService(t)
	ctx := context.Background()

	// Create root collections for user-1
	service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:    "User1 Folder1",
		OwnerId: "user-1",
	})
	service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:    "User1 Folder2",
		OwnerId: "user-1",
	})

	// Create root collection for user-2
	service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:    "User2 Folder",
		OwnerId: "user-2",
	})

	// List root collections for user-1
	resp, err := service.ListCollections(ctx, &v1.ListCollectionsRequest{
		OwnerId:  "user-1",
		ParentId: "", // Root collections
	})
	if err != nil {
		t.Fatalf("ListCollections failed: %v", err)
	}

	if len(resp.Collections) != 2 {
		t.Errorf("Expected 2 collections for user-1, got %d", len(resp.Collections))
	}
}

// TestCollectionsService_DeleteCollection tests deleting collections.
func TestCollectionsService_DeleteCollection(t *testing.T) {
	service := setupCollectionsService(t)
	ctx := context.Background()

	// Create collection with items
	collResp, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:    "To Delete",
		OwnerId: "user-1",
	})
	collID := collResp.Collection.Id

	service.AddToCollection(ctx, &v1.AddToCollectionRequest{
		CollectionId: collID,
		EntityId:     "doc-1",
		AddedBy:      "user-1",
	})

	// Try to delete without force - should fail
	_, err := service.DeleteCollection(ctx, &v1.DeleteCollectionRequest{
		Id: collID,
	})
	if err == nil {
		t.Error("Expected error when deleting collection with items")
	}

	// Delete with force
	deleteResp, err := service.DeleteCollection(ctx, &v1.DeleteCollectionRequest{
		Id:          collID,
		ForceDelete: true,
	})
	if err != nil {
		t.Fatalf("DeleteCollection failed: %v", err)
	}

	if !deleteResp.Deleted {
		t.Error("Expected Deleted=true")
	}
	if deleteResp.ItemsRemoved != 1 {
		t.Errorf("Expected items_removed=1, got %d", deleteResp.ItemsRemoved)
	}

	// Verify collection is deleted
	getResp, err := service.GetCollection(ctx, &v1.GetCollectionRequest{Id: collID})
	if err == nil && getResp.Collection != nil {
		t.Error("Expected collection to be deleted")
	}
}

// TestCollectionsService_MoveCollection tests moving collections.
func TestCollectionsService_MoveCollection(t *testing.T) {
	service := setupCollectionsService(t)
	ctx := context.Background()

	// Create hierarchy: FolderA -> SubFolder, FolderB
	folderA, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:    "Folder A",
		OwnerId: "user-1",
	})
	subFolder, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:     "SubFolder",
		OwnerId:  "user-1",
		ParentId: folderA.Collection.Id,
	})
	folderB, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:    "Folder B",
		OwnerId: "user-1",
	})

	// Move SubFolder from FolderA to FolderB
	moveResp, err := service.MoveCollection(ctx, &v1.MoveCollectionRequest{
		Id:          subFolder.Collection.Id,
		NewParentId: folderB.Collection.Id,
	})
	if err != nil {
		t.Fatalf("MoveCollection failed: %v", err)
	}

	if moveResp.Collection.ParentId != folderB.Collection.Id {
		t.Errorf("Expected parent_id=%s, got %s", folderB.Collection.Id, moveResp.Collection.ParentId)
	}
	if len(moveResp.Collection.Path) != 1 || moveResp.Collection.Path[0] != folderB.Collection.Id {
		t.Errorf("Expected path=[%s], got %v", folderB.Collection.Id, moveResp.Collection.Path)
	}

	// Verify FolderA's child_count decreased
	folderAResp, _ := service.GetCollection(ctx, &v1.GetCollectionRequest{Id: folderA.Collection.Id})
	if folderAResp.Collection.ChildCount != 0 {
		t.Errorf("Expected FolderA.child_count=0, got %d", folderAResp.Collection.ChildCount)
	}

	// Verify FolderB's child_count increased
	folderBResp, _ := service.GetCollection(ctx, &v1.GetCollectionRequest{Id: folderB.Collection.Id})
	if folderBResp.Collection.ChildCount != 1 {
		t.Errorf("Expected FolderB.child_count=1, got %d", folderBResp.Collection.ChildCount)
	}
}

// TestCollectionsService_CircularReferencePreventio tests that circular references are prevented.
func TestCollectionsService_CircularReferencePrevention(t *testing.T) {
	service := setupCollectionsService(t)
	ctx := context.Background()

	// Create hierarchy: A -> B -> C
	collA, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:    "A",
		OwnerId: "user-1",
	})
	collB, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:     "B",
		OwnerId:  "user-1",
		ParentId: collA.Collection.Id,
	})
	collC, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:     "C",
		OwnerId:  "user-1",
		ParentId: collB.Collection.Id,
	})

	// Try to move A into C (would create circular reference)
	_, err := service.MoveCollection(ctx, &v1.MoveCollectionRequest{
		Id:          collA.Collection.Id,
		NewParentId: collC.Collection.Id,
	})
	if err == nil {
		t.Error("Expected error when creating circular reference")
	}

	// Try to move A into itself
	_, err = service.MoveCollection(ctx, &v1.MoveCollectionRequest{
		Id:          collA.Collection.Id,
		NewParentId: collA.Collection.Id,
	})
	if err == nil {
		t.Error("Expected error when moving collection into itself")
	}
}

// TestCollectionsService_ReorderItems tests reordering items.
func TestCollectionsService_ReorderItems(t *testing.T) {
	service := setupCollectionsService(t)
	ctx := context.Background()

	// Create collection and add items
	collResp, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:    "Playlist",
		OwnerId: "user-1",
	})
	collID := collResp.Collection.Id

	for i := 1; i <= 3; i++ {
		service.AddToCollection(ctx, &v1.AddToCollectionRequest{
			CollectionId: collID,
			EntityId:     fmt.Sprintf("song-%d", i),
			AddedBy:      "user-1",
		})
	}

	// Reorder items: song-3 to position 1, song-1 to position 3
	_, err := service.ReorderItems(ctx, &v1.ReorderItemsRequest{
		CollectionId: collID,
		ItemOrders: []*v1.ItemOrder{
			{EntityId: "song-3", DisplayOrder: 1},
			{EntityId: "song-1", DisplayOrder: 3},
		},
	})
	if err != nil {
		t.Fatalf("ReorderItems failed: %v", err)
	}

	// Verify new order
	itemsResp, _ := service.GetCollectionItems(ctx, &v1.GetCollectionItemsRequest{
		CollectionId: collID,
	})

	// song-3 should now be first (display_order=1)
	if itemsResp.Items[0].EntityId != "song-3" {
		t.Errorf("Expected first item to be song-3, got %s", itemsResp.Items[0].EntityId)
	}
}

// TestCollectionsService_CollectionType tests that collection type is stored correctly.
func TestCollectionsService_CollectionType(t *testing.T) {
	service := setupCollectionsService(t)
	ctx := context.Background()

	types := []string{"folder", "playlist", "reading_list", "project", "album"}

	for _, typ := range types {
		resp, err := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
			Name:    fmt.Sprintf("My %s", typ),
			OwnerId: "user-1",
			Type:    typ,
		})
		if err != nil {
			t.Fatalf("CreateCollection failed for type %s: %v", typ, err)
		}

		if resp.Collection.Type != typ {
			t.Errorf("Expected type=%s, got %s", typ, resp.Collection.Type)
		}
	}
}

// TestCollectionsService_BatchOperations tests batch add and get operations.
func TestCollectionsService_BatchOperations(t *testing.T) {
	service := setupCollectionsService(t)
	ctx := context.Background()

	// Create collection
	collResp, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:    "Batch Test",
		OwnerId: "user-1",
	})
	collID := collResp.Collection.Id

	// Batch add items
	batchAddResp, err := service.BatchAddToCollection(ctx, &v1.BatchAddToCollectionRequest{
		CollectionId: collID,
		EntityIds: []string{
			"item-1",
			"item-2",
			"item-3",
		},
		AddedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("BatchAddToCollection failed: %v", err)
	}

	if batchAddResp.ItemsAdded != 3 {
		t.Errorf("Expected items_added=3, got %d", batchAddResp.ItemsAdded)
	}

	// Batch add same items again
	batchAddResp2, err := service.BatchAddToCollection(ctx, &v1.BatchAddToCollectionRequest{
		CollectionId: collID,
		EntityIds: []string{
			"item-1",
			"item-4",
		},
		AddedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("BatchAddToCollection (second) failed: %v", err)
	}

	if batchAddResp2.ItemsAdded != 1 {
		t.Errorf("Expected items_added=1, got %d", batchAddResp2.ItemsAdded)
	}
	if batchAddResp2.AlreadyExisted != 1 {
		t.Errorf("Expected already_existed=1, got %d", batchAddResp2.AlreadyExisted)
	}
}

// ============================================================================
// Hook Tests
// ============================================================================

// TestCollectionsService_OnAuthorizeHook tests the authorization hook.
func TestCollectionsService_OnAuthorizeHook(t *testing.T) {
	authCalled := false
	authDenied := false

	service := setupCollectionsServiceWithOptions(t,
		backends.WithOnAuthorize(func(ctx context.Context, hookCtx *backends.HookContext) error {
			authCalled = true
			if hookCtx.Operation != "CreateCollection" {
				t.Errorf("Expected operation=CreateCollection, got %s", hookCtx.Operation)
			}
			if authDenied {
				return fmt.Errorf("access denied")
			}
			return nil
		}),
	)

	ctx := context.Background()

	// Test successful authorization
	_, err := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:    "My Folder",
		OwnerId: "user-1",
	})
	if err != nil {
		t.Fatalf("CreateCollection failed: %v", err)
	}
	if !authCalled {
		t.Error("Expected OnAuthorize hook to be called")
	}

	// Test authorization denial
	authCalled = false
	authDenied = true
	_, err = service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:    "Another Folder",
		OwnerId: "user-1",
	})
	if err == nil {
		t.Error("Expected CreateCollection to fail when authorization denied")
	}
	if !authCalled {
		t.Error("Expected OnAuthorize hook to be called")
	}
}

// TestCollectionsService_ValidateEntityHook tests the entity validation hook.
func TestCollectionsService_ValidateEntityHook(t *testing.T) {
	validEntities := map[string]bool{
		"entity-1": true,
		"entity-2": true,
	}

	service := setupCollectionsServiceWithOptions(t,
		backends.WithValidateEntity(func(ctx context.Context, entityID string) error {
			if !validEntities[entityID] {
				return fmt.Errorf("entity not found: %s", entityID)
			}
			return nil
		}),
	)

	ctx := context.Background()

	// Create a collection first
	collResp, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:    "Test Collection",
		OwnerId: "user-1",
	})
	collID := collResp.Collection.Id

	// Test valid entity
	_, err := service.AddToCollection(ctx, &v1.AddToCollectionRequest{
		CollectionId: collID,
		EntityId:     "entity-1",
		AddedBy:      "user-1",
	})
	if err != nil {
		t.Fatalf("AddToCollection failed for valid entity: %v", err)
	}

	// Test invalid entity
	_, err = service.AddToCollection(ctx, &v1.AddToCollectionRequest{
		CollectionId: collID,
		EntityId:     "invalid-entity",
		AddedBy:      "user-1",
	})
	if err == nil {
		t.Error("Expected AddToCollection to fail for invalid entity")
	}
}

// TestCollectionsService_BeforeAfterCollectionSaveHooks tests the collection save lifecycle hooks.
func TestCollectionsService_BeforeAfterCollectionSaveHooks(t *testing.T) {
	beforeSaveCalled := false
	afterSaveCalled := false
	var savedCollectionID string

	service := setupCollectionsServiceWithOptions(t,
		backends.WithBeforeCollectionSave(func(ctx context.Context, collection *v1.Collection) error {
			beforeSaveCalled = true
			// Verify we can inspect the collection before save
			if collection.Name == "" {
				return fmt.Errorf("name should be set")
			}
			return nil
		}),
		backends.WithAfterCollectionSave(func(ctx context.Context, collection *v1.Collection) error {
			afterSaveCalled = true
			savedCollectionID = collection.Id
			return nil
		}),
	)

	ctx := context.Background()

	resp, err := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:    "My Collection",
		OwnerId: "user-1",
	})
	if err != nil {
		t.Fatalf("CreateCollection failed: %v", err)
	}

	if !beforeSaveCalled {
		t.Error("Expected BeforeCollectionSave hook to be called")
	}
	if !afterSaveCalled {
		t.Error("Expected AfterCollectionSave hook to be called")
	}
	if savedCollectionID != resp.Collection.Id {
		t.Errorf("Expected savedCollectionID=%s, got %s", resp.Collection.Id, savedCollectionID)
	}
}

// TestCollectionsService_BeforeAfterCollectionDeleteHooks tests the collection delete lifecycle hooks.
func TestCollectionsService_BeforeAfterCollectionDeleteHooks(t *testing.T) {
	beforeDeleteCalled := false
	afterDeleteCalled := false
	var deletedCollectionName string

	service := setupCollectionsServiceWithOptions(t,
		backends.WithBeforeCollectionDelete(func(ctx context.Context, collection *v1.Collection) error {
			beforeDeleteCalled = true
			return nil
		}),
		backends.WithAfterCollectionDelete(func(ctx context.Context, collection *v1.Collection) error {
			afterDeleteCalled = true
			deletedCollectionName = collection.Name
			return nil
		}),
	)

	ctx := context.Background()

	// First create a collection
	resp, err := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:    "To Delete",
		OwnerId: "user-1",
	})
	if err != nil {
		t.Fatalf("CreateCollection failed: %v", err)
	}

	// Now delete it
	_, err = service.DeleteCollection(ctx, &v1.DeleteCollectionRequest{
		Id: resp.Collection.Id,
	})
	if err != nil {
		t.Fatalf("DeleteCollection failed: %v", err)
	}

	if !beforeDeleteCalled {
		t.Error("Expected BeforeCollectionDelete hook to be called")
	}
	if !afterDeleteCalled {
		t.Error("Expected AfterCollectionDelete hook to be called")
	}
	if deletedCollectionName != "To Delete" {
		t.Errorf("Expected deletedCollectionName='To Delete', got %s", deletedCollectionName)
	}
}

// TestCollectionsService_BeforeAfterItemSaveHooks tests the item save lifecycle hooks.
func TestCollectionsService_BeforeAfterItemSaveHooks(t *testing.T) {
	beforeSaveCalled := false
	afterSaveCalled := false
	var savedEntityID string
	var savedCollectionName string

	service := setupCollectionsServiceWithOptions(t,
		backends.WithBeforeItemSave(func(ctx context.Context, item *v1.CollectionItem, collection *v1.Collection) error {
			beforeSaveCalled = true
			if item.EntityId == "" {
				return fmt.Errorf("entity_id should be set")
			}
			return nil
		}),
		backends.WithAfterItemSave(func(ctx context.Context, item *v1.CollectionItem, collection *v1.Collection) error {
			afterSaveCalled = true
			savedEntityID = item.EntityId
			savedCollectionName = collection.Name
			return nil
		}),
	)

	ctx := context.Background()

	// Create collection
	collResp, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:    "My Playlist",
		OwnerId: "user-1",
	})

	// Add item
	_, err := service.AddToCollection(ctx, &v1.AddToCollectionRequest{
		CollectionId: collResp.Collection.Id,
		EntityId:     "song-1",
		AddedBy:      "user-1",
	})
	if err != nil {
		t.Fatalf("AddToCollection failed: %v", err)
	}

	if !beforeSaveCalled {
		t.Error("Expected BeforeItemSave hook to be called")
	}
	if !afterSaveCalled {
		t.Error("Expected AfterItemSave hook to be called")
	}
	if savedEntityID != "song-1" {
		t.Errorf("Expected savedEntityID=song-1, got %s", savedEntityID)
	}
	if savedCollectionName != "My Playlist" {
		t.Errorf("Expected savedCollectionName='My Playlist', got %s", savedCollectionName)
	}
}

// TestCollectionsService_BeforeAfterItemDeleteHooks tests the item delete lifecycle hooks.
func TestCollectionsService_BeforeAfterItemDeleteHooks(t *testing.T) {
	beforeDeleteCalled := false
	afterDeleteCalled := false
	var deletedEntityID string

	service := setupCollectionsServiceWithOptions(t,
		backends.WithBeforeItemDelete(func(ctx context.Context, item *v1.CollectionItem) error {
			beforeDeleteCalled = true
			return nil
		}),
		backends.WithAfterItemDelete(func(ctx context.Context, item *v1.CollectionItem) error {
			afterDeleteCalled = true
			deletedEntityID = item.EntityId
			return nil
		}),
	)

	ctx := context.Background()

	// Create collection and add item
	collResp, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:    "My Playlist",
		OwnerId: "user-1",
	})
	service.AddToCollection(ctx, &v1.AddToCollectionRequest{
		CollectionId: collResp.Collection.Id,
		EntityId:     "song-1",
		AddedBy:      "user-1",
	})

	// Remove item
	_, err := service.RemoveFromCollection(ctx, &v1.RemoveFromCollectionRequest{
		CollectionId: collResp.Collection.Id,
		EntityId:     "song-1",
	})
	if err != nil {
		t.Fatalf("RemoveFromCollection failed: %v", err)
	}

	if !beforeDeleteCalled {
		t.Error("Expected BeforeItemDelete hook to be called")
	}
	if !afterDeleteCalled {
		t.Error("Expected AfterItemDelete hook to be called")
	}
	if deletedEntityID != "song-1" {
		t.Errorf("Expected deletedEntityID=song-1, got %s", deletedEntityID)
	}
}

// TestCollectionsService_OnEventHook tests the event notification hook.
func TestCollectionsService_OnEventHook(t *testing.T) {
	var events []*backends.Event

	service := setupCollectionsServiceWithOptions(t,
		backends.WithOnEvent(func(ctx context.Context, event *backends.Event) error {
			events = append(events, event)
			return nil
		}),
	)

	ctx := context.Background()

	// Create collection - should trigger EventCollectionCreated
	collResp, err := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:    "My Folder",
		OwnerId: "user-1",
	})
	if err != nil {
		t.Fatalf("CreateCollection failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}
	if events[0].Type != backends.EventCollectionCreated {
		t.Errorf("Expected event type=%s, got %s", backends.EventCollectionCreated, events[0].Type)
	}

	// Add item - should trigger EventItemAdded
	_, err = service.AddToCollection(ctx, &v1.AddToCollectionRequest{
		CollectionId: collResp.Collection.Id,
		EntityId:     "doc-1",
		AddedBy:      "user-1",
	})
	if err != nil {
		t.Fatalf("AddToCollection failed: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("Expected 2 events, got %d", len(events))
	}
	if events[1].Type != backends.EventItemAdded {
		t.Errorf("Expected event type=%s, got %s", backends.EventItemAdded, events[1].Type)
	}
	if events[1].EntityID != "doc-1" {
		t.Errorf("Expected entityID=doc-1, got %s", events[1].EntityID)
	}

	// Remove item - should trigger EventItemRemoved
	_, err = service.RemoveFromCollection(ctx, &v1.RemoveFromCollectionRequest{
		CollectionId: collResp.Collection.Id,
		EntityId:     "doc-1",
	})
	if err != nil {
		t.Fatalf("RemoveFromCollection failed: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("Expected 3 events, got %d", len(events))
	}
	if events[2].Type != backends.EventItemRemoved {
		t.Errorf("Expected event type=%s, got %s", backends.EventItemRemoved, events[2].Type)
	}

	// Delete collection - should trigger EventCollectionDeleted
	_, err = service.DeleteCollection(ctx, &v1.DeleteCollectionRequest{
		Id: collResp.Collection.Id,
	})
	if err != nil {
		t.Fatalf("DeleteCollection failed: %v", err)
	}

	if len(events) != 4 {
		t.Fatalf("Expected 4 events, got %d", len(events))
	}
	if events[3].Type != backends.EventCollectionDeleted {
		t.Errorf("Expected event type=%s, got %s", backends.EventCollectionDeleted, events[3].Type)
	}
}

// TestCollectionsService_AfterCollectionsReadHook tests the after collections read hook.
func TestCollectionsService_AfterCollectionsReadHook(t *testing.T) {
	enrichmentCalled := false

	service := setupCollectionsServiceWithOptions(t,
		backends.WithAfterCollectionsRead(func(ctx context.Context, collections []*v1.Collection) error {
			enrichmentCalled = true
			// Could enrich collections here (e.g., add computed fields)
			return nil
		}),
	)

	ctx := context.Background()

	// Create a collection
	_, err := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:    "My Folder",
		OwnerId: "user-1",
	})
	if err != nil {
		t.Fatalf("CreateCollection failed: %v", err)
	}

	// List collections - should trigger AfterCollectionsRead
	_, err = service.ListCollections(ctx, &v1.ListCollectionsRequest{
		OwnerId: "user-1",
	})
	if err != nil {
		t.Fatalf("ListCollections failed: %v", err)
	}

	if !enrichmentCalled {
		t.Error("Expected AfterCollectionsRead hook to be called")
	}
}

// TestCollectionsService_AfterItemsReadHook tests the after items read hook.
func TestCollectionsService_AfterItemsReadHook(t *testing.T) {
	enrichmentCalled := false

	service := setupCollectionsServiceWithOptions(t,
		backends.WithAfterItemsRead(func(ctx context.Context, items []*v1.CollectionItem) error {
			enrichmentCalled = true
			return nil
		}),
	)

	ctx := context.Background()

	// Create collection and add items
	collResp, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:    "My Playlist",
		OwnerId: "user-1",
	})
	service.AddToCollection(ctx, &v1.AddToCollectionRequest{
		CollectionId: collResp.Collection.Id,
		EntityId:     "song-1",
		AddedBy:      "user-1",
	})

	// Get items - should trigger AfterItemsRead
	_, err := service.GetCollectionItems(ctx, &v1.GetCollectionItemsRequest{
		CollectionId: collResp.Collection.Id,
	})
	if err != nil {
		t.Fatalf("GetCollectionItems failed: %v", err)
	}

	if !enrichmentCalled {
		t.Error("Expected AfterItemsRead hook to be called")
	}
}

// TestCollectionsService_HookCanModifyRequest tests that auth hook can modify request.
func TestCollectionsService_HookCanModifyRequest(t *testing.T) {
	service := setupCollectionsServiceWithOptions(t,
		backends.WithOnAuthorize(func(ctx context.Context, hookCtx *backends.HookContext) error {
			// Modify the request to set owner ID from "authenticated" context
			if req, ok := hookCtx.Request.(*v1.CreateCollectionRequest); ok {
				if req.OwnerId == "" {
					req.OwnerId = "auth-user-123"
				}
			}
			return nil
		}),
	)

	ctx := context.Background()

	// Create collection without OwnerId - hook should set it
	resp, err := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name: "My Folder",
		// OwnerId intentionally empty
	})
	if err != nil {
		t.Fatalf("CreateCollection failed: %v", err)
	}

	if resp.Collection.OwnerId != "auth-user-123" {
		t.Errorf("Expected OwnerId=auth-user-123, got %s", resp.Collection.OwnerId)
	}
}

// TestCollectionsService_UserIDFromContext tests user ID resolution from context.
func TestCollectionsService_UserIDFromContext(t *testing.T) {
	db := setupTestDB(t)

	// Create service with custom context key
	service := backends.NewGORMCollectionsService(db,
		backends.WithUserIDContextKey("my-user-key"),
	)
	if err := service.AutoMigrate(); err != nil {
		t.Fatalf("Failed to auto-migrate: %v", err)
	}

	// Create a collection first (with explicit owner)
	collResp, _ := service.CreateCollection(context.Background(), &v1.CreateCollectionRequest{
		Name:    "Test Collection",
		OwnerId: "explicit-owner",
	})

	// Add item with user ID from context
	ctx := context.WithValue(context.Background(), "my-user-key", "context-user-456")
	addResp, err := service.AddToCollection(ctx, &v1.AddToCollectionRequest{
		CollectionId: collResp.Collection.Id,
		EntityId:     "doc-1",
		// AddedBy intentionally empty - should be resolved from context
	})
	if err != nil {
		t.Fatalf("AddToCollection failed: %v", err)
	}

	if addResp.Item.AddedBy != "context-user-456" {
		t.Errorf("Expected AddedBy=context-user-456, got %s", addResp.Item.AddedBy)
	}
}
