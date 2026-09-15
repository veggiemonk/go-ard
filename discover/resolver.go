package discover

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	ard "github.com/veggiemonk/go-ard"
)

// SourceKind names the discovery mechanism of section 5.1 that gave the entries.
type SourceKind string

// The discovery mechanisms of section 5.1.
const (
	SourceWellKnown   SourceKind = "well-known"
	SourcePredecessor SourceKind = "predecessor"
	SourceLink        SourceKind = "link"
	SourceInPage      SourceKind = "in-page"
	SourceRobots      SourceKind = "robots"
)

// WellKnownPath is the path a consumer MUST fetch. See section 5.1.
const WellKnownPath = "/.well-known/ard.json"

// PredecessorPath is the well-known path of the predecessor specification.
const PredecessorPath = "/.well-known/ai-catalog.json"

// DefaultTimeout is the timeout of the HTTP client this package builds for itself.
const DefaultTimeout = 30 * time.Second

// DefaultMaxBytes is the size limit this package puts on a response body.
const DefaultMaxBytes int64 = 8 << 20

// DefaultUserAgent is the user agent this package sends when the caller names none.
const DefaultUserAgent = "go-ard/0.91"

// Codes that a Warning of this package carries.
const (
	WarnWellKnownUnreachable = "well_known_unreachable"
	WarnPredecessorPath      = "predecessor_path"
	WarnPredecessorRelation  = "predecessor_relation"
	WarnUnreadableJSONLD     = "unreadable_json_ld"
)

// Failures that this package reports for a domain or a body it cannot use.
var (
	ErrEmptyDomain       = errors.New("discover: domain is empty")
	ErrDomainHasPath     = errors.New("discover: domain holds a path, a query or a fragment")
	ErrUnsupportedScheme = errors.New("discover: scheme is neither http nor https")
	ErrBodyTooLarge      = errors.New("discover: body is longer than the byte limit")
)

var defaultClient = &http.Client{Timeout: DefaultTimeout}

// Warning is a finding that does not stop a resolution.
type Warning struct {
	Code    string
	Message string
	URL     string
}

// Result is what a resolution found: the mechanism that answered and what it gave.
type Result struct {
	Kind     SourceKind
	URL      string
	Manifest ard.Manifest
	Entries  []ard.Entry
	Warnings []Warning
}

// Attempt records one fetch of a resolution and the reason it gave no manifest.
type Attempt struct {
	URL string
	Err error
}

// ResolveError reports that no path gave a manifest for a domain.
type ResolveError struct {
	Domain   string
	Attempts []Attempt
}

func (e *ResolveError) Error() string {
	reasons := make([]string, 0, len(e.Attempts))
	for _, a := range e.Attempts {
		reasons = append(reasons, a.Err.Error())
	}
	return fmt.Sprintf("discover: no manifest for %q: %s", e.Domain, strings.Join(reasons, "; "))
}

// Unwrap gives the failure of every attempt, so that errors.Is and errors.As reach them.
func (e *ResolveError) Unwrap() []error {
	out := make([]error, 0, len(e.Attempts))
	for _, a := range e.Attempts {
		out = append(out, a.Err)
	}
	return out
}

// StatusError reports an HTTP answer that is not 200.
type StatusError struct {
	URL        string
	StatusCode int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("discover: %s: HTTP %d %s", e.URL, e.StatusCode, http.StatusText(e.StatusCode))
}

// Resolver performs the consumer resolution of section 5.1.
type Resolver struct {
	// HTTPClient fetches every document. A nil client means a client with DefaultTimeout.
	HTTPClient *http.Client

	// ConsultPredecessor also fetches PredecessorPath when WellKnownPath gives no
	// manifest. NewResolver turns it on, as the reference tool does; the zero Resolver
	// leaves it off and is still fully conformant.
	ConsultPredecessor bool

	// UserAgent goes on every request. An empty value means DefaultUserAgent.
	UserAgent string

	// MaxBytes caps the body of every answer. A value of zero or less means
	// DefaultMaxBytes.
	MaxBytes int64
}

// NewResolver gives a Resolver with the defaults of this package, predecessor path included.
func NewResolver() *Resolver { return &Resolver{ConsultPredecessor: true} }

