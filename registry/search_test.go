package registry_test

import (
	"context"
	"net/http/httptest"
	"testing"

	ard "github.com/veggiemonk/go-ard"
	"github.com/veggiemonk/go-ard/registry"
)

func newClient(t *testing.T, idx registry.Index, opt registry.Options) *registry.Client {
	t.Helper()
	if opt.Source == "" {
		opt.Source = localSource
	}
	server := httptest.NewServer(registry.Handler(idx, opt))
	t.Cleanup(server.Close)
	return &registry.Client{BaseURL: server.URL, HTTPClient: server.Client()}
}

func newFixtureClient(t *testing.T) *registry.Client {
	t.Helper()
	return newClient(t, fixtureIndex(t), registry.Options{})
}

func TestSearchWithTextOnly(t *testing.T) {
	client := newFixtureClient(t)
	answer := search(t, client, searchRequest("weather forecast"))

	wantSet(t, identifiers(answer.Results), []string{"urn:air:acme.com:server:weather"})
	result := answer.Results[0]
	if result.Score == nil {
		t.Fatal("the result carries no score, which the OpenAPI schema requires of a search result")
	}
	if *result.Score < 1 || *result.Score > 100 {
		t.Errorf("score %d falls outside the 0 to 100 of section 5.3.2", *result.Score)
	}
	if result.Source != localSource {
		t.Errorf("source %q, want %q", result.Source, localSource)
	}
}

func TestSearchRanksTheMatchingEntryFirst(t *testing.T) {
	client := newFixtureClient(t)
	answer := search(t, client, searchRequest("corporate travel"))

	if len(answer.Results) < 2 {
		t.Fatalf("the search gave %d results, want at least 2", len(answer.Results))
	}
	for i := 1; i < len(answer.Results); i++ {
		if *answer.Results[i-1].Score < *answer.Results[i].Score {
			t.Fatalf("result %d scores %d, below result %d at %d", i-1, *answer.Results[i-1].Score, i, *answer.Results[i].Score)
		}
	}
	if got := answer.Results[0].Entry.Identifier; got == "urn:air:acme.com:tool:unit-converter" {
		t.Errorf("an unrelated entry ranks first: %q", got)
	}
}

func TestSearchComposesTheTextAndTheFilterWithAnd(t *testing.T) {
	client := newFixtureClient(t)
	req := searchRequest("travel")
	req.Query.Filter = map[string][]string{ard.TermType: {ard.MediaTypeMCPServerCard}}
	req.PageSize = 50

	answer := search(t, client, req)

	wantSet(t, identifiers(answer.Results), nil)
}

func TestSearchWithFilterKeepsTheMatchingType(t *testing.T) {
	client := newFixtureClient(t)
	req := searchRequest("travel booking")
	req.Query.Filter = map[string][]string{ard.TermType: {ard.MediaTypeA2AAgentCard}}
	req.PageSize = 50

	answer := search(t, client, req)

	wantSet(t, identifiers(answer.Results), []string{
		"urn:air:travel.example:agent:flights",
		"urn:air:travel.example:agent:hotels",
		"urn:air:travel.example:agent:cars",
		"urn:air:example.com:agent:trains",
		"urn:air:example.com:agent:visas",
	})
}

func TestSearchWithTwoValuesOfOneFilterKeyIsOr(t *testing.T) {
	client := newFixtureClient(t)
	req := searchRequest("travel")
	req.Query.Filter = map[string][]string{"tags": {"rail", "rental"}}
	req.PageSize = 50

	answer := search(t, client, req)

	wantSet(t, identifiers(answer.Results), []string{
		"urn:air:travel.example:agent:cars",
		"urn:air:example.com:agent:trains",
	})
}

func TestSearchWithTwoFilterKeysIsAnd(t *testing.T) {
	client := newFixtureClient(t)
	req := searchRequest("travel")
	req.Query.Filter = map[string][]string{
		"tags":            {"booking"},
		ard.TermPublisher: {"travel.example"},
		ard.TermType:      {ard.MediaTypeA2AAgentCard},
	}
	req.PageSize = 50

	answer := search(t, client, req)

	wantSet(t, identifiers(answer.Results), []string{
		"urn:air:travel.example:agent:flights",
		"urn:air:travel.example:agent:hotels",
	})
}

