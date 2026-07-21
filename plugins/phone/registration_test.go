package phone

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

func TestPhoneBuiltinsRegisterOnImport(t *testing.T) {
	for _, name := range []string{"phone_parse", "phone_valid", "phone_country", "phone_parse_bulk", "phone_networks"} {
		if !eval.HasBuiltin(name) {
			t.Fatalf("expected phone builtin %q to be registered", name)
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

func hashFieldValue(t *testing.T, h *object.Hash, key string) object.Object {
	t.Helper()
	k := (&object.String{Value: key}).HashKey()
	pair, ok := h.Pairs[k]
	if !ok {
		t.Fatalf("hash missing field %q", key)
	}
	return pair.Value
}

func TestPhoneParseBulkFromHashRecords(t *testing.T) {
	fn := lookupBuiltin(t, "phone_parse_bulk")

	records := &object.Array{Elements: []object.Object{
		newRecord(map[string]object.Object{"id": &object.Integer{Value: 1}, "phone": &object.String{Value: "(650) 253-0000"}}),
		newRecord(map[string]object.Object{"id": &object.Integer{Value: 2}, "phone": &object.String{Value: "not a number"}}),
		newRecord(map[string]object.Object{"id": &object.Integer{Value: 3}}), // missing phone field
	}}

	result := fn.Fn(records, &object.String{Value: "phone"}, newRecord(map[string]object.Object{
		"default_region": &object.String{Value: "US"},
	}))
	report, ok := result.(*object.Hash)
	if !ok {
		t.Fatalf("phone_parse_bulk returned unexpected shape: %#v", result)
	}

	total := hashFieldValue(t, report, "total").(*object.Integer)
	if total.Value != 3 {
		t.Fatalf("expected total=3, got %d", total.Value)
	}
	validCount := hashFieldValue(t, report, "valid_count").(*object.Integer)
	if validCount.Value != 1 {
		t.Fatalf("expected valid_count=1, got %d", validCount.Value)
	}
	invalidCount := hashFieldValue(t, report, "invalid_count").(*object.Integer)
	if invalidCount.Value != 2 {
		t.Fatalf("expected invalid_count=2, got %d", invalidCount.Value)
	}

	results := hashFieldValue(t, report, "results").(*object.Array)
	if len(results.Elements) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results.Elements))
	}

	first := results.Elements[0].(*object.Hash)
	if v := hashFieldValue(t, first, "valid").(*object.Boolean); !v.Value {
		t.Fatalf("expected first record to be valid")
	}
	e164 := hashFieldValue(t, first, "e164").(*object.String)
	if e164.Value != "+16502530000" {
		t.Fatalf("expected e164=+16502530000, got %q", e164.Value)
	}
	firstID := hashFieldValue(t, first, "id").(*object.Integer)
	if firstID.Value != 1 {
		t.Fatalf("expected original record fields flattened into the result, id=1, got %d", firstID.Value)
	}

	third := results.Elements[2].(*object.Hash)
	if v := hashFieldValue(t, third, "valid").(*object.Boolean); v.Value {
		t.Fatalf("expected third record (missing field) to be invalid")
	}
	if errVal := hashFieldValue(t, third, "error"); errVal == object.NULL {
		t.Fatalf("expected non-nil error for missing field")
	}
}

func TestPhoneParseBulkFromPlainStrings(t *testing.T) {
	fn := lookupBuiltin(t, "phone_parse_bulk")

	records := &object.Array{Elements: []object.Object{
		&object.String{Value: "(650) 253-0000"},
		&object.String{Value: "garbage"},
	}}

	result := fn.Fn(records, object.NULL)
	report, ok := result.(*object.Hash)
	if !ok {
		t.Fatalf("phone_parse_bulk returned unexpected shape: %#v", result)
	}
	total := hashFieldValue(t, report, "total").(*object.Integer)
	if total.Value != 2 {
		t.Fatalf("expected total=2, got %d", total.Value)
	}
}

func TestPhoneParseBulkFromPlainStringsNoFieldArg(t *testing.T) {
	fn := lookupBuiltin(t, "phone_parse_bulk")

	records := &object.Array{Elements: []object.Object{
		&object.String{Value: "+16502530000"},
		&object.String{Value: "garbage"},
	}}

	// A bare array/slice of phone numbers: no field argument at all.
	result := fn.Fn(records)
	report, ok := result.(*object.Hash)
	if !ok {
		t.Fatalf("phone_parse_bulk(records) returned unexpected shape: %#v", result)
	}
	total := hashFieldValue(t, report, "total").(*object.Integer)
	if total.Value != 2 {
		t.Fatalf("expected total=2, got %d", total.Value)
	}
	validCount := hashFieldValue(t, report, "valid_count").(*object.Integer)
	if validCount.Value != 1 {
		t.Fatalf("expected valid_count=1 (E.164 number parses without a region), got %d", validCount.Value)
	}
}

func TestPhoneParseBulkRecordsWithOptsNoField(t *testing.T) {
	fn := lookupBuiltin(t, "phone_parse_bulk")

	records := &object.Array{Elements: []object.Object{
		&object.String{Value: "(650) 253-0000"},
	}}

	// records + opts, no field argument in between.
	result := fn.Fn(records, newRecord(map[string]object.Object{
		"default_region": &object.String{Value: "US"},
	}))
	report, ok := result.(*object.Hash)
	if !ok {
		t.Fatalf("phone_parse_bulk(records, opts) returned unexpected shape: %#v", result)
	}
	validCount := hashFieldValue(t, report, "valid_count").(*object.Integer)
	if validCount.Value != 1 {
		t.Fatalf("expected valid_count=1, got %d", validCount.Value)
	}
}

