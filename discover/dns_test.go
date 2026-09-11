package discover

import (
	"errors"
	"strings"
	"testing"
)

func TestEntrySourceName(t *testing.T) {
	cases := []struct{ domain, want string }{
		{"example.com", "_entries._agents.example.com"},
		{"example.com.", "_entries._agents.example.com"},
		{"sub.example.com", "_entries._agents.sub.example.com"},
	}
	for _, test := range cases {
		t.Run(test.domain, func(t *testing.T) {
			if got := EntrySourceName(test.domain); got != test.want {
				t.Errorf("EntrySourceName = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSearchName(t *testing.T) {
	cases := []struct{ domain, want string }{
		{"example.com", "_search._agents.example.com"},
		{"example.com.", "_search._agents.example.com"},
	}
	for _, test := range cases {
		t.Run(test.domain, func(t *testing.T) {
			if got := SearchName(test.domain); got != test.want {
				t.Errorf("SearchName = %q, want %q", got, test.want)
			}
		})
	}
}

func TestUnsupportedProberSatisfiesDNSProber(t *testing.T) {
	var prober DNSProber = UnsupportedProber{}
	if prober == nil {
		t.Fatal("UnsupportedProber does not satisfy DNSProber")
	}
}

func TestUnsupportedProberReportsSVCBUnsupported(t *testing.T) {
	prober := UnsupportedProber{}

	sources, err := prober.EntrySources(t.Context(), "example.com")
	if !errors.Is(err, ErrSVCBUnsupported) {
		t.Fatalf("EntrySources error = %v, want ErrSVCBUnsupported", err)
	}
	if sources != nil {
		t.Errorf("EntrySources = %v, want nothing", sources)
	}
	if !strings.Contains(err.Error(), EntrySourceName("example.com")) {
		t.Errorf("error does not name the record: %v", err)
	}

	endpoints, err := prober.SearchEndpoints(t.Context(), "example.com")
	if !errors.Is(err, ErrSVCBUnsupported) {
		t.Fatalf("SearchEndpoints error = %v, want ErrSVCBUnsupported", err)
	}
	if endpoints != nil {
		t.Errorf("SearchEndpoints = %v, want nothing", endpoints)
	}
	if !strings.Contains(err.Error(), SearchName("example.com")) {
		t.Errorf("error does not name the record: %v", err)
	}
}

func TestUnsupportedProberRejectsAnEmptyDomain(t *testing.T) {
	prober := UnsupportedProber{}
	for _, domain := range []string{"", "   ", "."} {
		t.Run("domain "+domain, func(t *testing.T) {
			if _, err := prober.EntrySources(t.Context(), domain); !errors.Is(err, ErrEmptyDomain) {
				t.Errorf("EntrySources error = %v, want ErrEmptyDomain", err)
			}
			if _, err := prober.SearchEndpoints(t.Context(), domain); !errors.Is(err, ErrEmptyDomain) {
				t.Errorf("SearchEndpoints error = %v, want ErrEmptyDomain", err)
			}
		})
	}
}
