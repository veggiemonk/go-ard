package ard

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func decodeEntry(t *testing.T, document string) Entry {
	t.Helper()
	var entry Entry
	if err := json.Unmarshal([]byte(document), &entry); err != nil {
		t.Fatalf("decode entry: %v", err)
	}
	return entry
}

func decodeManifest(t *testing.T, document string) Manifest {
	t.Helper()
	var manifest Manifest
	if err := json.Unmarshal([]byte(document), &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	return manifest
}

func codesOf(issues []Issue) []string {
	codes := make([]string, 0, len(issues))
	for _, issue := range issues {
		codes = append(codes, issue.Code)
	}
	return codes
}

func findIssue(issues []Issue, code string) (Issue, bool) {
	for _, issue := range issues {
		if issue.Code == code {
			return issue, true
		}
	}
	return Issue{}, false
}

func requireIssue(t *testing.T, issues []Issue, code, path, section string) Issue {
	t.Helper()
	issue, ok := findIssue(issues, code)
	if !ok {
		t.Fatalf("no issue with code %q, got %v", code, codesOf(issues))
	}
	if issue.Path != path {
		t.Errorf("path = %q, want %q", issue.Path, path)
	}
	if issue.Section != section {
		t.Errorf("section = %q, want %q", issue.Section, section)
	}
	if issue.Message == "" {
		t.Errorf("issue %q carries no message", code)
	}
	return issue
}

func conformingEntry() Entry {
	return Entry{
		Identifier:  "urn:air:acme.com:server:weather",
		DisplayName: "Weather Data Node",
		Type:        "application/mcp-server-card+json",
		URL:         "https://api.acme.com/mcp/weather.json",
		RepresentativeQueries: []string{
			"what is the current wind speed in Chicago",
			"get the 5-day forecast for Seattle",
		},
	}
}

func TestValidateConformingEntryReportsNothing(t *testing.T) {
	report := Validate(conformingEntry())
	if !report.OK() {
		t.Errorf("errors = %v, want none", report.Errors)
	}
	if len(report.Warnings) != 0 {
		t.Errorf("warnings = %v, want none", report.Warnings)
	}
	if err := report.Err(); err != nil {
		t.Errorf("Err() = %v, want nil", err)
	}
}

func TestValidateRequiredTermsSection42(t *testing.T) {
	cases := []struct {
		name string
		term string
		edit func(*Entry)
	}{
		{"4.2 identifier is a MUST", TermIdentifier, func(e *Entry) { e.Identifier = "" }},
		{"4.2 displayName is a MUST", TermDisplayName, func(e *Entry) { e.DisplayName = "" }},
		{"4.2 type is a MUST", TermType, func(e *Entry) { e.Type = "" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			entry := conformingEntry()
			c.edit(&entry)
			report := Validate(entry)
			if report.OK() {
				t.Fatalf("the entry conforms, want a missing term error")
			}
			issue := requireIssue(t, report.Errors, IssueMissingTerm, c.term, sectionEntryTerms)
			if !strings.Contains(issue.Message, c.term) {
				t.Errorf("message %q does not name the term %q", issue.Message, c.term)
			}
		})
	}
}

func TestValidateAllThreeRequiredTermsAbsent(t *testing.T) {
	report := Validate(Entry{URL: "https://example.com/card.json", RepresentativeQueries: []string{"a", "b"}})
	wantPaths := []string{TermIdentifier, TermDisplayName, TermType}
	got := make([]string, 0, len(report.Errors))
	for _, issue := range report.Errors {
		if issue.Code != IssueMissingTerm {
			t.Errorf("code = %q, want only missing terms", issue.Code)
			continue
		}
		got = append(got, issue.Path)
	}
	if !equalStrings(got, wantPaths) {
		t.Errorf("missing terms = %v, want %v", got, wantPaths)
	}
}

// Appendix C names urn:air: alone. The library reads the predecessor prefix anyway and
// reports it as a warning, because the entry is otherwise sound. See docs/spec-findings.md.
func TestValidateWarnsOnThePredecessorURNPrefix(t *testing.T) {
	entry := conformingEntry()
	entry.Identifier = "urn:ai:acme.com:server:weather"

	report := Validate(entry)

	if _, failed := findIssue(report.Errors, IssueBadURN); failed {
		t.Error("the predecessor prefix gave an error, want a warning")
	}
	issue, warned := findIssue(report.Warnings, IssueLegacyURNPrefix)
	if !warned {
		t.Fatalf("the predecessor prefix gave no %q warning (warnings %v)", IssueLegacyURNPrefix, report.Warnings)
	}
	if !strings.Contains(issue.Message, "urn:air:acme.com:server:weather") {
		t.Errorf("the warning %q does not name the spelling a writer must emit", issue.Message)
	}
}

func TestValidateIdentifierURNAppendixC(t *testing.T) {
	cases := []struct {
		name       string
		identifier string
		wantError  bool
	}{
		{"appendix C publisher namespace agent", "urn:air:acme.com:server:weather", false},
		{"adr-0007 no namespace segment", "urn:air:acme.com:assistant", false},
		{"adr-0007 recursive namespace", "urn:air:acme.com:finance:trading:trader", false},
		{"appendix C rejects an http iri", "https://acme.com/agents/weather", true},
		{"the predecessor nid is read, and warned about", "urn:ai:acme.com:server:weather", false},
		{"appendix C rejects a bare name", "weather", true},
		{"appendix C rejects a publisher only", "urn:air:acme.com", true},
		{"appendix C rejects an empty publisher", "urn:air::server:weather", true},
		{"appendix C rejects an underscore in the publisher", "urn:air:acme_corp.com:server:weather", true},
		{"appendix C rejects a space in a segment", "urn:air:acme.com:server:weather node", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			entry := conformingEntry()
			entry.Identifier = c.identifier
			report := Validate(entry)
			_, got := findIssue(report.Errors, IssueBadURN)
			if got != c.wantError {
				t.Fatalf("bad URN error = %v, want %v (errors %v)", got, c.wantError, codesOf(report.Errors))
			}
			if !c.wantError {
				return
			}
			issue := requireIssue(t, report.Errors, IssueBadURN, TermIdentifier, sectionURN)
			if !strings.Contains(issue.Message, c.identifier) {
				t.Errorf("message %q does not name the identifier %q", issue.Message, c.identifier)
			}
		})
	}
}

