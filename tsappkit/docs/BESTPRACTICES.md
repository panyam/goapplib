# Documentation Site Best Practices

This document captures best practices for building documentation sites using tsappkit, s3gen, and related tools.

## Architecture Overview

### Stack Components

| Component | Purpose |
|-----------|---------|
| **s3gen** | Go-based static site generator with 4-phase pipeline |
| **Templar** | Template dependency management for Go templates |
| **Webpack** | TypeScript/CSS bundling |
| **tsappkit** | Shared documentation components and styles |
| **DockView** | Multi-panel resizable layouts for playgrounds |
| **Ace Editor** | Code editing with syntax highlighting |

## Project Structure

```
docs/
├── main.go                 # s3gen site configuration
├── go.mod                  # Go module (links to s3gen, templar, goutils)
├── Makefile                # Build commands
├── package.json            # npm dependencies
├── tsconfig.json           # TypeScript config
├── webpack.config.js       # Webpack bundler config
├── content/                # Documentation content
│   ├── index.html          # Homepage (HTML for custom layouts)
│   ├── SiteMetadata.json   # Site name, description
│   ├── HeaderNavLinks.json # Navigation structure
│   ├── getting-started/
│   │   └── index.md        # Markdown content
│   └── playground/
│       └── index.html      # Interactive page (needs special handling)
├── templates/              # Templar templates
│   ├── BasePage.html       # Main layout
│   ├── Header.html         # Navigation header
│   ├── Sidebar.html        # Documentation sidebar
│   ├── Content.html        # Content renderer (MD/HTML)
│   └── Footer.html         # Footer
├── static/
│   ├── css/
│   │   ├── main.css        # Aggregator (imports components)
│   │   └── components/     # Modular CSS files
│   │       ├── variables.css
│   │       ├── tokens.css
│   │       └── playground.css
│   └── js/gen/             # Webpack output
└── components/             # TypeScript source
    ├── DocsPage.ts         # Main entry point
    └── playground/
        └── PlaygroundPage.ts
```

## CSS Organization

### Modular CSS Pattern

Organize CSS into separate component files under `static/css/components/`:

```css
/* main.css - Aggregator */
@import url('./components/variables.css');
@import url('./components/tokens.css');
@import url('./components/playground.css');
```

### Using tsappkit Styles

**Important:** Load base documentation styles via static `<link>` tag to prevent Flash of Unstyled Content (FOUC). Do NOT import via webpack/JavaScript.

1. Copy the CSS during build (in Makefile):
```makefile
copycss:
	cp node_modules/@panyam/tsappkit/dist/docs/DocsPage.css static/css/tsappkit-docs.css
```

2. Include in HTML `<head>` (in BasePage.html):
```html
<link rel="stylesheet" href="{{.Site.PathPrefix}}/static/css/tsappkit-docs.css">
<link rel="stylesheet" href="{{.Site.PathPrefix}}/static/css/main.css">
```

This ensures CSS loads before content renders, eliminating the flash where content appears unstyled.

### Theme Variables

Define custom theme variables in `components/variables.css`:

```css
:root {
  --color-primary: #7c3aed;        /* Brand color */
  --color-primary-dark: #6d28d9;
  --color-accent: #14b8a6;
  --color-bg: #fafafa;
  --color-bg-alt: #f1f5f9;
}

:root.dark {
  --color-bg: #0f0f1a;
  --color-bg-alt: #1a1a2e;
  --color-primary: #a78bfa;
}
```

## Webpack Configuration

### Entry Points

Define separate entry points for different page types:

```javascript
entry: {
  DocsPage: path.join(__dirname, "./components/DocsPage.ts"),
  PlaygroundPage: path.join(__dirname, "./components/playground/PlaygroundPage.ts"),
},
```

### Generated Templates

Create HtmlWebpackPlugin for each entry to generate script include files:

```javascript
new HtmlWebpackPlugin({
  chunks: ["DocsPage"],
  filename: path.resolve(__dirname, "./templates/gen.DocsPage.html"),
  templateContent: "",
}),
```

