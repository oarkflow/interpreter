package builtins

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
)

var nonSlugChars = regexp.MustCompile(`[^a-z0-9]+`)

func splitWords(input string) []string {
	if input == "" {
		return nil
	}
	var out []string
	var token []rune
	runes := []rune(input)
	flush := func() {
		if len(token) > 0 {
			out = append(out, strings.ToLower(string(token)))
			token = token[:0]
		}
	}
	for i, r := range runes {
		isSep := !(unicode.IsLetter(r) || unicode.IsDigit(r))
		if isSep {
			flush()
			continue
		}
		if i > 0 {
			prev := runes[i-1]
			if unicode.IsLower(prev) && unicode.IsUpper(r) {
				flush()
			}
		}
		token = append(token, r)
	}
	flush()
	return out
}

func capitalizeWord(s string) string {
	if s == "" {
		return s
	}
	r := []rune(strings.ToLower(s))
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func init() {
	eval.RegisterBuiltins(map[string]*object.Builtin{
		"trim": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				return &object.String{Value: strings.TrimSpace(s)}
			},
		},
		"replace": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 3 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=3", len(args))}
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				old, errObj := asString(args[1], "old")
				if errObj != nil {
					return errObj
				}
				newVal, errObj := asString(args[2], "new")
				if errObj != nil {
					return errObj
				}
				return &object.String{Value: strings.ReplaceAll(s, old, newVal)}
			},
		},
		"starts_with": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				prefix, errObj := asString(args[1], "prefix")
				if errObj != nil {
					return errObj
				}
				return object.NativeBoolToBooleanObject(strings.HasPrefix(s, prefix))
			},
		},
		"ends_with": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				suffix, errObj := asString(args[1], "suffix")
				if errObj != nil {
					return errObj
				}
				return object.NativeBoolToBooleanObject(strings.HasSuffix(s, suffix))
			},
		},
		"repeat": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				count, errObj := asInt(args[1], "count")
				if errObj != nil {
					return errObj
				}
				if count < 0 {
					return &object.String{Value: "ERROR: repeat count must be >= 0"}
				}
				return &object.String{Value: strings.Repeat(s, int(count))}
			},
		},
		"substring": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 3 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=3", len(args))}
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				start, errObj := asInt(args[1], "start")
				if errObj != nil {
					return errObj
				}
				end, errObj := asInt(args[2], "end")
				if errObj != nil {
					return errObj
				}
				runes := []rune(s)
				if start < 0 || end < start || end > int64(len(runes)) {
					return &object.String{Value: "ERROR: invalid substring bounds"}
				}
				return &object.String{Value: string(runes[start:end])}
			},
		},
		"index_of": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				sub, errObj := asString(args[1], "substr")
				if errObj != nil {
					return errObj
				}
				return &object.Integer{Value: int64(strings.Index(s, sub))}
			},
		},
		"title": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				words := splitWords(s)
				for i, w := range words {
					words[i] = capitalizeWord(w)
				}
				return &object.String{Value: strings.Join(words, " ")}
			},
		},
		"slug": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				words := splitWords(s)
				slug := strings.Join(words, "-")
				slug = nonSlugChars.ReplaceAllString(slug, "-")
				slug = strings.Trim(slug, "-")
				return &object.String{Value: slug}
			},
		},
		"snake_case": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				return &object.String{Value: strings.Join(splitWords(s), "_")}
			},
		},
		"kebab_case": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				return &object.String{Value: strings.Join(splitWords(s), "-")}
			},
		},
		"camel_case": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				words := splitWords(s)
				if len(words) == 0 {
					return &object.String{Value: ""}
				}
				for i := 1; i < len(words); i++ {
					words[i] = capitalizeWord(words[i])
				}
				return &object.String{Value: words[0] + strings.Join(words[1:], "")}
			},
		},
		"pascal_case": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				words := splitWords(s)
				for i := range words {
					words[i] = capitalizeWord(words[i])
				}
				return &object.String{Value: strings.Join(words, "")}
			},
		},
		"swap_case": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				runes := []rune(s)
				for i, r := range runes {
					switch {
					case unicode.IsLower(r):
						runes[i] = unicode.ToUpper(r)
					case unicode.IsUpper(r):
						runes[i] = unicode.ToLower(r)
					}
				}
				return &object.String{Value: string(runes)}
			},
		},
		"count_substr": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				sub, errObj := asString(args[1], "substr")
				if errObj != nil {
					return errObj
				}
				return &object.Integer{Value: int64(strings.Count(s, sub))}
			},
		},
		"truncate": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 2 || len(args) > 3 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2 or 3", len(args))}
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				maxLen, errObj := asInt(args[1], "max_len")
				if errObj != nil {
					return errObj
				}
				suffix := "..."
				if len(args) == 3 {
					suffix, errObj = asString(args[2], "suffix")
					if errObj != nil {
						return errObj
					}
				}
				runes := []rune(s)
				if maxLen < 0 {
					return &object.String{Value: "ERROR: max_len must be >= 0"}
				}
				if int(maxLen) >= len(runes) {
					return &object.String{Value: s}
				}
				return &object.String{Value: string(runes[:maxLen]) + suffix}
			},
		},
		"split_lines": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				s = strings.ReplaceAll(s, "\r\n", "\n")
				parts := strings.Split(s, "\n")
				out := make([]object.Object, len(parts))
				for i, p := range parts {
					out[i] = &object.String{Value: p}
				}
				return &object.Array{Elements: out}
			},
		},
		"regex_match": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				pattern, errObj := asString(args[1], "pattern")
				if errObj != nil {
					return errObj
				}
				re, err := regexp.Compile(pattern)
				if err != nil {
					return object.NewError("%s", err)
				}
				return object.NativeBoolToBooleanObject(re.MatchString(s))
			},
		},
		"regex_replace": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 3 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=3", len(args))}
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				pattern, errObj := asString(args[1], "pattern")
				if errObj != nil {
					return errObj
				}
				replacement, errObj := asString(args[2], "replacement")
				if errObj != nil {
					return errObj
				}
				re, err := regexp.Compile(pattern)
				if err != nil {
					return object.NewError("%s", err)
				}
				return &object.String{Value: re.ReplaceAllString(s, replacement)}
			},
		},
		"trim_prefix": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				prefix, errObj := asString(args[1], "prefix")
				if errObj != nil {
					return errObj
				}
				return &object.String{Value: strings.TrimPrefix(s, prefix)}
			},
		},
		"trim_suffix": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				suffix, errObj := asString(args[1], "suffix")
				if errObj != nil {
					return errObj
				}
				return &object.String{Value: strings.TrimSuffix(s, suffix)}
			},
		},
		"pad_left": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 2 || len(args) > 3 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2 or 3", len(args))}
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				width, errObj := asInt(args[1], "width")
				if errObj != nil {
					return errObj
				}
				pad := " "
				if len(args) == 3 {
					pad, errObj = asString(args[2], "pad")
					if errObj != nil {
						return errObj
					}
				}
				if width < 0 {
					return object.NewError("width must be >= 0")
				}
				if pad == "" {
					return object.NewError("pad must not be empty")
				}
				runes := []rune(s)
				if int64(len(runes)) >= width {
					return &object.String{Value: s}
				}
				padRunes := []rune(pad)
				need := int(width) - len(runes)
				out := make([]rune, 0, int(width))
				for i := 0; i < need; i++ {
					out = append(out, padRunes[i%len(padRunes)])
				}
				out = append(out, runes...)
				return &object.String{Value: string(out)}
			},
		},
		"pad_right": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 2 || len(args) > 3 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2 or 3", len(args))}
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				width, errObj := asInt(args[1], "width")
				if errObj != nil {
					return errObj
				}
				pad := " "
				if len(args) == 3 {
					pad, errObj = asString(args[2], "pad")
					if errObj != nil {
						return errObj
					}
				}
				if width < 0 {
					return object.NewError("width must be >= 0")
				}
				if pad == "" {
					return object.NewError("pad must not be empty")
				}
				runes := []rune(s)
				if int64(len(runes)) >= width {
					return &object.String{Value: s}
				}
				padRunes := []rune(pad)
				out := make([]rune, 0, int(width))
				out = append(out, runes...)
				for len(out) < int(width) {
					out = append(out, padRunes[(len(out)-len(runes))%len(padRunes)])
				}
				return &object.String{Value: string(out[:int(width)])}
			},
		},

		// -----------------------------------------------------------------
		// parse_type – type conversion / parsing
		// -----------------------------------------------------------------

		"parse_type": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 2 || len(args) > 4 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2..4", len(args))}
				}
				target, errObj := asString(args[1], "target_type")
				if errObj != nil {
					return errObj
				}
				switch strings.ToLower(target) {
				case "int", "integer":
					return toIntObject(args[0])
				case "float", "number":
					return toFloatObject(args[0])
				case "string":
					return &object.String{Value: args[0].Inspect()}
				case "bool", "boolean":
					return toBoolObject(args[0])
				case "time", "timestamp":
					if len(args) == 2 {
						if args[0].Type() == object.STRING_OBJ {
							tm, err := time.Parse(time.RFC3339, args[0].(*object.String).Value)
							if err != nil {
								return object.NewError("%s", err)
							}
							return &object.Integer{Value: tm.Unix()}
						}
						return object.NewError("parse_type(time) with 2 args expects STRING input")
					}
					if args[0].Type() != object.STRING_OBJ {
						return object.NewError("parse_type(time) expects STRING input")
					}
					if len(args) == 3 {
						format, e := asString(args[2], "format")
						if e != nil {
							return e
						}
						tm, err := time.Parse(normalizeTimeFormat(format), args[0].(*object.String).Value)
						if err != nil {
							return object.NewError("%s", err)
						}
						return &object.Integer{Value: tm.Unix()}
					}
					format, e := asString(args[2], "format")
					if e != nil {
						return e
					}
					tz, e := asString(args[3], "timezone")
					if e != nil {
						return e
					}
					loc, locErr := loadLocationOrError(tz)
					if locErr != nil {
						return locErr
					}
					tm, err := time.ParseInLocation(normalizeTimeFormat(format), args[0].(*object.String).Value, loc)
					if err != nil {
						return object.NewError("%s", err)
					}
					return &object.Integer{Value: tm.Unix()}
				default:
					return object.NewError("unsupported target_type %q", target)
				}
			},
		},
	})
}
