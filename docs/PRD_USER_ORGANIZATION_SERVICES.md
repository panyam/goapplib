# Product Requirements Document: User Organization & Collaboration Services

**Document Version:** 1.0
**Date:** January 22, 2026
**Author:** Engineering Team
**Status:** Draft

---

## 1. Executive Summary

### 1.1 Overview
This PRD defines a suite of common services for goapplib that enable user organization, content management, and collaboration features. These services are derived from analysis of successful platforms including Zotero, Notion, Evernote, Google Drive, GitHub, and Raindrop.io.

### 1.2 Scope
Add seven new services to goapplib with implementations for all three backends (fs, gorm, gae):
1. **TagsService** - Flexible labeling and categorization
2. **CollectionsService** - Hierarchical content organization
3. **OrganizationsService** - Teams and group management
4. **PermissionsService** - Role-based access control
5. **SharingService** - Resource sharing mechanisms
6. **NotesService** - Notes and annotations
7. **UserActivityService** - Personal annotations, interactions, and activity tracking

### 1.3 Success Criteria
- All services implemented with fs, gorm, and gae backends
- Services follow established goapplib patterns (proto-driven, DAL generation)
- 90%+ test coverage for core operations
- Performance: <100ms for single entity operations, <500ms for list operations

---

## 2. Problem Statement

### 2.1 Current State
goapplib currently provides:
- UsersService with multi-backend support
- Basic tag field on User model (string array)
- No organization/team structures
- No sharing or permission mechanisms
- No collection/folder organization
- No notes/annotation support

### 2.2 Gap Analysis

| Capability | Current State | Market Standard | Gap |
|------------|---------------|-----------------|-----|
| Tagging | Basic string array on users | Colored, hierarchical, searchable tags | High |
| Organization | None | Multi-parent collections, folders | Critical |
| Teams/Groups | None | Orgs, teams, membership roles | Critical |
| Permissions | None | RBAC, resource-level permissions | Critical |
| Sharing | None | Links, invites, public/private | High |
| Notes | None | Attached/standalone notes | Medium |

### 2.3 User Pain Points
1. **Developers building SaaS apps** need team/organization support out-of-box
2. **Content applications** require hierarchical organization
3. **Collaborative apps** need permission and sharing primitives
4. **Knowledge management apps** need tagging and notes

---

## 3. Goals and Non-Goals

### 3.1 Goals
- **G1**: Provide reusable, production-ready services for common user features
- **G2**: Maintain consistency with existing goapplib patterns
- **G3**: Support all three backends (fs, gorm, gae) from day one
- **G4**: Enable applications to compose services as needed
- **G5**: Minimize coupling between services while allowing integration

### 3.2 Non-Goals
- **NG1**: Full-text search (use external search services)
- **NG2**: Real-time collaboration (use external real-time services)
- **NG3**: File storage (use existing storage services)
- **NG4**: Billing/subscription management
- **NG5**: Notifications (separate concern)

---

## 4. Competitive Analysis

### 4.1 Zotero
**Strengths:**
- Colored tags with keyboard shortcuts (1-9)
- Automatic vs manual tag distinction
- Collections with multi-parent support
- Group libraries with 3 permission levels
- Built-in PDF annotation

**Weaknesses:**
- No hierarchical tags (requires plugin)
- 300MB free storage limit
- Sync-based, not real-time
- Groups are separate copies (not linked)

### 4.2 Notion
**Strengths:**
- Real-time collaboration
- Flexible database properties (tags)
- Teamspaces for departments
- 4+ permission levels
- @mentions and assignments

**Weaknesses:**
- Complex permission model
- No native hierarchical tags
- Performance issues at scale

### 4.3 Google Drive
**Strengths:**
- 5-tier permission system
- Shared drives (org-owned)
- 15GB free storage
- Target audiences for smart sharing
- Deep admin controls

**Weaknesses:**
- No tagging (only labels)
- Single-parent folder structure
- No annotation support

### 4.4 GitHub
**Strengths:**
- Most sophisticated permission model
- Custom roles with granular permissions
- Organization > Teams > Repos hierarchy
- Outside collaborators concept
- Permission inheritance

**Weaknesses:**
- Code-focused, not general purpose
- Complex for non-technical users

### 4.5 Feature Comparison Matrix

| Feature | Zotero | Notion | Google | GitHub | **Proposed** |
|---------|--------|--------|--------|--------|--------------|
| Tags | Colored | Properties | Labels | Labels | Colored, typed |
| Hierarchical Tags | Plugin | Relations | No | No | Native |
| Multi-parent | Yes | Yes | No | No | Yes |
| Permission Levels | 3 | 4+ | 5 | 5+ custom | Configurable |
| Custom Roles | No | Limited | No | Yes | Yes |
| Share Links | No | Yes | Yes | Limited | Yes |
| Link Expiration | No | No | No | No | Yes |
| Notes | Yes | Pages | No | Gists | Yes |
| Relations | Yes | Yes | No | Cross-refs | Yes |

---

## 5. User Stories

### 5.1 TagsService

| ID | As a... | I want to... | So that... | Priority |
|----|---------|--------------|------------|----------|
| T1 | Developer | Create tags with colors | Users can visually categorize items | P0 |
| T2 | User | Tag multiple items at once | I can organize efficiently | P0 |
| T3 | User | Filter items by tags | I can find related content | P0 |
| T4 | User | See tag suggestions | I maintain consistent tagging | P1 |
| T5 | Admin | Merge duplicate tags | The tag system stays clean | P1 |
| T6 | User | Use keyboard shortcuts for tags | I can tag quickly | P2 |

### 5.2 CollectionsService

| ID | As a... | I want to... | So that... | Priority |
|----|---------|--------------|------------|----------|
| C1 | User | Create nested collections | I can organize hierarchically | P0 |
| C2 | User | Add items to multiple collections | Items aren't duplicated | P0 |
| C3 | User | Create smart/saved searches | Collections auto-update | P1 |
| C4 | User | Drag items between collections | Organization is intuitive | P1 |
| C5 | User | See collection item counts | I know what's where | P0 |
| C6 | User | Reorder collections | Display matches my workflow | P2 |

### 5.3 OrganizationsService

