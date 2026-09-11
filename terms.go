package ard

import "encoding/json"

// ContextLoader fetches a remote context by its IRI. A resolver with no loader resolves
// the base context only, and reports any other remote context as unresolved.
type ContextLoader func(iri string) (json.RawMessage, error)

// TermResolver resolves a term or a filter key to its IRI, through the base context of
// section 4.1 and the contexts of the document. See section 5.3.1.
type TermResolver struct {
	loader ContextLoader
	terms  map[string]string
	prefix map[string]string
	vocab  string
}

// NewTermResolver builds a resolver. The base context applies first, then each context
// in order, so a later context overrides an earlier one.
func NewTermResolver(contexts ...Context) (*TermResolver, error) { panic("ard: not implemented") }

// WithLoader gives the resolver a way to fetch a remote context.
func (r *TermResolver) WithLoader(l ContextLoader) *TermResolver { panic("ard: not implemented") }

// Resolve gives the IRI of one term. It reports false for a term that no context binds.
func (r *TermResolver) Resolve(term string) (string, bool) { panic("ard: not implemented") }

// TermPath is a resolved filter key. Section 5.3.1 resolves the leading segment to an
// IRI and keeps the rest as a literal JSON path into a member that ARD does not expand.
type TermPath struct {
	Key  string
	IRI  string
	Term string
	Rest []string
}

// ResolvePath resolves a filter key of section 5.3.1, such as "type" or
// "trustManifest.attestations.type".
func (r *TermResolver) ResolvePath(key string) (TermPath, error) { panic("ard: not implemented") }
