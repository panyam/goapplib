# Content Organization Services

A set of reusable content organization services that can attach metadata to any entity via the `entity_id` pattern.

## Purpose

These services provide common content organization features that most applications need:

- **LikesService** - Reactions and social engagement
- **TagsService** - Labeling and categorization
- **CollectionsService** - Folders and hierarchical organization
- **NotesService** - Private annotations
- **CommentsService** - Threaded discussions
- **UserActivityService** - Progress, ratings, and bookmarks

## Architecture

All services follow a consistent layered architecture:

```
Service Interface (Public API)
    ↓
BaseService (Shared logic + optional caching)
    ↓                    ↓
gRPC Server          Connect RPC Adapter
(embedded)           (connect.go)
    ↓
StorageProvider Interface (Storage abstraction)
    ↓
Concrete Backends (GORM, Datastore)
```

### gRPC and Connect Support

Services are directly registrable as gRPC servers and include Connect RPC adapters:

```go
// Create service - it's already a gRPC server!
likesService := backends.NewGORMLikesService(db)

// Register directly as gRPC server (no wrapper needed)
grpcServer := grpc.NewServer()
likesv1.RegisterLikesServiceServer(grpcServer, likesService)

// Register as Connect handler (HTTP/2 + JSON support)
mux := http.NewServeMux()
path, handler := likesv1connect.NewLikesServiceHandler(likes.NewConnectLikesServer(likesService))
mux.Handle(path, handler)
```

### Key Patterns

1. **Backend Agnostic**: All services work with GORM (PostgreSQL/MySQL/SQLite) and Google Cloud Datastore
2. **Storage Provider Pattern**: Business logic separated from storage concerns
3. **Optional Caching**: In-memory caching layer available for read-heavy workloads
4. **Denormalized Counts**: Fast read access via pre-aggregated data where appropriate

## Directory Structure

```
content/
├── protos/                     # Protocol buffer definitions
│   ├── buf.yaml               # Buf module configuration
│   ├── buf.gen.yaml           # Code generation configuration
│   ├── common/v1/             # Shared types (Pagination)
│   ├── likes/v1/              # LikesService protos
│   ├── tags/v1/               # TagsService protos
│   ├── collections/v1/        # CollectionsService protos
│   └── ...
│
├── gen/                        # Generated code
│   ├── go/                    # Proto messages, gRPC services, Connect handlers
│   ├── gorm/                  # GORM models and DAL
│   └── datastore/             # Datastore models and DAL
│
├── services/                   # Service implementations
│   ├── likes/                 # LikesService
│   │   ├── service.go         # Interface + BaseService (embeds gRPC server)
│   │   ├── connect.go         # Connect RPC adapter
│   │   ├── README.md          # Service documentation
│   │   └── backends/          # Storage providers
│   │       ├── gorm.go        # GORM backend
│   │       └── gae.go         # Datastore backend
│   └── ...
│
├── tests/                      # Integration tests
│   ├── gorm/                  # GORM backend tests
│   └── datastore/             # Datastore backend tests
│       ├── main_test.go       # Shared TestMain, index validation, utilities
│       ├── likes_test.go      # LikesService tests only
│       ├── tags_test.go       # TagsService tests only
│       └── collections_test.go # CollectionsService tests only
│
└── Makefile                    # Build and test commands
```

## Quick Start

### Generate Code

```bash
cd content
make buf
```

### Run Tests

```bash
# SQLite (default)
make test

# PostgreSQL
make updb && make testpg && make downdb

# Datastore emulator
make upds && make testds && make downds

# Real Google Cloud Datastore
make testrealDS
```

### Datastore Index Management

The Datastore tests validate indexes once at startup in `TestMain` (shared via `main_test.go`). If indexes are missing, the test run will fail with instructions to deploy them:

```bash
# Generate index files
cd content/tests/datastore
go test -run TestMain -args -generate-indexes

# Deploy indexes (note: gcloud requires file to be named index.yaml)
cp tags_index.yaml /tmp/index.yaml && gcloud --project=YOUR_PROJECT datastore indexes create /tmp/index.yaml
```

