package registry_test

import (
	"context"
	"slices"
	"testing"

	ard "github.com/veggiemonk/go-ard"
	"github.com/veggiemonk/go-ard/registry"
)

func list(t *testing.T, client *registry.Client, opt registry.ListOptions) ard.ListResponse {
	t.Helper()
	answer, err := client.List(context.Background(), opt)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	return answer
}

func TestListWithoutAFilterGivesEveryEntry(t *testing.T) {
	client := newFixtureClient(t)

	answer := list(t, client, registry.ListOptions{PageSize: 50})

	if len(answer.Items) != 9 {
		t.Errorf("the listing holds %d items, want the 9 of the fixture", len(answer.Items))
	}
	if answer.Total == nil || *answer.Total != 9 {
		t.Errorf("total %v, want 9", answer.Total)
	}
}

func TestListOrdersDeterministicallyWithoutAnOrderBy(t *testing.T) {
	client := newFixtureClient(t)

	first := list(t, client, registry.ListOptions{PageSize: 50})
	second := list(t, client, registry.ListOptions{PageSize: 50})

	if !slices.Equal(itemIdentifiers(first.Items), itemIdentifiers(second.Items)) {
		t.Errorf("two identical listings gave two orders:\n%v\n%v", itemIdentifiers(first.Items), itemIdentifiers(second.Items))
	}
	if !slices.IsSorted(itemIdentifiers(first.Items)) {
		t.Errorf("the default order is not the identifier: %v", itemIdentifiers(first.Items))
	}
}

func TestListWithTheFilterFieldsOfAppendixA(t *testing.T) {
	client := newFixtureClient(t)
	cases := map[string]struct {
		filter string
		want   []string
	}{
		"type": {
			filter: "type = 'application/mcp-server-card+json'",
			want: []string{
				"urn:air:acme.com:server:weather",
				"urn:air:acme.com:tool:unit-converter",
			},
		},
		"two types are or": {
			filter: "type = 'application/mcp-server-card+json,application/ai-registry+json'",
			want: []string{
				"urn:air:acme.com:server:weather",
				"urn:air:acme.com:tool:unit-converter",
				"urn:air:example.com:registry:public",
			},
		},
		"publisherId": {
			filter: "publisherId = 'travel.example'",
			want: []string{
				"urn:air:travel.example:agent:flights",
				"urn:air:travel.example:agent:hotels",
				"urn:air:travel.example:agent:cars",
			},
		},
		"two parameters are and": {
			filter: "publisherId = 'travel.example' AND updatedAfter > '2026-03-02'",
			want: []string{
				"urn:air:travel.example:agent:hotels",
				"urn:air:travel.example:agent:cars",
			},
		},
		"displayName ignores the case": {
			filter: "displayName = 'travel agent'",
			want: []string{
				"urn:air:travel.example:agent:flights",
				"urn:air:travel.example:agent:hotels",
				"urn:air:travel.example:agent:cars",
			},
		},
		"createdAfter": {
			filter: "createdAfter > '2025-12-31'",
			want:   []string{"urn:air:acme.com:agent:assistant"},
		},
		"updatedAfter": {
			filter: "updatedAfter > '2026-03-03T12:00:00Z'",
			want: []string{
				"urn:air:example.com:agent:trains",
				"urn:air:example.com:agent:visas",
			},
		},
	}

	for name, each := range cases {
		t.Run(name, func(t *testing.T) {
			answer := list(t, client, registry.ListOptions{Filter: each.filter, PageSize: 50})
			wantSet(t, itemIdentifiers(answer.Items), each.want)
		})
	}
}

func TestListOrdersByAField(t *testing.T) {
	client := newFixtureClient(t)

	ascending := list(t, client, registry.ListOptions{OrderBy: "displayName", PageSize: 50})
	descending := list(t, client, registry.ListOptions{OrderBy: "displayName DESC", PageSize: 50})

	forward := itemIdentifiers(ascending.Items)
	backward := itemIdentifiers(descending.Items)
	slices.Reverse(backward)
	if !slices.Equal(forward, backward) {
		t.Errorf("DESC is not the reverse of ASC:\n%v\n%v", forward, backward)
	}
	if got := ascending.Items[0].DisplayName; got != "Car Rental Travel Agent" {
		t.Errorf("the first item by display name is %q, want %q", got, "Car Rental Travel Agent")
	}
}

func TestListOrdersByATimestamp(t *testing.T) {
	client := newFixtureClient(t)
	cases := map[string]string{
		"updatedAt":       "urn:air:acme.com:tool:unit-converter",
		"updated_at DESC": "urn:air:example.com:agent:visas",
		"createdAt DESC":  "urn:air:acme.com:agent:assistant",
	}

	for order, want := range cases {
		t.Run(order, func(t *testing.T) {
			answer := list(t, client, registry.ListOptions{OrderBy: order, PageSize: 50})
			if got := answer.Items[0].Identifier; got != want {
				t.Errorf("the first item by %q is %q, want %q", order, got, want)
			}
		})
	}
}

func TestListWalksThePages(t *testing.T) {
	client := newFixtureClient(t)
	opt := registry.ListOptions{PageSize: 4}

	var walked []string
	for range 5 {
		answer := list(t, client, opt)
		walked = append(walked, itemIdentifiers(answer.Items)...)
		if answer.PageToken == "" {
			break
		}
		opt.PageToken = answer.PageToken
	}

	if len(walked) != 9 {
		t.Errorf("the walk gave %d items over pages of 4, want 9: %v", len(walked), walked)
	}
}

func TestListClampsThePageSizeToTheMaximum(t *testing.T) {
	client := newFixtureClient(t)

	answer := list(t, client, registry.ListOptions{PageSize: 5000})

	if len(answer.Items) > ard.MaxListPageSize {
		t.Errorf("the listing holds %d items, above the maximum page size of %d", len(answer.Items), ard.MaxListPageSize)
	}
}
