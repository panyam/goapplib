# Content Organization Services

A set of reusable content organization services that can attach metadata to any entity via the `entity_type + entity_id` pattern.

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
│   ├── tags/v1/               # TagsService protos (planned)
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
```

## Implementation Status

| Service | Protos | GORM Backend | Datastore Backend | Tests | Docs |
|---------|--------|--------------|-------------------|-------|------|
| LikesService | ✅ | ✅ | ✅ | ✅ | ✅ |
| TagsService | ⏳ | ⏳ | ⏳ | ⏳ | ⏳ |
| CollectionsService | ⏳ | ⏳ | ⏳ | ⏳ | ⏳ |
| NotesService | ⏳ | ⏳ | ⏳ | ⏳ | ⏳ |
| CommentsService | ⏳ | ⏳ | ⏳ | ⏳ | ⏳ |
| UserActivityService | ⏳ | ⏳ | ⏳ | ⏳ | ⏳ |

## See Also

- [GitHub Issues](https://github.com/panyam/goapplib/issues) - Design and tracking
- [goapplib](../) - Parent library
