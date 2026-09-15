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

	// Balance caps what one upstream contributes to a merged answer. The zero value
	// caps nothing and merges purely by score.
	Balance Balance
}

// Balance caps what one upstream contributes to a merged answer.
//
// A score from one registry does not compare with a score from another: each registry
// scores against its own index, on its own scale, and the specification fixes only the
// range of 0 to 100. Merging purely by score therefore lets one generous upstream fill
// the answer and hide a better result from a modest one. Balance bounds that.
//
// The specification does not name the behaviour. The reference client of Hugging Face
// takes 3 results from each of at most 3 registries; a caller here chooses the numbers.
type Balance struct {
	// MaxSources caps how many upstreams contribute. An upstream that answers with
	// nothing takes no place. Zero means every upstream contributes.
	MaxSources int

	// PerSource caps how many results one upstream contributes, keeping its best by
	// score. Zero means all of them.
	PerSource int
}

// IsZero reports whether the balance caps nothing.
func (b Balance) IsZero() bool { return b.MaxSources <= 0 && b.PerSource <= 0 }

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
	return BalanceResults(f.Balance, answers...)
}

// BalanceResults merges result sets under a balance, and merges purely by score when the
// balance caps nothing. See Balance and MergeResults.
//
// The sets keep the order the caller gives them, which for a Fanout is the order of the
// upstreams. A set that holds nothing takes no place under MaxSources.
func BalanceResults(b Balance, sets ...[]ard.Result) []ard.Result {
	if b.IsZero() {
		return MergeResults(sets...)
	}
	kept := make([][]ard.Result, 0, len(sets))
	for _, set := range sets {
		if len(set) == 0 {
			continue
		}
		kept = append(kept, bestByScore(set, b.PerSource))
		if b.MaxSources > 0 && len(kept) == b.MaxSources {
			break
		}
	}
	return MergeResults(kept...)
}

// bestByScore gives the n best results of one set by score, without touching the set.
func bestByScore(set []ard.Result, n int) []ard.Result {
	if n <= 0 || len(set) <= n {
		return set
	}
	best := slices.Clone(set)
	slices.SortStableFunc(best, func(a, b ard.Result) int {
		return cmp.Compare(scoreOf(b), scoreOf(a))
	})
	return best[:n]
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
