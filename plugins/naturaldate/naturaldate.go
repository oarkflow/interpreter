// Package naturaldate wraps github.com/oarkflow/naturaldate, exposing
// natural-language date/time parsing ("tomorrow at 9am", "in 3 business
// days", "every first monday of the month") as SPL builtins. It follows the
// same plugin pattern as plugins/database, plugins/xql, etc.: a separate Go
// module so the root module doesn't carry this dependency, linked only into
// the "full" interpreter/playground binaries via plugins/builtins.go.
package naturaldate

import (
	"fmt"
	"time"

	"github.com/oarkflow/naturaldate"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
)

func init() {
	eval.RegisterPluginBuiltins(map[string]*object.Builtin{
		// naturaldate_parse(text[, opts]) -> (result, err)
		// Parses a single natural-language date/time expression. Returns a
		// HASH describing the parsed moment, or (null, err) when the text
		// could not be understood as a date/time expression.
		"naturaldate_parse": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 || len(args) > 2 {
					return object.NewError("naturaldate_parse() takes 1 or 2 arguments (text[, opts]), got %d", len(args))
				}
				text, errObj := asString(args[0], "text")
				if errObj != nil {
					return errObj
				}
				opts, errObj := parseOptions(args, 1)
				if errObj != nil {
					return errObj
				}
				result, ok := naturaldate.Parse(text, opts)
				if !ok {
					return tuple(object.NULL, &object.String{Value: fmt.Sprintf("naturaldate_parse: could not parse %q as a date/time expression", text)})
				}
				return tuple(resultToObject(result), object.NULL)
			},
		},

		// naturaldate_parse_all(text[, opts]) -> ARRAY of results
		// Scans free-form text and extracts every date/time expression found
		// in it, e.g. "remind me tomorrow at 9am and again next friday".
		"naturaldate_parse_all": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 || len(args) > 2 {
					return object.NewError("naturaldate_parse_all() takes 1 or 2 arguments (text[, opts]), got %d", len(args))
				}
				text, errObj := asString(args[0], "text")
				if errObj != nil {
					return errObj
				}
				opts, errObj := parseOptions(args, 1)
				if errObj != nil {
					return errObj
				}
				results := naturaldate.ParseAll(text, opts)
				elements := make([]object.Object, len(results))
				for i, r := range results {
					elements[i] = resultToObject(r)
				}
				return &object.Array{Elements: elements}
			},
		},
	})
}

// parseOptions reads an optional trailing `opts` HASH argument at args[idx]
// into naturaldate.Options. Supported keys: reference (RFC3339 STRING),
// location (IANA timezone name STRING), weekday_dir ("past"|"present"|
// "future" STRING), allow_embedded (BOOLEAN), holidays (ARRAY of
// "YYYY-MM-DD" STRINGs).
func parseOptions(args []object.Object, idx int) (naturaldate.Options, object.Object) {
	var opts naturaldate.Options
	if len(args) <= idx {
		return opts, nil
	}
	h, ok := args[idx].(*object.Hash)
	if !ok {
		return opts, object.NewError("argument `opts` must be HASH, got %s", args[idx].Type())
	}
	if v, ok := hashGet(h, "reference"); ok {
		s, errObj := asString(v, "opts.reference")
		if errObj != nil {
			return opts, errObj
		}
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return opts, object.NewError("opts.reference: %s", err)
		}
		opts.Reference = t
	}
	if v, ok := hashGet(h, "location"); ok {
		s, errObj := asString(v, "opts.location")
		if errObj != nil {
			return opts, errObj
		}
		loc, err := time.LoadLocation(s)
		if err != nil {
			return opts, object.NewError("opts.location: %s", err)
		}
		opts.Location = loc
	}
	if v, ok := hashGet(h, "weekday_dir"); ok {
		s, errObj := asString(v, "opts.weekday_dir")
		if errObj != nil {
			return opts, errObj
		}
		dir, err := parseDirection(s)
		if err != nil {
			return opts, object.NewError("opts.weekday_dir: %s", err)
		}
		opts.WeekdayDir = dir
	}
	if v, ok := hashGet(h, "allow_embedded"); ok {
		b, ok := v.(*object.Boolean)
		if !ok {
			return opts, object.NewError("argument `opts.allow_embedded` must be BOOLEAN, got %s", v.Type())
		}
		opts.AllowEmbedded = b.Value
	}
	if v, ok := hashGet(h, "holidays"); ok {
		arr, ok := v.(*object.Array)
		if !ok {
			return opts, object.NewError("argument `opts.holidays` must be ARRAY, got %s", v.Type())
		}
		holidays := make([]time.Time, 0, len(arr.Elements))
		for _, el := range arr.Elements {
			s, errObj := asString(el, "opts.holidays[]")
			if errObj != nil {
				return opts, errObj
			}
			t, err := time.Parse("2006-01-02", s)
			if err != nil {
				return opts, object.NewError("opts.holidays: %s", err)
			}
			holidays = append(holidays, t)
		}
		opts.Holidays = holidays
	}
	return opts, nil
}