func TestValidateValueOrReferenceSection43(t *testing.T) {
	cases := []struct {
		name      string
		document  string
		wantError bool
	}{
		{"4.3 url alone conforms", `{"identifier":"urn:air:acme.com:s:w","displayName":"W","type":"application/json","url":"https://acme.com/w.json","representativeQueries":["a","b"]}`, false},
		{"4.3 data alone conforms", `{"identifier":"urn:air:acme.com:s:w","displayName":"W","type":"application/json","data":{"name":"w"},"representativeQueries":["a","b"]}`, false},
		{"4.3 forbids both", `{"identifier":"urn:air:acme.com:s:w","displayName":"W","type":"application/json","url":"https://acme.com/w.json","data":{"name":"w"},"representativeQueries":["a","b"]}`, true},
		{"4.3 forbids neither", `{"identifier":"urn:air:acme.com:s:w","displayName":"W","type":"application/json","representativeQueries":["a","b"]}`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			report := Validate(decodeEntry(t, c.document))
			_, got := findIssue(report.Errors, IssueValueOrReference)
			if got != c.wantError {
				t.Fatalf("value or reference error = %v, want %v (errors %v)", got, c.wantError, codesOf(report.Errors))
			}
			if !c.wantError {
				return
			}
			issue := requireIssue(t, report.Errors, IssueValueOrReference, TermURL, sectionValueOrReference)
			if !strings.Contains(issue.Message, TermURL) || !strings.Contains(issue.Message, TermData) {
				t.Errorf("message %q does not name both terms", issue.Message)
			}
		})
	}
}

