package discover

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

func manifestFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile("../testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return body
}

type recordingServer struct {
	*httptest.Server
	mu   sync.Mutex
	seen []string
	head http.Header
}

func newRecordingServer(t *testing.T, bodies map[string]func(http.ResponseWriter)) *recordingServer {
	t.Helper()
	server := &recordingServer{}
	server.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		server.mu.Lock()
		server.seen = append(server.seen, r.URL.Path)
		server.head = r.Header.Clone()
		server.mu.Unlock()
		write, ok := bodies[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		write(w)
	}))
	t.Cleanup(server.Close)
	return server
}

func (s *recordingServer) paths() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.seen...)
}

func (s *recordingServer) header(name string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.head.Get(name)
}

func serveBytes(body []byte) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}
}

func serveStatus(status int) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) { w.WriteHeader(status) }
}

func warningCodes(warnings []Warning) []string {
	codes := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		codes = append(codes, warning.Code)
	}
	return codes
}

func TestResolveFindsTheWellKnownManifest(t *testing.T) {
	manifest := manifestFixture(t, "basic-ard.json")
	server := newRecordingServer(t, map[string]func(http.ResponseWriter){
		WellKnownPath: serveBytes(manifest),
	})

	result, err := NewResolver().Resolve(t.Context(), server.URL)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result.Kind != SourceWellKnown {
		t.Errorf("Kind = %q, want %q", result.Kind, SourceWellKnown)
	}
	if result.URL != server.URL+WellKnownPath {
		t.Errorf("URL = %q, want %q", result.URL, server.URL+WellKnownPath)
	}
	if len(result.Entries) == 0 {
		t.Error("Entries is empty")
	}
	if len(result.Entries) != len(result.Manifest.Entries) {
		t.Errorf("Entries has %d, Manifest.Entries has %d", len(result.Entries), len(result.Manifest.Entries))
	}
	if len(result.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none", warningCodes(result.Warnings))
	}
	if got := server.paths(); len(got) != 1 || got[0] != WellKnownPath {
		t.Errorf("requested %v, want only %q", got, WellKnownPath)
	}
}

func TestResolveFallsBackToThePredecessorPathAndWarns(t *testing.T) {
	manifest := manifestFixture(t, "basic-ard.json")
	server := newRecordingServer(t, map[string]func(http.ResponseWriter){
		PredecessorPath: serveBytes(manifest),
	})

	result, err := NewResolver().Resolve(t.Context(), server.URL)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result.Kind != SourcePredecessor {
		t.Errorf("Kind = %q, want %q", result.Kind, SourcePredecessor)
	}
	if result.URL != server.URL+PredecessorPath {
		t.Errorf("URL = %q, want %q", result.URL, server.URL+PredecessorPath)
	}
	if len(result.Entries) == 0 {
		t.Error("Entries is empty")
	}
	want := []string{WarnWellKnownUnreachable, WarnPredecessorPath}
	got := warningCodes(result.Warnings)
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("warning codes = %v, want %v", got, want)
	}
	if result.Warnings[1].URL != server.URL+PredecessorPath {
		t.Errorf("predecessor warning URL = %q", result.Warnings[1].URL)
	}
}

func TestResolveFailsWhenNeitherPathHasAManifest(t *testing.T) {
	server := newRecordingServer(t, nil)

	_, err := NewResolver().Resolve(t.Context(), server.URL)
	var resolveErr *ResolveError
	if !errors.As(err, &resolveErr) {
		t.Fatalf("error = %v, want *ResolveError", err)
	}
	if len(resolveErr.Attempts) != 2 {
		t.Fatalf("Attempts = %v, want two", resolveErr.Attempts)
	}
	if got := server.paths(); len(got) != 2 {
		t.Errorf("requested %v, want both paths", got)
	}
	var statusErr *StatusError
	if !errors.As(err, &statusErr) || statusErr.StatusCode != http.StatusNotFound {
		t.Errorf("error does not carry a 404 StatusError: %v", err)
	}
}

func TestResolveDoesNotConsultThePredecessorWhenSwitchedOff(t *testing.T) {
	manifest := manifestFixture(t, "basic-ard.json")
	server := newRecordingServer(t, map[string]func(http.ResponseWriter){
		PredecessorPath: serveBytes(manifest),
	})

	resolver := &Resolver{}
	_, err := resolver.Resolve(t.Context(), server.URL)
	var resolveErr *ResolveError
	if !errors.As(err, &resolveErr) {
		t.Fatalf("error = %v, want *ResolveError", err)
	}
	if len(resolveErr.Attempts) != 1 {
		t.Fatalf("Attempts = %v, want one", resolveErr.Attempts)
	}
	if got := server.paths(); len(got) != 1 || got[0] != WellKnownPath {
		t.Errorf("requested %v, want only %q", got, WellKnownPath)
	}
}

