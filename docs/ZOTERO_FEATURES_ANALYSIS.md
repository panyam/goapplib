# Zotero Features Analysis & Common Services Proposal

## Executive Summary

This document analyzes Zotero's tagging, sharing, and organization capabilities, compares them to other popular services (Notion, Evernote, Google Drive, Dropbox, Pinterest, Pocket, Raindrop.io, GitHub), and proposes common services to add to goapplib with fs, gorm, and gae implementations.

---

## Part 1: Zotero Feature Analysis

### 1.1 Tagging System

#### How Tags Work
- Tags are keywords attached to items for flexible categorization
- Items can have **unlimited tags**
- Tags are portable (transfer when copying items between libraries)
- Unlike collections (folders), tags are non-hierarchical labels

#### Tag Types

| Type | Icon | Source | Control |
|------|------|--------|---------|
| **Manual Tags** | Blue | User-created | Full control |
| **Automatic Tags** | Red | Downloaded from databases/catalogs | Can be disabled/bulk-deleted |

#### Tag Colors & Shortcuts
- Up to **9 colored tags** assignable per library
- Colored tags display as small squares before item titles
- **Keyboard shortcuts**: Number keys 1-9 quickly add/remove colored tags
- Colored tags appear first in tag selector (before alphabetical list)
- Use cases: workflow management ("to-read", "important", "review-needed")

#### Tag Hierarchies
- **Native**: NOT supported (requested for 10+ years)
- **Plugin workaround**: Zotero Style plugin enables `#topic/subtopic/subsub` syntax

#### Tag Search & Filtering
- Tag Selector panel shows all tags
- Click to filter items by tag
- Multiple tags can be selected (AND filter)
- Integrates with Advanced Search

---

### 1.2 Sharing Capabilities

#### Group Libraries
Primary sharing mechanism - appear in left pane alongside "My Library"

#### Group Types

| Type | Library Visibility | Membership | File Sharing |
|------|-------------------|------------|--------------|
| **Private** | Hidden | Invite only | Yes |
| **Public, Closed** | Public | Invite only | Configurable |
| **Public, Open** | Public | Anyone can join | No |

#### Permission Levels

**Group Roles:**
- **Owner**: Full control (settings, members, library)
- **Admin**: Manage members and library settings
- **Member**: Add/edit items (if permissions allow)

**Library Settings:**
- Who can add items (admins only vs all members)
- Who can edit items
- File sharing enabled/disabled

#### Key Characteristics
- Group libraries are **separate** from personal library
- Items dragged to groups are **copies** (not linked)
- Tags transfer; collection placements do NOT
- Storage counts against group creator's quota (300 MB free)

---

### 1.3 Organization Features

#### Collections & Sub-Collections
- Hierarchical folder-like structure
- Items can belong to **multiple collections** (like playlists)
- Unlimited nesting depth
- Collection placement NOT transferred between libraries

#### Saved Searches (Smart Collections)
- Dynamic collections auto-updating based on criteria
- Created via Advanced Search > "Save Search"
- Supports complex Boolean queries
- Cannot be organized into groups (limitation)

#### Related Items
- Manual bidirectional linking between any two items
- Use cases: book↔review, author's papers, software↔docs
- **Limitation**: Cannot relate items across different libraries

#### Notes & Annotations

| Type | Attachment | Features |
|------|-----------|----------|
| **Standalone Notes** | Independent | Organized in collections, exportable |
| **Item Notes** | Attached to item | Visible when expanding item |
| **PDF Annotations** | Within PDF reader | Highlight, underline, sticky notes, ink, text |

**PDF Annotation Features (Zotero 7):**
- Built-in reader with annotation tools
- EPUB and webpage snapshot support
- Annotations stored in Zotero metadata (original PDF unchanged)
- Can export annotated PDFs
- Convert annotations to notes for word processor

---

## Part 2: Comparative Analysis

### 2.1 Feature Matrix

