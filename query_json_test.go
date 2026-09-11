package ard

import (
	"encoding/json"
	"reflect"
	"testing"
)

const queryOfSection531 = `{
  "query": {
    "@context": { "okf": "https://openknowledgeformat.org/ns#" },
    "text": "find me a flight booking agent",
    "filter": {
      "type": ["application/a2a-agent-card+json"],
      "tags": ["finance"],
      "okf:taxonomy": ["us-gaap"],
      "trustManifest.attestations.type": ["SOC2-Type2"]
    }
  }
}`

func TestSearchRequestRoundTripsTheQueryOfSection531(t *testing.T) {
	var request struct {
		Query Query `json:"query"`
	}
	requireRoundTrip(t, queryOfSection531, &request)
	query := request.Query
	if query.Context.IsZero() {
		t.Fatal("the query context is lost")
	}
	want := map[string][]string{
		"type":                            {"application/a2a-agent-card+json"},
		"tags":                            {"finance"},
		"okf:taxonomy":                    {"us-gaap"},
		"trustManifest.attestations.type": {"SOC2-Type2"},
	}
	if !reflect.DeepEqual(query.Filter, want) {
		t.Errorf("the filter is %v", query.Filter)
	}
}

func TestQueryReadsABareScalarFilterValueAsAOneElementArray(t *testing.T) {
	cases := map[string]struct {
		document string
		want     []string
	}{
		"string":  {`{"filter":{"type":"application/ai-skill+md"}}`, []string{"application/ai-skill+md"}},
		"number":  {`{"filter":{"metadata.floor":3}}`, []string{"3"}},
		"decimal": {`{"filter":{"metadata.rating":4.5}}`, []string{"4.5"}},
		"boolean": {`{"filter":{"metadata.verified":true}}`, []string{"true"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			var query Query
			if err := json.Unmarshal([]byte(c.document), &query); err != nil {
				t.Fatal(err)
			}
			for _, values := range query.Filter {
				if !reflect.DeepEqual(values, c.want) {
					t.Fatalf("the values are %v", values)
				}
			}
			encoded, err := json.Marshal(query)
			if err != nil {
				t.Fatal(err)
			}
			var again Query
			if err := json.Unmarshal(encoded, &again); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(query, again) {
				t.Errorf("the round trip changed the query: %s", encoded)
			}
		})
	}
}

func TestQueryEncodesEveryFilterValueAsAnArray(t *testing.T) {
	var query Query
	if err := json.Unmarshal([]byte(`{"text":"weather","filter":{"type":"application/ai-skill+md","tags":["a","b"]}}`), &query); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(query)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"text":"weather","filter":{"tags":["a","b"],"type":["application/ai-skill+md"]}}`
	if string(encoded) != want {
		t.Errorf("the query encodes to %s", encoded)
	}
}

func TestQueryOmitsTheMembersItDoesNotCarry(t *testing.T) {
	encoded, err := json.Marshal(Query{})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != "{}" {
		t.Errorf("an empty query encodes to %s", encoded)
	}
}

func TestQueryReportsAFilterValueThatIsNotAScalar(t *testing.T) {
	cases := map[string]string{
		"object":            `{"filter":{"type":{"a":1}}}`,
		"object in array":   `{"filter":{"type":[{"a":1}]}}`,
		"null in array":     `{"filter":{"type":["a",null]}}`,
		"filter not object": `{"filter":["type"]}`,
	}
	for name, document := range cases {
		t.Run(name, func(t *testing.T) {
			var query Query
			if err := json.Unmarshal([]byte(document), &query); err == nil {
				t.Fatalf("the decoder accepted %s as %v", document, query.Filter)
			}
		})
	}
}

func TestExploreRequestCarriesTheQueryModel(t *testing.T) {
	const document = `{
  "query": {
    "filter": {
      "type": ["application/mcp-server-card+json"]
    }
  },
  "resultType": {
    "facets": [
      {"field": "capabilities", "limit": 5, "minCount": 2}
    ]
  }
}`
	var request ExploreRequest
	requireRoundTrip(t, document, &request)
	if request.ResultType.Facets[0].Field != TermCapabilities {
		t.Errorf("the facet field is %q", request.ResultType.Facets[0].Field)
	}
}
