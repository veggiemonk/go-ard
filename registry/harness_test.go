package registry_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	ard "github.com/veggiemonk/go-ard"
	"github.com/veggiemonk/go-ard/registry"
	"github.com/veggiemonk/go-ard/registry/memindex"
)

const localSource = "https://registry.test/api/ard/"

const okfNamespace = "https://openknowledgeformat.org/ns#"

func fixtureEntries(t *testing.T) []ard.Entry {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "entries.json"))
	if err != nil {
		t.Fatalf("read the fixture entries: %v", err)
	}
	var manifest ard.Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("decode the fixture entries: %v", err)
	}
	if len(manifest.Entries) == 0 {
		t.Fatal("the fixture holds no entry")
	}
	return manifest.Entries
}

func fixtureIndex(t *testing.T) *memindex.Index {
	t.Helper()
	return memindex.New(fixtureEntries(t))
}

func searchRequest(text string) ard.SearchRequest {
	return ard.SearchRequest{Query: ard.Query{Text: text}}
}

func queryContext(t *testing.T, bindings map[string]string) ard.Context {
	t.Helper()
	built, err := ard.NewContext(bindings)
	if err != nil {
		t.Fatalf("build the query context: %v", err)
	}
	return built
}

func search(t *testing.T, client *registry.Client, req ard.SearchRequest) ard.SearchResponse {
	t.Helper()
	answer, err := client.Search(context.Background(), req)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	return answer
}

func identifiers(results []ard.Result) []string {
	found := make([]string, 0, len(results))
	for _, result := range results {
		found = append(found, result.Entry.Identifier)
	}
	return found
}

func itemIdentifiers(items []ard.Entry) []string {
	found := make([]string, 0, len(items))
	for _, item := range items {
		found = append(found, item.Identifier)
	}
	return found
}

func facetValues(result ard.FacetResult) map[string]int {
	counts := make(map[string]int, len(result.Buckets))
	for _, bucket := range result.Buckets {
		if bucket.Count == nil {
			counts[bucket.Value] = -1
			continue
		}
		counts[bucket.Value] = *bucket.Count
	}
	return counts
}

func wantSet(t *testing.T, got, want []string) {
	t.Helper()
	sortedGot, sortedWant := slices.Clone(got), slices.Clone(want)
	slices.Sort(sortedGot)
	slices.Sort(sortedWant)
	if !slices.Equal(sortedGot, sortedWant) {
		t.Errorf("identifiers\n got %v\nwant %v", sortedGot, sortedWant)
	}
}
