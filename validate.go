package ard

import (
	"errors"
	"fmt"
	"strings"
)

// Issue codes that validation reports.
const (
	IssueMissingTerm         = "missing_term"
	IssueBadURN              = "bad_urn"
	IssueLegacyURNPrefix     = "legacy_urn_prefix"
	IssueValueOrReference    = "value_or_reference"
	IssueMissingIdentity     = "missing_identity"
	IssueAuthorityMismatch   = "authority_mismatch"
	IssueNoQueries           = "no_representative_queries"
	IssueQueryCount          = "representative_query_count"
	IssueLegacyCollections   = "legacy_collections"
	IssueIDMismatch          = "id_identifier_mismatch"
	IssueBadTimestamp        = "bad_timestamp"
	IssueBadContext          = "bad_context"
	IssueDuplicateIdentifier = "duplicate_identifier"
)

const (
	sectionJSONLD               = "4.1"
	sectionEntryTerms           = "4.2"
	sectionValueOrReference     = "4.3"
	sectionTrust                = "4.5"
	sectionAuthorityBinding     = "4.5.1"
	sectionURN                  = "C"
	sectionEntrySchema          = "D.1"
	sectionDiscoveryConstraints = "D.2"
	sectionLegacyCollections    = "ADR-0003"
)

const (
	legacyCollectionsMember  = "collections"
	trustIdentityMember      = "identity"
	minRepresentativeQueries = 2
	maxRepresentativeQueries = 5
)

// Issue is one finding of validation. Section names the clause of the specification that
// the finding comes from, such as "4.3" or "D.2".
type Issue struct {
	Path    string
	Code    string
	Section string
	Message string
}

// Error gives the finding as one line, so that an Issue reported as an error is itself
// an error value.
func (i Issue) Error() string {
	return fmt.Sprintf("ard: %s: %s (%s, section %s)", i.Path, i.Message, i.Code, i.Section)
}

// Report holds the findings of validation. Appendix D.2 separates the two severities: a
// warning leaves the document structurally valid.
type Report struct {
	Errors   []Issue
	Warnings []Issue
}

// OK reports whether the document has no error. Warnings do not change the result.
func (r Report) OK() bool { return len(r.Errors) == 0 }

// Err gives the errors of the report as one error, or nil. Every error of the report is
// an Issue and the result comes from errors.Join, so errors.As reaches an Issue and an
// unwrap to []error reaches every one of them.
func (r Report) Err() error {
	if len(r.Errors) == 0 {
		return nil
	}
	joined := make([]error, len(r.Errors))
	for i, issue := range r.Errors {
		joined[i] = issue
	}
	return errors.Join(joined...)
}

func (r *Report) fail(path, code, section, format string, args ...any) {
	r.Errors = append(r.Errors, Issue{Path: path, Code: code, Section: section, Message: fmt.Sprintf(format, args...)})
}

func (r *Report) warn(path, code, section, format string, args ...any) {
	r.Warnings = append(r.Warnings, Issue{Path: path, Code: code, Section: section, Message: fmt.Sprintf(format, args...)})
}

func (r *Report) adopt(other Report, prefix string) {
	for _, issue := range other.Errors {
		issue.Path = prefix + issue.Path
		r.Errors = append(r.Errors, issue)
	}
	for _, issue := range other.Warnings {
		issue.Path = prefix + issue.Path
		r.Warnings = append(r.Warnings, issue)
	}
}

// Validator holds the options of validation.
type Validator struct {
	// AllowSubdomainAuthority lets a trust domain be a subdomain of the publisher in the
	// binding of section 4.5.1. The default demands the same domain.
	AllowSubdomainAuthority bool
}

// Validate checks one entry with the default options.
func Validate(e Entry) Report { return Validator{}.Entry(e) }

// ValidateManifest checks a manifest and every entry in it, with the default options.
func ValidateManifest(m Manifest) Report { return Validator{}.Manifest(m) }

// Entry checks one entry against section 4 and appendix D.2.
func (v Validator) Entry(e Entry) Report {
	var report Report
	checkRequiredTerms(e, &report)
	identifier, parsed := checkIdentifier(e, &report)
	checkValueOrReference(e, &report)
	v.checkTrustManifest(e, identifier, parsed, &report)
	checkRepresentativeQueries(e, &report)
	checkNodeIdentifier(e, &report)
	checkUpdatedAt(e, &report)
	checkEntryContext(e, &report)
	return report
}

// Manifest checks a manifest against section 5.1, and every entry in it.
func (v Validator) Manifest(m Manifest) Report {
	var report Report
	firstSeenAt := make(map[string]int, len(m.Entries))
	for i, entry := range m.Entries {
		report.adopt(v.Entry(entry), fmt.Sprintf("%s[%d].", TermEntries, i))
		if entry.Identifier == "" {
			continue
		}
		if first, duplicate := firstSeenAt[entry.Identifier]; duplicate {
			report.warn(fmt.Sprintf("%s[%d].%s", TermEntries, i, TermIdentifier), IssueDuplicateIdentifier, sectionURN,
				"the identifier %q already names entry %d; appendix C rests on a discovery identifier naming one resource",
				entry.Identifier, first)
			continue
		}
		firstSeenAt[entry.Identifier] = i
	}
	if _, legacy := m.Extra[legacyCollectionsMember]; legacy {
		report.warn(legacyCollectionsMember, IssueLegacyCollections, sectionLegacyCollections,
			"the manifest carries a %q member at its root, which ADR-0003 removed; ARD ignores unrecognized top-level members, so this does not invalidate the manifest, but a hierarchy belongs inside %q",
			legacyCollectionsMember, TermEntries)
	}
	return report
}

