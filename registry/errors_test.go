package registry_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	ard "github.com/veggiemonk/go-ard"
	"github.com/veggiemonk/go-ard/registry"
)

type searchOnlyIndex struct {
	inner registry.Index
}

func (i searchOnlyIndex) Search(ctx context.Context, q registry.SearchQuery) (registry.Page, error) {
	return i.inner.Search(ctx, q)
}

func (searchOnlyIndex) Explore(context.Context, registry.ExploreQuery) (map[string]ard.FacetResult, error) {
	return nil, ard.ErrNotImplemented
}

func (searchOnlyIndex) List(context.Context, registry.ListQuery) (registry.ListPage, error) {
	return registry.ListPage{}, ard.ErrNotImplemented
}

func wantAPIError(t *testing.T, err error, status int, code string) {
	t.Helper()
	var answer *ard.APIError
	if !errors.As(err, &answer) {
		t.Fatalf("error %v is no *ard.APIError", err)
	}
	if answer.HTTPStatus != status {
		t.Errorf("status %d, want %d (%v)", answer.HTTPStatus, status, err)
	}
	if answer.Code != code {
		t.Errorf("code %q, want %q (%v)", answer.Code, code, err)
	}
	if answer.Message == "" {
		t.Error("the error answer carries no message, and appendix B requires one")
	}
}

func TestSearchWithoutTextIsInvalid(t *testing.T) {
	client := newFixtureClient(t)

	_, err := client.Search(context.Background(), ard.SearchRequest{})

	wantAPIError(t, err, http.StatusBadRequest, ard.CodeInvalidArgument)
}

func TestSearchWithABadRequestBodyIsInvalid(t *testing.T) {
	client := newFixtureClient(t)

	answer, err := rawPost(t, client, registry.RouteSearch, "{ this is not JSON")
	if err != nil {
		t.Fatalf("post a malformed body: %v", err)
	}

	wantAnswerError(t, answer, http.StatusBadRequest, ard.CodeInvalidArgument)
}

func TestSearchWithAnEmptyRequestBodyIsInvalid(t *testing.T) {
	client := newFixtureClient(t)

	answer, err := rawPost(t, client, registry.RouteSearch, "")
	if err != nil {
		t.Fatalf("post an empty body: %v", err)
	}

	wantAnswerError(t, answer, http.StatusBadRequest, ard.CodeInvalidArgument)
}

func TestSearchWithAnUnknownFederationModeIsInvalid(t *testing.T) {
	client := newFixtureClient(t)
	req := searchRequest("travel")
	req.Federation = "everywhere"

	_, err := client.Search(context.Background(), req)

	wantAPIError(t, err, http.StatusBadRequest, ard.CodeInvalidArgument)
}

func TestSearchWithANegativePageSizeIsInvalid(t *testing.T) {
	client := newFixtureClient(t)
	req := searchRequest("travel")
	req.PageSize = -1

	_, err := client.Search(context.Background(), req)

	wantAPIError(t, err, http.StatusBadRequest, ard.CodeInvalidArgument)
}

func TestSearchWithAnUnsupportedFilterKeyIsInvalid(t *testing.T) {
	client := newFixtureClient(t)
	req := searchRequest("travel")
	req.Query.Context = queryContext(t, map[string]string{"okf": okfNamespace})
	req.Query.Filter = map[string][]string{"okf:noSuchTerm": {"anything"}}

	_, err := client.Search(context.Background(), req)

	wantAPIError(t, err, http.StatusBadRequest, ard.CodeInvalidArgument)
}

func TestSearchWithAnUnboundFilterPrefixIsInvalid(t *testing.T) {
	client := newFixtureClient(t)
	req := searchRequest("travel")
	req.Query.Filter = map[string][]string{"nobodybound:taxonomy": {"iata"}}

	_, err := client.Search(context.Background(), req)

	wantAPIError(t, err, http.StatusBadRequest, ard.CodeInvalidArgument)
}

func TestSearchWithACorruptPageTokenIsInvalid(t *testing.T) {
	client := newFixtureClient(t)
	req := searchRequest("travel")
	req.PageToken = "this-is-not-a-cursor"

	_, err := client.Search(context.Background(), req)

	wantAPIError(t, err, http.StatusBadRequest, ard.CodeInvalidArgument)
}

func TestSearchWithThePageTokenOfAnotherQueryIsInvalid(t *testing.T) {
	client := newFixtureClient(t)
	first := searchRequest("corporate travel")
	first.PageSize = 2
	answer := search(t, client, first)
	if answer.PageToken == "" {
		t.Fatal("the first page gave no page token")
	}

	second := searchRequest("weather forecast")
	second.PageSize = 2
	second.PageToken = answer.PageToken
	_, err := client.Search(context.Background(), second)

	wantAPIError(t, err, http.StatusBadRequest, ard.CodeInvalidArgument)
}

func TestExploreWithoutAResultTypeIsInvalid(t *testing.T) {
	client := newFixtureClient(t)

	_, err := client.Explore(context.Background(), ard.ExploreRequest{})

	wantAPIError(t, err, http.StatusBadRequest, ard.CodeInvalidArgument)
}

func TestExploreWithAFacetWithoutAFieldIsInvalid(t *testing.T) {
	client := newFixtureClient(t)

	_, err := client.Explore(context.Background(), facetRequest(ard.FacetRequest{Limit: 5}))

	wantAPIError(t, err, http.StatusBadRequest, ard.CodeInvalidArgument)
}

