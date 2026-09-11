package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	ard "github.com/veggiemonk/go-ard"
	"github.com/veggiemonk/go-ard/discover"
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func exercise(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(t.Context(), args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestValidateAcceptsEveryExampleManifest(t *testing.T) {
	manifests, err := filepath.Glob("../../testdata/*-ard.json")
	if err != nil {
		t.Fatalf("glob the manifests: %v", err)
	}
	catalogs, err := filepath.Glob("../../testdata/*-catalog.json")
	if err != nil {
		t.Fatalf("glob the catalogs: %v", err)
	}
	manifests = append(manifests, catalogs...)
	if len(manifests) == 0 {
		t.Fatal("the test data holds no manifest")
	}

	for _, path := range manifests {
		t.Run(filepath.Base(path), func(t *testing.T) {
			code, out, errOut := exercise(t, "validate", path)
			if code != 0 {
				t.Fatalf("exit code %d, want 0\nstdout:\n%s\nstderr:\n%s", code, out, errOut)
			}
			if !strings.Contains(out, "PASS") {
				t.Errorf("the output carries no PASS:\n%s", out)
			}
		})
	}
}

func TestValidateRefusesADocumentThatDoesNotConform(t *testing.T) {
	squatted := `{"identifier":"urn:air:acme.com:server:weather","displayName":"W",` +
		`"type":"application/mcp-server-card+json","url":"https://acme.com/w.json",` +
		`"trustManifest":{"identity":"spiffe://evil.com/x"}}`
	path := filepath.Join(t.TempDir(), "squatted.json")
	if err := os.WriteFile(path, []byte(squatted), 0o600); err != nil {
		t.Fatalf("write the entry: %v", err)
	}

	code, out, errOut := exercise(t, "validate", path)
	if code != 1 {
		t.Fatalf("exit code %d, want 1\nstdout:\n%s", code, out)
	}
	if !strings.Contains(errOut, "FAIL") {
		t.Errorf("the error output carries no FAIL:\n%s", errOut)
	}
}

func TestValidateRefusesAFileThatIsNotThere(t *testing.T) {
	code, _, errOut := exercise(t, "validate", filepath.Join(t.TempDir(), "absent.json"))
	if code != 1 {
		t.Fatalf("exit code %d, want 1", code)
	}
	if !strings.Contains(errOut, "absent.json") {
		t.Errorf("the error output names no file:\n%s", errOut)
	}
}

func TestValidateReadsAManifestOverHTTP(t *testing.T) {
	server := httptest.NewServer(manifestHandler(t))
	defer server.Close()

	code, out, errOut := exercise(t, "validate", server.URL+discover.WellKnownPath)
	if code != 0 {
		t.Fatalf("exit code %d, want 0\nstdout:\n%s\nstderr:\n%s", code, out, errOut)
	}
	if !strings.Contains(out, "a manifest of 1 entries") {
		t.Errorf("the output does not name the kind:\n%s", out)
	}
}

func TestResolveReadsThePublisherWellKnownPath(t *testing.T) {
	server := httptest.NewServer(manifestHandler(t))
	defer server.Close()

	code, out, errOut := exercise(t, "resolve", server.URL)
	if code != 0 {
		t.Fatalf("exit code %d, want 0\nstdout:\n%s\nstderr:\n%s", code, out, errOut)
	}
	if !strings.Contains(out, "the source gave 1 entries") {
		t.Errorf("the output does not count the entries:\n%s", out)
	}
}

func TestResolveReportsADomainThatPublishesNothing(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	code, _, errOut := exercise(t, "resolve", server.URL)
	if code != 1 {
		t.Fatalf("exit code %d, want 1", code)
	}
	if !strings.Contains(errOut, "no manifest") {
		t.Errorf("the error output does not say that no path answered:\n%s", errOut)
	}
}

func TestServeAndProbeAgreeOverTheExampleManifest(t *testing.T) {
	address, wait := serveManifest(t, "../../testdata/basic-ard.json")
	defer wait()

	code, out, errOut := exercise(t, "probe", "http://"+address)
	if code != 0 {
		t.Fatalf("exit code %d, want 0\nstdout:\n%s\nstderr:\n%s", code, out, errOut)
	}
	for _, want := range []string{"GET /agents", "POST /search", "POST /explore", "the registry conforms"} {
		if !strings.Contains(out, want) {
			t.Errorf("the probe did not report %q:\n%s", want, out)
		}
	}
}

func TestProbeRefusesARegistryThatDoesNotAnswer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	code, out, errOut := exercise(t, "probe", server.URL)
	if code != 1 {
		t.Fatalf("exit code %d, want 1\nstdout:\n%s", code, out)
	}
	if !strings.Contains(errOut, "probe(s) did not conform") {
		t.Errorf("the error output does not count the probes:\n%s", errOut)
	}
}

func TestProbeSendsTheHeadersOfTheCommandLine(t *testing.T) {
	held := make(chan string, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		held <- r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	exercise(t, "probe", "-header", "Authorization: Bearer token", server.URL)
	select {
	case got := <-held:
		if got != "Bearer token" {
			t.Errorf("Authorization = %q, want the token of the command line", got)
		}
	default:
		t.Fatal("the registry saw no request")
	}
}

func TestServeRefusesAManifestItCannotRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rubbish.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatalf("write the file: %v", err)
	}

	code, _, errOut := exercise(t, "serve", path, "-addr", "127.0.0.1:0")
	if code != 1 {
		t.Fatalf("exit code %d, want 1", code)
	}
	if !strings.Contains(errOut, "no manifest") {
		t.Errorf("the error output does not name the kind:\n%s", errOut)
	}
}

