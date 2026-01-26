# Implementation Patterns and Learnings

Lessons learned from implementing LikesService, TagsService, and CollectionsService that apply to future services.

## Proto Definition

### Avoid `key` as a Field Name

**Problem**: In Datastore-generated code, a proto field named `key` becomes Go field `Key`, which conflicts with the auto-generated `Key *datastore.Key` field.

**Solution**: Use `name` or another alternative for metadata identifier fields.

```protobuf
// BAD - causes conflict in Datastore codegen
message Tag {
  string key = 1;  // "venue"
}

// GOOD
message Tag {
  string name = 1;  // "venue"
}
```

### Minimal Backend Proto Definitions

The `gorm.proto` and `gae.proto` files don't need to redefine every field from `models.proto`. Only include fields that need special handling:

**GORM proto** - Only fields with:
- `primaryKey` tag
- Index definitions
- Serializers (e.g., `serializer:json` for arrays/maps)
- Type overrides (e.g., `type:text` for JSON columns)
- Default values

**Datastore proto** - Only fields with:
- Key exclusion (`datastore_tags: ["-"]` for ID stored in key)
- `noindex` tag for large/unqueried fields

```protobuf
// gorm.proto - minimal definition
message CollectionGORM {
  option (dal.v1.gorm) = {
    source: "content.collections.v1.Collection"
    table: "collections"
  };
  string id = 1 [(dal.v1.column) = { gorm_tags: ["primaryKey"] }];
  // ... only indexed and special fields
  repeated string path = 16 [(dal.v1.column) = { gorm_tags: ["serializer:json", "type:text"] }];
}

// gae.proto - minimal definition
message CollectionDatastore {
  option (dal.v1.datastore) = {
    source: "content.collections.v1.Collection"
    kind: "Collection"
  };
  string id = 1 [(dal.v1.column) = { datastore_tags: ["-"] }];  // Stored in key
  string description = 4 [(dal.v1.column) = { datastore_tags: ["noindex"] }];
}
```

Fields not listed inherit default behavior from the source message.

## Service Constructor Pattern

### Return Error, Don't Panic

Constructors should return `(service, error)` so callers can handle failures gracefully and aggregate errors from multiple services.

```go
// GOOD
func NewDatastoreTagsService(client *datastore.Client, namespace string, options ...ServiceOption) (*DatastoreTagsService, error) {
    // ...
    if opts.ValidateCtx != nil {
        if err := service.EnsureIndexes(opts.ValidateCtx); err != nil {
            return nil, err
        }
    }
    return service, nil
}
```

### Functional Options for Configuration

Use functional options for optional configuration:

```go
// Core options in datastore/indexes.go
type ServiceOption func(*ServiceOptions)

func WithValidation(ctx context.Context) ServiceOption
func WithKindNames(names map[string]string) ServiceOption

// Usage
service, err := NewDatastoreTagsService(client, namespace,
    dsidx.WithValidation(ctx),
    dsidx.WithKindNames(map[string]string{"Tag": "MyApp_Tag"}))
```

### Per-Instance State

Validation flags and configuration should be per-service-instance, not per-package:

```go
type DatastoreTagsService struct {
    // ...
    indexesValidated bool  // Per-instance, not package-level var
    tagKind          string
    entityTagKind    string
}
```

## Datastore Index Management

### Index Provider Interface

Each service implements `IndexProvider` for self-describing indexes:

```go
type IndexProvider interface {
    ServiceName() string
    RequiredIndexes() []DatastoreIndex
    TestQueries() []*datastore.Query
}
```

### RequiredIndexes Must Match Actual Queries

For each query pattern in your storage provider, define a corresponding index. Common patterns:

1. **Equality filters + ordering**: Index needs all equality fields first, then order field
2. **Range/inequality filters**: The inequality field must be last before ordering
3. **Multiple inequalities**: Datastore is limited - may need to restructure queries

Example from SearchTags (equality + range + order):
```go
// Query: owner_type=, owner_id=, status=, normalized_value>=/<, ORDER BY -usage_count
{
    Kind: "Tag",
    Properties: []IndexProperty{
        {Name: "owner_type"},
        {Name: "owner_id"},
        {Name: "status"},
        {Name: "usage_count", Direction: "desc"},
        {Name: "normalized_value"},
    },
}
```

