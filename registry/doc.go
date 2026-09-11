// Package registry implements the registry REST API of section 5.3 of the Agentic
// Resource Discovery specification, version 0.91.
//
// Client calls a registry: Search, Explore and List, with SearchAll to walk the pages
// of a search. Handler serves the same three endpoints over an Index, which is the
// interface a search back end implements. Federator answers the federation modes of
// section 5.4.
//
// The handler resolves every filter key and facet field of section 5.3.1 before it
// calls the index, so that an index matches by IRI and never repeats the term
// resolution. An index reports a term path it does not index with ErrUnsupportedFilter,
// and the handler answers 400, as section 5.3.1 allows.
//
// Explore and List are optional. An index that implements neither returns
// ard.ErrNotImplemented and the handler answers 501, as section 5.3.3 prescribes.
//
// The subpackage memindex holds an in-memory index, so that a registry runs with no
// database behind it.
package registry
