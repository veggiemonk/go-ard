package ard

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"
)

func mustContext(t *testing.T, raw string) Context {
	t.Helper()
	if !json.Valid([]byte(raw)) {
		t.Fatalf("the test context %s is not JSON", raw)
	}
	return RawContext(json.RawMessage(raw))
}

func mustResolver(t *testing.T, contexts ...Context) *TermResolver {
	t.Helper()
	r, err := NewTermResolver(contexts...)
	if err != nil {
		t.Fatalf("NewTermResolver gave the error %v", err)
	}
	return r
}

func TestResolveBaseContextTerms(t *testing.T) {
	r := mustResolver(t)
	cases := []struct {
		name string
		term string
		want string
	}{
		{"identifier", TermIdentifier, DefaultNamespace + "identifier"},
		{"displayName", TermDisplayName, DefaultNamespace + "displayName"},
		{"type does not resolve to itself but to mediaType", TermType, DefaultNamespace + "mediaType"},
		{"url", TermURL, DefaultNamespace + "url"},
		{"data", TermData, DefaultNamespace + "data"},
		{"representativeQueries", TermRepresentativeQueries, DefaultNamespace + "representativeQueries"},
		{"capabilities", TermCapabilities, DefaultNamespace + "capabilities"},
		{"tags", TermTags, DefaultNamespace + "tags"},
		{"description", TermDescription, DefaultNamespace + "description"},
		{"version", TermVersion, DefaultNamespace + "version"},
		{"updatedAt", TermUpdatedAt, DefaultNamespace + "updatedAt"},
		{"metadata", TermMetadata, DefaultNamespace + "metadata"},
		{"trustManifest", TermTrustManifest, DefaultNamespace + "trustManifest"},
		{"the ard prefix used as a term", "ard", DefaultNamespace},
		{"a compact iri under the ard prefix", "ard:mediaType", DefaultNamespace + "mediaType"},
		{"an unbound term falls through to the vocab", "somethingNew", DefaultNamespace + "somethingNew"},
		{"the derived publisher key", TermPublisher, DefaultNamespace + "publisher"},
		{"an absolute iri resolves to itself", "https://example.com/ns#thing", "https://example.com/ns#thing"},
		{"an http absolute iri resolves to itself", "http://example.com/ns#thing", "http://example.com/ns#thing"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := r.Resolve(c.term)
			if !ok {
				t.Fatalf("Resolve(%q) reported no binding", c.term)
			}
			if got != c.want {
				t.Errorf("Resolve(%q) = %q, want %q", c.term, got, c.want)
			}
		})
	}
}

func TestResolveTheIDAlias(t *testing.T) {
	got, ok := mustResolver(t).Resolve("id")
	if !ok {
		t.Fatal("Resolve(\"id\") reported no binding")
	}
	if got != TermID {
		t.Errorf("Resolve(\"id\") = %q, want %q", got, TermID)
	}
}

func TestResolveReportsNoBinding(t *testing.T) {
	r := mustResolver(t)
	cases := []struct {
		name string
		term string
	}{
		{"the empty term", ""},
		{"an unbound prefix", "okf:taxonomy"},
		{"another unbound prefix", "acme:serviceTier"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got, ok := r.Resolve(c.term); ok {
				t.Errorf("Resolve(%q) = %q, want no binding", c.term, got)
			}
		})
	}
}

