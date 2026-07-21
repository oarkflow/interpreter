package yaml

import (
	"strings"
	"testing"

	"github.com/oarkflow/interpreter/pkg/object"
)

func hashGet(t *testing.T, h *object.Hash, key string) object.Object {
	t.Helper()
	for _, pair := range h.Pairs {
		if s, ok := pair.Key.(*object.String); ok && s.Value == key {
			return pair.Value
		}
	}
	t.Fatalf("key %q not found in hash", key)
	return nil
}

func TestYAMLDecodeNestedMappingAndSequence(t *testing.T) {
	src := `
name: widget
count: 3
active: true
tags:
  - red
  - blue
meta:
  owner: alice
  ratio: 1.5
`
	result := builtinYAMLDecode(&object.String{Value: src})
	if errObj, ok := result.(*object.Error); ok {
		t.Fatalf("unexpected error: %s", errObj.Message)
	}
	hash, ok := result.(*object.Hash)
	if !ok {
		t.Fatalf("expected *object.Hash, got %T", result)
	}

	name, ok := hashGet(t, hash, "name").(*object.String)
	if !ok || name.Value != "widget" {
		t.Fatalf("expected name=widget, got %#v", hashGet(t, hash, "name"))
	}

	count, ok := hashGet(t, hash, "count").(*object.Integer)
	if !ok || count.Value != 3 {
		t.Fatalf("expected count=3, got %#v", hashGet(t, hash, "count"))
	}

	active, ok := hashGet(t, hash, "active").(*object.Boolean)
	if !ok || active.Value != true {
		t.Fatalf("expected active=true, got %#v", hashGet(t, hash, "active"))
	}

	tags, ok := hashGet(t, hash, "tags").(*object.Array)
	if !ok || len(tags.Elements) != 2 {
		t.Fatalf("expected tags array of 2, got %#v", hashGet(t, hash, "tags"))
	}
	if s, ok := tags.Elements[0].(*object.String); !ok || s.Value != "red" {
		t.Fatalf("expected tags[0]=red, got %#v", tags.Elements[0])
	}

	meta, ok := hashGet(t, hash, "meta").(*object.Hash)
	if !ok {
		t.Fatalf("expected meta to be a hash, got %#v", hashGet(t, hash, "meta"))
	}
	owner, ok := hashGet(t, meta, "owner").(*object.String)
	if !ok || owner.Value != "alice" {
		t.Fatalf("expected meta.owner=alice, got %#v", hashGet(t, meta, "owner"))
	}
	ratio, ok := hashGet(t, meta, "ratio").(*object.Float)
	if !ok || ratio.Value != 1.5 {
		t.Fatalf("expected meta.ratio=1.5, got %#v", hashGet(t, meta, "ratio"))
	}
}

func TestYAMLEncodeDecodeRoundTrip(t *testing.T) {
	tagsElements := []object.Object{
		&object.String{Value: "a"},
		&object.String{Value: "b"},
	}
	inner := &object.Hash{Pairs: map[object.HashKey]object.HashPair{}}
	innerKey := &object.String{Value: "enabled"}
	inner.Pairs[innerKey.HashKey()] = object.HashPair{Key: innerKey, Value: &object.Boolean{Value: true}}

	top := &object.Hash{Pairs: map[object.HashKey]object.HashPair{}}
	setField := func(name string, val object.Object) {
		k := &object.String{Value: name}
		top.Pairs[k.HashKey()] = object.HashPair{Key: k, Value: val}
	}
	setField("title", &object.String{Value: "hello"})
	setField("count", &object.Integer{Value: 42})
	setField("tags", &object.Array{Elements: tagsElements})
	setField("nested", inner)

	encoded := builtinYAMLEncode(top)
	strObj, ok := encoded.(*object.String)
	if !ok {
		t.Fatalf("expected *object.String from yaml_encode, got %T (%v)", encoded, encoded)
	}
	if !strings.Contains(strObj.Value, "title: hello") {
		t.Fatalf("expected encoded YAML to contain 'title: hello', got:\n%s", strObj.Value)
	}

	decoded := builtinYAMLDecode(&object.String{Value: strObj.Value})
	hash, ok := decoded.(*object.Hash)
	if !ok {
		t.Fatalf("expected decode round-trip to yield *object.Hash, got %T", decoded)
	}

	title, ok := hashGet(t, hash, "title").(*object.String)
	if !ok || title.Value != "hello" {
		t.Fatalf("round-trip title mismatch: %#v", hashGet(t, hash, "title"))
	}
	count, ok := hashGet(t, hash, "count").(*object.Integer)
	if !ok || count.Value != 42 {
		t.Fatalf("round-trip count mismatch: %#v", hashGet(t, hash, "count"))
	}
	tags, ok := hashGet(t, hash, "tags").(*object.Array)
	if !ok || len(tags.Elements) != 2 {
		t.Fatalf("round-trip tags mismatch: %#v", hashGet(t, hash, "tags"))
	}
	nested, ok := hashGet(t, hash, "nested").(*object.Hash)
	if !ok {
		t.Fatalf("round-trip nested mismatch: %#v", hashGet(t, hash, "nested"))
	}
	enabled, ok := hashGet(t, nested, "enabled").(*object.Boolean)
	if !ok || enabled.Value != true {
		t.Fatalf("round-trip nested.enabled mismatch: %#v", hashGet(t, nested, "enabled"))
	}
}

func TestYAMLEncodeWithIndentOption(t *testing.T) {
	arr := &object.Array{Elements: []object.Object{&object.Integer{Value: 1}, &object.Integer{Value: 2}}}
	opts := &object.Hash{Pairs: map[object.HashKey]object.HashPair{}}
	k := &object.String{Value: "indent"}
	opts.Pairs[k.HashKey()] = object.HashPair{Key: k, Value: &object.Integer{Value: 4}}

	result := builtinYAMLEncode(arr, opts)
	strObj, ok := result.(*object.String)
	if !ok {
		t.Fatalf("expected *object.String, got %T", result)
	}
	if !strings.Contains(strObj.Value, "- 1") {
		t.Fatalf("expected sequence encoding, got:\n%s", strObj.Value)
	}
}

func TestYAMLDecodeMalformedInputReturnsError(t *testing.T) {
	bad := "key: [unterminated\n  - broken"
	result := builtinYAMLDecode(&object.String{Value: bad})
	errObj, ok := result.(*object.Error)
	if !ok {
		t.Fatalf("expected *object.Error for malformed YAML, got %T (%v)", result, result)
	}
	if !strings.Contains(strings.ToLower(errObj.Message), "yaml_decode") {
		t.Fatalf("expected error message to mention yaml_decode, got: %s", errObj.Message)
	}
}

func TestYAMLDecodeWrongArgType(t *testing.T) {
	result := builtinYAMLDecode(&object.Integer{Value: 1})
	if _, ok := result.(*object.Error); !ok {
		t.Fatalf("expected *object.Error for non-string argument, got %T", result)
	}
}

func TestYAMLEncodeWrongArgCount(t *testing.T) {
	result := builtinYAMLEncode()
	if _, ok := result.(*object.Error); !ok {
		t.Fatalf("expected *object.Error for missing argument, got %T", result)
	}
}
