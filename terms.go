package ard

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ContextLoader fetches a remote context by its IRI. A resolver with no loader resolves
// the base context only, and reports any other remote context as unresolved.
type ContextLoader func(iri string) (json.RawMessage, error)

// TermResolver resolves a term or a filter key to its IRI, through the base context of
// section 4.1 and the contexts of the document. See section 5.3.1.
type TermResolver struct {
	loader  ContextLoader
	sources []Context
	terms   map[string]string
	prefix  map[string]string
	vocab   string
}

// NewTermResolver builds a resolver. The base context applies first, then each context
// in order, so a later context overrides an earlier one.
func NewTermResolver(contexts ...Context) (*TermResolver, error) {
	r := &TermResolver{sources: contexts}
	bound, err := r.bind()
	if err != nil {
		return nil, err
	}
	r.adopt(bound)
	return r, nil
}

// WithLoader gives the resolver a way to fetch a remote context.
func (r *TermResolver) WithLoader(l ContextLoader) *TermResolver {
	r.loader = l
	if bound, err := r.bind(); err == nil {
		r.adopt(bound)
	}
	return r
}

func (r *TermResolver) bind() (*termBindings, error) {
	bound := newTermBindings()
	if err := bound.applyContext(baseContext, r.loader, 0); err != nil {
		return nil, err
	}
	for _, c := range r.sources {
		if err := bound.applyContext(c, r.loader, 0); err != nil {
			return nil, err
		}
	}
	return bound, nil
}

func (r *TermResolver) adopt(bound *termBindings) {
	r.terms, r.prefix, r.vocab = bound.terms, bound.prefix, bound.vocab
}

// Resolve gives the IRI of one term. It reports false for a term that no context binds.
func (r *TermResolver) Resolve(term string) (string, bool) {
	if term == "" {
		return "", false
	}
	if iri, ok := r.terms[term]; ok {
		return iri, true
	}
	if i := strings.Index(term, ":"); i > 0 {
		head, tail := term[:i], term[i+1:]
		if namespace, ok := r.prefix[head]; ok {
			return namespace + tail, true
		}
		if strings.HasPrefix(tail, "//") {
			return term, true
		}
		return "", false
	}
	if r.vocab != "" {
		return r.vocab + term, true
	}
	return "", false
}

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
func (r *TermResolver) ResolvePath(key string) (TermPath, error) {
	if key == "" {
		return TermPath{}, fmt.Errorf("ard: the filter key is empty")
	}
	if iri, ok := r.terms[key]; ok {
		return TermPath{Key: key, IRI: iri, Term: key}, nil
	}
	if strings.Contains(key, "://") {
		if iri, ok := r.Resolve(key); ok {
			return TermPath{Key: key, IRI: iri, Term: key}, nil
		}
	}
	segments := strings.Split(key, ".")
	for _, segment := range segments {
		if segment == "" {
			return TermPath{}, fmt.Errorf("ard: filter key %q has an empty path segment", key)
		}
	}
	iri, ok := r.Resolve(segments[0])
	if !ok {
		return TermPath{}, fmt.Errorf("ard: filter key %q starts with the term %q, which no context binds", key, segments[0])
	}
	resolved := TermPath{Key: key, IRI: iri, Term: segments[0]}
	if len(segments) > 1 {
		resolved.Rest = segments[1:]
	}
	return resolved, nil
}

const (
	contextKeywordVocab  = "@vocab"
	contextKeywordPrefix = "@prefix"
	maxContextDepth      = 8
)

var baseContext = embeddedBaseContext(baseContextDocument)

func embeddedBaseContext(document []byte) Context {
	var wrapper struct {
		Context json.RawMessage `json:"@context"`
	}
	if err := json.Unmarshal(document, &wrapper); err != nil {
		panic("ard: the embedded base context is not JSON: " + err.Error())
	}
	if len(wrapper.Context) == 0 {
		panic("ard: the embedded base context has no @context member")
	}
	return RawContext(wrapper.Context)
}

type termBindings struct {
	terms  map[string]string
	prefix map[string]string
	vocab  string
}

func newTermBindings() *termBindings {
	return &termBindings{terms: map[string]string{}, prefix: map[string]string{}}
}

func (b *termBindings) prefixBindings() map[string]string {
	bound := make(map[string]string, len(b.prefix)+1)
	for name, namespace := range b.prefix {
		bound[name] = namespace
	}
	if b.vocab != "" {
		bound[""] = b.vocab
	}
	return bound
}

func (b *termBindings) applyContext(c Context, loader ContextLoader, depth int) error {
	if c.IsZero() {
		return nil
	}
	return b.applyValue(c.Raw(), loader, depth)
}