### Conditional Script Loading

In BasePage.html, conditionally include the appropriate script:

```html
{{ if .FrontMatter.usePlayground }}
{{# include "gen.PlaygroundPage.html" #}}
{{ else }}
{{# include "gen.DocsPage.html" #}}
{{ end }}
```

## Template Patterns

### Markdown vs HTML Content

Handle both markdown and HTML content in Content.html:

```html
<div class="content-body">
    {{ if or (eq .Res.Ext ".md") (eq .Res.Ext ".mdx") }}
        {{ $parsed := ParseMD .Content }}
        {{ MDToHtml $parsed.Doc }}
    {{ else }}
        {{ BytesToString .Content | HTML }}
    {{ end }}
</div>
```

### Front Matter for Special Pages

Use front matter to control page behavior:

```yaml
---
title: "Playground"
usePlayground: true      # Load PlaygroundPage.js instead of DocsPage.js
fullWidth: true          # Skip sidebar, use full width
hideFooter: true         # Hide footer
bodyClass: "playground-page"  # Add CSS class to body
---
```

## DockView Integration

### Basic Setup

```typescript
import { DockviewComponent, DockviewApi } from "dockview-core";
import "dockview-core/dist/styles/dockview.css";

const dockview = new DockviewComponent(container, {
  createComponent: (options) => this.createComponent(options),
});
```

### Panel Creation Pattern

```typescript
private createComponent(options: any): any {
  switch (options.name) {
    case "rules":
      return this.createRulesPanel();
    case "input":
      return this.createInputPanel();
    // ...
  }
}

private createRulesPanel(): any {
  const template = document.getElementById("rules-panel-template");
  const element = template?.cloneNode(true) as HTMLElement;
  element.style.display = "flex";

  return {
    element,
    init: (params: any) => {
      // Setup event listeners, Ace editor, etc.
    },
  };
}
```

### Layout Persistence

Save and restore layouts to localStorage:

```typescript
private saveLayout(): void {
  const layout = this.dockview.toJSON();
  localStorage.setItem(LAYOUT_STORAGE_KEY, JSON.stringify(layout));
}

private loadLayout(): boolean {
  const saved = localStorage.getItem(LAYOUT_STORAGE_KEY);
  if (saved) {
    this.dockview.fromJSON(JSON.parse(saved));
    return true;
  }
  return false;
}
```

### Theme Support

Apply DockView theme based on document theme:

```typescript
container.className = this.isDarkMode()
  ? "dockview-theme-dark"
  : "dockview-theme-light";

// Watch for theme changes
const observer = new MutationObserver(() => {
  const isDark = this.isDarkMode();
  container.className = isDark ? "dockview-theme-dark" : "dockview-theme-light";
  this.updateEditorThemes(isDark);
});
observer.observe(document.documentElement, {
  attributes: true,
  attributeFilter: ["class"],
});
```

## tsappkit Exports

### Available Exports

tsappkit provides two separate entry points to avoid bundle bloat:

```typescript
// Core components (no heavy dependencies)
import { BaseComponent, EventBus, Modal } from "@panyam/tsappkit";

// Documentation components (requires ace-builds peer dep)
import { initCodeBlocks, initPageSetup, ActionCompiler } from "@panyam/tsappkit/docs";
```

**Note:** CSS should be loaded statically via `<link>` tags, not imported via JavaScript. See "Using tsappkit Styles" above.

### initCodeBlocks()

Adds copy-to-clipboard functionality to all `<pre><code>` blocks.

### initPageSetup()

- Highlights active sidebar links based on current URL
- Sets up common page initialization

## s3gen Configuration

### Site Configuration (main.go)

```go
var Site = &s3.Site{
    OutputDir:       "./dist/docs",
    ContentRoot:     "./content",
    PathPrefix:      "/myproject",  // URL prefix
    TemplateFolders: []string{"./templates"},
    StaticFolders:   []string{"/static/", "./static"},
    DefaultBaseTemplate: s3.BaseTemplate{
        Name: "BasePage.html",
        Params: map[any]any{"BodyTemplateName": "Content"},
    },
}
```

