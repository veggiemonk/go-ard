package memindex

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"

	ard "github.com/veggiemonk/go-ard"
	"github.com/veggiemonk/go-ard/internal/jsonx"
	"github.com/veggiemonk/go-ard/registry"
)

var (
	baseResolver = newBaseResolver()
	publisherIRI = termIRI(ard.TermPublisher)
	typeIRI      = termIRI(ard.TermType)
	corePaths    = newCorePaths()
	createdPaths = []ard.TermPath{
		{Key: "createdAt", IRI: termIRI("createdAt")},
		{Key: "metadata.createdAt", IRI: termIRI(ard.TermMetadata), Rest: []string{"createdAt"}},
	}
)

// Index is an in-memory index of ARD entries. It is safe for concurrent use.
type Index struct {
	mu      sync.RWMutex
	records []record
	known   map[string]bool
	cutoff  int
}

type record struct {
	entry     ard.Entry
	publisher string
	raw       map[string]json.RawMessage
	tokens    map[string]int
}

type scored struct {
	record record
	score  int
}

// New builds an index over a set of entries.
func New(entries []ard.Entry) *Index {
	x := &Index{cutoff: DefaultCutoff}
	x.SetEntries(entries)
	return x
}

// SetEntries replaces every entry of the index.
func (x *Index) SetEntries(entries []ard.Entry) {
	records := make([]record, 0, len(entries))
	for _, entry := range entries {
		records = append(records, newRecord(entry))
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	x.records = records
	x.known = knownPaths(records)
}

// SetCutoff sets the relevance cutoff of section 5.3.3. One cutoff governs the matched
// set of Search and of Explore alike.
func (x *Index) SetCutoff(cutoff int) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.cutoff = cutoff
}

// Len reports how many entries the index holds.
func (x *Index) Len() int {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return len(x.records)
}

// Search ranks the matched entries by relevance and gives back one page of them. See
// section 5.3.2.
func (x *Index) Search(ctx context.Context, q registry.SearchQuery) (registry.Page, error) {
	x.mu.RLock()
	defer x.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return registry.Page{}, err
	}
	matched, err := x.matched(q.Query)
	if err != nil {
		return registry.Page{}, err
	}
	slices.SortStableFunc(matched, byRelevance)
	print := searchPrint(q.Query)
	offset, err := decodeCursor(q.PageToken, print)
	if err != nil {
		return registry.Page{}, err
	}
	size := q.PageSize
	if size <= 0 {
		size = ard.DefaultSearchPageSize
	}
	from, to := window(len(matched), offset, size)
	page := registry.Page{Results: make([]ard.Result, 0, to-from)}
	for _, m := range matched[from:to] {
		score := m.score
		page.Results = append(page.Results, ard.Result{Entry: m.record.entry, Score: &score})
	}
	if to < len(matched) {
		page.PageToken = encodeCursor(print, to)
	}
	return page, nil
}

// Explore aggregates the matched set. See section 5.3.3.
func (x *Index) Explore(ctx context.Context, q registry.ExploreQuery) (map[string]ard.FacetResult, error) {
	x.mu.RLock()
	defer x.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, facet := range q.Facets {
		if err := x.supports(facet.Path); err != nil {
			return nil, err
		}
	}
	matched, err := x.matched(q.Query)
	if err != nil {
		return nil, err
	}
	records := make([]record, 0, len(matched))
	for _, m := range matched {
		records = append(records, m.record)
	}
	facets := make(map[string]ard.FacetResult, len(q.Facets))
	for _, facet := range q.Facets {
		result, err := aggregate(records, facet)
		if err != nil {
			return nil, err
		}
		facets[facet.Path.Key] = result
	}
	return facets, nil
}

// List gives one page of a deterministic listing. See section 5.3.4 and appendix A.
func (x *Index) List(ctx context.Context, q registry.ListQuery) (registry.ListPage, error) {
	x.mu.RLock()
	defer x.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return registry.ListPage{}, err
	}
	filter, err := parseListFilter(q.Filter)
	if err != nil {
		return registry.ListPage{}, err
	}
	order, err := parseOrderBy(q.OrderBy)
	if err != nil {
		return registry.ListPage{}, err
	}
	matched := make([]record, 0, len(x.records))
	for _, r := range x.records {
		if filter.matches(r) {
			matched = append(matched, r)
		}
	}
	slices.SortStableFunc(matched, ordered(order))
	print := listPrint(q)
	offset, err := decodeCursor(q.PageToken, print)
	if err != nil {
		return registry.ListPage{}, err
	}
	size := q.PageSize
	if size <= 0 {
		size = ard.DefaultListPageSize
	}
	from, to := window(len(matched), offset, size)
	page := registry.ListPage{Items: make([]ard.Entry, 0, to-from), Total: len(matched)}
	for _, r := range matched[from:to] {
		page.Items = append(page.Items, r.entry)
	}
	if to < len(matched) {
		page.PageToken = encodeCursor(print, to)
	}
	return page, nil
}

