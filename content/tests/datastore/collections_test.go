package datastore

import (
	"context"
	"fmt"
	"testing"

	v1 "github.com/panyam/goapplib/content/gen/go/collections/v1"
	commonv1 "github.com/panyam/goapplib/content/gen/go/common/v1"
	"github.com/panyam/goapplib/content/services/collections/backends"
)

// collectionsTestKinds are the Datastore kinds used by the collections service.
var collectionsTestKinds = []string{
	"Collection",
	"CollectionItem",
}

// setupCollectionsService creates a collections service for testing.
// Index validation is done once in TestMain, not here.
func setupCollectionsService(t *testing.T) *backends.DatastoreCollectionsService {
	client := setupTestClient(t, collectionsTestKinds)
	namespace := getTestNamespace()

	service, err := backends.NewDatastoreCollectionsService(client, namespace)
	if err != nil {
		t.Fatalf("Failed to create collections service: %v", err)
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
		OwnerType:  "user",
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
		Name:      "MY FOLDER", // Different case
		OwnerType: "user",
		OwnerId:   "user-1",
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
		Name:      "Root",
		OwnerType: "user",
		OwnerId:   "user-nested",
	})
	if err != nil {
		t.Fatalf("CreateCollection (root) failed: %v", err)
	}
	rootID := rootResp.Collection.Id

	// Create child collection
	childResp, err := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:      "Child",
		OwnerType: "user",
		OwnerId:   "user-nested",
		ParentId:  rootID,
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
		Name:      "Grandchild",
		OwnerType: "user",
		OwnerId:   "user-nested",
		ParentId:  childResp.Collection.Id,
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
		Name:      "RootPath",
		OwnerType: "user",
		OwnerId:   "user-path",
	})
	childResp, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:      "ChildPath",
		OwnerType: "user",
		OwnerId:   "user-path",
		ParentId:  rootResp.Collection.Id,
	})
	grandchildResp, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:      "GrandchildPath",
		OwnerType: "user",
		OwnerId:   "user-path",
		ParentId:  childResp.Collection.Id,
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
	if pathResp.Ancestors[0].Name != "RootPath" {
		t.Errorf("Expected first ancestor to be RootPath, got %s", pathResp.Ancestors[0].Name)
	}
	if pathResp.Ancestors[1].Name != "ChildPath" {
		t.Errorf("Expected second ancestor to be ChildPath, got %s", pathResp.Ancestors[1].Name)
	}
	if pathResp.Ancestors[2].Name != "GrandchildPath" {
		t.Errorf("Expected third ancestor to be GrandchildPath, got %s", pathResp.Ancestors[2].Name)
	}
}

// TestCollectionsService_AddRemoveItems tests adding and removing items from collections.
func TestCollectionsService_AddRemoveItems(t *testing.T) {
	service := setupCollectionsService(t)
	ctx := context.Background()

	// Create collection
	collResp, err := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:      "My Playlist",
		OwnerType: "user",
		OwnerId:   "user-items",
		Type:      "playlist",
	})
	if err != nil {
		t.Fatalf("CreateCollection failed: %v", err)
	}
	collID := collResp.Collection.Id

	// Add items
	addResp, err := service.AddToCollection(ctx, &v1.AddToCollectionRequest{
		CollectionId: collID,
		EntityType:   "song",
		EntityId:     "song-1",
		AddedBy:      "user-items",
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
		EntityType:   "song",
		EntityId:     "song-1",
		AddedBy:      "user-items",
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
		EntityType:   "song",
		EntityId:     "song-2",
		AddedBy:      "user-items",
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
		EntityType:   "song",
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
		Name:      "My Album",
		OwnerType: "user",
		OwnerId:   "user-album",
	})
	collID := collResp.Collection.Id

	for i := 1; i <= 5; i++ {
		_, err := service.AddToCollection(ctx, &v1.AddToCollectionRequest{
			CollectionId: collID,
			EntityType:   "photo",
			EntityId:     fmt.Sprintf("photo-%d", i),
			AddedBy:      "user-album",
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
		Name:      "FavoritesDS",
		OwnerType: "user",
		OwnerId:   "user-entity",
	})
	coll2, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:      "To ReadDS",
		OwnerType: "user",
		OwnerId:   "user-entity",
	})
	coll3, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:      "Other User's CollectionDS",
		OwnerType: "user",
		OwnerId:   "user-entity-2",
	})

	// Add same entity to multiple collections
	service.AddToCollection(ctx, &v1.AddToCollectionRequest{
		CollectionId: coll1.Collection.Id,
		EntityType:   "book",
		EntityId:     "book-ds-1",
		AddedBy:      "user-entity",
	})
	service.AddToCollection(ctx, &v1.AddToCollectionRequest{
		CollectionId: coll2.Collection.Id,
		EntityType:   "book",
		EntityId:     "book-ds-1",
		AddedBy:      "user-entity",
	})
	service.AddToCollection(ctx, &v1.AddToCollectionRequest{
		CollectionId: coll3.Collection.Id,
		EntityType:   "book",
		EntityId:     "book-ds-1",
		AddedBy:      "user-entity-2",
	})

	// Get all collections containing the book
	resp, err := service.GetEntityCollections(ctx, &v1.GetEntityCollectionsRequest{
		EntityType: "book",
		EntityId:   "book-ds-1",
	})
	if err != nil {
		t.Fatalf("GetEntityCollections failed: %v", err)
	}

	if len(resp.Collections) != 3 {
		t.Errorf("Expected 3 collections, got %d", len(resp.Collections))
	}

	// Get collections filtered by owner
	respFiltered, err := service.GetEntityCollections(ctx, &v1.GetEntityCollectionsRequest{
		EntityType: "book",
		EntityId:   "book-ds-1",
		OwnerType:  "user",
		OwnerId:    "user-entity",
	})
	if err != nil {
		t.Fatalf("GetEntityCollections (filtered) failed: %v", err)
	}

	if len(respFiltered.Collections) != 2 {
		t.Errorf("Expected 2 collections for user-entity, got %d", len(respFiltered.Collections))
	}
}