| Feature | Zotero | Notion | Evernote | Google Drive | Dropbox | Pinterest | Pocket | Raindrop.io | GitHub |
|---------|--------|--------|----------|--------------|---------|-----------|--------|-------------|--------|
| **Tags** | Yes (colored, auto) | Multi-select | Yes (nested) | Labels | Tags | No | Yes | Yes | Labels |
| **Hierarchical Tags** | Plugin | Via relations | Yes | No | No | No | No | No | No |
| **Collections** | Multi-parent | Pages/DB | Notebooks | Folders | Folders | Boards | Lists | Nested | Repos |
| **Items in Multiple** | Yes | Yes | No | No | No | No | No | Yes | No |
| **Saved Searches** | Yes | DB filters | Yes | No | No | No | No | Smart lists | Yes |
| **Group Types** | 3 | Workspaces | Shared NBs | Shared Drives | Team folders | Group boards | N/A | Shared colls | Orgs |
| **Permission Levels** | 3 | 4+ | 2 | 5 | 3 | 2 | N/A | 2 | 5+ custom |
| **Real-time Collab** | Sync | Yes | Limited | Yes | Yes | Yes | No | Sync | Yes |
| **Annotations** | PDF/EPUB | Comments | Yes | Comments | Comments | Comments | Highlights | Highlights | PR reviews |
| **Free Storage** | 300 MB | Limited | Limited | 15 GB | 2 GB | Unlimited | Yes | 100/month | Unlimited |

### 2.2 Key Differentiators by Service

#### Notion
- Real-time collaborative editing
- Flexible database-driven organization
- @mentions and assignments
- Teamspaces for department organization
- More granular page-level permissions

#### Evernote
- Nested tags natively supported
- Limited to 250 notebooks
- Items can only be in ONE notebook
- 95% of users use notebooks over tags
- Tags cannot be shared separately

#### Google Drive
- 5-tier permission system (Manager, Content Manager, Contributor, Commenter, Viewer)
- Shared drives owned by organization
- Target audiences for smart sharing
- 15 GB free storage

#### GitHub (Most Sophisticated)
- 5 base repository roles + custom roles
- Organization > Teams > Repositories hierarchy
- Fine-grained permissions at multiple levels
- Outside collaborators concept
- Permission inheritance (highest wins)

#### Raindrop.io (Most Similar for Bookmarking)
- Nested collections
- Tags with any characters/spaces
- AI-suggested organization
- Public/private sharing

---

## Part 3: Gap Analysis

### 3.1 What Zotero Has That Others Lack
1. **Citation-aware tagging** - Tags tied to bibliographic metadata
2. **Multi-parent collections** - Items in multiple collections simultaneously
3. **Automatic metadata extraction** - Tags from academic sources
4. **PDF annotation integration** - Annotations linked to citations

### 3.2 What Others Have That Zotero Lacks
1. **Real-time collaboration** (Notion, Google Drive)
2. **Hierarchical tags natively** (Evernote)
3. **Custom permission roles** (GitHub)
4. **Large free storage** (Google Drive - 15GB)
5. **Team/department structures** (Notion Teamspaces, GitHub Orgs)
6. **Link sharing with expiration** (Dropbox)
7. **AI-suggested organization** (Raindrop.io)

### 3.3 Common Patterns Across All Services

| Pattern | Services Using It | Complexity |
|---------|-------------------|------------|
| **Tags/Labels** | All except Pinterest | Low-Medium |
| **Hierarchical Containers** | All | Medium |
| **Role-based Permissions** | All with sharing | Medium-High |
| **Group/Team Structures** | All except Pocket | Medium |
| **Saved/Smart Searches** | Zotero, Evernote, Raindrop, GitHub | Medium |
| **Item Relations/Links** | Zotero, Notion | Low |
| **Annotations/Comments** | All | Low-Medium |

---

## Part 4: Proposed Services for goapplib

Based on the analysis, I recommend adding these common services that appear across multiple platforms:

### 4.1 TagsService

**Purpose**: Manage tags/labels for any entity type

**Features:**
- Create, update, delete tags
- Assign/remove tags from entities
- Tag colors and display order
- Tag counts and statistics
- Bulk tag operations
- Automatic vs manual tag distinction
- Tag search and filtering
- Tag suggestions/autocomplete

