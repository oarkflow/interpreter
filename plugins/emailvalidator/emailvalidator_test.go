package emailvalidator

import (
	"testing"

	"github.com/oarkflow/interpreter/pkg/object"
)

func TestEmailValidateSyntax(t *testing.T) {
	res := builtinEmailValidateSyntax(&object.String{Value: "User@Example.COM"})
	h, ok := res.(*object.Hash)
	if !ok {
		t.Fatalf("expected HASH, got %T (%v)", res, res)
	}
	v, ok := hashGet(h, "normalized")
	if !ok {
		t.Fatalf("missing normalized field")
	}
	// ev lowercases the domain but preserves local-part case (RFC 5321
	// treats the local part as potentially case-sensitive).
	if s := v.(*object.String).Value; s != "User@example.com" {
		t.Fatalf("normalized = %q, want User@example.com", s)
	}

	bad := builtinEmailValidateSyntax(&object.String{Value: "not-an-email"})
	h, ok = bad.(*object.Hash)
	if !ok {
		t.Fatalf("expected HASH, got %T", bad)
	}
	if errV, _ := hashGet(h, "error"); errV.(*object.String).Value == "" {
		t.Fatalf("expected non-empty error for invalid syntax")
	}
}

func TestEmailIsDisposable(t *testing.T) {
	// mailinator.com is a well-known disposable provider in ev's bundled list.
	res := builtinEmailIsDisposable(&object.String{Value: "someone@mailinator.com"})
	b, ok := res.(*object.Boolean)
	if !ok {
		t.Fatalf("expected BOOLEAN, got %T", res)
	}
	if !b.Value {
		t.Fatalf("expected mailinator.com to be flagged disposable")
	}

	res = builtinEmailIsDisposable(&object.String{Value: "someone@example.com"})
	if res.(*object.Boolean).Value {
		t.Fatalf("expected example.com to not be flagged disposable")
	}
}

func TestEmailIsRoleAccount(t *testing.T) {
	res := builtinEmailIsRoleAccount(&object.String{Value: "admin+tag@example.com"})
	if !res.(*object.Boolean).Value {
		t.Fatalf("expected admin@ to be flagged as a role account")
	}
	res = builtinEmailIsRoleAccount(&object.String{Value: "jane.doe@example.com"})
	if res.(*object.Boolean).Value {
		t.Fatalf("expected jane.doe@ to not be flagged as a role account")
	}
}

func TestEmailIsFreeProvider(t *testing.T) {
	res := builtinEmailIsFreeProvider(&object.String{Value: "someone@gmail.com"})
	if !res.(*object.Boolean).Value {
		t.Fatalf("expected gmail.com to be flagged as a free provider")
	}
	res = builtinEmailIsFreeProvider(&object.String{Value: "someone@example.com"})
	if res.(*object.Boolean).Value {
		t.Fatalf("expected example.com to not be flagged as a free provider")
	}
}

func TestEmailValidateNoNetwork(t *testing.T) {
	res := builtinEmailValidate(&object.String{Value: "user@example.com"}, &object.Hash{Pairs: map[object.HashKey]object.HashPair{
		(&object.String{Value: "check_dns"}).HashKey(): {Key: &object.String{Value: "check_dns"}, Value: object.FALSE},
	}})
	arr, ok := res.(*object.Array)
	if !ok || len(arr.Elements) != 2 {
		t.Fatalf("expected 2-element tuple, got %T", res)
	}
	if arr.Elements[1] != object.NULL {
		t.Fatalf("expected nil error, got %v", arr.Elements[1].Inspect())
	}
	h, ok := arr.Elements[0].(*object.Hash)
	if !ok {
		t.Fatalf("expected HASH result, got %T", arr.Elements[0])
	}
	verdict, _ := hashGet(h, "verdict")
	if verdict.(*object.String).Value == "" {
		t.Fatalf("expected non-empty verdict")
	}
}

