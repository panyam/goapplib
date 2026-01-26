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
    ↓
StorageProvider Interface (Storage abstraction)
    ↓
Concrete Backends (GORM, Datastore)
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
│   ├── common/v1/             # Shared types (EntityRef, Pagination)
│   ├── likes/v1/              # LikesService protos
│   ├── tags/v1/               # TagsService protos
│   ├── collections/v1/        # CollectionsService protos
│   └── ...
│
├── gen/                        # Generated code
│   ├── go/                    # Proto messages and services
│   ├── gorm/                  # GORM models and DAL
│   └── datastore/             # Datastore models and DAL
│
├── services/                   # Service implementations
│   ├── likes/                 # LikesService
│   │   ├── service.go         # Interface + BaseService
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

| Service | Protos | GORM Backend | Datastore Backend | Tests | Docs |
|---------|--------|--------------|-------------------|-------|------|
| LikesService | ✅ | ✅ | ✅ | ✅ | ✅ |
| TagsService | ✅ | ✅ | ✅ | ✅ | ✅ |
| CollectionsService | ✅ | ✅ | ✅ | ✅ | ✅ |
| NotesService | ⏳ | ⏳ | ⏳ | ⏳ | ⏳ |
| CommentsService | ⏳ | ⏳ | ⏳ | ⏳ | ⏳ |
| UserActivityService | ⏳ | ⏳ | ⏳ | ⏳ | ⏳ |

### Design Simplifications

**Removed `entity_type`**: All services now use only `entity_id` to identify entities. The entity type can be encoded in the ID if needed (e.g., "song:123") or determined by the service/table instance being used.

**Removed `owner_type`**: Collections and Tags services now use only `owner_id` in "type:id" format (e.g., "user:123", "org:456"). This simplifies Datastore indexes by removing one field from composite indexes.

### Tags Service Notes

The Tag model uses `name` instead of `key` for the metadata identifier field (e.g., `name="venue"`, `value="Wembley Stadium"`). This avoids a naming collision in the Datastore generator where proto field `key` becomes Go field `Key`, conflicting with the auto-generated `Key *datastore.Key` field.

## See Also

- [GitHub Issues](https://github.com/panyam/goapplib/issues) - Design and tracking
- [goapplib](../) - Parent library