**Proto Definition:**
```protobuf
message Tag {
  string id = 1;
  string name = 2;
  string color = 3;  // hex color or predefined
  int32 display_order = 4;
  bool is_automatic = 5;
  int64 usage_count = 6;
  google.protobuf.Timestamp created_at = 7;
  google.protobuf.Timestamp updated_at = 8;
  string owner_id = 9;  // user or org that owns the tag
  string scope = 10;    // "user", "org", "global"
}

message EntityTag {
  string entity_type = 1;  // "article", "bookmark", "note"
  string entity_id = 2;
  string tag_id = 3;
  google.protobuf.Timestamp tagged_at = 4;
  string tagged_by = 5;  // user who added the tag
}

service TagsService {
  rpc CreateTag(CreateTagRequest) returns (CreateTagResponse);
  rpc GetTag(GetTagRequest) returns (GetTagResponse);
  rpc ListTags(ListTagsRequest) returns (ListTagsResponse);
  rpc UpdateTag(UpdateTagRequest) returns (UpdateTagResponse);
  rpc DeleteTag(DeleteTagRequest) returns (DeleteTagResponse);
  rpc MergeTags(MergeTagsRequest) returns (MergeTagsResponse);

  // Entity tagging
  rpc TagEntity(TagEntityRequest) returns (TagEntityResponse);
  rpc UntagEntity(UntagEntityRequest) returns (UntagEntityResponse);
  rpc GetEntityTags(GetEntityTagsRequest) returns (GetEntityTagsResponse);
  rpc GetTaggedEntities(GetTaggedEntitiesRequest) returns (GetTaggedEntitiesResponse);
  rpc BulkTagEntities(BulkTagEntitiesRequest) returns (BulkTagEntitiesResponse);

  // Tag discovery
  rpc SuggestTags(SuggestTagsRequest) returns (SuggestTagsResponse);
  rpc GetTagStats(GetTagStatsRequest) returns (GetTagStatsResponse);
}
```

---

### 4.2 CollectionsService

**Purpose**: Hierarchical organization with multi-parent support

**Features:**
- Create nested collections (unlimited depth)
- Items can belong to multiple collections
- Collection sharing (linked to permissions)
- Smart/saved collections (dynamic based on criteria)
- Collection ordering and display
- Collection icons/colors

**Proto Definition:**
```protobuf
message Collection {
  string id = 1;
  string name = 2;
  string description = 3;
  string parent_id = 4;  // empty for root collections
  string owner_id = 5;
  string owner_type = 6;  // "user" or "org"
  int32 display_order = 7;
  string icon = 8;
  string color = 9;
  bool is_smart = 10;  // saved search collection
  string smart_query = 11;  // JSON query for smart collections
  google.protobuf.Timestamp created_at = 12;
  google.protobuf.Timestamp updated_at = 13;
  int64 item_count = 14;
  repeated string path = 15;  // ancestor IDs for quick hierarchy lookup
}

message CollectionItem {
  string collection_id = 1;
  string entity_type = 2;
  string entity_id = 3;
  int32 display_order = 4;
  google.protobuf.Timestamp added_at = 5;
  string added_by = 6;
}

service CollectionsService {
  rpc CreateCollection(CreateCollectionRequest) returns (CreateCollectionResponse);
  rpc GetCollection(GetCollectionRequest) returns (GetCollectionResponse);
  rpc ListCollections(ListCollectionsRequest) returns (ListCollectionsResponse);
  rpc GetCollectionTree(GetCollectionTreeRequest) returns (GetCollectionTreeResponse);
  rpc UpdateCollection(UpdateCollectionRequest) returns (UpdateCollectionResponse);
  rpc DeleteCollection(DeleteCollectionRequest) returns (DeleteCollectionResponse);
  rpc MoveCollection(MoveCollectionRequest) returns (MoveCollectionResponse);

  // Collection items
  rpc AddToCollection(AddToCollectionRequest) returns (AddToCollectionResponse);
  rpc RemoveFromCollection(RemoveFromCollectionRequest) returns (RemoveFromCollectionResponse);
  rpc GetCollectionItems(GetCollectionItemsRequest) returns (GetCollectionItemsResponse);
  rpc GetEntityCollections(GetEntityCollectionsRequest) returns (GetEntityCollectionsResponse);
  rpc BulkAddToCollection(BulkAddToCollectionRequest) returns (BulkAddToCollectionResponse);

  // Smart collections
  rpc CreateSmartCollection(CreateSmartCollectionRequest) returns (CreateSmartCollectionResponse);
  rpc RefreshSmartCollection(RefreshSmartCollectionRequest) returns (RefreshSmartCollectionResponse);
}
```

