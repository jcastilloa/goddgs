package engine

// Metadata describes one frozen source engine without constructing its
// transport or parser adapter.
type Metadata struct {
	Name     string  `json:"name"`
	Category string  `json:"category"`
	Provider string  `json:"provider"`
	Priority float64 `json:"priority"`
	Disabled bool    `json:"disabled"`
}

// Category contains frozen source engines in Python discovery order.
type Category struct {
	Category string     `json:"category"`
	Engines  []Metadata `json:"engines"`
}

// Registry is a read-only view of source engine metadata.
type Registry struct {
	categories []Category
	disabled   []Metadata
}

// FrozenRegistry returns the frozen Python active registry and explicitly
// disabled source engines. Each call returns independent metadata storage.
func FrozenRegistry() Registry {
	return Registry{
		categories: cloneCategories(frozenCategories),
		disabled:   cloneMetadata(frozenDisabled),
	}
}

// Categories returns active source categories in frozen Python discovery order.
func (r Registry) Categories() []Category {
	return cloneCategories(r.categories)
}

// Active returns one active category's engines in frozen Python discovery order.
func (r Registry) Active(category string) ([]Metadata, bool) {
	for _, entry := range r.categories {
		if entry.Category == category {
			return cloneMetadata(entry.Engines), true
		}
	}
	return nil, false
}

// Disabled returns source-present metadata excluded from the active registry.
func (r Registry) Disabled() []Metadata {
	return cloneMetadata(r.disabled)
}

var frozenCategories = []Category{
	{
		Category: "books",
		Engines: []Metadata{
			{Name: "annasarchive", Category: "books", Provider: "annasarchive", Priority: 1},
		},
	},
	{
		Category: "images",
		Engines: []Metadata{
			{Name: "bing", Category: "images", Provider: "bing", Priority: 1},
			{Name: "duckduckgo", Category: "images", Provider: "bing", Priority: 1},
		},
	},
	{
		Category: "news",
		Engines: []Metadata{
			{Name: "bing", Category: "news", Provider: "bing", Priority: 1},
			{Name: "duckduckgo", Category: "news", Provider: "bing", Priority: 1},
			{Name: "yahoo", Category: "news", Provider: "yahoo", Priority: 1},
		},
	},
	{
		Category: "text",
		Engines: []Metadata{
			{Name: "brave", Category: "text", Provider: "brave", Priority: 1},
			{Name: "duckduckgo", Category: "text", Provider: "bing", Priority: 1},
			{Name: "google", Category: "text", Provider: "google", Priority: 1},
			{Name: "grokipedia", Category: "text", Provider: "grokipedia", Priority: 1.9},
			{Name: "mojeek", Category: "text", Provider: "mojeek", Priority: 1},
			{Name: "startpage", Category: "text", Provider: "google", Priority: 1},
			{Name: "wikipedia", Category: "text", Provider: "wikipedia", Priority: 2},
			{Name: "yahoo", Category: "text", Provider: "bing", Priority: 1},
			{Name: "yandex", Category: "text", Provider: "yandex", Priority: 1},
		},
	},
	{
		Category: "videos",
		Engines: []Metadata{
			{Name: "duckduckgo", Category: "videos", Provider: "bing", Priority: 1},
		},
	},
}

var frozenDisabled = []Metadata{
	{Name: "bing", Category: "text", Provider: "bing", Priority: 1, Disabled: true},
}

func cloneCategories(categories []Category) []Category {
	copyOfCategories := make([]Category, len(categories))
	for index, category := range categories {
		copyOfCategories[index] = Category{
			Category: category.Category,
			Engines:  cloneMetadata(category.Engines),
		}
	}
	return copyOfCategories
}

func cloneMetadata(metadata []Metadata) []Metadata {
	copyOfMetadata := make([]Metadata, len(metadata))
	copy(copyOfMetadata, metadata)
	return copyOfMetadata
}
