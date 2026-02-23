package interpreter

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
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

var stringBuiltins = map[string]*Builtin{
	"trim": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			s, errObj := asString(args[0], "s")
			if errObj != nil {
				return errObj
			}
			return &String{Value: strings.TrimSpace(s)}
		},
	},
	"replace": {
		Fn: func(args ...Object) Object {
			if len(args) != 3 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=3", len(args))}
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
			return &String{Value: strings.ReplaceAll(s, old, newVal)}
		},
	},
	"starts_with": {
		Fn: func(args ...Object) Object {
			if len(args) != 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
			}
			s, errObj := asString(args[0], "s")
			if errObj != nil {
				return errObj
			}
			prefix, errObj := asString(args[1], "prefix")
			if errObj != nil {
				return errObj
			}
			return nativeBoolToBooleanObject(strings.HasPrefix(s, prefix))
		},
	},
	"ends_with": {
		Fn: func(args ...Object) Object {
			if len(args) != 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
			}
			s, errObj := asString(args[0], "s")
			if errObj != nil {
				return errObj
			}
			suffix, errObj := asString(args[1], "suffix")
			if errObj != nil {
				return errObj
			}
			return nativeBoolToBooleanObject(strings.HasSuffix(s, suffix))
		},
	},
	"repeat": {
		Fn: func(args ...Object) Object {
			if len(args) != 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
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
				return &String{Value: "ERROR: repeat count must be >= 0"}
			}
			return &String{Value: strings.Repeat(s, int(count))}
		},
	},
	"substring": {
		Fn: func(args ...Object) Object {
			if len(args) != 3 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=3", len(args))}
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
				return &String{Value: "ERROR: invalid substring bounds"}
			}
			return &String{Value: string(runes[start:end])}
		},
	},
	"index_of": {
		Fn: func(args ...Object) Object {
			if len(args) != 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
			}
			s, errObj := asString(args[0], "s")
			if errObj != nil {
				return errObj
			}
			sub, errObj := asString(args[1], "substr")
			if errObj != nil {
				return errObj
			}
			return &Integer{Value: int64(strings.Index(s, sub))}
		},
	},
	"title": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			s, errObj := asString(args[0], "s")
			if errObj != nil {
				return errObj
			}
			words := splitWords(s)
			for i, w := range words {
				words[i] = capitalizeWord(w)
			}
			return &String{Value: strings.Join(words, " ")}
		},
	},
	"slug": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			s, errObj := asString(args[0], "s")
			if errObj != nil {
				return errObj
			}
			words := splitWords(s)
			slug := strings.Join(words, "-")
			slug = nonSlugChars.ReplaceAllString(slug, "-")
			slug = strings.Trim(slug, "-")
			return &String{Value: slug}
		},
	},
	"snake_case": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			s, errObj := asString(args[0], "s")
			if errObj != nil {
				return errObj
			}
			return &String{Value: strings.Join(splitWords(s), "_")}
		},
	},
	"kebab_case": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			s, errObj := asString(args[0], "s")
			if errObj != nil {
				return errObj
			}
			return &String{Value: strings.Join(splitWords(s), "-")}
		},
	},
	"camel_case": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			s, errObj := asString(args[0], "s")
			if errObj != nil {
				return errObj
			}
			words := splitWords(s)
			if len(words) == 0 {
				return &String{Value: ""}
			}
			for i := 1; i < len(words); i++ {
				words[i] = capitalizeWord(words[i])
			}
			return &String{Value: words[0] + strings.Join(words[1:], "")}
		},
	},
	"pascal_case": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			s, errObj := asString(args[0], "s")
			if errObj != nil {
				return errObj
			}
			words := splitWords(s)
			for i := range words {
				words[i] = capitalizeWord(words[i])
			}
			return &String{Value: strings.Join(words, "")}
		},
	},
	"swap_case": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
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
			return &String{Value: string(runes)}
		},
	},
	"count_substr": {
		Fn: func(args ...Object) Object {
			if len(args) != 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
			}
			s, errObj := asString(args[0], "s")
			if errObj != nil {
				return errObj
			}
			sub, errObj := asString(args[1], "substr")
			if errObj != nil {
				return errObj
			}
			return &Integer{Value: int64(strings.Count(s, sub))}
		},
	},
	"truncate": {
		Fn: func(args ...Object) Object {
			if len(args) < 2 || len(args) > 3 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2 or 3", len(args))}
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
				return &String{Value: "ERROR: max_len must be >= 0"}
			}
			if int(maxLen) >= len(runes) {
				return &String{Value: s}
			}
			return &String{Value: string(runes[:maxLen]) + suffix}
		},
	},
	"split_lines": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			s, errObj := asString(args[0], "s")
			if errObj != nil {
				return errObj
			}
			s = strings.ReplaceAll(s, "\r\n", "\n")
			parts := strings.Split(s, "\n")
			out := make([]Object, len(parts))
			for i, p := range parts {
				out[i] = &String{Value: p}
			}
			return &Array{Elements: out}
		},
	},
}

func init() {
	registerBuiltins(stringBuiltins)
}
