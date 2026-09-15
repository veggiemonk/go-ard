package ard

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/veggiemonk/go-ard/internal/jsonx"
)

const (
	memberIdentity            = "identity"
	memberIdentityType        = "identityType"
	memberTrustSchema         = "trustSchema"
	memberAttestations        = "attestations"
	memberProvenance          = "provenance"
	memberSignature           = "signature"
	memberGovernanceURI       = "governanceUri"
	memberVerificationMethods = "verificationMethods"
	memberURI                 = "uri"
	memberMediaType           = "mediaType"
	memberDigest              = "digest"
	memberRelation            = "relation"
	memberSourceID            = "sourceId"
	memberSourceDigest        = "sourceDigest"
	memberText                = "text"
	memberFilter              = "filter"
)

var (
	manifestTerms       = []string{TermEntries}
	resultTerms         = slices.Concat(EntryTerms, []string{TermScore, TermSource})
	trustManifestTerms  = []string{memberIdentity, memberIdentityType, memberTrustSchema, memberAttestations, memberProvenance, memberSignature}
	trustSchemaTerms    = []string{TermIdentifier, TermVersion, memberGovernanceURI, memberVerificationMethods}
	attestationTerms    = []string{TermType, memberURI, memberMediaType, memberDigest}
	provenanceLinkTerms = []string{memberRelation, memberSourceID, memberSourceDigest}
)

// MarshalJSON writes the named terms and then merges Extra. It reports an error when a
// key of Extra names a term that has its own field, because that would lose data.
func (e Entry) MarshalJSON() ([]byte, error) {
	out, err := e.builder().Build(e.Extra, EntryTerms)
	if err != nil {
		return nil, fmt.Errorf("ard: entry %q: %w", e.Identifier, err)
	}
	return out, nil
}

// UnmarshalJSON reads the named terms and keeps every other term in Extra.
func (e *Entry) UnmarshalJSON(b []byte) error {
	members, err := objectMembers(b)
	if err != nil {
		return fmt.Errorf("ard: entry: %w", err)
	}
	return e.fromMembers(members)
}

func (e Entry) builder() *jsonx.Builder {
	b := &jsonx.Builder{}
	b.Raw(TermContext, e.Context.Raw())
	b.String(TermID, e.ID)
	b.String(TermIdentifier, e.Identifier)
	b.String(TermDisplayName, e.DisplayName)
	b.String(TermType, e.Type)
	b.String(TermURL, e.URL)
	b.Raw(TermData, e.Data)
	b.Strings(TermRepresentativeQueries, e.RepresentativeQueries)
	b.Strings(TermCapabilities, e.Capabilities)
	b.String(TermDescription, e.Description)
	b.Strings(TermTags, e.Tags)
	b.String(TermVersion, e.Version)
	b.String(TermUpdatedAt, e.UpdatedAt)
	if e.Metadata != nil {
		b.Value(TermMetadata, e.Metadata)
	}
	if e.TrustManifest != nil {
		b.Value(TermTrustManifest, e.TrustManifest)
	}
	return b
}

func (e *Entry) fromMembers(members map[string]json.RawMessage) error {
	*e = Entry{}
	err := takeMembers(members, []target{
		{TermContext, &e.Context},
		{TermID, &e.ID},
		{TermIdentifier, &e.Identifier},
		{TermDisplayName, &e.DisplayName},
		{TermType, &e.Type},
		{TermURL, &e.URL},
		{TermData, &e.Data},
		{TermRepresentativeQueries, &e.RepresentativeQueries},
		{TermCapabilities, &e.Capabilities},
		{TermDescription, &e.Description},
		{TermTags, &e.Tags},
		{TermVersion, &e.Version},
		{TermUpdatedAt, &e.UpdatedAt},
		{TermMetadata, &e.Metadata},
		{TermTrustManifest, &e.TrustManifest},
	})
	if err != nil {
		return fmt.Errorf("ard: entry: %w", err)
	}
	e.Extra = remaining(members)
	return nil
}

// MarshalJSON writes the entries member and then merges Extra.
func (m Manifest) MarshalJSON() ([]byte, error) {
	b := &jsonx.Builder{}
	if m.Entries != nil {
		b.Value(TermEntries, m.Entries)
	}
	out, err := b.Build(m.Extra, manifestTerms)
	if err != nil {
		return nil, fmt.Errorf("ard: manifest: %w", err)
	}
	return out, nil
}

// UnmarshalJSON reads the entries member and keeps every other top-level member in Extra.
func (m *Manifest) UnmarshalJSON(b []byte) error {
	members, err := objectMembers(b)
	if err != nil {
		return fmt.Errorf("ard: manifest: %w", err)
	}
	*m = Manifest{}
	if err := takeMembers(members, []target{{TermEntries, &m.Entries}}); err != nil {
		return fmt.Errorf("ard: manifest: %w", err)
	}
	m.Extra = remaining(members)
	return nil
}