func TestValidateTrustManifestIdentitySection45(t *testing.T) {
	cases := []struct {
		name     string
		document string
		wantCode string
	}{
		{
			"4.5 an absent trustManifest conforms",
			`{"identifier":"urn:air:acme.com:s:w","displayName":"W","type":"application/json","url":"https://acme.com/w.json","representativeQueries":["a","b"]}`,
			"",
		},
		{
			"4.5 an empty identity is an error",
			`{"identifier":"urn:air:acme.com:s:w","displayName":"W","type":"application/json","url":"https://acme.com/w.json","representativeQueries":["a","b"],"trustManifest":{"identity":"","identityType":"spiffe"}}`,
			IssueMissingIdentity,
		},
		{
			"4.5 a trustManifest with no identity member is an error",
			`{"identifier":"urn:air:acme.com:s:w","displayName":"W","type":"application/json","url":"https://acme.com/w.json","representativeQueries":["a","b"],"trustManifest":{"identityType":"spiffe"}}`,
			IssueMissingIdentity,
		},
		{
			"4.5 a whitespace identity is an error",
			`{"identifier":"urn:air:acme.com:s:w","displayName":"W","type":"application/json","url":"https://acme.com/w.json","representativeQueries":["a","b"],"trustManifest":{"identity":"   "}}`,
			IssueMissingIdentity,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			report := Validate(decodeEntry(t, c.document))
			if c.wantCode == "" {
				if !report.OK() {
					t.Fatalf("errors = %v, want none", report.Errors)
				}
				return
			}
			requireIssue(t, report.Errors, c.wantCode, TermTrustManifest+"."+trustIdentityMember, sectionTrust)
		})
	}
}

func TestValidatePublisherAuthorityBindingSection451(t *testing.T) {
	cases := []struct {
		name          string
		identifier    string
		identity      string
		requireExact  bool
		wantMismatch  bool
		wantInMessage string
	}{
		{
			name:       "4.5.1 a spiffe id of the publisher binds",
			identifier: "urn:air:acme.com:server:weather",
			identity:   "spiffe://acme.com/ns/prod/sa/weather",
		},
		{
			name:       "4.5.1 a did:web of the publisher binds",
			identifier: "urn:air:fda.gov:api:drug-ndc",
			identity:   "did:web:fda.gov",
		},
		{
			name:       "4.5.1 an https uri of the publisher binds",
			identifier: "urn:air:noaa.gov:api:climate-data-online",
			identity:   "https://noaa.gov/.well-known/jwks.json",
		},
		{
			name:          "4.5.1 a foreign trust domain is namespace squatting",
			identifier:    "urn:air:acme.com:server:weather",
			identity:      "spiffe://evil.com/x",
			wantMismatch:  true,
			wantInMessage: "evil.com",
		},
		{
			name:       "4.5.1 a subdomain binds by default",
			identifier: "urn:air:acme.com:server:weather",
			identity:   "spiffe://mesh.acme.com/ns/prod/sa/weather",
		},
		{
			name:       "4.5.1 a deep subdomain binds by default",
			identifier: "urn:air:acme.com:server:weather",
			identity:   "spiffe://eu.prod.acme.com/ns/prod/sa/weather",
		},
		{
			name:          "4.5.1 a subdomain does not bind when the validator demands the exact domain",
			identifier:    "urn:air:acme.com:server:weather",
			identity:      "spiffe://mesh.acme.com/ns/prod/sa/weather",
			requireExact:  true,
			wantMismatch:  true,
			wantInMessage: "mesh.acme.com",
		},
		{
			name:          "4.5.1 a parent domain never binds",
			identifier:    "urn:air:mesh.acme.com:server:weather",
			identity:      "spiffe://acme.com/ns/prod/sa/weather",
			wantMismatch:  true,
			wantInMessage: "acme.com",
		},
		{
			name:          "4.5.1 a look alike domain never binds",
			identifier:    "urn:air:acme.com:server:weather",
			identity:      "spiffe://evilacme.com/x",
			wantMismatch:  true,
			wantInMessage: "evilacme.com",
		},
		{
			name:          "4.5.1 a bare top level domain publisher binds nothing under it",
			identifier:    "urn:air:com:server:weather",
			identity:      "spiffe://acme.com/ns/prod/sa/weather",
			wantMismatch:  true,
			wantInMessage: "acme.com",
		},
		{
			name:          "4.5.1 an identity that carries no domain is a mismatch",
			identifier:    "urn:air:acme.com:server:weather",
			identity:      "did:key:z6MkhaXgBZDvotDkL5257faiztiGiC2QtKLGpbnnEGta2doK",
			wantMismatch:  true,
			wantInMessage: "did:key",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			entry := conformingEntry()
			entry.Identifier = c.identifier
			entry.TrustManifest = &TrustManifest{Identity: c.identity}
			report := Validator{RequireExactAuthority: c.requireExact}.Entry(entry)
			_, got := findIssue(report.Errors, IssueAuthorityMismatch)
			if got != c.wantMismatch {
				t.Fatalf("authority mismatch = %v, want %v (errors %v)", got, c.wantMismatch, report.Errors)
			}
			if !c.wantMismatch {
				return
			}
			issue := requireIssue(t, report.Errors, IssueAuthorityMismatch, TermTrustManifest+"."+trustIdentityMember, sectionAuthorityBinding)
			if !strings.Contains(issue.Message, c.wantInMessage) {
				t.Errorf("message %q does not name %q", issue.Message, c.wantInMessage)
			}
		})
	}
}

