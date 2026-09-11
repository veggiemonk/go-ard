package jsonx

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestMergeKeepsMemberOrderAndSortsExtras(t *testing.T) {
	members := []Member{
		{Name: "identifier", Value: json.RawMessage(`"urn:air:acme.com:server:weather"`)},
		{Name: "displayName", Value: json.RawMessage(`"Weather Data Node"`)},
	}
	extras := map[string]json.RawMessage{
		"zz:last":  json.RawMessage(`1`),
		"aa:first": json.RawMessage(`[2]`),
	}
	got, err := Merge(members, extras, nil)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"identifier":"urn:air:acme.com:server:weather","displayName":"Weather Data Node","aa:first":[2],"zz:last":1}`
	if string(got) != want {
		t.Errorf("the object is %s", got)
	}
}

func TestMergeLeavesOutAMemberWithNoValue(t *testing.T) {
	got, err := Merge([]Member{{Name: "url"}, {Name: "type", Value: json.RawMessage(`"a/b"`)}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"type":"a/b"}` {
		t.Errorf("the object is %s", got)
	}
}

func TestMergeGivesAnEmptyObjectForNothing(t *testing.T) {
	got, err := Merge(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "{}" {
		t.Errorf("the object is %s", got)
	}
}

func TestMergeEscapesAMemberName(t *testing.T) {
	got, err := Merge(nil, map[string]json.RawMessage{`a"b`: json.RawMessage(`1`)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"a\"b":1}` {
		t.Errorf("the object is %s", got)
	}
}

func TestMergeReportsAnExtraThatRepeatsAReservedName(t *testing.T) {
	_, err := Merge(nil, map[string]json.RawMessage{"url": json.RawMessage(`"a"`)}, []string{"identifier", "url"})
	if !errors.Is(err, ErrReserved) {
		t.Fatalf("the error is %v", err)
	}
}

func TestMergeReportsAnExtraThatHoldsNoValidJSON(t *testing.T) {
	_, err := Merge(nil, map[string]json.RawMessage{"a": json.RawMessage(`{`)}, nil)
	if err == nil {
		t.Fatal("the merge wrote a broken object")
	}
}

func TestBuilderLeavesOutAnEmptyValue(t *testing.T) {
	b := &Builder{}
	b.String("url", "")
	b.String("type", "a/b")
	b.Strings("tags", nil)
	b.Strings("capabilities", []string{})
	b.Raw("data", nil)
	b.Raw("metadata", json.RawMessage(`{"a":1}`))
	got, err := b.Build(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"type":"a/b","capabilities":[],"metadata":{"a":1}}` {
		t.Errorf("the object is %s", got)
	}
}

func TestBuilderReportsAValueThatDoesNotEncode(t *testing.T) {
	b := &Builder{}
	b.Value("bad", make(chan int))
	if _, err := b.Build(nil, nil); err == nil {
		t.Fatal("the builder hid an encoding failure")
	}
}
