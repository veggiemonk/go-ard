package ard_test

import (
	"testing"

	ard "github.com/veggiemonk/go-ard"
)

func TestCanonicalMediaTypeFoldsTheSpellings(t *testing.T) {
	cases := map[string]string{
		"application/ai-skill":                   ard.MediaTypeAISkill,
		"application/ai-skill+md":                ard.MediaTypeAISkill,
		"Application/AI-Skill":                   ard.MediaTypeAISkill,
		"  application/ai-skill  ":               ard.MediaTypeAISkill,
		"application/mcp-server+json":            ard.MediaTypeMCPServerCard,
		"application/mcp-server-card+json":       ard.MediaTypeMCPServerCard,
		"application/ai-registry":                ard.MediaTypeAIRegistry,
		"application/ai-registry+json":           ard.MediaTypeAIRegistry,
		"application/a2a-agent-card+json":        ard.MediaTypeA2AAgentCard,
		"application/vnd.huggingface.space+json": "application/vnd.huggingface.space+json",
		"application/agent-skills+zip":           "application/agent-skills+zip",
		"":                                       "",
	}

	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			if got := ard.CanonicalMediaType(input); got != want {
				t.Errorf("CanonicalMediaType(%q) = %q, want %q", input, got, want)
			}
		})
	}
}

// A parameter carries meaning and its value is not the library's to change.
func TestCanonicalMediaTypeKeepsAParameter(t *testing.T) {
	const input = `Text/Markdown; profile="urn:air:agent-skills"`
	const want = `text/markdown; profile="urn:air:agent-skills"`

	if got := ard.CanonicalMediaType(input); got != want {
		t.Errorf("CanonicalMediaType(%q) = %q, want %q", input, got, want)
	}
}

// A zipped skill bundle is not a Markdown skill, and the table must not say it is.
func TestCanonicalMediaTypeFoldsNoArtifactKind(t *testing.T) {
	if got := ard.CanonicalMediaType("application/agent-skills+zip"); got == ard.MediaTypeAISkill {
		t.Errorf("CanonicalMediaType folded a zip bundle onto %q", ard.MediaTypeAISkill)
	}
}