func TestValidateSkipsAuthorityBindingWhenTheIdentifierIsNotAURN(t *testing.T) {
	entry := conformingEntry()
	entry.Identifier = "https://acme.com/agents/weather"
	entry.TrustManifest = &TrustManifest{Identity: "spiffe://evil.com/x"}
	report := Validate(entry)
	if _, found := findIssue(report.Errors, IssueAuthorityMismatch); found {
		t.Errorf("errors = %v, want no authority mismatch when the publisher segment is unknown", codesOf(report.Errors))
	}
	requireIssue(t, report.Errors, IssueBadURN, TermIdentifier, sectionURN)
}

func TestValidateRepresentativeQueriesSectionD2(t *testing.T) {
	cases := []struct {
		name     string
		document string
		wantCode string
	}{
		{"D.2 two queries conform", `{"identifier":"urn:air:acme.com:s:w","displayName":"W","type":"application/json","url":"https://acme.com/w.json","representativeQueries":["a","b"]}`, ""},
		{"D.2 five queries conform", `{"identifier":"urn:air:acme.com:s:w","displayName":"W","type":"application/json","url":"https://acme.com/w.json","representativeQueries":["a","b","c","d","e"]}`, ""},
		{"D.2 an absent term is a warning", `{"identifier":"urn:air:acme.com:s:w","displayName":"W","type":"application/json","url":"https://acme.com/w.json"}`, IssueNoQueries},
		{"D.2 an empty array is a count warning", `{"identifier":"urn:air:acme.com:s:w","displayName":"W","type":"application/json","url":"https://acme.com/w.json","representativeQueries":[]}`, IssueQueryCount},
		{"D.2 one query is a count warning", `{"identifier":"urn:air:acme.com:s:w","displayName":"W","type":"application/json","url":"https://acme.com/w.json","representativeQueries":["a"]}`, IssueQueryCount},
		{"D.2 six queries are a count warning", `{"identifier":"urn:air:acme.com:s:w","displayName":"W","type":"application/json","url":"https://acme.com/w.json","representativeQueries":["a","b","c","d","e","f"]}`, IssueQueryCount},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			report := Validate(decodeEntry(t, c.document))
			if !report.OK() {
				t.Fatalf("errors = %v, want none: D.2 keeps this a warning", report.Errors)
			}
			if c.wantCode == "" {
				if len(report.Warnings) != 0 {
					t.Fatalf("warnings = %v, want none", codesOf(report.Warnings))
				}
				return
			}
			requireIssue(t, report.Warnings, c.wantCode, TermRepresentativeQueries, sectionDiscoveryConstraints)
		})
	}
}

func TestValidateAbsentQueriesWarningExplainsTheLossOfSearch(t *testing.T) {
	entry := conformingEntry()
	entry.RepresentativeQueries = nil
	report := Validate(entry)
	issue := requireIssue(t, report.Warnings, IssueNoQueries, TermRepresentativeQueries, sectionDiscoveryConstraints)
	for _, want := range []string{"catalog entry", "search"} {
		if !strings.Contains(issue.Message, want) {
			t.Errorf("message %q does not say %q", issue.Message, want)
		}
	}
}

