# Findings against the ARD specification v0.91

This library implements the specification at
[ards-project/ard-spec](https://github.com/ards-project/ard-spec). While it was written,
the prose, the JSON Schema, the CDDL, the OpenAPI file and the reference Python tool were
compared against each other. The points below are the places where they disagree.

Each point says which side the library follows, and why. No point is a change to the
standard: the specification stays the authority, and these are reported upstream as
issues.

## 1. The schema cannot enforce `trustManifest.identity`

`spec/schemas/ard-entry.schema.json`, `$defs.EntryFields.properties`, spells the member
`TrustManifest` with a capital T. The prose (§4.2, §4.5), the CDDL
(`ard-entry-descriptive`) and `ard.context.jsonld` all spell it `trustManifest`.

Because `additionalProperties` is true, a correctly spelled `trustManifest` never matches
`$ref: #/$defs/TrustManifest`, so the `required: ["identity"]` of that definition never
fires. §4.5 says ARD requires only `trustManifest.identity`, and that one hard
requirement has no schema teeth.

**The library follows the prose.** The wire name is `trustManifest`. A present trust
manifest with an absent or empty `identity` is an error.

## 2. A search result requires only `identifier` in the prose, three members in the schemas

§5.3.2 says a result MUST carry `identifier` and that "every other term is at the
registry's discretion". The OpenAPI `SearchResultItem` lists `identifier`, `score` and
`source` as required, and the CDDL `search-result-item` makes `score` and `source`
mandatory too.

**The library follows the prose on the client side and the schemas on the server side.**
`Result.Score` is a pointer and `Source` may be empty, so a lean result from another
registry decodes without error. The server always emits all three, because it knows them.

## 3. The CDDL query model has no `@context`

§5.3.1 defines one query object with three members, `@context` among them, shared by
Search and Explore. The OpenAPI agrees: `QueryModel` carries `@context`. The CDDL gives
`@context` to `search-query-model` only, so an Explore request cannot bind a prefix,
although §5.3.1 says it can.

**The library follows the prose.** Both endpoints take the same query object.

## 4. The CDDL is stricter than the JSON Schema on empty arrays and facet counts

- `ard-entry-descriptive` demands non-empty `tags` and `capabilities` (`[+ tstr]`); the
  JSON Schema accepts `[]`.
- `explore-facet-bucket` makes `count` mandatory and `explore-facet-result.buckets` a
  non-empty array; §5.3.3 says a bucket SHOULD carry `count` and a registry MAY omit it.

**The library follows the JSON Schema and the prose.** An empty array survives a decode
and an encode unchanged, and `Bucket.Count` is a pointer.

## 5. The reference tool omits five checks that the prose mandates

`conformance/bin/conformance-test` does not check:

1. the publisher authority binding of §4.5.1, which appendix D.2 lists as a conformance
   check and §4.5.1 states as a MUST;
2. `@id` against `identifier`, which appendix C states as a MUST;
3. `updatedAt` against the `date-time` format of the schema;
4. a duplicate `identifier` across the entries of one manifest, on which appendix C item 5
   rests;
5. an unreadable `@context` (§4.1).

The first one matters most: an entry that claims `urn:air:acme.com:server:weather` with
the identity `spiffe://evil.com/x` passes the official tool with zero errors. That is
exactly the namespace squatting §4.5.1 names.

**The library follows the prose.** It reports the first as an error and the other four as
warnings.

## 6. The reference tool warns on a media type outside a hard-coded list

`conformance-test` warns when `type` is not one of nine hard-coded values. §3.3 and §4.2
say only that `type` is an IANA Media Type, and the schema adds that it "names what the
artifact is without constraining its internal schema". Neither limits the value to a
list. The list also omits `application/ai-skill+md`, so the third entry example of §4.4
draws a warning from the specification's own tool.

**The library follows the prose and the schema.** It does not implement this check. It is
the one warning the tool raises that the library does not.

## 7. Appendix D.2 gives no severity for the authority binding

The first bullet of appendix D.2 marks `representativeQueries` explicitly as a warning
and not an error. The second bullet, the publisher authority binding, carries no severity
mark at all.

**The library reads it as an error**, because §4.5.1 says the domains MUST align and that
a verifying registry rejects an entry that cannot produce a valid attestation.

## 8. `collections` is called legacy, but no ADR records its removal

`conformance-test` and `conformance/README.md` both say the `collections` root member was
"removed under ADR-0003". ADR-0003 is about RFC 2606 placeholder domains only, and the
word `collections` appears nowhere in `spec/` or `adr/`.

**The library warns on the member**, as the tool does, but the decision it cites is
recorded nowhere normative.

## 9. The OpenAPI version does not track the specification version

`spec/schemas/ard.openapi.yaml` declares `info.version: 0.5.0`. The specification is
v0.91.

## 10. Appendix C names one URN prefix, and the predecessor prefix is still in the field

Appendix C and ADR-0009 name `urn:air:`. Entries that carry the predecessor `urn:ai:` are
still published, and the reference client of Hugging Face reads them and reports a
deprecation warning.

**The library reads the predecessor prefix and warns.** `ParseURN` accepts it, marks the
result `Legacy`, and `URN.String` writes `urn:air:` back, so a round trip rewrites the
identifier. The validator reports `legacy_urn_prefix` as a warning and not an error. A
reader accepts what a writer must not emit; rejecting the entry loses a publisher that
appendix C can still reach.

## 11. §5.3 gives two rules for `application/ai-registry+json`

§5.3 says a registry **entry** of that media type carries the "operational base URL". The
OpenAPI says a **referral** of the same media type carries the search endpoint:
`RegistryReferral.url` is the "endpoint URL for the referred registry's search route".
One media type, two rules, and a client cannot tell which one it holds.

**The library accepts both.** `registry.Client` removes a route of §5.3 that the base URL
already carries, so a client built from a referral and a client built from a base URL
both reach `/search`, `/explore` and `/agents`. A registry genuinely mounted under a path
that ends in a route name is unreachable this way, which is the cost of the ambiguity.
