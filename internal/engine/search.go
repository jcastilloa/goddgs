package engine

import "context"

// SearchRequest carries the shared source inputs for one engine call.
type SearchRequest struct {
	Query      string
	Region     string
	SafeSearch string
	TimeLimit  *string
	Page       int
	Parameters []SearchParameter
}

// SearchParameter is one source keyword argument in caller-supplied order.
// It intentionally retains source names such as type_image instead of exposing
// a transport-specific representation.
type SearchParameter struct {
	Name  string
	Value string
}

// Searcher is implemented by one source engine adapter.
type Searcher interface {
	Search(context.Context, SearchRequest) ([]Result, error)
}
