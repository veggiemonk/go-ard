package memindex

import (
	"context"
	"errors"
	"testing"

	ard "github.com/veggiemonk/go-ard"
	"github.com/veggiemonk/go-ard/registry"
)

func sample() []ard.Entry {
	return []ard.Entry{
		{
			Identifier:            "urn:air:acme.com:server:weather",
			DisplayName:           "Weather Data Node",
			Type:                  ard.MediaTypeMCPServerCard,
			URL:                   "https://api.acme.com/mcp/weather.json",
			Description:           "Enterprise weather MCP server for live telemetry.",
			Capabilities:          []string{"WeatherTool", "ForecastTool"},
			Tags:                  []string{"weather"},
			UpdatedAt:             "2026-02-01T09:00:00Z",
			RepresentativeQueries: []string{"what is the current wind speed in Chicago"},
		},
		{
			Identifier:  "urn:air:acme.com:tool:unit-converter",
			DisplayName: "Unit Converter",
			Type:        ard.MediaTypeMCPServerCard,
			URL:         "https://api.acme.com/mcp/unit-converter.json",
			Description: "Converts between units of length, mass and volume.",
			UpdatedAt:   "2025-12-01T09:00:00Z",
		},
	}
}

func resolved(t *testing.T, text string, filter map[string][]string) registry.ResolvedQuery {
	t.Helper()
	resolver, err := ard.NewTermResolver()
	if err != nil {
		t.Fatalf("build the resolver: %v", err)
	}
	query := registry.ResolvedQuery{Text: text}
	for key, values := range filter {
		path, err := resolver.ResolvePath(key)
		if err != nil {
			t.Fatalf("resolve %q: %v", key, err)
		}
		query.Filter = append(query.Filter, registry.Constraint{Path: path, Values: values})
	}
	return query
}

func TestTokenizeSplitsTheCamelCase(t *testing.T) {
	cases := map[string][]string{
		"ForecastTool":                 {"forecast", "tool"},
		"Weather Data Node":            {"weather", "data", "node"},
		"application/a2a-agent-card":   {"application", "a2a", "agent", "card"},
		"CI/CD deployment":             {"ci", "cd", "deployment"},
		"  spaces   and\tpunctuation!": {"spaces", "and", "punctuation"},
		"":                             nil,
	}

	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			got := tokenize(input)
			if len(got) != len(want) {
				t.Fatalf("tokenize(%q) = %v, want %v", input, got, want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("token %d of %q is %q, want %q", i, input, got[i], want[i])
				}
			}
		})
	}
}

func TestRelevanceStaysWithinZeroAndOneHundred(t *testing.T) {
	tokens := entryTokens(sample()[0])
	cases := []string{"weather", "weather forecast", "weather forecast telemetry chicago", "quantum accounting"}

	for _, text := range cases {
		t.Run(text, func(t *testing.T) {
			score := relevance(tokens, queryTokens(text))
			if score < 0 || score > 100 {
				t.Errorf("relevance of %q is %d, outside the 0 to 100 of section 5.3.2", text, score)
			}
		})
	}
}

func TestRelevanceRanksTheMatchingEntryAbove(t *testing.T) {
	entries := sample()
	matching := relevance(entryTokens(entries[0]), queryTokens("weather forecast"))
	unrelated := relevance(entryTokens(entries[1]), queryTokens("weather forecast"))

	if matching <= unrelated {
		t.Errorf("the weather entry scores %d and the unrelated one %d", matching, unrelated)
	}
	if unrelated != 0 {
		t.Errorf("the unrelated entry scores %d, want 0", unrelated)
	}
}