---

### 4.3 OrganizationsService

**Purpose**: Team/group structures for collaboration

**Features:**
- Create organizations/groups/teams
- Organization types (private, public-closed, public-open)
- Nested teams within organizations
- Member management
- Organization settings
- Storage quotas

**Proto Definition:**
```protobuf
message Organization {
  string id = 1;
  string name = 2;
  string description = 3;
  string slug = 4;  // URL-friendly identifier
  string owner_id = 5;  // creator/primary owner
  OrgVisibility visibility = 6;
  OrgJoinPolicy join_policy = 7;
  string image_url = 8;
  google.protobuf.Timestamp created_at = 9;
  google.protobuf.Timestamp updated_at = 10;
  int64 member_count = 11;
  google.protobuf.Struct settings = 12;
  google.protobuf.Struct extras = 13;
}

enum OrgVisibility {
  ORG_VISIBILITY_UNSPECIFIED = 0;
  ORG_VISIBILITY_PRIVATE = 1;      // Hidden, invite-only
  ORG_VISIBILITY_PUBLIC = 2;       // Visible to all
}

enum OrgJoinPolicy {
  ORG_JOIN_POLICY_UNSPECIFIED = 0;
  ORG_JOIN_POLICY_INVITE_ONLY = 1;
  ORG_JOIN_POLICY_REQUEST = 2;     // Users can request to join
  ORG_JOIN_POLICY_OPEN = 3;        // Anyone can join
}

message Team {
  string id = 1;
  string org_id = 2;
  string name = 3;
  string description = 4;
  string parent_team_id = 5;  // for nested teams
  google.protobuf.Timestamp created_at = 6;
  google.protobuf.Timestamp updated_at = 7;
  int64 member_count = 8;
}

message OrgMember {
  string org_id = 1;
  string user_id = 2;
  string role = 3;  // "owner", "admin", "member"
  google.protobuf.Timestamp joined_at = 4;
  string invited_by = 5;
  MemberStatus status = 6;
}

enum MemberStatus {
  MEMBER_STATUS_UNSPECIFIED = 0;
  MEMBER_STATUS_PENDING = 1;   // Invitation pending
  MEMBER_STATUS_ACTIVE = 2;
  MEMBER_STATUS_SUSPENDED = 3;
}

service OrganizationsService {
  // Organization CRUD
  rpc CreateOrganization(CreateOrganizationRequest) returns (CreateOrganizationResponse);
  rpc GetOrganization(GetOrganizationRequest) returns (GetOrganizationResponse);
  rpc ListOrganizations(ListOrganizationsRequest) returns (ListOrganizationsResponse);
  rpc UpdateOrganization(UpdateOrganizationRequest) returns (UpdateOrganizationResponse);
  rpc DeleteOrganization(DeleteOrganizationRequest) returns (DeleteOrganizationResponse);

  // Membership
  rpc InviteMember(InviteMemberRequest) returns (InviteMemberResponse);
  rpc AcceptInvitation(AcceptInvitationRequest) returns (AcceptInvitationResponse);
  rpc RemoveMember(RemoveMemberRequest) returns (RemoveMemberResponse);
  rpc UpdateMemberRole(UpdateMemberRoleRequest) returns (UpdateMemberRoleResponse);
  rpc ListMembers(ListMembersRequest) returns (ListMembersResponse);
  rpc GetUserOrganizations(GetUserOrganizationsRequest) returns (GetUserOrganizationsResponse);

  // Teams (nested groups)
  rpc CreateTeam(CreateTeamRequest) returns (CreateTeamResponse);
  rpc GetTeam(GetTeamRequest) returns (GetTeamResponse);
  rpc ListTeams(ListTeamsRequest) returns (ListTeamsResponse);
  rpc UpdateTeam(UpdateTeamRequest) returns (UpdateTeamResponse);
  rpc DeleteTeam(DeleteTeamRequest) returns (DeleteTeamResponse);
  rpc AddTeamMember(AddTeamMemberRequest) returns (AddTeamMemberResponse);
  rpc RemoveTeamMember(RemoveTeamMemberRequest) returns (RemoveTeamMemberResponse);
}
```

