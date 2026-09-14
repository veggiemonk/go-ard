package registry_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	ard "github.com/veggiemonk/go-ard"
	"github.com/veggiemonk/go-ard/registry"
)

type recorder struct {
	mu       sync.Mutex
	requests []ard.SearchRequest
}

func (r *recorder) add(req ard.SearchRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, req)
}

func (r *recorder) taken() []ard.SearchRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.requests)
}

type upstream struct {
	server   *httptest.Server
	referral ard.Referral
	seen     *recorder
}

func newUpstream(t *testing.T, name string, results []ard.Result) upstream {
	t.Helper()
	seen := &recorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ard.SearchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		seen.add(req)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ard.SearchResponse{Results: results})
	}))
	t.Cleanup(server.Close)
	return upstream{
		server: server,
		seen:   seen,
		referral: ard.Referral{
			Identifier:  "urn:air:" + name + ".example:registry:public",
			DisplayName: name,
			Type:        ard.MediaTypeAIRegistry,
			URL:         server.URL + registry.RouteSearch,
		},
	}
}

func upstreamResult(identifier string, score int) ard.Result {
	return ard.Result{
		Entry: ard.Entry{Identifier: identifier, DisplayName: identifier, Type: ard.MediaTypeA2AAgentCard},
		Score: &score,
	}
}

func federatedClient(t *testing.T, ups ...upstream) *registry.Client {
	t.Helper()
	federator := &registry.Federator{Timeout: 5 * time.Second}
	for _, up := range ups {
		federator.Upstreams = append(federator.Upstreams, registry.Upstream{
			Referral: up.referral,
			Client:   &registry.Client{BaseURL: up.server.URL, HTTPClient: up.server.Client()},
		})
	}
	return newClient(t, fixtureIndex(t), registry.Options{Federator: federator})
}

func federatedSearch(mode ard.FederationMode) ard.SearchRequest {
	req := searchRequest("travel booking")
	req.Federation = mode
	req.PageSize = 20
	return req
}

func TestFederationAutoSendsFederationNoneUpstream(t *testing.T) {
	first := newUpstream(t, "first", nil)
	second := newUpstream(t, "second", nil)
	client := federatedClient(t, first, second)

	search(t, client, federatedSearch(ard.FederationAuto))

	for name, seen := range map[string]*recorder{"first": first.seen, "second": second.seen} {
		taken := seen.taken()
		if len(taken) != 1 {
			t.Fatalf("upstream %s took %d requests, want 1", name, len(taken))
		}
		if taken[0].Federation != ard.FederationNone {
			t.Errorf("upstream %s took federation %q, want %q, or a cycle in the topology loops for ever",
				name, taken[0].Federation, ard.FederationNone)
		}
		if taken[0].Query.Text != "travel booking" {
			t.Errorf("upstream %s took the text %q, want the text of the client", name, taken[0].Query.Text)
		}
	}
}

func TestFederationAutoMergesAndRemovesTheDuplicates(t *testing.T) {
	first := newUpstream(t, "first", []ard.Result{
		upstreamResult("urn:air:first.example:agent:ferries", 60),
		upstreamResult("urn:air:travel.example:agent:flights", 40),
	})
	second := newUpstream(t, "second", []ard.Result{
		upstreamResult("urn:air:first.example:agent:ferries", 95),
	})
	client := federatedClient(t, first, second)

	answer := search(t, client, federatedSearch(ard.FederationAuto))

	found := identifiers(answer.Results)
	wantSet(t, found, []string{
		"urn:air:travel.example:agent:flights",
		"urn:air:travel.example:agent:hotels",
		"urn:air:travel.example:agent:cars",
		"urn:air:example.com:agent:trains",
		"urn:air:example.com:agent:visas",
		"urn:air:first.example:agent:ferries",
	})
	if got := scoreFor(t, answer.Results, "urn:air:first.example:agent:ferries"); got != 95 {
		t.Errorf("the duplicate upstream entry kept the score %d, want the better score 95", got)
	}
	if got := scoreFor(t, answer.Results, "urn:air:travel.example:agent:flights"); got != 100 {
		t.Errorf("the local entry kept the score %d, want the better local score 100", got)
	}
	for i := 1; i < len(answer.Results); i++ {
		if *answer.Results[i-1].Score < *answer.Results[i].Score {
			t.Fatalf("the merged results are not sorted by score: %v", answer.Results)
		}
	}
}

