package builtins

import (
	"bufio"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5"
	crand "crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	mrand "math/rand"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oarkflow/interpreter/pkg/config"
	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/security"
)

// ---------------------------------------------------------------------------
// Channel / ImmutableValue / GeneratorValue
// ---------------------------------------------------------------------------

type Channel struct {
	ch chan object.Object
}

func (c *Channel) Type() object.ObjectType { return object.BUILTIN_OBJ }
func (c *Channel) Inspect() string         { return "<channel>" }

type ImmutableValue struct {
	inner object.Object
}

func (i *ImmutableValue) Type() object.ObjectType { return i.inner.Type() }
func (i *ImmutableValue) Inspect() string         { return i.inner.Inspect() }

type GeneratorValue struct {
	elements []object.Object
}

func (g *GeneratorValue) Type() object.ObjectType { return object.ARRAY_OBJ }
func (g *GeneratorValue) Inspect() string {
	return (&object.Array{Elements: g.elements}).Inspect()
}

// ---------------------------------------------------------------------------
// Telemetry state
// ---------------------------------------------------------------------------

var telemetryState = struct {
	mu      sync.Mutex
	metrics map[string]float64
	traces  []string
}{metrics: map[string]float64{}, traces: []string{}}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

func deepImmutableClone(obj object.Object) object.Object {
	switch v := obj.(type) {
	case *object.Array:
		elements := make([]object.Object, len(v.Elements))
		for i, el := range v.Elements {
			elements[i] = deepImmutableClone(el)
		}
		return &ImmutableValue{inner: &object.Array{Elements: elements}}
	case *object.Hash:
		pairs := make(map[object.HashKey]object.HashPair, len(v.Pairs))
		for k, pair := range v.Pairs {
			pairs[k] = object.HashPair{Key: pair.Key, Value: deepImmutableClone(pair.Value)}
		}
		return &ImmutableValue{inner: &object.Hash{Pairs: pairs}}
	default:
		return &ImmutableValue{inner: v}
	}
}

func hashStringValue(h *object.Hash, key string) string {
	if h == nil {
		return ""
	}
	pair, ok := h.Pairs[(&object.String{Value: key}).HashKey()]
	if !ok {
		return ""
	}
	if s, ok := pair.Value.(*object.String); ok {
		return strings.TrimSpace(s.Value)
	}
	return strings.TrimSpace(pair.Value.Inspect())
}

func hashBoolValue(h *object.Hash, key string, def bool) bool {
	if h == nil {
		return def
	}
	pair, ok := h.Pairs[(&object.String{Value: key}).HashKey()]
	if !ok {
		return def
	}
	if b, ok := pair.Value.(*object.Boolean); ok {
		return b.Value
	}
	return def
}

func hashStringArray(h *object.Hash, key string) []string {
	if h == nil {
		return nil
	}
	pair, ok := h.Pairs[(&object.String{Value: key}).HashKey()]
	if !ok {
		return nil
	}
	arr, ok := pair.Value.(*object.Array)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr.Elements))
	for _, el := range arr.Elements {
		if s, ok := el.(*object.String); ok && strings.TrimSpace(s.Value) != "" {
			out = append(out, strings.TrimSpace(s.Value))
		}
	}
	return out
}

func objectToNative(obj object.Object) interface{} {
	switch v := obj.(type) {
	case *object.Null:
		return nil
	case *object.Boolean:
		return v.Value
	case *object.Integer:
		return v.Value
	case *object.Float:
		return v.Value
	case *object.String:
		return v.Value
	case *object.Secret:
		return v.Value
	case *object.Array:
		out := make([]interface{}, len(v.Elements))
		for i, el := range v.Elements {
			out[i] = objectToNative(el)
		}
		return out
	case *object.Hash:
		out := make(map[string]interface{}, len(v.Pairs))
		for _, pair := range v.Pairs {
			out[pair.Key.Inspect()] = objectToNative(pair.Value)
		}
		return out
	default:
		return v.Inspect()
	}
}

func objectToDisplayString(obj object.Object) string {
	if obj == nil {
		return "null"
	}
	if s, ok := obj.(*object.Secret); ok {
		_ = s
		return "***"
	}
	if s, ok := obj.(*object.String); ok {
		return s.Value
	}
	return obj.Inspect()
}

// numericToFloat converts an Integer or Float object to float64.
func numericToFloat(obj object.Object) float64 {
	switch v := obj.(type) {
	case *object.Integer:
		return float64(v.Value)
	case *object.Float:
		return v.Value
	default:
		return 0
	}
}

// objectToInt64ForFmt coerces any SPL value to int64 for %d/%o/%x/%X/%b verbs.
func objectToInt64ForFmt(obj object.Object) int64 {
	switch v := obj.(type) {
	case *object.Integer:
		return v.Value
	case *object.Float:
		return int64(v.Value)
	case *object.Boolean:
		if v.Value {
			return 1
		}
		return 0
	case *object.String:
		if n, err := strconv.ParseInt(v.Value, 0, 64); err == nil {
			return n
		}
		if f, err := strconv.ParseFloat(v.Value, 64); err == nil {
			return int64(f)
		}
		return 0
	case *object.Null:
		return 0
	default:
		return 0
	}
}

// objectToFloat64ForFmt coerces any SPL value to float64 for %f/%e/%E/%g/%G verbs.
func objectToFloat64ForFmt(obj object.Object) float64 {
	switch v := obj.(type) {
	case *object.Float:
		return v.Value
	case *object.Integer:
		return float64(v.Value)
	case *object.Boolean:
		if v.Value {
			return 1.0
		}
		return 0.0
	case *object.String:
		if f, err := strconv.ParseFloat(v.Value, 64); err == nil {
			return f
		}
		return 0.0
	case *object.Null:
		return 0.0
	default:
		return 0.0
	}
}

// objectToBoolForFmt coerces any SPL value to bool for %t verb.
func objectToBoolForFmt(obj object.Object) bool {
	switch v := obj.(type) {
	case *object.Boolean:
		return v.Value
	case *object.Integer:
		return v.Value != 0
	case *object.Float:
		return v.Value != 0.0
	case *object.String:
		return v.Value != ""
	case *object.Null:
		return false
	case *object.Array:
		return len(v.Elements) > 0
	default:
		return true
	}
}

func objectToFmtValue(obj object.Object) interface{} {
	if obj == nil {
		return nil
	}
	switch v := obj.(type) {
	case *object.Null:
		return nil
	case *object.Boolean:
		return v.Value
	case *object.Integer:
		return v.Value
	case *object.Float:
		return v.Value
	case *object.String:
		return v.Value
	default:
		return objectToNative(obj)
	}
}

