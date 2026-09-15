package discover

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

func mustBase(t *testing.T, raw string) *url.URL {
	t.Helper()
	base, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse base %q: %v", raw, err)
	}
	return base
}

func scan(t *testing.T, document string, base *url.URL) HTMLResult {
	t.Helper()
	result, err := ScanHTML(strings.NewReader(document), base)
	if err != nil {
		t.Fatalf("ScanHTML: %v", err)
	}
	return result
}

func linkURLs(links []Link) []string {
	out := make([]string, 0, len(links))
	for _, link := range links {
		out = append(out, link.URL)
	}
	return out
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestScanHTMLLinks(t *testing.T) {
	base := mustBase(t, "https://example.com/docs/page.html")
	cases := []struct {
		name        string
		document    string
		wantURLs    []string
		wantPredAt  []int
		wantWarning []string
	}{
		{
			name:     "a single rel token",
			document: `<head><link rel="ard" href="/.well-known/ard.json"></head>`,
			wantURLs: []string{"https://example.com/.well-known/ard.json"},
		},
		{
			name:     "several space separated rel tokens",
			document: `<link rel="alternate ARD preload" href="entries.json">`,
			wantURLs: []string{"https://example.com/docs/entries.json"},
		},
		{
			name:     "uppercase tag and attribute names",
			document: `<LINK REL="ARD" HREF="https://cdn.example.net/ard.json">`,
			wantURLs: []string{"https://cdn.example.net/ard.json"},
		},
		{
			name:     "single quoted attributes",
			document: `<link rel='ard' href='/a/ard.json'>`,
			wantURLs: []string{"https://example.com/a/ard.json"},
		},
		{
			name:     "unquoted attributes",
			document: `<link rel=ard href=/b/ard.json>`,
			wantURLs: []string{"https://example.com/b/ard.json"},
		},
		{
			name:     "a self closing tag",
			document: `<link rel="ard" href="/c/ard.json" />`,
			wantURLs: []string{"https://example.com/c/ard.json"},
		},
		{
			name:     "a relative href without a leading slash",
			document: `<link rel="ard" href="sub/ard.json">`,
			wantURLs: []string{"https://example.com/docs/sub/ard.json"},
		},
		{
			name:        "the predecessor relation is flagged",
			document:    `<link rel="ai-catalog" href="/.well-known/ai-catalog.json">`,
			wantURLs:    []string{"https://example.com/.well-known/ai-catalog.json"},
			wantPredAt:  []int{0},
			wantWarning: []string{WarnPredecessorRelation},
		},
		{
			name:       "both relations on one page",
			document:   `<link rel="ard" href="/one.json"><link rel="ai-catalog" href="/two.json">`,
			wantURLs:   []string{"https://example.com/one.json", "https://example.com/two.json"},
			wantPredAt: []int{1},
			wantWarning: []string{
				WarnPredecessorRelation,
			},
		},
		{
			name:     "an unrelated relation is ignored",
			document: `<link rel="stylesheet" href="/style.css">`,
		},
		{
			name:     "a link without an href is ignored",
			document: `<link rel="ard">`,
		},
		{
			name:     "a document without a head",
			document: `<html><body><p>hi</p><link rel="ard" href="/d/ard.json"></body></html>`,
			wantURLs: []string{"https://example.com/d/ard.json"},
		},
		{
			name:     "a link inside a comment is ignored",
			document: `<!-- <link rel="ard" href="/hidden.json"> --><link rel="ard" href="/real.json">`,
			wantURLs: []string{"https://example.com/real.json"},
		},
		{
			name:     "a link written inside a script is ignored",
			document: `<script>var s = '<link rel="ard" href="/hidden.json">';</script><link rel="ard" href="/real.json">`,
			wantURLs: []string{"https://example.com/real.json"},
		},
		{
			name:     "a truncated tag at the end of the document",
			document: `<link rel="ard" href="/good.json"><link rel="ard" href="/trunc`,
			wantURLs: []string{"https://example.com/good.json"},
		},
		{
			name:     "a truncated comment",
			document: `<link rel="ard" href="/good.json"><!-- never closed`,
			wantURLs: []string{"https://example.com/good.json"},
		},
		{
			name:     "a doctype and stray angle brackets",
			document: `<!DOCTYPE html><p>3 < 4 and 5 > 2</p><link rel=ard href=/e.json>`,
			wantURLs: []string{"https://example.com/e.json"},
		},
		{
			name:     "an attribute without a value",
			document: `<link disabled rel=ard href=/f.json>`,
			wantURLs: []string{"https://example.com/f.json"},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result := scan(t, test.document, base)
			if got := linkURLs(result.Links); !equalStrings(got, test.wantURLs) {
				t.Fatalf("link URLs = %v, want %v", got, test.wantURLs)
			}
			for index, link := range result.Links {
				want := false
				for _, predecessor := range test.wantPredAt {
					if predecessor == index {
						want = true
					}
				}
				if link.Predecessor != want {
					t.Errorf("link %d Predecessor = %v, want %v", index, link.Predecessor, want)
				}
			}
			if got := warningCodes(result.Warnings); !equalStrings(got, test.wantWarning) {
				t.Errorf("warning codes = %v, want %v", got, test.wantWarning)
			}
		})
	}
}

func TestScanHTMLKeepsAnHrefWhenThereIsNoBase(t *testing.T) {
	result := scan(t, `<link rel="ard" href="/.well-known/ard.json">`, nil)
	if got := linkURLs(result.Links); !equalStrings(got, []string{"/.well-known/ard.json"}) {
		t.Errorf("link URLs = %v", got)
	}
}

