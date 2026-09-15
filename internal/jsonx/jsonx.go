// Package jsonx encodes and inspects JSON objects that are only partly known, so that a
// decode followed by an encode keeps the members the decoder has no field for.
package jsonx

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
)

// ErrReserved reports that an extra member repeats the name of a member the caller holds
// in a field of its own. Writing it would lose one of the two values.
var ErrReserved = errors.New("jsonx: extra member repeats a reserved name")

// Member is one named member of a JSON object, with its value already encoded.
type Member struct {
	Name  string
	Value json.RawMessage
}

// Merge encodes one JSON object from named members and extra members. The named members
// keep the order they are given and the extra members follow in sorted name order, so
// one value always gives the same bytes. A named member with no value is left out.
//
// Merge reports ErrReserved when an extra member repeats one of the reserved names.
func Merge(members []Member, extras map[string]json.RawMessage, reserved []string) ([]byte, error) {
	for _, name := range reserved {
		if _, ok := extras[name]; ok {
			return nil, fmt.Errorf("%w: %q", ErrReserved, name)
		}
	}
	object := bytes.NewBufferString("{")
	for _, m := range members {
		if len(m.Value) == 0 {
			continue
		}
		if err := writeMember(object, m.Name, m.Value); err != nil {
			return nil, err
		}
	}
	for _, name := range slices.Sorted(maps.Keys(extras)) {
		if err := writeMember(object, name, extras[name]); err != nil {
			return nil, err
		}
	}
	object.WriteByte('}')
	return object.Bytes(), nil
}

func writeMember(object *bytes.Buffer, name string, value json.RawMessage) error {
	if !json.Valid(value) {
		return fmt.Errorf("jsonx: member %q holds no valid JSON", name)
	}
	key, err := json.Marshal(name)
	if err != nil {
		return fmt.Errorf("jsonx: member name %q: %w", name, err)
	}
	if object.Len() > len("{") {
		object.WriteByte(',')
	}
	object.Write(key)
	object.WriteByte(':')
	object.Write(value)
	return nil
}

// Builder collects the named members of a JSON object in the order they are to be
// written. It keeps the first encoding error and gives it back from Build.
type Builder struct {
	members []Member
	err     error
}

// Raw adds a member whose value is already encoded, and leaves out an empty value.
func (b *Builder) Raw(name string, value json.RawMessage) {
	if len(value) == 0 {
		return
	}
	b.members = append(b.members, Member{Name: name, Value: value})
}

// Value adds a member and encodes it.
func (b *Builder) Value(name string, value any) {
	raw, err := json.Marshal(value)
	if err != nil {
		b.err = errors.Join(b.err, fmt.Errorf("jsonx: member %q: %w", name, err))
		return
	}
	b.Raw(name, raw)
}

// String adds a string member and leaves out the empty string.
func (b *Builder) String(name, value string) {
	if value == "" {
		return
	}
	b.Value(name, value)
}

// Strings adds a string array member and leaves out a nil slice. It keeps an empty
// slice, so that a document that carries an empty array gets it back.
func (b *Builder) Strings(name string, values []string) {
	if values == nil {
		return
	}
	b.Value(name, values)
}

// Build merges the named members with the extra members. See Merge.
func (b *Builder) Build(extras map[string]json.RawMessage, reserved []string) ([]byte, error) {
	if b.err != nil {
		return nil, b.err
	}
	return Merge(b.members, extras, reserved)
}
