package ard

import (
	"slices"
	"testing"
)

func TestParseURNAccepts(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		publisher string
		namespace []string
		agent     string
	}{
		{"spec 4.4 weather server", "urn:air:acme.com:server:weather", "acme.com", []string{"server"}, "weather"},
		{"spec 4.4 skill from a solo developer", "urn:air:github.com:alice-dev:pptx-creator", "github.com", []string{"alice-dev"}, "pptx-creator"},
		{"spec 5.3.2 corporate assistant", "urn:air:acme.com:agent:assistant", "acme.com", []string{"agent"}, "assistant"},
		{"spec 5.3.2 two segments with no namespace", "urn:air:example.com:weather-server", "example.com", nil, "weather-server"},
		{"spec 5.3.2 public registry referral", "urn:air:nlweb.ai:registry:public", "nlweb.ai", []string{"registry"}, "public"},
		{"spec 5.4 travel registry", "urn:air:example.com:registry:travel", "example.com", []string{"registry"}, "travel"},
		{"spec 6 expense agent", "urn:air:acme.com:agent:expense", "acme.com", []string{"agent"}, "expense"},
		{"example catalog entry", "urn:air:acme.com:catalog:engineering", "acme.com", []string{"catalog"}, "engineering"},
		{"example tool entry", "urn:air:acme.com:tool:unit-converter", "acme.com", []string{"tool"}, "unit-converter"},
		{"example reserved tld publisher", "urn:air:bobs-plumbing.example:mcp:storefront", "bobs-plumbing.example", []string{"mcp"}, "storefront"},
		{"example government api", "urn:air:fda.gov:api:drug-ndc", "fda.gov", []string{"api"}, "drug-ndc"},
		{"example hyphenated agent name", "urn:air:noaa.gov:api:climate-data-online", "noaa.gov", []string{"api"}, "climate-data-online"},
		{"adr-0007 no namespace", "urn:air:acme.com:assistant", "acme.com", nil, "assistant"},
		{"adr-0007 recursive namespace", "urn:air:acme.com:finance:trading:trader", "acme.com", []string{"finance", "trading"}, "trader"},
		{"adr-0007 deep namespace", "urn:air:acme.com:department:team:service:agent", "acme.com", []string{"department", "team", "service"}, "agent"},
		{"adr-0007 underscore in the agent name", "urn:air:acme.com:finance:tax_agent", "acme.com", []string{"finance"}, "tax_agent"},
		{"adr-0007 underscore in a namespace segment", "urn:air:acme.com:back_office:tax_agent", "acme.com", []string{"back_office"}, "tax_agent"},
		{"dot in the agent name", "urn:air:acme.com:agent:v1.assistant", "acme.com", []string{"agent"}, "v1.assistant"},
		{"hyphen in the publisher", "urn:air:my-company.co.uk:agent:helper", "my-company.co.uk", []string{"agent"}, "helper"},
		{"digits only", "urn:air:1.example:2:3", "1.example", []string{"2"}, "3"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseURN(c.in)
			if err != nil {
				t.Fatalf("ParseURN(%q) gave the error %v", c.in, err)
			}
			if got.Publisher != c.publisher {
				t.Errorf("publisher = %q, want %q", got.Publisher, c.publisher)
			}
			if !slices.Equal(got.Namespace, c.namespace) {
				t.Errorf("namespace = %v, want %v", got.Namespace, c.namespace)
			}
			if got.Name != c.agent {
				t.Errorf("name = %q, want %q", got.Name, c.agent)
			}
			if got.String() != c.in {
				t.Errorf("String() = %q, want %q", got.String(), c.in)
			}
		})
	}
}

func TestParseURNRejects(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"the old two letter nid", "urn:ai:acme.com:server:weather"},
		{"an unknown nid", "urn:agent:acme.com:server:weather"},
		{"no urn scheme", "air:acme.com:server:weather"},
		{"an http uri", "https://acme.com/agents/weather"},
		{"an upper case nid", "URN:AIR:acme.com:server:weather"},
		{"the prefix alone", "urn:air:"},
		{"a publisher with no name", "urn:air:acme.com"},
		{"a trailing colon", "urn:air:acme.com:"},
		{"an empty namespace segment", "urn:air:acme.com::weather"},
		{"an empty publisher segment", "urn:air::server:weather"},
		{"a slash in the agent name", "urn:air:acme.com:server/weather"},
		{"a slash in a namespace segment", "urn:air:acme.com:server/sub:weather"},
		{"an underscore in the publisher", "urn:air:acme_corp.com:server:weather"},
		{"a space in the agent name", "urn:air:acme.com:server:weather agent"},
		{"a percent escape in the agent name", "urn:air:acme.com:server:weather%20agent"},
		{"a plus in the agent name", "urn:air:acme.com:server:weather+agent"},
		{"an at sign in the publisher", "urn:air:acme@com:server:weather"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseURN(c.in)
			if err == nil {
				t.Fatalf("ParseURN(%q) gave %+v, want an error", c.in, got)
			}
		})
	}
}

