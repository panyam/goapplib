# GoAppLib

## Version
0.1.1

## Provides
- web-app-scaffold: Server-rendered web application framework (stdlib-native)
- view-context: Generic ViewContext type system for page rendering
- page-mixins: Composable mixins (WithPagination, WithFiltering, WithAuth, WithHtmx)
- htmx-support: HTMX request detection and response utilities
- responsive-ui: Built-in UI components (drawers, modals, pagination, search filters)
- border-layout: 5-region layout component (North/South/East/West/Center) with pure CSS flexbox
- users-service: UsersService with multi-backend support (FS, GORM, Google Datastore)
- auth-integration: Integration with oneauth for authentication
- template-management: Template management via Templar integration
- rate-limiting: Rate limiting middleware for auth vs API endpoints
- admin-pages: Admin pages and user management

## Module
github.com/panyam/goapplib

## Location
newstack/goapplib/main

## Stack Dependencies
- goutils v0.1.13 (github.com/panyam/goutils)
- oneauth v0.1.13 (github.com/panyam/oneauth) — uses accounts + federatedauth + localauth + stores/fs subpackages. v0.1.x stores use a context + request/response API (CreateUser(ctx, *CreateUserRequest) etc.)
- protoc-gen-dal (github.com/panyam/protoc-gen-dal)
- templar v0.1.0 (github.com/panyam/templar) — NewFileSystemLoader now takes FSFolder values; use templar.LocalFolder(path) for local directories

## Integration

### Go Module
```go
// go.mod
require github.com/panyam/goapplib 0.0.26

// Local development
replace github.com/panyam/goapplib => ~/newstack/goapplib/main
```

### Key Imports
```go
import "github.com/panyam/goapplib/views"
```

## Status
Active

## Conventions
- Generic ViewContext
- Stdlib-native (no custom router)
- Mixin-based composition
- BasePage mixin pattern
