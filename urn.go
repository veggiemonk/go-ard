package ard

import (
	"fmt"
	"net/url"
	"strings"
)

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
func (u URN) String() string {
	if u.Publisher == "" && u.Name == "" && len(u.Namespace) == 0 {
		return ""
	}
	segments := make([]string, 0, len(u.Namespace)+2)
	segments = append(segments, u.Publisher)
	segments = append(segments, u.Namespace...)
	segments = append(segments, u.Name)
	return URNPrefix + strings.Join(segments, ":")
}

// ParseURN parses a discovery identifier. See appendix C and ADR-0007.
func ParseURN(s string) (URN, error) {
	if s == "" {
		return URN{}, fmt.Errorf("ard: the discovery identifier is empty")
	}
	if !strings.HasPrefix(s, URNPrefix) {
		return URN{}, fmt.Errorf("ard: discovery identifier %q does not start with %q", s, URNPrefix)
	}
	segments := strings.Split(strings.TrimPrefix(s, URNPrefix), ":")
	if len(segments) < 2 {
		return URN{}, fmt.Errorf("ard: discovery identifier %q needs at least a publisher segment and an agent name segment", s)
	}
	if err := checkPublisherSegment(segments[0]); err != nil {
		return URN{}, fmt.Errorf("ard: discovery identifier %q has a bad publisher segment: %w", s, err)
	}
	for _, segment := range segments[1:] {
		if err := checkNameSegment(segment); err != nil {
			return URN{}, fmt.Errorf("ard: discovery identifier %q has a bad segment: %w", s, err)
		}
	}
	parsed := URN{Publisher: segments[0], Name: segments[len(segments)-1]}
	if len(segments) > 2 {
		parsed.Namespace = segments[1 : len(segments)-1]
	}
	return parsed, nil
}

func checkPublisherSegment(segment string) error {
	if segment == "" {
		return fmt.Errorf("the publisher segment is empty")
	}
	for _, r := range segment {
		if isDomainRune(r) {
			continue
		}
		if r == '_' {
			return fmt.Errorf("the publisher segment %q holds an underscore, which DNS does not allow in a public host name", segment)
		}
		return fmt.Errorf("the publisher segment %q holds the character %q, which only letters, digits, dots and hyphens may replace", segment, r)
	}
	return nil
}

func checkNameSegment(segment string) error {
	if segment == "" {
		return fmt.Errorf("a segment is empty")
	}
	for _, r := range segment {
		if isDomainRune(r) || r == '_' {
			continue
		}
		return fmt.Errorf("the segment %q holds the character %q, which only letters, digits, dots, hyphens and underscores may replace", segment, r)
	}
	return nil
}

func isDomainRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	case r == '.', r == '-':
		return true
	}
	return false
}

const didWebPrefix = "did:web:"

// TrustDomain reads the domain out of a workload identity: a SPIFFE ID, a did:web
// identifier or an HTTPS URI. Section 4.5.1 binds that domain to the publisher segment
// of the entry identifier.
func TrustDomain(identity string) (string, error) {
	trimmed := strings.TrimSpace(identity)
	if trimmed == "" {
		return "", fmt.Errorf("ard: the workload identity is empty")
	}
	if strings.HasPrefix(trimmed, didWebPrefix) {
		return didWebDomain(trimmed, identity)
	}
	if strings.HasPrefix(trimmed, "did:") {
		return "", fmt.Errorf("ard: workload identity %q carries no domain: of the DID methods only did:web does", identity)
	}
	if strings.Contains(trimmed, "://") {
		return uriDomain(trimmed, identity)
	}
	return cleanDomain(trimmed, identity)
}

func didWebDomain(trimmed, identity string) (string, error) {
	head, _, _ := strings.Cut(strings.TrimPrefix(trimmed, didWebPrefix), ":")
	decoded, err := url.PathUnescape(head)
	if err != nil {
		return "", fmt.Errorf("ard: workload identity %q has a bad did:web identifier: %w", identity, err)
	}
	return cleanDomain(decoded, identity)
}

func uriDomain(trimmed, identity string) (string, error) {
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("ard: workload identity %q is not a URI: %w", identity, err)
	}
	if parsed.Hostname() == "" {
		return "", fmt.Errorf("ard: workload identity %q has no host", identity)
	}
	return cleanDomain(parsed.Hostname(), identity)
}

func cleanDomain(host, identity string) (string, error) {
	domain := strings.TrimSuffix(stripPort(strings.ToLower(strings.TrimSpace(host))), ".")
	if domain == "" {
		return "", fmt.Errorf("ard: workload identity %q names no domain", identity)
	}
	for _, r := range domain {
		if !isDomainRune(r) {
			return "", fmt.Errorf("ard: workload identity %q names the domain %q, which holds the character %q", identity, domain, r)
		}
	}
	if strings.HasPrefix(domain, ".") {
		return "", fmt.Errorf("ard: workload identity %q names the domain %q, which starts with a dot", identity, domain)
	}
	return domain, nil
}

func stripPort(host string) string {
	i := strings.LastIndex(host, ":")
	if i < 0 {
		return host
	}
	port := host[i+1:]
	if port == "" {
		return host[:i]
	}
	for _, r := range port {
		if r < '0' || r > '9' {
			return host
		}
	}
	return host[:i]
}

// SameAuthority reports whether a trust domain satisfies the publisher authority binding
// of section 4.5.1. With allowSubdomain the trust domain may be a subdomain of the
// publisher.
func SameAuthority(publisher, trustDomain string, allowSubdomain bool) bool {
	authority := authorityLabel(publisher)
	claimed := authorityLabel(trustDomain)
	if authority == "" || claimed == "" {
		return false
	}
	if authority == claimed {
		return true
	}
	return allowSubdomain && strings.HasSuffix(claimed, "."+authority)
}

func authorityLabel(s string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), ".")
}
