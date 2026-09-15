package ard

import "strings"

// Media types the specification names. See section 3.3 and ADR-0008.
const (
	MediaTypeA2AAgentCard  = "application/a2a-agent-card+json"
	MediaTypeMCPServerCard = "application/mcp-server-card+json"
	MediaTypeAISkill       = "application/ai-skill+md"
	MediaTypeAICatalog     = "application/ai-catalog+json"
	MediaTypeAIRegistry    = "application/ai-registry+json"
	MediaTypeAIRegistryOld = "application/ai-registry"
)

// mediaTypeAliases folds another spelling of a media type onto the one above.
//
// The specification does not settle the spellings. Four are in the field for a skill
// alone: the prose writes "application/ai-skill+md", the reference tool knows neither
// that nor the one the largest outside implementation publishes, which is
// "application/ai-skill". Until the specification names one, a filter written with one
// spelling must find an entry written with the other. See finding 6 of
// docs/spec-findings.md.
//
// This table folds a spelling; it does not fold one artifact kind onto another. A
// zipped skill bundle is not a Markdown skill, so it is absent.
var mediaTypeAliases = map[string]string{
	"application/ai-skill":        MediaTypeAISkill,
	"application/mcp-server+json": MediaTypeMCPServerCard,
	MediaTypeAIRegistryOld:        MediaTypeAIRegistry,
}

// CanonicalMediaType folds a media type onto the spelling this library uses, so that two
// spellings of one type compare equal.
//
// It lowers the case of the type and the subtype, and maps a known alias. It keeps any
// parameter as it stands, because a parameter such as profile carries meaning and its
// value is not the library's to change. It gives an unknown media type back unchanged
// but for the case, so a comparison stays a comparison.
func CanonicalMediaType(mediaType string) string {
	essence, parameters, hasParameters := strings.Cut(strings.TrimSpace(mediaType), ";")
	essence = strings.ToLower(strings.TrimSpace(essence))
	if canonical, aliased := mediaTypeAliases[essence]; aliased {
		essence = canonical
	}
	if !hasParameters {
		return essence
	}
	return essence + ";" + parameters
}
