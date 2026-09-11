package discover

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// EntrySourcePrefix labels the DNS name that points at a static entry source.
const EntrySourcePrefix = "_entries._agents."

// SearchPrefix labels the DNS name that points at a registry search endpoint.
const SearchPrefix = "_search._agents."

// ErrSVCBUnsupported reports that a prober cannot read the Service Binding records of section 5.1.
var ErrSVCBUnsupported = errors.New("discover: this prober cannot query SVCB records")

// ErrNoRecord is the answer a prober gives when a domain publishes no record under the name.
var ErrNoRecord = errors.New("discover: the domain publishes no record under this name")

// DNSProber reads the Service Binding records that section 5.1 defines for a domain.
//
// Both methods give the targets the records point at, in the order the prober read them.
// A prober reports ErrNoRecord when the domain publishes nothing under the name, and
// ErrSVCBUnsupported when it cannot ask the question at all.
type DNSProber interface {
	// EntrySources gives the static entry sources published under EntrySourceName.
	EntrySources(ctx context.Context, domain string) ([]string, error)

	// SearchEndpoints gives the registry search endpoints published under SearchName.
	SearchEndpoints(ctx context.Context, domain string) ([]string, error)
}

// EntrySourceName builds the DNS name of the static entry source of a domain.
func EntrySourceName(domain string) string { return EntrySourcePrefix + strings.Trim(domain, ".") }

// SearchName builds the DNS name of the registry search endpoint of a domain.
func SearchName(domain string) string { return SearchPrefix + strings.Trim(domain, ".") }

// UnsupportedProber is the DNSProber of this package: it reports ErrSVCBUnsupported.
//
// Section 5.1 publishes the DNS mechanism as Service Binding records, record type 64.
// The net package of the standard library resolves addresses, names, mail exchangers,
// name servers, text records and service records, and offers no way to ask for any other
// record type, so it cannot read a Service Binding record or its target.
//
// The obvious substitute does not work either. Asking net.Resolver whether the name
// exists tells a caller nothing, because the standard library reports an empty answer
// and an absent name as the same "no such host" error: a name that carries only SVCB
// records is indistinguishable from a name that was never published. A prober built on
// that test would report an absent record for a domain that publishes one, so this
// package does not build one.
//
// A caller that needs the DNS mechanism supplies a DNSProber of its own, built on a
// resolver library that queries record type 64.
type UnsupportedProber struct{}

// EntrySources reports ErrSVCBUnsupported.
func (UnsupportedProber) EntrySources(ctx context.Context, domain string) ([]string, error) {
	return nil, unsupported(EntrySourceName(domain), domain)
}

// SearchEndpoints reports ErrSVCBUnsupported.
func (UnsupportedProber) SearchEndpoints(ctx context.Context, domain string) ([]string, error) {
	return nil, unsupported(SearchName(domain), domain)
}

func unsupported(name, domain string) error {
	if strings.Trim(domain, ". ") == "" {
		return fmt.Errorf("%w", ErrEmptyDomain)
	}
	return fmt.Errorf("discover: %s: %w", name, ErrSVCBUnsupported)
}
