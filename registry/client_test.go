package registry_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	ard "github.com/veggiemonk/go-ard"
	"github.com/veggiemonk/go-ard/registry"
)

func echoServer(t *testing.T, seen *url.URL, header *http.Header) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*seen = *r.URL
		*header = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ard.SearchResponse{Results: []ard.Result{}})
	}))
	t.Cleanup(server.Close)
	return server
}

func TestClientJoinsTheBaseURLAndTheRoute(t *testing.T) {
	var seen url.URL
	var header http.Header
	server := echoServer(t, &seen, &header)
	cases := map[string]struct {
		suffix string
		want   string
	}{
		"bare":                                {"", "/search"},
		"trailing slash":                      {"/", "/search"},
		"path prefix":                         {"/api/ard", "/api/ard/search"},
		"path prefix and slash":               {"/api/ard/", "/api/ard/search"},
		"path prefix and double":              {"/api//ard", "/api/ard/search"},
		"referral search route":               {"/search", "/search"},
		"referral and slash":                  {"/search/", "/search"},
		"referral under a path":               {"/registries/tools/search", "/registries/tools/search"},
		"a path that ends in the word search": {"/api/research", "/api/research/search"},
	}

	for name, each := range cases {
		t.Run(name, func(t *testing.T) {
			client := &registry.Client{BaseURL: server.URL + each.suffix, HTTPClient: server.Client()}
			if _, err := client.Search(context.Background(), searchRequest("travel")); err != nil {
				t.Fatalf("search: %v", err)
			}
			if seen.Path != each.want {
				t.Errorf("the registry saw the path %q, want %q", seen.Path, each.want)
			}
		})
	}
}

// A referral names the search route. A Client built from one must still reach the other
// two routes of section 5.3, at the same registry.
func TestClientBuiltFromAReferralReachesEveryRoute(t *testing.T) {
	var seen url.URL
	var header http.Header
	server := echoServer(t, &seen, &header)
	client := &registry.Client{
		BaseURL:    server.URL + "/registries/tools/search",
		HTTPClient: server.Client(),
	}

	if _, err := client.Search(context.Background(), searchRequest("travel")); err != nil {
		t.Fatalf("search: %v", err)
	}
	if seen.Path != "/registries/tools/search" {
		t.Errorf("search asked for %q, want /registries/tools/search", seen.Path)
	}

	if _, err := client.Explore(context.Background(), ard.ExploreRequest{}); err != nil {
		t.Fatalf("explore: %v", err)
	}
	if seen.Path != "/registries/tools/explore" {
		t.Errorf("explore asked for %q, want /registries/tools/explore", seen.Path)
	}

	if _, err := client.List(context.Background(), registry.ListOptions{}); err != nil {
		t.Fatalf("list: %v", err)
	}
	if seen.Path != "/registries/tools/agents" {
		t.Errorf("list asked for %q, want /registries/tools/agents", seen.Path)
	}
}

func TestClientSendsTheConfiguredHeader(t *testing.T) {
	var seen url.URL
	var header http.Header
	server := echoServer(t, &seen, &header)
	client := &registry.Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
		Header:     http.Header{"Authorization": {"Bearer secret"}, "X-Tenant": {"acme"}},
	}

	if _, err := client.Search(context.Background(), searchRequest("travel")); err != nil {
		t.Fatalf("search: %v", err)
	}

	if got := header.Get("Authorization"); got != "Bearer secret" {
		t.Errorf("the registry saw the authorization %q, want %q", got, "Bearer secret")
	}
	if got := header.Get("X-Tenant"); got != "acme" {
		t.Errorf("the registry saw the tenant %q, want %q", got, "acme")
	}
	if got := header.Get("Content-Type"); got != "application/json" {
		t.Errorf("the registry saw the content type %q, want application/json", got)
	}
}

func TestClientSendsTheListParametersAsAQuery(t *testing.T) {
	var seen url.URL
	var header http.Header
	server := echoServer(t, &seen, &header)
	client := &registry.Client{BaseURL: server.URL + "/api", HTTPClient: server.Client()}

	if _, err := client.List(context.Background(), registry.ListOptions{
		Filter:    "type = 'application/mcp-server-card+json'",
		OrderBy:   "displayName DESC",
		PageSize:  7,
		PageToken: "abc",
	}); err != nil {
		t.Fatalf("list: %v", err)
	}

	if seen.Path != "/api/agents" {
		t.Errorf("the registry saw the path %q, want /api/agents", seen.Path)
	}
	values := seen.Query()
	want := map[string]string{
		"filter":    "type = 'application/mcp-server-card+json'",
		"orderBy":   "displayName DESC",
		"pageSize":  "7",
		"pageToken": "abc",
	}
	for name, value := range want {
		if got := values.Get(name); got != value {
			t.Errorf("the registry saw %s=%q, want %q", name, got, value)
		}
	}
}

func TestClientRejectsABaseURLWithoutAHost(t *testing.T) {
	client := &registry.Client{BaseURL: "/api/ard"}

	if _, err := client.Search(context.Background(), searchRequest("travel")); err == nil {
		t.Fatal("a base URL with no host gave no error")
	}
}

func TestClientTurnsANonJSONErrorAnswerIntoAnAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "the gateway is unwell", http.StatusBadGateway)
	}))
	t.Cleanup(server.Close)
	client := &registry.Client{BaseURL: server.URL, HTTPClient: server.Client()}

	_, err := client.Search(context.Background(), searchRequest("travel"))

	wantAPIError(t, err, http.StatusBadGateway, ard.CodeInternalError)
}

func TestClientToleratesAResultWithoutAScoreOrASource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"identifier":"urn:air:acme.com:agent:bare"}]}`))
	}))
	t.Cleanup(server.Close)
	client := &registry.Client{BaseURL: server.URL, HTTPClient: server.Client()}

	answer, err := client.Search(context.Background(), searchRequest("travel"))
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(answer.Results) != 1 {
		t.Fatalf("the answer holds %d results, want 1", len(answer.Results))
	}
	if answer.Results[0].Score != nil {
		t.Error("a result with no score decoded to a score")
	}
	if answer.Results[0].Entry.Identifier != "urn:air:acme.com:agent:bare" {
		t.Errorf("identifier %q, want urn:air:acme.com:agent:bare", answer.Results[0].Entry.Identifier)
	}
}

func TestSearchAllStopsOnACancelledContext(t *testing.T) {
	client := newFixtureClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := searchRequest("corporate travel")
	req.PageSize = 1

	var walked int
	var last error
	for _, err := range client.SearchAll(ctx, req) {
		walked++
		last = err
		break
	}

	if walked != 1 || last == nil {
		t.Errorf("the walk gave %d steps and the error %v, want one step and a cancellation", walked, last)
	}
}

func TestSearchAllStopsWhenTheCallerBreaks(t *testing.T) {
	client := newFixtureClient(t)
	req := searchRequest("corporate travel")
	req.PageSize = 1

	var walked int
	for _, err := range client.SearchAll(context.Background(), req) {
		if err != nil {
			t.Fatalf("walk the pages: %v", err)
		}
		walked++
		if walked == 2 {
			break
		}
	}

	if walked != 2 {
		t.Errorf("the walk gave %d steps, want 2", walked)
	}
}
