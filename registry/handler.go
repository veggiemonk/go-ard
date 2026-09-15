package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"strconv"

	ard "github.com/veggiemonk/go-ard"
)

const maxRequestBytes = 4 << 20

// ResultTypeFacets is the only result shape that section 5.3.3 defines for Explore.
const ResultTypeFacets = "facets"

// Options configures a Handler.
type Options struct {
	// Source is the registry URL that every search result carries in its source member.
	Source string

	// Federator answers the federation modes of section 5.4. Nil searches only the local index.
	Federator *Federator
}

// Handler serves the registry REST API of section 5.3 over an index.
//
// It routes POST /search, POST /explore and GET /agents, answers any other method on
// those paths with 405 and any other path with 404. It checks the request against
// sections 5.3.2, 5.3.3 and 5.3.4, resolves every filter key and facet field through
// the effective context of section 5.3.1, and turns an index error into the error
// answer of appendix B.
//
// Every search result leaves the handler with a score and a source, which the OpenAPI
// schema requires of a result: the handler fills the source from Options and, for an
// index that computes no relevance, the score with zero.
func Handler(idx Index, opt Options) http.Handler {
	s := &server{index: idx, opt: opt}
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+RouteSearch, s.search)
	mux.HandleFunc("POST "+RouteExplore, s.explore)
	mux.HandleFunc("GET "+RouteAgents, s.list)
	mux.HandleFunc(RouteSearch, methodNotAllowed)
	mux.HandleFunc(RouteExplore, methodNotAllowed)
	mux.HandleFunc(RouteAgents, methodNotAllowed)
	mux.HandleFunc("/", routeNotFound)
	return mux
}

type server struct {
	index Index
	opt   Options
}

type exploreBody struct {
	Query      ard.Query         `json:"query"`
	ResultType *ard.ExploreShape `json:"resultType"`
}

func (s *server) search(w http.ResponseWriter, r *http.Request) {
	var req ard.SearchRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if req.Query.Text == "" {
		writeError(w, badRequest("section 5.3.2 requires %q on a search", "query.text"))
		return
	}
	mode, err := federationMode(req.Federation)
	if err != nil {
		writeError(w, err)
		return
	}
	size, err := boundPageSize(req.PageSize, ard.DefaultSearchPageSize, ard.MaxSearchPageSize)
	if err != nil {
		writeError(w, err)
		return
	}
	resolved, _, err := resolveQuery(req.Query)
	if err != nil {
		writeError(w, err)
		return
	}
	page, err := s.index.Search(r.Context(), SearchQuery{Query: resolved, PageSize: size, PageToken: req.PageToken})
	if err != nil {
		writeError(w, err)
		return
	}
	answer := ard.SearchResponse{Results: s.complete(page.Results), PageToken: page.PageToken}
	s.federate(r.Context(), mode, req, size, &answer)
	writeJSON(w, http.StatusOK, answer)
}

func (s *server) explore(w http.ResponseWriter, r *http.Request) {
	var body exploreBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if body.ResultType == nil {
		writeError(w, badRequest("section 5.3.3 requires %q on an explore", "resultType"))
		return
	}
	if len(body.ResultType.Facets) == 0 {
		writeError(w, badRequest("%q names the only result shape that section 5.3.3 defines, and it is empty", "resultType.facets"))
		return
	}
	resolved, resolver, err := resolveQuery(body.Query)
	if err != nil {
		writeError(w, err)
		return
	}
	facets, err := resolveFacets(resolver, body.ResultType.Facets)
	if err != nil {
		writeError(w, err)
		return
	}
	computed, err := s.index.Explore(r.Context(), ExploreQuery{Query: resolved, Facets: facets})
	if err != nil {
		writeError(w, err)
		return
	}
	if computed == nil {
		computed = map[string]ard.FacetResult{}
	}
	writeJSON(w, http.StatusOK, ard.ExploreResponse{ResultType: ResultTypeFacets, Facets: computed})
}

