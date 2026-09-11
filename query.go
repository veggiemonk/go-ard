package ard

// FederationMode selects the federation topology of a search. See section 5.4.
type FederationMode string

// The federation modes of section 5.4.
const (
	FederationAuto      FederationMode = "auto"
	FederationReferrals FederationMode = "referrals"
	FederationNone      FederationMode = "none"
)

// Limits and defaults of the registry REST API. See sections 5.3.2, 5.3.3 and 5.3.4.
const (
	DefaultSearchPageSize = 10
	MaxSearchPageSize     = 100
	DefaultListPageSize   = 20
	MaxListPageSize       = 100
	DefaultFacetLimit     = 20
	DefaultFacetMinCount  = 1
)

// Query is the query object that Search and Explore share. See section 5.3.1.
//
// Filter values are arrays. A bare scalar in the wire format decodes to a one-element
// array. Within one key the values combine with OR, across keys with AND.
type Query struct {
	Context Context             `json:"@context,omitempty"`
	Text    string              `json:"text,omitempty"`
	Filter  map[string][]string `json:"filter,omitempty"`
}

// SearchRequest is the body of POST /search. Section 5.3.2 requires Query.Text.
type SearchRequest struct {
	Query      Query          `json:"query"`
	Federation FederationMode `json:"federation,omitempty"`
	PageSize   int            `json:"pageSize,omitempty"`
	PageToken  string         `json:"pageToken,omitempty"`
}

// SearchResponse is the body of a successful POST /search. See section 5.3.2.
type SearchResponse struct {
	Results   []Result   `json:"results"`
	Referrals []Referral `json:"referrals,omitempty"`
	PageToken string     `json:"pageToken,omitempty"`
}

// Result is one search result. Section 5.3.2 requires only the identifier of the entry,
// so Score is a pointer and Source may be empty.
type Result struct {
	Entry  Entry
	Score  *int
	Source string
}

// Referral names another registry that the client may query. See section 5.4.
type Referral struct {
	Identifier  string `json:"identifier"`
	DisplayName string `json:"displayName"`
	Type        string `json:"type"`
	URL         string `json:"url"`
}

// ExploreRequest is the body of POST /explore. See section 5.3.3.
type ExploreRequest struct {
	Query      Query        `json:"query,omitempty"`
	ResultType ExploreShape `json:"resultType"`
}

// ExploreShape is the shape of result that Explore computes. Facets is the only shape
// this version of the specification defines.
type ExploreShape struct {
	Facets []FacetRequest `json:"facets,omitempty"`
}

// FacetRequest asks for the aggregation of one term path. See section 5.3.3.
type FacetRequest struct {
	Field    string `json:"field"`
	Limit    int    `json:"limit,omitempty"`
	MinCount int    `json:"minCount,omitempty"`
}

// ExploreResponse is the body of a successful POST /explore. See section 5.3.3.
type ExploreResponse struct {
	ResultType string                 `json:"resultType"`
	Facets     map[string]FacetResult `json:"facets"`
}

// FacetResult is the aggregation of one field. OtherCount reports the matching entries
// in the buckets beyond the limit.
type FacetResult struct {
	Buckets    []Bucket `json:"buckets"`
	OtherCount *int     `json:"otherCount,omitempty"`
}

// Bucket is one value of a facet and the number of entries that carry it.
type Bucket struct {
	Value string `json:"value"`
	Count *int   `json:"count,omitempty"`
}

// ListResponse is the body of a successful GET /agents. See section 5.3.4.
type ListResponse struct {
	Items     []Entry `json:"items"`
	Total     *int    `json:"total,omitempty"`
	PageToken string  `json:"pageToken,omitempty"`
}

// ErrorBody is the error payload of appendix B.
type ErrorBody struct {
	ErrorCode string `json:"errorCode"`
	Message   string `json:"message"`
}