func TestEmailValidateInvalidSyntax(t *testing.T) {
	res := builtinEmailValidate(&object.String{Value: "not-an-email"}, &object.Hash{Pairs: map[object.HashKey]object.HashPair{
		(&object.String{Value: "check_dns"}).HashKey(): {Key: &object.String{Value: "check_dns"}, Value: object.FALSE},
	}})
	arr := res.(*object.Array)
	h := arr.Elements[0].(*object.Hash)
	verdict, _ := hashGet(h, "verdict")
	if verdict.(*object.String).Value != "undeliverable" {
		t.Fatalf("expected undeliverable verdict for invalid syntax, got %q", verdict.(*object.String).Value)
	}
}

func hashOf(pairs map[string]object.Object) *object.Hash {
	return hashFromPairs(pairs)
}

func TestEmailValidateBulkPlainStrings(t *testing.T) {
	records := &object.Array{Elements: []object.Object{
		&object.String{Value: "user@example.com"},
		&object.String{Value: "not-an-email"},
	}}
	res := builtinEmailValidateBulk(records)
	report, ok := res.(*object.Hash)
	if !ok {
		t.Fatalf("expected HASH report, got %T (%v)", res, res)
	}
	total, _ := hashGet(report, "total")
	if total.(*object.Integer).Value != 2 {
		t.Fatalf("total = %v, want 2", total.Inspect())
	}
	validCount, _ := hashGet(report, "valid_count")
	if validCount.(*object.Integer).Value != 1 {
		t.Fatalf("valid_count = %v, want 1", validCount.Inspect())
	}
	invalidCount, _ := hashGet(report, "invalid_count")
	if invalidCount.(*object.Integer).Value != 1 {
		t.Fatalf("invalid_count = %v, want 1", invalidCount.Inspect())
	}
	resultsV, _ := hashGet(report, "results")
	results := resultsV.(*object.Array).Elements
	if len(results) != 2 {
		t.Fatalf("results len = %d, want 2", len(results))
	}
	row0 := results[0].(*object.Hash)
	if v, _ := hashGet(row0, "valid"); v != object.TRUE {
		t.Fatalf("row0.valid = %v, want true", v.Inspect())
	}
	if v, _ := hashGet(row0, "verdict"); v.(*object.String).Value == "" {
		t.Fatalf("row0.verdict is empty, expected full validation fields since check_dns defaults false but syntax/disposable checks always run")
	}
	row1 := results[1].(*object.Hash)
	if v, _ := hashGet(row1, "valid"); v != object.FALSE {
		t.Fatalf("row1.valid = %v, want false", v.Inspect())
	}
	if v, _ := hashGet(row1, "error"); v.(*object.String).Value == "" {
		t.Fatalf("row1.error should describe the syntax failure")
	}
}

func TestEmailValidateBulkHashRecordsWithField(t *testing.T) {
	records := &object.Array{Elements: []object.Object{
		hashOf(map[string]object.Object{"id": &object.Integer{Value: 1}, "email": &object.String{Value: "a@example.com"}}),
		hashOf(map[string]object.Object{"id": &object.Integer{Value: 2}, "email": &object.String{Value: "bad"}}),
	}}
	res := builtinEmailValidateBulk(records, &object.String{Value: "email"})
	report := res.(*object.Hash)
	resultsV, _ := hashGet(report, "results")
	results := resultsV.(*object.Array).Elements
	row0 := results[0].(*object.Hash)
	if id, _ := hashGet(row0, "id"); id.(*object.Integer).Value != 1 {
		t.Fatalf("expected original record field `id` to ride along, got %v", row0.Inspect())
	}
}

func TestEmailValidateBulkMissingField(t *testing.T) {
	records := &object.Array{Elements: []object.Object{
		hashOf(map[string]object.Object{"id": &object.Integer{Value: 1}}),
	}}
	res := builtinEmailValidateBulk(records, &object.String{Value: "email"})
	report := res.(*object.Hash)
	invalidCount, _ := hashGet(report, "invalid_count")
	if invalidCount.(*object.Integer).Value != 1 {
		t.Fatalf("invalid_count = %v, want 1 (missing field)", invalidCount.Inspect())
	}
}