func splSprintf(format string, args []object.Object) (string, object.Object) {
	var out strings.Builder
	argIndex := 0

	for i := 0; i < len(format); i++ {
		ch := format[i]
		if ch != '%' {
			out.WriteByte(ch)
			continue
		}

		if i+1 < len(format) && format[i+1] == '%' {
			out.WriteByte('%')
			i++
			continue
		}

		j := i + 1
		for j < len(format) {
			c := format[j]
			if strings.ContainsRune("+-#0 .123456789*", rune(c)) {
				j++
				continue
			}
			break
		}
		if j >= len(format) {
			return "", &object.String{Value: "ERROR: invalid format string: incomplete placeholder"}
		}

		verb := format[j]
		if argIndex >= len(args) {
			return "", &object.String{Value: fmt.Sprintf("ERROR: not enough format arguments for placeholder %%%c", verb)}
		}

		arg := args[argIndex]
		argIndex++

		spec := format[i : j+1]
		switch verb {
		case 'T':
			if arg == nil {
				out.WriteString("NULL")
			} else {
				out.WriteString(arg.Type().String())
			}
		case 'v':
			if arg == nil {
				out.WriteString("null")
			} else {
				out.WriteString(arg.Inspect())
			}
		case 's':
			out.WriteString(fmt.Sprintf(spec, objectToDisplayString(arg)))
		case 'd', 'o', 'x', 'X', 'b':
			// Integer verbs: coerce all numeric-like values to int64.
			out.WriteString(fmt.Sprintf(spec, objectToInt64ForFmt(arg)))
		case 'f', 'e', 'E', 'g', 'G':
			// Float verbs: coerce all numeric-like values to float64.
			out.WriteString(fmt.Sprintf(spec, objectToFloat64ForFmt(arg)))
		case 't':
			// Boolean verb: coerce to bool.
			out.WriteString(fmt.Sprintf(spec, objectToBoolForFmt(arg)))
		case 'c':
			// Character verb: Integer → rune, String → first rune.
			switch v := arg.(type) {
			case *object.Integer:
				out.WriteRune(rune(v.Value))
			case *object.String:
				for _, r := range v.Value {
					out.WriteRune(r)
					break
				}
			default:
				out.WriteString(fmt.Sprintf(spec, objectToFmtValue(arg)))
			}
		case 'q':
			// Quoted-string verb.
			out.WriteString(fmt.Sprintf(spec, objectToDisplayString(arg)))
		default:
			out.WriteString(fmt.Sprintf(spec, objectToFmtValue(arg)))
		}

		i = j
	}

	if argIndex < len(args) {
		return "", &object.String{Value: fmt.Sprintf("ERROR: too many format arguments: used=%d, total=%d", argIndex, len(args))}
	}

	return out.String(), nil
}

func interpolateTemplate(template string, data object.Object, positional []object.Object) (string, object.Object) {
	var out strings.Builder

	resolve := func(key string) (object.Object, bool) {
		if data != nil {
			switch d := data.(type) {
			case *object.Hash:
				hk := (&object.String{Value: key}).HashKey()
				if pair, ok := d.Pairs[hk]; ok {
					return pair.Value, true
				}
				return nil, false
			case *object.Array:
				idx, err := strconv.Atoi(key)
				if err != nil || idx < 0 || idx >= len(d.Elements) {
					return nil, false
				}
				return d.Elements[idx], true
			}
		}
		idx, err := strconv.Atoi(key)
		if err != nil || idx < 0 || idx >= len(positional) {
			return nil, false
		}
		return positional[idx], true
	}

	for i := 0; i < len(template); i++ {
		ch := template[i]
		if ch == '{' {
			if i+1 < len(template) && template[i+1] == '{' {
				out.WriteByte('{')
				i++
				continue
			}
			end := strings.IndexByte(template[i+1:], '}')
			if end < 0 {
				return "", &object.String{Value: "ERROR: invalid interpolation template: missing }"}
			}
			end += i + 1
			key := strings.TrimSpace(template[i+1 : end])
			if key == "" {
				return "", &object.String{Value: "ERROR: invalid interpolation template: empty placeholder"}
			}
			val, ok := resolve(key)
			if !ok {
				return "", &object.String{Value: fmt.Sprintf("ERROR: missing interpolation value for {%s}", key)}
			}
			out.WriteString(objectToDisplayString(val))
			i = end
			continue
		}
		if ch == '}' && i+1 < len(template) && template[i+1] == '}' {
			out.WriteByte('}')
			i++
			continue
		}
		out.WriteByte(ch)
	}

	return out.String(), nil
}

func toIntObject(arg object.Object) object.Object {
	switch v := arg.(type) {
	case *object.Integer:
		return v
	case *object.Float:
		return &object.Integer{Value: int64(v.Value)}
	case *object.String:
		val, err := strconv.ParseInt(v.Value, 10, 64)
		if err != nil {
			return object.NewError("could not convert %q to int", v.Value)
		}
		return &object.Integer{Value: val}
	case *object.Boolean:
		if v.Value {
			return &object.Integer{Value: 1}
		}
		return &object.Integer{Value: 0}
	default:
		return object.NewError("cannot convert %s to int", arg.Type())
	}
}

func toFloatObject(arg object.Object) object.Object {
	switch v := arg.(type) {
	case *object.Float:
		return v
	case *object.Integer:
		return &object.Float{Value: float64(v.Value)}
	case *object.String:
		val, err := strconv.ParseFloat(v.Value, 64)
		if err != nil {
			return object.NewError("could not convert %q to float", v.Value)
		}
		return &object.Float{Value: val}
	case *object.Boolean:
		if v.Value {
			return &object.Float{Value: 1}
		}
		return &object.Float{Value: 0}
	default:
		return object.NewError("cannot convert %s to float", arg.Type())
	}
}

func toBoolObject(arg object.Object) object.Object {
	switch v := arg.(type) {
	case *object.Boolean:
		return v
	case *object.Integer:
		return object.NativeBoolToBooleanObject(v.Value != 0)
	case *object.Float:
		return object.NativeBoolToBooleanObject(v.Value != 0)
	case *object.String:
		s := strings.TrimSpace(strings.ToLower(v.Value))
		switch s {
		case "true", "1", "yes", "y", "on":
			return object.TRUE
		case "false", "0", "no", "n", "off", "":
			return object.FALSE
		default:
			return object.NewError("could not parse %q as bool", v.Value)
		}
	default:
		return object.NewError("cannot convert %s to bool", arg.Type())
	}
}

func ParseJSONToObject(input string) (object.Object, error) {
	var v interface{}
	if err := json.Unmarshal([]byte(input), &v); err != nil {
		return nil, err
	}
	return eval.ToObject(v), nil
}

func randomBytes(n int) ([]byte, error) {
	if n <= 0 {
		return nil, fmt.Errorf("n must be > 0")
	}
	b := make([]byte, n)
	if _, err := crand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

func randomInt(max int) (int, error) {
	if max <= 0 {
		return 0, fmt.Errorf("max must be > 0")
	}
	n, err := crand.Int(crand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()), nil
}

func randomStringWithAlphabet(length int, alphabet string) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("length must be > 0")
	}
	if len(alphabet) == 0 {
		return "", fmt.Errorf("alphabet must not be empty")
	}
	var out strings.Builder
	out.Grow(length)
	for i := 0; i < length; i++ {
		idx, err := randomInt(len(alphabet))
		if err != nil {
			return "", err
		}
		out.WriteByte(alphabet[idx])
	}
	return out.String(), nil
}

func hashPasswordSHA(password string, algo string) (string, error) {
	saltBytes, err := randomBytes(16)
	if err != nil {
		return "", err
	}
	salt := hex.EncodeToString(saltBytes)
	combined := salt + ":" + password
	var sum []byte
	switch strings.ToLower(algo) {
	case "sha256":
		h := sha256.Sum256([]byte(combined))
		sum = h[:]
	case "sha512":
		h := sha512.Sum512([]byte(combined))
		sum = h[:]
	default:
		return "", fmt.Errorf("unsupported password hash algo: %s", algo)
	}
	return fmt.Sprintf("%s$%s$%s", strings.ToLower(algo), salt, hex.EncodeToString(sum)), nil
}

func verifyPasswordSHA(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 3 {
		return false, fmt.Errorf("invalid password hash format")
	}
	algo := parts[0]
	salt := parts[1]
	expectedHex := parts[2]
	combined := salt + ":" + password
	var sum []byte
	switch algo {
	case "sha256":
		h := sha256.Sum256([]byte(combined))
		sum = h[:]
	case "sha512":
		h := sha512.Sum512([]byte(combined))
		sum = h[:]
	default:
		return false, fmt.Errorf("unsupported password hash algo: %s", algo)
	}
	got := hex.EncodeToString(sum)
	return subtle.ConstantTimeCompare([]byte(got), []byte(expectedHex)) == 1, nil
}