// TestCollectionsService_ListCollections tests listing collections with filtering.
func TestCollectionsService_ListCollections(t *testing.T) {
	service := setupCollectionsService(t)
	ctx := context.Background()

	// Create root collections for user-list-1
	service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:      "User1 Folder1DS",
		OwnerType: "user",
		OwnerId:   "user-list-1",
	})
	service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:      "User1 Folder2DS",
		OwnerType: "user",
		OwnerId:   "user-list-1",
	})

	// Create root collection for user-list-2
	service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:      "User2 FolderDS",
		OwnerType: "user",
		OwnerId:   "user-list-2",
	})

	// List root collections for user-list-1
	resp, err := service.ListCollections(ctx, &v1.ListCollectionsRequest{
		OwnerType: "user",
		OwnerId:   "user-list-1",
		ParentId:  "", // Root collections
	})
	if err != nil {
		t.Fatalf("ListCollections failed: %v", err)
	}

	if len(resp.Collections) != 2 {
		t.Errorf("Expected 2 collections for user-list-1, got %d", len(resp.Collections))
	}
}

// TestCollectionsService_DeleteCollection tests deleting collections.
func TestCollectionsService_DeleteCollection(t *testing.T) {
	service := setupCollectionsService(t)
	ctx := context.Background()

	// Create collection with items
	collResp, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:      "To DeleteDS",
		OwnerType: "user",
		OwnerId:   "user-delete",
	})
	collID := collResp.Collection.Id

	service.AddToCollection(ctx, &v1.AddToCollectionRequest{
		CollectionId: collID,
		EntityType:   "doc",
		EntityId:     "doc-ds-1",
		AddedBy:      "user-delete",
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
		Name:      "Folder ADS",
		OwnerType: "user",
		OwnerId:   "user-move",
	})
	subFolder, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:      "SubFolderDS",
		OwnerType: "user",
		OwnerId:   "user-move",
		ParentId:  folderA.Collection.Id,
	})
	folderB, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:      "Folder BDS",
		OwnerType: "user",
		OwnerId:   "user-move",
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

// TestCollectionsService_CircularReferencePrevention tests that circular references are prevented.
func TestCollectionsService_CircularReferencePrevention(t *testing.T) {
	service := setupCollectionsService(t)
	ctx := context.Background()

	// Create hierarchy: A -> B -> C
	collA, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:      "ADS",
		OwnerType: "user",
		OwnerId:   "user-circular",
	})
	collB, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:      "BDS",
		OwnerType: "user",
		OwnerId:   "user-circular",
		ParentId:  collA.Collection.Id,
	})
	collC, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
		Name:      "CDS",
		OwnerType: "user",
		OwnerId:   "user-circular",
		ParentId:  collB.Collection.Id,
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
		Name:      "PlaylistDS",
		OwnerType: "user",
		OwnerId:   "user-reorder",
	})
	collID := collResp.Collection.Id

	for i := 1; i <= 3; i++ {
		service.AddToCollection(ctx, &v1.AddToCollectionRequest{
			CollectionId: collID,
			EntityType:   "song",
			EntityId:     fmt.Sprintf("song-ds-%d", i),
			AddedBy:      "user-reorder",
		})
	}

	// Reorder items: song-ds-3 to position 1, song-ds-1 to position 3
	_, err := service.ReorderItems(ctx, &v1.ReorderItemsRequest{
		CollectionId: collID,
		ItemOrders: []*v1.ItemOrder{
			{EntityType: "song", EntityId: "song-ds-3", DisplayOrder: 1},
			{EntityType: "song", EntityId: "song-ds-1", DisplayOrder: 3},
		},
	})
	if err != nil {
		t.Fatalf("ReorderItems failed: %v", err)
	}

	// Verify new order
	itemsResp, _ := service.GetCollectionItems(ctx, &v1.GetCollectionItemsRequest{
		CollectionId: collID,
	})

	// song-ds-3 should now be first (display_order=1)
	if itemsResp.Items[0].EntityId != "song-ds-3" {
		t.Errorf("Expected first item to be song-ds-3, got %s", itemsResp.Items[0].EntityId)
	}
}

// TestCollectionsService_CollectionType tests that collection type is stored correctly.
func TestCollectionsService_CollectionType(t *testing.T) {
	service := setupCollectionsService(t)
	ctx := context.Background()

	types := []string{"folder", "playlist", "reading_list", "project", "album"}

	for _, typ := range types {
		resp, err := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
			Name:      fmt.Sprintf("My %sDS", typ),
			OwnerType: "user",
			OwnerId:   "user-type",
			Type:      typ,
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
		Name:      "Batch TestDS",
		OwnerType: "user",
		OwnerId:   "user-batch",
	})
	collID := collResp.Collection.Id

	// Batch add items
	batchAddResp, err := service.BatchAddToCollection(ctx, &v1.BatchAddToCollectionRequest{
		CollectionId: collID,
		Entities: []*commonv1.EntityRef{
			{EntityType: "item", EntityId: "item-ds-1"},
			{EntityType: "item", EntityId: "item-ds-2"},
			{EntityType: "item", EntityId: "item-ds-3"},
		},
		AddedBy: "user-batch",
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
		Entities: []*commonv1.EntityRef{
			{EntityType: "item", EntityId: "item-ds-1"},
			{EntityType: "item", EntityId: "item-ds-4"},
		},
		AddedBy: "user-batch",
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