func TestCutoffDropsTheEntriesBelowIt(t *testing.T) {
	index := New(sample())

	loose, err := index.Search(context.Background(), registry.SearchQuery{Query: resolved(t, "weather forecast", nil), PageSize: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	index.SetCutoff(101)
	strict, err := index.Search(context.Background(), registry.SearchQuery{Query: resolved(t, "weather forecast", nil), PageSize: 10})
	if err != nil {
		t.Fatalf("search with a strict cutoff: %v", err)
	}

	if len(loose.Results) != 1 {
		t.Errorf("the default cutoff gave %d results, want 1", len(loose.Results))
	}
	if len(strict.Results) != 0 {
		t.Errorf("a cutoff above every score gave %d results, want none", len(strict.Results))
	}
}

func TestSetEntriesReplacesTheContent(t *testing.T) {
	index := New(sample())
	if index.Len() != 2 {
		t.Fatalf("the index holds %d entries, want 2", index.Len())
	}

	index.SetEntries(sample()[:1])

	if index.Len() != 1 {
		t.Errorf("the index holds %d entries after the replacement, want 1", index.Len())
	}
}

func TestSearchReportsAnUnsupportedFilterKey(t *testing.T) {
	index := New(sample())
	query := registry.ResolvedQuery{
		Text:   "weather",
		Filter: []registry.Constraint{{Path: ard.TermPath{Key: "okf:taxonomy", IRI: "https://elsewhere.example/ns#taxonomy"}, Values: []string{"iata"}}},
	}

	_, err := index.Search(context.Background(), registry.SearchQuery{Query: query, PageSize: 10})

	if !errors.Is(err, registry.ErrUnsupportedFilter) {
		t.Fatalf("error %v, want one that satisfies errors.Is(err, registry.ErrUnsupportedFilter)", err)
	}
	if !errors.Is(err, registry.ErrInvalidArgument) {
		t.Error("an unsupported filter key is an invalid argument of appendix B")
	}
}

func TestSearchSupportsACoreKeyThatMatchesNothing(t *testing.T) {
	index := New(sample())
	query := resolved(t, "weather", map[string][]string{ard.TermTags: {"nothing-carries-this"}})

	page, err := index.Search(context.Background(), registry.SearchQuery{Query: query, PageSize: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(page.Results) != 0 {
		t.Errorf("the search gave %d results, want none", len(page.Results))
	}
}

func TestCursorRoundTrip(t *testing.T) {
	print := searchPrint(resolved(t, "weather", nil))

	token := encodeCursor(print, 40)
	offset, err := decodeCursor(token, print)
	if err != nil {
		t.Fatalf("decode the cursor: %v", err)
	}

	if offset != 40 {
		t.Errorf("offset %d, want 40", offset)
	}
}

func TestCursorRejectsTheTokenOfAnotherQuery(t *testing.T) {
	token := encodeCursor(searchPrint(resolved(t, "weather", nil)), 2)

	_, err := decodeCursor(token, searchPrint(resolved(t, "converter", nil)))

	if !errors.Is(err, registry.ErrBadPageToken) {
		t.Fatalf("error %v, want one that satisfies errors.Is(err, registry.ErrBadPageToken)", err)
	}
}

func TestCursorRejectsRubbish(t *testing.T) {
	print := searchPrint(resolved(t, "weather", nil))
	cases := map[string]string{
		"not base64": "!!!!",
		"not json":   "YWJjZGVm",
	}

	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeCursor(token, print); !errors.Is(err, registry.ErrBadPageToken) {
				t.Errorf("error %v, want one that satisfies errors.Is(err, registry.ErrBadPageToken)", err)
			}
		})
	}
}

func TestCursorAcceptsAnEmptyToken(t *testing.T) {
	offset, err := decodeCursor("", searchPrint(resolved(t, "weather", nil)))
	if err != nil {
		t.Fatalf("decode an empty token: %v", err)
	}
	if offset != 0 {
		t.Errorf("offset %d, want 0", offset)
	}
}

func TestParseListFilterReadsTheFieldsOfAppendixA(t *testing.T) {
	filter, err := parseListFilter("displayName = 'Weather' AND type = 'a,b' AND publisherId = acme.com AND updatedAfter > '2026-01-01'")
	if err != nil {
		t.Fatalf("parse the filter: %v", err)
	}

	if len(filter.displayName) != 1 || filter.displayName[0] != "weather" {
		t.Errorf("displayName %v, want the lowercased weather", filter.displayName)
	}
	if len(filter.types) != 2 {
		t.Errorf("type %v, want two values", filter.types)
	}
	if len(filter.publishers) != 1 || filter.publishers[0] != "acme.com" {
		t.Errorf("publisherId %v, want acme.com", filter.publishers)
	}
	if filter.updatedAfter.IsZero() {
		t.Error("updatedAfter stayed zero")
	}
}

func TestParseOrderByReadsTheDirections(t *testing.T) {
	order, err := parseOrderBy("displayName, created_at DESC")
	if err != nil {
		t.Fatalf("parse the order: %v", err)
	}

	if len(order) != 2 {
		t.Fatalf("the order holds %d terms, want 2", len(order))
	}
	if order[0].field != "displayname" || order[0].descending {
		t.Errorf("the first term is %+v, want displayname ascending", order[0])
	}
	if order[1].field != "createdat" || !order[1].descending {
		t.Errorf("the second term is %+v, want createdat descending", order[1])
	}
}

func TestExploreOverAnEmptyIndexGivesEmptyFacets(t *testing.T) {
	index := New(nil)
	resolver, err := ard.NewTermResolver()
	if err != nil {
		t.Fatalf("build the resolver: %v", err)
	}
	path, err := resolver.ResolvePath(ard.TermType)
	if err != nil {
		t.Fatalf("resolve the type term: %v", err)
	}

	facets, err := index.Explore(context.Background(), registry.ExploreQuery{Facets: []registry.Facet{{Path: path, Limit: 10, MinCount: 1}}})
	if err != nil {
		t.Fatalf("explore: %v", err)
	}

	result, held := facets[ard.TermType]
	if !held {
		t.Fatal("the answer holds no type facet")
	}
	if len(result.Buckets) != 0 {
		t.Errorf("the facet holds %d buckets over an empty index", len(result.Buckets))
	}
}

func TestSortKeyReadsEveryFieldOfAppendixA(t *testing.T) {
	r := newRecord(ard.Entry{
		Identifier:  "urn:air:acme.com:server:weather",
		DisplayName: "Weather Data Node",
		Type:        ard.MediaTypeMCPServerCard,
		URL:         "https://api.acme.com/mcp/weather.json",
		Version:     "2.1.0",
		UpdatedAt:   "2026-02-01T09:00:00Z",
		Metadata:    map[string]any{"createdAt": "2025-06-01T00:00:00Z"},
	})

	cases := map[string]string{
		"displayname": "weather data node",
		"identifier":  "urn:air:acme.com:server:weather",
		"type":        ard.MediaTypeMCPServerCard,
		"version":     "2.1.0",
		"publisherid": "acme.com",
		"updatedat":   "2026-02-01T09:00:00Z",
		"createdat":   "2025-06-01T00:00:00Z",
		"unknown":     "",
	}

	for field, want := range cases {
		t.Run(field, func(t *testing.T) {
			if got := r.sortKey(field); got != want {
				t.Errorf("sortKey(%q) = %q, want %q", field, got, want)
			}
		})
	}
}

func TestSortKeyIsEmptyWhenTheMomentIsUnknown(t *testing.T) {
	r := newRecord(ard.Entry{Identifier: "urn:air:acme.com:server:weather", UpdatedAt: "the first of March"})

	if got := r.sortKey("updatedat"); got != "" {
		t.Errorf("sortKey(updatedat) = %q over an unreadable timestamp, want the empty key", got)
	}
	if got := r.sortKey("createdat"); got != "" {
		t.Errorf("sortKey(createdat) = %q over an entry without one, want the empty key", got)
	}
}

func TestOrderFieldReadsTheAliases(t *testing.T) {
	cases := map[string]string{
		"displayName": "displayname",
		"name":        "displayname",
		"identifier":  "identifier",
		"id":          "identifier",
		"type":        "type",
		"version":     "version",
		"publisherId": "publisherid",
		"publisher":   "publisherid",
		"updated_at":  "updatedat",
		"CREATEDAT":   "createdat",
	}

	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			got, ok := orderField(name)
			if !ok {
				t.Fatalf("orderField(%q) names no field", name)
			}
			if got != want {
				t.Errorf("orderField(%q) = %q, want %q", name, got, want)
			}
		})
	}

	if _, ok := orderField("score"); ok {
		t.Error("orderField(score) names a field, and no registry sorts a listing on it")
	}
}