func TestURNString(t *testing.T) {
	cases := []struct {
		name string
		urn  URN
		want string
	}{
		{"the zero value", URN{}, ""},
		{"no namespace", URN{Publisher: "example.com", Name: "weather-server"}, "urn:air:example.com:weather-server"},
		{"one namespace segment", URN{Publisher: "acme.com", Namespace: []string{"server"}, Name: "weather"}, "urn:air:acme.com:server:weather"},
		{"two namespace segments", URN{Publisher: "acme.com", Namespace: []string{"finance", "trading"}, Name: "trader"}, "urn:air:acme.com:finance:trading:trader"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.urn.String(); got != c.want {
				t.Errorf("String() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestEntryURN(t *testing.T) {
	e := Entry{Identifier: "urn:air:acme.com:server:weather"}
	got, err := e.URN()
	if err != nil {
		t.Fatalf("URN() gave the error %v", err)
	}
	if got.Publisher != "acme.com" || got.Name != "weather" {
		t.Errorf("URN() = %+v, want the publisher acme.com and the name weather", got)
	}
}

func TestTrustDomain(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"a spiffe id", "spiffe://acme.com/ns/default/sa/x", "acme.com"},
		{"a spiffe id with no path", "spiffe://acme.com", "acme.com"},
		{"a spiffe id with a port", "spiffe://acme.com:8443/ns/default/sa/x", "acme.com"},
		{"a spiffe id in upper case", "spiffe://ACME.COM/ns/default/sa/x", "acme.com"},
		{"a did web identifier", "did:web:acme.com", "acme.com"},
		{"a did web identifier with a path", "did:web:acme.com:user:alice", "acme.com"},
		{"a did web identifier with an escaped port", "did:web:acme.com%3A8443:path", "acme.com"},
		{"a did web identifier with an escaped port and no path", "did:web:acme.com%3A8443", "acme.com"},
		{"a did web identifier in upper case", "did:web:ACME.com", "acme.com"},
		{"an https uri", "https://acme.com/x", "acme.com"},
		{"an https uri with a port", "https://acme.com:8443/x", "acme.com"},
		{"an https uri with no path", "https://acme.com", "acme.com"},
		{"a bare fqdn", "acme.com", "acme.com"},
		{"a bare fqdn with a trailing dot", "acme.com.", "acme.com"},
		{"a bare fqdn with a port", "acme.com:8443", "acme.com"},
		{"a bare fqdn in upper case", "ACME.COM", "acme.com"},
		{"a bare fqdn with surrounding space", "  acme.com  ", "acme.com"},
		{"a subdomain", "api.acme.com", "api.acme.com"},
		{"a trailing dot inside a uri", "https://acme.com./x", "acme.com"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := TrustDomain(c.in)
			if err != nil {
				t.Fatalf("TrustDomain(%q) gave the error %v", c.in, err)
			}
			if got != c.want {
				t.Errorf("TrustDomain(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestTrustDomainRejects(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"blank", "   "},
		{"a did method that carries no domain", "did:key:z6MkhaXgBZDvotDkL5257faiztiGiC2QtKLGpbnnEGta2doK"},
		{"a uri with no host", "https:///path"},
		{"a bare name with a path", "acme.com/x"},
		{"a bare name with an underscore", "acme_corp.com"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := TrustDomain(c.in)
			if err == nil {
				t.Fatalf("TrustDomain(%q) = %q, want an error", c.in, got)
			}
		})
	}
}

func TestSameAuthority(t *testing.T) {
	cases := []struct {
		name           string
		publisher      string
		trustDomain    string
		allowSubdomain bool
		want           bool
	}{
		{"the same domain", "acme.com", "acme.com", false, true},
		{"the same domain in another case", "ACME.com", "acme.COM", false, true},
		{"the same domain with a trailing dot", "acme.com", "acme.com.", false, true},
		{"a different domain", "acme.com", "example.com", false, false},
		{"a subdomain without the option", "acme.com", "api.acme.com", false, false},
		{"a subdomain with the option", "acme.com", "api.acme.com", true, true},
		{"a deep subdomain with the option", "acme.com", "eu.api.acme.com", true, true},
		{"a look alike domain without the option", "acme.com", "evilacme.com", false, false},
		{"a look alike domain with the option", "acme.com", "evilacme.com", true, false},
		{"a look alike subdomain with the option", "acme.com", "api.evilacme.com", true, false},
		{"a suffix that crosses no label boundary", "acme.com", "notacme.com", true, false},
		{"the publisher is a subdomain of the trust domain", "api.acme.com", "acme.com", true, false},
		{"a squatted parent domain", "google.com", "attacker.example", true, false},
		{"an empty publisher", "", "acme.com", true, false},
		{"an empty trust domain", "acme.com", "", true, false},
		{"both empty", "", "", true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SameAuthority(c.publisher, c.trustDomain, c.allowSubdomain)
			if got != c.want {
				t.Errorf("SameAuthority(%q, %q, %v) = %v, want %v", c.publisher, c.trustDomain, c.allowSubdomain, got, c.want)
			}
		})
	}
}

func TestSameAuthorityWithAParsedIdentifier(t *testing.T) {
	u, err := ParseURN("urn:air:acme.com:server:weather")
	if err != nil {
		t.Fatalf("ParseURN gave the error %v", err)
	}
	domain, err := TrustDomain("spiffe://acme.com/ns/default/sa/weather")
	if err != nil {
		t.Fatalf("TrustDomain gave the error %v", err)
	}
	if !SameAuthority(u.Publisher, domain, false) {
		t.Errorf("SameAuthority(%q, %q, false) = false, want true", u.Publisher, domain)
	}
}