// MarshalJSON writes the named members of the trust envelope and then merges Extra.
func (t TrustManifest) MarshalJSON() ([]byte, error) {
	b := &jsonx.Builder{}
	b.String(memberIdentity, t.Identity)
	b.String(memberIdentityType, t.IdentityType)
	if t.TrustSchema != nil {
		b.Value(memberTrustSchema, t.TrustSchema)
	}
	if t.Attestations != nil {
		b.Value(memberAttestations, t.Attestations)
	}
	if t.Provenance != nil {
		b.Value(memberProvenance, t.Provenance)
	}
	b.String(memberSignature, t.Signature)
	out, err := b.Build(t.Extra, trustManifestTerms)
	if err != nil {
		return nil, fmt.Errorf("ard: trust manifest %q: %w", t.Identity, err)
	}
	return out, nil
}

// UnmarshalJSON reads the named members of the trust envelope and keeps the rest in Extra.
func (t *TrustManifest) UnmarshalJSON(b []byte) error {
	members, err := objectMembers(b)
	if err != nil {
		return fmt.Errorf("ard: trust manifest: %w", err)
	}
	*t = TrustManifest{}
	err = takeMembers(members, []target{
		{memberIdentity, &t.Identity},
		{memberIdentityType, &t.IdentityType},
		{memberTrustSchema, &t.TrustSchema},
		{memberAttestations, &t.Attestations},
		{memberProvenance, &t.Provenance},
		{memberSignature, &t.Signature},
	})
	if err != nil {
		return fmt.Errorf("ard: trust manifest: %w", err)
	}
	t.Extra = remaining(members)
	return nil
}

// MarshalJSON writes the named members of the trust schema and then merges Extra.
func (t TrustSchema) MarshalJSON() ([]byte, error) {
	b := &jsonx.Builder{}
	b.String(TermIdentifier, t.Identifier)
	b.String(TermVersion, t.Version)
	b.String(memberGovernanceURI, t.GovernanceURI)
	b.Strings(memberVerificationMethods, t.VerificationMethods)
	out, err := b.Build(t.Extra, trustSchemaTerms)
	if err != nil {
		return nil, fmt.Errorf("ard: trust schema %q: %w", t.Identifier, err)
	}
	return out, nil
}

// UnmarshalJSON reads the named members of the trust schema and keeps the rest in Extra.
func (t *TrustSchema) UnmarshalJSON(b []byte) error {
	members, err := objectMembers(b)
	if err != nil {
		return fmt.Errorf("ard: trust schema: %w", err)
	}
	*t = TrustSchema{}
	err = takeMembers(members, []target{
		{TermIdentifier, &t.Identifier},
		{TermVersion, &t.Version},
		{memberGovernanceURI, &t.GovernanceURI},
		{memberVerificationMethods, &t.VerificationMethods},
	})
	if err != nil {
		return fmt.Errorf("ard: trust schema: %w", err)
	}
	t.Extra = remaining(members)
	return nil
}

// MarshalJSON writes the named members of the attestation and then merges Extra.
func (a Attestation) MarshalJSON() ([]byte, error) {
	b := &jsonx.Builder{}
	b.String(TermType, a.Type)
	b.String(memberURI, a.URI)
	b.String(memberMediaType, a.MediaType)
	b.String(memberDigest, a.Digest)
	out, err := b.Build(a.Extra, attestationTerms)
	if err != nil {
		return nil, fmt.Errorf("ard: attestation %q: %w", a.Type, err)
	}
	return out, nil
}

// UnmarshalJSON reads the named members of the attestation and keeps the rest in Extra.
func (a *Attestation) UnmarshalJSON(b []byte) error {
	members, err := objectMembers(b)
	if err != nil {
		return fmt.Errorf("ard: attestation: %w", err)
	}
	*a = Attestation{}
	err = takeMembers(members, []target{
		{TermType, &a.Type},
		{memberURI, &a.URI},
		{memberMediaType, &a.MediaType},
		{memberDigest, &a.Digest},
	})
	if err != nil {
		return fmt.Errorf("ard: attestation: %w", err)
	}
	a.Extra = remaining(members)
	return nil
}

// MarshalJSON writes the named members of the provenance link and then merges Extra.
func (p ProvenanceLink) MarshalJSON() ([]byte, error) {
	b := &jsonx.Builder{}
	b.String(memberRelation, p.Relation)
	b.String(memberSourceID, p.SourceID)
	b.String(memberSourceDigest, p.SourceDigest)
	out, err := b.Build(p.Extra, provenanceLinkTerms)
	if err != nil {
		return nil, fmt.Errorf("ard: provenance link %q: %w", p.SourceID, err)
	}
	return out, nil
}

