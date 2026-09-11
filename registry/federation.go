package registry

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"time"

	ard "github.com/veggiemonk/go-ard"
)

// DefaultUpstreamTimeout bounds one upstream call of a federated search.
const DefaultUpstreamTimeout = 5 * time.Second

// Upstream is one registry that a federating registry queries in auto mode and refers a
// client to in referrals mode. See section 5.4.
type Upstream struct {
	// Referral is what referrals mode hands to the client.
	Referral ard.Referral

	// Client is what auto mode queries. A nil client is never queried.
	Client *Client
}

// Federator answers the federation modes of section 5.4.
//
// It queries the upstreams at the same time and gives each one a timeout. An upstream
// that fails or times out degrades the answer; it does not fail the search.
type Federator struct {
	Upstreams []Upstream

	// Timeout bounds one upstream call. Zero means DefaultUpstreamTimeout.
	Timeout time.Duration
}

// Referrals lists the upstream registries that a client may query itself.
func (f *Federator) Referrals() []ard.Referral {
	if f == nil {
		return nil
	}
	referrals := make([]ard.Referral, 0, len(f.Upstreams))
	for _, up := range f.Upstreams {
		if up.Referral.Identifier != "" || up.Referral.URL != "" {
			referrals = append(referrals, up.Referral)
		}
	}
	if len(referrals) == 0 {
		return nil
	}
	return referrals
}

// Fanout queries every upstream and merges what answers.
//
// It sends federation "none" upstream, never "auto", because a cycle in the topology
// would otherwise loop for ever.
func (f *Federator) Fanout(ctx context.Context, req ard.SearchRequest) []ard.Result {
	if f == nil || len(f.Upstreams) == 0 {
		return nil
	}
	req.Federation = ard.FederationNone
	req.PageToken = ""
	answers := make([][]ard.Result, len(f.Upstreams))
	var wg sync.WaitGroup
	for i, up := range f.Upstreams {
		if up.Client == nil {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			answers[i] = f.ask(ctx, up, req)
		}()
	}
	wg.Wait()
	return MergeResults(answers...)
}

// MergeResults merges result sets, removes the duplicates by identifier keeping the
// better score, and sorts by score. See section 5.4.
func MergeResults(sets ...[]ard.Result) []ard.Result {
	merged := []ard.Result{}
	at := map[string]int{}
	for _, set := range sets {
		for _, result := range set {
			identifier := result.Entry.Identifier
			if identifier == "" {
				merged = append(merged, result)
				continue
			}
			if i, seen := at[identifier]; seen {
				if scoreOf(result) > scoreOf(merged[i]) {
					merged[i] = result
				}
				continue
			}
			at[identifier] = len(merged)
			merged = append(merged, result)
		}
	}
	slices.SortStableFunc(merged, func(a, b ard.Result) int {
		return cmp.Compare(scoreOf(b), scoreOf(a))
	})
	return merged
}

func (f *Federator) ask(ctx context.Context, up Upstream, req ard.SearchRequest) []ard.Result {
	ctx, cancel := context.WithTimeout(ctx, f.timeout())
	defer cancel()
	answer, err := up.Client.Search(ctx, req)
	if err != nil {
		return nil
	}
	return sourced(answer.Results, up.Client.BaseURL)
}

func (f *Federator) timeout() time.Duration {
	if f.Timeout > 0 {
		return f.Timeout
	}
	return DefaultUpstreamTimeout
}

func sourced(results []ard.Result, source string) []ard.Result {
	for i := range results {
		if results[i].Source == "" {
			results[i].Source = source
		}
	}
	return results
}

func scoreOf(r ard.Result) int {
	if r.Score == nil {
		return -1
	}
	return *r.Score
}
