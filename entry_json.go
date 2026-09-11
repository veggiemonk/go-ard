package ard

import "encoding/json"

// MarshalJSON writes the named terms and then merges Extra. It reports an error when a
// key of Extra names a term that has its own field, because that would lose data.
func (e Entry) MarshalJSON() ([]byte, error) { panic("ard: not implemented") }

// UnmarshalJSON reads the named terms and keeps every other term in Extra.
func (e *Entry) UnmarshalJSON(b []byte) error { panic("ard: not implemented") }

// MarshalJSON writes the entries member and then merges Extra.
func (m Manifest) MarshalJSON() ([]byte, error) { panic("ard: not implemented") }

// UnmarshalJSON reads the entries member and keeps every other top-level member in Extra.
func (m *Manifest) UnmarshalJSON(b []byte) error { panic("ard: not implemented") }

func (t TrustManifest) MarshalJSON() ([]byte, error)   { panic("ard: not implemented") }
func (t *TrustManifest) UnmarshalJSON(b []byte) error  { panic("ard: not implemented") }
func (t TrustSchema) MarshalJSON() ([]byte, error)     { panic("ard: not implemented") }
func (t *TrustSchema) UnmarshalJSON(b []byte) error    { panic("ard: not implemented") }
func (a Attestation) MarshalJSON() ([]byte, error)     { panic("ard: not implemented") }
func (a *Attestation) UnmarshalJSON(b []byte) error    { panic("ard: not implemented") }
func (p ProvenanceLink) MarshalJSON() ([]byte, error)  { panic("ard: not implemented") }
func (p *ProvenanceLink) UnmarshalJSON(b []byte) error { panic("ard: not implemented") }

// MarshalJSON writes the entry terms of the result and adds score and source.
func (r Result) MarshalJSON() ([]byte, error) { panic("ard: not implemented") }

// UnmarshalJSON reads a result. Section 5.3.2 requires only the identifier, so a missing
// score or source is not an error.
func (r *Result) UnmarshalJSON(b []byte) error { panic("ard: not implemented") }

// MarshalJSON writes a filter value as an array.
func (q Query) MarshalJSON() ([]byte, error) { panic("ard: not implemented") }

// UnmarshalJSON reads a query. A bare scalar filter value becomes a one-element array.
func (q *Query) UnmarshalJSON(b []byte) error { panic("ard: not implemented") }

var _ = json.Marshal
