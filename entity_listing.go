package goapplib

// EntityListingData provides data for the EntityListing template component.
// This struct is designed to work with the EntityListing.html template.
type EntityListingData struct {
	// Page header
	Title       string
	Subtitle    string
	CreateUrl   string
	CreateLabel string

	// View configuration
	ViewMode            string // "grid" or "list"
	ViewModeStorageKey  string // localStorage key for saving preference
	EnableViewToggle    bool
	GridContainerId     string
	SearchInputId       string
	SortSelectId        string
	SearchPlaceholder   string

	// URLs
	ViewUrl   string
	EditUrl   string
	DeleteUrl string
	SearchUrl string
	RefreshUrl string

	// Sort options
	SortOptions []SortOption

	// Items to display (must implement EntityItem interface or have Id, Name, Description fields)
	Items []EntityItem

	// Actions
	ShowActions bool
	HtmxEnabled bool
	RefreshTrigger string

	// Empty state
	EmptyTitle   string
	EmptyMessage string
}

// SortOption represents a sort dropdown option
type SortOption struct {
	Value    string
	Label    string
	Selected bool
}

// EntityItem is the interface for items displayed in the entity listing.
// Items passed to EntityListingData.Items should implement this interface.
type EntityItem interface {
	GetId() string
	GetName() string
	GetDescription() string
	GetPreviewUrl() string
}

// EntityItemAdapter wraps any struct to implement EntityItem
type EntityItemAdapter struct {
	Id          string
	Name        string
	Description string
	PreviewUrl  string
	// Extra allows passing additional fields to templates
	Extra map[string]interface{}
}

func (e EntityItemAdapter) GetId() string          { return e.Id }
func (e EntityItemAdapter) GetName() string        { return e.Name }
func (e EntityItemAdapter) GetDescription() string { return e.Description }
func (e EntityItemAdapter) GetPreviewUrl() string  { return e.PreviewUrl }

// NewEntityListingData creates a new EntityListingData with sensible defaults.
func NewEntityListingData(title string, viewUrl string) *EntityListingData {
	return &EntityListingData{
		Title:              title,
		ViewUrl:            viewUrl,
		ViewMode:           "grid",
		ViewModeStorageKey: "entity-view-mode",
		EnableViewToggle:   true,
		GridContainerId:    "entity-grid",
		SearchInputId:      "search-entities",
		SortSelectId:       "sort-entities",
		SearchPlaceholder:  "Search...",
		ShowActions:        true,
		SortOptions: []SortOption{
			{Value: "updated", Label: "Last Modified", Selected: true},
			{Value: "name", Label: "Name"},
			{Value: "created", Label: "Date Created"},
		},
	}
}

// WithCreate sets the create URL and label
func (d *EntityListingData) WithCreate(url, label string) *EntityListingData {
	d.CreateUrl = url
	d.CreateLabel = label
	return d
}

// WithEdit sets the edit URL
func (d *EntityListingData) WithEdit(url string) *EntityListingData {
	d.EditUrl = url
	return d
}

// WithDelete sets the delete URL
func (d *EntityListingData) WithDelete(url string) *EntityListingData {
	d.DeleteUrl = url
	return d
}

// WithHtmx enables HTMX for the listing
func (d *EntityListingData) WithHtmx(searchUrl string) *EntityListingData {
	d.HtmxEnabled = true
	d.SearchUrl = searchUrl
	return d
}
