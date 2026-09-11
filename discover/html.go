package discover

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	ard "github.com/veggiemonk/go-ard"
)

// LinkRelation is the link relation a consumer MUST honour. See section 5.1.
const LinkRelation = "ard"

// PredecessorLinkRelation is the link relation of the predecessor specification.
const PredecessorLinkRelation = "ai-catalog"

// JSONLDScriptType is the script type that carries in-page entry markup.
const JSONLDScriptType = "application/ld+json"

// Link is a link element that points at an entry source, resolved against the base URL.
type Link struct {
	Rel         string
	URL         string
	Predecessor bool
}

// HTMLResult is what ScanHTML read out of a document.
type HTMLResult struct {
	Links    []Link
	Entries  []ard.Entry
	Warnings []Warning
}

// ScanHTML reads the entry sources of section 5.1 out of an HTML document.
func ScanHTML(r io.Reader, base *url.URL) (HTMLResult, error) {
	source, err := io.ReadAll(r)
	if err != nil {
		return HTMLResult{}, fmt.Errorf("discover: read html: %w", err)
	}
	return scanDocument(string(source), base), nil
}

func scanDocument(source string, base *url.URL) HTMLResult {
	lower := asciiLower(source)
	result := HTMLResult{}
	at := 0
	for at < len(source) {
		open := strings.IndexByte(source[at:], '<')
		if open < 0 {
			break
		}
		at += open
		if strings.HasPrefix(lower[at:], "<!--") {
			at = skipComment(source, at)
			continue
		}
		element, complete := readTag(source, lower, at)
		if !complete {
			break
		}
		at = element.end
		if element.closing {
			continue
		}
		switch element.name {
		case "link":
			result.appendLink(element, base)
		case "script", "style":
			content, after := readRawText(source, lower, element)
			if element.name == "script" && isJSONLD(element.attrs) {
				result.appendJSONLD(content, base)
			}
			at = after
		}
	}
	return result
}

func (h *HTMLResult) appendLink(element tag, base *url.URL) {
	href := strings.TrimSpace(element.attrs["href"])
	if href == "" {
		return
	}
	for _, token := range strings.Fields(asciiLower(element.attrs["rel"])) {
		switch token {
		case LinkRelation:
			h.Links = append(h.Links, Link{Rel: LinkRelation, URL: resolveReference(base, href)})
		case PredecessorLinkRelation:
			target := resolveReference(base, href)
			h.Links = append(h.Links, Link{Rel: PredecessorLinkRelation, URL: target, Predecessor: true})
			h.Warnings = append(h.Warnings, Warning{
				Code:    WarnPredecessorRelation,
				Message: "the document carries the predecessor link relation " + PredecessorLinkRelation + "; a consumer is only required to honour " + LinkRelation + " (section 5.1)",
				URL:     target,
			})
		}
	}
}

func (h *HTMLResult) appendJSONLD(block string, base *url.URL) {
	entries, err := decodeJSONLD([]byte(block))
	if err != nil {
		h.Warnings = append(h.Warnings, Warning{
			Code:    WarnUnreadableJSONLD,
			Message: fmt.Sprintf("a %s block holds no readable JSON: %v", JSONLDScriptType, err),
			URL:     baseString(base),
		})
		return
	}
	h.Entries = append(h.Entries, entries...)
}

func decodeJSONLD(block []byte) ([]ard.Entry, error) {
	trimmed := bytes.TrimSpace(block)
	if len(trimmed) == 0 {
		return nil, nil
	}
	switch trimmed[0] {
	case '[':
		var elements []json.RawMessage
		if err := json.Unmarshal(trimmed, &elements); err != nil {
			return nil, err
		}
		out := make([]ard.Entry, 0, len(elements))
		for _, element := range elements {
			entry, ok, err := decodeEntryObject(element)
			if err != nil {
				return nil, err
			}
			if ok {
				out = append(out, entry)
			}
		}
		return out, nil
	case '{':
		var members map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &members); err != nil {
			return nil, err
		}
		if _, ok := members[ard.TermEntries]; ok {
			var manifest ard.Manifest
			if err := json.Unmarshal(trimmed, &manifest); err != nil {
				return nil, err
			}
			return manifest.Entries, nil
		}
		entry, ok, err := decodeEntryObject(trimmed)
		if err != nil || !ok {
			return nil, err
		}
		return []ard.Entry{entry}, nil
	default:
		return nil, fmt.Errorf("a %s block holds neither an object nor an array", JSONLDScriptType)
	}
}

