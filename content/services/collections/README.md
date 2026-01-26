# CollectionsService

A hierarchical collections service for organizing entities into folders, playlists, albums, and other groupings with support for nesting, multi-parent membership, and efficient subtree queries.

## Features

- **Hierarchical Collections**: Create nested folder structures with configurable max depth
- **Multi-Parent Support**: Entities can belong to multiple collections simultaneously
- **Path-Based Queries**: Efficient subtree operations using ancestor path arrays
- **Flexible Collection Types**: folder, playlist, reading_list, project, album, or custom types
- **Denormalized Counts**: Fast access via pre-aggregated item and child counts
- **Ordered Items**: Control item display order within collections
- **Backend Agnostic**: Works with GORM (PostgreSQL/MySQL/SQLite) and Google Cloud Datastore

## Architecture

```
CollectionsService Interface (Public API)
    |
BaseCollectionsService (Shared logic)
    |                    |
gRPC Server          Connect RPC Adapter
(embedded)           (ConnectCollectionsServer)
    |
CollectionsStorageProvider Interface (Storage abstraction)
    |
Concrete Backends
    +-- GORMCollectionsService (SQL databases)
    +-- DatastoreCollectionsService (Google Cloud)
```

### gRPC and Connect Support

The service is directly registrable as a gRPC server and includes a Connect RPC adapter:

```go
import (
    "github.com/panyam/goapplib/content/services/collections"
    "github.com/panyam/goapplib/content/services/collections/backends"
    collectionsv1 "github.com/panyam/goapplib/content/gen/go/collections/v1"
    "github.com/panyam/goapplib/content/gen/go/collections/v1/collectionsv1connect"
)

// Create service
service := backends.NewGORMCollectionsService(db)

// Register as gRPC server
grpcServer := grpc.NewServer()
collectionsv1.RegisterCollectionsServiceServer(grpcServer, service)

// Register as Connect handler (HTTP/2 + JSON)
mux := http.NewServeMux()
path, handler := collectionsv1connect.NewCollectionsServiceHandler(collections.NewConnectCollectionsServer(service))
mux.Handle(path, handler)
```

## Quick Start

### GORM Backend (PostgreSQL/MySQL/SQLite)

```go
import (
    "github.com/panyam/goapplib/content/services/collections/backends"
    "gorm.io/gorm"
)

// Create service
db, _ := gorm.Open(sqlite.Open("test.db"), &gorm.Config{})
service := backends.NewGORMCollectionsService(db)

// Auto-migrate tables
service.AutoMigrate()

// Create a folder
collResp, err := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
    Name:       "My Projects",
    OwnerId:    "user-123",
    Type:       "folder",
    Visibility: v1.CollectionVisibility_COLLECTION_VISIBILITY_PRIVATE,
})

// Add an entity to the collection
addResp, err := service.AddToCollection(ctx, &v1.AddToCollectionRequest{
    CollectionId: collResp.Collection.Id,
    EntityId:     "doc-456",
    AddedBy:      "user-123",
})
```

### Datastore Backend (Google Cloud)

```go
import (
    "cloud.google.com/go/datastore"
    "github.com/panyam/goapplib/content/services/collections/backends"
)

client, _ := datastore.NewClient(ctx, "my-project")
service := backends.NewDatastoreCollectionsService(client, "my-namespace")

// Create a nested collection
childResp, err := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
    Name:      "Subfolder",
    OwnerId:   "user-123",
    ParentId:  parentCollectionID,  // Nested under parent
})
```

## API Reference

### Collection Management

| Method             | Description                                      |
|--------------------|--------------------------------------------------|
| `CreateCollection` | Create a new collection (returns existing if duplicate name) |
| `GetCollection`    | Get a collection by ID                           |
| `UpdateCollection` | Update collection properties                     |
| `DeleteCollection` | Delete a collection (with force option for non-empty) |
| `ListCollections`  | List collections with filtering and pagination   |

### Hierarchy Operations

| Method               | Description                                    |
|----------------------|------------------------------------------------|
| `GetCollectionTree`  | Get collection with nested children            |
| `MoveCollection`     | Move collection to a new parent (updates paths)|
| `GetCollectionPath`  | Get breadcrumb path (ancestors) to a collection|