func TestFederationReferralsReturnsTheReferralsAndQueriesNoUpstream(t *testing.T) {
	first := newUpstream(t, "first", nil)
	second := newUpstream(t, "second", nil)
	client := federatedClient(t, first, second)

	answer := search(t, client, federatedSearch(ard.FederationReferrals))

	if len(answer.Referrals) != 2 {
		t.Fatalf("the answer holds %d referrals, want 2", len(answer.Referrals))
	}
	if answer.Referrals[0].URL != first.referral.URL {
		t.Errorf("referral URL %q, want %q", answer.Referrals[0].URL, first.referral.URL)
	}
	if len(first.seen.taken())+len(second.seen.taken()) != 0 {
		t.Error("referrals mode queried an upstream, and section 5.4 leaves that to the client")
	}
	if len(answer.Results) == 0 {
		t.Error("referrals mode returned no local result")
	}
}

func TestFederationNoneCallsNoUpstream(t *testing.T) {
	first := newUpstream(t, "first", []ard.Result{upstreamResult("urn:air:first.example:agent:ferries", 95)})
	client := federatedClient(t, first)

	answer := search(t, client, federatedSearch(ard.FederationNone))

	if len(first.seen.taken()) != 0 {
		t.Error("none mode queried an upstream")
	}
	if len(answer.Referrals) != 0 {
		t.Errorf("none mode returned %d referrals, want none", len(answer.Referrals))
	}
	if slices.Contains(identifiers(answer.Results), "urn:air:first.example:agent:ferries") {
		t.Error("none mode returned an upstream entry")
	}
}

func TestFederationAutoDegradesWhenAnUpstreamIsDead(t *testing.T) {
	live := newUpstream(t, "live", []ard.Result{upstreamResult("urn:air:live.example:agent:ferries", 95)})
	dead := newUpstream(t, "dead", nil)
	dead.server.Close()
	client := federatedClient(t, live, dead)

	answer := search(t, client, federatedSearch(ard.FederationAuto))

	if !slices.Contains(identifiers(answer.Results), "urn:air:live.example:agent:ferries") {
		t.Error("the live upstream did not contribute")
	}
	if len(answer.Results) != 6 {
		t.Errorf("the answer holds %d results, want the 5 local plus the 1 live upstream", len(answer.Results))
	}
}

func TestFederationWithoutAFederatorSearchesTheLocalIndexOnly(t *testing.T) {
	client := newFixtureClient(t)

	answer := search(t, client, federatedSearch(ard.FederationReferrals))

	if len(answer.Referrals) != 0 {
		t.Errorf("a registry with no federator returned %d referrals", len(answer.Referrals))
	}
	if len(answer.Results) != 5 {
		t.Errorf("the answer holds %d results, want the 5 local ones", len(answer.Results))
	}
}

func TestMergeResultsKeepsTheBetterScore(t *testing.T) {
	merged := registry.MergeResults(
		[]ard.Result{upstreamResult("a", 10), upstreamResult("b", 80)},
		[]ard.Result{upstreamResult("a", 90)},
	)

	if len(merged) != 2 {
		t.Fatalf("the merge gave %d results, want 2", len(merged))
	}
	if merged[0].Entry.Identifier != "a" || *merged[0].Score != 90 {
		t.Errorf("the merge gave %q at %d first, want a at 90", merged[0].Entry.Identifier, *merged[0].Score)
	}
}

func TestBalanceResultsCapsWhatOneUpstreamContributes(t *testing.T) {
	generous := []ard.Result{
		upstreamResult("g1", 99), upstreamResult("g2", 98),
		upstreamResult("g3", 97), upstreamResult("g4", 96),
	}
	modest := []ard.Result{upstreamResult("m1", 40), upstreamResult("m2", 30)}

	balanced := registry.BalanceResults(registry.Balance{PerSource: 2}, generous, modest)

	if len(balanced) != 4 {
		t.Fatalf("the balance gave %d results, want 4", len(balanced))
	}
	for _, unwanted := range []string{"g3", "g4"} {
		if slices.ContainsFunc(balanced, func(r ard.Result) bool { return r.Entry.Identifier == unwanted }) {
			t.Errorf("the balance kept %q, which is outside the two best of its upstream", unwanted)
		}
	}
	for _, wanted := range []string{"g1", "g2", "m1", "m2"} {
		if !slices.ContainsFunc(balanced, func(r ard.Result) bool { return r.Entry.Identifier == wanted }) {
			t.Errorf("the balance dropped %q", wanted)
		}
	}
}

