// Package memindex holds an in-memory index of ARD entries, so that a registry server
// runs with no database behind it.
//
// Index implements registry.Index: the search of section 5.3.2, the facets of section
// 5.3.3 and the deterministic listing of section 5.3.4 with the filter expression of
// appendix A.
//
// Matching follows section 5.3.1. A core or namespaced filter key matches by the IRI
// that the query context resolves it to, and an entry is indexed by the IRIs its own
// context binds, so a client finds the same entries whichever prefix it chose. A
// dot-path into trustManifest, metadata or data is a literal JSON path on the raw
// member. The publisher key is derived from the entry identifier.
//
// Relevance is token overlap, not a vector search: the index scores the tokens of the
// query text against the display name, the description, the representative queries, the
// tags and the capabilities of an entry, and normalizes the result to the 0 to 100 of
// section 5.3.2. One cutoff governs the matched set of Search and of Explore alike, as
// section 5.3.3 requires.
package memindex