func TestTheUsageDecidesTheExitCode(t *testing.T) {
	cases := map[string]struct {
		args []string
		want int
	}{
		"no argument":          {args: nil, want: 2},
		"an unknown command":   {args: []string{"lint"}, want: 2},
		"the help":             {args: []string{"help"}, want: 0},
		"the help flag":        {args: []string{"-h"}, want: 0},
		"validate without one": {args: []string{"validate"}, want: 1},
		"an unknown flag":      {args: []string{"probe", "-loud", "http://localhost"}, want: 1},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			code, _, _ := exercise(t, c.args...)
			if code != c.want {
				t.Errorf("exit code %d, want %d", code, c.want)
			}
		})
	}
}

func manifestHandler(t *testing.T) http.Handler {
	t.Helper()
	manifest := ard.Manifest{Entries: []ard.Entry{{
		Identifier:  "urn:air:acme.com:server:weather",
		DisplayName: "Weather Data Node",
		Type:        ard.MediaTypeMCPServerCard,
		URL:         "https://api.acme.com/mcp/weather.json",
		UpdatedAt:   "2026-02-01T09:00:00Z",
	}}}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("encode the manifest: %v", err)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != discover.WellKnownPath {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/ld+json")
		_, _ = w.Write(body)
	})
}

func serveManifest(t *testing.T, path string) (string, func()) {
	t.Helper()
	ctx, stop := context.WithCancel(t.Context())
	out := &syncBuffer{}
	done := make(chan int, 1)
	go func() { done <- run(ctx, []string{"serve", path, "-addr", "127.0.0.1:0"}, out, out) }()

	address := ""
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && address == "" {
		address = listenAddress(out.String())
		if address == "" {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if address == "" {
		stop()
		t.Fatalf("the server named no address:\n%s", out.String())
	}
	return address, func() {
		stop()
		select {
		case code := <-done:
			if code != 0 {
				t.Errorf("serve gave the exit code %d, want 0:\n%s", code, out.String())
			}
		case <-time.After(10 * time.Second):
			t.Error("serve did not stop when the context was cancelled")
		}
	}
}

func listenAddress(output string) string {
	_, rest, found := strings.Cut(output, "entries on ")
	if !found {
		return ""
	}
	line, done, found := strings.Cut(rest, "\n")
	if !found || done == "" {
		return ""
	}
	return strings.TrimSpace(line)
}