func TestResolveTheSection44Examples(t *testing.T) {
	t.Run("the plain entry with no context", func(t *testing.T) {
		plain := Entry{
			Identifier:            "urn:air:acme.com:server:weather",
			DisplayName:           "Weather Data Node",
			Type:                  MediaTypeMCPServerCard,
			URL:                   "https://api.acme.com/mcp/weather.json",
			Capabilities:          []string{"WeatherTool", "ForecastTool"},
			Description:           "Enterprise weather MCP server for live telemetry.",
			RepresentativeQueries: []string{"what is the current wind speed in Chicago", "get the 5-day forecast for Seattle"},
		}
		r := mustResolver(t, plain.Context)
		for _, term := range []string{TermIdentifier, TermDisplayName, TermType, TermURL, TermCapabilities, TermDescription, TermRepresentativeQueries} {
			if _, ok := r.Resolve(term); !ok {
				t.Errorf("Resolve(%q) reported no binding", term)
			}
		}
		if got, _ := r.Resolve(TermType); got != DefaultNamespace+"mediaType" {
			t.Errorf("Resolve(type) = %q, want %q", got, DefaultNamespace+"mediaType")
		}
	})

	t.Run("the entry enriched with a publisher namespace", func(t *testing.T) {
		enriched := Entry{
			Context:     mustContext(t, `{"acme": "https://acme.com/vocab#"}`),
			Identifier:  "urn:air:acme.com:server:weather",
			DisplayName: "Weather Data Node",
			Type:        MediaTypeMCPServerCard,
			URL:         "https://api.acme.com/mcp/weather.json",
			Extra: map[string]json.RawMessage{
				"acme:serviceTier": json.RawMessage(`"enterprise"`),
				"acme:region":      json.RawMessage(`["us-east","eu-west"]`),
			},
		}
		r := mustResolver(t, enriched.Context)
		cases := []struct {
			term string
			want string
		}{
			{TermType, DefaultNamespace + "mediaType"},
			{TermCapabilities, DefaultNamespace + "capabilities"},
			{"acme:serviceTier", "https://acme.com/vocab#serviceTier"},
			{"acme:region", "https://acme.com/vocab#region"},
		}
		for _, c := range cases {
			got, ok := r.Resolve(c.term)
			if !ok {
				t.Fatalf("Resolve(%q) reported no binding", c.term)
			}
			if got != c.want {
				t.Errorf("Resolve(%q) = %q, want %q", c.term, got, c.want)
			}
		}
	})

	t.Run("the skill entry from a solo developer", func(t *testing.T) {
		skill := Entry{
			Identifier:  "urn:air:github.com:alice-dev:pptx-creator",
			DisplayName: "pptx-creator",
			Type:        MediaTypeAISkill,
			URL:         "https://github.com/alice-dev/pptx-creator",
		}
		r := mustResolver(t, skill.Context)
		if got, ok := r.Resolve(TermType); !ok || got != DefaultNamespace+"mediaType" {
			t.Errorf("Resolve(type) = %q %v, want %q true", got, ok, DefaultNamespace+"mediaType")
		}
	})
}

func TestResolvePathTheSection531FilterKeys(t *testing.T) {
	query := Query{
		Context: mustContext(t, `{"okf": "https://openknowledgeformat.org/ns#"}`),
		Text:    "find me a flight booking agent",
		Filter: map[string][]string{
			TermType:                          {MediaTypeA2AAgentCard},
			TermTags:                          {"finance"},
			"okf:taxonomy":                    {"us-gaap"},
			"trustManifest.attestations.type": {"SOC2-Type2"},
		},
	}
	r := mustResolver(t, query.Context)
	cases := []struct {
		name string
		key  string
		iri  string
		term string
		rest []string
	}{
		{"a core term", TermType, DefaultNamespace + "mediaType", TermType, nil},
		{"a core set term", TermTags, DefaultNamespace + "tags", TermTags, nil},
		{"a namespaced term", "okf:taxonomy", openKnowledgeNS + "taxonomy", "okf:taxonomy", nil},
		{"a path into a non expanded member", "trustManifest.attestations.type", DefaultNamespace + "trustManifest", TermTrustManifest, []string{"attestations", "type"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := r.ResolvePath(c.key)
			if err != nil {
				t.Fatalf("ResolvePath(%q) gave the error %v", c.key, err)
			}
			if got.Key != c.key {
				t.Errorf("Key = %q, want %q", got.Key, c.key)
			}
			if got.IRI != c.iri {
				t.Errorf("IRI = %q, want %q", got.IRI, c.iri)
			}
			if got.Term != c.term {
				t.Errorf("Term = %q, want %q", got.Term, c.term)
			}
			if !slices.Equal(got.Rest, c.rest) {
				t.Errorf("Rest = %v, want %v", got.Rest, c.rest)
			}
		})
	}
	for key := range query.Filter {
		if _, err := r.ResolvePath(key); err != nil {
			t.Errorf("ResolvePath(%q) gave the error %v", key, err)
		}
	}
}

