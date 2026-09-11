# go-ard

A Go library for the [Agentic Resource Discovery](https://agenticresourcediscovery.org/spec/)
specification, version 0.91.

It reads, writes and validates ARD entries, resolves a publisher the way a conformant
consumer does, calls a registry through the REST API, and runs one.

**The standard library is the only dependency.** There is no external module, in the
library or in its tests.

```
go get github.com/veggiemonk/go-ard
```

## Packages

| Package | What it gives | Specification |
| :--- | :--- | :--- |
| `ard` | the entry model, the JSON codec, URN parsing, term resolution, validation, the API payloads | 4, 5.3.1, D.1, D.2, B, C |
| `ard/discover` | resolution of a domain to its entries | 5.1 |
| `ard/registry` | the REST client, the server handler, the `Index` interface, federation | 5.3, 5.4 |
| `ard/registry/memindex` | an in-memory index, so a registry runs out of the box | 5.3, A |
| `cmd/ard` | a conformance CLI: validate, resolve, probe, serve | D.4 |

## The entry model

An ARD entry is open: the schema sets `additionalProperties: true`, and section 5.3.1
asks a consumer to keep the terms it does not recognize. So `Entry` holds the terms of
the default namespace in named fields and every other term in `Extra`. A decode followed
by an encode loses nothing.

```go
var manifest ard.Manifest
if err := json.Unmarshal(raw, &manifest); err != nil {
	return err
}
for _, entry := range manifest.Entries {
	fmt.Println(entry.Identifier, entry.DisplayName, entry.Extra["acme:serviceTier"])
}
```

## Validation

`Validate` and `ValidateManifest` give a report that follows appendix D.2: an error means
the document does not conform, a warning means it conforms but loses something.

```go
report := ard.ValidateManifest(manifest)
for _, issue := range report.Warnings {
	log.Printf("%s: %s (section %s)", issue.Path, issue.Message, issue.Section)
}
if err := report.Err(); err != nil {
	return err
}
```

A missing `representativeQueries` is a warning, so output from existing tooling still
validates. The publisher authority binding of section 4.5.1 is an error: an entry that
claims `urn:air:acme.com:...` with an identity in another trust domain is rejected.

## Resolving a publisher

```go
result, err := discover.Resolve(ctx, "example.com")
```

It fetches `/.well-known/ard.json` first. When that gives nothing and the caller allows
it, it consults the predecessor path and warns, because a consumer is not required to
look there. `ScanHTML` and `ScanRobots` read a `rel="ard"` link, in-page JSON-LD, and the
`Agentmap` directive.

## Calling a registry

```go
client := &registry.Client{BaseURL: "https://registry.example.com/api"}
response, err := client.Search(ctx, ard.SearchRequest{
	Query:      ard.Query{Text: "find me a flight booking agent"},
	Federation: ard.FederationReferrals,
})
for result, err := range client.SearchAll(ctx, request) { ... }
```

Every answer that is not 200 becomes an `*ard.APIError` carrying the code of appendix B.
A 501 from Explore satisfies `errors.Is(err, ard.ErrNotImplemented)`.

## Running a registry

```go
index := memindex.New(manifest.Entries)
handler := registry.Handler(index, registry.Options{Source: "https://registry.example.com"})
http.ListenAndServe(":9010", handler)
```

The handler resolves every filter key to an IRI through the effective context of section
5.3.1 before it calls the index, so an index never repeats that work. Put your own store
behind the `registry.Index` interface and the handler stays the same.

## The CLI

```
ard validate <file|url>       check a manifest or a single entry
ard resolve  <domain>         resolve a publisher as a consumer does
ard probe    <registry-url>   probe a live registry REST API
ard serve    <manifest.json>  serve a registry over the entries of a manifest
```

It is a second implementation of the conformance modes of the official Python tool, and
the two agree:

- both report 0 errors and 2 warnings on `conformance/examples/basic/ard.json`, and 0 and
  0 on the other three examples;
- the official Python tool passes all three probes against `ard serve`;
- this CLI passes all three probes against the Python mock registry;
- a manifest written by this library passes the Python tool.

## Limits

- **Term resolution is not full JSON-LD processing.** The library resolves a term or a
  filter key to an IRI through the base context and the context of the document, which is
  what sections 4.1 and 5.3.1 need. It does not expand, compact or frame a document, and
  it fetches no remote context unless the caller gives a `ContextLoader`.
- **The HTML scanner is a byte scanner, not an HTML5 parser.** It knows no document tree,
  so it honours a `<link>` anywhere, and it decodes no character reference. A dependency
  would fix this, and the library takes none.
- **DNS discovery needs a prober from the caller.** Section 5.1 uses SVCB records and the
  standard library cannot query them. The package gives the `DNSProber` seam and a clear
  `ErrSVCBUnsupported`, rather than a prober that looks right and is not.

## Findings against the specification

Writing this library surfaced nine places where the prose, the JSON Schema, the CDDL, the
OpenAPI file and the reference tool disagree. They are listed in
[docs/spec-findings.md](docs/spec-findings.md), with the side this library follows and
why. The specification stays the authority; the findings are reported upstream.

## License

Apache-2.0. See [LICENSE](LICENSE).