### Item Management

| Method                 | Description                                  |
|------------------------|----------------------------------------------|
| `AddToCollection`      | Add an entity to a collection                |
| `RemoveFromCollection` | Remove an entity from a collection           |
| `GetCollectionItems`   | Get items in a collection with pagination    |
| `GetEntityCollections` | Get all collections containing an entity     |
| `ReorderItems`         | Update display order of items                |

### Batch Operations

| Method                       | Description                              |
|------------------------------|------------------------------------------|
| `BatchAddToCollection`       | Add multiple entities to a collection    |
| `BatchGetEntityCollections`  | Get collections for multiple entities    |

## Collection Types

Collection types are simple strings stored as metadata. The service does not enforce behavior differences between types:

| Type           | Typical Use                          |
|----------------|--------------------------------------|
| `folder`       | General-purpose organization         |
| `playlist`     | Ordered media collections            |
| `reading_list` | Saved articles/books                 |
| `project`      | Project-related groupings            |
| `album`        | Photo/media albums                   |
| *(custom)*     | Any application-specific type        |

## Hierarchy and Paths

### Creating Nested Collections

```go
// Create root collection
root, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
    Name:      "Documents",
    OwnerId:   "user-123",
})
// root.Depth = 0, root.Path = []

// Create child collection
child, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
    Name:      "Work",
    OwnerId:   "user-123",
    ParentId:  root.Collection.Id,
})
// child.Depth = 1, child.Path = [root.Id]

// Create grandchild
grandchild, _ := service.CreateCollection(ctx, &v1.CreateCollectionRequest{
    Name:      "Projects",
    OwnerId:   "user-123",
    ParentId:  child.Collection.Id,
})
// grandchild.Depth = 2, grandchild.Path = [root.Id, child.Id]
```

### Getting Breadcrumb Path

```go
pathResp, _ := service.GetCollectionPath(ctx, &v1.GetCollectionPathRequest{
    Id: grandchildID,
})
// pathResp.Ancestors = [root, child, grandchild]
// Use for breadcrumb navigation: Documents > Work > Projects
```

### Moving Collections

```go
// Move "Projects" from Work to a new parent
moveResp, _ := service.MoveCollection(ctx, &v1.MoveCollectionRequest{
    Id:          projectsID,
    NewParentId: newParentID,
})
// All descendant paths are automatically updated
```

Circular references are automatically prevented:

```go
// Given hierarchy: A -> B -> C
// Trying to move A under C fails
_, err := service.MoveCollection(ctx, &v1.MoveCollectionRequest{
    Id:          collectionA_ID,
    NewParentId: collectionC_ID,
})
// err = "circular reference detected"
```

## Item Ordering

Items have a `display_order` field for custom ordering:

```go
// Add items (auto-assigned sequential display_order)
service.AddToCollection(ctx, &v1.AddToCollectionRequest{
    CollectionId: collID,
    EntityId:     "song-1",
    AddedBy:      "user-123",
})
// display_order = 1

service.AddToCollection(ctx, &v1.AddToCollectionRequest{
    CollectionId: collID,
    EntityId:     "song-2",
    AddedBy:      "user-123",
})
// display_order = 2

// Reorder items
service.ReorderItems(ctx, &v1.ReorderItemsRequest{
    CollectionId: collID,
    ItemOrders: []*v1.ItemOrder{
        {EntityId: "song-2", DisplayOrder: 1},
        {EntityId: "song-1", DisplayOrder: 2},
    },
})
```

### Sorting Options

When listing items, you can sort by:
- `SORT_FIELD_DISPLAY_ORDER` - Manual order (default)
- `SORT_FIELD_ADDED_AT` - Chronological order

```go
itemsResp, _ := service.GetCollectionItems(ctx, &v1.GetCollectionItemsRequest{
    CollectionId:   collID,
    SortBy:         v1.SortField_SORT_FIELD_ADDED_AT,
    SortDescending: true,  // Newest first
})
```

For sorting by entity properties (name, rating, etc.), use your application's search layer (e.g., Elasticsearch).

## Data Models

### Collection