func TestResolvePathNonExpandedMembers(t *testing.T) {
	r := mustResolver(t)
	cases := []struct {
		name string
		key  string
		iri  string
		term string
		rest []string
	}{
		{"a metadata path", "metadata.location", DefaultNamespace + "metadata", TermMetadata, []string{"location"}},
		{"a deep metadata path", "metadata.region.code", DefaultNamespace + "metadata", TermMetadata, []string{"region", "code"}},
		{"a trust manifest identity path", "trustManifest.identity", DefaultNamespace + "trustManifest", TermTrustManifest, []string{"identity"}},
		{"a data path", "data.title", DefaultNamespace + "data", TermData, []string{"title"}},
		{"a dotted path whose leading term is expanded", "capabilities.name", DefaultNamespace + "capabilities", TermCapabilities, []string{"name"}},
		{"a dotted path under the vocab", "custom.nested", DefaultNamespace + "custom", "custom", []string{"nested"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := r.ResolvePath(c.key)
			if err != nil {
				t.Fatalf("ResolvePath(%q) gave the error %v", c.key, err)
			}
			if got.IRI != c.iri || got.Term != c.term || !slices.Equal(got.Rest, c.rest) {
				t.Errorf("ResolvePath(%q) = {IRI:%q Term:%q Rest:%v}, want {IRI:%q Term:%q Rest:%v}", c.key, got.IRI, got.Term, got.Rest, c.iri, c.term, c.rest)
			}
		})
	}
}

func TestResolvePathRejects(t *testing.T) {
	r := mustResolver(t)
	cases := []struct {
		name string
		key  string
	}{
		{"an empty key", ""},
		{"a leading dot", ".type"},
		{"a trailing dot", "trustManifest."},
		{"a double dot", "trustManifest..type"},
		{"an unbound prefix", "okf:taxonomy"},
		{"an unbound prefix in a path", "okf:taxonomy.code"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := r.ResolvePath(c.key)
			if err == nil {
				t.Fatalf("ResolvePath(%q) = %+v, want an error", c.key, got)
			}
		})
	}
}

func TestResolvePathKeepsAnAbsoluteIRIWhole(t *testing.T) {
	r := mustResolver(t)
	const key = "https://openknowledgeformat.org/ns#taxonomy"
	got, err := r.ResolvePath(key)
	if err != nil {
		t.Fatalf("ResolvePath(%q) gave the error %v", key, err)
	}
	if got.IRI != key || got.Term != key || len(got.Rest) != 0 {
		t.Errorf("ResolvePath(%q) = %+v, want the whole key as one term", key, got)
	}
}

func TestCrossPublisherPrefixesMeetAtOneIRI(t *testing.T) {
	publisher := mustResolver(t, mustContext(t, `{"okf": "https://openknowledgeformat.org/ns#"}`))
	client := mustResolver(t, mustContext(t, `{"openknowledge": "https://openknowledgeformat.org/ns#"}`))

	fromPublisher, ok := publisher.Resolve("okf:taxonomy")
	if !ok {
		t.Fatal("the publisher resolver reported no binding for okf:taxonomy")
	}
	fromClient, ok := client.Resolve("openknowledge:taxonomy")
	if !ok {
		t.Fatal("the client resolver reported no binding for openknowledge:taxonomy")
	}
	if fromPublisher != fromClient {
		t.Errorf("okf:taxonomy resolved to %q and openknowledge:taxonomy to %q, want one IRI", fromPublisher, fromClient)
	}
	if fromPublisher != openKnowledgeNS+"taxonomy" {
		t.Errorf("okf:taxonomy = %q, want %q", fromPublisher, openKnowledgeNS+"taxonomy")
	}
	if _, ok := publisher.Resolve("openknowledge:taxonomy"); ok {
		t.Error("the publisher resolver bound a prefix that its context does not declare")
	}
}