---

### 4.4 PermissionsService

**Purpose**: Role-based access control for resources

**Features:**
- Define permission roles (viewer, editor, admin, owner)
- Custom roles with granular permissions
- Resource-level permissions
- Permission inheritance
- Permission checking

**Proto Definition:**
```protobuf
message Role {
  string id = 1;
  string name = 2;
  string description = 3;
  repeated string permissions = 4;  // "read", "write", "delete", "admin", "share"
  bool is_system = 5;  // built-in role vs custom
  string scope = 6;    // "global", "org", "resource"
  int32 priority = 7;  // higher priority wins in inheritance
}

message ResourcePermission {
  string resource_type = 1;  // "collection", "document", "org"
  string resource_id = 2;
  string principal_type = 3;  // "user", "team", "org", "public"
  string principal_id = 4;
  string role_id = 5;
  google.protobuf.Timestamp granted_at = 6;
  string granted_by = 7;
  google.protobuf.Timestamp expires_at = 8;  // optional expiration
}

message PermissionCheck {
  string user_id = 1;
  string resource_type = 2;
  string resource_id = 3;
  string permission = 4;  // the permission being checked
}

service PermissionsService {
  // Role management
  rpc CreateRole(CreateRoleRequest) returns (CreateRoleResponse);
  rpc GetRole(GetRoleRequest) returns (GetRoleResponse);
  rpc ListRoles(ListRolesRequest) returns (ListRolesResponse);
  rpc UpdateRole(UpdateRoleRequest) returns (UpdateRoleResponse);
  rpc DeleteRole(DeleteRoleRequest) returns (DeleteRoleResponse);

  // Permission grants
  rpc GrantPermission(GrantPermissionRequest) returns (GrantPermissionResponse);
  rpc RevokePermission(RevokePermissionRequest) returns (RevokePermissionResponse);
  rpc GetResourcePermissions(GetResourcePermissionsRequest) returns (GetResourcePermissionsResponse);
  rpc GetUserPermissions(GetUserPermissionsRequest) returns (GetUserPermissionsResponse);

  // Permission checks
  rpc CheckPermission(CheckPermissionRequest) returns (CheckPermissionResponse);
  rpc BatchCheckPermissions(BatchCheckPermissionsRequest) returns (BatchCheckPermissionsResponse);
  rpc GetEffectivePermissions(GetEffectivePermissionsRequest) returns (GetEffectivePermissionsResponse);
}
```

---

### 4.5 SharingService

**Purpose**: Share resources with users, teams, or publicly

**Features:**
- Share links with optional expiration
- Share with specific users/teams
- Public/private sharing
- Share tracking (views, downloads)
- Share notifications