Indexes are project-wide (not namespace-specific). Wait for indexes to build in GCP Console > Datastore > Indexes before running tests.

## Implementation Status

| Service             | Protos | GORM Backend | Datastore Backend | Hooks | gRPC | Connect | Mount | Tests | Docs |
|---------------------|--------|--------------|-------------------|-------|------|---------|-------|-------|------|
| LikesService        | ✅     | ✅           | ✅                | ✅    | ✅   | ✅      | ✅    | ✅    | ✅   |
| TagsService         | ✅     | ✅           | ✅                | ✅    | ✅   | ✅      | ✅    | ✅    | ✅   |
| CollectionsService  | ✅     | ✅           | ✅                | ✅    | ✅   | ✅      | ✅    | ✅    | ✅   |
| NotesService        | ⏳     | ⏳           | ⏳                | ⏳    | ⏳   | ⏳      | ⏳    | ⏳    | ⏳   |
| CommentsService     | ⏳     | ⏳           | ⏳                | ⏳    | ⏳   | ⏳      | ⏳    | ⏳    | ⏳   |
| UserActivityService | ⏳     | ⏳           | ⏳                | ⏳    | ⏳   | ⏳      | ⏳    | ⏳    | ⏳   |

### Design Simplifications

**Removed `entity_type`**: All services now use only `entity_id` to identify entities. The entity type can be encoded in the ID if needed (e.g., "song:123") or determined by the service/table instance being used.

**Removed `owner_type`**: Collections and Tags services now use only `owner_id` in "type:id" format (e.g., "user:123", "org:456"). This simplifies Datastore indexes by removing one field from composite indexes.

### Hooks Pattern

All services support lifecycle hooks for customization without modifying core code.

**LikesService Example:**
```go
service := backends.NewGORMLikesService(db,
    backends.WithUserIDContextKey(myCtxKey),  // Context key for user ID
    backends.WithOnAuthorize(authHook),       // Authorization check
    backends.WithValidateEntity(validateHook), // Entity validation
    backends.WithBeforeSave(beforeSaveHook),  // Before save
    backends.WithAfterSave(afterSaveHook),    // After save
    backends.WithOnEvent(eventHook),          // Event notifications
)
```

**CollectionsService Example:**
```go
service := backends.NewGORMCollectionsService(db,
    backends.WithOnAuthorize(authHook),            // Authorization check
    backends.WithValidateEntity(validateHook),     // Validate entity before adding
    backends.WithBeforeCollectionSave(beforeHook), // Before collection save
    backends.WithAfterCollectionSave(afterHook),   // After collection save
    backends.WithBeforeItemSave(beforeItemHook),   // Before item save
    backends.WithAfterItemSave(afterItemHook),     // After item save
    backends.WithOnEvent(eventHook),               // Event notifications
)
```

**Available Hooks by Service:**

| Hook                                              | Likes | Tags | Collections | Description                                      |
|---------------------------------------------------|-------|------|-------------|--------------------------------------------------|
| OnAuthorize                                       | ✅    | ✅   | ✅          | Runs first on every RPC; can deny or modify the request |
| ValidateEntity                                    | ✅    | ✅   | ✅          | Validate entity exists before attaching data     |
| BeforeSave / AfterSave                            | ✅    | ✅   | -           | Lifecycle around like/tag save                   |
| BeforeCollectionSave / AfterCollectionSave        | -     | -    | ✅          | Collection lifecycle                             |
| BeforeItemSave / AfterItemSave                    | -     | -    | ✅          | Collection item lifecycle                        |
| BeforeDelete / AfterDelete                        | ✅    | ✅   | ✅          | Lifecycle around delete                          |
| AfterRead / AfterCollectionsRead / AfterItemsRead | ✅    | ✅   | ✅          | Data enrichment after reading                    |
| OnEvent                                           | ✅    | ✅   | ✅          | Event notifications                              |

