package ard

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/veggiemonk/go-ard/internal/jsonx"
)

const plainEntry = `{
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
}`

const namespacedEntry = `{
  "@context": {
    "acme": "https://acme.com/vocab#"
  },
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
}`

const skillEntry = `{
  "identifier": "urn:air:github.com:alice-dev:pptx-creator",
  "displayName": "pptx-creator",
  "type": "application/ai-skill+md",
  "url": "https://github.com/alice-dev/pptx-creator",
  "description": "Create professional PowerPoint presentations following brand guidelines.",
  "representativeQueries": [
    "turn these bullet points into a branded slide deck",
    "make a PowerPoint from this outline"
  ]
}`

const searchResponse = `{
  "results": [
    {
      "identifier": "urn:air:acme.com:agent:assistant",
      "displayName": "Corporate Assistant (A2A)",
      "type": "application/a2a-agent-card+json",
      "url": "https://api.acme.com/agents/assistant.json",
      "score": 95,
      "source": "https://registry.acme.com/api/v1/"
    },
    {
      "identifier": "urn:air:example.com:weather-server",
      "displayName": "Global Weather Service",
      "type": "application/mcp-server-card+json",
      "url": "https://weather.example.com/mcp",
      "capabilities": ["WeatherTool"],
      "score": 88,
      "source": "https://finder.external.org/api/"
    }
  ],
  "referrals": [
    {
      "identifier": "urn:air:nlweb.ai:registry:public",
      "displayName": "Public Agent Finder",
      "type": "application/ai-registry+json",
      "url": "https://finder.nlweb.ai/search"
    }
  ],
  "pageToken": "eyJwYWdlIjogMn0="
}`

func requireSameDocument(t *testing.T, want, got []byte) {
	t.Helper()
	var wantValue, gotValue any
	if err := json.Unmarshal(want, &wantValue); err != nil {
		t.Fatalf("the wanted document does not decode: %v", err)
	}
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("the encoded document does not decode: %v\n%s", err, got)
	}
	if !reflect.DeepEqual(wantValue, gotValue) {
		t.Errorf("the encoded document differs\nwant: %s\ngot:  %s", want, got)
	}
}

