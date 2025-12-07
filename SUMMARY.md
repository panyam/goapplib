# goapplib - Web Application Library

A lightweight, stdlib-native Go library for building server-rendered web applications.

## Module Structure

```
goapplib/
├── go.mod              # Module definition
├── USAGE_GUIDE.md      # Comprehensive documentation
├── SUMMARY.md          # This file
│
├── app.go              # App type, template setup, default funcs
├── view.go             # View, Loader, PageGroup interfaces
├── mixins.go           # BasePage, WithPagination, WithFiltering, WithAuth, WithHtmx
├── htmx.go             # HtmxResponse helpers
├── register.go         # Register, RegisterGroup, RegisterFunc, RegisterHandler
├── muxbuilder.go       # Fluent MuxBuilder API
│
└── templates/          # Base templates (copy/symlink to your app)
    ├── BasePage.html
    ├── Header.html
    └── components/
        ├── Drawer.html
        ├── EntityGrid.html
        ├── EntityTable.html
        ├── Modal.html
        ├── Pagination.html
        ├── SearchFilter.html
        ├── SplashScreen.html
        └── Toast.html
```

## Key Features

### 1. Generic ViewContext
Your application defines its own ViewContext type. The library uses Go generics to work with any ViewContext.

### 2. stdlib-Native
Everything uses `*http.ServeMux` and `http.Handler`. No custom router.

### 3. Composable Mixins
Embed common behaviors in your pages:
- `BasePage` - Page metadata (title, body class, etc.)
- `WithPagination` - Pagination state and helpers
- `WithFiltering` - Search query, sort, view mode
- `WithAuth` - Authentication state
- `WithHtmx` - HTMX request detection

### 4. Chain Loading
```go
goal.LoadAll(r, w, vc, &p.BasePage, &p.WithPagination, &p.WithAuth)
```

### 5. Simple Registration
```go
goal.Register[HomePage](app, mux, "/")
goal.RegisterGroup[GamesGroup](app, mux, "/games")
```

### 6. HTMX-Ready
- `WithHtmx` mixin for request detection
- `HtmxResponse` helpers for response headers
- HTMX-aware template components

### 7. Responsive Components
Templates include mobile-friendly patterns:
- Drawers with swipe gestures
- Bottom navigation bar
- Responsive grids

## Quick Start

See `USAGE_GUIDE.md` for complete documentation.

```go
// 1. Define ViewContext
type ViewContext struct {
    ClientMgr *services.ClientMgr
}

// 2. Define Page
type HomePage struct {
    goal.BasePage
}

func (p *HomePage) Load(r *http.Request, w http.ResponseWriter, vc *ViewContext) (error, bool) {
    p.Title = "Home"
    return nil, false
}

// 3. Setup App
vc := &ViewContext{ClientMgr: services.NewClientMgr()}
templates := goal.SetupTemplates("./templates")
app := goal.NewApp(vc, templates)

// 4. Register Routes
mux := http.NewServeMux()
goal.Register[HomePage](app, mux, "/")

// 5. Serve
http.ListenAndServe(":8080", mux)
```

## Template Installation

Since Go modules can't export static files, copy or symlink the templates directory:

```bash
# Option 1: Symlink
ln -s $(go list -m -f '{{.Dir}}' github.com/panyam/goapplib)/templates ./templates/goapplib

# Option 2: Copy
cp -r $(go list -m -f '{{.Dir}}' github.com/panyam/goapplib)/templates ./templates/goapplib
```

Then configure template loader with fallback:
```go
templates := goal.SetupTemplates(
    "./templates",                    // Your overrides
    "./templates/goapplib",             // Library defaults
)
```

## Next Steps

- [x] Publish to github.com/panyam/goapplib
- [ ] Add more component templates
- [ ] Add form helpers
- [ ] Add middleware utilities
