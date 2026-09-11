package memindex

import (
	"cmp"
	"slices"

	ard "github.com/veggiemonk/go-ard"
	"github.com/veggiemonk/go-ard/registry"
)

type tally struct {
	value string
	count int
}

func aggregate(records []record, facet registry.Facet) (ard.FacetResult, error) {
	counts := map[string]int{}
	for _, r := range records {
		values, err := r.values(facet.Path)
		if err != nil {
			return ard.FacetResult{}, err
		}
		for _, value := range distinct(values) {
			counts[value]++
		}
	}
	ranked := rank(counts, facet.MinCount)
	limit := facet.Limit
	if limit <= 0 {
		limit = ard.DefaultFacetLimit
	}
	other := 0
	if len(ranked) > limit {
		for _, t := range ranked[limit:] {
			other += t.count
		}
		ranked = ranked[:limit]
	}
	result := ard.FacetResult{Buckets: make([]ard.Bucket, 0, len(ranked)), OtherCount: &other}
	for _, t := range ranked {
		count := t.count
		result.Buckets = append(result.Buckets, ard.Bucket{Value: t.value, Count: &count})
	}
	return result, nil
}

func rank(counts map[string]int, minCount int) []tally {
	if minCount < ard.DefaultFacetMinCount {
		minCount = ard.DefaultFacetMinCount
	}
	ranked := make([]tally, 0, len(counts))
	for value, count := range counts {
		if count < minCount {
			continue
		}
		ranked = append(ranked, tally{value: value, count: count})
	}
	slices.SortFunc(ranked, func(a, b tally) int {
		if c := cmp.Compare(b.count, a.count); c != 0 {
			return c
		}
		return cmp.Compare(a.value, b.value)
	})
	return ranked
}

func distinct(values []string) []string {
	if len(values) < 2 {
		return values
	}
	seen := make(map[string]bool, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		unique = append(unique, value)
	}
	return unique
}
