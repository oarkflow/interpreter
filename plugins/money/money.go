// Package money wraps github.com/oarkflow/money, exposing fixed-point
// (integer-minor-unit) money arithmetic as SPL builtins so scripts can add,
// subtract, take a percentage of, and format monetary amounts without
// floating-point rounding error. Follows the same plugin pattern as
// plugins/naturaldate, plugins/xql, etc.
package money

import (
	"github.com/oarkflow/money"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
)

// A money value round-trips through SPL as a HASH: {amount, currency,
// decimals, display} - "amount" is the integer minor-unit amount (e.g.
// cents), so it survives JSON/HASH conversion without floating-point drift.
func moneyToHash(m money.Money) *object.Hash {
	c := m.Currency()
	return hashFromPairs(map[string]object.Object{
		"amount":   &object.Integer{Value: m.Minor()},
		"currency": &object.String{Value: c.Code},
		"decimals": &object.Integer{Value: int64(c.Decimals)},
		"display":  &object.String{Value: m.String()},
	})
}

func moneyFromHash(h *object.Hash, name string) (money.Money, object.Object) {
	amountObj, ok := hashGet(h, "amount")
	if !ok {
		return money.Money{}, object.NewError("argument `%s` is missing required field `amount`", name)
	}
	amount, errObj := asInt(amountObj, name+".amount")
	if errObj != nil {
		return money.Money{}, errObj
	}
	codeObj, ok := hashGet(h, "currency")
	if !ok {
		return money.Money{}, object.NewError("argument `%s` is missing required field `currency`", name)
	}
	code, errObj := asString(codeObj, name+".currency")
	if errObj != nil {
		return money.Money{}, errObj
	}
	currency, ok := money.GetCurrency(code)
	if !ok {
		return money.Money{}, object.NewError("%s: unknown currency code %q", name, code)
	}
	return money.NewFromMinor(amount, currency), nil
}

func init() {
	eval.RegisterPluginBuiltins(map[string]*object.Builtin{
		// money_new(amount, currency_code) -> (money, err)
		// Builds a money value from a major-unit amount (STRING, e.g. "19.99",
		// or INTEGER/FLOAT) and an ISO 4217 currency code, e.g.
		// money_new("19.99", "USD"). STRING amounts avoid float rounding.
		"money_new": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return object.NewError("money_new() takes 2 arguments (amount, currency_code), got %d", len(args))
				}
				code, errObj := asString(args[1], "currency_code")
				if errObj != nil {
					return errObj
				}
				currency, ok := money.GetCurrency(code)
				if !ok {
					return object.NewError("money_new: unknown currency code %q", code)
				}

				var m money.Money
				var err error
				switch amt := args[0].(type) {
				case *object.String:
					m, err = money.Parse(amt.Value, currency)
				case *object.Integer:
					m = money.NewFromFloat(float64(amt.Value), currency)
				case *object.Float:
					m = money.NewFromFloat(amt.Value, currency)
				default:
					return object.NewError("argument `amount` must be STRING, INTEGER, or FLOAT, got %s", args[0].Type())
				}
				if err != nil {
					return tuple(object.NULL, &object.String{Value: err.Error()})
				}
				return tuple(moneyToHash(m), object.NULL)
			},
		},

		// money_add(a, b) -> (money, err). Both operands must share a currency.
		"money_add": {
			Fn: func(args ...object.Object) object.Object {
				a, b, errObj := moneyPair(args, "money_add")
				if errObj != nil {
					return errObj
				}
				result, err := a.Add(b)
				if err != nil {
					return tuple(object.NULL, &object.String{Value: err.Error()})
				}
				return tuple(moneyToHash(result), object.NULL)
			},
		},

		// money_sub(a, b) -> (money, err). Both operands must share a currency.
		"money_sub": {
			Fn: func(args ...object.Object) object.Object {
				a, b, errObj := moneyPair(args, "money_sub")
				if errObj != nil {
					return errObj
				}
				result, err := a.Sub(b)
				if err != nil {
					return tuple(object.NULL, &object.String{Value: err.Error()})
				}
				return tuple(moneyToHash(result), object.NULL)
			},
		},

		// money_mul(a, factor) -> (money, err). factor is a whole-number
		// INTEGER multiplier (e.g. quantity * unit price); for fractional
		// multipliers use money_percent.
		"money_mul": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return object.NewError("money_mul() takes 2 arguments (money, factor), got %d", len(args))
				}
				h, ok := args[0].(*object.Hash)
				if !ok {
					return object.NewError("argument `money` must be HASH, got %s", args[0].Type())
				}
				a, errObj := moneyFromHash(h, "money")
				if errObj != nil {
					return errObj
				}
				factor, errObj := asInt(args[1], "factor")
				if errObj != nil {
					return errObj
				}
				result, err := a.Mul(factor)
				if err != nil {
					return tuple(object.NULL, &object.String{Value: err.Error()})
				}
				return tuple(moneyToHash(result), object.NULL)
			},
		},

		// money_percent(money, pct) -> money. Returns pct percent of the
		// amount, e.g. money_percent(price, 8.5) for 8.5% sales tax. Rounds
		// half up.
		"money_percent": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return object.NewError("money_percent() takes 2 arguments (money, pct), got %d", len(args))
				}
				h, ok := args[0].(*object.Hash)
				if !ok {
					return object.NewError("argument `money` must be HASH, got %s", args[0].Type())
				}
				a, errObj := moneyFromHash(h, "money")
				if errObj != nil {
					return errObj
				}
				pct, errObj := asFloat(args[1], "pct")
				if errObj != nil {
					return errObj
				}
				return moneyToHash(a.Percent(pct, money.HALF_UP))
			},
		},

		// money_format(money[, opts]) -> STRING
		// Formats a money value for display. Optional `opts` HASH supports:
		//   locale:         e.g. "en_US", "de_DE" (default from currency)
		//   without_symbol: BOOLEAN, omit the currency symbol
		//   without_comma:  BOOLEAN, omit thousands separators
		"money_format": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 || len(args) > 2 {
					return object.NewError("money_format() takes 1 or 2 arguments (money[, opts]), got %d", len(args))
				}
				h, ok := args[0].(*object.Hash)
				if !ok {
					return object.NewError("argument `money` must be HASH, got %s", args[0].Type())
				}
				a, errObj := moneyFromHash(h, "money")
				if errObj != nil {
					return errObj
				}
				var formatOpts []money.FormatOption
				if len(args) == 2 {
					optsHash, ok := args[1].(*object.Hash)
					if !ok {
						return object.NewError("argument `opts` must be HASH, got %s", args[1].Type())
					}
					if v, ok := hashGet(optsHash, "locale"); ok {
						locale, errObj := asString(v, "opts.locale")
						if errObj != nil {
							return errObj
						}
						formatOpts = append(formatOpts, money.WithLocale(money.Locale(locale)))
					}
					if v, ok := hashGet(optsHash, "without_symbol"); ok {
						b, ok := v.(*object.Boolean)
						if !ok {
							return object.NewError("argument `opts.without_symbol` must be BOOLEAN, got %s", v.Type())
						}
						if b.Value {
							formatOpts = append(formatOpts, money.WithoutSymbol())
						}
					}
					if v, ok := hashGet(optsHash, "without_comma"); ok {
						b, ok := v.(*object.Boolean)
						if !ok {
							return object.NewError("argument `opts.without_comma` must be BOOLEAN, got %s", v.Type())
						}
						if b.Value {
							formatOpts = append(formatOpts, money.WithoutComma())
						}
					}
				}
				return &object.String{Value: a.Format(formatOpts...)}
			},
		},
	})
}

