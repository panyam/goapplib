# TagsService

A flexible tagging service supporting pure labels, metadata tags (name-value pairs), and multi-user tagging for any entity.

## Features

- **Pure Tags**: Simple labels like "Favorites", "Rock", "To Read"
- **Metadata Tags**: Name-value pairs like `venue="Wembley Stadium"`, `year="2024"`
- **Multi-User Tagging**: Multiple users can apply the same tag to an entity
- **Tag Ownership**: Tags belong to users/organizations with privacy scopes
- **Denormalized Counts**: Fast access via pre-aggregated usage counts
- **Inline Tag Creation**: Create tags on-the-fly when tagging entities
- **Backend Agnostic**: Works with GORM (PostgreSQL/MySQL/SQLite) and Google Cloud Datastore

## Architecture

```
TagsService Interface (Public API)
    |
BaseTagsService (Shared logic + caching)
    |
TagsStorageProvider Interface (Storage abstraction)
    |
Concrete Backends
    +-- GORMTagsService (SQL databases)
    +-- DatastoreTagsService (Google Cloud)
```

## Quick Start

### GORM Backend (PostgreSQL/MySQL/SQLite)

```go
import (
    "github.com/panyam/goapplib/content/services/tags/backends"
    "gorm.io/gorm"
)

// Create service
db, _ := gorm.Open(sqlite.Open("test.db"), &gorm.Config{})
service := backends.NewGORMTagsService(db)

// Auto-migrate tables
service.AutoMigrate()

// Create a tag
createResp, err := service.CreateTag(ctx, &v1.CreateTagRequest{
    Value:     "Rock",
    OwnerType: "user",
    OwnerId:   "user-123",
    Scope:     v1.TagScope_TAG_SCOPE_PRIVATE,
})

// Tag an entity
tagResp, err := service.TagEntity(ctx, &v1.TagEntityRequest{
    EntityType: "song",
    EntityId:   "song-456",
    TagId:      createResp.Tag.Id,
    TaggedBy:   "user-123",
})
```

### Datastore Backend (Google Cloud)

```go
import (
    "cloud.google.com/go/datastore"
    "github.com/panyam/goapplib/content/services/tags/backends"
)

client, _ := datastore.NewClient(ctx, "my-project")
service := backends.NewDatastoreTagsService(client, "my-namespace")

// Create a tag with inline tagging
tagResp, err := service.TagEntity(ctx, &v1.TagEntityRequest{
    EntityType: "book",
    EntityId:   "book-123",
    Value:      "To Read",
    OwnerType:  "user",
    OwnerId:    "user-456",
    TaggedBy:   "user-456",
})
```

## API Reference

### Tag Management

| Method       | Description                          |
|--------------|--------------------------------------|
| `CreateTag`  | Create a new tag (returns existing if duplicate) |
| `GetTag`     | Get a tag by ID                      |
| `UpdateTag`  | Update tag properties                |
| `DeleteTag`  | Soft-delete a tag                    |
| `ListTags`   | List tags for an owner               |
| `SearchTags` | Search tags by prefix                |

### Entity Tagging

| Method          | Description                              |
|-----------------|------------------------------------------|
| `TagEntity`     | Apply a tag to an entity (with optional inline creation) |
| `UntagEntity`   | Remove a tag from an entity              |
| `GetEntityTags` | Get all tags for an entity               |

### Advanced Operations

| Method        | Description                              |
|---------------|------------------------------------------|
| `MergeTags`   | Merge multiple tags into one             |
| `PromoteTag`  | Promote a private tag to shared/public   |
| `GetPopularTags` | Get most-used tags                    |

## Tag Types

### Pure Tags (Labels)

Simple labels without a name component:

```go
// Creates a tag with value="Favorites", name=""
service.CreateTag(ctx, &v1.CreateTagRequest{
    Value:     "Favorites",
    OwnerType: "user",
    OwnerId:   "user-123",
})
```

### Metadata Tags (Name-Value Pairs)

Structured tags with both name and value:

```go
// Creates a tag with name="venue", value="Wembley Stadium"
service.CreateTag(ctx, &v1.CreateTagRequest{
    Name:      "venue",
    Value:     "Wembley Stadium",
    OwnerType: "user",
    OwnerId:   "user-123",
})
```

## Tag Scopes

| Scope               | Description                        |
|---------------------|------------------------------------|
| `TAG_SCOPE_PRIVATE` | Only visible to the owner          |
| `TAG_SCOPE_SHARED`  | Visible to organization members    |
| `TAG_SCOPE_PUBLIC`  | Visible to everyone                |

