// Package discover implements the discovery mechanisms of section 5.1 of the Agentic
// Resource Discovery specification, version 0.91.
//
// Resolver performs the consumer resolution of a domain: it fetches the well-known
// manifest, and may additionally consult the predecessor path. ScanHTML reads the link
// relation and the in-page JSON-LD of a document. ScanRobots reads the Agentmap
// directive of a robots.txt. The DNS mechanism is declared as the DNSProber interface,
// because the standard library cannot query the SVCB records that section 5.1 names.
//
// This package resolves; it does not validate. It returns the manifest and the entries
// it finds, and the caller decides whether they conform to section 4 and appendix D.2.
// A body counts as a manifest when it decodes as a JSON object, not when it holds a
// valid entries array.
//
// ScanHTML is a narrow scanner, not an HTML5 parser. It finds link and script elements
// by name and reads their attributes; it knows nothing of the document tree, of
// character references, of foreign content, or of the error recovery that the HTML
// standard prescribes. It never fails on malformed input: it reports what it can read
// and ignores the rest. Use a real parser when the document matters more than the two
// elements ARD cares about.
package discover