func TestPhoneParseValidNumber(t *testing.T) {
	parseFn := lookupBuiltin(t, "phone_parse")
	result := parseFn.Fn(&object.String{Value: "(650) 253-0000"}, &object.String{Value: "US"})
	arr, ok := result.(*object.Array)
	if !ok || len(arr.Elements) != 2 {
		t.Fatalf("phone_parse returned unexpected shape: %#v", result)
	}
	if arr.Elements[1] != object.NULL {
		t.Fatalf("phone_parse returned err for a valid number: %#v", arr.Elements[1])
	}
	h, ok := arr.Elements[0].(*object.Hash)
	if !ok {
		t.Fatalf("phone_parse returned non-hash: %#v", arr.Elements[0])
	}
	e164Key := (&object.String{Value: "e164"}).HashKey()
	pair, ok := h.Pairs[e164Key]
	if !ok {
		t.Fatalf("result missing `e164` field")
	}
	e164, ok := pair.Value.(*object.String)
	if !ok || e164.Value != "+16502530000" {
		t.Fatalf("expected e164=+16502530000, got %#v", pair.Value)
	}
	if _, ok := h.Pairs[(&object.String{Value: "carrier"}).HashKey()]; !ok {
		t.Fatalf("result missing `carrier` field")
	}
}

func TestPhoneNetworks(t *testing.T) {
	fn := lookupBuiltin(t, "phone_networks")

	result := fn.Fn(&object.String{Value: "US"})
	arr, ok := result.(*object.Array)
	if !ok || len(arr.Elements) != 2 {
		t.Fatalf("phone_networks returned unexpected shape: %#v", result)
	}
	if arr.Elements[1] != object.NULL {
		t.Fatalf("phone_networks returned err for US: %#v", arr.Elements[1])
	}
	networks, ok := arr.Elements[0].(*object.Array)
	if !ok || len(networks.Elements) == 0 {
		t.Fatalf("expected at least one US network entry, got %#v", arr.Elements[0])
	}
	first, ok := networks.Elements[0].(*object.Hash)
	if !ok {
		t.Fatalf("expected network entry HASH, got %#v", networks.Elements[0])
	}
	for _, field := range []string{"mcc", "mnc", "plmn", "operator", "status"} {
		if _, ok := first.Pairs[(&object.String{Value: field}).HashKey()]; !ok {
			t.Fatalf("network entry missing %q field", field)
		}
	}
}

func TestPhoneNetworksStatusFilter(t *testing.T) {
	fn := lookupBuiltin(t, "phone_networks")

	all := fn.Fn(&object.String{Value: "US"}).(*object.Array).Elements[0].(*object.Array)
	filtered := fn.Fn(&object.String{Value: "US"}, newRecord(map[string]object.Object{
		"status": &object.String{Value: "Operational"},
	}))
	filteredArr, ok := filtered.(*object.Array)
	if !ok || len(filteredArr.Elements) != 2 {
		t.Fatalf("phone_networks with opts returned unexpected shape: %#v", filtered)
	}
	networks, ok := filteredArr.Elements[0].(*object.Array)
	if !ok {
		t.Fatalf("expected ARRAY result, got %#v", filteredArr.Elements[0])
	}
	if len(networks.Elements) == 0 || len(networks.Elements) >= len(all.Elements) {
		t.Fatalf("expected status filter to narrow results: all=%d filtered=%d", len(all.Elements), len(networks.Elements))
	}
	for _, el := range networks.Elements {
		h := el.(*object.Hash)
		status := h.Pairs[(&object.String{Value: "status"}).HashKey()].Value.(*object.String).Value
		if status != "Operational" {
			t.Fatalf("expected only Operational entries, got status=%q", status)
		}
	}
}

func TestPhoneValid(t *testing.T) {
	validFn := lookupBuiltin(t, "phone_valid")

	ok := validFn.Fn(&object.String{Value: "(650) 253-0000"}, &object.String{Value: "US"})
	if b, isBool := ok.(*object.Boolean); !isBool || !b.Value {
		t.Fatalf("expected valid US number to report true, got %#v", ok)
	}

	bad := validFn.Fn(&object.String{Value: "not a phone number"}, &object.String{Value: "US"})
	if b, isBool := bad.(*object.Boolean); !isBool || b.Value {
		t.Fatalf("expected garbage input to report false, got %#v", bad)
	}
}

func TestPhoneCountry(t *testing.T) {
	countryFn := lookupBuiltin(t, "phone_country")
	result := countryFn.Fn(&object.String{Value: "AU"})
	arr, ok := result.(*object.Array)
	if !ok || len(arr.Elements) != 2 {
		t.Fatalf("phone_country returned unexpected shape: %#v", result)
	}
	if arr.Elements[1] != object.NULL {
		t.Fatalf("phone_country returned err for AU: %#v", arr.Elements[1])
	}
	h, ok := arr.Elements[0].(*object.Hash)
	if !ok {
		t.Fatalf("phone_country returned non-hash: %#v", arr.Elements[0])
	}
	currencyKey := (&object.String{Value: "currency"}).HashKey()
	pair, ok := h.Pairs[currencyKey]
	if !ok {
		t.Fatalf("result missing `currency` field")
	}
	currency, ok := pair.Value.(*object.String)
	if !ok || currency.Value != "AUD" {
		t.Fatalf("expected currency=AUD, got %#v", pair.Value)
	}
}