func checkRequiredTerms(e Entry, report *Report) {
	required := []struct {
		term  string
		value string
	}{
		{TermIdentifier, e.Identifier},
		{TermDisplayName, e.DisplayName},
		{TermType, e.Type},
	}
	for _, r := range required {
		if r.value == "" {
			report.fail(r.term, IssueMissingTerm, sectionEntryTerms,
				"the entry carries no %q, which section 4.2 requires of every ARD entry", r.term)
		}
	}
}

func checkIdentifier(e Entry, report *Report) (URN, bool) {
	if e.Identifier == "" {
		return URN{}, false
	}
	identifier, err := ParseURN(e.Identifier)
	if err != nil {
		report.fail(TermIdentifier, IssueBadURN, sectionURN,
			"the identifier %q is not a discovery URN of the form urn:air:<publisher>:<namespace>:<agent-name>: %v",
			e.Identifier, err)
		return URN{}, false
	}
	if identifier.Legacy {
		report.warn(TermIdentifier, IssueLegacyURNPrefix, sectionURN,
			"the identifier %q starts with the predecessor prefix %q, which ADR-0009 replaced with %q: this library reads it, and a writer must emit %q",
			e.Identifier, URNPrefixLegacy, URNPrefix, identifier.String())
	}
	return identifier, true
}

func checkValueOrReference(e Entry, report *Report) {
	hasURL := e.URL != ""
	hasData := len(e.Data) > 0
	switch {
	case hasURL && hasData:
		report.fail(TermURL, IssueValueOrReference, sectionValueOrReference,
			"the entry carries both %q (%s) and %q (%s), and section 4.3 allows exactly one",
			TermURL, e.URL, TermData, e.Data)
	case !hasURL && !hasData:
		report.fail(TermURL, IssueValueOrReference, sectionValueOrReference,
			"the entry carries neither %q nor %q, and section 4.3 demands exactly one", TermURL, TermData)
	}
}

func (v Validator) checkTrustManifest(e Entry, identifier URN, parsed bool, report *Report) {
	if e.TrustManifest == nil {
		return
	}
	identity := strings.TrimSpace(e.TrustManifest.Identity)
	path := TermTrustManifest + "." + trustIdentityMember
	if identity == "" {
		report.fail(path, IssueMissingIdentity, sectionTrust,
			"the %q of the entry carries an empty %q, and section 4.5 requires that member",
			TermTrustManifest, trustIdentityMember)
		return
	}
	if !parsed {
		return
	}
	domain, err := TrustDomain(identity)
	if err != nil {
		report.fail(path, IssueAuthorityMismatch, sectionAuthorityBinding,
			"the identity %q names no trust domain to bind to the publisher %q of section 4.5.1: %v",
			identity, identifier.Publisher, err)
		return
	}
	if !SameAuthority(identifier.Publisher, domain, v.AllowSubdomainAuthority) {
		report.fail(path, IssueAuthorityMismatch, sectionAuthorityBinding,
			"the identity %q has the trust domain %q, which does not satisfy the publisher %q of the identifier %q",
			identity, domain, identifier.Publisher, e.Identifier)
	}
}

func checkRepresentativeQueries(e Entry, report *Report) {
	if e.RepresentativeQueries == nil {
		report.warn(TermRepresentativeQueries, IssueNoQueries, sectionDiscoveryConstraints,
			"the entry carries no %q: it stays a valid catalog entry, but it cannot be found by search, because a registry builds its semantic index from that term",
			TermRepresentativeQueries)
		return
	}
	count := len(e.RepresentativeQueries)
	if count < minRepresentativeQueries || count > maxRepresentativeQueries {
		report.warn(TermRepresentativeQueries, IssueQueryCount, sectionDiscoveryConstraints,
			"%q holds %d queries, and sections 4.2 and D.2 recommend %d to %d for the semantic index",
			TermRepresentativeQueries, count, minRepresentativeQueries, maxRepresentativeQueries)
	}
}

func checkNodeIdentifier(e Entry, report *Report) {
	if e.ID == "" || e.Identifier == "" || e.ID == e.Identifier {
		return
	}
	report.warn(TermID, IssueIDMismatch, sectionURN,
		"the %q %q differs from the %q %q, and appendix C demands that both denote the same resource",
		TermID, e.ID, TermIdentifier, e.Identifier)
}

func checkUpdatedAt(e Entry, report *Report) {
	if e.UpdatedAt == "" {
		return
	}
	if _, err := e.UpdatedAtTime(); err != nil {
		report.warn(TermUpdatedAt, IssueBadTimestamp, sectionEntrySchema,
			"the %q %q is not an RFC 3339 timestamp: %v", TermUpdatedAt, e.UpdatedAt, err)
	}
}

func checkEntryContext(e Entry, report *Report) {
	if _, err := e.Context.Prefixes(); err != nil {
		report.warn(TermContext, IssueBadContext, sectionJSONLD,
			"the %q of the entry cannot be read, so its prefixes bind no term: %v", TermContext, err)
	}
}
