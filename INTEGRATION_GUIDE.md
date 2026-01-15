# goapplib Integration Guide

Quick reference for integrating goapplib into new projects. See USAGE_GUIDE.md for full documentation.

## Minimal Working Example

### 1. Project Structure

```
mysite/
├── go.mod
├── main.go
├── app.yaml              # For App Engine
├── Makefile
├── server/
│   ├── app.go            # App context
│   └── views.go          # Page handlers
├── templates/
│   ├── templar.yaml      # Template config
│   ├── BasePage.html     # Your base layout
│   ├── Header.html
│   ├── Footer.html
│   ├── HomePage.html
│   └── templar_modules/  # Vendored goapplib templates (after `templar get`)
└── static/
    └── css/
```

### 2. go.mod

```go
module mysite

go 1.24.0

require (
    github.com/panyam/goapplib v0.0.13
    github.com/panyam/templar v0.0.28
)
```

### 3. server/app.go

```go
package server

type MyApp struct {
    AppName   string
    BaseURL   string
    GitHubURL string
}

func NewMyApp() *MyApp {
    return &MyApp{
        AppName:   "My Site",
        BaseURL:   "https://mysite.appspot.com",
        GitHubURL: "https://github.com/user/repo",
    }
}
```

### 4. server/views.go

```go
package server

import (
    "net/http"
    goal "github.com/panyam/goapplib"
)

// Header is a reusable component across pages
type Header struct {
    AppName string
}

func (h *Header) Load(r *http.Request, w http.ResponseWriter, app *goal.App[*MyApp]) (error, bool) {
    h.AppName = app.Context.AppName
    return nil, false
}

// HomePage - CRITICAL: Embed goal.BasePage, not a custom struct!
type HomePage struct {
    goal.BasePage  // Must be embedded for templates to work
    Header    Header
    GitHubURL string
}

func (p *HomePage) Load(r *http.Request, w http.ResponseWriter, app *goal.App[*MyApp]) (error, bool) {
    // Set BasePage fields directly
    p.Title = "Home - My Site"
    p.DisableSplashScreen = true

    // Set custom fields
    p.Header.AppName = app.Context.AppName
    p.GitHubURL = app.Context.GitHubURL

    return nil, false
}

// SetupRoutes registers all routes
func SetupRoutes(app *goal.App[*MyApp]) *http.ServeMux {
    mux := http.NewServeMux()
    goal.Register[*HomePage](app, mux, "/")
    return mux
}
```

### 5. main.go

```go
package main

import (
    "context"
    "log"
    "net/http"
    "os"
    "path/filepath"

    goal "github.com/panyam/goapplib"
    tmplr "github.com/panyam/templar"
    "mysite/server"
)

const TEMPLATES_FOLDER = "./templates"
const STATIC_FOLDER = "./static"

func main() {
    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }

    // Create app context
    appCtx := server.NewMyApp()

    // Setup templates with SourceLoader
    templates := tmplr.NewTemplateGroup()

    // CRITICAL: Use absolute path for templar config
    configPath, _ := filepath.Abs(filepath.Join(TEMPLATES_FOLDER, "templar.yaml"))
    sourceLoader, err := tmplr.NewSourceLoaderFromConfig(configPath)
    if err != nil {
        log.Printf("Warning: Could not load templar.yaml: %v", err)
        templates.Loader = tmplr.NewFileSystemLoader(TEMPLATES_FOLDER)
    } else {
        templates.Loader = sourceLoader
    }
    templates.AddFuncs(goal.DefaultFuncMap())

    // Create app
    app := goal.NewApp(appCtx, templates)

    // Setup routes
    mux := server.SetupRoutes(app)
    mux.Handle("/static/", http.StripPrefix("/static", http.FileServer(http.Dir(STATIC_FOLDER))))

    // Start server
    log.Printf("Starting server on :%s", port)
    log.Fatal(http.ListenAndServe(":"+port, mux))
}
```

### 6. templates/templar.yaml

```yaml
sources:
  goapplib:
    url: github.com/panyam/goapplib
    path: templates
    ref: main

vendor_dir: templar_modules
search_paths:
  - .
  - ./templar_modules
```

### 7. templates/BasePage.html

```html
{{# namespace "GoalBase" "@goapplib/BasePage.html" #}}
{{# include "./Header.html" #}}
{{# include "./Footer.html" #}}

{{ define "ExcaliframeExtraHeadSection" }}
<link rel="stylesheet" href="/static/css/style.css">
{{ end }}

{{ define "BodySection" }}
<!-- Override in page templates -->
{{ end }}

{{ define "PostBodySection" }}
{{ template "Footer" . }}
{{ end }}

{{# extend "GoalBase:BasePage" "BasePage"
           "GoalBase:ExtraHeadSection" "ExcaliframeExtraHeadSection"
           "GoalBase:BodySection" "BodySection"
           "GoalBase:PostBodySection" "PostBodySection" #}}
```

### 8. templates/HomePage.html

```html
{{# include "./BasePage.html" #}}

{{ define "BodySection" }}
<main class="max-w-4xl mx-auto px-4 py-8">
    <h1 class="text-3xl font-bold">{{ .Header.AppName }}</h1>
    <p>Welcome to my site!</p>
    <a href="{{ .GitHubURL }}">View on GitHub</a>
</main>
{{ end }}

{{ define "HomePage" }}
{{ template "BasePage" . }}
{{ end }}
```

### 9. Fetch Dependencies

```bash
cd templates
templar get
```

This creates `templar_modules/goapplib/` with base templates.

---

## Critical Patterns (Common Mistakes)

### 1. Page struct MUST embed goal.BasePage

```go
// CORRECT
type MyPage struct {
    goal.BasePage  // Required!
    MyField string
}

// WRONG - Templates will fail with "can't evaluate field Title"
type MyPage struct {
    BasePage MyCustomBasePage  // Don't do this
    MyField  string
}
```

### 2. Set Title in Load(), not in struct definition

```go
func (p *MyPage) Load(...) (error, bool) {
    p.Title = "My Page Title"  // Set here
    return nil, false
}
```

### 3. Use absolute path for templar config

```go
// CORRECT
configPath, _ := filepath.Abs(filepath.Join(TEMPLATES_FOLDER, "templar.yaml"))
sourceLoader, _ := tmplr.NewSourceLoaderFromConfig(configPath)

// WRONG - May fail with relative paths
sourceLoader, _ := tmplr.NewSourceLoaderFromConfig("./templates/templar.yaml")
```

### 4. Templates use @source/ prefix for vendored files

```html
{{# namespace "GoalBase" "@goapplib/BasePage.html" #}}
                         ^^^^^^^^^^
                         Source name from templar.yaml
```

---

## App Engine Deployment

### app.yaml

```yaml
runtime: go124

env_variables:
  MY_ENV: production

handlers:
- url: /static
  static_dir: static
- url: /.*
  script: auto
```

### Makefile

```makefile
run:
	go run .

deploy:
	gcloud app deploy --project myproject
```

---

## Reference Examples

- **excaliframe/site/** - Simple marketing site with 4 pages
- **lilbattle/web/** - Full web app with games, auth, pagination