// An upstream keeps its best results, whatever order it sent them in.
func TestBalanceResultsKeepsTheBestOfAnUnsortedUpstream(t *testing.T) {
	unsorted := []ard.Result{upstreamResult("low", 10), upstreamResult("high", 90), upstreamResult("mid", 50)}

	balanced := registry.BalanceResults(registry.Balance{PerSource: 1}, unsorted)

	if len(balanced) != 1 || balanced[0].Entry.Identifier != "high" {
		t.Errorf("the balance kept %v, want the one result high", balanced)
	}
	if len(unsorted) != 3 || unsorted[0].Entry.Identifier != "low" {
		t.Error("the balance reordered the set the caller gave it")
	}
}

func TestBalanceResultsCapsHowManyUpstreamsContribute(t *testing.T) {
	first := []ard.Result{upstreamResult("a", 10)}
	second := []ard.Result{upstreamResult("b", 20)}
	third := []ard.Result{upstreamResult("c", 100)}

	balanced := registry.BalanceResults(registry.Balance{MaxSources: 2}, first, second, third)

	if len(balanced) != 2 {
		t.Fatalf("the balance gave %d results, want 2", len(balanced))
	}
	if slices.ContainsFunc(balanced, func(r ard.Result) bool { return r.Entry.Identifier == "c" }) {
		t.Error("the balance kept the third upstream, and MaxSources is 2")
	}
}

// An upstream that answers with nothing must not spend a place under MaxSources.
func TestBalanceResultsGivesNoPlaceToAnEmptyUpstream(t *testing.T) {
	balanced := registry.BalanceResults(registry.Balance{MaxSources: 2},
		nil, []ard.Result{upstreamResult("a", 10)}, nil, []ard.Result{upstreamResult("b", 20)})

	if len(balanced) != 2 {
		t.Fatalf("the balance gave %d results, want 2", len(balanced))
	}
}

func TestBalanceResultsWithoutACapMergesPurelyByScore(t *testing.T) {
	sets := [][]ard.Result{
		{upstreamResult("a", 10), upstreamResult("b", 80)},
		{upstreamResult("a", 90)},
	}

	balanced := registry.BalanceResults(registry.Balance{}, sets...)
	merged := registry.MergeResults(sets...)

	if len(balanced) != len(merged) {
		t.Fatalf("the empty balance gave %d results and the merge gave %d", len(balanced), len(merged))
	}
	for i := range merged {
		if balanced[i].Entry.Identifier != merged[i].Entry.Identifier {
			t.Errorf("at %d the empty balance gave %q and the merge gave %q",
				i, balanced[i].Entry.Identifier, merged[i].Entry.Identifier)
		}
	}
}

// A score from one registry does not compare with a score from another, so a generous
// upstream must not fill the answer and hide a modest one.
func TestFederationAutoHonoursTheBalance(t *testing.T) {
	generous := newUpstream(t, "generous", []ard.Result{
		upstreamResult("urn:air:generous.example:agent:one", 99),
		upstreamResult("urn:air:generous.example:agent:two", 98),
		upstreamResult("urn:air:generous.example:agent:three", 97),
	})
	modest := newUpstream(t, "modest", []ard.Result{
		upstreamResult("urn:air:modest.example:agent:one", 20),
	})

	federator := &registry.Federator{
		Timeout: 5 * time.Second,
		Balance: registry.Balance{PerSource: 1},
		Upstreams: []registry.Upstream{
			{Referral: generous.referral, Client: &registry.Client{BaseURL: generous.server.URL, HTTPClient: generous.server.Client()}},
			{Referral: modest.referral, Client: &registry.Client{BaseURL: modest.server.URL, HTTPClient: modest.server.Client()}},
		},
	}

	merged := federator.Fanout(t.Context(), federatedSearch(ard.FederationAuto))

	if len(merged) != 2 {
		t.Fatalf("the fanout gave %d results, want one from each upstream", len(merged))
	}
	if !slices.ContainsFunc(merged, func(r ard.Result) bool {
		return r.Entry.Identifier == "urn:air:modest.example:agent:one"
	}) {
		t.Error("the generous upstream hid the modest one")
	}
}

func scoreFor(t *testing.T, results []ard.Result, identifier string) int {
	t.Helper()
	for _, result := range results {
		if result.Entry.Identifier != identifier {
			continue
		}
		if result.Score == nil {
			t.Fatalf("the result %q carries no score", identifier)
		}
		return *result.Score
	}
	t.Fatalf("the results hold no %q", identifier)
	return 0
}