// Resolve fetches the entries that a domain publishes, the way section 5.1 tells a consumer to.
func (r *Resolver) Resolve(ctx context.Context, domain string) (*Result, error) {
	origin, err := Origin(domain)
	if err != nil {
		return nil, err
	}
	wellKnown := origin.JoinPath(WellKnownPath).String()
	manifest, wellKnownErr := r.fetchManifest(ctx, wellKnown)
	if wellKnownErr == nil {
		return newResult(SourceWellKnown, wellKnown, manifest, nil), nil
	}
	attempts := []Attempt{{URL: wellKnown, Err: wellKnownErr}}
	if !r.ConsultPredecessor || ctx.Err() != nil {
		return nil, &ResolveError{Domain: domain, Attempts: attempts}
	}
	predecessor := origin.JoinPath(PredecessorPath).String()
	manifest, predecessorErr := r.fetchManifest(ctx, predecessor)
	if predecessorErr != nil {
		attempts = append(attempts, Attempt{URL: predecessor, Err: predecessorErr})
		return nil, &ResolveError{Domain: domain, Attempts: attempts}
	}
	return newResult(SourcePredecessor, predecessor, manifest, predecessorWarnings(wellKnown, wellKnownErr, predecessor)), nil
}

// FetchManifest reads and decodes the manifest at one explicit URL.
func (r *Resolver) FetchManifest(ctx context.Context, manifestURL string) (ard.Manifest, error) {
	if !strings.Contains(manifestURL, "://") {
		manifestURL = "https://" + manifestURL
	}
	return r.fetchManifest(ctx, manifestURL)
}

func (r *Resolver) fetchManifest(ctx context.Context, rawURL string) (ard.Manifest, error) {
	body, err := r.get(ctx, rawURL)
	if err != nil {
		return ard.Manifest{}, err
	}
	var manifest ard.Manifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return ard.Manifest{}, fmt.Errorf("discover: %s: %w", rawURL, err)
	}
	return manifest, nil
}

func (r *Resolver) get(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("discover: %s: %w", rawURL, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", r.userAgent())
	resp, err := r.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("discover: %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &StatusError{URL: rawURL, StatusCode: resp.StatusCode}
	}
	limit := r.maxBytes()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("discover: %s: %w", rawURL, err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("discover: %s: %w of %d", rawURL, ErrBodyTooLarge, limit)
	}
	return body, nil
}

func (r *Resolver) client() *http.Client {
	if r.HTTPClient != nil {
		return r.HTTPClient
	}
	return defaultClient
}

func (r *Resolver) userAgent() string {
	if r.UserAgent != "" {
		return r.UserAgent
	}
	return DefaultUserAgent
}

func (r *Resolver) maxBytes() int64 {
	if r.MaxBytes > 0 {
		return r.MaxBytes
	}
	return DefaultMaxBytes
}

// Resolve resolves a domain with a Resolver that holds the defaults of this package.
func Resolve(ctx context.Context, domain string) (*Result, error) {
	return NewResolver().Resolve(ctx, domain)
}

// FetchManifest reads the manifest at one explicit URL with the defaults of this package.
func FetchManifest(ctx context.Context, manifestURL string) (ard.Manifest, error) {
	return NewResolver().FetchManifest(ctx, manifestURL)
}

// Origin turns a bare domain, a host with a port or a full URL into the origin to resolve.
func Origin(domain string) (*url.URL, error) {
	target := strings.TrimSpace(domain)
	if target == "" {
		return nil, fmt.Errorf("%w", ErrEmptyDomain)
	}
	if !strings.Contains(target, "://") {
		target = "https://" + target
	}
	parsed, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("discover: %q: %w", domain, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("discover: %q: %w", domain, ErrUnsupportedScheme)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("discover: %q: %w", domain, ErrEmptyDomain)
	}
	if strings.Trim(parsed.Path, "/") != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("discover: %q: %w", domain, ErrDomainHasPath)
	}
	return &url.URL{Scheme: parsed.Scheme, User: parsed.User, Host: parsed.Host}, nil
}

func newResult(kind SourceKind, source string, manifest ard.Manifest, warnings []Warning) *Result {
	return &Result{
		Kind:     kind,
		URL:      source,
		Manifest: manifest,
		Entries:  manifest.Entries,
		Warnings: warnings,
	}
}

func predecessorWarnings(wellKnown string, wellKnownErr error, predecessor string) []Warning {
	return []Warning{
		{
			Code:    WarnWellKnownUnreachable,
			Message: fmt.Sprintf("no manifest at the required path: %v", wellKnownErr),
			URL:     wellKnown,
		},
		{
			Code:    WarnPredecessorPath,
			Message: "reachable only at the predecessor path; consulting it is optional for a consumer (section 5.1), so this publisher may not be found. Serve the manifest at " + WellKnownPath + " as well.",
			URL:     predecessor,
		},
	}
}