func TestScanHTMLJSONLD(t *testing.T) {
	base := mustBase(t, "https://example.com/")
	oneEntry := `{"identifier":"urn:air:example.com:agent:one","displayName":"One","url":"https://example.com/one.json"}`
	otherEntry := `{"identifier":"urn:air:example.com:agent:two","displayName":"Two","url":"https://example.com/two.json"}`
	cases := []struct {
		name            string
		document        string
		wantIdentifiers []string
		wantWarnings    []string
	}{
		{
			name:            "one entry object",
			document:        `<script type="application/ld+json">` + oneEntry + `</script>`,
			wantIdentifiers: []string{"urn:air:example.com:agent:one"},
		},
		{
			name:            "an array of entries",
			document:        `<script type="application/ld+json">[` + oneEntry + `,` + otherEntry + `]</script>`,
			wantIdentifiers: []string{"urn:air:example.com:agent:one", "urn:air:example.com:agent:two"},
		},
		{
			name:            "a manifest object",
			document:        `<script type="application/ld+json">{"specVersion":"1.0","entries":[` + oneEntry + `]}</script>`,
			wantIdentifiers: []string{"urn:air:example.com:agent:one"},
		},
		{
			name:            "unrelated json-ld is skipped",
			document:        `<script type="application/ld+json">{"@context":"https://schema.org","@type":"Organization","name":"Acme"}</script>`,
			wantIdentifiers: nil,
		},
		{
			name:            "an array holding unrelated json-ld is skipped",
			document:        `<script type="application/ld+json">[{"@type":"Person","name":"Ada"},` + oneEntry + `]</script>`,
			wantIdentifiers: []string{"urn:air:example.com:agent:one"},
		},
		{
			name:            "a plain script is not read",
			document:        `<script>{"identifier":"urn:air:example.com:agent:nope"}</script>`,
			wantIdentifiers: nil,
		},
		{
			name:            "the type parameter is ignored",
			document:        `<script type="application/ld+json; charset=utf-8">` + oneEntry + `</script>`,
			wantIdentifiers: []string{"urn:air:example.com:agent:one"},
		},
		{
			name:            "an uppercase script type",
			document:        `<SCRIPT TYPE="APPLICATION/LD+JSON">` + oneEntry + `</SCRIPT>`,
			wantIdentifiers: []string{"urn:air:example.com:agent:one"},
		},
		{
			name:            "several blocks",
			document:        `<script type="application/ld+json">` + oneEntry + `</script><p>x</p><script type="application/ld+json">` + otherEntry + `</script>`,
			wantIdentifiers: []string{"urn:air:example.com:agent:one", "urn:air:example.com:agent:two"},
		},
		{
			name:            "an empty block",
			document:        `<script type="application/ld+json">  </script>`,
			wantIdentifiers: nil,
		},
		{
			name:            "a block holding broken json warns",
			document:        `<script type="application/ld+json">{"identifier":</script>`,
			wantIdentifiers: nil,
			wantWarnings:    []string{WarnUnreadableJSONLD},
		},
		{
			name:            "a block holding a bare string warns",
			document:        `<script type="application/ld+json">"hello"</script>`,
			wantIdentifiers: nil,
			wantWarnings:    []string{WarnUnreadableJSONLD},
		},
		{
			name:            "a block that is never closed",
			document:        `<script type="application/ld+json">` + oneEntry,
			wantIdentifiers: []string{"urn:air:example.com:agent:one"},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result := scan(t, test.document, base)
			got := make([]string, 0, len(result.Entries))
			for _, entry := range result.Entries {
				got = append(got, entry.Identifier)
			}
			if !equalStrings(got, test.wantIdentifiers) {
				t.Fatalf("identifiers = %v, want %v", got, test.wantIdentifiers)
			}
			if codes := warningCodes(result.Warnings); !equalStrings(codes, test.wantWarnings) {
				t.Errorf("warning codes = %v, want %v", codes, test.wantWarnings)
			}
		})
	}
}

func TestScanHTMLReadsLinksAndEntriesTogether(t *testing.T) {
	document := `<!DOCTYPE html>
<html><head>
<link rel="ard" href="/.well-known/ard.json">
<script type="application/ld+json">
{"identifier":"urn:air:example.com:agent:one","displayName":"One","url":"https://example.com/one.json"}
</script>
</head><body><p>page</p></body></html>`

	result := scan(t, document, mustBase(t, "https://example.com/index.html"))
	if len(result.Links) != 1 || result.Links[0].URL != "https://example.com/.well-known/ard.json" {
		t.Errorf("links = %v", linkURLs(result.Links))
	}
	if len(result.Entries) != 1 || result.Entries[0].Identifier != "urn:air:example.com:agent:one" {
		t.Errorf("entries = %v", result.Entries)
	}
}

func TestScanHTMLReportsAReadFailure(t *testing.T) {
	want := errors.New("broken reader")
	if _, err := ScanHTML(failingReader{want}, nil); !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

type failingReader struct{ err error }

func (f failingReader) Read([]byte) (int, error) { return 0, f.err }

func TestScanHTMLTerminatesOnHostileInput(t *testing.T) {
	documents := []string{
		"<",
		"<<<<<<<<",
		"<link",
		"<link rel",
		"<link rel=",
		"<link rel=\"",
		"<link rel='ard' href=",
		"<link =value>",
		"<link ==== >",
		"</",
		"<!",
		"<!-",
		"<script type=application/ld+json",
		strings.Repeat("<link rel=ard href=/a.json>", 500),
		strings.Repeat("<", 1000),
	}
	for _, document := range documents {
		t.Run(document[:min(len(document), 24)], func(t *testing.T) {
			if _, err := ScanHTML(strings.NewReader(document), nil); err != nil {
				t.Fatalf("ScanHTML: %v", err)
			}
		})
	}
}