func TestResolveReportsAServerError(t *testing.T) {
	server := newRecordingServer(t, map[string]func(http.ResponseWriter){
		WellKnownPath: serveStatus(http.StatusInternalServerError),
	})

	_, err := (&Resolver{}).Resolve(t.Context(), server.URL)
	var statusErr *StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("error = %v, want *StatusError", err)
	}
	if statusErr.StatusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want 500", statusErr.StatusCode)
	}
}

func TestResolveReportsABodyThatIsNotJSON(t *testing.T) {
	server := newRecordingServer(t, map[string]func(http.ResponseWriter){
		WellKnownPath: serveBytes([]byte("<html><body>not here</body></html>")),
	})

	_, err := (&Resolver{}).Resolve(t.Context(), server.URL)
	if err == nil {
		t.Fatal("Resolve succeeded on an HTML body")
	}
	if !strings.Contains(err.Error(), WellKnownPath) {
		t.Errorf("error does not name the path: %v", err)
	}
}

func TestResolveReportsABodyThatIsAJSONArray(t *testing.T) {
	server := newRecordingServer(t, map[string]func(http.ResponseWriter){
		WellKnownPath: serveBytes([]byte(`[{"identifier":"urn:air:example.com:agent:a"}]`)),
	})

	if _, err := (&Resolver{}).Resolve(t.Context(), server.URL); err == nil {
		t.Fatal("Resolve accepted a top-level array as a manifest")
	}
}

func TestResolveReportsABodyLongerThanMaxBytes(t *testing.T) {
	manifest := manifestFixture(t, "basic-ard.json")
	server := newRecordingServer(t, map[string]func(http.ResponseWriter){
		WellKnownPath: serveBytes(manifest),
	})

	resolver := &Resolver{MaxBytes: 32}
	_, err := resolver.Resolve(t.Context(), server.URL)
	if !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("error = %v, want ErrBodyTooLarge", err)
	}
}

func TestResolveStopsOnACancelledContext(t *testing.T) {
	server := newRecordingServer(t, map[string]func(http.ResponseWriter){
		WellKnownPath: serveBytes(manifestFixture(t, "basic-ard.json")),
	})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := NewResolver().Resolve(ctx, server.URL)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if got := server.paths(); len(got) != 0 {
		t.Errorf("requested %v on a cancelled context, want nothing", got)
	}
}

func TestResolveSendsTheUserAgent(t *testing.T) {
	server := newRecordingServer(t, map[string]func(http.ResponseWriter){
		WellKnownPath: serveBytes(manifestFixture(t, "basic-ard.json")),
	})

	if _, err := (&Resolver{}).Resolve(t.Context(), server.URL); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := server.header("User-Agent"); got != DefaultUserAgent {
		t.Errorf("User-Agent = %q, want %q", got, DefaultUserAgent)
	}

	named := &Resolver{UserAgent: "crawler/2"}
	if _, err := named.Resolve(t.Context(), server.URL); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := server.header("User-Agent"); got != "crawler/2" {
		t.Errorf("User-Agent = %q, want %q", got, "crawler/2")
	}
}

func TestResolveFollowsARedirect(t *testing.T) {
	manifest := manifestFixture(t, "basic-ard.json")
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveBytes(manifest)(w)
	}))
	t.Cleanup(final.Close)
	server := newRecordingServer(t, map[string]func(http.ResponseWriter){
		WellKnownPath: func(w http.ResponseWriter) {
			w.Header().Set("Location", final.URL+WellKnownPath)
			w.WriteHeader(http.StatusMovedPermanently)
		},
	})

	result, err := (&Resolver{}).Resolve(t.Context(), server.URL)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(result.Entries) == 0 {
		t.Error("Entries is empty after a redirect")
	}
}

