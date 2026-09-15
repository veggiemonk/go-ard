package ard

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const openKnowledgeNS = "https://openknowledgeformat.org/ns#"

func TestBaseContextHoldsTheContextValue(t *testing.T) {
	base := BaseContext()
	if base.IsZero() {
		t.Fatal("BaseContext() is the zero context")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(base.Raw(), &object); err != nil {
		t.Fatalf("the base context is not a JSON object: %v", err)
	}
	if _, wrapped := object[TermContext]; wrapped {
		t.Error("the base context still holds its @context wrapper")
	}
	for _, term := range []string{"@vocab", "ard", "identifier", "type", "url", "trustManifest"} {
		if _, ok := object[term]; !ok {
			t.Errorf("the base context declares no %q", term)
		}
	}
}

func TestBaseContextMatchesTheSpecificationCopy(t *testing.T) {
	published, err := os.ReadFile(filepath.Join("testdata", "ard.context.jsonld"))
	if err != nil {
		t.Fatalf("cannot read the published context: %v", err)
	}
	var fromSpec, embedded any
	if err := json.Unmarshal(published, &fromSpec); err != nil {
		t.Fatalf("the published context is not JSON: %v", err)
	}
	if err := json.Unmarshal(baseContextDocument, &embedded); err != nil {
		t.Fatalf("the embedded context is not JSON: %v", err)
	}
	if !reflect.DeepEqual(fromSpec, embedded) {
		t.Error("the embedded base context has drifted from testdata/ard.context.jsonld")
	}
}

func TestPrefixes(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want map[string]string
	}{
		{
			"the base context as a string",
			`"https://agenticresourcediscovery.org/context/v1"`,
			map[string]string{"": DefaultNamespace, "ard": DefaultNamespace},
		},
		{
			"an unknown remote context as a string",
			`"https://example.com/context/v1"`,
			map[string]string{},
		},
		{
			"an object that declares one prefix",
			`{"okf": "https://openknowledgeformat.org/ns#"}`,
			map[string]string{"okf": openKnowledgeNS},
		},
		{
			"the section 4.4 publisher extension object",
			`{"acme": "https://acme.com/vocab#"}`,
			map[string]string{"acme": "https://acme.com/vocab#"},
		},
		{
			"an object that declares a vocab and a prefix",
			`{"@vocab": "https://example.com/ns#", "ex": "https://example.com/ns#"}`,
			map[string]string{"": "https://example.com/ns#", "ex": "https://example.com/ns#"},
		},
		{
			"an object whose term definitions are not prefixes",
			`{"identifier": "ard:identifier", "url": {"@id": "ard:url", "@type": "@id"}}`,
			map[string]string{},
		},
		{
			"an object that marks a term as a prefix",
			`{"okf": {"@id": "https://openknowledgeformat.org/ns", "@prefix": true}}`,
			map[string]string{"okf": "https://openknowledgeformat.org/ns"},
		},
		{
			"an array of the base context and a local object",
			`["https://agenticresourcediscovery.org/context/v1", {"okf": "https://openknowledgeformat.org/ns#"}]`,
			map[string]string{"": DefaultNamespace, "ard": DefaultNamespace, "okf": openKnowledgeNS},
		},
		{
			"an array of two objects where the later one wins",
			`[{"ex": "https://first.example/ns#"}, {"ex": "https://second.example/ns#"}]`,
			map[string]string{"ex": "https://second.example/ns#"},
		},
		{
			"an array of objects that bind one namespace under two prefixes",
			`[{"okf": "https://openknowledgeformat.org/ns#"}, {"openknowledge": "https://openknowledgeformat.org/ns#"}]`,
			map[string]string{"okf": openKnowledgeNS, "openknowledge": openKnowledgeNS},
		},
		{
			"an empty object",
			`{}`,
			map[string]string{},
		},
		{
			"an empty array",
			`[]`,
			map[string]string{},
		},
		{
			"a document that wraps its context",
			`{"@context": {"okf": "https://openknowledgeformat.org/ns#"}}`,
			map[string]string{"okf": openKnowledgeNS},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := RawContext(json.RawMessage(c.raw)).Prefixes()
			if err != nil {
				t.Fatalf("Prefixes() gave the error %v", err)
			}
			if !maps.Equal(got, c.want) {
				t.Errorf("Prefixes() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestPrefixesOfTheZeroContext(t *testing.T) {
	got, err := Context{}.Prefixes()
	if err != nil {
		t.Fatalf("Prefixes() gave the error %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Prefixes() = %v, want no binding", got)
	}
}

func TestPrefixesRejectsABadContext(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"a number", `42`},
		{"a boolean", `true`},
		{"an array that holds a number", `[{"ex": "https://example.com/ns#"}, 42]`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := RawContext(json.RawMessage(c.raw)).Prefixes()
			if err == nil {
				t.Fatalf("Prefixes() = %v, want an error", got)
			}
		})
	}
}

func TestNewContextRoundTrip(t *testing.T) {
	c, err := NewContext(map[string]string{"okf": openKnowledgeNS})
	if err != nil {
		t.Fatalf("NewContext gave the error %v", err)
	}
	got, err := c.Prefixes()
	if err != nil {
		t.Fatalf("Prefixes() gave the error %v", err)
	}
	if got["okf"] != openKnowledgeNS {
		t.Errorf("Prefixes()[okf] = %q, want %q", got["okf"], openKnowledgeNS)
	}
}