**Proto Definition:**
```protobuf
message Share {
  string id = 1;
  string resource_type = 2;
  string resource_id = 3;
  ShareType type = 4;
  string shared_by = 5;
  google.protobuf.Timestamp created_at = 6;
  google.protobuf.Timestamp expires_at = 7;
  string access_level = 8;  // "view", "comment", "edit"
  bool requires_auth = 9;
  string password_hash = 10;  // optional password protection
  int64 view_count = 11;
  int64 download_count = 12;
  google.protobuf.Struct settings = 13;
}

enum ShareType {
  SHARE_TYPE_UNSPECIFIED = 0;
  SHARE_TYPE_LINK = 1;       // Anyone with link
  SHARE_TYPE_USER = 2;       // Specific user
  SHARE_TYPE_TEAM = 3;       // Specific team
  SHARE_TYPE_ORG = 4;        // Organization-wide
  SHARE_TYPE_PUBLIC = 5;     // Fully public
}

message ShareRecipient {
  string share_id = 1;
  string recipient_type = 2;  // "user", "team", "email"
  string recipient_id = 3;
  google.protobuf.Timestamp notified_at = 4;
  google.protobuf.Timestamp accessed_at = 5;
}

service SharingService {
  rpc CreateShare(CreateShareRequest) returns (CreateShareResponse);
  rpc GetShare(GetShareRequest) returns (GetShareResponse);
  rpc ListShares(ListSharesRequest) returns (ListSharesResponse);
  rpc UpdateShare(UpdateShareRequest) returns (UpdateShareResponse);
  rpc DeleteShare(DeleteShareRequest) returns (DeleteShareResponse);

  // Share access
  rpc AccessShare(AccessShareRequest) returns (AccessShareResponse);
  rpc ValidateShareAccess(ValidateShareAccessRequest) returns (ValidateShareAccessResponse);

  // Recipients
  rpc AddShareRecipient(AddShareRecipientRequest) returns (AddShareRecipientResponse);
  rpc RemoveShareRecipient(RemoveShareRecipientRequest) returns (RemoveShareRecipientResponse);
  rpc ListShareRecipients(ListShareRecipientsRequest) returns (ListShareRecipientsResponse);

  // Analytics
  rpc GetShareStats(GetShareStatsRequest) returns (GetShareStatsResponse);
  rpc RecordShareAccess(RecordShareAccessRequest) returns (RecordShareAccessResponse);
}
```

---

### 4.6 NotesService

**Purpose**: Notes and annotations attached to entities

**Features:**
- Standalone notes
- Entity-attached notes
- Rich text content
- Note versioning
- Collaborative notes
- Note search

**Proto Definition:**
```protobuf
message Note {
  string id = 1;
  string title = 2;
  string content = 3;  // Markdown or HTML
  string content_type = 4;  // "markdown", "html", "plain"
  string owner_id = 5;
  NoteAttachment attachment = 6;
  google.protobuf.Timestamp created_at = 7;
  google.protobuf.Timestamp updated_at = 8;
  repeated string tags = 9;
  bool is_pinned = 10;
  string color = 11;
  google.protobuf.Struct extras = 12;
}

message NoteAttachment {
  string entity_type = 1;  // empty for standalone
  string entity_id = 2;
  string position = 3;     // for annotations: page/location
  string selection = 4;    // selected text/region
}

message NoteVersion {
  string id = 1;
  string note_id = 2;
  string content = 3;
  string edited_by = 4;
  google.protobuf.Timestamp created_at = 5;
}

service NotesService {
  rpc CreateNote(CreateNoteRequest) returns (CreateNoteResponse);
  rpc GetNote(GetNoteRequest) returns (GetNoteResponse);
  rpc ListNotes(ListNotesRequest) returns (ListNotesResponse);
  rpc UpdateNote(UpdateNoteRequest) returns (UpdateNoteResponse);
  rpc DeleteNote(DeleteNoteRequest) returns (DeleteNoteResponse);

  // Entity notes
  rpc GetEntityNotes(GetEntityNotesRequest) returns (GetEntityNotesResponse);
  rpc AttachNote(AttachNoteRequest) returns (AttachNoteResponse);
  rpc DetachNote(DetachNoteRequest) returns (DetachNoteResponse);

  // Versioning
  rpc GetNoteVersions(GetNoteVersionsRequest) returns (GetNoteVersionsResponse);
  rpc RestoreNoteVersion(RestoreNoteVersionRequest) returns (RestoreNoteVersionResponse);

  // Search
  rpc SearchNotes(SearchNotesRequest) returns (SearchNotesResponse);
}
```

---

### 4.7 RelationsService

**Purpose**: Link related items together

**Features:**
- Create bidirectional relations
- Relation types (related, parent-child, references)
- Bulk relation management
- Relation traversal

