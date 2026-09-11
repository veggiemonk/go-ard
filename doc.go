// Package ard implements the description layer of the Agentic Resource Discovery
// specification, version 0.91.
//
// It gives the entry model of section 4, the term resolution of section 5.3.1, the
// validation rules of section 4 and appendix D.2, and the payload types of the registry
// REST API of section 5.3.
//
// Term resolution is not a full JSON-LD processor. The package resolves a term or a
// filter key to an IRI through the base context and the context of the document. It does
// not expand, compact or frame a document, and it fetches no remote context unless the
// caller gives a ContextLoader.
package ard