```protobuf
message Collection {
  string id = 1;
  string name = 2;              // Display name
  string normalized_name = 3;   // Lowercase for deduplication
  string description = 4;
  string owner_id = 6;
  string parent_id = 7;         // Parent collection (empty = root)
  repeated string path = 8;     // Ancestor IDs for subtree queries
  int32 depth = 9;              // Hierarchy level (0 = root)
  int32 display_order = 10;     // Order among siblings
  string type = 11;             // folder, playlist, etc.
  string icon = 12;
  string color = 13;
  int64 item_count = 14;        // Denormalized
  int64 child_count = 15;       // Denormalized
  CollectionVisibility visibility = 16;
  CollectionStatus status = 17;
  // timestamps, creator_id...
}
```

### CollectionItem

```protobuf
message CollectionItem {
  string collection_id = 1;
  string entity_type = 2;       // "document", "song", "photo"
  string entity_id = 3;
  int32 display_order = 4;
  string added_by = 5;
  google.protobuf.Timestamp added_at = 6;
  string metadata = 7;          // Optional JSON
}
```

## Database Schema

### GORM Tables

- `collections` - Collection definitions with indexes on owner, parent, path
- `collection_items` - Item memberships with composite primary key

### Datastore Kinds

- `Collection` - Collection definition entities
- `CollectionItem` - Item membership entities with deterministic key

### Datastore Indexes

The Datastore backend requires composite indexes. Enable validation:

```go
import dsidx "github.com/panyam/goapplib/datastore"

// Validate indexes during construction
service, err := backends.NewDatastoreCollectionsService(client, namespace,
    dsidx.WithValidation(ctx))
if err != nil {
    log.Fatal(err)  // Includes missing indexes + fix command
}
```

To deploy indexes:
```bash
gcloud datastore indexes create collections_index.yaml
```

## Visibility Levels

| Visibility                      | Description                  |
|---------------------------------|------------------------------|
| `COLLECTION_VISIBILITY_PRIVATE` | Only visible to owner        |
| `COLLECTION_VISIBILITY_SHARED`  | Visible to collaborators     |
| `COLLECTION_VISIBILITY_PUBLIC`  | Visible to everyone          |

## Deleting Collections

```go
// Delete empty collection
service.DeleteCollection(ctx, &v1.DeleteCollectionRequest{
    Id: collID,
})

// Delete non-empty collection (requires ForceDelete)
service.DeleteCollection(ctx, &v1.DeleteCollectionRequest{
    Id:          collID,
    ForceDelete: true,  // Removes all items
})

// Delete with recursive option (also removes child collections)
service.DeleteCollection(ctx, &v1.DeleteCollectionRequest{
    Id:        collID,
    Recursive: true,
})
```

## Integration Testing

### Running Tests with SQLite (Default)

```bash
go test ./content/tests/gorm/... -run TestCollections
```

### Running Tests with Datastore Emulator

```bash
# Start emulator
gcloud beta emulators datastore start --no-store-on-disk

# In another terminal
$(gcloud beta emulators datastore env-init)
go test ./content/tests/datastore/... -run TestCollections
```

## Best Practices

1. **Use Normalized Names**: Collections are deduplicated within the same parent using normalized names. "My Folder" and "MY FOLDER" are treated as the same collection.

2. **Leverage Multi-Parent**: Instead of duplicating entities, add them to multiple collections:
   ```go
   // Add document to both "Work" and "Favorites" collections
   service.AddToCollection(ctx, &v1.AddToCollectionRequest{
       CollectionId: workCollID,
       EntityId:     "doc-123",
       AddedBy:      "user-1",
   })
   service.AddToCollection(ctx, &v1.AddToCollectionRequest{
       CollectionId: favoritesCollID,
       EntityId:     "doc-123",
       AddedBy:      "user-1",
   })
   ```

3. **Keep Hierarchies Shallow**: While the service supports deep nesting, shallow hierarchies (3-4 levels) provide better UX and performance.

4. **Use Batch Operations**: For bulk operations, use `BatchAddToCollection` and `BatchGetEntityCollections` instead of individual calls.

## See Also

- [content/protos/collections/v1/](../../protos/collections/v1/) - Proto definitions
- [../SUMMARY.md](../SUMMARY.md) - Content services overview