func (b *termBindings) applyValue(raw json.RawMessage, loader ContextLoader, depth int) error {
	body := strings.TrimSpace(string(raw))
	if body == "" || body == "null" || depth > maxContextDepth {
		return nil
	}
	switch body[0] {
	case '"':
		var iri string
		if err := json.Unmarshal(raw, &iri); err != nil {
			return fmt.Errorf("ard: bad context reference: %w", err)
		}
		return b.applyReference(iri, loader, depth)
	case '{':
		var object map[string]json.RawMessage
		if err := json.Unmarshal(raw, &object); err != nil {
			return fmt.Errorf("ard: bad context object: %w", err)
		}
		if inner, ok := object[TermContext]; ok && len(object) == 1 {
			return b.applyValue(inner, loader, depth)
		}
		b.applyObject(object)
		return nil
	case '[':
		var elements []json.RawMessage
		if err := json.Unmarshal(raw, &elements); err != nil {
			return fmt.Errorf("ard: bad context array: %w", err)
		}
		for _, element := range elements {
			if err := b.applyValue(element, loader, depth); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("ard: a context must be a string, an object or an array, not %s", body)
}

func (b *termBindings) applyReference(iri string, loader ContextLoader, depth int) error {
	if iri == BaseContextIRI {
		return b.applyValue(baseContext.Raw(), loader, depth+1)
	}
	if loader == nil {
		return nil
	}
	fetched, err := loader(iri)
	if err != nil || len(fetched) == 0 {
		return nil
	}
	return b.applyValue(fetched, loader, depth+1)
}

func (b *termBindings) applyObject(object map[string]json.RawMessage) {
	b.collectPrefixes(object)
	b.collectTerms(object)
}

func (b *termBindings) collectPrefixes(object map[string]json.RawMessage) {
	pending := map[string]string{}
	for name, raw := range object {
		if name == contextKeywordVocab {
			if iri, ok := contextString(raw); ok {
				pending[""] = iri
			}
			continue
		}
		if strings.HasPrefix(name, "@") {
			continue
		}
		if iri, ok := contextString(raw); ok {
			if isNamespaceIRI(iri) {
				pending[name] = iri
			}
			continue
		}
		definition, ok := contextObject(raw)
		if !ok {
			continue
		}
		iri, ok := contextString(definition[TermID])
		if !ok {
			continue
		}
		if contextTrue(definition[contextKeywordPrefix]) || isNamespaceIRI(iri) {
			pending[name] = iri
		}
	}
	b.installPrefixes(pending)
}

func (b *termBindings) installPrefixes(pending map[string]string) {
	for range len(pending) + 1 {
		settled := true
		for name, iri := range pending {
			expanded := b.expandAgainst(pending, iri)
			if expanded != iri {
				pending[name] = expanded
				settled = false
			}
		}
		if settled {
			break
		}
	}
	for name, iri := range pending {
		if name == "" {
			b.vocab = iri
			continue
		}
		b.prefix[name] = iri
	}
}

func (b *termBindings) expandAgainst(pending map[string]string, iri string) string {
	head, tail, ok := compactParts(iri)
	if !ok {
		return iri
	}
	if namespace, found := pending[head]; found && namespace != iri {
		return namespace + tail
	}
	if namespace, found := b.prefix[head]; found {
		return namespace + tail
	}
	return iri
}

func (b *termBindings) collectTerms(object map[string]json.RawMessage) {
	for name, raw := range object {
		if strings.HasPrefix(name, "@") {
			continue
		}
		if contextNull(raw) {
			delete(b.terms, name)
			delete(b.prefix, name)
			continue
		}
		if iri, ok := contextString(raw); ok {
			b.terms[name] = b.expandTermValue(iri)
			continue
		}
		definition, ok := contextObject(raw)
		if !ok {
			continue
		}
		if iri, ok := contextString(definition[TermID]); ok {
			b.terms[name] = b.expandTermValue(iri)
			continue
		}
		if b.vocab != "" {
			b.terms[name] = b.vocab + name
		}
	}
}

func (b *termBindings) expandTermValue(iri string) string {
	if strings.HasPrefix(iri, "@") {
		return iri
	}
	head, tail, ok := compactParts(iri)
	if !ok {
		if strings.Contains(iri, ":") || b.vocab == "" {
			return iri
		}
		return b.vocab + iri
	}
	if namespace, found := b.prefix[head]; found {
		return namespace + tail
	}
	return iri
}

func compactParts(iri string) (string, string, bool) {
	i := strings.Index(iri, ":")
	if i <= 0 {
		return "", "", false
	}
	head, tail := iri[:i], iri[i+1:]
	if strings.HasPrefix(tail, "//") {
		return "", "", false
	}
	return head, tail, true
}

func isNamespaceIRI(iri string) bool {
	if iri == "" || strings.HasPrefix(iri, "@") {
		return false
	}
	return strings.ContainsRune("#/:?[]@", rune(iri[len(iri)-1]))
}

func contextString(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return value, true
}

func contextObject(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, false
	}
	return value, true
}

func contextTrue(raw json.RawMessage) bool {
	var value bool
	return len(raw) > 0 && json.Unmarshal(raw, &value) == nil && value
}

func contextNull(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == "null"
}