func TestValidateNodeIdentifierAppendixC(t *testing.T) {
	cases := []struct {
		name        string
		id          string
		wantWarning bool
	}{
		{"appendix C an absent @id conforms", "", false},
		{"appendix C an @id mirroring the identifier conforms", "urn:air:acme.com:server:weather", false},
		{"appendix C a differing @id is a warning", "https://acme.com/agents/other", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			entry := conformingEntry()
			entry.ID = c.id
			report := Validate(entry)
			if !report.OK() {
				t.Fatalf("errors = %v, want none: a difference cannot be proved wrong", report.Errors)
			}
			_, got := findIssue(report.Warnings, IssueIDMismatch)
			if got != c.wantWarning {
				t.Fatalf("id mismatch warning = %v, want %v", got, c.wantWarning)
			}
			if !c.wantWarning {
				return
			}
			issue := requireIssue(t, report.Warnings, IssueIDMismatch, TermID, sectionURN)
			if !strings.Contains(issue.Message, c.id) {
				t.Errorf("message %q does not name the @id %q", issue.Message, c.id)
			}
		})
	}
}

func TestValidateUpdatedAtTimestamp(t *testing.T) {
	cases := []struct {
		name        string
		updatedAt   string
		wantWarning bool
	}{
		{"D.1 an absent updatedAt conforms", "", false},
		{"D.1 an RFC 3339 instant in UTC conforms", "2025-11-04T10:30:00Z", false},
		{"D.1 an RFC 3339 instant with an offset conforms", "2025-11-04T10:30:00+01:00", false},
		{"D.1 a date alone is a warning", "2025-11-04", true},
		{"D.1 a timestamp with no zone is a warning", "2025-11-04T10:30:00", true},
		{"D.1 a free text date is a warning", "4 November 2025", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			entry := conformingEntry()
			entry.UpdatedAt = c.updatedAt
			report := Validate(entry)
			if !report.OK() {
				t.Fatalf("errors = %v, want none", report.Errors)
			}
			_, got := findIssue(report.Warnings, IssueBadTimestamp)
			if got != c.wantWarning {
				t.Fatalf("bad timestamp warning = %v, want %v", got, c.wantWarning)
			}
			if !c.wantWarning {
				return
			}
			issue := requireIssue(t, report.Warnings, IssueBadTimestamp, TermUpdatedAt, sectionEntrySchema)
			if !strings.Contains(issue.Message, c.updatedAt) {
				t.Errorf("message %q does not name the timestamp %q", issue.Message, c.updatedAt)
			}
		})
	}
}

func TestValidateEntryContextSection41(t *testing.T) {
	cases := []struct {
		name        string
		document    string
		wantWarning bool
	}{
		{"4.1 an absent @context conforms", `{"identifier":"urn:air:acme.com:s:w","displayName":"W","type":"application/json","url":"https://acme.com/w.json","representativeQueries":["a","b"]}`, false},
		{"4.1 the base context conforms", `{"@context":"https://agenticresourcediscovery.org/context/v1","identifier":"urn:air:acme.com:s:w","displayName":"W","type":"application/json","url":"https://acme.com/w.json","representativeQueries":["a","b"]}`, false},
		{"4.1 a prefix object conforms", `{"@context":{"acme":"https://acme.com/vocab#"},"identifier":"urn:air:acme.com:s:w","displayName":"W","type":"application/json","url":"https://acme.com/w.json","representativeQueries":["a","b"]}`, false},
		{"4.1 a number is not a context", `{"@context":7,"identifier":"urn:air:acme.com:s:w","displayName":"W","type":"application/json","url":"https://acme.com/w.json","representativeQueries":["a","b"]}`, true},
		{"4.1 a boolean is not a context", `{"@context":true,"identifier":"urn:air:acme.com:s:w","displayName":"W","type":"application/json","url":"https://acme.com/w.json","representativeQueries":["a","b"]}`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			report := Validate(decodeEntry(t, c.document))
			if !report.OK() {
				t.Fatalf("errors = %v, want none", report.Errors)
			}
			_, got := findIssue(report.Warnings, IssueBadContext)
			if got != c.wantWarning {
				t.Fatalf("bad context warning = %v, want %v (warnings %v)", got, c.wantWarning, codesOf(report.Warnings))
			}
			if c.wantWarning {
				requireIssue(t, report.Warnings, IssueBadContext, TermContext, sectionJSONLD)
			}
		})
	}
}

