package goapplib

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"reflect"
	"strings"

	tmplr "github.com/panyam/templar"
)

// App holds app-level shared state and template system.
// AppContext is the application context type defined by your application (e.g., *WeewarApp).
type App[AppContext any] struct {
	Context            AppContext
	Templates          *tmplr.TemplateGroup
	RenderTemplateFunc func(w http.ResponseWriter, templateFileName string, templateBlockName string, view any) error
}

// NewApp creates a new App with the given application context and templates.
func NewApp[AppContext any](ctx AppContext, templates *tmplr.TemplateGroup) *App[AppContext] {
	return &App[AppContext]{
		Context:   ctx,
		Templates: templates,
	}
}

// RenderTemplate renders the named template with the given view data.
// If RenderTemplateFunc is set, it delegates to that function.
func (app *App[AppContext]) RenderTemplate(
	w http.ResponseWriter,
	templateFileName string,
	templateBlockName string,
	view any) error {
	if app.RenderTemplateFunc != nil {
		return app.RenderTemplateFunc(w, templateFileName, templateBlockName, view)
	}

	// Default implementation using Templates
	templateFile := templateFileName + ".html"
	tmpl, err := app.Templates.Loader.Load(templateFile, "")
	if err != nil {
		log.Printf("Template load error: %s - %v", templateFile, err)
		return fmt.Errorf("template load error: %s - %w", templateFile, err)
	}

	err = app.Templates.RenderHtmlTemplate(w, tmpl[0], templateBlockName, view, nil)
	if err != nil {
		log.Printf("Template render error: %s[%s] - %v", templateFileName, templateBlockName, err)
		return fmt.Errorf("template render error: %s[%s] - %w", templateFileName, templateBlockName, err)
	}

	return nil
}

// NewMux creates a MuxBuilder for fluent route building.
func (app *App[AppContext]) NewMux() *MuxBuilder[AppContext] {
	return &MuxBuilder[AppContext]{
		app: app,
		mux: http.NewServeMux(),
	}
}

// SetupTemplates creates a TemplateGroup with the given paths.
// Paths are checked in order, so put your app's templates first for overrides.
func SetupTemplates(paths ...string) *tmplr.TemplateGroup {
	templates := tmplr.NewTemplateGroup()

	loader := &tmplr.LoaderList{}
	for _, path := range paths {
		loader.AddLoader(tmplr.NewFileSystemLoader(path))
	}
	templates.Loader = loader

	// Add default functions
	templates.AddFuncs(DefaultFuncMap())

	return templates
}

// DefaultFuncMap returns the default template functions.
func DefaultFuncMap() template.FuncMap {
	return template.FuncMap{
		"safeHTML": func(s string) template.HTML {
			return template.HTML(s)
		},
		"safeHTMLAttr": func(s string) template.HTMLAttr {
			return template.HTMLAttr(s)
		},
		"safeJS": func(s string) template.JS {
			return template.JS(s)
		},
		"safeURL": func(s string) template.URL {
			return template.URL(s)
		},
		"default": func(defaultVal, val any) any {
			if val == nil {
				return defaultVal
			}
			if s, ok := val.(string); ok && s == "" {
				return defaultVal
			}
			return val
		},
		"dict": func(pairs ...any) map[string]any {
			m := make(map[string]any)
			for i := 0; i < len(pairs); i += 2 {
				if i+1 < len(pairs) {
					key, _ := pairs[i].(string)
					m[key] = pairs[i+1]
				}
			}
			return m
		},
		"list": func(items ...any) []any {
			return items
		},
		"eq": func(a, b any) bool {
			return a == b
		},
		"ne": func(a, b any) bool {
			return a != b
		},
		"or": func(vals ...any) any {
			for _, v := range vals {
				if v != nil {
					if s, ok := v.(string); ok && s != "" {
						return v
					}
					if b, ok := v.(bool); ok && b {
						return v
					}
					if !reflect.ValueOf(v).IsZero() {
						return v
					}
				}
			}
			if len(vals) > 0 {
				return vals[len(vals)-1]
			}
			return nil
		},
		"and": func(vals ...any) any {
			for _, v := range vals {
				if v == nil {
					return nil
				}
				if s, ok := v.(string); ok && s == "" {
					return nil
				}
				if b, ok := v.(bool); ok && !b {
					return nil
				}
			}
			if len(vals) > 0 {
				return vals[len(vals)-1]
			}
			return nil
		},
		"not": func(v any) bool {
			if v == nil {
				return true
			}
			if b, ok := v.(bool); ok {
				return !b
			}
			if s, ok := v.(string); ok {
				return s == ""
			}
			return reflect.ValueOf(v).IsZero()
		},
		// Dict/list mutation helpers
		"dset": func(d map[string]any, key string, value any) map[string]any {
			d[key] = value
			return d
		},
		"lset": func(a []any, index int, value any) []any {
			a[index] = value
			return a
		},
		// Formatting helpers
		"Indented": func(nspaces int, code string) string {
			lines := strings.Split(strings.TrimSpace(code), "\n")
			return strings.Join(lines, "<br/>")
		},
		// Generic ToJson (apps can override with protobuf-aware version)
		"ToJson": func(v any) template.JS {
			if v == nil {
				return template.JS("null")
			}
			jsonBytes, err := json.Marshal(v)
			if err != nil {
				log.Printf("Error marshaling to JSON: %v", err)
				return template.JS("null")
			}
			return template.JS(jsonBytes)
		},
	}
}

// templateNameFromType extracts the template name from a View type.
// For *GameListingPage, returns "GameListingPage".
func templateNameFromType[V any]() string {
	var v V
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}