func TestParseOrderByRefusesWhatItCannotSort(t *testing.T) {
	cases := map[string]string{
		"an unknown field":     "score DESC",
		"an unknown direction": "displayName SIDEWAYS",
		"three words":          "displayName DESC NOW",
	}

	for name, expression := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseOrderBy(expression); !errors.Is(err, registry.ErrInvalidArgument) {
				t.Errorf("parseOrderBy(%q) gave %v, want ErrInvalidArgument", expression, err)
			}
		})
	}
}

func TestScalarStringReadsOnlyAScalar(t *testing.T) {
	cases := map[string]struct {
		raw   string
		want  string
		known bool
	}{
		"a string":   {raw: `"weather"`, want: "weather", known: true},
		"a number":   {raw: `12`, want: "12", known: true},
		"a boolean":  {raw: `true`, want: "true", known: true},
		"null":       {raw: `null`},
		"an object":  {raw: `{"a":1}`},
		"an array":   {raw: `["a"]`},
		"nothing":    {raw: ``},
		"bad string": {raw: `"unterminated`},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got, known := scalarString([]byte(c.raw))
			if known != c.known {
				t.Fatalf("scalarString(%s) is known = %v, want %v", c.raw, known, c.known)
			}
			if got != c.want {
				t.Errorf("scalarString(%s) = %q, want %q", c.raw, got, c.want)
			}
		})
	}
}

