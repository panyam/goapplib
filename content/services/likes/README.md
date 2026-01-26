# LikesService

A high-volume reactions service supporting multiple reaction types (like, love, celebrate, etc.) for any entity.

## Features

- **Multiple Reaction Types**: Support for configurable reactions (like, love, celebrate, etc.)
- **One Reaction Per User Per Entity**: Changing reaction type replaces the previous one
- **Denormalized Counts**: Fast read access via pre-aggregated counts
- **Batch Operations**: Efficient bulk queries for feed-style UIs
- **Backend Agnostic**: Works with GORM (PostgreSQL/MySQL/SQLite) and Google Cloud Datastore

## Architecture

```
LikesService Interface (Public API)
    ↓
BaseLikesService (Shared logic + caching)
    ↓                    ↓
gRPC Server          Connect RPC Adapter
(embedded)           (ConnectLikesServer)
    ↓
LikesStorageProvider Interface (Storage abstraction)
    ↓
Concrete Backends
    ├── GORMLikesService (SQL databases)
    └── DatastoreLikesService (Google Cloud)
```

### gRPC and Connect Support

The service is directly registrable as a gRPC server and includes a Connect RPC adapter:

```go
import (
    "github.com/panyam/goapplib/content/services/likes"
    "github.com/panyam/goapplib/content/services/likes/backends"
    likesv1 "github.com/panyam/goapplib/content/gen/go/likes/v1"
    "github.com/panyam/goapplib/content/gen/go/likes/v1/likesv1connect"
)

// Create service
service := backends.NewGORMLikesService(db)

// Register as gRPC server
grpcServer := grpc.NewServer()
likesv1.RegisterLikesServiceServer(grpcServer, service)

// Register as Connect handler (HTTP/2 + JSON)
mux := http.NewServeMux()
path, handler := likesv1connect.NewLikesServiceHandler(likes.NewConnectLikesServer(service))
mux.Handle(path, handler)
```

## Quick Start

### GORM Backend (PostgreSQL/MySQL/SQLite)

```go
import (
    "github.com/panyam/goapplib/content/services/likes/backends"
    "gorm.io/gorm"
)

// Create service
db, _ := gorm.Open(sqlite.Open("test.db"), &gorm.Config{})
service := backends.NewGORMLikesService(db)

// Auto-migrate tables
service.AutoMigrate()

// Enable caching (optional)
service.InitializeCache()

// Add a reaction
resp, err := service.AddReaction(ctx, &v1.AddReactionRequest{
    EntityId:     "post-123",
    UserId:       "user-456",
    ReactionType: "like",
})
```

### Datastore Backend (Google Cloud)

```go
import (
    "cloud.google.com/go/datastore"
    "github.com/panyam/goapplib/content/services/likes/backends"
)

client, _ := datastore.NewClient(ctx, "my-project")
service := backends.NewDatastoreLikesService(client, "my-namespace")

// Enable caching (optional)
service.InitializeCache()

// Add a reaction
resp, err := service.AddReaction(ctx, &v1.AddReactionRequest{
    EntityId:     "post-123",
    UserId:       "user-456",
    ReactionType: "love",
})
```

## API Reference

### Core Operations

| Method           | Description                                  |
|------------------|----------------------------------------------|
| `AddReaction`    | Add or update a user's reaction to an entity |
| `RemoveReaction` | Remove a user's reaction from an entity      |
| `ToggleReaction` | Toggle a reaction on/off                     |

### Query Operations

| Method              | Description                                |
|---------------------|--------------------------------------------|
| `GetUserReaction`   | Get a user's current reaction on an entity |
| `GetLikeCounts`     | Get aggregated counts for an entity        |
| `ListReactors`      | List users who reacted to an entity        |
| `ListUserReactions` | List all reactions by a user               |

### Batch Operations

| Method                  | Description                                  |
|-------------------------|----------------------------------------------|
| `BatchGetUserReactions` | Get a user's reactions for multiple entities |
| `BatchGetLikeCounts`    | Get counts for multiple entities             |

### Reaction Type Management

| Method               | Description                        |
|----------------------|------------------------------------|
| `CreateReactionType` | Create a new reaction type (admin) |
| `ListReactionTypes`  | List available reaction types      |

## Data Models

### Like

```protobuf
message Like {
  string id = 1;
  string entity_type = 2;    // "post", "comment", etc.
  string entity_id = 3;
  string user_id = 4;
  string reaction_type = 5;  // "like", "love", etc.
  // timestamps...
}
```

### LikeCounts

```protobuf
message LikeCounts {
  string entity_type = 1;
  string entity_id = 2;
  int64 total_count = 3;
  map<string, int64> by_reaction_type = 4;  // {"like": 10, "love": 3}
}
```

### ReactionType

```protobuf
message ReactionType {
  string id = 1;           // "like", "love"
  string name = 2;         // Display name
  string emoji = 3;        // "👍", "❤️"
  int32 display_order = 4;
  bool is_default = 5;
}
```

## Database Schema

### GORM Tables

- `likes` - Individual reactions with composite unique index on (entity_type, entity_id, user_id)
- `like_counts` - Denormalized counts with composite primary key (entity_type, entity_id)
- `reaction_types` - Configurable reaction definitions

### Datastore Kinds

- `Like` - Individual reaction entities
- `LikeCounts` - Denormalized count entities (uses PropertyLoadSaver for map serialization)
- `ReactionType` - Reaction type definitions

Note: The `LikeCounts` entity uses `implement_property_loader: true` in the proto to generate PropertyLoadSaver methods that serialize the `map<string, int64>` field as JSON, since Datastore doesn't natively support Go maps.

### Datastore Indexes

The Datastore backend requires composite indexes for complex queries. Enable validation at construction:

```go
import dsidx "github.com/panyam/goapplib/datastore"

// Validate indexes during construction (recommended for production)
service, err := backends.NewDatastoreLikesService(client, namespace,
    dsidx.WithValidation(ctx))
if err != nil {
    // Error includes missing indexes + gcloud command to fix
    // Also writes likes_index.yaml automatically
    log.Fatal(err)
}

// Or skip validation (e.g., for emulator)
service, err := backends.NewDatastoreLikesService(client, namespace)
```

To deploy indexes manually:
```bash
gcloud datastore indexes create likes_index.yaml
```

## Integration Testing

### Running Tests with SQLite (Default)

```bash
cd content
go test ./tests/gorm/...
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

## Caching

The service includes an optional in-memory cache for like counts:

```go
service.InitializeCache()
```

Cache is automatically invalidated when reactions are added/removed.

## Best Practices

1. **Use Batch Operations**: When displaying feeds, use `BatchGetLikeCounts` to fetch counts for multiple items in one call.

2. **Pre-create Reaction Types**: Create your reaction types at application startup rather than on-demand.

3. **Enable Caching**: For read-heavy workloads, enable the counts cache to reduce database load.

4. **Consider Rate Limiting**: Implement rate limiting for reaction endpoints to prevent abuse.

## See Also

- [GitHub Issue #3](https://github.com/panyam/goapplib/issues/3) - Original design
- [content/protos/likes/v1/](../../protos/likes/v1/) - Proto definitions
