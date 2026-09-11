package ard

// URNPrefix is the URN namespace identifier of a discovery identifier. See ADR-0009.
const URNPrefix = "urn:air:"

// URN is a parsed discovery identifier of the form
// urn:air:<publisher>:<namespace>:<agent-name>.
//
// ADR-0007 makes the middle segments optional and recursive, so Namespace may be empty
// or hold more than one segment.
type URN struct {
	Publisher string
	Namespace []string
	Name      string
}

// String gives the identifier back in its wire form.
func (u URN) String() string { panic("ard: not implemented") }

// ParseURN parses a discovery identifier. See appendix C and ADR-0007.
func ParseURN(s string) (URN, error) { panic("ard: not implemented") }

// TrustDomain reads the domain out of a workload identity: a SPIFFE ID, a did:web
// identifier or an HTTPS URI. Section 4.5.1 binds that domain to the publisher segment
// of the entry identifier.
func TrustDomain(identity string) (string, error) { panic("ard: not implemented") }

// SameAuthority reports whether a trust domain satisfies the publisher authority binding
// of section 4.5.1. With allowSubdomain the trust domain may be a subdomain of the
// publisher.
func SameAuthority(publisher, trustDomain string, allowSubdomain bool) bool {
	panic("ard: not implemented")
}