func TestDistinctKeepsTheFirstOfEachValue(t *testing.T) {
	cases := map[string][]string{
		"nothing":       nil,
		"one value":     {"weather"},
		"two the same":  {"weather", "weather"},
		"three of them": {"weather", "maps", "weather"},
	}
	want := map[string]int{"nothing": 0, "one value": 1, "two the same": 1, "three of them": 2}

	for name, values := range cases {
		t.Run(name, func(t *testing.T) {
			if got := len(distinct(values)); got != want[name] {
				t.Errorf("distinct(%v) holds %d values, want %d", values, got, want[name])
			}
		})
	}
}

func TestAFacetCountsARepeatedValueOnce(t *testing.T) {
	index := New([]ard.Entry{{
		Identifier:  "urn:air:acme.com:server:weather",
		DisplayName: "Weather Data Node",
		Type:        ard.MediaTypeMCPServerCard,
		URL:         "https://api.acme.com/mcp/weather.json",
		Tags:        []string{"weather", "weather", "maps"},
	}})
	resolver, err := ard.NewTermResolver()
	if err != nil {
		t.Fatalf("build the resolver: %v", err)
	}
	path, err := resolver.ResolvePath(ard.TermTags)
	if err != nil {
		t.Fatalf("resolve the tags term: %v", err)
	}

	facets, err := index.Explore(context.Background(), registry.ExploreQuery{Facets: []registry.Facet{{Path: path, Limit: 10, MinCount: 1}}})
	if err != nil {
		t.Fatalf("explore: %v", err)
	}

	buckets := facets[ard.TermTags].Buckets
	if len(buckets) != 2 {
		t.Fatalf("the tags facet holds %d buckets, want 2", len(buckets))
	}
	for _, bucket := range buckets {
		if bucket.Count == nil || *bucket.Count != 1 {
			t.Errorf("bucket %q counts %v, want 1 for one entry", bucket.Value, bucket.Count)
		}
	}
}
