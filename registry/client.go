package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	ard "github.com/veggiemonk/go-ard"
)

// The routes of the registry REST API. See section 5.3.
const (
	RouteSearch  = "/search"
	RouteExplore = "/explore"
	RouteAgents  = "/agents"
)

const maxAnswerBytes = 32 << 20

// Client calls the registry REST API of section 5.3.
type Client struct {
	// BaseURL addresses the registry, with or without a path prefix and a trailing
	// slash. It also accepts a full route URL, such as the url of a referral, which
	// names the search route of the referred registry. See section 5.4.
	BaseURL string

	// HTTPClient sends the requests. A nil client sends them with http.DefaultClient.
	HTTPClient *http.Client

	// Header goes on every request, as a private registry needs.
	Header http.Header
}

// ListOptions are the query parameters of GET /agents. See section 5.3.4 and appendix A.
type ListOptions struct {
	Filter    string
	OrderBy   string
	PageSize  int
	PageToken string
}

// Search calls POST /search. See section 5.3.2.
func (c *Client) Search(ctx context.Context, req ard.SearchRequest) (ard.SearchResponse, error) {
	return post[ard.SearchResponse](ctx, c, RouteSearch, req)
}

// Explore calls POST /explore. A registry that does not implement it answers 501, and
// the error satisfies errors.Is(err, ard.ErrNotImplemented). See section 5.3.3.
func (c *Client) Explore(ctx context.Context, req ard.ExploreRequest) (ard.ExploreResponse, error) {
	return post[ard.ExploreResponse](ctx, c, RouteExplore, req)
}

// List calls GET /agents. See section 5.3.4.
func (c *Client) List(ctx context.Context, opt ListOptions) (ard.ListResponse, error) {
	endpoint, err := c.endpoint(RouteAgents, opt.values())
	if err != nil {
		return ard.ListResponse{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ard.ListResponse{}, fmt.Errorf("ard: build the request for %s: %w", endpoint, err)
	}
	c.decorate(request)
	return send[ard.ListResponse](c, request)
}

// SearchAll walks the pages of a search, following the page token until the registry
// returns none. It stops on the first error and on a cancelled context.
func (c *Client) SearchAll(ctx context.Context, req ard.SearchRequest) iter.Seq2[ard.Result, error] {
	return func(yield func(ard.Result, error) bool) {
		seen := map[string]bool{}
		for {
			if err := ctx.Err(); err != nil {
				yield(ard.Result{}, err)
				return
			}
			page, err := c.Search(ctx, req)
			if err != nil {
				yield(ard.Result{}, err)
				return
			}
			for _, result := range page.Results {
				if !yield(result, nil) {
					return
				}
			}
			if page.PageToken == "" || seen[page.PageToken] {
				return
			}
			seen[page.PageToken] = true
			req.PageToken = page.PageToken
		}
	}
}

func (opt ListOptions) values() url.Values {
	values := url.Values{}
	if opt.Filter != "" {
		values.Set("filter", opt.Filter)
	}
	if opt.OrderBy != "" {
		values.Set("orderBy", opt.OrderBy)
	}
	if opt.PageSize > 0 {
		values.Set("pageSize", strconv.Itoa(opt.PageSize))
	}
	if opt.PageToken != "" {
		values.Set("pageToken", opt.PageToken)
	}
	return values
}

func (c *Client) endpoint(route string, query url.Values) (string, error) {
	base, err := url.Parse(strings.TrimSpace(c.BaseURL))
	if err != nil {
		return "", fmt.Errorf("ard: the registry base URL %q is not a URL: %w", c.BaseURL, err)
	}
	if base.Scheme == "" || base.Host == "" {
		return "", fmt.Errorf("ard: the registry base URL %q names no scheme and host", c.BaseURL)
	}
	joined := *base
	joined.Path = path.Join("/", trimRoute(base.Path), route)
	joined.RawPath = ""
	if len(query) > 0 {
		joined.RawQuery = query.Encode()
	}
	return joined.String(), nil
}

// trimRoute removes a route of section 5.3 that the base URL already carries.
//
// A Client is built from two kinds of URL. A person names the registry itself, and a
// referral names the search route of the referred registry: the OpenAPI describes
// RegistryReferral.url as the "endpoint URL for the referred registry's search route".
// Without this, a client built from a referral asks for /search/search and gets 404.
//
// A registry that is genuinely mounted under a path that ends in one of the three route
// names is unreachable this way. That trade is deliberate: a referral is common and such
// a mount point is not.
func trimRoute(prefix string) string {
	clean := path.Join("/", prefix)
	for _, route := range []string{RouteSearch, RouteExplore, RouteAgents} {
		if clean == route || strings.HasSuffix(clean, route) {
			return strings.TrimSuffix(clean, route)
		}
	}
	return clean
}

func (c *Client) decorate(request *http.Request) {
	request.Header.Set("Accept", "application/json")
	for name, values := range c.Header {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

func post[T any](ctx context.Context, c *Client, route string, body any) (T, error) {
	var zero T
	encoded, err := json.Marshal(body)
	if err != nil {
		return zero, fmt.Errorf("ard: encode the request body for %s: %w", route, err)
	}
	endpoint, err := c.endpoint(route, nil)
	if err != nil {
		return zero, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return zero, fmt.Errorf("ard: build the request for %s: %w", endpoint, err)
	}
	request.Header.Set("Content-Type", "application/json")
	c.decorate(request)
	return send[T](c, request)
}

func send[T any](c *Client, request *http.Request) (T, error) {
	var zero T
	answer, err := c.httpClient().Do(request)
	if err != nil {
		return zero, fmt.Errorf("ard: call %s: %w", request.URL, err)
	}
	defer answer.Body.Close()
	body, err := io.ReadAll(io.LimitReader(answer.Body, maxAnswerBytes))
	if err != nil {
		return zero, fmt.Errorf("ard: read the answer of %s: %w", request.URL, err)
	}
	if answer.StatusCode != http.StatusOK {
		return zero, answerError(answer.StatusCode, body)
	}
	var decoded T
	if err := json.Unmarshal(body, &decoded); err != nil {
		return zero, fmt.Errorf("ard: decode the answer of %s: %w", request.URL, err)
	}
	return decoded, nil
}

func answerError(status int, body []byte) error {
	var payload ard.ErrorBody
	if err := json.Unmarshal(body, &payload); err == nil && payload.ErrorCode != "" {
		return ard.NewAPIError(status, payload.ErrorCode, payload.Message)
	}
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = http.StatusText(status)
	}
	return ard.NewAPIError(status, "", message)
}