func TestSearchWithANamespacedFilterKeyUnderTwoPrefixes(t *testing.T) {
	client := newFixtureClient(t)
	want := []string{
		"urn:air:travel.example:agent:flights",
		"urn:air:travel.example:agent:hotels",
	}
	prefixes := map[string]string{"okf": "okf:taxonomy", "anything": "anything:taxonomy"}

	for prefix, key := range prefixes {
		t.Run(prefix, func(t *testing.T) {
			req := searchRequest("travel booking")
			req.Query.Context = queryContext(t, map[string]string{prefix: okfNamespace})
			req.Query.Filter = map[string][]string{key: {"iata"}}
			req.PageSize = 50

			answer := search(t, client, req)

			wantSet(t, identifiers(answer.Results), want)
		})
	}
}

func TestSearchWithAPathFilterIntoTheTrustManifest(t *testing.T) {
	client := newFixtureClient(t)
	cases := map[string][]string{
		"ISO27001": {"urn:air:acme.com:agent:assistant"},
		"SOC2-Type2": {
			"urn:air:acme.com:agent:assistant",
			"urn:air:acme.com:server:weather",
			"urn:air:travel.example:agent:flights",
		},
	}

	for attestation, want := range cases {
		t.Run(attestation, func(t *testing.T) {
			req := searchRequest("corporate travel weather")
			req.Query.Filter = map[string][]string{"trustManifest.attestations.type": {attestation}}
			req.PageSize = 50

			answer := search(t, client, req)

			wantSet(t, identifiers(answer.Results), want)
		})
	}
}

func TestSearchWithAPathFilterIntoTheMetadata(t *testing.T) {
	client := newFixtureClient(t)
	req := searchRequest("corporate assistant")
	req.Query.Filter = map[string][]string{"metadata.region": {"us-east-1"}}

	answer := search(t, client, req)

	wantSet(t, identifiers(answer.Results), []string{"urn:air:acme.com:agent:assistant"})
}

func TestSearchAllWalksEveryPage(t *testing.T) {
	client := newFixtureClient(t)
	req := searchRequest("corporate travel")
	req.PageSize = 2

	var walked []string
	for result, err := range client.SearchAll(context.Background(), req) {
		if err != nil {
			t.Fatalf("walk the pages: %v", err)
		}
		walked = append(walked, result.Entry.Identifier)
	}

	wantSet(t, walked, []string{
		"urn:air:acme.com:agent:assistant",
		"urn:air:travel.example:agent:flights",
		"urn:air:travel.example:agent:hotels",
		"urn:air:travel.example:agent:cars",
		"urn:air:example.com:agent:trains",
		"urn:air:example.com:agent:visas",
	})
}

func TestSearchPagesAreDisjointAndOrdered(t *testing.T) {
	client := newFixtureClient(t)
	req := searchRequest("corporate travel")
	req.PageSize = 2

	var pages [][]string
	for {
		answer := search(t, client, req)
		pages = append(pages, identifiers(answer.Results))
		if answer.PageToken == "" {
			break
		}
		req.PageToken = answer.PageToken
		if len(pages) > 10 {
			t.Fatal("the page token never emptied")
		}
	}

	if len(pages) != 3 {
		t.Fatalf("the walk took %d pages of 2 over 6 results, want 3: %v", len(pages), pages)
	}
	seen := map[string]bool{}
	for _, page := range pages {
		for _, identifier := range page {
			if seen[identifier] {
				t.Errorf("identifier %q came back on two pages", identifier)
			}
			seen[identifier] = true
		}
	}
}

func TestSearchClampsThePageSizeToTheMaximum(t *testing.T) {
	client := newFixtureClient(t)
	req := searchRequest("travel")
	req.PageSize = 5000

	answer := search(t, client, req)

	if len(answer.Results) > ard.MaxSearchPageSize {
		t.Errorf("the answer holds %d results, above the maximum page size of %d", len(answer.Results), ard.MaxSearchPageSize)
	}
}