func encryptAESGCM(key, plaintext, aad string) (string, error) {
	keyBytes := []byte(key)
	if !(len(keyBytes) == 16 || len(keyBytes) == 24 || len(keyBytes) == 32) {
		return "", fmt.Errorf("key must be 16, 24, or 32 bytes")
	}
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce, err := randomBytes(gcm.NonceSize())
	if err != nil {
		return "", err
	}
	cipherText := gcm.Seal(nil, nonce, []byte(plaintext), []byte(aad))
	out := append(nonce, cipherText...)
	return base64.StdEncoding.EncodeToString(out), nil
}

func decryptAESGCM(key, b64CipherText, aad string) (string, error) {
	keyBytes := []byte(key)
	if !(len(keyBytes) == 16 || len(keyBytes) == 24 || len(keyBytes) == 32) {
		return "", fmt.Errorf("key must be 16, 24, or 32 bytes")
	}
	raw, err := base64.StdEncoding.DecodeString(b64CipherText)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce := raw[:gcm.NonceSize()]
	cipherText := raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, cipherText, []byte(aad))
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func makeUUIDv4() (string, error) {
	b, err := randomBytes(16)
	if err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func makeUUIDv7() (string, error) {
	b, err := randomBytes(16)
	if err != nil {
		return "", err
	}
	ms := uint64(time.Now().UnixMilli())
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	b[6] = (b[6] & 0x0f) | 0x70
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// ---------------------------------------------------------------------------
// Core builtins
// ---------------------------------------------------------------------------

func init() {
	eval.RegisterBuiltins(map[string]*object.Builtin{
		"len": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				switch arg := args[0].(type) {
				case *object.String:
					return &object.Integer{Value: int64(len(arg.Value))}
				case *object.Array:
					return &object.Integer{Value: int64(len(arg.Elements))}
				case *object.Hash:
					return &object.Integer{Value: int64(len(arg.Pairs))}
				default:
					return &object.String{Value: fmt.Sprintf("argument to `len` not supported, got %s", args[0].Type())}
				}
			},
		},
		"keys": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				if args[0].Type() != object.HASH_OBJ {
					return &object.String{Value: fmt.Sprintf("argument to `keys` must be HASH, got %s", args[0].Type())}
				}
				hash := args[0].(*object.Hash)
				elements := []object.Object{}
				for _, pair := range hash.Pairs {
					elements = append(elements, pair.Key)
				}
				return &object.Array{Elements: elements}
			},
		},
		"type": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				return &object.String{Value: args[0].Type().String()}
			},
		},
		"puts": {
			Fn: func(args ...object.Object) object.Object {
				for _, arg := range args {
					fmt.Println(arg.Inspect())
				}
				return object.NULL
			},
		},
		"upper": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				if args[0].Type() != object.STRING_OBJ {
					return &object.String{Value: fmt.Sprintf("argument to `upper` must be STRING, got %s", args[0].Type())}
				}
				return &object.String{Value: strings.ToUpper(args[0].(*object.String).Value)}
			},
		},
		"lower": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				if args[0].Type() != object.STRING_OBJ {
					return &object.String{Value: fmt.Sprintf("argument to `lower` must be STRING, got %s", args[0].Type())}
				}
				return &object.String{Value: strings.ToLower(args[0].(*object.String).Value)}
			},
		},
		"split": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
				}
				if args[0].Type() != object.STRING_OBJ || args[1].Type() != object.STRING_OBJ {
					return &object.String{Value: fmt.Sprintf("arguments to `split` must be STRING, got %s and %s", args[0].Type(), args[1].Type())}
				}
				parts := strings.Split(args[0].(*object.String).Value, args[1].(*object.String).Value)
				elements := make([]object.Object, len(parts))
				for i, part := range parts {
					elements[i] = &object.String{Value: part}
				}
				return &object.Array{Elements: elements}
			},
		},
		"join": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
				}
				if args[0].Type() != object.ARRAY_OBJ {
					return &object.String{Value: fmt.Sprintf("first argument to `join` must be ARRAY, got %s", args[0].Type())}
				}
				if args[1].Type() != object.STRING_OBJ {
					return &object.String{Value: fmt.Sprintf("second argument to `join` must be STRING, got %s", args[1].Type())}
				}
				arr := args[0].(*object.Array)
				sep := args[1].(*object.String).Value
				parts := make([]string, len(arr.Elements))
				for i, el := range arr.Elements {
					parts[i] = el.Inspect()
					if el.Type() == object.STRING_OBJ {
						parts[i] = el.(*object.String).Value
					}
				}
				return &object.String{Value: strings.Join(parts, sep)}
			},
		},
		"push": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
				}
				if args[0].Type() != object.ARRAY_OBJ {
					return &object.String{Value: fmt.Sprintf("argument to `push` must be ARRAY, got %s", args[0].Type())}
				}
				arr := args[0].(*object.Array)
				newElements := make([]object.Object, len(arr.Elements)+1)
				copy(newElements, arr.Elements)
				newElements[len(arr.Elements)] = args[1]
				return &object.Array{Elements: newElements}
			},
		},
		"time": {
			Fn: func(args ...object.Object) object.Object {
				return &object.Integer{Value: time.Now().Unix()}
			},
		},
		"sleep": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				if args[0].Type() != object.INTEGER_OBJ {
					return &object.String{Value: fmt.Sprintf("argument to `sleep` must be INTEGER (ms), got %s", args[0].Type())}
				}
				ms := args[0].(*object.Integer).Value
				time.Sleep(time.Duration(ms) * time.Millisecond)
				return object.NULL
			},
		},
		"to_int": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				return toIntObject(args[0])
			},
		},
		"to_float": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				return toFloatObject(args[0])
			},
		},
		"to_string": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				return &object.String{Value: args[0].Inspect()}
			},
		},
		"parse_string": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				return &object.String{Value: args[0].Inspect()}
			},
		},
		"parse_bool": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				return toBoolObject(args[0])
			},
		},
		"is_int": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				if _, ok := args[0].(*object.Integer); ok {
					return object.TRUE
				}
				return object.FALSE
			},
		},
		"is_float": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				if _, ok := args[0].(*object.Float); ok {
					return object.TRUE
				}
				return object.FALSE
			},
		},
		"is_number": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				t := args[0].Type()
				if t == object.INTEGER_OBJ || t == object.FLOAT_OBJ {
					return object.TRUE
				}
				return object.FALSE
			},
		},
		"is_string": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				if _, ok := args[0].(*object.String); ok {
					return object.TRUE
				}
				return object.FALSE
			},
		},
		"is_bool": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				if _, ok := args[0].(*object.Boolean); ok {
					return object.TRUE
				}
				return object.FALSE
			},
		},
		"is_array": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				if _, ok := args[0].(*object.Array); ok {
					return object.TRUE
				}
				return object.FALSE
			},
		},
		"is_hash": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				if _, ok := args[0].(*object.Hash); ok {
					return object.TRUE
				}
				return object.FALSE
			},
		},
		"is_null": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				if _, ok := args[0].(*object.Null); ok {
					return object.TRUE
				}
				return object.FALSE
			},
		},
		"is_function": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				t := args[0].Type()
				if t == object.FUNCTION_OBJ || t == object.BUILTIN_OBJ {
					return object.TRUE
				}
				return object.FALSE
			},
		},
		"typeof": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				if args[0] == nil {
					return &object.String{Value: "NULL"}
				}
				return &object.String{Value: args[0].Type().String()}
			},
		},
		"input": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) > 0 {
					fmt.Print(args[0].Inspect())
				}
				reader := bufio.NewReader(os.Stdin)
				text, _ := reader.ReadString('\n')
				return &object.String{Value: strings.TrimSuffix(text, "\n")}
			},
		},
		"random": {
			Fn: func(args ...object.Object) object.Object {
				max := int64(math.MaxInt64)
				if len(args) > 0 {
					if args[0].Type() != object.INTEGER_OBJ {
						return &object.String{Value: fmt.Sprintf("argument to `random` must be INTEGER, got %s", args[0].Type())}
					}
					max = args[0].(*object.Integer).Value
				}
				if max <= 0 {
					return object.NewError("random max must be > 0")
				}
				return &object.Integer{Value: mrand.Int63n(max)}
			},
		},
		"seed_random": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				seed, errObj := asInt(args[0], "seed")
				if errObj != nil {
					return errObj
				}
				mrand.Seed(seed)
				return object.NULL
			},
		},
		"abs": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				switch v := args[0].(type) {
				case *object.Integer:
					if v.Value < 0 {
						return &object.Integer{Value: -v.Value}
					}
					return &object.Integer{Value: v.Value}
				case *object.Float:
					return &object.Float{Value: math.Abs(v.Value)}
				default:
					return &object.String{Value: fmt.Sprintf("argument to `abs` must be INTEGER or FLOAT, got %s", args[0].Type())}
				}
			},
		},
		"pow": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
				}
				if args[0].Type() != object.INTEGER_OBJ || args[1].Type() != object.INTEGER_OBJ {
					return &object.String{Value: fmt.Sprintf("arguments to `pow` must be INTEGER, got %s and %s", args[0].Type(), args[1].Type())}
				}
				base := float64(args[0].(*object.Integer).Value)
				exp := float64(args[1].(*object.Integer).Value)
				return &object.Integer{Value: int64(math.Pow(base, exp))}
			},
		},
		"sqrt": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				if args[0].Type() != object.INTEGER_OBJ {
					return &object.String{Value: fmt.Sprintf("argument to `sqrt` must be INTEGER, got %s", args[0].Type())}
				}
				val := float64(args[0].(*object.Integer).Value)
				return &object.Integer{Value: int64(math.Sqrt(val))}
			},
		},
		"min": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=at least 1", len(args))}
				}
				// Check if any argument is a float — if so, compare as float64.
				hasFloat := false
				for _, arg := range args {
					if arg.Type() == object.FLOAT_OBJ {
						hasFloat = true
					} else if arg.Type() != object.INTEGER_OBJ {
						return &object.String{Value: fmt.Sprintf("arguments to `min` must be INTEGER or FLOAT, got %s", arg.Type())}
					}
				}
				if hasFloat {
					minVal := numericToFloat(args[0])
					for _, arg := range args[1:] {
						v := numericToFloat(arg)
						if v < minVal {
							minVal = v
						}
					}
					return &object.Float{Value: minVal}
				}
				minVal := args[0].(*object.Integer).Value
				for _, arg := range args[1:] {
					val := arg.(*object.Integer).Value
					if val < minVal {
						minVal = val
					}
				}
				return &object.Integer{Value: minVal}
			},
		},
		"max": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=at least 1", len(args))}
				}
				hasFloat := false
				for _, arg := range args {
					if arg.Type() == object.FLOAT_OBJ {
						hasFloat = true
					} else if arg.Type() != object.INTEGER_OBJ {
						return &object.String{Value: fmt.Sprintf("arguments to `max` must be INTEGER or FLOAT, got %s", arg.Type())}
					}
				}
				if hasFloat {
					maxVal := numericToFloat(args[0])
					for _, arg := range args[1:] {
						v := numericToFloat(arg)
						if v > maxVal {
							maxVal = v
						}
					}
					return &object.Float{Value: maxVal}
				}
				maxVal := args[0].(*object.Integer).Value
				for _, arg := range args[1:] {
					val := arg.(*object.Integer).Value
					if val > maxVal {
						maxVal = val
					}
				}
				return &object.Integer{Value: maxVal}
			},
		},
		"contains": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
				}
				switch args[0].Type() {
				case object.STRING_OBJ:
					if args[1].Type() != object.STRING_OBJ {
						return &object.String{Value: fmt.Sprintf("needle must be STRING for string contains, got %s", args[1].Type())}
					}
					return object.NativeBoolToBooleanObject(strings.Contains(args[0].(*object.String).Value, args[1].(*object.String).Value))
				case object.ARRAY_OBJ:
					arr := args[0].(*object.Array)
					target := args[1].Inspect()
					for _, el := range arr.Elements {
						if el.Inspect() == target {
							return object.TRUE
						}
					}
					return object.FALSE
				default:
					return &object.String{Value: fmt.Sprintf("contains not supported for %s", args[0].Type())}
				}
			},
		},
		"sort": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				if args[0].Type() != object.ARRAY_OBJ {
					return &object.String{Value: fmt.Sprintf("argument to `sort` must be ARRAY, got %s", args[0].Type())}
				}
				arr := args[0].(*object.Array)
				out := make([]object.Object, len(arr.Elements))
				copy(out, arr.Elements)
				allInt := true
				allString := true
				for _, el := range out {
					if el.Type() != object.INTEGER_OBJ {
						allInt = false
					}
					if el.Type() != object.STRING_OBJ {
						allString = false
					}
				}
				switch {
				case allInt:
					sort.Slice(out, func(i, j int) bool {
						return out[i].(*object.Integer).Value < out[j].(*object.Integer).Value
					})
				case allString:
					sort.Slice(out, func(i, j int) bool {
						return out[i].(*object.String).Value < out[j].(*object.String).Value
					})
				default:
					return &object.String{Value: "ERROR: sort supports only homogeneous ARRAY of INTEGER or STRING"}
				}
				return &object.Array{Elements: out}
			},
		},
		"uniq": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				if args[0].Type() != object.ARRAY_OBJ {
					return &object.String{Value: fmt.Sprintf("argument to `uniq` must be ARRAY, got %s", args[0].Type())}
				}
				arr := args[0].(*object.Array)
				seen := map[string]bool{}
				out := []object.Object{}
				for _, el := range arr.Elements {
					key := el.Type().String() + "::" + el.Inspect()
					if !seen[key] {
						seen[key] = true
						out = append(out, el)
					}
				}
				return &object.Array{Elements: out}
			},
		},
		"find": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
				}
				if args[0].Type() != object.ARRAY_OBJ {
					return &object.String{Value: fmt.Sprintf("first argument to `find` must be ARRAY, got %s", args[0].Type())}
				}
				arr := args[0].(*object.Array)
				target := args[1].Type().String() + "::" + args[1].Inspect()
				for _, el := range arr.Elements {
					if el.Type().String()+"::"+el.Inspect() == target {
						return el
					}
				}
				return object.NULL
			},
		},
		"reduce": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
				}
				if args[0].Type() != object.ARRAY_OBJ {
					return &object.String{Value: fmt.Sprintf("first argument to `reduce` must be ARRAY, got %s", args[0].Type())}
				}
				op, errObj := asString(args[1], "operator")
				if errObj != nil {
					return errObj
				}
				arr := args[0].(*object.Array)
				if len(arr.Elements) == 0 {
					return object.NULL
				}
				switch op {
				case "sum":
					var total int64
					for _, el := range arr.Elements {
						if el.Type() != object.INTEGER_OBJ {
							return &object.String{Value: "ERROR: reduce(sum) supports INTEGER array only"}
						}
						total += el.(*object.Integer).Value
					}
					return &object.Integer{Value: total}
				case "concat":
					var sb strings.Builder
					for _, el := range arr.Elements {
						sb.WriteString(el.Inspect())
					}
					return &object.String{Value: sb.String()}
				default:
					return &object.String{Value: fmt.Sprintf("ERROR: unsupported reduce operator %q", op)}
				}
			},
		},
		"range": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 || len(args) > 3 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1..3", len(args))}
				}
				var start, end, step int64
				var errObj object.Object
				if len(args) == 1 {
					start = 0
					end, errObj = asInt(args[0], "end")
					if errObj != nil {
						return errObj
					}
					step = 1
				} else {
					start, errObj = asInt(args[0], "start")
					if errObj != nil {
						return errObj
					}
					end, errObj = asInt(args[1], "end")
					if errObj != nil {
						return errObj
					}
					step = 1
					if len(args) == 3 {
						step, errObj = asInt(args[2], "step")
						if errObj != nil {
							return errObj
						}
					}
				}
				if step == 0 {
					return &object.String{Value: "ERROR: step cannot be zero"}
				}
				elements := []object.Object{}
				if step > 0 {
					for i := start; i < end; i += step {
						elements = append(elements, &object.Integer{Value: i})
					}
				} else {
					for i := start; i > end; i += step {
						elements = append(elements, &object.Integer{Value: i})
					}
				}
				return &object.Array{Elements: elements}
			},
		},
		"await_all": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("await_all: expected 1 argument (array of futures), got %d", len(args))
				}
				arr, ok := args[0].(*object.Array)
				if !ok {
					return object.NewError("await_all: argument must be an array, got %s", args[0].Type())
				}
				results := make([]object.Object, len(arr.Elements))
				for i, elem := range arr.Elements {
					if future, ok := elem.(*object.Future); ok {
						results[i] = future.Resolve()
					} else {
						results[i] = elem
					}
				}
				return &object.Array{Elements: results}
			},
		},
		"await_race": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("await_race: expected 1 argument (array of futures), got %d", len(args))
				}
				arr, ok := args[0].(*object.Array)
				if !ok {
					return object.NewError("await_race: argument must be an array, got %s", args[0].Type())
				}
				if len(arr.Elements) == 0 {
					return object.NULL
				}
				ch := make(chan object.Object, len(arr.Elements))
				for _, elem := range arr.Elements {
					elem := elem
					go func() {
						if future, ok := elem.(*object.Future); ok {
							ch <- future.Resolve()
						} else {
							ch <- elem
						}
					}()
				}
				return <-ch
			},
		},
		"random_bytes": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				n, errObj := asInt(args[0], "n")
				if errObj != nil {
					return errObj
				}
				b, err := randomBytes(int(n))
				if err != nil {
					return &object.String{Value: fmt.Sprintf("ERROR: %s", err)}
				}
				return &object.String{Value: base64.StdEncoding.EncodeToString(b)}
			},
		},
		"random_string": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
				}
				n, errObj := asInt(args[0], "length")
				if errObj != nil {
					return errObj
				}
				alphabet, errObj := asString(args[1], "alphabet")
				if errObj != nil {
					return errObj
				}
				s, err := randomStringWithAlphabet(int(n), alphabet)
				if err != nil {
					return &object.String{Value: fmt.Sprintf("ERROR: %s", err)}
				}
				return &object.String{Value: s}
			},
		},
		"uuid": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) > 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=0 or 1", len(args))}
				}
				version := int64(7)
				if len(args) == 1 {
					if args[0].Type() == object.INTEGER_OBJ {
						version = args[0].(*object.Integer).Value
					} else if args[0].Type() == object.STRING_OBJ {
						parsed, perr := strconv.ParseInt(strings.TrimSpace(args[0].(*object.String).Value), 10, 64)
						if perr != nil {
							return &object.String{Value: fmt.Sprintf("ERROR: invalid uuid version %q", args[0].(*object.String).Value)}
						}
						version = parsed
					} else {
						return &object.String{Value: fmt.Sprintf("argument to `uuid` must be INTEGER or STRING, got %s", args[0].Type())}
					}
				}
				var (
					id  string
					err error
				)
				switch version {
				case 4:
					id, err = makeUUIDv4()
				case 7:
					id, err = makeUUIDv7()
				default:
					return &object.String{Value: fmt.Sprintf("ERROR: unsupported uuid version %d (supported: 4, 7)", version)}
				}
				if err != nil {
					return &object.String{Value: fmt.Sprintf("ERROR: %s", err)}
				}
				return &object.String{Value: id}
			},
		},
		"hash": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
				}
				algo, errObj := asString(args[0], "algo")
				if errObj != nil {
					return errObj
				}
				data, errObj := asString(args[1], "data")
				if errObj != nil {
					return errObj
				}
				switch strings.ToLower(algo) {
				case "md5":
					sum := md5.Sum([]byte(data))
					return &object.String{Value: hex.EncodeToString(sum[:])}
				case "sha256":
					sum := sha256.Sum256([]byte(data))
					return &object.String{Value: hex.EncodeToString(sum[:])}
				case "sha512":
					sum := sha512.Sum512([]byte(data))
					return &object.String{Value: hex.EncodeToString(sum[:])}
				default:
					return &object.String{Value: fmt.Sprintf("ERROR: unsupported hash algorithm %q", algo)}
				}
			},
		},
		"hmac": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 3 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=3", len(args))}
				}
				algo, errObj := asString(args[0], "algo")
				if errObj != nil {
					return errObj
				}
				key, errObj := asString(args[1], "key")
				if errObj != nil {
					return errObj
				}
				data, errObj := asString(args[2], "data")
				if errObj != nil {
					return errObj
				}
				switch strings.ToLower(algo) {
				case "sha256":
					h := hmac.New(sha256.New, []byte(key))
					h.Write([]byte(data))
					return &object.String{Value: hex.EncodeToString(h.Sum(nil))}
				case "sha512":
					h := hmac.New(sha512.New, []byte(key))
					h.Write([]byte(data))
					return &object.String{Value: hex.EncodeToString(h.Sum(nil))}
				default:
					return &object.String{Value: fmt.Sprintf("ERROR: unsupported hmac algorithm %q", algo)}
				}
			},
		},
		"password_hash": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 || len(args) > 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1 or 2", len(args))}
				}
				password, errObj := asString(args[0], "password")
				if errObj != nil {
					return errObj
				}
				algo := "sha256"
				if len(args) == 2 {
					algo, errObj = asString(args[1], "algo")
					if errObj != nil {
						return errObj
					}
				}
				encoded, err := hashPasswordSHA(password, algo)
				if err != nil {
					return &object.String{Value: fmt.Sprintf("ERROR: %s", err)}
				}
				return &object.String{Value: encoded}
			},
		},
		"password_verify": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
				}
				password, errObj := asString(args[0], "password")
				if errObj != nil {
					return errObj
				}
				encoded, errObj := asString(args[1], "encoded")
				if errObj != nil {
					return errObj
				}
				ok, err := verifyPasswordSHA(password, encoded)
				if err != nil {
					return &object.String{Value: fmt.Sprintf("ERROR: %s", err)}
				}
				return object.NativeBoolToBooleanObject(ok)
			},
		},
		"encrypt": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 3 || len(args) > 4 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=3 or 4", len(args))}
				}
				alg, errObj := asString(args[0], "alg")
				if errObj != nil {
					return errObj
				}
				key, errObj := asString(args[1], "key")
				if errObj != nil {
					return errObj
				}
				plaintext, errObj := asString(args[2], "plaintext")
				if errObj != nil {
					return errObj
				}
				aad := ""
				if len(args) == 4 {
					aad, errObj = asString(args[3], "aad")
					if errObj != nil {
						return errObj
					}
				}
				switch strings.ToLower(alg) {
				case "aes_gcm", "aes-256-gcm":
					out, err := encryptAESGCM(key, plaintext, aad)
					if err != nil {
						return &object.String{Value: fmt.Sprintf("ERROR: %s", err)}
					}
					return &object.String{Value: out}
				default:
					return &object.String{Value: fmt.Sprintf("ERROR: unsupported encrypt algorithm %q", alg)}
				}
			},
		},
		"decrypt": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 3 || len(args) > 4 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=3 or 4", len(args))}
				}
				alg, errObj := asString(args[0], "alg")
				if errObj != nil {
					return errObj
				}
				key, errObj := asString(args[1], "key")
				if errObj != nil {
					return errObj
				}
				cipherText, errObj := asString(args[2], "ciphertext")
				if errObj != nil {
					return errObj
				}
				aad := ""
				if len(args) == 4 {
					aad, errObj = asString(args[3], "aad")
					if errObj != nil {
						return errObj
					}
				}
				switch strings.ToLower(alg) {
				case "aes_gcm", "aes-256-gcm":
					out, err := decryptAESGCM(key, cipherText, aad)
					if err != nil {
						return &object.String{Value: fmt.Sprintf("ERROR: %s", err)}
					}
					return &object.String{Value: out}
				default:
					return &object.String{Value: fmt.Sprintf("ERROR: unsupported decrypt algorithm %q", alg)}
				}
			},
		},
		"constant_time_eq": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
				}
				a, errObj := asString(args[0], "a")
				if errObj != nil {
					return errObj
				}
				b, errObj := asString(args[1], "b")
				if errObj != nil {
					return errObj
				}
				return object.NativeBoolToBooleanObject(subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1)
			},
		},
		"base64_encode": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				s, errObj := asString(args[0], "data")
				if errObj != nil {
					return errObj
				}
				return &object.String{Value: base64.StdEncoding.EncodeToString([]byte(s))}
			},
		},
		"base64_decode": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				s, errObj := asString(args[0], "data")
				if errObj != nil {
					return errObj
				}
				out, err := base64.StdEncoding.DecodeString(s)
				if err != nil {
					return &object.String{Value: fmt.Sprintf("ERROR: %s", err)}
				}
				return &object.String{Value: string(out)}
			},
		},
		"hex_encode": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				s, errObj := asString(args[0], "data")
				if errObj != nil {
					return errObj
				}
				return &object.String{Value: hex.EncodeToString([]byte(s))}
			},
		},
		"hex_decode": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				s, errObj := asString(args[0], "data")
				if errObj != nil {
					return errObj
				}
				out, err := hex.DecodeString(s)
				if err != nil {
					return &object.String{Value: fmt.Sprintf("ERROR: %s", err)}
				}
				return &object.String{Value: string(out)}
			},
		},
		"url_encode": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				s, errObj := asString(args[0], "value")
				if errObj != nil {
					return errObj
				}
				return &object.String{Value: url.QueryEscape(s)}
			},
		},
		"url_decode": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				s, errObj := asString(args[0], "value")
				if errObj != nil {
					return errObj
				}
				out, err := url.QueryUnescape(s)
				if err != nil {
					return &object.String{Value: fmt.Sprintf("ERROR: %s", err)}
				}
				return &object.String{Value: out}
			},
		},
		"json_encode": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				raw := objectToNative(args[0])
				out, err := json.Marshal(raw)
				if err != nil {
					return &object.String{Value: fmt.Sprintf("ERROR: %s", err)}
				}
				return &object.String{Value: string(out)}
			},
		},
		"json_decode": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				s, errObj := asString(args[0], "json")
				if errObj != nil {
					return errObj
				}
				obj, err := ParseJSONToObject(s)
				if err != nil {
					return &object.String{Value: fmt.Sprintf("ERROR: %s", err)}
				}
				return obj
			},
		},
		"secret": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				s, errObj := asString(args[0], "value")
				if errObj != nil {
					return errObj
				}
				return &object.Secret{Value: s}
			},
		},
		"secret_reveal": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				if s, ok := args[0].(*object.Secret); ok {
					return &object.String{Value: s.Value}
				}
				if args[0].Type() == object.STRING_OBJ {
					return args[0]
				}
				return object.NewError("argument `secret_value` must be SECRET or STRING, got %s", args[0].Type())
			},
		},
		"secret_mask": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 || len(args) > 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1 or 2", len(args))}
				}
				s, errObj := asString(args[0], "value")
				if errObj != nil {
					return errObj
				}
				visible := int64(2)
				if len(args) == 2 {
					visible, errObj = asInt(args[1], "visible")
					if errObj != nil {
						return errObj
					}
					if visible < 0 {
						visible = 0
					}
				}
				if len(s) == 0 {
					return &object.String{Value: ""}
				}
				if int(visible) >= len(s) {
					return &object.String{Value: strings.Repeat("*", len(s))}
				}
				masked := strings.Repeat("*", len(s)-int(visible)) + s[len(s)-int(visible):]
				return &object.String{Value: masked}
			},
		},
		"parse_int": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 || len(args) > 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1 or 2", len(args))}
				}
				s, errObj := asString(args[0], "value")
				if errObj != nil {
					return errObj
				}
				base := int64(10)
				if len(args) == 2 {
					base, errObj = asInt(args[1], "base")
					if errObj != nil {
						return errObj
					}
				}
				v, err := strconv.ParseInt(s, int(base), 64)
				if err != nil {
					return &object.String{Value: fmt.Sprintf("ERROR: %s", err)}
				}
				return &object.Integer{Value: v}
			},
		},
		"parse_float": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				s, errObj := asString(args[0], "value")
				if errObj != nil {
					return errObj
				}
				v, err := strconv.ParseFloat(s, 64)
				if err != nil {
					return &object.String{Value: fmt.Sprintf("ERROR: %s", err)}
				}
				return &object.Float{Value: v}
			},
		},
		"sprintf": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=at least 1", len(args))}
				}
				format, errObj := asString(args[0], "format")
				if errObj != nil {
					return errObj
				}
				out, ferr := splSprintf(format, args[1:])
				if ferr != nil {
					return ferr
				}
				return &object.String{Value: out}
			},
		},
		"printf": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=at least 1", len(args))}
				}
				format, errObj := asString(args[0], "format")
				if errObj != nil {
					return errObj
				}
				out, ferr := splSprintf(format, args[1:])
				if ferr != nil {
					return ferr
				}
				fmt.Print(out)
				return &object.String{Value: out}
			},
		},
		"interpolate": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=at least 2", len(args))}
				}
				template, errObj := asString(args[0], "template")
				if errObj != nil {
					return errObj
				}
				data := args[1]
				if data.Type() != object.HASH_OBJ && data.Type() != object.ARRAY_OBJ && data.Type() != object.NULL_OBJ {
					return &object.String{Value: fmt.Sprintf("argument `data` must be HASH, ARRAY, or NULL, got %s", data.Type())}
				}
				out, ferr := interpolateTemplate(template, data, args[2:])
				if ferr != nil {
					return ferr
				}
				return &object.String{Value: out}
			},
		},
		"Error": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 || len(args) > 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1 or 2", len(args))}
				}
				msg := objectToDisplayString(args[0])
				pairs := map[object.HashKey]object.HashPair{}
				put := func(k string, v object.Object) {
					key := &object.String{Value: k}
					pairs[key.HashKey()] = object.HashPair{Key: key, Value: v}
				}
				put("name", &object.String{Value: "Error"})
				put("message", &object.String{Value: msg})
				put("code", &object.String{Value: "E_RUNTIME"})
				put("stack", &object.String{Value: ""})
				if len(args) == 2 {
					if details, ok := args[1].(*object.Hash); ok {
						for _, field := range []string{"code", "stack", "path", "line", "column"} {
							if p, ok := details.Pairs[(&object.String{Value: field}).HashKey()]; ok {
								put(field, p.Value)
							}
						}
					}
				}
				return &object.Hash{Pairs: pairs}
			},
		},
		"channel": {
			Fn: func(args ...object.Object) object.Object {
				buffer := 0
				if len(args) > 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=0 or 1", len(args))}
				}
				if len(args) == 1 {
					i, ok := args[0].(*object.Integer)
					if !ok || i.Value < 0 {
						return &object.String{Value: "channel buffer must be non-negative integer"}
					}
					buffer = int(i.Value)
				}
				return &Channel{ch: make(chan object.Object, buffer)}
			},
		},
		"send": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return &object.String{Value: "send expects 2 arguments"}
				}
				ch, ok := args[0].(*Channel)
				if !ok {
					return &object.String{Value: "send first argument must be channel"}
				}
				ch.ch <- args[1]
				return object.NULL
			},
		},
		"recv": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: "recv expects 1 argument"}
				}
				ch, ok := args[0].(*Channel)
				if !ok {
					return &object.String{Value: "recv argument must be channel"}
				}
				return <-ch.ch
			},
		},
		"metric": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 2 {
					return &object.String{Value: "metric(name, value[, labels]) expects at least 2 arguments"}
				}
				name, ok := args[0].(*object.String)
				if !ok {
					return &object.String{Value: "metric name must be STRING"}
				}
				val := 0.0
				switch n := args[1].(type) {
				case *object.Integer:
					val = float64(n.Value)
				case *object.Float:
					val = n.Value
				default:
					return &object.String{Value: "metric value must be number"}
				}
				telemetryState.mu.Lock()
				telemetryState.metrics[name.Value] += val
				telemetryState.mu.Unlock()
				return object.TRUE
			},
		},
		"trace": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 {
					return &object.String{Value: "trace(name[, attrs]) expects at least 1 argument"}
				}
				name, ok := args[0].(*object.String)
				if !ok {
					return &object.String{Value: "trace name must be STRING"}
				}
				entry := name.Value
				if len(args) > 1 {
					entry = entry + " " + args[1].Inspect()
				}
				telemetryState.mu.Lock()
				telemetryState.traces = append(telemetryState.traces, entry)
				telemetryState.mu.Unlock()
				return object.TRUE
			},
		},
		"immutable": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: "immutable expects exactly 1 argument"}
				}
				return deepImmutableClone(args[0])
			},
		},
		"password_generate": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 || len(args) > 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1 or 2", len(args))}
				}
				length, errObj := asInt(args[0], "length")
				if errObj != nil {
					return errObj
				}
				alphabet := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()-_=+[]{}<>?"
				if len(args) == 2 {
					custom, errObj := asString(args[1], "alphabet")
					if errObj != nil {
						return errObj
					}
					alphabet = custom
				}
				pass, err := randomStringWithAlphabet(int(length), alphabet)
				if err != nil {
					return &object.String{Value: fmt.Sprintf("ERROR: %s", err)}
				}
				return &object.String{Value: pass}
			},
		},
		"api_key": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
				}
				prefix, errObj := asString(args[0], "prefix")
				if errObj != nil {
					return errObj
				}
				n, errObj := asInt(args[1], "bytes")
				if errObj != nil {
					return errObj
				}
				b, err := randomBytes(int(n))
				if err != nil {
					return &object.String{Value: fmt.Sprintf("ERROR: %s", err)}
				}
				token := base64.RawURLEncoding.EncodeToString(b)
				return &object.String{Value: prefix + "_" + token}
			},
		},

		// -----------------------------------------------------------------
		// File I/O builtins
		// -----------------------------------------------------------------

		"read_file": {
			Fn: func(args ...object.Object) object.Object {
				retErr := func(msg string) object.Object {
					return &object.Array{Elements: []object.Object{object.NULL, &object.String{Value: msg}}}
				}
				retOk := func(val string) object.Object {
					return &object.Array{Elements: []object.Object{&object.String{Value: val}, object.NULL}}
				}
				if len(args) != 1 {
					return retErr(fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args)))
				}
				if args[0].Type() != object.STRING_OBJ {
					return retErr(fmt.Sprintf("argument to `read_file` must be STRING, got %s", args[0].Type()))
				}
				path := args[0].(*object.String).Value
				safePath, err := SanitizePathLocal(path)
				if err != nil {
					return retErr(fmt.Sprintf("%s", err))
				}
				if err := security.CheckFileReadAllowed(safePath); err != nil {
					return retErr(fmt.Sprintf("%s", err))
				}
				content, err := os.ReadFile(safePath)
				if err != nil {
					return retErr(fmt.Sprintf("%s", err))
				}
				return retOk(string(content))
			},
		},
		"write_file": {
			Fn: func(args ...object.Object) object.Object {
				retErr := func(msg string) object.Object {
					return &object.Array{Elements: []object.Object{object.FALSE, &object.String{Value: msg}}}
				}
				retOk := func() object.Object {
					return &object.Array{Elements: []object.Object{object.TRUE, object.NULL}}
				}
				if len(args) != 2 {
					return retErr(fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args)))
				}
				if args[0].Type() != object.STRING_OBJ || args[1].Type() != object.STRING_OBJ {
					return retErr(fmt.Sprintf("arguments to `write_file` must be STRING, got %s and %s", args[0].Type(), args[1].Type()))
				}
				path := args[0].(*object.String).Value
				safePath, err := SanitizePathLocal(path)
				if err != nil {
					return retErr(fmt.Sprintf("%s", err))
				}
				if err := security.CheckFileWriteAllowed(safePath); err != nil {
					return retErr(fmt.Sprintf("%s", err))
				}
				content := args[1].(*object.String).Value
				if err := os.MkdirAll(filepath.Dir(safePath), 0o755); err != nil {
					return retErr(fmt.Sprintf("%s", err))
				}
				err = os.WriteFile(safePath, []byte(content), 0644)
				if err != nil {
					return retErr(fmt.Sprintf("%s", err))
				}
				return retOk()
			},
		},
		"file_exists": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				if args[0].Type() != object.STRING_OBJ {
					return &object.String{Value: fmt.Sprintf("argument to `file_exists` must be STRING, got %s", args[0].Type())}
				}
				path := args[0].(*object.String).Value
				safePath, err := SanitizePathLocal(path)
				if err != nil {
					return &object.String{Value: fmt.Sprintf("IO ERROR: %s", err)}
				}
				if err := security.CheckFileReadAllowed(safePath); err != nil {
					return &object.String{Value: fmt.Sprintf("IO ERROR: %s", err)}
				}
				_, err = os.Stat(safePath)
				return object.NativeBoolToBooleanObject(!os.IsNotExist(err))
			},
		},
		"remove_file": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				if args[0].Type() != object.STRING_OBJ {
					return &object.String{Value: fmt.Sprintf("argument to `remove_file` must be STRING, got %s", args[0].Type())}
				}
				path := args[0].(*object.String).Value
				safePath, err := SanitizePathLocal(path)
				if err != nil {
					return &object.String{Value: fmt.Sprintf("IO ERROR: %s", err)}
				}
				if err := security.CheckFileWriteAllowed(safePath); err != nil {
					return &object.String{Value: fmt.Sprintf("IO ERROR: %s", err)}
				}
				err = os.Remove(safePath)
				if err != nil {
					return &object.String{Value: fmt.Sprintf("IO ERROR: %s", err)}
				}
				return object.TRUE
			},
		},

		// -----------------------------------------------------------------
		// OS / process builtins
		// -----------------------------------------------------------------

		"os_env": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) == 1 {
					if args[0].Type() != object.STRING_OBJ {
						return &object.String{Value: fmt.Sprintf("argument to `os_env` must be STRING, got %s", args[0].Type())}
					}
					key := args[0].(*object.String).Value
					return &object.String{Value: os.Getenv(key)}
				} else if len(args) == 2 {
					if args[0].Type() != object.STRING_OBJ || args[1].Type() != object.STRING_OBJ {
						return &object.String{Value: fmt.Sprintf("arguments to `os_env` must be STRING, got %s and %s", args[0].Type(), args[1].Type())}
					}
					key := args[0].(*object.String).Value
					val := args[1].(*object.String).Value
					if err := security.EnvWriteAllowed(key); err != nil {
						return &object.String{Value: fmt.Sprintf("ERROR: %s", err)}
					}
					os.Setenv(key, val)
					return object.NULL
				}
				return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1 or 2", len(args))}
			},
		},
		"exit": {
			Fn: func(args ...object.Object) object.Object {
				if err := security.ExitAllowed(); err != nil {
					return &object.String{Value: fmt.Sprintf("ERROR: %s", err)}
				}
				code := 0
				if len(args) == 1 {
					if args[0].Type() != object.INTEGER_OBJ {
						return &object.String{Value: fmt.Sprintf("argument to `exit` must be INTEGER, got %s", args[0].Type())}
					}
					code = int(args[0].(*object.Integer).Value)
				}
				os.Exit(code)
				return object.NULL
			},
		},
		"exec": {
			FnWithEnv: func(env *object.Environment, args ...object.Object) object.Object {
				if isTruthyEnvVar("SPL_DISABLE_EXEC") {
					return &object.String{Value: "ERROR: exec is disabled by SPL_DISABLE_EXEC"}
				}
				if len(args) < 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=at least 1", len(args))}
				}
				if args[0].Type() != object.STRING_OBJ {
					return &object.String{Value: fmt.Sprintf("command must be STRING, got %s", args[0].Type())}
				}
				cmdName := args[0].(*object.String).Value
				if err := security.CheckExecAllowed(cmdName); err != nil {
					return &object.String{Value: fmt.Sprintf("ERROR: %s", err)}
				}

				timeoutArgMs := int64(0)
				argEnd := len(args)
				if len(args) > 1 && args[len(args)-1].Type() == object.INTEGER_OBJ {
					timeoutArgMs = args[len(args)-1].(*object.Integer).Value
					argEnd = len(args) - 1
				}

				timeoutDur, timeoutErr := execTimeoutFromArgsAndEnv(timeoutArgMs)
				if timeoutErr != nil {
					return timeoutErr
				}

				cmdArgs := []string{}
				for i := 1; i < argEnd; i++ {
					if args[i].Type() != object.STRING_OBJ {
						return &object.String{Value: fmt.Sprintf("exec argument %d must be STRING, got %s", i, args[i].Type())}
					}
					cmdArgs = append(cmdArgs, args[i].(*object.String).Value)
				}

				ctx, cancel := runtimeContextWithTimeout(env, timeoutDur)
				defer cancel()
				cmd := exec.CommandContext(ctx, cmdName, cmdArgs...)

				output, err := cmd.CombinedOutput()
				if err != nil {
					if errors.Is(ctx.Err(), context.DeadlineExceeded) {
						return &object.String{Value: fmt.Sprintf("ERROR: command timed out after %dms\n%s", timeoutDur/time.Millisecond, string(output))}
					}
					if errors.Is(ctx.Err(), context.Canceled) {
						return &object.String{Value: fmt.Sprintf("ERROR: command cancelled\n%s", string(output))}
					}
					return &object.String{Value: fmt.Sprintf("ERROR: %s\n%s", err, string(output))}
				}
				return &object.String{Value: string(output)}
			},
		},

		// -----------------------------------------------------------------
		// Async
		// -----------------------------------------------------------------

		"go_async": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 {
					return object.NewError("go_async: expected at least 1 argument (function)")
				}
				fn := args[0]
				fnArgs := args[1:]
				go func() {
					eval.ApplyFn(fn, fnArgs, nil, nil)
				}()
				return object.NULL
			},
		},

		// -----------------------------------------------------------------
		// Config builtins
		// -----------------------------------------------------------------

		"config_load": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 || len(args) > 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1 or 2", len(args))}
				}
				path, errObj := asString(args[0], "path")
				if errObj != nil {
					return errObj
				}
				format := ""
				if len(args) == 2 {
					format, errObj = asString(args[1], "format")
					if errObj != nil {
						return errObj
					}
				}
				obj, err := config.LoadConfigObjectFromPath(path, format)
				if err != nil {
					return &object.String{Value: fmt.Sprintf("ERROR: %s", err)}
				}
				return obj
			},
		},
		"config_parse": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
				}
				raw, errObj := asString(args[0], "raw")
				if errObj != nil {
					return errObj
				}
				format, errObj := asString(args[1], "format")
				if errObj != nil {
					return errObj
				}
				obj, err := config.LoadConfigObjectFromRaw(raw, format)
				if err != nil {
					return &object.String{Value: fmt.Sprintf("ERROR: %s", err)}
				}
				return obj
			},
		},
	})
}

