package memindex

import (
	"strings"
	"unicode"

	ard "github.com/veggiemonk/go-ard"
)

// DefaultCutoff is the relevance cutoff of section 5.3.3 that a new index applies: an
// entry that shares no token with the query text leaves the matched set.
const DefaultCutoff = 1

const (
	weightDisplayName  = 3
	weightQueries      = 3
	weightCapabilities = 2
	weightTags         = 2
	weightDescription  = 1
	weightMost         = 3
)

func entryTokens(e ard.Entry) map[string]int {
	tokens := map[string]int{}
	collect(tokens, weightDisplayName, e.DisplayName)
	collect(tokens, weightQueries, e.RepresentativeQueries...)
	collect(tokens, weightCapabilities, e.Capabilities...)
	collect(tokens, weightTags, e.Tags...)
	collect(tokens, weightDescription, e.Description)
	return tokens
}

func collect(tokens map[string]int, weight int, texts ...string) {
	for _, text := range texts {
		for _, token := range tokenize(text) {
			if tokens[token] < weight {
				tokens[token] = weight
			}
		}
	}
}

func queryTokens(text string) []string {
	seen := map[string]bool{}
	unique := make([]string, 0, 8)
	for _, token := range tokenize(text) {
		if seen[token] {
			continue
		}
		seen[token] = true
		unique = append(unique, token)
	}
	return unique
}

func relevance(tokens map[string]int, query []string) int {
	if len(query) == 0 {
		return 0
	}
	total := 0
	for _, token := range query {
		total += tokens[token]
	}
	return total * 100 / (len(query) * weightMost)
}

func tokenize(text string) []string {
	var tokens []string
	var current []rune
	previous := rune(0)
	flush := func() {
		if len(current) > 0 {
			tokens = append(tokens, strings.ToLower(string(current)))
			current = current[:0]
		}
	}
	for _, r := range text {
		switch {
		case unicode.IsUpper(r) && unicode.IsLower(previous):
			flush()
			current = append(current, r)
		case unicode.IsLetter(r), unicode.IsDigit(r):
			current = append(current, r)
		default:
			flush()
		}
		previous = r
	}
	flush()
	return tokens
}