func TestValidateManifestPrefixesEveryPath(t *testing.T) {
	manifest := decodeManifest(t, `{"entries":[
		{"identifier":"urn:air:acme.com:s:ok","displayName":"OK","type":"application/json","url":"https://acme.com/a.json","representativeQueries":["a","b"]},
		{"identifier":"urn:air:acme.com:s:one","displayName":"One","type":"application/json","representativeQueries":["a","b"]},
		{"identifier":"not-a-urn","displayName":"Two","type":"application/json","url":"https://acme.com/c.json"}
	]}`)
	report := ValidateManifest(manifest)
	requireIssue(t, report.Errors, IssueValueOrReference, "entries[1].url", sectionValueOrReference)
	requireIssue(t, report.Errors, IssueBadURN, "entries[2].identifier", sectionURN)
	requireIssue(t, report.Warnings, IssueNoQueries, "entries[2].representativeQueries", sectionDiscoveryConstraints)
}

func TestValidateManifestDuplicateIdentifierAppendixC(t *testing.T) {
	manifest := decodeManifest(t, `{"entries":[
		{"identifier":"urn:air:acme.com:s:w","displayName":"First","type":"application/json","url":"https://acme.com/a.json","representativeQueries":["a","b"]},
		{"identifier":"urn:air:acme.com:s:other","displayName":"Other","type":"application/json","url":"https://acme.com/b.json","representativeQueries":["a","b"]},
		{"identifier":"urn:air:acme.com:s:w","displayName":"Second","type":"application/json","url":"https://acme.com/c.json","representativeQueries":["a","b"]}
	]}`)
	report := ValidateManifest(manifest)
	if !report.OK() {
		t.Fatalf("errors = %v, want none", report.Errors)
	}
	if len(report.Warnings) != 1 {
		t.Fatalf("warnings = %v, want one duplicate", codesOf(report.Warnings))
	}
	issue := requireIssue(t, report.Warnings, IssueDuplicateIdentifier, "entries[2].identifier", sectionURN)
	if !strings.Contains(issue.Message, "urn:air:acme.com:s:w") {
		t.Errorf("message %q does not name the identifier", issue.Message)
	}
}

func TestValidateManifestLegacyCollectionsADR0003(t *testing.T) {
	document := `{"collections":[{"name":"engineering"}],"entries":[
		{"identifier":"urn:air:acme.com:s:w","displayName":"W","type":"application/json","url":"https://acme.com/a.json","representativeQueries":["a","b"]}
	]}`
	report := ValidateManifest(decodeManifest(t, document))
	if !report.OK() {
		t.Fatalf("errors = %v, want none: ARD ignores unrecognized top-level members", report.Errors)
	}
	requireIssue(t, report.Warnings, IssueLegacyCollections, legacyCollectionsMember, sectionLegacyCollections)
}

func TestValidateManifestIgnoresOtherRootMembers(t *testing.T) {
	document := `{"host":"acme.com","specVersion":"0.91","entries":[
		{"identifier":"urn:air:acme.com:s:w","displayName":"W","type":"application/json","url":"https://acme.com/a.json","representativeQueries":["a","b"]}
	]}`
	report := ValidateManifest(decodeManifest(t, document))
	if !report.OK() || len(report.Warnings) != 0 {
		t.Errorf("report = %+v, want nothing: section 5.1 leaves other root members to the transport", report)
	}
}

func TestValidateManifestWithNoEntries(t *testing.T) {
	report := ValidateManifest(decodeManifest(t, `{"entries":[]}`))
	if !report.OK() || len(report.Warnings) != 0 {
		t.Errorf("report = %+v, want nothing", report)
	}
}