func TestFetchManifestReadsOneExplicitURL(t *testing.T) {
	manifest := manifestFixture(t, "local-business-catalog.json")
	server := newRecordingServer(t, map[string]func(http.ResponseWriter){
		"/catalog/ard.json": serveBytes(manifest),
	})

	got, err := (&Resolver{}).FetchManifest(t.Context(), server.URL+"/catalog/ard.json")
	if err != nil {
		t.Fatalf("FetchManifest: %v", err)
	}
	if len(got.Entries) == 0 {
		t.Error("Entries is empty")
	}
}

func TestFetchManifestReportsAMissingDocument(t *testing.T) {
	server := newRecordingServer(t, nil)

	_, err := (&Resolver{}).FetchManifest(t.Context(), server.URL+"/nothing.json")
	var statusErr *StatusError
	if !errors.As(err, &statusErr) || statusErr.StatusCode != http.StatusNotFound {
		t.Fatalf("error = %v, want a 404 StatusError", err)
	}
}

func TestOrigin(t *testing.T) {
	cases := []struct {
		name   string
		domain string
		want   string
		err    error
	}{
		{name: "a bare domain becomes an https origin", domain: "example.com", want: "https://example.com"},
		{name: "a host with a port keeps the port", domain: "example.com:8443", want: "https://example.com:8443"},
		{name: "a full https url is kept", domain: "https://example.com", want: "https://example.com"},
		{name: "a full http url keeps its scheme", domain: "http://127.0.0.1:9010", want: "http://127.0.0.1:9010"},
		{name: "a trailing slash is not a path", domain: "example.com/", want: "https://example.com"},
		{name: "surrounding space is trimmed", domain: "  example.com  ", want: "https://example.com"},
		{name: "an empty domain is rejected", domain: "", err: ErrEmptyDomain},
		{name: "a domain of only space is rejected", domain: "   ", err: ErrEmptyDomain},
		{name: "a path-only value is rejected", domain: "/entries.json", err: ErrEmptyDomain},
		{name: "a domain with a path is rejected", domain: "example.com/catalog", err: ErrDomainHasPath},
		{name: "a url with a path is rejected", domain: "https://example.com/.well-known/ard.json", err: ErrDomainHasPath},
		{name: "a domain with a query is rejected", domain: "example.com?a=1", err: ErrDomainHasPath},
		{name: "a domain with a fragment is rejected", domain: "example.com#top", err: ErrDomainHasPath},
		{name: "another scheme is rejected", domain: "ftp://example.com", err: ErrUnsupportedScheme},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got, err := Origin(test.domain)
			if test.err != nil {
				if !errors.Is(err, test.err) {
					t.Fatalf("error = %v, want %v", err, test.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Origin: %v", err)
			}
			if got.String() != test.want {
				t.Errorf("Origin = %q, want %q", got, test.want)
			}
		})
	}
}

func TestOriginBuildsTheWellKnownURL(t *testing.T) {
	origin, err := Origin("example.com")
	if err != nil {
		t.Fatalf("Origin: %v", err)
	}
	if got := origin.JoinPath(WellKnownPath).String(); got != "https://example.com/.well-known/ard.json" {
		t.Errorf("well-known URL = %q", got)
	}
}

func TestResolveErrorNamesEveryAttempt(t *testing.T) {
	first := &StatusError{URL: "https://example.com" + WellKnownPath, StatusCode: 404}
	second := &StatusError{URL: "https://example.com" + PredecessorPath, StatusCode: 500}
	err := &ResolveError{Domain: "example.com", Attempts: []Attempt{{URL: first.URL, Err: first}, {URL: second.URL, Err: second}}}

	if !strings.Contains(err.Error(), WellKnownPath) || !strings.Contains(err.Error(), PredecessorPath) {
		t.Errorf("message does not name both paths: %v", err)
	}
	if !errors.Is(err, error(second)) {
		t.Error("errors.Is does not reach the second attempt")
	}
}

func TestPackageResolveUsesTheDefaults(t *testing.T) {
	server := newRecordingServer(t, map[string]func(http.ResponseWriter){
		PredecessorPath: serveBytes(manifestFixture(t, "basic-ard.json")),
	})

	result, err := Resolve(t.Context(), server.URL)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result.Kind != SourcePredecessor {
		t.Errorf("Kind = %q, want %q", result.Kind, SourcePredecessor)
	}
}

func TestResolveRejectsABadDomainBeforeAnyRequest(t *testing.T) {
	if _, err := NewResolver().Resolve(t.Context(), "example.com/catalog"); !errors.Is(err, ErrDomainHasPath) {
		t.Fatalf("error = %v, want ErrDomainHasPath", err)
	}
}