### TestQueries for Validation

Each index needs a corresponding test query that exercises it:

```go
func (s *Service) TestQueries() []*datastore.Query {
    return []*datastore.Query{
        // Must match RequiredIndexes order and structure
        datastore.NewQuery(s.tagKind).
            FilterField("owner_type", "=", "__test__").
            FilterField("owner_id", "=", "__test__").
            FilterField("status", "=", 1).
            FilterField("normalized_value", ">=", "a").
            FilterField("normalized_value", "<", "b").
            Order("-usage_count"),
    }
}
```

### gcloud Index Deployment

- gcloud requires the file to be named `index.yaml`
- Indexes are **project-wide**, not namespace-specific
- Use: `cp service_index.yaml /tmp/index.yaml && gcloud --project=X datastore indexes create /tmp/index.yaml`
- Wait for indexes to build (check GCP Console > Datastore > Indexes)
- "ALREADY_EXISTS" error is OK - existing indexes are skipped

### Emulator vs Real Datastore Differences

The Datastore emulator has stricter query limitations than real Datastore with proper composite indexes:

**1. Inequality Filter + ORDER BY Restriction**

The emulator requires the first ORDER BY property to match the inequality filter property:

```go
// FAILS on emulator (inequality on normalized_value, order by usage_count)
query.FilterField("normalized_value", ">=", "a").
      FilterField("normalized_value", "<", "b").
      Order("-usage_count")

// WORKS on emulator (but no usage_count ordering)
query.FilterField("normalized_value", ">=", "a").
      FilterField("normalized_value", "<", "b")
// Then sort in memory
```

Real Datastore with the proper composite index handles both. For emulator compatibility, skip ORDER BY and sort in memory:

```go
if hasPrefixSearch {
    // Skip ORDER BY for emulator compatibility
    query.FilterField("normalized_value", ">=", prefix)
    // Fetch results, then sort in memory
    sort.Slice(results, func(i, j int) bool {
        return results[i].UsageCount > results[j].UsageCount
    })
}
```

**2. Eventual Consistency on Queries**

The emulator has eventual consistency on queries. For strongly consistent reads, use direct key lookups instead of queries:

```go
// BAD - query may not see recently written entities
func (p *Provider) GetLike(ctx, entityType, entityID, userID string) (*Like, error) {
    query := datastore.NewQuery("Like").
        FilterField("entity_type", "=", entityType).
        FilterField("entity_id", "=", entityID).
        FilterField("user_id", "=", userID)
    // ...
}

// GOOD - direct key lookup is strongly consistent
func (p *Provider) GetLike(ctx, entityType, entityID, userID string) (*Like, error) {
    key := datastore.NameKey("Like", fmt.Sprintf("%s:%s:%s", entityType, entityID, userID), nil)
    err := p.client.Get(ctx, key, &like)
    // ...
}
```

Use deterministic keys based on natural identifiers when possible for strong consistency.

## Test Organization

### Shared TestMain

Put shared test infrastructure in `main_test.go`:

```go
// main_test.go
func TestMain(m *testing.M) {
    flag.Parse()

    if *generateIndexesOnly {
        generateAllIndexFiles()
        os.Exit(0)
    }

    if !*skipIndexValidation {
        if err := validateAllIndexes(); err != nil {
            fmt.Fprintf(os.Stderr, "\n%s\n", err)
            os.Exit(1)
        }
    }

    os.Exit(m.Run())
}
```

### Per-Service Test Files

Each service gets its own test file with:
- Service-specific kind names for cleanup
- Setup function that uses shared `setupTestClient(t, kinds)`

```go
// tags_test.go
var tagsTestKinds = []string{"Tag", "EntityTag", "TagUsageCounts"}

func setupTagsService(t *testing.T) *backends.DatastoreTagsService {
    client := setupTestClient(t, tagsTestKinds)
    namespace := getTestNamespace()
    service, err := backends.NewDatastoreTagsService(client, namespace)
    // ...
}
```

### Namespace Safety

For real Datastore tests, check namespace is empty before running to prevent accidental data loss:

```go
func ensureNamespaceEmpty(t *testing.T, ctx context.Context, client *datastore.Client, namespace string, kinds []string) {
    hasEntities, _ := namespaceHasEntities(ctx, client, namespace, kinds)
    if hasEntities && !*forceDeleteNamespace {
        t.Fatalf("Namespace %q has existing entities. Use -force-delete-ns or different namespace", namespace)
    }
}
```

## Storage Provider Pattern

### Separation of Concerns

```
Service Interface (proto-generated)
    ↓
BaseService (business logic, caching, validation)
    ↓
StorageProvider Interface (CRUD operations)
    ↓
Concrete Backend (GORM, Datastore)
```

### Storage Provider Methods

Keep storage provider methods simple - just CRUD, no business logic:

```go
type TagsStorageProvider interface {
    SaveTag(ctx context.Context, tag *v1.Tag) error
    GetTag(ctx context.Context, id string) (*v1.Tag, error)
    DeleteTag(ctx context.Context, id string) error
    ListTags(ctx context.Context, opts ListTagsOptions) ([]*v1.Tag, int, error)
    // ...
}
```

Business logic (normalization, deduplication, count updates) goes in BaseService.

## Mountable Services Pattern

Services can be mounted at arbitrary user-defined paths with automatic entity ID extraction from URL parameters. Two patterns are supported depending on architecture.

### Problem

Services like LikesService need to work with different entity types (songs, posts, videos). Instead of requiring entity_id in every request body, we want URLs like:
- `POST /songs/{songId}/likes/` - Add reaction to song
- `GET /posts/{postId}/likes/counts` - Get like counts for post

### Pattern A: In-Process (Direct)

Service instance runs in the same process as the HTTP server. Uses `RegisterXYZServiceHandlerServer`.

```
HTTP Request → gRPC-Gateway → Service Instance (same process)
                              ↓
                        Direct method call
```

**Setup:**
```go
// Create service instance
service := backends.NewGORMLikesService(db)

// Option 1: Simple mount with default mux
handler := service.RESTHandler(ctx, common.WithEntityParam("songId"))
router.Handle("/songs/{songId}/likes/", handler)

// Option 2: Custom mux with more control
mux := runtime.NewServeMux(/* custom options */)
service.RegisterRESTHandlers(ctx, mux)
router.Handle("/songs/{songId}/likes/", common.WrapWithEntityExtraction("songId", mux))

// Connect RPC
path, handler := service.ConnectHandler(ctx, common.WithEntityParam("songId"))
router.Handle("/songs/{songId}/rpc/", handler)
```

### Pattern B: Remote Endpoint (gRPC Gateway Client)

HTTP gateway connects to a remote gRPC server. Entity ID flows through gRPC metadata. Uses `RegisterXYZServiceHandlerFromEndpoint`.

```
HTTP Request → gRPC-Gateway → gRPC Client → Network → gRPC Server
                              ↓                        ↓
                        Makes gRPC call         Receives & processes
```

**Gateway side (HTTP server):**
```go
// Option 1: Let common create the mux
mux := common.NewGatewayMux(common.WithEntityParamToMetadata("songId"))
likesv1.RegisterLikesServiceHandlerFromEndpoint(ctx, mux, grpcAddr, opts)
router.Handle("/songs/{songId}/likes/", mux)

// Option 2: Create your own mux with custom options
mux := runtime.NewServeMux(
    common.EntityMetadataOption(common.WithEntityParamToMetadata("songId")),
    runtime.WithErrorHandler(myErrorHandler),
    runtime.WithMarshalerOption("application/json", &runtime.JSONPb{}),
)
likesv1.RegisterLikesServiceHandlerFromEndpoint(ctx, mux, grpcAddr, opts)
router.Handle("/songs/{songId}/likes/", mux)

// Option 3: Combine gateway options with runtime options
mux := common.NewGatewayMuxWithOptions(
    []common.GatewayMuxOption{
        common.WithEntityParamToMetadata("songId"),
        common.WithExtraMetadata(func(ctx context.Context, r *http.Request) metadata.MD {
            return metadata.Pairs("x-request-id", r.Header.Get("X-Request-ID"))
        }),
    },
    runtime.WithErrorHandler(myErrorHandler),
)
```