func TestExploreWithAnUnsupportedFacetFieldIsInvalid(t *testing.T) {
	client := newFixtureClient(t)
	req := facetRequest(ard.FacetRequest{Field: "okf:noSuchTerm"})
	req.Query.Context = queryContext(t, map[string]string{"okf": okfNamespace})

	_, err := client.Explore(context.Background(), req)

	wantAPIError(t, err, http.StatusBadRequest, ard.CodeInvalidArgument)
}

func TestListWithAnUnparsableFilterIsInvalid(t *testing.T) {
	client := newFixtureClient(t)
	cases := map[string]string{
		"no operator":     "type 'application/mcp-server-card+json'",
		"unknown field":   "colour = 'blue'",
		"unknown join":    "type = 'a' OR type = 'b'",
		"open quote":      "type = 'application/mcp",
		"bad operator":    "type != 'a'",
		"bad timestamp":   "updatedAfter > 'last tuesday'",
		"empty clause":    "type =",
		"nothing at all ": "AND",
	}

	for name, filter := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := client.List(context.Background(), registry.ListOptions{Filter: filter})
			wantAPIError(t, err, http.StatusBadRequest, ard.CodeInvalidArgument)
		})
	}
}

func TestListWithAnUnknownOrderByFieldIsInvalid(t *testing.T) {
	client := newFixtureClient(t)

	_, err := client.List(context.Background(), registry.ListOptions{OrderBy: "colour DESC"})

	wantAPIError(t, err, http.StatusBadRequest, ard.CodeInvalidArgument)
}

func TestListWithABadPageSizeIsInvalid(t *testing.T) {
	client := newFixtureClient(t)

	answer, err := rawGet(t, client, registry.RouteAgents+"?pageSize=many")
	if err != nil {
		t.Fatalf("get a bad page size: %v", err)
	}

	wantAnswerError(t, answer, http.StatusBadRequest, ard.CodeInvalidArgument)
}

func TestListAnswers501WhenTheIndexDoesNotImplementIt(t *testing.T) {
	client := newClient(t, searchOnlyIndex{inner: fixtureIndex(t)}, registry.Options{})

	_, err := client.List(context.Background(), registry.ListOptions{})

	if !errors.Is(err, ard.ErrNotImplemented) {
		t.Fatalf("error %v, want one that satisfies errors.Is(err, ard.ErrNotImplemented)", err)
	}
	wantAPIError(t, err, http.StatusNotImplemented, ard.CodeNotImplemented)
}

func TestAWrongMethodIsNotAllowed(t *testing.T) {
	client := newFixtureClient(t)
	cases := map[string]struct {
		method string
		route  string
	}{
		"get search":    {http.MethodGet, registry.RouteSearch},
		"get explore":   {http.MethodGet, registry.RouteExplore},
		"post agents":   {http.MethodPost, registry.RouteAgents},
		"delete search": {http.MethodDelete, registry.RouteSearch},
	}

	for name, each := range cases {
		t.Run(name, func(t *testing.T) {
			answer, err := rawCall(t, client, each.method, each.route, "")
			if err != nil {
				t.Fatalf("call %s %s: %v", each.method, each.route, err)
			}
			wantAnswerError(t, answer, http.StatusMethodNotAllowed, ard.CodeInvalidArgument)
		})
	}
}

func TestAnUnknownRouteIsNotFound(t *testing.T) {
	client := newFixtureClient(t)

	answer, err := rawGet(t, client, "/nowhere")
	if err != nil {
		t.Fatalf("get an unknown route: %v", err)
	}

	wantAnswerError(t, answer, http.StatusNotFound, ard.CodeNotFound)
}

type rawAnswer struct {
	status int
	body   ard.ErrorBody
}

func wantAnswerError(t *testing.T, answer rawAnswer, status int, code string) {
	t.Helper()
	if answer.status != status {
		t.Errorf("status %d, want %d", answer.status, status)
	}
	if answer.body.ErrorCode != code {
		t.Errorf("errorCode %q, want %q", answer.body.ErrorCode, code)
	}
	if answer.body.Message == "" {
		t.Error("the error answer carries no message, and appendix B requires one")
	}
}

func rawGet(t *testing.T, client *registry.Client, route string) (rawAnswer, error) {
	t.Helper()
	return rawCall(t, client, http.MethodGet, route, "")
}

func rawPost(t *testing.T, client *registry.Client, route, body string) (rawAnswer, error) {
	t.Helper()
	return rawCall(t, client, http.MethodPost, route, body)
}

func rawCall(t *testing.T, client *registry.Client, method, route, body string) (rawAnswer, error) {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), method, client.BaseURL+route, strings.NewReader(body))
	if err != nil {
		return rawAnswer{}, err
	}
	answer, err := client.HTTPClient.Do(request)
	if err != nil {
		return rawAnswer{}, err
	}
	defer answer.Body.Close()
	raw, err := io.ReadAll(answer.Body)
	if err != nil {
		return rawAnswer{}, err
	}
	got := rawAnswer{status: answer.StatusCode}
	if err := json.Unmarshal(raw, &got.body); err != nil {
		return rawAnswer{}, err
	}
	return got, nil
}
