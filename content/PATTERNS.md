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
7. [ ] Add tests:
   - [ ] GORM tests in `tests/gorm/`
   - [ ] Datastore tests in `tests/datastore/`
   - [ ] Update main_test.go with new service validation
   - [ ] Add service kinds to cleanup
8. [ ] Update documentation:
   - [ ] Service README.md
   - [ ] content/SUMMARY.md status table
