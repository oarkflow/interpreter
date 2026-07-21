package ip

import (
	"testing"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
)

func lookupBuiltin(t *testing.T, name string) *object.Builtin {
	t.Helper()
	b, ok := eval.BuiltinByName(name)
	if !ok {
		t.Fatalf("expected builtin %q to be registered", name)
	}
	return b
}

func TestIPBuiltinsRegisterOnImport(t *testing.T) {
	for _, name := range []string{"ip_is_private", "ip_client_from_header", "ip_geo_init", "ip_country", "ip_lookup", "ip_lookup_bulk"} {
		if !eval.HasBuiltin(name) {
			t.Fatalf("expected ip builtin %q to be registered", name)
		}
	}
}

func newRecord(pairs map[string]object.Object) *object.Hash {
	h := &object.Hash{Pairs: map[object.HashKey]object.HashPair{}}
	for k, v := range pairs {
		key := &object.String{Value: k}
		h.Pairs[key.HashKey()] = object.HashPair{Key: key, Value: v}
	}
	return h
}

func hashField(t *testing.T, h *object.Hash, key string) object.Object {
	t.Helper()
	k := (&object.String{Value: key}).HashKey()
	pair, ok := h.Pairs[k]
	if !ok {
		t.Fatalf("hash missing field %q", key)
	}
	return pair.Value
}

func TestIPIsPrivate(t *testing.T) {
	fn := lookupBuiltin(t, "ip_is_private")

	priv := fn.Fn(&object.String{Value: "10.0.0.1"})
	if b, ok := priv.(*object.Boolean); !ok || !b.Value {
		t.Fatalf("expected 10.0.0.1 to be private, got %#v", priv)
	}

	loopback := fn.Fn(&object.String{Value: "127.0.0.1"})
	if b, ok := loopback.(*object.Boolean); !ok || !b.Value {
		t.Fatalf("expected 127.0.0.1 to be private, got %#v", loopback)
	}

	pub := fn.Fn(&object.String{Value: "8.8.8.8"})
	if b, ok := pub.(*object.Boolean); !ok || b.Value {
		t.Fatalf("expected 8.8.8.8 to be public, got %#v", pub)
	}

	bad := fn.Fn(&object.String{Value: "not-an-ip"})
	if _, ok := bad.(*object.Error); !ok {
		t.Fatalf("expected error for invalid IP, got %#v", bad)
	}
}

func TestIPClientFromHeader(t *testing.T) {
	fn := lookupBuiltin(t, "ip_client_from_header")

	result := fn.Fn(&object.String{Value: "10.0.0.1"}, &object.String{Value: "203.0.113.5, 10.0.0.1"})
	s, ok := result.(*object.String)
	if !ok || s.Value != "203.0.113.5" {
		t.Fatalf("expected first public IP 203.0.113.5, got %#v", result)
	}

	opts := &object.Hash{Pairs: map[object.HashKey]object.HashPair{}}
	key := &object.String{Value: "trust_proxy"}
	opts.Pairs[key.HashKey()] = object.HashPair{Key: key, Value: object.FALSE}
	untrusted := fn.Fn(&object.String{Value: "10.0.0.1"}, &object.String{Value: "203.0.113.5"}, opts)
	if s, ok := untrusted.(*object.String); !ok || s.Value != "10.0.0.1" {
		t.Fatalf("expected remote_ip returned untouched when trust_proxy=false, got %#v", untrusted)
	}
}

func TestIPCountryAndLookupWithoutInit(t *testing.T) {
	countryFn := lookupBuiltin(t, "ip_country")
	result := countryFn.Fn(&object.String{Value: "8.8.8.8"})
	if s, ok := result.(*object.String); !ok || s.Value != "" {
		t.Fatalf("expected empty country before ip_geo_init(), got %#v", result)
	}

	lookupFn := lookupBuiltin(t, "ip_lookup")
	lookupResult := lookupFn.Fn(&object.String{Value: "8.8.8.8"})
	h, ok := lookupResult.(*object.Hash)
	if !ok {
		t.Fatalf("expected HASH result, got %#v", lookupResult)
	}
	foundKey := (&object.String{Value: "found"}).HashKey()
	pair, ok := h.Pairs[foundKey]
	if !ok {
		t.Fatalf("result missing `found` field")
	}
	if b, ok := pair.Value.(*object.Boolean); !ok || b.Value {
		t.Fatalf("expected found=false before ip_geo_init(), got %#v", pair.Value)
	}
}