**Event Types:**
- LikesService: `reaction.added`, `reaction.removed`, `reaction.changed`
- TagsService: `tag.created`, `tag.deleted`, `entity.tagged`, `entity.untagged`
- CollectionsService: `collection.created`, `collection.updated`, `collection.deleted`, `collection.moved`, `item.added`, `item.removed`

**Context-based User ID:**
An interceptor or middleware puts the logged-in user in the context, and the services act as that user:
```go
// In interceptor
ctx = likes.WithUserID(ctx, myCtxKey, "user:123")

// Service uses it automatically
service.AddReaction(ctx, &AddReactionRequest{EntityId: "post:1"})  // UserId auto-filled
```

The user fields of a write (`owner_id`, `user_id`, `added_by`, `tagged_by`) follow `common.ResolveActingUser`:
- With a user in the context, a request naming a different user gets `PermissionDenied`.
- A client call with no user in the context gets `Unauthenticated`.
- An in-process call with no user in the context acts as the user the request names, since server code set it.

`common.WithClientCall` marks a client call. `CombinedMetadataInterceptor`, `UserIDFromMetadataInterceptor`, and each service's `RESTHandler` and `ConnectHandler` mark their requests. An app with its own entry point uses `common.ClientCallInterceptor` or `common.ClientCallMiddleware`.

### Authorization

Every RPC calls `OnAuthorize` first, before validation, with `Operation` set to the RPC's name. `HookContext` carries `CallerID` (the context user, never the request's) and, where the RPC names one, the stored `Collection`/`Parent` or `Tag`/`TargetTag`.

Each service ships `OwnerOnly()`, a policy for user-owned data:
```go
collections.WithOnAuthorize(collections.OwnerOnly())
tags.WithOnAuthorize(tags.OwnerOnly())
likes.WithOnAuthorize(likes.OwnerOnly())
```
It checks client calls and lets in-process calls through. Owners write. Anyone reads a PUBLIC collection or tag, and other private reads get `NotFound`. Lists default to the caller's own data. Like counts and reactors are public. Admin operations (`PromoteTag`, `CreateReactionType`) and unknown operations get `PermissionDenied`. Each function's doc comment has the full rules.

The gRPC port trusts the `x-user-id` metadata it receives, so clients must not reach it. `NewGatewayMux` drops `Grpc-Metadata-X-User-Id` and `Grpc-Metadata-X-Entity-Id` from client headers (`common.TrustedHeaderMatcher`).

### Mountable Services

Services can be mounted at arbitrary paths with entity ID extracted from URL parameters:

```go
// Mount likes at /songs/{songId}/likes
handler := likesService.RESTHandler(ctx, common.WithEntityParam("songId"))
router.Handle("/songs/{songId}/likes/", handler)

// Resulting URLs:
// POST /songs/song123/likes/         -> AddReaction with entity_id=song123
// GET  /songs/song123/likes/counts   -> GetLikeCounts for entity_id=song123
// GET  /songs/song123/likes/reactors -> ListReactors for entity_id=song123
```

For full control over the gRPC-gateway mux (custom marshalers, interceptors):
```go
mux := runtime.NewServeMux(common.TrustedHeaderMatcherOption() /*, your options */)
likesService.RegisterRESTHandlers(ctx, mux)
router.Handle("/songs/{songId}/likes/",
    common.ClientCallMiddleware(common.WrapWithEntityExtraction("songId", mux)))
```

See [PATTERNS.md](PATTERNS.md#mountable-services-pattern) for detailed implementation guide.

### Tags Service Notes

The Tag model uses `name` instead of `key` for the metadata identifier field (e.g., `name="venue"`, `value="Wembley Stadium"`). This avoids a naming collision in the Datastore generator where proto field `key` becomes Go field `Key`, conflicting with the auto-generated `Key *datastore.Key` field.

## See Also

- [GitHub Issues](https://github.com/panyam/goapplib/issues) - Design and tracking
- [goapplib](../) - Parent library
