package ard

// Issue codes that validation reports.
const (
	IssueMissingTerm         = "missing_term"
	IssueBadURN              = "bad_urn"
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

// Issue is one finding of validation. Section names the clause of the specification that
// the finding comes from, such as "4.3" or "D.2".
type Issue struct {
	Path    string
	Code    string
	Section string
	Message string
}

// Report holds the findings of validation. Appendix D.2 separates the two severities: a
// warning leaves the document structurally valid.
type Report struct {
	Errors   []Issue
	Warnings []Issue
}

// OK reports whether the document has no error. Warnings do not change the result.
func (r Report) OK() bool { return len(r.Errors) == 0 }

// Err gives the errors of the report as one error, or nil.
func (r Report) Err() error { panic("ard: not implemented") }

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
func (v Validator) Entry(e Entry) Report { panic("ard: not implemented") }

// Manifest checks a manifest against section 5.1, and every entry in it.
func (v Validator) Manifest(m Manifest) Report { panic("ard: not implemented") }
