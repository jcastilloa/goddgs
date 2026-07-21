package engine

import "context"

// SearchRequest carries the shared source inputs for one engine call.
type SearchRequest struct {
	Query      string
	Region     string
	SafeSearch string
	TimeLimit  *string
	Page       int
}

// Searcher is implemented by one source engine adapter.
type Searcher interface {
	Search(context.Context, SearchRequest) ([]Result, error)
}