func TestIPLookupBulkFromHashRecords(t *testing.T) {
	fn := lookupBuiltin(t, "ip_lookup_bulk")

	records := &object.Array{Elements: []object.Object{
		newRecord(map[string]object.Object{"id": &object.Integer{Value: 1}, "ip": &object.String{Value: "8.8.8.8"}}),
		newRecord(map[string]object.Object{"id": &object.Integer{Value: 2}, "ip": &object.String{Value: "not-an-ip"}}),
		newRecord(map[string]object.Object{"id": &object.Integer{Value: 3}}), // missing ip field
	}}

	result := fn.Fn(records, &object.String{Value: "ip"})
	report, ok := result.(*object.Hash)
	if !ok {
		t.Fatalf("ip_lookup_bulk returned unexpected shape: %#v", result)
	}

	total := hashField(t, report, "total").(*object.Integer)
	if total.Value != 3 {
		t.Fatalf("expected total=3, got %d", total.Value)
	}
	validCount := hashField(t, report, "valid_count").(*object.Integer)
	if validCount.Value != 1 {
		t.Fatalf("expected valid_count=1, got %d", validCount.Value)
	}

	results := hashField(t, report, "results").(*object.Array)
	first := results.Elements[0].(*object.Hash)
	if v := hashField(t, first, "valid").(*object.Boolean); !v.Value {
		t.Fatalf("expected first record to be a valid IP")
	}
	if v := hashField(t, first, "is_private").(*object.Boolean); v.Value {
		t.Fatalf("expected 8.8.8.8 to not be private")
	}
	firstID := hashField(t, first, "id").(*object.Integer)
	if firstID.Value != 1 {
		t.Fatalf("expected original record fields flattened into the result, id=1, got %d", firstID.Value)
	}

	third := results.Elements[2].(*object.Hash)
	if v := hashField(t, third, "valid").(*object.Boolean); v.Value {
		t.Fatalf("expected third record (missing field) to be invalid")
	}
	if errVal := hashField(t, third, "error"); errVal == object.NULL {
		t.Fatalf("expected non-nil error for missing field")
	}
}

func TestIPLookupBulkFromTableValue(t *testing.T) {
	fn := lookupBuiltin(t, "ip_lookup_bulk")

	table := &object.TableValue{
		Columns: []string{"ip"},
		Rows: []map[string]object.Object{
			{"ip": &object.String{Value: "1.1.1.1"}},
			{"ip": &object.String{Value: "10.0.0.5"}},
		},
	}

	result := fn.Fn(table, &object.String{Value: "ip"})
	report, ok := result.(*object.Hash)
	if !ok {
		t.Fatalf("ip_lookup_bulk returned unexpected shape: %#v", result)
	}
	total := hashField(t, report, "total").(*object.Integer)
	if total.Value != 2 {
		t.Fatalf("expected total=2 from TABLE_VALUE input, got %d", total.Value)
	}
	validCount := hashField(t, report, "valid_count").(*object.Integer)
	if validCount.Value != 2 {
		t.Fatalf("expected valid_count=2, got %d", validCount.Value)
	}
}

func TestIPLookupBulkFromPlainStringsNoFieldArg(t *testing.T) {
	fn := lookupBuiltin(t, "ip_lookup_bulk")

	// A bare array/slice of IP addresses: no field argument at all.
	records := &object.Array{Elements: []object.Object{
		&object.String{Value: "8.8.8.8"},
		&object.String{Value: "not-an-ip"},
	}}

	result := fn.Fn(records)
	report, ok := result.(*object.Hash)
	if !ok {
		t.Fatalf("ip_lookup_bulk(records) returned unexpected shape: %#v", result)
	}
	total := hashField(t, report, "total").(*object.Integer)
	if total.Value != 2 {
		t.Fatalf("expected total=2, got %d", total.Value)
	}
	validCount := hashField(t, report, "valid_count").(*object.Integer)
	if validCount.Value != 1 {
		t.Fatalf("expected valid_count=1, got %d", validCount.Value)
	}
}