func moneyPair(args []object.Object, fname string) (money.Money, money.Money, object.Object) {
	var zero money.Money
	if len(args) != 2 {
		return zero, zero, object.NewError("%s() takes 2 arguments (a, b), got %d", fname, len(args))
	}
	aHash, ok := args[0].(*object.Hash)
	if !ok {
		return zero, zero, object.NewError("argument `a` must be HASH, got %s", args[0].Type())
	}
	bHash, ok := args[1].(*object.Hash)
	if !ok {
		return zero, zero, object.NewError("argument `b` must be HASH, got %s", args[1].Type())
	}
	a, errObj := moneyFromHash(aHash, "a")
	if errObj != nil {
		return zero, zero, errObj
	}
	b, errObj := moneyFromHash(bHash, "b")
	if errObj != nil {
		return zero, zero, errObj
	}
	return a, b, nil
}

func hashFromPairs(pairs map[string]object.Object) *object.Hash {
	out := make(map[object.HashKey]object.HashPair, len(pairs))
	for k, v := range pairs {
		key := &object.String{Value: k}
		out[key.HashKey()] = object.HashPair{Key: key, Value: v}
	}
	return &object.Hash{Pairs: out}
}

func hashGet(h *object.Hash, key string) (object.Object, bool) {
	k := &object.String{Value: key}
	pair, ok := h.Pairs[k.HashKey()]
	if !ok {
		return nil, false
	}
	return pair.Value, true
}

func asString(arg object.Object, name string) (string, object.Object) {
	if s, ok := arg.(*object.Secret); ok {
		return s.Value, nil
	}
	if arg == nil {
		return "", object.NewError("argument `%s` must be STRING, got <nil>", name)
	}
	if arg.Type() != object.STRING_OBJ {
		return "", object.NewError("argument `%s` must be STRING, got %s", name, arg.Type())
	}
	return arg.(*object.String).Value, nil
}

func asInt(arg object.Object, name string) (int64, object.Object) {
	switch v := arg.(type) {
	case *object.Integer:
		return v.Value, nil
	case *object.Float:
		return int64(v.Value), nil
	case nil:
		return 0, object.NewError("argument `%s` must be INTEGER, got <nil>", name)
	default:
		return 0, object.NewError("argument `%s` must be INTEGER, got %s", name, arg.Type())
	}
}

func asFloat(arg object.Object, name string) (float64, object.Object) {
	switch v := arg.(type) {
	case *object.Float:
		return v.Value, nil
	case *object.Integer:
		return float64(v.Value), nil
	case nil:
		return 0, object.NewError("argument `%s` must be FLOAT, got <nil>", name)
	default:
		return 0, object.NewError("argument `%s` must be FLOAT, got %s", name, arg.Type())
	}
}

func tuple(values ...object.Object) *object.Array {
	return &object.Array{Elements: values}
}