| ID | As a... | I want to... | So that... | Priority |
|----|---------|--------------|------------|----------|
| O1 | User | Create an organization | I can collaborate with others | P0 |
| O2 | Admin | Invite members | People can join my org | P0 |
| O3 | Admin | Set member roles | Access is controlled | P0 |
| O4 | User | See my organizations | I can switch contexts | P0 |
| O5 | Admin | Create teams within org | I can group members | P1 |
| O6 | Owner | Transfer ownership | Continuity is maintained | P1 |
| O7 | Admin | Set org visibility | I control discoverability | P1 |

### 5.4 PermissionsService

| ID | As a... | I want to... | So that... | Priority |
|----|---------|--------------|------------|----------|
| P1 | Developer | Define custom roles | Permissions match my app | P0 |
| P2 | Admin | Grant resource permissions | Access is controlled | P0 |
| P3 | Developer | Check permissions efficiently | App stays performant | P0 |
| P4 | Admin | See who has access | I can audit permissions | P1 |
| P5 | Admin | Set permission expiration | Temporary access works | P2 |
| P6 | User | See my permissions | I know what I can do | P1 |

### 5.5 SharingService

| ID | As a... | I want to... | So that... | Priority |
|----|---------|--------------|------------|----------|
| S1 | User | Share via link | Recipients don't need accounts | P0 |
| S2 | User | Share with specific users | Only intended recipients access | P0 |
| S3 | User | Set link expiration | Shares don't persist forever | P1 |
| S4 | User | Password protect shares | Extra security is available | P2 |
| S5 | User | See share analytics | I know who accessed | P2 |
| S6 | Admin | Revoke shares | I can control access | P0 |

### 5.6 NotesService

| ID | As a... | I want to... | So that... | Priority |
|----|---------|--------------|------------|----------|
| N1 | User | Create standalone notes | I can capture thoughts | P0 |
| N2 | User | Attach notes to items | Context is preserved | P0 |
| N3 | User | Search note content | I can find information | P1 |
| N4 | User | Version note history | I can recover changes | P2 |
| N5 | User | Pin important notes | Quick access is available | P1 |
| N6 | User | Color-code notes | Visual organization helps | P2 |

### 5.7 UserActivityService

**Purpose**: Track private, personal interactions with content - distinct from shared tags/notes.

**Examples of Personal Activities:**
- "I completed workout #567 on Tuesday - it was hard"
- "I read this article on Jan 15"
- "Rating: 4/5 stars"
- "I tried this recipe - substituted butter for oil"
- "Bookmarked for later review"
- "Progress: 60% complete"
- "Last accessed 3 days ago"

| ID | As a... | I want to... | So that... | Priority |
|----|---------|--------------|------------|----------|
| UA1 | User | Record when I interacted with content | I can track my history | P0 |
| UA2 | User | Add private notes to any content | I remember my personal context | P0 |
| UA3 | User | Rate/score content privately | I can remember what I liked | P1 |
| UA4 | User | Track progress on content | I know where I left off | P1 |
| UA5 | User | See my activity history | I can review what I've done | P0 |
| UA6 | User | Filter content by my activity | I find things I've interacted with | P1 |
| UA7 | User | Set personal status on content | I can track my workflow | P1 |
| UA8 | User | Keep activities private from shared content | My personal notes stay private | P0 |
| UA9 | User | Export my activity history | I own my data | P2 |

---

## 6. Functional Requirements

### 6.1 TagsService

#### 6.1.1 Tag Management
- **FR-T1**: Create tag with name, color (optional), display order
- **FR-T2**: Update tag name, color, order
- **FR-T3**: Delete tag (with option to remove from all entities)
- **FR-T4**: Merge two tags into one
- **FR-T5**: List tags with filtering and pagination
- **FR-T6**: Support tag scopes: user-level, org-level, global

#### 6.1.2 Entity Tagging
- **FR-T7**: Add tag to entity (any entity type)
- **FR-T8**: Remove tag from entity
- **FR-T9**: Get all tags for an entity
- **FR-T10**: Get all entities with a tag (with pagination)
- **FR-T11**: Bulk tag multiple entities
- **FR-T12**: Track tag usage counts

#### 6.1.3 Tag Discovery
- **FR-T13**: Suggest tags based on entity content/similar entities
- **FR-T14**: Get tag statistics (usage count, trend)
- **FR-T15**: Search tags by name prefix (autocomplete)

### 6.2 CollectionsService

#### 6.2.1 Collection Management
- **FR-C1**: Create collection with name, description, parent (optional)
- **FR-C2**: Update collection name, description, icon, color
- **FR-C3**: Delete collection (with option for children)
- **FR-C4**: Move collection to different parent
- **FR-C5**: Get collection tree (full hierarchy)
- **FR-C6**: Support max nesting depth configuration

#### 6.2.2 Collection Items
- **FR-C7**: Add entity to collection (any entity type)
- **FR-C8**: Remove entity from collection
- **FR-C9**: Get collection items (with pagination)
- **FR-C10**: Get all collections containing an entity
- **FR-C11**: Bulk add entities to collection
- **FR-C12**: Reorder items within collection

#### 6.2.3 Smart Collections
- **FR-C13**: Create smart collection with query criteria
- **FR-C14**: Auto-refresh smart collection membership
- **FR-C15**: Support query operators: AND, OR, NOT, comparisons

### 6.3 OrganizationsService

