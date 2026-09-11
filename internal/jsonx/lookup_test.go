package jsonx

import (
	"encoding/json"
	"slices"
	"testing"
)

const entry = `{
  "identifier": "urn:air:acme.com:server:weather",
  "tags": ["weather", "telemetry"],
  "metadata": {"location": "Chicago", "floor": 3},
  "trustManifest": {
    "identity": "did:web:acme.com",
    "attestations": [
      {"type": "SOC2-Type2", "uri": "https://acme.com/soc2"},
      {"type": "ISO-27001", "uri": "https://acme.com/iso"}
    ]
  }
}`

func found(t *testing.T, document string, path ...string) []string {
	t.Helper()
	values, err := Lookup(json.RawMessage(document), path)
	if err != nil {
		t.Fatalf("lookup of %v: %v", path, err)
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func requireValues(t *testing.T, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("the lookup gives %v, not %v", got, want)
	}
}

func TestLookupWalksALiteralDotPath(t *testing.T) {
	requireValues(t, found(t, entry, "identifier"), []string{`"urn:air:acme.com:server:weather"`})
	requireValues(t, found(t, entry, "metadata", "location"), []string{`"Chicago"`})
	requireValues(t, found(t, entry, "metadata", "floor"), []string{`3`})
	requireValues(t, found(t, entry, "trustManifest", "identity"), []string{`"did:web:acme.com"`})
}

func TestLookupVisitsTheElementsOfAnArray(t *testing.T) {
	requireValues(t, found(t, entry, "tags"), []string{`"weather"`, `"telemetry"`})
	requireValues(t, found(t, entry, "trustManifest", "attestations", "type"),
		[]string{`"SOC2-Type2"`, `"ISO-27001"`})
}

func TestLookupVisitsTheElementsOfANestedArray(t *testing.T) {
	requireValues(t, found(t, `{"a":[[1,2],[3]]}`, "a"), []string{`1`, `2`, `3`})
	requireValues(t, found(t, `[{"a":1},{"a":2}]`, "a"), []string{`1`, `2`})
}

func TestLookupGivesTheDocumentForAnEmptyPath(t *testing.T) {
	requireValues(t, found(t, `{"a":1}`), []string{`{"a":1}`})
}

func TestLookupGivesNothingForAPathThatReachesNothing(t *testing.T) {
	requireValues(t, found(t, entry, "absent"), nil)
	requireValues(t, found(t, entry, "metadata", "absent"), nil)
	requireValues(t, found(t, entry, "identifier", "deeper"), nil)
	requireValues(t, found(t, entry, "tags", "deeper"), nil)
	requireValues(t, found(t, `{"a":[]}`, "a"), nil)
	requireValues(t, found(t, "", "a"), nil)
}

func TestLookupKeepsANullApartFromAnAbsentMember(t *testing.T) {
	requireValues(t, found(t, `{"a":null}`, "a"), []string{`null`})
}

func TestLookupReportsADocumentThatHoldsNoValidJSON(t *testing.T) {
	if _, err := Lookup(json.RawMessage(`{"a":`), []string{"a"}); err == nil {
		t.Fatal("the lookup read a broken document")
	}
}