func TestReportErr(t *testing.T) {
	t.Run("no error gives nil", func(t *testing.T) {
		report := Report{Warnings: []Issue{{Path: TermRepresentativeQueries, Code: IssueNoQueries}}}
		if err := report.Err(); err != nil {
			t.Errorf("Err() = %v, want nil", err)
		}
	})

	t.Run("errors.As reaches an Issue", func(t *testing.T) {
		err := Validate(Entry{}).Err()
		if err == nil {
			t.Fatal("Err() = nil, want the errors of an empty entry")
		}
		var issue Issue
		if !errors.As(err, &issue) {
			t.Fatalf("errors.As did not reach an Issue in %v", err)
		}
		if issue.Code == "" || issue.Path == "" || issue.Section == "" {
			t.Errorf("issue = %+v, want a code, a path and a section", issue)
		}
	})

	t.Run("an unwrap to a slice reaches every issue", func(t *testing.T) {
		report := Validate(Entry{})
		err := report.Err()
		unwrapped, ok := err.(interface{ Unwrap() []error })
		if !ok {
			t.Fatalf("%T does not unwrap to a slice of errors", err)
		}
		if got := len(unwrapped.Unwrap()); got != len(report.Errors) {
			t.Errorf("unwrapped %d errors, want %d", got, len(report.Errors))
		}
	})

	t.Run("the message names the path and the section", func(t *testing.T) {
		issue := Issue{Path: "entries[2].url", Code: IssueValueOrReference, Section: sectionValueOrReference, Message: "both terms"}
		text := issue.Error()
		for _, want := range []string{"entries[2].url", IssueValueOrReference, sectionValueOrReference, "both terms"} {
			if !strings.Contains(text, want) {
				t.Errorf("Error() = %q, does not carry %q", text, want)
			}
		}
	})
}

func TestValidateSection44Examples(t *testing.T) {
	cases := []struct {
		name     string
		document string
	}{
		{
			"4.4 a plain entry with no @context",
			`{
			  "identifier": "urn:air:acme.com:server:weather",
			  "displayName": "Weather Data Node",
			  "type": "application/mcp-server-card+json",
			  "url": "https://api.acme.com/mcp/weather.json",
			  "capabilities": ["WeatherTool", "ForecastTool"],
			  "description": "Enterprise weather MCP server for live telemetry.",
			  "representativeQueries": [
			    "what is the current wind speed in Chicago",
			    "get the 5-day forecast for Seattle"
			  ]
			}`,
		},
		{
			"4.4 the same entry enriched with a publisher namespace",
			`{
			  "@context": { "acme": "https://acme.com/vocab#" },
			  "identifier": "urn:air:acme.com:server:weather",
			  "displayName": "Weather Data Node",
			  "type": "application/mcp-server-card+json",
			  "url": "https://api.acme.com/mcp/weather.json",
			  "capabilities": ["WeatherTool", "ForecastTool"],
			  "description": "Enterprise weather MCP server for live telemetry.",
			  "representativeQueries": [
			    "what is the current wind speed in Chicago",
			    "get the 5-day forecast for Seattle"
			  ],
			  "acme:serviceTier": "enterprise",
			  "acme:region": ["us-east", "eu-west"]
			}`,
		},
		{
			"4.4 a skill entry from a solo developer, no trust ceremony",
			`{
			  "identifier": "urn:air:github.com:alice-dev:pptx-creator",
			  "displayName": "pptx-creator",
			  "type": "application/ai-skill+md",
			  "url": "https://github.com/alice-dev/pptx-creator",
			  "description": "Create professional PowerPoint presentations following brand guidelines.",
			  "representativeQueries": [
			    "turn these bullet points into a branded slide deck",
			    "make a PowerPoint from this outline"
			  ]
			}`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			report := Validate(decodeEntry(t, c.document))
			if !report.OK() {
				t.Errorf("errors = %v, want none", report.Errors)
			}
			if len(report.Warnings) != 0 {
				t.Errorf("warnings = %v, want none", report.Warnings)
			}
		})
	}
}