func decodeEntryObject(raw json.RawMessage) (ard.Entry, bool, error) {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw, &members); err != nil {
		return ard.Entry{}, false, err
	}
	if _, ok := members[ard.TermIdentifier]; !ok {
		return ard.Entry{}, false, nil
	}
	var entry ard.Entry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return ard.Entry{}, false, err
	}
	return entry, true, nil
}

type tag struct {
	name    string
	attrs   map[string]string
	end     int
	closing bool
}

func readTag(source, lower string, start int) (tag, bool) {
	at := start + 1
	element := tag{attrs: map[string]string{}, end: start + 1}
	if at < len(source) && source[at] == '/' {
		element.closing = true
		at++
	}
	nameStart := at
	if at >= len(source) || !isLetterByte(lower[at]) {
		return element, true
	}
	for at < len(source) && isNameByte(lower[at]) {
		at++
	}
	element.name = lower[nameStart:at]
	for {
		for at < len(source) && isSpaceByte(source[at]) {
			at++
		}
		if at >= len(source) {
			return element, false
		}
		if source[at] == '>' {
			element.end = at + 1
			return element, true
		}
		if source[at] == '/' {
			at++
			continue
		}
		attributeStart := at
		for at < len(source) && !isSpaceByte(source[at]) && source[at] != '=' && source[at] != '>' && source[at] != '/' {
			at++
		}
		if at == attributeStart {
			at++
			continue
		}
		name := lower[attributeStart:at]
		for at < len(source) && isSpaceByte(source[at]) {
			at++
		}
		if at >= len(source) {
			return element, false
		}
		if source[at] != '=' {
			element.attrs[name] = ""
			continue
		}
		at++
		for at < len(source) && isSpaceByte(source[at]) {
			at++
		}
		if at >= len(source) {
			return element, false
		}
		value, next, complete := readAttributeValue(source, at)
		if !complete {
			return element, false
		}
		element.attrs[name] = value
		at = next
	}
}

func readAttributeValue(source string, at int) (string, int, bool) {
	quote := source[at]
	if quote != '"' && quote != '\'' {
		start := at
		for at < len(source) && !isSpaceByte(source[at]) && source[at] != '>' {
			at++
		}
		return source[start:at], at, true
	}
	at++
	start := at
	for at < len(source) && source[at] != quote {
		at++
	}
	if at >= len(source) {
		return "", at, false
	}
	return source[start:at], at + 1, true
}

func readRawText(source, lower string, element tag) (string, int) {
	closing := "</" + element.name
	end := strings.Index(lower[element.end:], closing)
	if end < 0 {
		return source[element.end:], len(source)
	}
	content := source[element.end : element.end+end]
	after := strings.IndexByte(source[element.end+end:], '>')
	if after < 0 {
		return content, len(source)
	}
	return content, element.end + end + after + 1
}

func skipComment(source string, at int) int {
	end := strings.Index(source[at+4:], "-->")
	if end < 0 {
		return len(source)
	}
	return at + 4 + end + 3
}

func isJSONLD(attrs map[string]string) bool {
	mediaType, _, _ := strings.Cut(attrs["type"], ";")
	return strings.EqualFold(strings.TrimSpace(mediaType), JSONLDScriptType)
}

func isLetterByte(c byte) bool { return c >= 'a' && c <= 'z' }

func isNameByte(c byte) bool {
	return isLetterByte(c) || c >= '0' && c <= '9' || c == '-' || c == '_' || c == ':'
}

func isSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

func asciiLower(s string) string {
	out := []byte(s)
	for i, c := range out {
		if c >= 'A' && c <= 'Z' {
			out[i] = c + 'a' - 'A'
		}
	}
	return string(out)
}

func resolveReference(base *url.URL, reference string) string {
	if base == nil {
		return reference
	}
	parsed, err := url.Parse(reference)
	if err != nil {
		return reference
	}
	return base.ResolveReference(parsed).String()
}

func baseString(base *url.URL) string {
	if base == nil {
		return ""
	}
	return base.String()
}