// UnmarshalJSON reads the named members of the provenance link and keeps the rest in Extra.
func (p *ProvenanceLink) UnmarshalJSON(b []byte) error {
	members, err := objectMembers(b)
	if err != nil {
		return fmt.Errorf("ard: provenance link: %w", err)
	}
	*p = ProvenanceLink{}
	err = takeMembers(members, []target{
		{memberRelation, &p.Relation},
		{memberSourceID, &p.SourceID},
		{memberSourceDigest, &p.SourceDigest},
	})
	if err != nil {
		return fmt.Errorf("ard: provenance link: %w", err)
	}
	p.Extra = remaining(members)
	return nil
}

// MarshalJSON writes the entry terms of the result and adds score and source.
func (r Result) MarshalJSON() ([]byte, error) {
	b := r.Entry.builder()
	if r.Score != nil {
		b.Value(TermScore, *r.Score)
	}
	b.String(TermSource, r.Source)
	out, err := b.Build(r.Entry.Extra, resultTerms)
	if err != nil {
		return nil, fmt.Errorf("ard: result %q: %w", r.Entry.Identifier, err)
	}
	return out, nil
}

// UnmarshalJSON reads a result. Section 5.3.2 requires only the identifier, so a missing
// score or source is not an error.
func (r *Result) UnmarshalJSON(b []byte) error {
	members, err := objectMembers(b)
	if err != nil {
		return fmt.Errorf("ard: result: %w", err)
	}
	*r = Result{}
	err = takeMembers(members, []target{
		{TermScore, &r.Score},
		{TermSource, &r.Source},
	})
	if err != nil {
		return fmt.Errorf("ard: result: %w", err)
	}
	return r.Entry.fromMembers(members)
}

// MarshalJSON writes a filter value as an array.
func (q Query) MarshalJSON() ([]byte, error) {
	filter, err := encodeFilter(q.Filter)
	if err != nil {
		return nil, fmt.Errorf("ard: query: %w", err)
	}
	b := &jsonx.Builder{}
	b.Raw(TermContext, q.Context.Raw())
	b.String(memberText, q.Text)
	b.Raw(memberFilter, filter)
	out, err := b.Build(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("ard: query: %w", err)
	}
	return out, nil
}

// UnmarshalJSON reads a query. A bare scalar filter value becomes a one-element array.
func (q *Query) UnmarshalJSON(b []byte) error {
	members, err := objectMembers(b)
	if err != nil {
		return fmt.Errorf("ard: query: %w", err)
	}
	*q = Query{}
	err = takeMembers(members, []target{
		{TermContext, &q.Context},
		{memberText, &q.Text},
	})
	if err != nil {
		return fmt.Errorf("ard: query: %w", err)
	}
	q.Filter, err = decodeFilter(members[memberFilter])
	if err != nil {
		return fmt.Errorf("ard: query: %w", err)
	}
	return nil
}

func encodeFilter(filter map[string][]string) (json.RawMessage, error) {
	if len(filter) == 0 {
		return nil, nil
	}
	b := &jsonx.Builder{}
	for _, key := range slices.Sorted(maps.Keys(filter)) {
		b.Value(key, append([]string{}, filter[key]...))
	}
	return b.Build(nil, nil)
}

func decodeFilter(raw json.RawMessage) (map[string][]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw, &members); err != nil {
		return nil, fmt.Errorf("member %q: %w", memberFilter, err)
	}
	if len(members) == 0 {
		return nil, nil
	}
	filter := make(map[string][]string, len(members))
	for key, value := range members {
		values, err := filterValues(value)
		if err != nil {
			return nil, fmt.Errorf("filter key %q: %w", key, err)
		}
		filter[key] = values
	}
	return filter, nil
}

func filterValues(raw json.RawMessage) ([]string, error) {
	var array []json.RawMessage
	if err := json.Unmarshal(raw, &array); err != nil {
		value, err := filterScalar(raw)
		if err != nil {
			return nil, err
		}
		return []string{value}, nil
	}
	values := make([]string, 0, len(array))
	for _, element := range array {
		value, err := filterScalar(element)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func filterScalar(raw json.RawMessage) (string, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	switch scalar := value.(type) {
	case string:
		return scalar, nil
	case float64, bool:
		return string(raw), nil
	}
	return "", fmt.Errorf("a filter value holds a string, a number or a boolean, not %s", raw)
}

type target struct {
	name string
	dst  any
}

func takeMembers(members map[string]json.RawMessage, targets []target) error {
	var errs []error
	for _, t := range targets {
		raw, ok := members[t.name]
		if !ok {
			continue
		}
		delete(members, t.name)
		if err := json.Unmarshal(raw, t.dst); err != nil {
			errs = append(errs, fmt.Errorf("member %q: %w", t.name, err))
		}
	}
	return errors.Join(errs...)
}

func objectMembers(b []byte) (map[string]json.RawMessage, error) {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(b, &members); err != nil {
		return nil, err
	}
	if members == nil {
		members = map[string]json.RawMessage{}
	}
	return members, nil
}

func remaining(members map[string]json.RawMessage) map[string]json.RawMessage {
	if len(members) == 0 {
		return nil
	}
	return members
}
