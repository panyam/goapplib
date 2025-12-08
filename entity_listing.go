package goapplib

// EntityListingData provides data for the EntityListing template component.
// This struct is designed to work with the EntityListing.html template.
// ItemType is the type of items in the listing - templates access item fields directly (e.g., .Id, .Name).
type EntityListingData[ItemType any] struct {
	// Page header
	Title       string
	Subtitle    string
	CreateUrl   string
	CreateLabel string

	// View configuration
	ViewMode           string // "grid" or "list"
	ViewModeStorageKey string // localStorage key for saving preference
	EnableViewToggle   bool
	GridContainerId    string
	SearchInputId      string
	SortSelectId       string
	SearchPlaceholder  string

	// URLs
	ViewUrl    string
	EditUrl    string
	DeleteUrl  string
	SearchUrl  string
	RefreshUrl string

	// Sort options
	SortOptions []SortOption

	// Items to display - templates access fields directly (e.g., .Id, .Name, .Description)
	Items []ItemType

	// Actions
	ShowActions    bool
	HtmxEnabled    bool
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

// NewEntityListingData creates a new EntityListingData with sensible defaults.
func NewEntityListingData[ItemType any](title string, viewUrl string) *EntityListingData[ItemType] {
	return &EntityListingData[ItemType]{
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
func (d *EntityListingData[ItemType]) WithCreate(url, label string) *EntityListingData[ItemType] {
	d.CreateUrl = url
	d.CreateLabel = label
	return d
}

// WithEdit sets the edit URL
func (d *EntityListingData[ItemType]) WithEdit(url string) *EntityListingData[ItemType] {
	d.EditUrl = url
	return d
}

// WithDelete sets the delete URL
func (d *EntityListingData[ItemType]) WithDelete(url string) *EntityListingData[ItemType] {
	d.DeleteUrl = url
	return d
}

// WithHtmx enables HTMX for the listing
func (d *EntityListingData[ItemType]) WithHtmx(searchUrl string) *EntityListingData[ItemType] {
	d.HtmxEnabled = true
	d.SearchUrl = searchUrl
	return d
}