**gRPC server side:**
```go
// Add interceptor to read entity ID from metadata
grpcServer := grpc.NewServer(
    grpc.UnaryInterceptor(common.EntityIDFromMetadataInterceptor()),
)
likesv1.RegisterLikesServiceServer(grpcServer, likesService)

// Or combine multiple interceptors
grpcServer := grpc.NewServer(
    grpc.ChainUnaryInterceptor(
        common.EntityIDFromMetadataInterceptor(),
        common.UserIDFromMetadataInterceptor(myUserIDKey),
    ),
)
```

### Common Utilities

**Context helpers (`services/common/mount.go`):**
- `WithMountedEntityID(ctx, entityID)` - Store entity ID in context
- `GetMountedEntityID(ctx)` - Retrieve entity ID from context
- `GetEntityIDFromContextOrMetadata(ctx)` - Check both context and gRPC metadata

**HTTP middleware (Pattern A):**
- `WithEntityParam(paramName)` - Handler option to specify path param name
- `EntityParamMiddleware(paramName)` - Middleware that extracts path param
- `WrapWithEntityExtraction(paramName, handler)` - Convenience wrapper
- `ExtractPathParam(r, name)` - Router-agnostic param extraction (Go 1.22+, chi, gorilla)

**Gateway helpers (Pattern B):**
- `NewGatewayMux(opts...)` - Create mux with entity extraction
- `NewGatewayMuxWithOptions(gatewayOpts, runtimeOpts...)` - Create mux with both option types
- `EntityMetadataOption(opts...)` - Returns `runtime.ServeMuxOption` for custom mux
- `WithEntityParamToMetadata(paramName)` - Extract param, send as metadata
- `WithUserParamToMetadata(paramName)` - Extract user ID param
- `WithExtraMetadata(fn)` - Custom metadata extraction

**gRPC interceptors (Pattern B server-side):**
- `EntityIDFromMetadataInterceptor()` - Read entity ID from metadata
- `UserIDFromMetadataInterceptor(contextKey)` - Read user ID from metadata
- `CombinedMetadataInterceptor(userIDKey)` - Read both

**Metadata keys:**
- `MetadataKeyEntityID = "x-entity-id"`
- `MetadataKeyUserID = "x-user-id"`

### Entity ID Resolution in Services

Each service method resolves entity ID from request, context, or metadata:

```go
func (s *BaseLikesService) resolveEntityID(ctx context.Context, requestEntityID string) string {
    if requestEntityID != "" {
        return requestEntityID  // Explicit request wins
    }
    return GetMountedEntityID(ctx)  // Fall back to mounted context
}

func (s *BaseLikesService) AddReaction(ctx context.Context, req *v1.AddReactionRequest) (*v1.AddReactionResponse, error) {
    req.UserId = s.resolveUserID(ctx, req.UserId)
    req.EntityId = s.resolveEntityID(ctx, req.EntityId)
    // ... rest of method
}
```

For Pattern B, the `EntityIDFromMetadataInterceptor` on the gRPC server populates the context before the service method is called.

### Files for Mount Pattern

| File | Purpose |
|------|---------|
| `services/common/mount.go` | Shared context keys, middleware, gateway helpers, interceptors |
| `services/likes/mount.go` | RESTHandler, ConnectHandler factories (Pattern A) |
| `services/likes/hooks.go` | GetMountedEntityID, WithMountedEntityID wrappers |
| `services/likes/service.go` | resolveEntityID method, updated service methods |

## Checklist for New Services

1. [ ] Define protos (avoid `key` field name)
2. [ ] Generate code: `make buf`
3. [ ] Create service interface + BaseService
4. [ ] Create StorageProvider interface
5. [ ] Implement GORM backend
6. [ ] Implement Datastore backend with:
   - [ ] Configurable kind names
   - [ ] RequiredIndexes()
   - [ ] TestQueries()
   - [ ] WriteIndexFile() / IndexesYAML()
7. [ ] Add mount support:
   - [ ] Add `mount.go` with RESTHandler, ConnectHandler
   - [ ] Add resolveEntityID to service.go
   - [ ] Add GetMountedEntityID to hooks.go
8. [ ] Add tests:
   - [ ] GORM tests in `tests/gorm/`
   - [ ] Datastore tests in `tests/datastore/`
   - [ ] Update main_test.go with new service validation
   - [ ] Add service kinds to cleanup
9. [ ] Update documentation:
   - [ ] Service README.md
   - [ ] content/SUMMARY.md status table