func TestCrossPublisherPathsMeetAtOneIRI(t *testing.T) {
	publisher := mustResolver(t, mustContext(t, `{"okf": "https://openknowledgeformat.org/ns#"}`))
	client := mustResolver(t, mustContext(t, `{"openknowledge": "https://openknowledgeformat.org/ns#"}`))

	left, err := publisher.ResolvePath("okf:taxonomy")
	if err != nil {
		t.Fatalf("ResolvePath gave the error %v", err)
	}
	right, err := client.ResolvePath("openknowledge:taxonomy")
	if err != nil {
		t.Fatalf("ResolvePath gave the error %v", err)
	}
	if left.IRI != right.IRI {
		t.Errorf("the two prefixes gave %q and %q, want one IRI", left.IRI, right.IRI)
	}
}

func TestTheBaseContextAlwaysAppliesFirst(t *testing.T) {
	t.Run("a local context adds without removing the base", func(t *testing.T) {
		r := mustResolver(t, mustContext(t, `{"okf": "https://openknowledgeformat.org/ns#"}`))
		if got, ok := r.Resolve(TermType); !ok || got != DefaultNamespace+"mediaType" {
			t.Errorf("Resolve(type) = %q %v, want the base binding", got, ok)
		}
	})

	t.Run("a local context may override a base term", func(t *testing.T) {
		r := mustResolver(t, mustContext(t, `{"type": "https://schema.org/encodingFormat"}`))
		if got, _ := r.Resolve(TermType); got != "https://schema.org/encodingFormat" {
			t.Errorf("Resolve(type) = %q, want the overriding binding", got)
		}
		if got, _ := r.Resolve(TermIdentifier); got != DefaultNamespace+"identifier" {
			t.Errorf("Resolve(identifier) = %q, want the base binding", got)
		}
	})

	t.Run("a local context may override the vocab", func(t *testing.T) {
		r := mustResolver(t, mustContext(t, `{"@vocab": "https://example.com/ns#"}`))
		if got, _ := r.Resolve("somethingNew"); got != "https://example.com/ns#somethingNew" {
			t.Errorf("Resolve(somethingNew) = %q, want the overriding vocab", got)
		}
	})

	t.Run("a local context may reuse the ard prefix of the base", func(t *testing.T) {
		r := mustResolver(t, mustContext(t, `{"score": "ard:score"}`))
		if got, _ := r.Resolve(TermScore); got != DefaultNamespace+"score" {
			t.Errorf("Resolve(score) = %q, want %q", got, DefaultNamespace+"score")
		}
	})

	t.Run("the base context named as a string changes nothing", func(t *testing.T) {
		r := mustResolver(t, mustContext(t, `"https://agenticresourcediscovery.org/context/v1"`))
		if got, _ := r.Resolve(TermType); got != DefaultNamespace+"mediaType" {
			t.Errorf("Resolve(type) = %q, want the base binding", got)
		}
	})

	t.Run("an array that names the base context and adds a namespace", func(t *testing.T) {
		r := mustResolver(t, mustContext(t, `["https://agenticresourcediscovery.org/context/v1", {"okf": "https://openknowledgeformat.org/ns#"}]`))
		if got, _ := r.Resolve(TermType); got != DefaultNamespace+"mediaType" {
			t.Errorf("Resolve(type) = %q, want the base binding", got)
		}
		if got, _ := r.Resolve("okf:taxonomy"); got != openKnowledgeNS+"taxonomy" {
			t.Errorf("Resolve(okf:taxonomy) = %q, want %q", got, openKnowledgeNS+"taxonomy")
		}
	})
}

func TestLaterContextsWin(t *testing.T) {
	r := mustResolver(t,
		mustContext(t, `{"ex": "https://first.example/ns#"}`),
		mustContext(t, `{"ex": "https://second.example/ns#"}`),
	)
	if got, _ := r.Resolve("ex:thing"); got != "https://second.example/ns#thing" {
		t.Errorf("Resolve(ex:thing) = %q, want the later binding", got)
	}
}

func TestATermDefinitionWithNoIDUsesTheVocab(t *testing.T) {
	r := mustResolver(t, mustContext(t, `{"rating": {"@type": "http://www.w3.org/2001/XMLSchema#integer"}}`))
	if got, ok := r.Resolve("rating"); !ok || got != DefaultNamespace+"rating" {
		t.Errorf("Resolve(rating) = %q %v, want %q true", got, ok, DefaultNamespace+"rating")
	}
}

