package ard

import (
	"encoding/json"
	"fmt"
	"time"
)

// Term names of the default namespace that this package reads by name.
const (
	TermContext               = "@context"
	TermID                    = "@id"
	TermIdentifier            = "identifier"
	TermDisplayName           = "displayName"
	TermType                  = "type"
	TermURL                   = "url"
	TermData                  = "data"
	TermRepresentativeQueries = "representativeQueries"
	TermCapabilities          = "capabilities"
	TermDescription           = "description"
	TermTags                  = "tags"
	TermVersion               = "version"
	TermUpdatedAt             = "updatedAt"
	TermMetadata              = "metadata"
	TermTrustManifest         = "trustManifest"
	TermEntries               = "entries"
	TermScore                 = "score"
	TermSource                = "source"
)

// TermPublisher is the filter key a registry derives from the publisher segment of the
// entry identifier. No entry carries it as a term. See section 5.3.1.
const TermPublisher = "publisher"

// EntryTerms lists every term the Entry struct holds in a named field.
var EntryTerms = []string{
	TermContext, TermID, TermIdentifier, TermDisplayName, TermType, TermURL, TermData,
	TermRepresentativeQueries, TermCapabilities, TermDescription, TermTags, TermVersion,
	TermUpdatedAt, TermMetadata, TermTrustManifest,
}

// Entry is an ARD entry: the description of one agentic resource in a form that search
// can find. See section 4.
//
// Extra holds every term that has no named field, so that a decode followed by an encode
// loses nothing. Section 5.3.1 requires a consumer to keep the terms it does not
// recognize.
type Entry struct {
	Context               Context
	ID                    string
	Identifier            string
	DisplayName           string
	Type                  string
	URL                   string
	Data                  json.RawMessage
	RepresentativeQueries []string
	Capabilities          []string
	Description           string
	Tags                  []string
	Version               string
	UpdatedAt             string
	Metadata              map[string]any
	TrustManifest         *TrustManifest
	Extra                 map[string]json.RawMessage
}

// UpdatedAtTime parses UpdatedAt as an RFC 3339 timestamp.
func (e Entry) UpdatedAtTime() (time.Time, error) {
	if e.UpdatedAt == "" {
		return time.Time{}, fmt.Errorf("ard: entry %q has no %s", e.Identifier, TermUpdatedAt)
	}
	return time.Parse(time.RFC3339, e.UpdatedAt)
}

// URN parses the entry identifier.
func (e Entry) URN() (URN, error) { return ParseURN(e.Identifier) }

// Manifest is the document published at /.well-known/ard.json. ARD reads only the
// entries member; every other top-level member is transport defined and kept in Extra.
// See section 5.1.
type Manifest struct {
	Entries []Entry
	Extra   map[string]json.RawMessage
}

// TrustManifest is the identity, compliance and provenance envelope of section 4.5. ARD
// reads only Identity. The envelope stays open, so Extra keeps the rest.
type TrustManifest struct {
	Identity     string
	IdentityType string
	TrustSchema  *TrustSchema
	Attestations []Attestation
	Provenance   []ProvenanceLink
	Signature    string
	Extra        map[string]json.RawMessage
}

// TrustSchema names the trust framework of a trust manifest and the source of its
// verification procedure. See section 4.5.2.
type TrustSchema struct {
	Identifier          string
	Version             string
	GovernanceURI       string
	VerificationMethods []string
	Extra               map[string]json.RawMessage
}

// Attestation is a verifiable claim carried by a trust manifest.
type Attestation struct {
	Type      string
	URI       string
	MediaType string
	Digest    string
	Extra     map[string]json.RawMessage
}

// ProvenanceLink is one step of the lineage trail of an artifact.
type ProvenanceLink struct {
	Relation     string
	SourceID     string
	SourceDigest string
	Extra        map[string]json.RawMessage
}
