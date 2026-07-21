package money

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

func TestMoneyBuiltinsRegisterOnImport(t *testing.T) {
	for _, name := range []string{"money_new", "money_add", "money_sub", "money_mul", "money_percent", "money_format"} {
		if !eval.HasBuiltin(name) {
			t.Fatalf("expected money builtin %q to be registered", name)
		}
	}
}

func newMoney(t *testing.T, amount, currency string) *object.Hash {
	t.Helper()
	newFn := lookupBuiltin(t, "money_new")
	result := newFn.Fn(&object.String{Value: amount}, &object.String{Value: currency})
	arr, ok := result.(*object.Array)
	if !ok || len(arr.Elements) != 2 {
		t.Fatalf("money_new returned unexpected shape: %#v", result)
	}
	if arr.Elements[1] != object.NULL {
		t.Fatalf("money_new(%q, %q) returned err: %#v", amount, currency, arr.Elements[1])
	}
	h, ok := arr.Elements[0].(*object.Hash)
	if !ok {
		t.Fatalf("money_new returned non-hash: %#v", arr.Elements[0])
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

func TestMoneyNewAndFormat(t *testing.T) {
	m := newMoney(t, "19.99", "USD")
	amount := hashField(t, m, "amount")
	amt, ok := amount.(*object.Integer)
	if !ok || amt.Value != 1999 {
		t.Fatalf("expected amount=1999 (minor units), got %#v", amount)
	}

	formatFn := lookupBuiltin(t, "money_format")
	formatted := formatFn.Fn(m)
	s, ok := formatted.(*object.String)
	if !ok || s.Value == "" {
		t.Fatalf("money_format returned non-string or empty: %#v", formatted)
	}
}

func TestMoneyAddSubPercent(t *testing.T) {
	a := newMoney(t, "10.00", "USD")
	b := newMoney(t, "5.00", "USD")

	addFn := lookupBuiltin(t, "money_add")
	sumResult := addFn.Fn(a, b)
	sumArr, ok := sumResult.(*object.Array)
	if !ok || len(sumArr.Elements) != 2 || sumArr.Elements[1] != object.NULL {
		t.Fatalf("money_add returned unexpected result: %#v", sumResult)
	}
	sumHash := sumArr.Elements[0].(*object.Hash)
	sumAmount := hashField(t, sumHash, "amount").(*object.Integer)
	if sumAmount.Value != 1500 {
		t.Fatalf("expected 10.00 + 5.00 = 1500 minor units, got %d", sumAmount.Value)
	}

	subFn := lookupBuiltin(t, "money_sub")
	diffResult := subFn.Fn(a, b)
	diffArr := diffResult.(*object.Array)
	diffHash := diffArr.Elements[0].(*object.Hash)
	diffAmount := hashField(t, diffHash, "amount").(*object.Integer)
	if diffAmount.Value != 500 {
		t.Fatalf("expected 10.00 - 5.00 = 500 minor units, got %d", diffAmount.Value)
	}

	pctFn := lookupBuiltin(t, "money_percent")
	pctResult := pctFn.Fn(a, &object.Float{Value: 10})
	pctHash, ok := pctResult.(*object.Hash)
	if !ok {
		t.Fatalf("money_percent returned unexpected result: %#v", pctResult)
	}
	pctAmount := hashField(t, pctHash, "amount").(*object.Integer)
	if pctAmount.Value != 100 {
		t.Fatalf("expected 10%% of 10.00 = 100 minor units, got %d", pctAmount.Value)
	}
}

func TestMoneyAddRejectsCurrencyMismatch(t *testing.T) {
	usd := newMoney(t, "10.00", "USD")
	eur := newMoney(t, "10.00", "EUR")

	addFn := lookupBuiltin(t, "money_add")
	result := addFn.Fn(usd, eur)
	arr, ok := result.(*object.Array)
	if !ok || len(arr.Elements) != 2 {
		t.Fatalf("money_add returned unexpected shape: %#v", result)
	}
	if arr.Elements[1] == object.NULL {
		t.Fatalf("expected err for mismatched currencies")
	}
}
