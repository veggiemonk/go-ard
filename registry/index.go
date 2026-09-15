package registry

import (
	"context"
	"errors"
	"fmt"

	ard "github.com/veggiemonk/go-ard"
)

// ErrInvalidArgument reports a request that the index cannot answer because the request
// is wrong. The handler answers it with 400 INVALID_ARGUMENT. See appendix B.
var ErrInvalidArgument = errors.New("registry: invalid argument")

// ErrUnsupportedFilter reports a filter key or a facet field that the index does not
// index. Section 5.3.1 lets a registry reject one.
var ErrUnsupportedFilter = fmt.Errorf("%w: unsupported term path", ErrInvalidArgument)

// ErrBadPageToken reports a page token that belongs to no query the index knows.
var ErrBadPageToken = fmt.Errorf("%w: bad page token", ErrInvalidArgument)

// Constraint is one resolved filter key and the values it accepts. Section 5.3.1
// combines the values of one key with OR and the keys with AND.
type Constraint struct {
	Path   ard.TermPath
	Values []string
}

// ResolvedQuery is the query object of section 5.3.1 whose filter keys the handler has
// already resolved.
type ResolvedQuery struct {
	Text   string
	Filter []Constraint
}

// SearchQuery is one page of a search. See section 5.3.2 and ADR-0002.
type SearchQuery struct {
	Query     ResolvedQuery
	PageSize  int
	PageToken string
}

// Page is one page of search results. An empty PageToken ends the walk.
type Page struct {
	Results   []ard.Result
	PageToken string
}

// Facet asks for the aggregation of one resolved term path. See section 5.3.3.
type Facet struct {
	Path     ard.TermPath
	Limit    int
	MinCount int
}

// ExploreQuery is an aggregation over the matched set. See section 5.3.3.
type ExploreQuery struct {
	Query  ResolvedQuery
	Facets []Facet
}

// ListQuery is a deterministic listing. Filter holds the expression of appendix A and
// OrderBy the sort order of section 5.3.4.
type ListQuery struct {
	Filter    string
	OrderBy   string
	PageSize  int
	PageToken string
}

// ListPage is one page of a deterministic listing.
type ListPage struct {
	Items     []ard.Entry
	Total     int
	PageToken string
}

// Index is the search back end of a registry.
//
// Search is mandatory. Explore and List are optional, and an index that implements
// neither returns ard.ErrNotImplemented, which the handler answers with 501. An index
// reports a term path it does not index with ErrUnsupportedFilter and a page token it
// did not write with ErrBadPageToken.
type Index interface {
	Search(ctx context.Context, q SearchQuery) (Page, error)
	Explore(ctx context.Context, q ExploreQuery) (map[string]ard.FacetResult, error)
	List(ctx context.Context, q ListQuery) (ListPage, error)
}