### Local Development Links (go.mod)

For development, link to local copies of dependencies:

```go
replace github.com/panyam/s3gen => ../../../golang/s3gen
replace github.com/panyam/templar => ../../../golang/templar
replace github.com/panyam/goutils => ../../../golang/goutils
```

## Event Communication Pattern

### Simple EventHub

```typescript
export class EventHub {
  private listeners: Map<string, Set<EventCallback>> = new Map();

  on(event: string, callback: EventCallback): void {
    if (!this.listeners.has(event)) {
      this.listeners.set(event, new Set());
    }
    this.listeners.get(event)!.add(callback);
  }

  emit(event: string, ...args: any[]): void {
    this.listeners.get(event)?.forEach(cb => cb(...args));
  }
}
```

### Event Constants

```typescript
export const Events = {
  LEXER_COMPILED: "lexerCompiled",
  TOKENS_GENERATED: "tokensGenerated",
  CONSOLE_LOG: "consoleLog",
};
```

## Testing & Verification

### Build Commands

```bash
# Install dependencies
pnpm install

# Build webpack bundles
pnpm build

# Start development server
make run
```

### Verification Checklist

- [ ] Homepage renders correctly
- [ ] Markdown pages convert to HTML
- [ ] Code blocks have copy buttons
- [ ] Sidebar highlights current page
- [ ] Theme toggle works (light/dark)
- [ ] Playground panels are resizable
- [ ] Layout persists across sessions
- [ ] Console shows compile/tokenize output

## Common Patterns

### Ace Editor Setup

```typescript
const editor = ace.edit(container);
editor.setTheme(isDark ? "ace/theme/monokai" : "ace/theme/github");
editor.session.setMode("ace/mode/text");
editor.setShowPrintMargin(false);
editor.setFontSize(14);
```

### Token Display

```html
<div class="token-row">
  <span class="token-index">0</span>
  <span class="token-range">0-5</span>
  <span class="token-tag">NUMBER</span>
  <span class="token-value">12345</span>
</div>
```

```css
.token-row {
  display: grid;
  grid-template-columns: 2.5rem 5rem 8rem 1fr;
  gap: 0.5rem;
}
```

## Troubleshooting

### Markdown vs HTML Content Rendering

Use conditional rendering in Content.html based on file extension:

```html
{{ if or (eq .Res.Ext ".md") (eq .Res.Ext ".mdx") }}
  {{ $parsed := ParseMD .Content }}
  {{ MDToHtml $parsed.Doc }}
{{ else }}
  {{ BytesToString .Content | HTML }}
{{ end }}
```

This ensures markdown files are parsed and rendered, while HTML files are output directly.

### Flash of Unstyled Content (FOUC)

**Problem:** Page flashes with unstyled content before CSS applies.

**Cause:** CSS is loaded via JavaScript (webpack import) instead of statically in the HTML head.

**Solution:** Load base CSS via static `<link>` tag in HTML head, not via JavaScript import:

```makefile
# Makefile - copy CSS during build
copycss:
	cp node_modules/@panyam/tsappkit/dist/docs/DocsPage.css static/css/tsappkit-docs.css
```

```html
<!-- BasePage.html - load BEFORE content renders -->
<link rel="stylesheet" href="{{.Site.PathPrefix}}/static/css/tsappkit-docs.css">
<link rel="stylesheet" href="{{.Site.PathPrefix}}/static/css/main.css">
```

Remove any CSS imports from your TypeScript entry points:
```typescript
// DON'T do this - causes FOUC
import "@panyam/tsappkit/dist/docs/DocsPage.css";
```

### CSS Not Loading from node_modules

Don't use `@import url()` for node_modules paths in CSS. Copy the CSS to your static folder during build (see FOUC solution above).

### DockView Not Theming

Ensure you set the correct class on the container:

```typescript
container.className = isDark ? "dockview-theme-dark" : "dockview-theme-light";
```

And define the theme variables in your CSS.