// ---------------------------------------------------------------------------
// Helper functions for the new builtins
// ---------------------------------------------------------------------------

// SanitizePathLocal resolves user-supplied paths to absolute paths.
func SanitizePathLocal(userPath string) (string, error) {
	return filepath.Abs(userPath)
}

// isTruthyEnvVar checks if an environment variable is set to a truthy value.
func isTruthyEnvVar(name string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	switch v {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// parsePositiveDurationMs parses a string as positive milliseconds.
func parsePositiveDurationMs(value string, envName string) (time.Duration, object.Object) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	ms, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, &object.String{Value: fmt.Sprintf("ERROR: invalid %s value %q: must be integer milliseconds", envName, value)}
	}
	if ms <= 0 {
		return 0, &object.String{Value: fmt.Sprintf("ERROR: invalid %s value %q: must be > 0", envName, value)}
	}
	return time.Duration(ms) * time.Millisecond, nil
}

// execTimeoutFromArgsAndEnv determines the exec timeout from a direct argument or env var.
func execTimeoutFromArgsAndEnv(timeoutArgMs int64) (time.Duration, object.Object) {
	if timeoutArgMs > 0 {
		return time.Duration(timeoutArgMs) * time.Millisecond, nil
	}
	if timeoutArgMs < 0 {
		return 0, &object.String{Value: "ERROR: exec timeout must be > 0 milliseconds"}
	}
	return parsePositiveDurationMs(os.Getenv("SPL_EXEC_TIMEOUT_MS"), "SPL_EXEC_TIMEOUT_MS")
}

// runtimeContextWithTimeout creates a context from the environment with an optional timeout.
func runtimeContextWithTimeout(env *object.Environment, timeout time.Duration) (context.Context, context.CancelFunc) {
	base := runtimeContext(env)
	if timeout > 0 {
		return context.WithTimeout(base, timeout)
	}
	return context.WithCancel(base)
}

// runtimeContext extracts the runtime context from the environment or returns background.
func runtimeContext(env *object.Environment) context.Context {
	if env != nil && env.RuntimeLimits != nil && env.RuntimeLimits.Ctx != nil {
		return env.RuntimeLimits.Ctx
	}
	return context.Background()
}