func (s *server) list(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	asked, err := queryInt(values.Get("pageSize"))
	if err != nil {
		writeError(w, err)
		return
	}
	size, err := boundPageSize(asked, ard.DefaultListPageSize, ard.MaxListPageSize)
	if err != nil {
		writeError(w, err)
		return
	}
	page, err := s.index.List(r.Context(), ListQuery{
		Filter:    values.Get("filter"),
		OrderBy:   values.Get("orderBy"),
		PageSize:  size,
		PageToken: values.Get("pageToken"),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	items := page.Items
	if items == nil {
		items = []ard.Entry{}
	}
	total := page.Total
	writeJSON(w, http.StatusOK, ard.ListResponse{Items: items, Total: &total, PageToken: page.PageToken})
}

func (s *server) federate(ctx context.Context, mode ard.FederationMode, req ard.SearchRequest, size int, answer *ard.SearchResponse) {
	if s.opt.Federator == nil {
		return
	}
	switch mode {
	case ard.FederationReferrals:
		answer.Referrals = s.opt.Federator.Referrals()
	case ard.FederationAuto:
		if req.PageToken != "" {
			return
		}
		merged := MergeResults(answer.Results, s.opt.Federator.Fanout(ctx, req))
		if len(merged) > size {
			merged = merged[:size]
		}
		answer.Results = merged
	}
}

func (s *server) complete(results []ard.Result) []ard.Result {
	completed := make([]ard.Result, 0, len(results))
	for _, result := range results {
		if result.Source == "" {
			result.Source = s.opt.Source
		}
		if result.Score == nil {
			none := 0
			result.Score = &none
		}
		completed = append(completed, result)
	}
	return completed
}

func resolveQuery(q ard.Query) (ResolvedQuery, *ard.TermResolver, error) {
	resolver, err := ard.NewTermResolver(q.Context)
	if err != nil {
		return ResolvedQuery{}, nil, badRequest("the %q of the query is not usable: %v", "@context", err)
	}
	resolved := ResolvedQuery{Text: q.Text}
	for _, key := range slices.Sorted(maps.Keys(q.Filter)) {
		path, err := resolver.ResolvePath(key)
		if err != nil {
			return ResolvedQuery{}, nil, badRequest("%v", err)
		}
		resolved.Filter = append(resolved.Filter, Constraint{Path: path, Values: q.Filter[key]})
	}
	return resolved, resolver, nil
}

func resolveFacets(resolver *ard.TermResolver, asked []ard.FacetRequest) ([]Facet, error) {
	facets := make([]Facet, 0, len(asked))
	for _, request := range asked {
		if request.Field == "" {
			return nil, badRequest("section 5.3.3 requires %q on every facet", "field")
		}
		path, err := resolver.ResolvePath(request.Field)
		if err != nil {
			return nil, badRequest("%v", err)
		}
		limit := request.Limit
		if limit <= 0 {
			limit = ard.DefaultFacetLimit
		}
		minCount := request.MinCount
		if minCount <= 0 {
			minCount = ard.DefaultFacetMinCount
		}
		facets = append(facets, Facet{Path: path, Limit: limit, MinCount: minCount})
	}
	return facets, nil
}

func federationMode(mode ard.FederationMode) (ard.FederationMode, error) {
	switch mode {
	case "":
		return ard.FederationAuto, nil
	case ard.FederationAuto, ard.FederationReferrals, ard.FederationNone:
		return mode, nil
	}
	return "", badRequest("federation %q is none of auto, referrals and none", mode)
}

func boundPageSize(asked, byDefault, most int) (int, error) {
	switch {
	case asked < 0:
		return 0, badRequest("pageSize %d is negative", asked)
	case asked == 0:
		return byDefault, nil
	case asked > most:
		return most, nil
	}
	return asked, nil
}

func queryInt(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, badRequest("pageSize %q is not a whole number", raw)
	}
	return value, nil
}

func decodeBody(r *http.Request, into any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBytes))
	if err != nil {
		return badRequest("the request body could not be read: %v", err)
	}
	if len(body) == 0 {
		return badRequest("the request carries no body")
	}
	if err := json.Unmarshal(body, into); err != nil {
		return badRequest("the request body is not a valid request object: %v", err)
	}
	return nil
}

func badRequest(format string, args ...any) *ard.APIError {
	return ard.NewAPIError(http.StatusBadRequest, ard.CodeInvalidArgument, fmt.Sprintf(format, args...))
}

func methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	writeError(w, ard.NewAPIError(http.StatusMethodNotAllowed, ard.CodeInvalidArgument,
		fmt.Sprintf("the route %s takes no %s", r.URL.Path, r.Method)))
}

func routeNotFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, ard.NewAPIError(http.StatusNotFound, ard.CodeNotFound,
		fmt.Sprintf("the registry serves no route %s %s", r.Method, r.URL.Path)))
}

func writeError(w http.ResponseWriter, err error) {
	answer := errorAnswer(err)
	writeJSON(w, answer.HTTPStatus, answer.Body())
}

func errorAnswer(err error) *ard.APIError {
	var answer *ard.APIError
	if errors.As(err, &answer) {
		return answer
	}
	switch {
	case errors.Is(err, ard.ErrNotImplemented):
		return ard.NewAPIError(http.StatusNotImplemented, ard.CodeNotImplemented, err.Error())
	case errors.Is(err, ErrInvalidArgument):
		return ard.NewAPIError(http.StatusBadRequest, ard.CodeInvalidArgument, err.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return ard.NewAPIError(http.StatusInternalServerError, ard.CodeInternalError, "the registry gave up on the request")
	}
	return ard.NewAPIError(http.StatusInternalServerError, ard.CodeInternalError, "the registry could not answer the request")
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	encoded, err := json.Marshal(body)
	if err != nil {
		encoded, status = []byte(`{"errorCode":"INTERNAL_ERROR","message":"the answer could not be encoded"}`), http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(encoded)
}