func parseDirection(s string) (naturaldate.Direction, error) {
	switch s {
	case "past":
		return naturaldate.Past, nil
	case "present":
		return naturaldate.Present, nil
	case "future":
		return naturaldate.Future, nil
	default:
		return naturaldate.Past, fmt.Errorf("must be one of \"past\", \"present\", \"future\", got %q", s)
	}
}

func directionName(d naturaldate.Direction) string {
	switch d {
	case naturaldate.Past:
		return "past"
	case naturaldate.Future:
		return "future"
	default:
		return "present"
	}
}

func unitName(u naturaldate.Unit) string {
	switch u {
	case naturaldate.UnitSecond:
		return "second"
	case naturaldate.UnitMinute:
		return "minute"
	case naturaldate.UnitHour:
		return "hour"
	case naturaldate.UnitDay:
		return "day"
	case naturaldate.UnitWeek:
		return "week"
	case naturaldate.UnitMonth:
		return "month"
	case naturaldate.UnitYear:
		return "year"
	default:
		return "none"
	}
}

func resultToObject(r naturaldate.Result) object.Object {
	pairs := map[string]object.Object{
		"time":      &object.String{Value: r.Time.Format(time.RFC3339)},
		"unix":      &object.Integer{Value: r.Time.Unix()},
		"direction": &object.String{Value: directionName(r.Direction)},
		"truncated": &object.String{Value: unitName(r.Truncated)},
		"has_recur": object.NativeBoolToBooleanObject(r.HasRecur),
	}
	if r.HasRecur {
		pairs["recur"] = recurrenceToObject(r.Recur)
	}
	return hashFromPairs(pairs)
}

func recurrenceToObject(r naturaldate.Recurrence) object.Object {
	pairs := map[string]object.Object{
		"every":      &object.String{Value: unitName(r.Every)},
		"interval":   &object.Integer{Value: int64(r.Interval)},
		"has_at":     object.NativeBoolToBooleanObject(r.HasAt),
		"on_day":     &object.Integer{Value: int64(r.OnDay)},
		"on_date":    &object.Integer{Value: int64(r.OnDate)},
		"on_month":   &object.Integer{Value: int64(r.OnMonth)},
		"on_ordinal": &object.Integer{Value: int64(r.OnOrdinal)},
	}
	if r.HasAt {
		pairs["at"] = hashFromPairs(map[string]object.Object{
			"hour": &object.Integer{Value: int64(r.At.Hour)},
			"min":  &object.Integer{Value: int64(r.At.Min)},
			"sec":  &object.Integer{Value: int64(r.At.Sec)},
		})
	}
	return hashFromPairs(pairs)
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

func tuple(values ...object.Object) *object.Array {
	return &object.Array{Elements: values}
}