func TestANullTermDefinitionRemovesTheBinding(t *testing.T) {
	r := mustResolver(t, mustContext(t, `{"tags": null}`))
	if got, _ := r.Resolve(TermTags); got != DefaultNamespace+"tags" {
		t.Errorf("Resolve(tags) = %q, want the vocab fallback %q", got, DefaultNamespace+"tags")
	}
}

func TestNewTermResolverRejectsABadContext(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"a number", `42`},
		{"a boolean", `false`},
		{"an array that holds a number", `[{"ex": "https://example.com/ns#"}, 7]`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := NewTermResolver(RawContext(json.RawMessage(c.raw)))
			if err == nil {
				t.Fatalf("NewTermResolver = %+v, want an error", got)
			}
		})
	}
}

func TestRemoteContextsWithoutALoader(t *testing.T) {
	r := mustResolver(t, mustContext(t, `"https://openknowledgeformat.org/context/v1"`))
	if got, ok := r.Resolve("okf:taxonomy"); ok {
		t.Errorf("Resolve(okf:taxonomy) = %q, want no binding without a loader", got)
	}
	if got, _ := r.Resolve(TermType); got != DefaultNamespace+"mediaType" {
		t.Errorf("Resolve(type) = %q, want the base binding", got)
	}
}

func TestWithLoaderBindsARemoteContext(t *testing.T) {
	const remote = "https://openknowledgeformat.org/context/v1"
	fetched := 0
	loader := func(iri string) (json.RawMessage, error) {
		if iri != remote {
			return nil, errors.New("unknown context")
		}
		fetched++
		return json.RawMessage(`{"@context": {"okf": "https://openknowledgeformat.org/ns#"}}`), nil
	}
	r := mustResolver(t, mustContext(t, `"`+remote+`"`)).WithLoader(loader)
	if fetched == 0 {
		t.Fatal("WithLoader did not use the loader")
	}
	if got, ok := r.Resolve("okf:taxonomy"); !ok || got != openKnowledgeNS+"taxonomy" {
		t.Errorf("Resolve(okf:taxonomy) = %q %v, want %q true", got, ok, openKnowledgeNS+"taxonomy")
	}
	if got, _ := r.Resolve(TermType); got != DefaultNamespace+"mediaType" {
		t.Errorf("Resolve(type) = %q, want the base binding", got)
	}
}

func TestWithLoaderNeverFetchesTheBaseContext(t *testing.T) {
	loader := func(iri string) (json.RawMessage, error) {
		t.Errorf("the loader was asked for %q, want the embedded copy of the base context", iri)
		return nil, errors.New("no network in this package")
	}
	r := mustResolver(t, mustContext(t, `"`+BaseContextIRI+`"`)).WithLoader(loader)
	if got, _ := r.Resolve(TermType); got != DefaultNamespace+"mediaType" {
		t.Errorf("Resolve(type) = %q, want the base binding", got)
	}
}

func TestWithLoaderKeepsTheBaseWhenTheFetchFails(t *testing.T) {
	loader := func(iri string) (json.RawMessage, error) { return nil, errors.New("offline") }
	r := mustResolver(t, mustContext(t, `"https://example.com/context/v1"`)).WithLoader(loader)
	if got, _ := r.Resolve(TermType); got != DefaultNamespace+"mediaType" {
		t.Errorf("Resolve(type) = %q, want the base binding", got)
	}
	if got, ok := r.Resolve("ex:thing"); ok {
		t.Errorf("Resolve(ex:thing) = %q, want no binding after a failed fetch", got)
	}
}

func TestALoaderCycleTerminates(t *testing.T) {
	loader := func(iri string) (json.RawMessage, error) {
		return json.RawMessage(`"https://example.com/context/v1"`), nil
	}
	r := mustResolver(t, mustContext(t, `"https://example.com/context/v1"`)).WithLoader(loader)
	if got, _ := r.Resolve(TermType); got != DefaultNamespace+"mediaType" {
		t.Errorf("Resolve(type) = %q, want the base binding", got)
	}
}