func TestValidateSoloDeveloperEntryNeedsNoTrustManifest(t *testing.T) {
	entry := decodeEntry(t, `{
	  "identifier": "urn:air:github.com:alice-dev:pptx-creator",
	  "displayName": "pptx-creator",
	  "type": "application/ai-skill+md",
	  "url": "https://github.com/alice-dev/pptx-creator",
	  "representativeQueries": ["make a PowerPoint from this outline", "turn these bullets into slides"]
	}`)
	if entry.TrustManifest != nil {
		t.Fatalf("trustManifest = %+v, want none", entry.TrustManifest)
	}
	if err := Validate(entry).Err(); err != nil {
		t.Errorf("Err() = %v, want nil: section 4.5 makes the trust manifest optional", err)
	}
}

func TestValidateForeignTrustDomainOnAnAcmeIdentifier(t *testing.T) {
	entry := decodeEntry(t, `{
	  "identifier": "urn:air:acme.com:server:weather",
	  "displayName": "Weather Data Node",
	  "type": "application/mcp-server-card+json",
	  "url": "https://api.acme.com/mcp/weather.json",
	  "representativeQueries": ["what is the wind speed in Chicago", "get the forecast for Seattle"],
	  "trustManifest": { "identity": "spiffe://evil.com/x", "identityType": "spiffe" }
	}`)
	report := Validate(entry)
	issue := requireIssue(t, report.Errors, IssueAuthorityMismatch, TermTrustManifest+"."+trustIdentityMember, sectionAuthorityBinding)
	for _, want := range []string{"evil.com", "acme.com"} {
		if !strings.Contains(issue.Message, want) {
			t.Errorf("message %q does not name %q", issue.Message, want)
		}
	}
}

func TestValidateManifestTestdataAgreesWithTheReferenceTool(t *testing.T) {
	cases := []struct {
		name         string
		file         string
		wantErrors   []string
		wantWarnings []string
	}{
		{
			name:       "conformance/examples/basic/ard.json",
			file:       "basic-ard.json",
			wantErrors: nil,
			wantWarnings: []string{
				IssueNoQueries,
				IssueNoQueries,
			},
		},
		{
			name:         "conformance/examples/fda-ndc/fda-ndc-catalog.json",
			file:         "fda-ndc-catalog.json",
			wantErrors:   nil,
			wantWarnings: nil,
		},
		{
			name:         "conformance/examples/local-business/local-business-catalog.json",
			file:         "local-business-catalog.json",
			wantErrors:   nil,
			wantWarnings: nil,
		},
		{
			name:         "conformance/examples/noaa-weather/noaa-weather-catalog.json",
			file:         "noaa-weather-catalog.json",
			wantErrors:   nil,
			wantWarnings: nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata", c.file))
			if err != nil {
				t.Fatalf("read manifest: %v", err)
			}
			var manifest Manifest
			if err := json.Unmarshal(raw, &manifest); err != nil {
				t.Fatalf("decode manifest: %v", err)
			}
			report := ValidateManifest(manifest)
			if got := codesOf(report.Errors); !equalStrings(got, c.wantErrors) {
				t.Errorf("errors = %v, want %v (%v)", got, c.wantErrors, report.Errors)
			}
			if got := codesOf(report.Warnings); !equalStrings(got, c.wantWarnings) {
				t.Errorf("warnings = %v, want %v (%v)", got, c.wantWarnings, report.Warnings)
			}
			if c.wantErrors == nil && report.Err() != nil {
				t.Errorf("Err() = %v, want nil", report.Err())
			}
		})
	}
}

func TestValidateManifestBasicWarnsOnTheTwoEntriesWithoutQueries(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "basic-ard.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	report := ValidateManifest(manifest)
	wantPaths := []string{"entries[2].representativeQueries", "entries[3].representativeQueries"}
	got := make([]string, 0, len(report.Warnings))
	for _, issue := range report.Warnings {
		got = append(got, issue.Path)
	}
	if !equalStrings(got, wantPaths) {
		t.Errorf("warning paths = %v, want %v", got, wantPaths)
	}
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