func (x *Index) matched(q registry.ResolvedQuery) ([]scored, error) {
	for _, constraint := range q.Filter {
		if err := x.supports(constraint.Path); err != nil {
			return nil, err
		}
	}
	tokens := queryTokens(q.Text)
	matched := make([]scored, 0, len(x.records))
	for _, r := range x.records {
		ok, err := r.matches(q.Filter)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		score := 0
		if len(tokens) > 0 {
			score = relevance(r.tokens, tokens)
			if score < x.cutoff {
				continue
			}
		}
		matched = append(matched, scored{record: r, score: score})
	}
	return matched, nil
}

func (x *Index) supports(path ard.TermPath) error {
	if x.known[path.IRI] {
		return nil
	}
	return fmt.Errorf("%w: this registry does not index the term path %q, which resolves to %q",
		registry.ErrUnsupportedFilter, path.Key, path.IRI)
}

func (r record) matches(filter []registry.Constraint) (bool, error) {
	for _, constraint := range filter {
		values, err := r.values(constraint.Path)
		if err != nil {
			return false, err
		}
		wanted := constraint.Values
		if constraint.Path.IRI == typeIRI && len(constraint.Path.Rest) == 0 {
			values, wanted = canonicalMediaTypes(values), canonicalMediaTypes(wanted)
		}
		if !sharesValue(values, wanted) {
			return false, nil
		}
	}
	return true, nil
}

func (r record) values(path ard.TermPath) ([]string, error) {
	if path.IRI == publisherIRI && len(path.Rest) == 0 {
		if r.publisher == "" {
			return nil, nil
		}
		return []string{r.publisher}, nil
	}
	raw, held := r.raw[path.IRI]
	if !held {
		return nil, nil
	}
	found, err := jsonx.Lookup(raw, path.Rest)
	if err != nil {
		return nil, fmt.Errorf("memindex: entry %q: %w", r.entry.Identifier, err)
	}
	values := make([]string, 0, len(found))
	for _, value := range found {
		if scalar, ok := scalarString(value); ok {
			values = append(values, scalar)
		}
	}
	return values, nil
}

func newRecord(e ard.Entry) record {
	r := record{entry: e, raw: map[string]json.RawMessage{}, tokens: entryTokens(e)}
	if identifier, err := e.URN(); err == nil {
		r.publisher = identifier.Publisher
	}
	encoded, err := json.Marshal(e)
	if err != nil {
		return r
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &members); err != nil {
		return r
	}
	resolver := entryResolver(e)
	for name, value := range members {
		if strings.HasPrefix(name, "@") {
			continue
		}
		if iri, bound := resolver.Resolve(name); bound {
			r.raw[iri] = value
		}
	}
	return r
}

func entryResolver(e ard.Entry) *ard.TermResolver {
	if e.Context.IsZero() {
		return baseResolver
	}
	resolver, err := ard.NewTermResolver(e.Context)
	if err != nil {
		return baseResolver
	}
	return resolver
}

func knownPaths(records []record) map[string]bool {
	known := make(map[string]bool, len(corePaths)+len(records))
	for iri := range corePaths {
		known[iri] = true
	}
	for _, r := range records {
		for iri := range r.raw {
			known[iri] = true
		}
	}
	return known
}

func newBaseResolver() *ard.TermResolver {
	resolver, err := ard.NewTermResolver()
	if err != nil {
		panic("memindex: the ARD base context does not resolve: " + err.Error())
	}
	return resolver
}

func newCorePaths() map[string]bool {
	known := map[string]bool{publisherIRI: true}
	for _, term := range ard.EntryTerms {
		if strings.HasPrefix(term, "@") {
			continue
		}
		if iri := termIRI(term); iri != "" {
			known[iri] = true
		}
	}
	return known
}

func termIRI(term string) string {
	iri, _ := baseResolver.Resolve(term)
	return iri
}

func byRelevance(a, b scored) int {
	if difference := cmp.Compare(b.score, a.score); difference != 0 {
		return difference
	}
	return cmp.Compare(a.record.entry.Identifier, b.record.entry.Identifier)
}

func window(length, offset, size int) (int, int) {
	from := min(offset, length)
	return from, min(from+size, length)
}

// canonicalMediaTypes folds every spelling of a media type onto one, so that a filter
// for "application/ai-skill" finds an entry of type "application/ai-skill+md". See
// ard.CanonicalMediaType.
func canonicalMediaTypes(values []string) []string {
	folded := make([]string, len(values))
	for i, value := range values {
		folded[i] = ard.CanonicalMediaType(value)
	}
	return folded
}

func sharesValue(held, wanted []string) bool {
	for _, value := range held {
		if slices.Contains(wanted, value) {
			return true
		}
	}
	return false
}

func scalarString(raw json.RawMessage) (string, bool) {
	trimmed := strings.TrimSpace(string(raw))
	switch {
	case trimmed == "", trimmed == "null":
		return "", false
	case trimmed[0] == '"':
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", false
		}
		return value, true
	case trimmed[0] == '{', trimmed[0] == '[':
		return "", false
	}
	return trimmed, true
}
