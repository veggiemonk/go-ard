package registry_test

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"

	ard "github.com/veggiemonk/go-ard"
	"github.com/veggiemonk/go-ard/registry"
)

func explore(t *testing.T, client *registry.Client, req ard.ExploreRequest) ard.ExploreResponse {
	t.Helper()
	answer, err := client.Explore(context.Background(), req)
	if err != nil {
		t.Fatalf("explore: %v", err)
	}
	return answer
}

func facetRequest(fields ...ard.FacetRequest) ard.ExploreRequest {
	return ard.ExploreRequest{ResultType: ard.ExploreShape{Facets: fields}}
}

func TestExploreFacetsTheWholeRegistryWhenTheQueryIsAbsent(t *testing.T) {
	client := newFixtureClient(t)

	answer := explore(t, client, facetRequest(
		ard.FacetRequest{Field: ard.TermType},
		ard.FacetRequest{Field: ard.TermPublisher},
	))

	if answer.ResultType != registry.ResultTypeFacets {
		t.Errorf("resultType %q, want %q", answer.ResultType, registry.ResultTypeFacets)
	}
	wantCounts(t, "type", facetValues(answer.Facets[ard.TermType]), map[string]int{
		ard.MediaTypeA2AAgentCard:  6,
		ard.MediaTypeMCPServerCard: 2,
		ard.MediaTypeAIRegistry:    1,
	})
	wantCounts(t, "publisher", facetValues(answer.Facets[ard.TermPublisher]), map[string]int{
		"acme.com":       3,
		"example.com":    3,
		"travel.example": 3,
	})
}

func TestExploreSortsTheBucketsByCount(t *testing.T) {
	client := newFixtureClient(t)

	answer := explore(t, client, facetRequest(ard.FacetRequest{Field: ard.TermType}))

	buckets := answer.Facets[ard.TermType].Buckets
	for i := 1; i < len(buckets); i++ {
		if *buckets[i-1].Count < *buckets[i].Count {
			t.Fatalf("bucket %q at %d sorts above %q at %d", buckets[i-1].Value, *buckets[i-1].Count, buckets[i].Value, *buckets[i].Count)
		}
	}
}

func TestExploreReportsTheOtherCountBeyondTheLimit(t *testing.T) {
	client := newFixtureClient(t)

	answer := explore(t, client, facetRequest(ard.FacetRequest{Field: ard.TermPublisher, Limit: 2}))

	result := answer.Facets[ard.TermPublisher]
	if len(result.Buckets) != 2 {
		t.Fatalf("the facet holds %d buckets, want the limit of 2", len(result.Buckets))
	}
	if result.OtherCount == nil {
		t.Fatal("the facet reports no otherCount")
	}
	if *result.OtherCount != 3 {
		t.Errorf("otherCount %d, want the 3 entries of the third publisher", *result.OtherCount)
	}
}

func TestExploreSuppressesTheBucketsBelowTheMinimumCount(t *testing.T) {
	client := newFixtureClient(t)

	answer := explore(t, client, facetRequest(ard.FacetRequest{Field: ard.TermType, MinCount: 3}))

	wantCounts(t, "type", facetValues(answer.Facets[ard.TermType]), map[string]int{
		ard.MediaTypeA2AAgentCard: 6,
	})
}

func TestExploreNarrowsTheFacetsWithTheSameCutoffAsSearch(t *testing.T) {
	client := newFixtureClient(t)
	req := facetRequest(ard.FacetRequest{Field: ard.TermType})
	req.Query = ard.Query{Text: "weather forecast"}

	answer := explore(t, client, req)
	found := search(t, client, searchRequest("weather forecast"))

	wantCounts(t, "type", facetValues(answer.Facets[ard.TermType]), map[string]int{
		ard.MediaTypeMCPServerCard: 1,
	})
	if len(found.Results) != 1 {
		t.Errorf("the same query gave %d search results and one facet bucket of 1", len(found.Results))
	}
}

func TestExploreNarrowsTheFacetsWithAFilter(t *testing.T) {
	client := newFixtureClient(t)
	req := facetRequest(ard.FacetRequest{Field: ard.TermPublisher})
	req.Query = ard.Query{Filter: map[string][]string{"trustManifest.attestations.type": {"SOC2-Type2"}}}

	answer := explore(t, client, req)

	wantCounts(t, "publisher", facetValues(answer.Facets[ard.TermPublisher]), map[string]int{
		"acme.com":       2,
		"travel.example": 1,
	})
}

func TestExploreAnswers501WhenTheIndexDoesNotImplementIt(t *testing.T) {
	client := newClient(t, searchOnlyIndex{inner: fixtureIndex(t)}, registry.Options{})

	_, err := client.Explore(context.Background(), facetRequest(ard.FacetRequest{Field: ard.TermType}))

	if !errors.Is(err, ard.ErrNotImplemented) {
		t.Fatalf("error %v, want one that satisfies errors.Is(err, ard.ErrNotImplemented)", err)
	}
	var answer *ard.APIError
	if !errors.As(err, &answer) {
		t.Fatalf("error %v is no *ard.APIError", err)
	}
	if answer.HTTPStatus != 501 || answer.Code != ard.CodeNotImplemented {
		t.Errorf("status %d and code %q, want 501 and %q", answer.HTTPStatus, answer.Code, ard.CodeNotImplemented)
	}
}

func wantCounts(t *testing.T, field string, got, want map[string]int) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("facet %q holds the buckets %v, want %v", field, sortedKeys(got), sortedKeys(want))
		return
	}
	for value, count := range want {
		if got[value] != count {
			t.Errorf("facet %q bucket %q counts %d, want %d", field, value, got[value], count)
		}
	}
}

func sortedKeys(counts map[string]int) []string {
	return slices.Sorted(maps.Keys(counts))
}