#### 6.3.1 Organization Management
- **FR-O1**: Create organization with name, slug, visibility
- **FR-O2**: Update organization details
- **FR-O3**: Delete organization (with confirmation)
- **FR-O4**: Get organization by ID or slug
- **FR-O5**: List organizations (public, user's orgs)

#### 6.3.2 Membership
- **FR-O6**: Invite member by email
- **FR-O7**: Accept/decline invitation
- **FR-O8**: Remove member
- **FR-O9**: Update member role (owner, admin, member)
- **FR-O10**: List organization members
- **FR-O11**: Get user's organizations

#### 6.3.3 Teams
- **FR-O12**: Create team within organization
- **FR-O13**: Update team details
- **FR-O14**: Delete team
- **FR-O15**: Add/remove team members
- **FR-O16**: Nest teams (parent/child relationship)

#### 6.3.4 Settings
- **FR-O17**: Set organization visibility (private, public)
- **FR-O18**: Set join policy (invite-only, request, open)
- **FR-O19**: Configure organization-level settings (JSON)

### 6.4 PermissionsService

#### 6.4.1 Role Management
- **FR-P1**: Create role with name and permissions list
- **FR-P2**: Update role permissions
- **FR-P3**: Delete role (if not in use)
- **FR-P4**: List roles by scope (global, org, resource)
- **FR-P5**: System roles: viewer, editor, admin, owner (non-deletable)

#### 6.4.2 Permission Grants
- **FR-P6**: Grant permission to user/team for resource
- **FR-P7**: Revoke permission
- **FR-P8**: Set permission expiration
- **FR-P9**: Get resource permissions
- **FR-P10**: Get user's permissions across resources

#### 6.4.3 Permission Checks
- **FR-P11**: Check single permission (returns boolean)
- **FR-P12**: Batch check multiple permissions
- **FR-P13**: Get effective permissions (after inheritance)
- **FR-P14**: Support permission inheritance (org > team > user)

### 6.5 SharingService

#### 6.5.1 Share Management
- **FR-S1**: Create share link for resource
- **FR-S2**: Create share for specific user/team
- **FR-S3**: Update share settings
- **FR-S4**: Delete/revoke share
- **FR-S5**: List shares for a resource

#### 6.5.2 Share Settings
- **FR-S6**: Set access level (view, comment, edit)
- **FR-S7**: Set expiration date
- **FR-S8**: Set password protection
- **FR-S9**: Require authentication

#### 6.5.3 Share Access
- **FR-S10**: Validate share access (link + password if set)
- **FR-S11**: Record share access for analytics
- **FR-S12**: Get share statistics (views, downloads)

### 6.6 NotesService

#### 6.6.1 Note Management
- **FR-N1**: Create note (standalone or attached)
- **FR-N2**: Update note content
- **FR-N3**: Delete note
- **FR-N4**: Get note by ID
- **FR-N5**: List notes (with filtering)

#### 6.6.2 Note Attachments
- **FR-N6**: Attach note to entity
- **FR-N7**: Detach note from entity
- **FR-N8**: Get all notes for an entity
- **FR-N9**: Support attachment position (page, location)

#### 6.6.3 Note Features
- **FR-N10**: Support content types: markdown, html, plain
- **FR-N11**: Pin/unpin notes
- **FR-N12**: Color-code notes
- **FR-N13**: Version history (track changes)
- **FR-N14**: Restore previous version

### 6.7 UserActivityService

**Key Distinction**: UserActivity is always **private to the user** and attached to external content. Unlike Notes (which can be shared) or Tags (which can be org-level), activities are strictly personal.

#### 6.7.1 Activity Types
- **FR-UA1**: Support multiple activity types: interaction, progress, rating, bookmark, status, note
- **FR-UA2**: Interaction records: timestamp, duration (optional), context
- **FR-UA3**: Progress tracking: percentage, checkpoint, last position
- **FR-UA4**: Ratings: numeric (1-5, 1-10 configurable), optional text
- **FR-UA5**: Personal bookmarks: saved for later, favorites, reading list
- **FR-UA6**: Status workflow: custom statuses ("to-do", "in-progress", "completed", "archived")
- **FR-UA7**: Private notes: personal annotations not visible to others

#### 6.7.2 Activity Management
- **FR-UA8**: Create activity for any entity (entity_type + entity_id)
- **FR-UA9**: Update activity (change status, progress, rating)
- **FR-UA10**: Delete activity
- **FR-UA11**: Get all activities for an entity (by current user)
- **FR-UA12**: List user's activities across all entities (with filtering)
- **FR-UA13**: Aggregate activity statistics (total completed, average ratings, etc.)

#### 6.7.3 Activity Queries
- **FR-UA14**: Filter entities by activity (e.g., "show all workouts I've completed")
- **FR-UA15**: Filter by activity date range
- **FR-UA16**: Filter by rating threshold
- **FR-UA17**: Filter by status
- **FR-UA18**: Sort by last activity, rating, progress

#### 6.7.4 Activity History
- **FR-UA19**: Maintain activity timeline (multiple interactions over time)
- **FR-UA20**: Query activity history for an entity
- **FR-UA21**: Aggregate stats over time (e.g., "workouts per week")

---

## 7. Technical Requirements

### 7.1 Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Application Layer                       │
├─────────────────────────────────────────────────────────────┤
│  TagsService │ CollectionsService │ OrganizationsService    │
│  PermissionsService │ SharingService │ NotesService         │
├─────────────────────────────────────────────────────────────┤
│                     Base Service Layer                       │
│         (Caching, Common CRUD, Storage Provider)            │
├───────────────┬───────────────┬─────────────────────────────┤
│   FS Backend  │  GORM Backend │     GAE Backend             │
│   (JSON/Disk) │  (SQL/ORM)    │     (Datastore)             │
└───────────────┴───────────────┴─────────────────────────────┘
```

### 7.2 Service Interface Pattern

Each service follows established goapplib patterns:

```go
// Service interface
type TagsService interface {
    CreateTag(ctx context.Context, req *CreateTagRequest) (*CreateTagResponse, error)
    GetTag(ctx context.Context, req *GetTagRequest) (*GetTagResponse, error)
    // ... additional methods
}

// Storage provider interface (for multi-backend)
type TagStorageProvider interface {
    LoadTag(ctx context.Context, id string) (*Tag, error)
    SaveTag(ctx context.Context, id string, tag *Tag) error
    DeleteTag(ctx context.Context, id string) error
    ListTags(ctx context.Context, filter *TagFilter) ([]*Tag, error)
}

// Base implementation (shared logic, caching)
type BaseTagsService struct {
    TagStorageProvider
    cache map[string]*Tag
    cacheMu sync.RWMutex
}
```

### 7.3 Proto-Driven Development

All entities defined in protobuf with backend-specific mappings:

```
protos/goapplib/v1/
├── tags.proto              # Tag, EntityTag messages + service
├── collections.proto       # Collection, CollectionItem + service
├── organizations.proto     # Organization, Team, Member + service
├── permissions.proto       # Role, Permission + service
├── sharing.proto          # Share, ShareRecipient + service
└── notes.proto            # Note, NoteVersion + service

protos/goapplib/v1/gorm/
├── tags.proto             # TagGORM mapping
├── collections.proto
├── organizations.proto
├── permissions.proto
├── sharing.proto
└── notes.proto

protos/goapplib/v1/datastore/
├── tags.proto             # TagDatastore mapping
├── collections.proto
├── organizations.proto
├── permissions.proto
├── sharing.proto
└── notes.proto
```

### 7.4 Backend-Specific Requirements

#### 7.4.1 FileSystem (fs)
- JSON file storage with structured directories
- File locking for concurrent access
- Index files for efficient queries (tags by entity, etc.)
- Example structure:
  ```
  {storageDir}/
    tags/
      {tagId}/tag.json
    entity_tags/
      {entityType}/{entityId}/tags.json
    collections/
      {collectionId}/collection.json
    organizations/
      {orgId}/
        org.json
        members/
          {userId}.json
  ```

#### 7.4.2 GORM
- SQL models with proper indexes
- Foreign keys where appropriate
- Many-to-many via join tables
- JSON columns for flexible data (extras, settings)
- Support PostgreSQL, MySQL, SQLite
- Example indexes:
  ```sql
  CREATE INDEX idx_tags_owner ON tags(owner_id, scope);
  CREATE INDEX idx_entity_tags ON entity_tags(entity_type, entity_id);
  CREATE INDEX idx_collections_parent ON collections(parent_id);
  CREATE INDEX idx_org_members ON org_members(org_id, user_id);
  ```

#### 7.4.3 GAE (Datastore)
- Entity kinds with selective indexing
- Composite indexes for common queries
- Namespace support for multi-tenancy
- Ancestor keys for strong consistency where needed
- Example:
  ```
  Kind: Tag
    Properties: name (indexed), color, owner_id (indexed), scope (indexed)

  Kind: Collection
    Ancestor: User or Organization
    Properties: name (indexed), parent_id (indexed), is_smart
  ```

### 7.5 Performance Requirements

| Operation | Target Latency | Throughput |
|-----------|---------------|------------|
| Get single entity | <50ms | 1000 rps |
| List with pagination | <200ms | 500 rps |
| Create/Update | <100ms | 200 rps |
| Permission check | <20ms | 5000 rps |
| Batch operations | <500ms | 100 rps |

### 7.6 Caching Strategy
- In-memory cache in BaseService (configurable)
- Cache invalidation on mutations
- Permission checks should use cache aggressively
- TTL-based expiration for frequently accessed data

---

## 8. Searching, Indexing, and Hydration

This section addresses how these services integrate with search systems, what indexes are needed for efficient queries, and how to hydrate entities with their related data during list operations.

### 8.1 The Hydration Problem

When listing entities (e.g., "show all articles"), applications typically need to display associated data:
- Tags attached to each article
- Collections the article belongs to
- User's personal activity (rating, progress, bookmark status)
- Permission level for each item
- Share status

**Naive Approach (N+1 Problem):**
```go
// BAD: N+1 queries
articles := listArticles(pageSize: 20)
for _, article := range articles {
    article.Tags = getTags(article.ID)           // +20 queries
    article.Collections = getCollections(article.ID)  // +20 queries
    article.MyActivity = getActivity(userID, article.ID)  // +20 queries
}
// Total: 1 + 60 = 61 queries for 20 items
```

**Hydration Pattern (Batch Loading):**
```go
// GOOD: Batch queries
articles := listArticles(pageSize: 20)
articleIDs := extractIDs(articles)

tagMap := batchGetTags(articleIDs)           // 1 query
collectionMap := batchGetCollections(articleIDs)  // 1 query
activityMap := batchGetActivities(userID, articleIDs)  // 1 query

for _, article := range articles {
    article.Tags = tagMap[article.ID]
    article.Collections = collectionMap[article.ID]
    article.MyActivity = activityMap[article.ID]
}
// Total: 4 queries regardless of page size
```

### 8.2 Hydrator Interface

Each service should provide a batch hydration method:

```go
// TagsService
type TagHydrator interface {
    // Returns map[entityID][]Tag
    HydrateTags(ctx context.Context, entityType string, entityIDs []string) (map[string][]*Tag, error)
}

// CollectionsService
type CollectionHydrator interface {
    // Returns map[entityID][]Collection
    HydrateCollections(ctx context.Context, entityType string, entityIDs []string) (map[string][]*Collection, error)
}

// UserActivityService
type ActivityHydrator interface {
    // Returns map[entityID]*ActivitySummary
    HydrateActivities(ctx context.Context, userID string, entityType string, entityIDs []string) (map[string]*ActivitySummary, error)
}

// PermissionsService
type PermissionHydrator interface {
    // Returns map[entityID]EffectivePermission
    HydratePermissions(ctx context.Context, userID string, resourceType string, resourceIDs []string) (map[string]*EffectivePermission, error)
}
```

### 8.3 Composite Hydrator

For convenience, provide a composite hydrator that loads all related data in parallel:

```go
type EntityHydrationRequest struct {
    EntityType string
    EntityIDs  []string
    UserID     string  // for user-specific data

    // What to hydrate (all optional)
    IncludeTags        bool
    IncludeCollections bool
    IncludeActivity    bool
    IncludePermissions bool
    IncludeShares      bool
}

type EntityHydrationResult struct {
    Tags        map[string][]*Tag                  // entityID -> tags
    Collections map[string][]*Collection           // entityID -> collections
    Activities  map[string]*ActivitySummary        // entityID -> activity
    Permissions map[string]*EffectivePermission    // entityID -> permission
    Shares      map[string][]*Share                // entityID -> shares
}

type CompositeHydrator interface {
    Hydrate(ctx context.Context, req *EntityHydrationRequest) (*EntityHydrationResult, error)
}
```

**Implementation uses parallel goroutines:**
```go
func (h *CompositeHydrator) Hydrate(ctx context.Context, req *EntityHydrationRequest) (*EntityHydrationResult, error) {
    result := &EntityHydrationResult{}
    var wg sync.WaitGroup
    var mu sync.Mutex
    var firstErr error

    if req.IncludeTags {
        wg.Add(1)
        go func() {
            defer wg.Done()
            tags, err := h.tagHydrator.HydrateTags(ctx, req.EntityType, req.EntityIDs)
            mu.Lock()
            if err != nil && firstErr == nil {
                firstErr = err
            }
            result.Tags = tags
            mu.Unlock()
        }()
    }

    // Similar for other hydrators...
    wg.Wait()
    return result, firstErr
}
```

### 8.4 Indexing Strategies

#### 8.4.1 Tags Indexing

**GORM (SQL):**
```sql
-- entity_tags table
CREATE TABLE entity_tags (
    entity_type VARCHAR(50) NOT NULL,
    entity_id VARCHAR(100) NOT NULL,
    tag_id VARCHAR(100) NOT NULL,
    tagged_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    tagged_by VARCHAR(100),
    PRIMARY KEY (entity_type, entity_id, tag_id)
);

-- Index for "get all tags for entities" (hydration)
CREATE INDEX idx_entity_tags_lookup ON entity_tags(entity_type, entity_id);

-- Index for "get all entities with tag" (filtering)
CREATE INDEX idx_entity_tags_by_tag ON entity_tags(tag_id, entity_type);

-- Tags table
CREATE INDEX idx_tags_owner ON tags(owner_id, owner_type, scope);
CREATE INDEX idx_tags_name ON tags(name);  -- for autocomplete
```

**Datastore (GAE):**
```yaml
# index.yaml
indexes:
- kind: EntityTag
  properties:
  - name: entity_type
  - name: entity_id

- kind: EntityTag
  properties:
  - name: tag_id
  - name: entity_type

- kind: Tag
  properties:
  - name: owner_id
  - name: scope
  - name: name
```

#### 8.4.2 Collections Indexing

**GORM (SQL):**
```sql
-- collections table
CREATE INDEX idx_collections_owner ON collections(owner_id, owner_type);
CREATE INDEX idx_collections_parent ON collections(parent_id);
CREATE INDEX idx_collections_path ON collections USING GIN(path);  -- PostgreSQL array

-- collection_items table
CREATE TABLE collection_items (
    collection_id VARCHAR(100) NOT NULL,
    entity_type VARCHAR(50) NOT NULL,
    entity_id VARCHAR(100) NOT NULL,
    display_order INT DEFAULT 0,
    added_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (collection_id, entity_type, entity_id)
);

-- Index for "get all items in collection"
CREATE INDEX idx_collection_items_coll ON collection_items(collection_id, display_order);

-- Index for "get all collections containing entity" (hydration)
CREATE INDEX idx_collection_items_entity ON collection_items(entity_type, entity_id);
```

#### 8.4.3 User Activity Indexing

**GORM (SQL):**
```sql
-- user_activities table - partitioned or indexed for user isolation
CREATE INDEX idx_activities_user_entity ON user_activities(user_id, entity_type, entity_id);

-- For "get user's activities across all entities"
CREATE INDEX idx_activities_user_type ON user_activities(user_id, entity_type, updated_at DESC);

-- For "filter entities by activity status"
CREATE INDEX idx_activities_user_status ON user_activities(user_id, entity_type, status);

-- For "filter by bookmark list"
CREATE INDEX idx_activities_bookmarks ON user_activities(user_id, bookmark_list)
    WHERE type = 'bookmark';
```

**Datastore (GAE):**
```yaml
indexes:
- kind: UserActivity
  ancestor: yes  # Ancestor is User for strong consistency
  properties:
  - name: entity_type
  - name: entity_id

- kind: UserActivity
  ancestor: yes
  properties:
  - name: entity_type
  - name: updated_at
    direction: desc
```

#### 8.4.4 Permissions Indexing

**GORM (SQL):**
```sql
-- resource_permissions table
CREATE INDEX idx_perms_resource ON resource_permissions(resource_type, resource_id);
CREATE INDEX idx_perms_principal ON resource_permissions(principal_type, principal_id);

-- For batch permission checks (hydration)
CREATE INDEX idx_perms_batch ON resource_permissions(principal_id, principal_type, resource_type);
```

### 8.5 Search Integration

#### 8.5.1 Search Service Interface

Services should provide data for external search indexing:

```go
type SearchIndexable interface {
    // Returns data formatted for search indexing
    GetSearchDocument(ctx context.Context, entityType, entityID string) (*SearchDocument, error)

    // Returns updates since timestamp for incremental indexing
    GetSearchUpdates(ctx context.Context, since time.Time, limit int) ([]*SearchUpdate, error)
}

type SearchDocument struct {
    ID         string
    Type       string

    // Core fields
    Title      string
    Content    string

    // Facets (for filtering)
    Tags       []string           // tag names
    Collections []string          // collection paths
    OwnerID    string
    OrgID      string

    // Timestamps
    CreatedAt  time.Time
    UpdatedAt  time.Time
}

type SearchUpdate struct {
    EntityType string
    EntityID   string
    Operation  string  // "upsert" or "delete"
    Document   *SearchDocument
}
```

#### 8.5.2 Search Query Integration

When searching, the search service should support filtering by our services' data:

```go
type SearchQuery struct {
    Query      string

    // Filters populated from our services
    Tags       []string           // must have these tags
    ExcludeTags []string          // must not have these tags
    Collections []string          // must be in these collections

    // Activity-based filters (user-specific)
    UserID     string             // for activity filters
    HasActivity bool              // user has interacted
    Statuses   []string           // activity status filter
    MinRating  float32            // minimum user rating
    IsBookmarked bool             // in user's bookmarks

    // Pagination
    Offset     int
    Limit      int
}
```

#### 8.5.3 Recommended Search Backends

| Backend | Use Case | Integration |
|---------|----------|-------------|
| **Elasticsearch** | Full-text search, faceted navigation | REST API, bulk indexing |
| **Algolia** | Fast, hosted search | SDK, instant search |
| **Typesense** | Open-source alternative | Docker, REST API |
| **PostgreSQL FTS** | Simple full-text | Built-in, no extra infra |
| **Cloud Datastore** | GAE apps | Native queries (limited) |

### 8.6 Filtering by Related Data

Common query patterns that need efficient indexes:

#### 8.6.1 Filter Entities by Tags

"Show all articles tagged 'golang' AND 'tutorial'"

**SQL Approach:**
```sql
SELECT DISTINCT e.* FROM articles e
JOIN entity_tags et1 ON et1.entity_type = 'article' AND et1.entity_id = e.id
JOIN entity_tags et2 ON et2.entity_type = 'article' AND et2.entity_id = e.id
JOIN tags t1 ON t1.id = et1.tag_id AND t1.name = 'golang'
JOIN tags t2 ON t2.id = et2.tag_id AND t2.name = 'tutorial'
ORDER BY e.created_at DESC
LIMIT 20;
```

**Service Method:**
```go
func (s *ArticleService) ListByTags(ctx context.Context, tagNames []string, page Pagination) ([]*Article, error) {
    // Get tag IDs from names
    tagIDs, err := s.tagsService.GetTagIDsByNames(ctx, tagNames)

    // Get entity IDs that have ALL these tags
    entityIDs, err := s.tagsService.GetEntitiesWithAllTags(ctx, "article", tagIDs)

    // Load and paginate articles
    return s.loadArticlesByIDs(ctx, entityIDs, page)
}
```

#### 8.6.2 Filter Entities by User Activity

"Show all workouts I've completed"

**SQL Approach:**
```sql
SELECT w.* FROM workouts w
JOIN user_activities ua ON ua.entity_type = 'workout'
    AND ua.entity_id = w.id
    AND ua.user_id = $1
WHERE ua.status = 'completed'
ORDER BY ua.updated_at DESC
LIMIT 20;
```

**Service Method:**
```go
func (s *WorkoutService) ListByActivity(ctx context.Context, userID string, filter ActivityFilter, page Pagination) ([]*Workout, error) {
    // Get workout IDs matching activity filter
    workoutIDs, err := s.activityService.GetEntitiesByActivity(ctx, userID, "workout", filter)

    // Load workouts
    return s.loadWorkoutsByIDs(ctx, workoutIDs, page)
}
```

#### 8.6.3 Filter Entities in Collection

"Show all items in 'Favorites' collection"

```go
func (s *EntityService) ListByCollection(ctx context.Context, collectionID string, page Pagination) ([]*Entity, error) {
    // Get entity IDs in collection (ordered)
    items, err := s.collectionsService.GetCollectionItems(ctx, collectionID, page)

    // Group by entity type and batch load
    return s.batchLoadEntities(ctx, items)
}
```

### 8.7 Denormalization Strategies

For read-heavy workloads, consider denormalizing frequently-accessed data:

#### 8.7.1 Tag Count on Entity

```protobuf
message Article {
  // ... core fields
  int32 tag_count = 20;        // denormalized
  repeated string tag_names = 21;  // denormalized (most common tags)
}
```

**Update on tag change:**
```go
func (s *TagsService) TagEntity(ctx context.Context, req *TagEntityRequest) error {
    // ... add tag

    // Update denormalized count (async or sync)
    s.entityService.IncrementTagCount(ctx, req.EntityType, req.EntityID)

    // Optionally update tag_names if in top N
    if s.shouldIncludeInNames(tag) {
        s.entityService.AddTagName(ctx, req.EntityType, req.EntityID, tag.Name)
    }
}
```

#### 8.7.2 Activity Summary on Entity (Per User)

For user-specific views, cache the activity summary:

```go
// Materialized view or cache
type UserEntitySummary struct {
    UserID      string
    EntityType  string
    EntityID    string

    // Denormalized activity summary
    LastViewed  time.Time
    ViewCount   int
    Progress    float32
    Rating      float32
    Status      string
    IsBookmarked bool
}
```

### 8.8 Batch Operations for Performance

#### 8.8.1 Batch Tag Assignment

```protobuf
message BulkTagEntitiesRequest {
  repeated string entity_ids = 1;
  string entity_type = 2;
  repeated string tag_ids = 3;
  bool replace = 4;  // replace existing tags vs add to existing
}

message BulkTagEntitiesResponse {
  int32 entities_updated = 1;
  int32 tags_added = 2;
  repeated string failed_entity_ids = 3;
}
```

#### 8.8.2 Batch Collection Operations

```protobuf
message BulkAddToCollectionRequest {
  string collection_id = 1;
  repeated EntityReference entities = 2;
}

message EntityReference {
  string entity_type = 1;
  string entity_id = 2;
}
```

### 8.9 Performance Benchmarks

| Operation | Target | Index Required |
|-----------|--------|----------------|
| Hydrate tags for 100 entities | <50ms | idx_entity_tags_lookup |
| Hydrate activities for 100 entities | <50ms | idx_activities_user_entity |
| Filter by single tag | <100ms | idx_entity_tags_by_tag |
| Filter by 3 tags (AND) | <200ms | idx_entity_tags_by_tag + app logic |
| Get collection tree (3 levels) | <100ms | idx_collections_parent |
| Permission check (batch 100) | <50ms | idx_perms_batch |

---

## 9. Data Models

### 8.1 Tag

```protobuf
message Tag {
  string id = 1;
  string name = 2;
  string color = 3;           // hex: "#FF5733" or name: "blue"
  int32 display_order = 4;
  bool is_automatic = 5;      // auto-generated vs manual
  int64 usage_count = 6;      // cached count
  string owner_id = 7;        // user or org ID
  string owner_type = 8;      // "user" or "org"
  string scope = 9;           // "private", "org", "public"
  google.protobuf.Timestamp created_at = 10;
  google.protobuf.Timestamp updated_at = 11;
}
```

### 8.2 Collection

```protobuf
message Collection {
  string id = 1;
  string name = 2;
  string description = 3;
  string parent_id = 4;       // empty for root
  string owner_id = 5;
  string owner_type = 6;      // "user" or "org"
  int32 display_order = 7;
  string icon = 8;            // emoji or icon name
  string color = 9;
  bool is_smart = 10;         // saved search
  string smart_query = 11;    // JSON query definition
  int64 item_count = 12;      // cached count
  repeated string path = 13;  // ancestor IDs
  google.protobuf.Timestamp created_at = 14;
  google.protobuf.Timestamp updated_at = 15;
}
```

### 8.3 Organization

```protobuf
message Organization {
  string id = 1;
  string name = 2;
  string description = 3;
  string slug = 4;            // URL-friendly: "my-org"
  string owner_id = 5;        // primary owner user ID
  OrgVisibility visibility = 6;
  OrgJoinPolicy join_policy = 7;
  string image_url = 8;
  int64 member_count = 9;     // cached
  google.protobuf.Struct settings = 10;
  google.protobuf.Timestamp created_at = 11;
  google.protobuf.Timestamp updated_at = 12;
}

enum OrgVisibility {
  ORG_VISIBILITY_UNSPECIFIED = 0;
  ORG_VISIBILITY_PRIVATE = 1;
  ORG_VISIBILITY_PUBLIC = 2;
}

enum OrgJoinPolicy {
  ORG_JOIN_POLICY_UNSPECIFIED = 0;
  ORG_JOIN_POLICY_INVITE = 1;
  ORG_JOIN_POLICY_REQUEST = 2;
  ORG_JOIN_POLICY_OPEN = 3;
}
```

### 8.4 Permission

```protobuf
message Role {
  string id = 1;
  string name = 2;
  string description = 3;
  repeated string permissions = 4;  // "read", "write", "delete", "admin"
  bool is_system = 5;               // built-in vs custom
  string scope = 6;                 // "global", "org:{id}", "resource"
  int32 priority = 7;               // for inheritance resolution
}

message ResourcePermission {
  string id = 1;
  string resource_type = 2;
  string resource_id = 3;
  string principal_type = 4;        // "user", "team", "org"
  string principal_id = 5;
  string role_id = 6;
  google.protobuf.Timestamp granted_at = 7;
  string granted_by = 8;
  google.protobuf.Timestamp expires_at = 9;  // optional
}
```

### 8.5 Share

```protobuf
message Share {
  string id = 1;
  string resource_type = 2;
  string resource_id = 3;
  ShareType type = 4;
  string access_level = 5;          // "view", "comment", "edit"
  string shared_by = 6;
  bool requires_auth = 7;
  string password_hash = 8;         // bcrypt if set
  int64 view_count = 9;
  int64 download_count = 10;
  google.protobuf.Timestamp created_at = 11;
  google.protobuf.Timestamp expires_at = 12;
}

enum ShareType {
  SHARE_TYPE_UNSPECIFIED = 0;
  SHARE_TYPE_LINK = 1;
  SHARE_TYPE_USER = 2;
  SHARE_TYPE_TEAM = 3;
  SHARE_TYPE_ORG = 4;
  SHARE_TYPE_PUBLIC = 5;
}
```

### 8.6 Note

```protobuf
message Note {
  string id = 1;
  string title = 2;
  string content = 3;
  string content_type = 4;          // "markdown", "html", "plain"
  string owner_id = 5;
  string attached_to_type = 6;      // empty for standalone
  string attached_to_id = 7;
  string attachment_position = 8;   // page number, location
  bool is_pinned = 9;
  string color = 10;
  repeated string tags = 11;
  google.protobuf.Timestamp created_at = 12;
  google.protobuf.Timestamp updated_at = 13;
}
```

### 8.7 UserActivity

```protobuf
// UserActivity captures a user's private interaction with any entity
message UserActivity {
  string id = 1;
  string user_id = 2;               // the user who created this activity
  string entity_type = 3;           // "workout", "article", "recipe", etc.
  string entity_id = 4;
  ActivityType type = 5;

  // Activity-specific data (use one based on type)
  InteractionData interaction = 6;
  ProgressData progress = 7;
  RatingData rating = 8;
  BookmarkData bookmark = 9;
  StatusData status = 10;
  string private_note = 11;         // personal annotation

  google.protobuf.Timestamp created_at = 12;
  google.protobuf.Timestamp updated_at = 13;
  google.protobuf.Struct extras = 14;  // app-specific custom fields
}

enum ActivityType {
  ACTIVITY_TYPE_UNSPECIFIED = 0;
  ACTIVITY_TYPE_INTERACTION = 1;    // viewed, clicked, started
  ACTIVITY_TYPE_PROGRESS = 2;       // partial completion tracking
  ACTIVITY_TYPE_RATING = 3;         // personal score/rating
  ACTIVITY_TYPE_BOOKMARK = 4;       // saved for later
  ACTIVITY_TYPE_STATUS = 5;         // workflow status
  ACTIVITY_TYPE_NOTE = 6;           // private annotation only
}

message InteractionData {
  string action = 1;                // "viewed", "started", "completed", "attempted"
  int32 duration_seconds = 2;       // time spent (optional)
  string context = 3;               // "from homepage", "via search", etc.
  google.protobuf.Timestamp occurred_at = 4;
}

message ProgressData {
  float percentage = 1;             // 0.0 to 100.0
  string checkpoint = 2;            // "chapter 3", "exercise 5", etc.
  string last_position = 3;         // app-specific position marker
  google.protobuf.Timestamp last_activity = 4;
}

message RatingData {
  float score = 1;                  // numeric rating (scale configurable)
  float max_score = 2;              // max possible (5, 10, 100)
  string difficulty = 3;            // "easy", "medium", "hard" (for workouts, etc.)
  string review = 4;                // brief personal review text
}

message BookmarkData {
  string list_name = 1;             // "favorites", "read-later", "to-try", custom
  int32 priority = 2;               // ordering within list
  google.protobuf.Timestamp reminder_at = 3;  // optional reminder
}

message StatusData {
  string status = 1;                // "todo", "in_progress", "completed", "skipped", "archived"
  string previous_status = 2;       // for history tracking
  google.protobuf.Timestamp status_changed_at = 3;
}

// ActivitySummary provides aggregated view of user's activities on an entity
message ActivitySummary {
  string entity_type = 1;
  string entity_id = 2;
  string user_id = 3;

  int32 interaction_count = 4;      // how many times interacted
  google.protobuf.Timestamp first_interaction = 5;
  google.protobuf.Timestamp last_interaction = 6;
  int32 total_duration_seconds = 7;

  float current_progress = 8;       // latest progress percentage
  float latest_rating = 9;
  string current_status = 10;
  bool is_bookmarked = 11;
  string bookmark_list = 12;

  int32 note_count = 13;            // number of private notes
}
```

---

## 9. API Design

### 9.1 REST-like RPC Pattern

Following goapplib conventions, services use RPC-style methods:

```go
// Tags
CreateTag(CreateTagRequest) → CreateTagResponse
GetTag(GetTagRequest) → GetTagResponse
ListTags(ListTagsRequest) → ListTagsResponse
UpdateTag(UpdateTagRequest) → UpdateTagResponse
DeleteTag(DeleteTagRequest) → DeleteTagResponse
TagEntity(TagEntityRequest) → TagEntityResponse
UntagEntity(UntagEntityRequest) → UntagEntityResponse

// Collections
CreateCollection(CreateCollectionRequest) → CreateCollectionResponse
GetCollectionTree(GetCollectionTreeRequest) → GetCollectionTreeResponse
AddToCollection(AddToCollectionRequest) → AddToCollectionResponse
// ...

// Organizations
CreateOrganization(CreateOrganizationRequest) → CreateOrganizationResponse
InviteMember(InviteMemberRequest) → InviteMemberResponse
// ...

// Permissions
CheckPermission(CheckPermissionRequest) → CheckPermissionResponse
GrantPermission(GrantPermissionRequest) → GrantPermissionResponse
// ...

// Sharing
CreateShare(CreateShareRequest) → CreateShareResponse
AccessShare(AccessShareRequest) → AccessShareResponse
// ...

// Notes
CreateNote(CreateNoteRequest) → CreateNoteResponse
GetEntityNotes(GetEntityNotesRequest) → GetEntityNotesResponse
// ...

// User Activities
RecordActivity(RecordActivityRequest) → RecordActivityResponse
GetActivity(GetActivityRequest) → GetActivityResponse
UpdateActivity(UpdateActivityRequest) → UpdateActivityResponse
DeleteActivity(DeleteActivityRequest) → DeleteActivityResponse
ListMyActivities(ListMyActivitiesRequest) → ListMyActivitiesResponse
GetActivitySummary(GetActivitySummaryRequest) → GetActivitySummaryResponse
GetEntitiesByActivity(GetEntitiesByActivityRequest) → GetEntitiesByActivityResponse
// ...
```

### 9.2 Common Request/Response Patterns

```protobuf
// Pagination
message ListTagsRequest {
  string owner_id = 1;
  string scope = 2;
  int32 page_size = 3;
  string page_token = 4;
  string order_by = 5;  // "name", "usage_count", "created_at"
}

message ListTagsResponse {
  repeated Tag tags = 1;
  string next_page_token = 2;
  int32 total_count = 3;
}

// Batch operations
message BulkTagEntitiesRequest {
  repeated string entity_ids = 1;
  string entity_type = 2;
  repeated string tag_ids = 3;
}
```

---

## 10. Security Considerations

### 10.1 Authorization
- All operations must check permissions via PermissionsService
- Resource-level access control enforced at service layer
- Org/team membership validated before access

### 10.2 Data Isolation
- Multi-tenant data isolation via owner_id or namespace
- Cross-tenant access prevented by default
- Share links are the only cross-boundary mechanism

### 10.3 Input Validation
- Tag names: max 100 chars, no special chars except `-_`
- Collection depth: configurable max (default 10)
- Org slugs: lowercase alphanumeric + hyphen
- Passwords: bcrypt hashed, never stored plain

### 10.4 Audit Logging
- Track permission grants/revokes
- Track share access
- Track membership changes

---

## 11. Implementation Phases

### Phase 1: Foundation (Priority: P0)
**Duration:** 2-3 sprints

| Service | Scope |
|---------|-------|
| TagsService | Full implementation all backends |
| CollectionsService | Basic collections (no smart) |
| UserActivityService | Core activity tracking |

**Deliverables:**
- Proto definitions for Tags, Collections, UserActivity
- Generated DAL code
- FS, GORM, GAE implementations
- Hydrator interfaces
- Unit tests (90%+ coverage)
- Integration tests

### Phase 2: Collaboration (Priority: P0)
**Duration:** 2-3 sprints

| Service | Scope |
|---------|-------|
| OrganizationsService | Orgs + membership |
| PermissionsService | Core RBAC |

**Deliverables:**
- Org/team management
- Role-based permissions
- Permission checking
- Permission hydration
- Tests

### Phase 3: Sharing & Integration (Priority: P1)
**Duration:** 1-2 sprints

| Service | Scope |
|---------|-------|
| SharingService | Full implementation |
| OrganizationsService | Teams (nested) |
| CompositeHydrator | Unified hydration service |

**Deliverables:**
- Share links
- Share analytics
- Team nesting
- Composite hydrator for batch loading

### Phase 4: Advanced Features (Priority: P1-P2)
**Duration:** 2 sprints

| Service | Scope |
|---------|-------|
| NotesService | Full implementation |
| CollectionsService | Smart collections |
| TagsService | Tag suggestions |
| UserActivityService | Advanced filtering, aggregations |

**Deliverables:**
- Notes with versioning
- Smart/saved collections
- Tag autocomplete
- Activity history aggregations
- Search integration layer

---

## 12. Success Metrics

### 12.1 Development Metrics
- [ ] All services implemented with 3 backends
- [ ] Test coverage ≥90%
- [ ] Zero critical bugs at launch
- [ ] Documentation complete

### 12.2 Performance Metrics
- [ ] P99 latency <100ms for single operations
- [ ] P99 latency <500ms for list operations
- [ ] Permission checks <20ms

### 12.3 Adoption Metrics (Post-Launch)
- [ ] ≥3 production apps using services within 3 months
- [ ] Developer satisfaction score ≥4/5
- [ ] <5 critical issues reported first month

---

## 13. Open Questions

| # | Question | Status | Decision |
|---|----------|--------|----------|
| 1 | Should tags support hierarchical naming convention (topic/subtopic)? | Open | |
| 2 | Max collection nesting depth default? | Proposed: 10 | |
| 3 | Should permissions support deny rules or just allow? | Proposed: Allow only | |
| 4 | Share link format (UUID vs short code)? | Proposed: Short code | |
| 5 | Note content size limit? | Proposed: 1MB | |
| 6 | Should we support real-time collaboration hooks? | Open | |
| 7 | Activity history retention period? | Proposed: 1 year, configurable | |
| 8 | Should activity be anonymized for analytics sharing? | Open | |
| 9 | Default search backend recommendation? | Proposed: PostgreSQL FTS (simple), Elasticsearch (advanced) | |
| 10 | Should hydrators use Redis for caching cross-request? | Open | |
| 11 | Max entities per hydration batch? | Proposed: 500 | |
| 12 | Should UserActivity support custom activity types per app? | Proposed: Yes, via extras field | |
| 13 | Activity data export format? | Proposed: JSON, CSV | |

---

## 14. Appendix

### A. Glossary

| Term | Definition |
|------|------------|
| **Tag** | A keyword label that can be attached to any entity |
| **Collection** | A container that can hold entities and other collections |
| **Smart Collection** | A collection whose membership is determined by a saved query |
| **Organization** | A group of users who collaborate on shared resources |
| **Team** | A subset of organization members |
| **Role** | A named set of permissions |
| **Permission** | A specific capability (read, write, delete, admin) |
| **Share** | A mechanism to grant access to non-members |
| **Principal** | An entity that can be granted permissions (user, team, org) |
| **UserActivity** | A private, user-specific interaction or annotation on content |
| **Activity Summary** | Aggregated view of a user's activities on a single entity |
| **Hydration** | The process of loading related data (tags, activities, etc.) for entities |
| **Hydrator** | A service that batch-loads related data for multiple entities efficiently |
| **N+1 Problem** | Performance anti-pattern of loading related data one entity at a time |
| **Denormalization** | Duplicating data to optimize read performance |
| **Facet** | A filterable attribute used in search (e.g., tags, status) |

### B. Related Documents
- goapplib USAGE_GUIDE.md
- goapplib INTEGRATION_GUIDE.md
- ZOTERO_FEATURES_ANALYSIS.md

### C. Revision History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-01-22 | Engineering | Initial draft |
