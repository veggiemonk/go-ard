package main

import (
	"flag"
	"testing"
)

func TestParseArgsTakesFlagsOnEitherSideOfThePositional(t *testing.T) {
	cases := map[string][]string{
		"flag before the positional": {"-addr", ":9011", "manifest.json"},
		"flag after the positional":  {"manifest.json", "-addr", ":9011"},
		"flag on both sides":         {"-source", "x", "manifest.json", "-addr", ":9011"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			flags := flag.NewFlagSet("serve", flag.ContinueOnError)
			addr := flags.String("addr", ":9010", "")
			flags.String("source", "", "")
			positional, err := parseArgs(flags, args)
			if err != nil {
				t.Fatalf("parseArgs: %v", err)
			}
			if len(positional) != 1 || positional[0] != "manifest.json" {
				t.Fatalf("positional = %v, want [manifest.json]", positional)
			}
			if *addr != ":9011" {
				t.Fatalf("addr = %q, want :9011", *addr)
			}
		})
	}
}

func TestValidateDocumentPicksTheKind(t *testing.T) {
	manifest := `{"entries":[{"identifier":"urn:air:acme.com:server:weather","displayName":"W","type":"application/mcp-server-card+json","url":"https://acme.com/w.json"}]}`
	entry := `{"identifier":"urn:air:acme.com:server:weather","displayName":"W","type":"application/mcp-server-card+json","url":"https://acme.com/w.json"}`

	t.Run("a document with entries is a manifest", func(t *testing.T) {
		report, kind, err := validateDocument([]byte(manifest))
		if err != nil {
			t.Fatalf("validateDocument: %v", err)
		}
		if kind != "a manifest of 1 entries" {
			t.Fatalf("kind = %q", kind)
		}
		if !report.OK() {
			t.Fatalf("report has errors: %v", report.Errors)
		}
	})

	t.Run("a document with an identifier is one entry", func(t *testing.T) {
		_, kind, err := validateDocument([]byte(entry))
		if err != nil {
			t.Fatalf("validateDocument: %v", err)
		}
		if kind != "a single entry" {
			t.Fatalf("kind = %q", kind)
		}
	})

	t.Run("a document with neither is an error", func(t *testing.T) {
		if _, _, err := validateDocument([]byte(`{"host":{}}`)); err == nil {
			t.Fatal("want an error")
		}
	})

	t.Run("a document that is no JSON object is an error", func(t *testing.T) {
		if _, _, err := validateDocument([]byte(`[1,2]`)); err == nil {
			t.Fatal("want an error")
		}
	})
}

func TestValidateDocumentReportsTheAuthorityBinding(t *testing.T) {
	squatted := `{"identifier":"urn:air:acme.com:server:weather","displayName":"W","type":"application/mcp-server-card+json","url":"https://acme.com/w.json","trustManifest":{"identity":"spiffe://evil.com/x"}}`
	report, _, err := validateDocument([]byte(squatted))
	if err != nil {
		t.Fatalf("validateDocument: %v", err)
	}
	if report.OK() {
		t.Fatal("an identity outside the publisher trust domain must be an error")
	}
}

func TestHeaderListBuildsAClient(t *testing.T) {
	var headers headerList
	if err := headers.Set("Authorization: Bearer token"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := headers.Set("no-colon"); err == nil {
		t.Fatal("a header without a colon must be refused")
	}
	client, err := headers.client("https://registry.example.com/api")
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	if got := client.Header.Get("Authorization"); got != "Bearer token" {
		t.Fatalf("Authorization = %q", got)
	}
}