**Proto Definition:**
```protobuf
message Relation {
  string id = 1;
  string source_type = 2;
  string source_id = 3;
  string target_type = 4;
  string target_id = 5;
  string relation_type = 6;  // "related", "parent", "child", "references", "referenced_by"
  google.protobuf.Timestamp created_at = 7;
  string created_by = 8;
  google.protobuf.Struct metadata = 9;
}

service RelationsService {
  rpc CreateRelation(CreateRelationRequest) returns (CreateRelationResponse);
  rpc DeleteRelation(DeleteRelationRequest) returns (DeleteRelationResponse);
  rpc GetRelations(GetRelationsRequest) returns (GetRelationsResponse);
  rpc GetRelatedEntities(GetRelatedEntitiesRequest) returns (GetRelatedEntitiesResponse);
  rpc BulkCreateRelations(BulkCreateRelationsRequest) returns (BulkCreateRelationsResponse);
}
```

---

## Part 5: Implementation Strategy

### 5.1 Priority Order

Based on usage across services and foundational nature:

| Priority | Service | Rationale |
|----------|---------|-----------|
| 1 | **TagsService** | Universal, low complexity, immediate value |
| 2 | **CollectionsService** | Core organization primitive |
| 3 | **OrganizationsService** | Enables sharing/collaboration |
| 4 | **PermissionsService** | Required for secure sharing |
| 5 | **SharingService** | Builds on permissions |
| 6 | **NotesService** | Common feature, moderate complexity |
| 7 | **RelationsService** | Advanced feature, lower priority |

### 5.2 Backend Implementation Notes

#### FileSystem (fs)
- Store as JSON files in structured directories
- Example: `{storageDir}/tags/{tagId}/tag.json`
- EntityTags in separate index files for efficient queries
- Good for development/testing

#### GORM
- Define SQL models with proper indexes
- Many-to-many relationships via join tables
- JSON columns for flexible fields (extras)
- Support PostgreSQL, MySQL, SQLite

#### GAE (Datastore)
- Entity kinds with appropriate indexing
- Composite indexes for common queries
- Namespace support for multi-tenancy
- Consider ancestor keys for consistency

### 5.3 Service Dependencies

```
┌─────────────┐
│   Users     │ (existing)
└──────┬──────┘
       │
       ▼
┌─────────────┐     ┌─────────────┐
│Organizations│◄────│    Teams    │
└──────┬──────┘     └─────────────┘
       │
       ▼
┌─────────────┐     ┌─────────────┐
│ Permissions │◄────│   Roles     │
└──────┬──────┘     └─────────────┘
       │
       ▼
┌─────────────┐
│   Sharing   │
└─────────────┘

┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│    Tags     │     │ Collections │     │    Notes    │
└─────────────┘     └─────────────┘     └─────────────┘
       │                   │                   │
       └───────────────────┴───────────────────┘
                           │
                           ▼
                   ┌─────────────┐
                   │  Relations  │
                   └─────────────┘
```

---

## Part 6: Comparison Summary Table

### What We're Adding vs Competitors

| Proposed Service | Zotero | Notion | Evernote | Google Drive | GitHub |
|-----------------|--------|--------|----------|--------------|--------|
| **TagsService** | Colored tags, auto tags | Properties | Nested tags | Labels | Labels |
| **CollectionsService** | Multi-parent | Pages | Notebooks | Folders | N/A |
| **OrganizationsService** | Groups | Teamspaces | N/A | Shared Drives | Orgs |
| **PermissionsService** | 3 roles | 4+ roles | 2 levels | 5 levels | 5+ custom |
| **SharingService** | Group libs | Page sharing | NB sharing | Link sharing | Collaborators |
| **NotesService** | Built-in | Pages | Core feature | N/A | Gists |
| **RelationsService** | Related items | Relations | N/A | N/A | Cross-refs |

---

## Appendix: Research Sources

### Zotero Documentation
- [Collections and Tags](https://www.zotero.org/support/collections_and_tags)
- [Groups](https://www.zotero.org/support/groups)
- [PDF Reader](https://www.zotero.org/support/pdf_reader)

### Competitor Documentation
- [Notion Sharing & Permissions](https://www.notion.com/help/sharing-and-permissions)
- [Evernote Tags](https://help.evernote.com/hc/en-us/articles/39651699994003)
- [Google Shared Drives](https://support.google.com/a/answer/7662202)
- [GitHub Repository Roles](https://docs.github.com/en/organizations/managing-user-access-to-your-organizations-repositories)
- [Raindrop Collections](https://help.raindrop.io/collections/)
