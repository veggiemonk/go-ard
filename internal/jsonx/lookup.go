package jsonx

import (
	"encoding/json"
	"fmt"
)

// Lookup walks a literal JSON dot-path through a document and gives every value the path
// reaches. Section 5.3.1 of the specification filters on such a path, as in
// trustManifest.attestations.type.
//
// An array is a set of values rather than one value: wherever the walk meets an array it
// visits the elements in its place. A path therefore continues through an array of
// objects, one path may reach more than one value, and no value it gives back is itself
// an array.
//
// A path that reaches nothing gives no value and no error, and so does an empty
// document. An empty path gives the document itself. Lookup reports an error only for a
// document that holds no valid JSON.
func Lookup(raw json.RawMessage, path []string) ([]json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("jsonx: lookup of %v: the document holds no valid JSON", path)
	}
	var found []json.RawMessage
	if err := walk(raw, path, &found); err != nil {
		return nil, err
	}
	return found, nil
}

func walk(raw json.RawMessage, path []string, found *[]json.RawMessage) error {
	switch firstByte(raw) {
	case '[':
		var elements []json.RawMessage
		if err := json.Unmarshal(raw, &elements); err != nil {
			return fmt.Errorf("jsonx: lookup of %v: %w", path, err)
		}
		for _, element := range elements {
			if err := walk(element, path, found); err != nil {
				return err
			}
		}
		return nil
	case '{':
		if len(path) == 0 {
			break
		}
		var members map[string]json.RawMessage
		if err := json.Unmarshal(raw, &members); err != nil {
			return fmt.Errorf("jsonx: lookup of %v: %w", path, err)
		}
		value, ok := members[path[0]]
		if !ok {
			return nil
		}
		return walk(value, path[1:], found)
	default:
		if len(path) > 0 {
			return nil
		}
	}
	*found = append(*found, raw)
	return nil
}

func firstByte(raw json.RawMessage) byte {
	for _, c := range raw {
		switch c {
		case ' ', '\t', '\r', '\n':
			continue
		}
		return c
	}
	return 0
}