func requireRoundTrip(t *testing.T, document string, value any) {
	t.Helper()
	if err := json.Unmarshal([]byte(document), value); err != nil {
		t.Fatalf("decode: %v", err)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	requireSameDocument(t, []byte(document), encoded)
}

func TestManifestRoundTripsEveryTestdataDocument(t *testing.T) {
	names, err := filepath.Glob(filepath.Join("testdata", "*-catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	names = append(names, filepath.Join("testdata", "basic-ard.json"))
	for _, name := range names {
		t.Run(filepath.Base(name), func(t *testing.T) {
			document, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			var manifest Manifest
			requireRoundTrip(t, string(document), &manifest)
			if len(manifest.Entries) == 0 {
				t.Fatal("the manifest carries no entry")
			}
			if _, ok := manifest.Extra["host"]; !ok {
				t.Error("the host member did not reach Extra")
			}
		})
	}
}

func TestEntryRoundTripsTheExamplesOfSection44(t *testing.T) {
	t.Run("plain entry", func(t *testing.T) {
		var entry Entry
		requireRoundTrip(t, plainEntry, &entry)
		if !entry.Context.IsZero() {
			t.Error("the entry carries no context")
		}
		if len(entry.Extra) != 0 {
			t.Errorf("no term belongs in Extra: %v", entry.Extra)
		}
	})
	t.Run("entry with a namespace extension", func(t *testing.T) {
		var entry Entry
		requireRoundTrip(t, namespacedEntry, &entry)
		if entry.Context.IsZero() {
			t.Fatal("the context is lost")
		}
		if string(entry.Extra["acme:serviceTier"]) != `"enterprise"` {
			t.Errorf("acme:serviceTier is %s", entry.Extra["acme:serviceTier"])
		}
		if string(entry.Extra["acme:region"]) != `["us-east", "eu-west"]` {
			t.Errorf("acme:region is %s", entry.Extra["acme:region"])
		}
	})
	t.Run("skill entry", func(t *testing.T) {
		var entry Entry
		requireRoundTrip(t, skillEntry, &entry)
		if entry.Type != MediaTypeAISkill {
			t.Errorf("the type is %q", entry.Type)
		}
	})
}

func TestSearchResponseRoundTripsTheExampleOfSection532(t *testing.T) {
	var response SearchResponse
	requireRoundTrip(t, searchResponse, &response)
	if len(response.Results) != 2 {
		t.Fatalf("the response carries %d results", len(response.Results))
	}
	first := response.Results[0]
	if first.Score == nil || *first.Score != 95 {
		t.Errorf("the score is %v", first.Score)
	}
	if first.Source != "https://registry.acme.com/api/v1/" {
		t.Errorf("the source is %q", first.Source)
	}
	if first.Entry.Identifier != "urn:air:acme.com:agent:assistant" {
		t.Errorf("the identifier is %q", first.Entry.Identifier)
	}
	if len(first.Entry.Extra) != 0 {
		t.Errorf("score and source do not belong in Extra: %v", first.Entry.Extra)
	}
}

func TestResultAcceptsAnEntryWithNoScoreAndNoSource(t *testing.T) {
	var result Result
	requireRoundTrip(t, `{"identifier": "urn:air:acme.com:agent:assistant"}`, &result)
	if result.Score != nil {
		t.Errorf("the score is %v", result.Score)
	}
	if result.Source != "" {
		t.Errorf("the source is %q", result.Source)
	}
}

func TestEntryCodecLeavesTheValueOrReferenceRuleToValidation(t *testing.T) {
	t.Run("url and data together", func(t *testing.T) {
		var entry Entry
		requireRoundTrip(t, `{
  "identifier": "urn:air:acme.com:server:weather",
  "displayName": "Weather Data Node",
  "type": "application/mcp-server-card+json",
  "url": "https://api.acme.com/mcp/weather.json",
  "data": {"name": "weather", "tools": []}
}`, &entry)
		if entry.URL == "" || entry.Data == nil {
			t.Error("the codec dropped one of the two terms")
		}
	})
	t.Run("neither url nor data", func(t *testing.T) {
		var entry Entry
		requireRoundTrip(t, `{
  "identifier": "urn:air:acme.com:server:weather",
  "displayName": "Weather Data Node",
  "type": "application/mcp-server-card+json"
}`, &entry)
	})
}

func TestEntryKeepsTheBytesOfData(t *testing.T) {
	const document = `{"identifier":"urn:air:acme.com:server:weather","data":{"tools":[{"name":"a"}],"n":1.50}}`
	var entry Entry
	if err := json.Unmarshal([]byte(document), &entry); err != nil {
		t.Fatal(err)
	}
	if string(entry.Data) != `{"tools":[{"name":"a"}],"n":1.50}` {
		t.Errorf("the data member is %s", entry.Data)
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(`"n":1.50`)) {
		t.Errorf("the encoder rewrote the number: %s", encoded)
	}
	requireSameDocument(t, []byte(document), encoded)
}

func TestMarshalReportsAnExtraTermThatCollidesWithANamedTerm(t *testing.T) {
	cases := map[string]any{
		"entry": Entry{
			Identifier: "urn:air:acme.com:server:weather",
			Extra:      map[string]json.RawMessage{TermURL: json.RawMessage(`"https://other.example"`)},
		},
		"manifest": Manifest{
			Extra: map[string]json.RawMessage{TermEntries: json.RawMessage(`[]`)},
		},
		"trust manifest": TrustManifest{
			Identity: "did:web:acme.com",
			Extra:    map[string]json.RawMessage{memberIdentity: json.RawMessage(`"did:web:other.example"`)},
		},
		"trust schema": TrustSchema{
			Extra: map[string]json.RawMessage{memberGovernanceURI: json.RawMessage(`"https://other.example"`)},
		},
		"attestation": Attestation{
			Extra: map[string]json.RawMessage{memberURI: json.RawMessage(`"https://other.example"`)},
		},
		"provenance link": ProvenanceLink{
			Extra: map[string]json.RawMessage{memberSourceID: json.RawMessage(`"other"`)},
		},
		"result": Result{
			Entry: Entry{Extra: map[string]json.RawMessage{TermScore: json.RawMessage(`1`)}},
		},
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := json.Marshal(value)
			if err == nil {
				t.Fatal("the encoder lost a value without reporting it")
			}
			if !errors.Is(err, jsonx.ErrReserved) {
				t.Errorf("the error is %v", err)
			}
		})
	}
}

func TestTrustManifestKeepsAnUnknownMemberOfTheEnvelope(t *testing.T) {
	const document = `{
  "identity": "did:web:acme.com",
  "identityType": "did",
  "trustSchema": {
    "identifier": "acme-trust",
    "version": "2",
    "governanceUri": "https://acme.com/governance",
    "verificationMethods": ["did-jwk"],
    "renewal": "yearly"
  },
  "attestations": [
    {"type": "SOC2-Type2", "uri": "https://acme.com/soc2.pdf", "mediaType": "application/pdf", "issuer": "auditor"}
  ],
  "provenance": [
    {"relation": "derivedFrom", "sourceId": "urn:air:acme.com:server:weather", "sourceDigest": "sha256:00", "at": "2026-01-01"}
  ],
  "signature": "abc",
  "policy": {"tier": 1}
}`
	var manifest TrustManifest
	requireRoundTrip(t, document, &manifest)
	if _, ok := manifest.Extra["policy"]; !ok {
		t.Error("the policy member did not reach Extra")
	}
	if _, ok := manifest.TrustSchema.Extra["renewal"]; !ok {
		t.Error("the renewal member did not reach the trust schema Extra")
	}
	if _, ok := manifest.Attestations[0].Extra["issuer"]; !ok {
		t.Error("the issuer member did not reach the attestation Extra")
	}
	if _, ok := manifest.Provenance[0].Extra["at"]; !ok {
		t.Error("the at member did not reach the provenance Extra")
	}
}

func TestEntryEncodesNoEmptyNamedTerm(t *testing.T) {
	encoded, err := json.Marshal(Entry{})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != "{}" {
		t.Errorf("an empty entry encodes to %s", encoded)
	}
}

func TestEntryEncodesExtraInSortedOrder(t *testing.T) {
	entry := Entry{
		Identifier: "urn:air:acme.com:server:weather",
		Extra: map[string]json.RawMessage{
			"zz:last":   json.RawMessage(`1`),
			"aa:first":  json.RawMessage(`2`),
			"mm:middle": json.RawMessage(`3`),
		},
	}
	const want = `{"identifier":"urn:air:acme.com:server:weather","aa:first":2,"mm:middle":3,"zz:last":1}`
	for range 8 {
		encoded, err := json.Marshal(entry)
		if err != nil {
			t.Fatal(err)
		}
		if string(encoded) != want {
			t.Fatalf("the encoding is %s", encoded)
		}
	}
}

func TestEntryKeepsAnEmptyArrayApartFromAMissingOne(t *testing.T) {
	var entry Entry
	requireRoundTrip(t, `{"identifier":"urn:air:acme.com:server:weather","tags":[],"metadata":{}}`, &entry)
	if entry.Tags == nil {
		t.Error("the empty tags array is lost")
	}
	if entry.Metadata == nil {
		t.Error("the empty metadata object is lost")
	}
}

func FuzzEntryRoundTrip(f *testing.F) {
	f.Add([]byte(plainEntry))
	f.Add([]byte(namespacedEntry))
	f.Add([]byte(`{"identifier":"x","data":{"a":[1,2,{"b":null}]},"metadata":{"n":1.5,"ok":true}}`))
	f.Add([]byte(`{"@context":["https://agenticresourcediscovery.org/context/v1",{"a":"b#"}],"tags":[],"trustManifest":{"identity":"did:web:x","x":1}}`))
	f.Add([]byte(`{"url":"","@context":null,"trustManifest":null,"data":null}`))
	f.Add([]byte(`null`))
	f.Fuzz(func(t *testing.T, document []byte) {
		var first Entry
		if err := json.Unmarshal(document, &first); err != nil {
			return
		}
		encodedFirst, err := json.Marshal(first)
		if err != nil {
			t.Fatalf("the decoded entry does not encode: %v", err)
		}
		var second Entry
		if err := json.Unmarshal(encodedFirst, &second); err != nil {
			t.Fatalf("the encoded entry does not decode: %v\n%s", err, encodedFirst)
		}
		encodedSecond, err := json.Marshal(second)
		if err != nil {
			t.Fatalf("the second entry does not encode: %v", err)
		}
		if !bytes.Equal(encodedFirst, encodedSecond) {
			t.Fatalf("the round trip changed the document\nfirst:  %s\nsecond: %s", encodedFirst, encodedSecond)
		}
		var third Entry
		if err := json.Unmarshal(encodedSecond, &third); err != nil {
			t.Fatalf("the second document does not decode: %v\n%s", err, encodedSecond)
		}
		if !reflect.DeepEqual(second, third) {
			t.Errorf("the round trip changed the entry\nsecond: %#v\nthird:  %#v", second, third)
		}
	})
}