## Multi-User Tagging

Multiple users can apply the same tag to the same entity. Each application is tracked separately via the `tagged_by` field:

```go
// User 1 tags a book
service.TagEntity(ctx, &v1.TagEntityRequest{
    EntityType: "book",
    EntityId:   "book-123",
    TagId:      tagID,
    TaggedBy:   "user-1",
})

// User 2 also tags the same book with the same tag
service.TagEntity(ctx, &v1.TagEntityRequest{
    EntityType: "book",
    EntityId:   "book-123",
    TagId:      tagID,
    TaggedBy:   "user-2",
})

// User 1 removes their tag (User 2's tag remains)
service.UntagEntity(ctx, &v1.UntagEntityRequest{
    EntityType: "book",
    EntityId:   "book-123",
    TagId:      tagID,
    TaggedBy:   "user-1",
})
```

## Data Models

### Tag

```protobuf
message Tag {
  string id = 1;
  string owner_type = 2;       // "user", "org"
  string owner_id = 3;
  string name = 4;             // For metadata tags (optional)
  string normalized_name = 5;
  string value = 6;            // The tag label
  string normalized_value = 7; // Lowercase for deduplication
  TagScope scope = 8;
  TagState state = 9;
  int64 usage_count = 10;      // Denormalized count
  // timestamps...
}
```

### EntityTag

```protobuf
message EntityTag {
  string id = 1;
  string tag_id = 2;
  string entity_type = 3;    // "post", "song", "book"
  string entity_id = 4;
  string tagged_by = 5;      // User who applied this tag
  EntityTagVisibility visibility = 6;
  // timestamps...
}
```

## Database Schema

### GORM Tables

- `tags` - Tag definitions with composite unique index on (owner_type, owner_id, normalized_name, normalized_value)
- `entity_tags` - Tag applications with composite unique index on (tag_id, entity_type, entity_id, tagged_by)

### Datastore Kinds

- `Tag` - Tag definition entities
- `EntityTag` - Tag application entities with composite key

### Datastore Indexes

The Datastore backend requires composite indexes for complex queries. Use the built-in index validation:

```go
service := backends.NewDatastoreTagsService(client, namespace)

// Validate indexes exist (fails fast with helpful error)
if err := service.ValidateIndexes(ctx); err != nil {
    // Prints missing indexes and gcloud command to fix
    log.Fatal(err)
}

// Or export indexes to a file for deployment
service.WriteIndexFile("tags_index.yaml")
// Then deploy: gcloud datastore indexes create tags_index.yaml
```

## Integration Testing

### Running Tests with SQLite (Default)

```bash
cd content
go test ./tests/gorm/... -run TestTagsService
```

### Running Tests with PostgreSQL

```bash
# Start PostgreSQL
make updb

# Run tests
make testpg

# Stop PostgreSQL
make downdb
```

### Running Tests with Datastore Emulator

```bash
# Start emulator
make upds

# Run tests
make testds

# Stop emulator
make downds
```

### Running Tests with Real Datastore

```bash
make testrealDS DS_REAL_PROJECT=my-project DS_REAL_CREDENTIALS=~/creds.json
```

## Best Practices

1. **Use Normalized Values**: Tags are deduplicated using normalized (lowercase) values. "Rock" and "ROCK" are the same tag.

2. **Choose Appropriate Scope**: Start with private tags and promote to shared/public as needed.

3. **Use Inline Creation**: When tagging entities, use the inline creation feature to create tags on-the-fly:
   ```go
   service.TagEntity(ctx, &v1.TagEntityRequest{
       EntityType: "book",
       EntityId:   "book-123",
       Value:      "To Read",  // Creates tag if not exists
       OwnerType:  "user",
       OwnerId:    "user-456",
       TaggedBy:   "user-456",
   })
   ```

4. **Leverage Metadata Tags**: For structured data like locations, dates, or custom attributes, use name-value pairs instead of encoding data in plain tag values.

## Design Notes

The Tag model uses `name` instead of `key` for the metadata identifier field. This avoids a naming collision in the Datastore generator where proto field `key` becomes Go field `Key`, conflicting with the auto-generated `Key *datastore.Key` field.

## See Also

- [content/protos/tags/v1/](../../protos/tags/v1/) - Proto definitions
- [../SUMMARY.md](../SUMMARY.md) - Content services overview
