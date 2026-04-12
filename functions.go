package interpreter

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
)

type Channel struct {
	ch chan Object
}

func (c *Channel) Type() ObjectType { return BUILTIN_OBJ }
func (c *Channel) Inspect() string  { return "<channel>" }

type ImmutableValue struct {
	inner Object
}

func (i *ImmutableValue) Type() ObjectType { return i.inner.Type() }
func (i *ImmutableValue) Inspect() string  { return i.inner.Inspect() }

type GeneratorValue struct {
	elements []Object
}

func (g *GeneratorValue) Type() ObjectType { return ARRAY_OBJ }
func (g *GeneratorValue) Inspect() string {
	return (&Array{Elements: g.elements}).Inspect()
}

var telemetryState = struct {
	mu      sync.Mutex
	metrics map[string]float64
	traces  []string
}{metrics: map[string]float64{}, traces: []string{}}

func deepImmutableClone(obj Object) Object {
	switch v := obj.(type) {
	case *Array:
		elements := make([]Object, len(v.Elements))
		for i, el := range v.Elements {
			elements[i] = deepImmutableClone(el)
		}
		return &ImmutableValue{inner: &Array{Elements: elements}}
	case *Hash:
		pairs := make(map[HashKey]HashPair, len(v.Pairs))
		for k, pair := range v.Pairs {
			pairs[k] = HashPair{Key: pair.Key, Value: deepImmutableClone(pair.Value)}
		}
		return &ImmutableValue{inner: &Hash{Pairs: pairs}}
	default:
		return &ImmutableValue{inner: v}
	}
}

func hashStringValue(h *Hash, key string) string {
	if h == nil {
		return ""
	}
	pair, ok := h.Pairs[(&String{Value: key}).HashKey()]
	if !ok {
		return ""
	}
	if s, ok := pair.Value.(*String); ok {
		return strings.TrimSpace(s.Value)
	}
	return strings.TrimSpace(pair.Value.Inspect())
}

func hashBoolValue(h *Hash, key string, def bool) bool {
	if h == nil {
		return def
	}
	pair, ok := h.Pairs[(&String{Value: key}).HashKey()]
	if !ok {
		return def
	}
	if b, ok := pair.Value.(*Boolean); ok {
		return b.Value
	}
	return def
}

func hashStringArray(h *Hash, key string) []string {
	if h == nil {
		return nil
	}
	pair, ok := h.Pairs[(&String{Value: key}).HashKey()]
	if !ok {
		return nil
	}
	arr, ok := pair.Value.(*Array)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr.Elements))
	for _, el := range arr.Elements {
		if s, ok := el.(*String); ok && strings.TrimSpace(s.Value) != "" {
			out = append(out, strings.TrimSpace(s.Value))
		}
	}
	return out
}

type Object interface {
	Type() ObjectType
	Inspect() string
}

type BuiltinFunction func(args ...Object) Object
type BuiltinFunctionWithEnv func(env *Environment, args ...Object) Object

type Builtin struct {
	Fn        BuiltinFunction
	FnWithEnv BuiltinFunctionWithEnv
	Env       *Environment
}

func (b *Builtin) Type() ObjectType { return BUILTIN_OBJ }
func (b *Builtin) Inspect() string  { return "builtin function" }

func (b *Builtin) bindEnv(env *Environment) *Builtin {
	if b == nil {
		return nil
	}
	if b.FnWithEnv == nil {
		return b
	}
	cloned := *b
	cloned.Env = env
	return &cloned
}

func runtimeContext(env *Environment) context.Context {
	if env != nil && env.runtimeLimits != nil && env.runtimeLimits.Ctx != nil {
		return env.runtimeLimits.Ctx
	}
	return context.Background()
}

func runtimeContextWithTimeout(env *Environment, timeout time.Duration) (context.Context, context.CancelFunc) {
	base := runtimeContext(env)
	if timeout > 0 {
		return context.WithTimeout(base, timeout)
	}
	return context.WithCancel(base)
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

func objectToNative(obj Object) interface{} {
	switch v := obj.(type) {
	case *Null:
		return nil
	case *Boolean:
		return v.Value
	case *Integer:
		return v.Value
	case *Float:
		return v.Value
	case *String:
		return v.Value
	case *Secret:
		return v.Value
	case *Array:
		out := make([]interface{}, len(v.Elements))
		for i, el := range v.Elements {
			out[i] = objectToNative(el)
		}
		return out
	case *Hash:
		out := make(map[string]interface{}, len(v.Pairs))
		for _, pair := range v.Pairs {
			out[pair.Key.Inspect()] = objectToNative(pair.Value)
		}
		return out
	default:
		return v.Inspect()
	}
}

func objectToDisplayString(obj Object) string {
	if obj == nil {
		return "null"
	}
	if s, ok := secretValue(obj); ok {
		_ = s
		return "***"
	}
	if s, ok := obj.(*String); ok {
		return s.Value
	}
	return obj.Inspect()
}

func objectToFmtValue(obj Object) interface{} {
	if obj == nil {
		return nil
	}
	switch v := obj.(type) {
	case *Null:
		return nil
	case *Boolean:
		return v.Value
	case *Integer:
		return v.Value
	case *Float:
		return v.Value
	case *String:
		return v.Value
	default:
		return objectToNative(obj)
	}
}

func splSprintf(format string, args []Object) (string, Object) {
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
			return "", &String{Value: "ERROR: invalid format string: incomplete placeholder"}
		}

		verb := format[j]
		if argIndex >= len(args) {
			return "", &String{Value: fmt.Sprintf("ERROR: not enough format arguments for placeholder %%%c", verb)}
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
		default:
			out.WriteString(fmt.Sprintf(spec, objectToFmtValue(arg)))
		}

		i = j
	}

	if argIndex < len(args) {
		return "", &String{Value: fmt.Sprintf("ERROR: too many format arguments: used=%d, total=%d", argIndex, len(args))}
	}

	return out.String(), nil
}

func interpolateTemplate(template string, data Object, positional []Object) (string, Object) {
	var out strings.Builder

	resolve := func(key string) (Object, bool) {
		if data != nil {
			switch d := data.(type) {
			case *Hash:
				hk := (&String{Value: key}).HashKey()
				if pair, ok := d.Pairs[hk]; ok {
					return pair.Value, true
				}
				return nil, false
			case *Array:
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
				return "", &String{Value: "ERROR: invalid interpolation template: missing }"}
			}
			end += i + 1
			key := strings.TrimSpace(template[i+1 : end])
			if key == "" {
				return "", &String{Value: "ERROR: invalid interpolation template: empty placeholder"}
			}
			val, ok := resolve(key)
			if !ok {
				return "", &String{Value: fmt.Sprintf("ERROR: missing interpolation value for {%s}", key)}
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

func asString(arg Object, name string) (string, Object) {
	if s, ok := secretValue(arg); ok {
		return s, nil
	}
	if arg.Type() != STRING_OBJ {
		return "", newError("argument `%s` must be STRING, got %s", name, arg.Type())
	}
	return arg.(*String).Value, nil
}

func asInt(arg Object, name string) (int64, Object) {
	if arg.Type() != INTEGER_OBJ {
		return 0, newError("argument `%s` must be INTEGER, got %s", name, arg.Type())
	}
	return arg.(*Integer).Value, nil
}

func toStringSlice(arr *Array) ([]string, Object) {
	out := make([]string, len(arr.Elements))
	for i, el := range arr.Elements {
		if el.Type() != STRING_OBJ {
			return nil, newError("array element at index %d must be STRING, got %s", i, el.Type())
		}
		out[i] = el.(*String).Value
	}
	return out, nil
}

func toIntObject(arg Object) Object {
	switch v := arg.(type) {
	case *Integer:
		return v
	case *Float:
		return &Integer{Value: int64(v.Value)}
	case *String:
		val, err := strconv.ParseInt(v.Value, 10, 64)
		if err != nil {
			return newError("could not convert %q to int", v.Value)
		}
		return &Integer{Value: val}
	case *Boolean:
		if v.Value {
			return &Integer{Value: 1}
		}
		return &Integer{Value: 0}
	default:
		return newError("cannot convert %s to int", arg.Type())
	}
}

func toFloatObject(arg Object) Object {
	switch v := arg.(type) {
	case *Float:
		return v
	case *Integer:
		return &Float{Value: float64(v.Value)}
	case *String:
		val, err := strconv.ParseFloat(v.Value, 64)
		if err != nil {
			return newError("could not convert %q to float", v.Value)
		}
		return &Float{Value: val}
	case *Boolean:
		if v.Value {
			return &Float{Value: 1}
		}
		return &Float{Value: 0}
	default:
		return newError("cannot convert %s to float", arg.Type())
	}
}

func toBoolObject(arg Object) Object {
	switch v := arg.(type) {
	case *Boolean:
		return v
	case *Integer:
		return nativeBoolToBooleanObject(v.Value != 0)
	case *Float:
		return nativeBoolToBooleanObject(v.Value != 0)
	case *String:
		s := strings.TrimSpace(strings.ToLower(v.Value))
		switch s {
		case "true", "1", "yes", "y", "on":
			return TRUE
		case "false", "0", "no", "n", "off", "":
			return FALSE
		default:
			return newError("could not parse %q as bool", v.Value)
		}
	default:
		return newError("cannot convert %s to bool", arg.Type())
	}
}

func parseJSONToObject(input string) (Object, error) {
	var v interface{}
	if err := json.Unmarshal([]byte(input), &v); err != nil {
		return nil, err
	}
	return toObject(v), nil
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
		b[0:4],
		b[4:6],
		b[6:8],
		b[8:10],
		b[10:16]), nil
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

	// Set version (7) and variant (RFC 4122 / RFC 9562)
	b[6] = (b[6] & 0x0f) | 0x70
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4],
		b[4:6],
		b[6:8],
		b[8:10],
		b[10:16]), nil
}

var builtinHelpDescriptions = map[string]string{
	"help":            "help() lists builtin names; help(\"name\") shows details for one builtin",
	"sprintf":         "sprintf(format, ...args) formats values with printf-style placeholders; supports %T for SPL type",
	"printf":          "printf(format, ...args) prints formatted text and returns it",
	"interpolate":     "interpolate(template, data[, ...positional]) replaces {key} or {index} placeholders",
	"uuid":            "uuid([version]) generates UUID, default version is 7; supports 4 or 7",
	"http_request":    "http_request(method, url[, body][, headers][, timeout_ms]) performs an HTTP request",
	"http_get":        "http_get(url[, headers][, timeout_ms]) performs HTTP GET",
	"http_post":       "http_post(url, body[, headers][, timeout_ms]) performs HTTP POST",
	"webhook":         "webhook(url, payload[, headers][, timeout_ms]) sends a webhook POST",
	"db_connect":      "db_connect(driver, connection_string) opens a database connection",
	"db_query":        "db_query(db_or_tx, query[, params][, format]) runs a query; params may be ARRAY or HASH; format is table or array",
	"db_exec":         "db_exec(db_or_tx, query[, params]) executes a statement; params may be ARRAY or HASH",
	"db_begin":        "db_begin(db) starts a database transaction",
	"db_commit":       "db_commit(tx) commits a database transaction",
	"db_rollback":     "db_rollback(tx) rolls back a database transaction",
	"db_tables":       "db_tables(db_or_tx) lists database tables",
	"db_close":        "db_close(db) closes a database connection",
	"smtp_send":       "smtp_send(config) sends email via SMTP",
	"ftp_list":        "ftp_list(config, remote_dir) lists directory entries over FTP",
	"ftp_get":         "ftp_get(config, remote_path, local_path) downloads file over FTP",
	"ftp_put":         "ftp_put(config, local_path, remote_path) uploads file over FTP",
	"sftp_list":       "sftp_list(config, remote_dir) lists directory entries over SFTP",
	"sftp_get":        "sftp_get(config, remote_path, local_path) downloads file over SFTP",
	"sftp_put":        "sftp_put(config, local_path, remote_path) uploads file over SFTP",
	"assert_true":     "assert_true(condition[, message]) fails test when condition is false",
	"assert_eq":       "assert_eq(actual, expected[, message]) fails test when values differ",
	"assert_neq":      "assert_neq(actual, unexpected[, message]) fails test when values are equal",
	"assert_contains": "assert_contains(haystack, needle[, message]) fails test when needle not found in haystack string or array",
	"assert_throws":   "assert_throws(fn[, message]) fails test when fn does not produce an error",
	"test_summary":    "test_summary() returns {total, passed, failed}",
	"run_tests":       "run_tests(path_or_paths) executes SPL test scripts and returns summary",
	"exec":            "exec(command, ...args[, timeout_ms]) runs a whitelisted OS command; disabled by SPL_DISABLE_EXEC or host protection",
	"config_load":     "config_load(path[, format]) loads JSON/YAML/.env config and wraps secret-like fields",
	"config_parse":    "config_parse(raw, format) parses JSON/YAML/.env string and wraps secret-like fields",
	"secret":          "secret(value) wraps a string as non-displayable secret",
	"secret_reveal":   "secret_reveal(secret_value) reveals a SECRET as plain STRING",
	"secret_mask":     "secret_mask(value[, visible]) returns masked display string",
	"Error":           "Error(message[, details]) returns structured error object with message, code, stack",
	"channel":         "channel([buffer_size]) creates a message channel",
	"send":            "send(channel, value) sends a value to channel",
	"recv":            "recv(channel) receives a value from channel",
	"go":              "go(fn[, ...args]) runs function asynchronously and returns future",
	"generator":       "generator(fn) wraps function result as lazy iterable",
	"permissions":     "permissions(policy_hash) applies runtime allow/deny policy",
	"metric":          "metric(name, value[, labels]) records metric point",
	"trace":           "trace(name[, attrs]) emits trace event",
	"immutable":       "immutable(value) returns deeply frozen copy",
	"move":            "move(value) transfers ownership marker to current scope",
}

var testStats = struct {
	mu     sync.Mutex
	total  int64
	passed int64
	failed int64
}{}

func resetTestStats() {
	testStats.mu.Lock()
	defer testStats.mu.Unlock()
	testStats.total = 0
	testStats.passed = 0
	testStats.failed = 0
}

func testSummaryObject() Object {
	pairs := make(map[HashKey]HashPair, 3)
	keys := []string{"total", "passed", "failed"}
	testStats.mu.Lock()
	total, passed, failed := testStats.total, testStats.passed, testStats.failed
	testStats.mu.Unlock()
	vals := []int64{total, passed, failed}
	for i, k := range keys {
		key := &String{Value: k}
		pairs[key.HashKey()] = HashPair{Key: key, Value: &Integer{Value: vals[i]}}
	}
	return &Hash{Pairs: pairs}
}

func assertFail(msg string) Object {
	testStats.mu.Lock()
	testStats.total++
	testStats.failed++
	testStats.mu.Unlock()
	if msg == "" {
		msg = "assertion failed"
	}
	return &String{Value: "ERROR: " + msg}
}

func assertPass() Object {
	testStats.mu.Lock()
	testStats.total++
	testStats.passed++
	testStats.mu.Unlock()
	return TRUE
}

func isTruthyEnvVar(name string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	switch v {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func parsePositiveDurationMs(value string, envName string) (time.Duration, Object) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	ms, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, &String{Value: fmt.Sprintf("ERROR: invalid %s value %q: must be integer milliseconds", envName, value)}
	}
	if ms <= 0 {
		return 0, &String{Value: fmt.Sprintf("ERROR: invalid %s value %q: must be > 0", envName, value)}
	}
	return time.Duration(ms) * time.Millisecond, nil
}

func execTimeoutFromArgsAndEnv(timeoutArgMs int64) (time.Duration, Object) {
	if timeoutArgMs > 0 {
		return time.Duration(timeoutArgMs) * time.Millisecond, nil
	}
	if timeoutArgMs < 0 {
		return 0, &String{Value: "ERROR: exec timeout must be > 0 milliseconds"}
	}
	return parsePositiveDurationMs(os.Getenv("SPL_EXEC_TIMEOUT_MS"), "SPL_EXEC_TIMEOUT_MS")
}

func builtinHelpText(name string) string {
	if details, ok := builtinHelpDescriptions[name]; ok {
		return details
	}
	return fmt.Sprintf("%s(...) builtin function", name)
}

func builtinNames() []string {
	names := make([]string, 0, len(builtins))
	for name := range builtins {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func hasBuiltin(name string) bool {
	_, ok := builtins[name]
	return ok
}

func init() {
	builtins["help"] = &Builtin{
		Fn: func(args ...Object) Object {
			if len(args) > 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=0 or 1", len(args))}
			}

			if len(args) == 0 {
				names := builtinNames()

				elements := make([]Object, len(names))
				for i, name := range names {
					elements[i] = &String{Value: name}
				}
				return &Array{Elements: elements}
			}

			if args[0].Type() != STRING_OBJ {
				return &String{Value: fmt.Sprintf("argument to `help` must be STRING, got %s", args[0].Type())}
			}

			name := args[0].(*String).Value
			if !hasBuiltin(name) {
				return &String{Value: fmt.Sprintf("ERROR: builtin %q not found", name)}
			}
			return &String{Value: builtinHelpText(name)}
		},
	}

	builtins["assert_true"] = &Builtin{
		Fn: func(args ...Object) Object {
			if len(args) < 1 || len(args) > 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1 or 2", len(args))}
			}
			if isTruthy(args[0]) {
				return assertPass()
			}
			if len(args) == 2 {
				msg, errObj := asString(args[1], "message")
				if errObj != nil {
					return errObj
				}
				return assertFail(msg)
			}
			return assertFail("assert_true failed")
		},
	}

	builtins["assert_eq"] = &Builtin{
		Fn: func(args ...Object) Object {
			if len(args) < 2 || len(args) > 3 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2 or 3", len(args))}
			}
			if args[0].Inspect() == args[1].Inspect() {
				return assertPass()
			}
			msg := fmt.Sprintf("assert_eq failed: got=%s expected=%s", args[0].Inspect(), args[1].Inspect())
			if len(args) == 3 {
				custom, errObj := asString(args[2], "message")
				if errObj != nil {
					return errObj
				}
				msg = custom
			}
			return assertFail(msg)
		},
	}

	builtins["assert_neq"] = &Builtin{
		Fn: func(args ...Object) Object {
			if len(args) < 2 || len(args) > 3 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2 or 3", len(args))}
			}
			if args[0].Inspect() != args[1].Inspect() {
				return assertPass()
			}
			msg := fmt.Sprintf("assert_neq failed: both values are %s", args[0].Inspect())
			if len(args) == 3 {
				custom, errObj := asString(args[2], "message")
				if errObj != nil {
					return errObj
				}
				msg = custom
			}
			return assertFail(msg)
		},
	}

	builtins["assert_contains"] = &Builtin{
		Fn: func(args ...Object) Object {
			if len(args) < 2 || len(args) > 3 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2 or 3", len(args))}
			}
			found := false
			switch haystack := args[0].(type) {
			case *String:
				needle, ok := args[1].(*String)
				if !ok {
					return &String{Value: "assert_contains: needle must be a string when haystack is a string"}
				}
				found = strings.Contains(haystack.Value, needle.Value)
			case *Array:
				for _, el := range haystack.Elements {
					if el.Inspect() == args[1].Inspect() {
						found = true
						break
					}
				}
			default:
				return &String{Value: fmt.Sprintf("assert_contains: first argument must be STRING or ARRAY, got %s", args[0].Type())}
			}
			if found {
				return assertPass()
			}
			msg := fmt.Sprintf("assert_contains failed: %s does not contain %s", args[0].Inspect(), args[1].Inspect())
			if len(args) == 3 {
				custom, errObj := asString(args[2], "message")
				if errObj != nil {
					return errObj
				}
				msg = custom
			}
			return assertFail(msg)
		},
	}

	builtins["assert_throws"] = &Builtin{
		Fn: func(args ...Object) Object {
			if len(args) < 1 || len(args) > 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1 or 2", len(args))}
			}
			res := executeCallback(args[0], []Object{})
			if isError(res) {
				return assertPass()
			}
			msg := "assert_throws failed: expected an error but none was thrown"
			if len(args) == 2 {
				custom, errObj := asString(args[1], "message")
				if errObj != nil {
					return errObj
				}
				msg = custom
			}
			return assertFail(msg)
		},
	}

	builtins["test_summary"] = &Builtin{
		Fn: func(args ...Object) Object {
			if len(args) != 0 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=0", len(args))}
			}
			return testSummaryObject()
		},
	}

	builtins["run_tests"] = &Builtin{
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			resetTestStats()

			paths := []string{}
			switch args[0].Type() {
			case STRING_OBJ:
				paths = append(paths, args[0].(*String).Value)
			case ARRAY_OBJ:
				for _, el := range args[0].(*Array).Elements {
					if el.Type() != STRING_OBJ {
						return &String{Value: "ERROR: run_tests array elements must be STRING"}
					}
					paths = append(paths, el.(*String).Value)
				}
			default:
				return &String{Value: fmt.Sprintf("argument to `run_tests` must be STRING or ARRAY, got %s", args[0].Type())}
			}

			for _, p := range paths {
				safePath, err := sanitizePath(p)
				if err != nil {
					return &String{Value: fmt.Sprintf("ERROR: invalid test path %q: %s", p, err)}
				}
				if err := checkFileReadAllowed(safePath); err != nil {
					return &String{Value: fmt.Sprintf("ERROR: denied test path %q: %s", p, err)}
				}
				content, err := os.ReadFile(safePath)
				if err != nil {
					return &String{Value: fmt.Sprintf("ERROR: failed to read test file %q: %s", p, err)}
				}
				env := NewGlobalEnvironment([]string{})
				env.moduleDir = filepath.Dir(safePath)
				l := NewLexer(string(content))
				parser := NewParser(l)
				program := parser.ParseProgram()
				if len(parser.Errors()) > 0 {
					return &String{Value: fmt.Sprintf("ERROR: parser errors in %q: %s", p, strings.Join(parser.Errors(), "; "))}
				}
				result := Eval(program, env)
				if isError(result) {
					return &String{Value: fmt.Sprintf("ERROR: test execution failed in %q: %s", p, objectErrorString(result))}
				}
			}

			return testSummaryObject()
		},
	}

	builtins["sprintf"] = &Builtin{
		Fn: func(args ...Object) Object {
			if len(args) < 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=at least 1", len(args))}
			}
			format, errObj := asString(args[0], "format")
			if errObj != nil {
				return errObj
			}
			out, ferr := splSprintf(format, args[1:])
			if ferr != nil {
				return ferr
			}
			return &String{Value: out}
		},
	}

	builtins["printf"] = &Builtin{
		Fn: func(args ...Object) Object {
			if len(args) < 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=at least 1", len(args))}
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
			return &String{Value: out}
		},
	}

	builtins["interpolate"] = &Builtin{
		Fn: func(args ...Object) Object {
			if len(args) < 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=at least 2", len(args))}
			}
			template, errObj := asString(args[0], "template")
			if errObj != nil {
				return errObj
			}
			data := args[1]
			if data.Type() != HASH_OBJ && data.Type() != ARRAY_OBJ && data.Type() != NULL_OBJ {
				return &String{Value: fmt.Sprintf("argument `data` must be HASH, ARRAY, or NULL, got %s", data.Type())}
			}
			out, ferr := interpolateTemplate(template, data, args[2:])
			if ferr != nil {
				return ferr
			}
			return &String{Value: out}
		},
	}

	builtins["Error"] = &Builtin{
		Fn: func(args ...Object) Object {
			if len(args) < 1 || len(args) > 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1 or 2", len(args))}
			}
			msg := objectToDisplayString(args[0])
			pairs := map[HashKey]HashPair{}
			put := func(k string, v Object) {
				key := &String{Value: k}
				pairs[key.HashKey()] = HashPair{Key: key, Value: v}
			}
			put("name", &String{Value: "Error"})
			put("message", &String{Value: msg})
			put("code", &String{Value: "E_RUNTIME"})
			put("stack", &String{Value: ""})
			if len(args) == 2 {
				if details, ok := args[1].(*Hash); ok {
					for _, field := range []string{"code", "stack", "path", "line", "column"} {
						if p, ok := details.Pairs[(&String{Value: field}).HashKey()]; ok {
							put(field, p.Value)
						}
					}
				}
			}
			return &Hash{Pairs: pairs}
		},
	}

	builtins["channel"] = &Builtin{
		Fn: func(args ...Object) Object {
			buffer := 0
			if len(args) > 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=0 or 1", len(args))}
			}
			if len(args) == 1 {
				i, ok := args[0].(*Integer)
				if !ok || i.Value < 0 {
					return &String{Value: "channel buffer must be non-negative integer"}
				}
				buffer = int(i.Value)
			}
			return &Channel{ch: make(chan Object, buffer)}
		},
	}

	builtins["send"] = &Builtin{
		Fn: func(args ...Object) Object {
			if len(args) != 2 {
				return &String{Value: "send expects 2 arguments"}
			}
			ch, ok := args[0].(*Channel)
			if !ok {
				return &String{Value: "send first argument must be channel"}
			}
			ch.ch <- args[1]
			return NULL
		},
	}

	builtins["recv"] = &Builtin{
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: "recv expects 1 argument"}
			}
			ch, ok := args[0].(*Channel)
			if !ok {
				return &String{Value: "recv argument must be channel"}
			}
			return <-ch.ch
		},
	}

	builtins["go"] = &Builtin{
		FnWithEnv: func(env *Environment, args ...Object) Object {
			if len(args) < 1 {
				return &String{Value: "go expects function and optional args"}
			}
			fn := args[0]
			callArgs := []Object{}
			if len(args) > 1 {
				callArgs = args[1:]
			}
			ch := make(chan Object, 1)
			go func() {
				ch <- applyFn(fn, callArgs, env, nil)
			}()
			return &Future{ch: ch}
		},
	}

	builtins["generator"] = &Builtin{
		FnWithEnv: func(env *Environment, args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: "generator expects 1 function"}
			}
			res := applyFn(args[0], []Object{}, env, nil)
			if isError(res) {
				return res
			}
			if arr, ok := res.(*Array); ok {
				return &GeneratorValue{elements: arr.Elements}
			}
			return &GeneratorValue{elements: []Object{res}}
		},
	}

	builtins["permissions"] = &Builtin{
		FnWithEnv: func(env *Environment, args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: "permissions expects 1 hash argument"}
			}
			h, ok := args[0].(*Hash)
			if !ok {
				return &String{Value: "permissions argument must be HASH"}
			}
			policy := &SecurityPolicy{
				StrictMode:          hashBoolValue(h, "strict", false),
				ProtectHost:         hashBoolValue(h, "protect_host", false),
				AllowEnvWrite:       hashBoolValue(h, "allow_env_write", true),
				AllowedExecCommands: hashStringArray(h, "allow_exec"),
				DeniedExecCommands:  hashStringArray(h, "deny_exec"),
				AllowedNetworkHosts: hashStringArray(h, "allow_http"),
				DeniedNetworkHosts:  hashStringArray(h, "deny_http"),
			}
			if env != nil {
				env.securityPolicy = policy
			}
			securityPolicyOverride.mu.Lock()
			securityPolicyOverride.policy = policy
			securityPolicyOverride.mu.Unlock()
			return TRUE
		},
	}

	builtins["metric"] = &Builtin{
		Fn: func(args ...Object) Object {
			if len(args) < 2 {
				return &String{Value: "metric(name, value[, labels]) expects at least 2 arguments"}
			}
			name, ok := args[0].(*String)
			if !ok {
				return &String{Value: "metric name must be STRING"}
			}
			val := 0.0
			switch n := args[1].(type) {
			case *Integer:
				val = float64(n.Value)
			case *Float:
				val = n.Value
			default:
				return &String{Value: "metric value must be number"}
			}
			telemetryState.mu.Lock()
			telemetryState.metrics[name.Value] += val
			telemetryState.mu.Unlock()
			return TRUE
		},
	}

	builtins["trace"] = &Builtin{
		Fn: func(args ...Object) Object {
			if len(args) < 1 {
				return &String{Value: "trace(name[, attrs]) expects at least 1 argument"}
			}
			name, ok := args[0].(*String)
			if !ok {
				return &String{Value: "trace name must be STRING"}
			}
			entry := name.Value
			if len(args) > 1 {
				entry = entry + " " + args[1].Inspect()
			}
			telemetryState.mu.Lock()
			telemetryState.traces = append(telemetryState.traces, entry)
			telemetryState.mu.Unlock()
			return TRUE
		},
	}

	builtins["immutable"] = &Builtin{
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: "immutable expects exactly 1 argument"}
			}
			return deepImmutableClone(args[0])
		},
	}

	builtins["move"] = &Builtin{
		FnWithEnv: func(env *Environment, args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: "move expects exactly 1 argument"}
			}
			if env == nil {
				return args[0]
			}
			return maybeWrapOwned(args[0], env)
		},
	}
}

var builtins = map[string]*Builtin{
	"len": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}

			switch arg := args[0].(type) {
			case *String:
				return &Integer{Value: int64(len(arg.Value))}
			case *Array:
				return &Integer{Value: int64(len(arg.Elements))}
			case *Hash:
				return &Integer{Value: int64(len(arg.Pairs))}
			default:
				return &String{Value: fmt.Sprintf("argument to `len` not supported, got %s", args[0].Type())}
			}
		},
	},
	"keys": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			if args[0].Type() != HASH_OBJ {
				return &String{Value: fmt.Sprintf("argument to `keys` must be HASH, got %s", args[0].Type())}
			}

			hash := args[0].(*Hash)
			elements := []Object{}
			for _, pair := range hash.Pairs {
				elements = append(elements, pair.Key)
			}
			return &Array{Elements: elements}
		},
	},
	"type": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			return &String{Value: args[0].Type().String()}
		},
	},
	"puts": {
		Fn: func(args ...Object) Object {
			for _, arg := range args {
				fmt.Println(arg.Inspect())
			}
			return NULL
		},
	},
	"upper": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			if args[0].Type() != STRING_OBJ {
				return &String{Value: fmt.Sprintf("argument to `upper` must be STRING, got %s", args[0].Type())}
			}
			return &String{Value: strings.ToUpper(args[0].(*String).Value)}
		},
	},
	"lower": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			if args[0].Type() != STRING_OBJ {
				return &String{Value: fmt.Sprintf("argument to `lower` must be STRING, got %s", args[0].Type())}
			}
			return &String{Value: strings.ToLower(args[0].(*String).Value)}
		},
	},
	"split": {
		Fn: func(args ...Object) Object {
			if len(args) != 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
			}
			if args[0].Type() != STRING_OBJ || args[1].Type() != STRING_OBJ {
				return &String{Value: fmt.Sprintf("arguments to `split` must be STRING, got %s and %s", args[0].Type(), args[1].Type())}
			}
			parts := strings.Split(args[0].(*String).Value, args[1].(*String).Value)
			elements := make([]Object, len(parts))
			for i, part := range parts {
				elements[i] = &String{Value: part}
			}
			return &Array{Elements: elements}
		},
	},
	"join": {
		Fn: func(args ...Object) Object {
			if len(args) != 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
			}
			if args[0].Type() != ARRAY_OBJ {
				return &String{Value: fmt.Sprintf("first argument to `join` must be ARRAY, got %s", args[0].Type())}
			}
			if args[1].Type() != STRING_OBJ {
				return &String{Value: fmt.Sprintf("second argument to `join` must be STRING, got %s", args[1].Type())}
			}

			arr := args[0].(*Array)
			sep := args[1].(*String).Value

			parts := make([]string, len(arr.Elements))
			for i, el := range arr.Elements {
				parts[i] = el.Inspect()
				if el.Type() == STRING_OBJ {
					parts[i] = el.(*String).Value
				}
			}

			return &String{Value: strings.Join(parts, sep)}
		},
	},
	"read_file": {
		Fn: func(args ...Object) Object {
			// Helper to return error tuple: [NULL, "ERROR msg"]
			retErr := func(msg string) Object {
				// Use NULL_OBJ for value (which is &Null{})
				return &Array{Elements: []Object{&Null{}, &String{Value: msg}}}
			}
			// Helper to return success tuple: [String, NULL]
			retOk := func(val string) Object {
				return &Array{Elements: []Object{&String{Value: val}, &Null{}}}
			}

			if len(args) != 1 {
				return retErr(fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args)))
			}
			if args[0].Type() != STRING_OBJ {
				return retErr(fmt.Sprintf("argument to `read_file` must be STRING, got %s", args[0].Type()))
			}

			path := args[0].(*String).Value
			safePath, err := sanitizePath(path)
			if err != nil {
				return retErr(fmt.Sprintf("%s", err))
			}
			if err := checkFileReadAllowed(safePath); err != nil {
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
		Fn: func(args ...Object) Object {
			// Returns [Result(bool), Error(string/null)]
			retErr := func(msg string) Object {
				return &Array{Elements: []Object{FALSE, &String{Value: msg}}}
			}
			retOk := func() Object {
				return &Array{Elements: []Object{TRUE, &Null{}}}
			}

			if len(args) != 2 {
				return retErr(fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args)))
			}
			if args[0].Type() != STRING_OBJ || args[1].Type() != STRING_OBJ {
				return retErr(fmt.Sprintf("arguments to `write_file` must be STRING, got %s and %s", args[0].Type(), args[1].Type()))
			}

			path := args[0].(*String).Value
			safePath, err := sanitizePath(path)
			if err != nil {
				return retErr(fmt.Sprintf("%s", err))
			}
			if err := checkFileWriteAllowed(safePath); err != nil {
				return retErr(fmt.Sprintf("%s", err))
			}

			content := args[1].(*String).Value
			if err := os.MkdirAll(filepath.Dir(safePath), 0o755); err != nil {
				return retErr(fmt.Sprintf("%s", err))
			}
			err = os.WriteFile(safePath, []byte(content), 0644)
			if err != nil {
				return retErr(fmt.Sprintf("%s", err))
			}
			return retOk() // success
		},
	},
	"file_exists": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			if args[0].Type() != STRING_OBJ {
				return &String{Value: fmt.Sprintf("argument to `file_exists` must be STRING, got %s", args[0].Type())}
			}

			path := args[0].(*String).Value
			safePath, err := sanitizePath(path)
			if err != nil {
				return &String{Value: fmt.Sprintf("IO ERROR: %s", err)}
			}
			if err := checkFileReadAllowed(safePath); err != nil {
				return &String{Value: fmt.Sprintf("IO ERROR: %s", err)}
			}

			_, err = os.Stat(safePath)
			return nativeBoolToBooleanObject(!os.IsNotExist(err))
		},
	},
	"remove_file": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			if args[0].Type() != STRING_OBJ {
				return &String{Value: fmt.Sprintf("argument to `remove_file` must be STRING, got %s", args[0].Type())}
			}

			path := args[0].(*String).Value
			safePath, err := sanitizePath(path)
			if err != nil {
				return &String{Value: fmt.Sprintf("IO ERROR: %s", err)}
			}
			if err := checkFileWriteAllowed(safePath); err != nil {
				return &String{Value: fmt.Sprintf("IO ERROR: %s", err)}
			}

			err = os.Remove(safePath)
			if err != nil {
				return &String{Value: fmt.Sprintf("IO ERROR: %s", err)}
			}
			return TRUE
		},
	},
	"os_env": {
		Fn: func(args ...Object) Object {
			var key, val string
			if len(args) == 1 {
				if args[0].Type() != STRING_OBJ {
					return &String{Value: fmt.Sprintf("argument to `os_env` must be STRING, got %s", args[0].Type())}
				}
				key = args[0].(*String).Value
				return &String{Value: os.Getenv(key)}
			} else if len(args) == 2 {
				if args[0].Type() != STRING_OBJ || args[1].Type() != STRING_OBJ {
					return &String{Value: fmt.Sprintf("arguments to `os_env` must be STRING, got %s and %s", args[0].Type(), args[1].Type())}
				}
				key = args[0].(*String).Value
				val = args[1].(*String).Value
				if err := envWriteAllowed(key); err != nil {
					return &String{Value: fmt.Sprintf("ERROR: %s", err)}
				}
				os.Setenv(key, val)
				return NULL
			} else {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1 or 2", len(args))}
			}
		},
	},
	"exit": {
		Fn: func(args ...Object) Object {
			if err := exitAllowed(); err != nil {
				return &String{Value: fmt.Sprintf("ERROR: %s", err)}
			}
			code := 0
			if len(args) == 1 {
				if args[0].Type() != INTEGER_OBJ {
					return &String{Value: fmt.Sprintf("argument to `exit` must be INTEGER, got %s", args[0].Type())}
				}
				code = int(args[0].(*Integer).Value)
			}
			os.Exit(code)
			return NULL
		},
	},
	"exec": {
		FnWithEnv: func(env *Environment, args ...Object) Object {
			if isTruthyEnvVar("SPL_DISABLE_EXEC") {
				return &String{Value: "ERROR: exec is disabled by SPL_DISABLE_EXEC"}
			}

			if len(args) < 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=at least 1", len(args))}
			}
			if args[0].Type() != STRING_OBJ {
				return &String{Value: fmt.Sprintf("command must be STRING, got %s", args[0].Type())}
			}

			cmdName := args[0].(*String).Value
			if err := checkExecAllowed(cmdName); err != nil {
				return &String{Value: fmt.Sprintf("ERROR: %s", err)}
			}

			timeoutArgMs := int64(0)
			argEnd := len(args)
			if len(args) > 1 && args[len(args)-1].Type() == INTEGER_OBJ {
				timeoutArgMs = args[len(args)-1].(*Integer).Value
				argEnd = len(args) - 1
			}

			timeoutDur, timeoutErr := execTimeoutFromArgsAndEnv(timeoutArgMs)
			if timeoutErr != nil {
				return timeoutErr
			}

			cmdArgs := []string{}

			for i := 1; i < argEnd; i++ {
				if args[i].Type() != STRING_OBJ {
					return &String{Value: fmt.Sprintf("exec argument %d must be STRING, got %s", i, args[i].Type())}
				}
				cmdArgs = append(cmdArgs, args[i].(*String).Value)
			}

			var (
				cmd *exec.Cmd
				ctx context.Context
			)
			ctx, cancel := runtimeContextWithTimeout(env, timeoutDur)
			defer cancel()
			cmd = exec.CommandContext(ctx, cmdName, cmdArgs...)

			output, err := cmd.CombinedOutput()
			if err != nil {
				if errors.Is(ctx.Err(), context.DeadlineExceeded) {
					return &String{Value: fmt.Sprintf("ERROR: command timed out after %dms\n%s", timeoutDur/time.Millisecond, string(output))}
				}
				if errors.Is(ctx.Err(), context.Canceled) {
					return &String{Value: fmt.Sprintf("ERROR: command cancelled\n%s", string(output))}
				}
				return &String{Value: fmt.Sprintf("ERROR: %s\n%s", err, string(output))}
			}
			return &String{Value: string(output)}
		},
	},
	"time": {
		Fn: func(args ...Object) Object {
			return &Integer{Value: time.Now().Unix()}
		},
	},
	"sleep": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			if args[0].Type() != INTEGER_OBJ {
				return &String{Value: fmt.Sprintf("argument to `sleep` must be INTEGER (ms), got %s", args[0].Type())}
			}
			ms := args[0].(*Integer).Value
			time.Sleep(time.Duration(ms) * time.Millisecond)
			return NULL
		},
	},
	"to_int": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			return toIntObject(args[0])
		},
	},
	"to_float": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			return toFloatObject(args[0])
		},
	},
	"to_string": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			return &String{Value: args[0].Inspect()}
		},
	},
	"parse_string": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			return &String{Value: args[0].Inspect()}
		},
	},
	"parse_bool": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			return toBoolObject(args[0])
		},
	},
	"input": {
		Fn: func(args ...Object) Object {
			if len(args) > 0 {
				fmt.Print(args[0].Inspect())
			}
			reader := bufio.NewReader(os.Stdin)
			text, _ := reader.ReadString('\n')
			return &String{Value: strings.TrimSuffix(text, "\n")}
		},
	},
	"random": {
		Fn: func(args ...Object) Object {
			max := int64(math.MaxInt64)
			if len(args) > 0 {
				if args[0].Type() != INTEGER_OBJ {
					return &String{Value: fmt.Sprintf("argument to `random` must be INTEGER, got %s", args[0].Type())}
				}
				max = args[0].(*Integer).Value
			}
			if max <= 0 {
				return newError("random max must be > 0")
			}
			return &Integer{Value: mrand.Int63n(max)}
		},
	},
	"seed_random": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			seed, errObj := asInt(args[0], "seed")
			if errObj != nil {
				return errObj
			}
			mrand.Seed(seed)
			return NULL
		},
	},
	"abs": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			if args[0].Type() != INTEGER_OBJ {
				return &String{Value: fmt.Sprintf("argument to `abs` must be INTEGER, got %s", args[0].Type())}
			}
			val := args[0].(*Integer).Value
			if val < 0 {
				return &Integer{Value: -val}
			}
			return &Integer{Value: val}
		},
	},
	"pow": {
		Fn: func(args ...Object) Object {
			if len(args) != 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
			}
			if args[0].Type() != INTEGER_OBJ || args[1].Type() != INTEGER_OBJ {
				return &String{Value: fmt.Sprintf("arguments to `pow` must be INTEGER, got %s and %s", args[0].Type(), args[1].Type())}
			}
			base := float64(args[0].(*Integer).Value)
			exp := float64(args[1].(*Integer).Value)
			return &Integer{Value: int64(math.Pow(base, exp))}
		},
	},
	"sqrt": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			if args[0].Type() != INTEGER_OBJ {
				return &String{Value: fmt.Sprintf("argument to `sqrt` must be INTEGER, got %s", args[0].Type())}
			}
			val := float64(args[0].(*Integer).Value)
			return &Integer{Value: int64(math.Sqrt(val))}
		},
	},
	"min": {
		Fn: func(args ...Object) Object {
			if len(args) < 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=at least 1", len(args))}
			}
			minVal := args[0].(*Integer).Value
			for _, arg := range args {
				if arg.Type() != INTEGER_OBJ {
					return &String{Value: fmt.Sprintf("arguments to `min` must be INTEGER, got %s", arg.Type())}
				}
				val := arg.(*Integer).Value
				if val < minVal {
					minVal = val
				}
			}
			return &Integer{Value: minVal}
		},
	},
	"max": {
		Fn: func(args ...Object) Object {
			if len(args) < 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=at least 1", len(args))}
			}
			maxVal := args[0].(*Integer).Value
			for _, arg := range args {
				if arg.Type() != INTEGER_OBJ {
					return &String{Value: fmt.Sprintf("arguments to `max` must be INTEGER, got %s", arg.Type())}
				}
				val := arg.(*Integer).Value
				if val > maxVal {
					maxVal = val
				}
			}
			return &Integer{Value: maxVal}
		},
	},
	"push": {
		Fn: func(args ...Object) Object {
			if len(args) != 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
			}
			if args[0].Type() != ARRAY_OBJ {
				return &String{Value: fmt.Sprintf("argument to `push` must be ARRAY, got %s", args[0].Type())}
			}

			arr := args[0].(*Array)
			newElements := make([]Object, len(arr.Elements)+1)
			copy(newElements, arr.Elements)
			newElements[len(arr.Elements)] = args[1]

			return &Array{Elements: newElements}
		},
	},
	"random_bytes": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			n, errObj := asInt(args[0], "n")
			if errObj != nil {
				return errObj
			}
			b, err := randomBytes(int(n))
			if err != nil {
				return &String{Value: fmt.Sprintf("ERROR: %s", err)}
			}
			return &String{Value: base64.StdEncoding.EncodeToString(b)}
		},
	},
	"random_string": {
		Fn: func(args ...Object) Object {
			if len(args) != 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
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
				return &String{Value: fmt.Sprintf("ERROR: %s", err)}
			}
			return &String{Value: s}
		},
	},
	"uuid": {
		Fn: func(args ...Object) Object {
			if len(args) > 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=0 or 1", len(args))}
			}

			version := int64(7)
			if len(args) == 1 {
				if args[0].Type() == INTEGER_OBJ {
					version = args[0].(*Integer).Value
				} else if args[0].Type() == STRING_OBJ {
					parsed, perr := strconv.ParseInt(strings.TrimSpace(args[0].(*String).Value), 10, 64)
					if perr != nil {
						return &String{Value: fmt.Sprintf("ERROR: invalid uuid version %q", args[0].(*String).Value)}
					}
					version = parsed
				} else {
					return &String{Value: fmt.Sprintf("argument to `uuid` must be INTEGER or STRING, got %s", args[0].Type())}
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
				return &String{Value: fmt.Sprintf("ERROR: unsupported uuid version %d (supported: 4, 7)", version)}
			}

			if err != nil {
				return &String{Value: fmt.Sprintf("ERROR: %s", err)}
			}
			return &String{Value: id}
		},
	},
	"hash": {
		Fn: func(args ...Object) Object {
			if len(args) != 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
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
				return &String{Value: hex.EncodeToString(sum[:])}
			case "sha256":
				sum := sha256.Sum256([]byte(data))
				return &String{Value: hex.EncodeToString(sum[:])}
			case "sha512":
				sum := sha512.Sum512([]byte(data))
				return &String{Value: hex.EncodeToString(sum[:])}
			default:
				return &String{Value: fmt.Sprintf("ERROR: unsupported hash algorithm %q", algo)}
			}
		},
	},
	"hmac": {
		Fn: func(args ...Object) Object {
			if len(args) != 3 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=3", len(args))}
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
				return &String{Value: hex.EncodeToString(h.Sum(nil))}
			case "sha512":
				h := hmac.New(sha512.New, []byte(key))
				h.Write([]byte(data))
				return &String{Value: hex.EncodeToString(h.Sum(nil))}
			default:
				return &String{Value: fmt.Sprintf("ERROR: unsupported hmac algorithm %q", algo)}
			}
		},
	},
	"password_generate": {
		Fn: func(args ...Object) Object {
			if len(args) < 1 || len(args) > 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1 or 2", len(args))}
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
				return &String{Value: fmt.Sprintf("ERROR: %s", err)}
			}
			return &String{Value: pass}
		},
	},
	"api_key": {
		Fn: func(args ...Object) Object {
			if len(args) != 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
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
				return &String{Value: fmt.Sprintf("ERROR: %s", err)}
			}
			token := base64.RawURLEncoding.EncodeToString(b)
			return &String{Value: prefix + "_" + token}
		},
	},
	"password_hash": {
		Fn: func(args ...Object) Object {
			if len(args) < 1 || len(args) > 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1 or 2", len(args))}
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
				return &String{Value: fmt.Sprintf("ERROR: %s", err)}
			}
			return &String{Value: encoded}
		},
	},
	"password_verify": {
		Fn: func(args ...Object) Object {
			if len(args) != 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
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
				return &String{Value: fmt.Sprintf("ERROR: %s", err)}
			}
			return nativeBoolToBooleanObject(ok)
		},
	},
	"encrypt": {
		Fn: func(args ...Object) Object {
			if len(args) < 3 || len(args) > 4 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=3 or 4", len(args))}
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
					return &String{Value: fmt.Sprintf("ERROR: %s", err)}
				}
				return &String{Value: out}
			default:
				return &String{Value: fmt.Sprintf("ERROR: unsupported encrypt algorithm %q", alg)}
			}
		},
	},
	"decrypt": {
		Fn: func(args ...Object) Object {
			if len(args) < 3 || len(args) > 4 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=3 or 4", len(args))}
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
					return &String{Value: fmt.Sprintf("ERROR: %s", err)}
				}
				return &String{Value: out}
			default:
				return &String{Value: fmt.Sprintf("ERROR: unsupported decrypt algorithm %q", alg)}
			}
		},
	},
	"constant_time_eq": {
		Fn: func(args ...Object) Object {
			if len(args) != 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
			}
			a, errObj := asString(args[0], "a")
			if errObj != nil {
				return errObj
			}
			b, errObj := asString(args[1], "b")
			if errObj != nil {
				return errObj
			}
			return nativeBoolToBooleanObject(subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1)
		},
	},
	"base64_encode": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			s, errObj := asString(args[0], "data")
			if errObj != nil {
				return errObj
			}
			return &String{Value: base64.StdEncoding.EncodeToString([]byte(s))}
		},
	},
	"base64_decode": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			s, errObj := asString(args[0], "data")
			if errObj != nil {
				return errObj
			}
			out, err := base64.StdEncoding.DecodeString(s)
			if err != nil {
				return &String{Value: fmt.Sprintf("ERROR: %s", err)}
			}
			return &String{Value: string(out)}
		},
	},
	"hex_encode": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			s, errObj := asString(args[0], "data")
			if errObj != nil {
				return errObj
			}
			return &String{Value: hex.EncodeToString([]byte(s))}
		},
	},
	"hex_decode": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			s, errObj := asString(args[0], "data")
			if errObj != nil {
				return errObj
			}
			out, err := hex.DecodeString(s)
			if err != nil {
				return &String{Value: fmt.Sprintf("ERROR: %s", err)}
			}
			return &String{Value: string(out)}
		},
	},
	"url_encode": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			s, errObj := asString(args[0], "value")
			if errObj != nil {
				return errObj
			}
			return &String{Value: url.QueryEscape(s)}
		},
	},
	"url_decode": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			s, errObj := asString(args[0], "value")
			if errObj != nil {
				return errObj
			}
			out, err := url.QueryUnescape(s)
			if err != nil {
				return &String{Value: fmt.Sprintf("ERROR: %s", err)}
			}
			return &String{Value: out}
		},
	},
	"json_encode": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			raw := objectToNative(args[0])
			out, err := json.Marshal(raw)
			if err != nil {
				return &String{Value: fmt.Sprintf("ERROR: %s", err)}
			}
			return &String{Value: string(out)}
		},
	},
	"json_decode": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			s, errObj := asString(args[0], "json")
			if errObj != nil {
				return errObj
			}
			obj, err := parseJSONToObject(s)
			if err != nil {
				return &String{Value: fmt.Sprintf("ERROR: %s", err)}
			}
			return obj
		},
	},
	"config_load": {
		Fn: func(args ...Object) Object {
			if len(args) < 1 || len(args) > 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1 or 2", len(args))}
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
			obj, err := loadConfigObjectFromPath(path, format)
			if err != nil {
				return &String{Value: fmt.Sprintf("ERROR: %s", err)}
			}
			return obj
		},
	},
	"config_parse": {
		Fn: func(args ...Object) Object {
			if len(args) != 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
			}
			raw, errObj := asString(args[0], "raw")
			if errObj != nil {
				return errObj
			}
			format, errObj := asString(args[1], "format")
			if errObj != nil {
				return errObj
			}
			obj, err := loadConfigObjectFromRaw(raw, format)
			if err != nil {
				return &String{Value: fmt.Sprintf("ERROR: %s", err)}
			}
			return obj
		},
	},
	"secret": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			s, errObj := asString(args[0], "value")
			if errObj != nil {
				return errObj
			}
			return &Secret{Value: s}
		},
	},
	"secret_reveal": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			if s, ok := secretValue(args[0]); ok {
				return &String{Value: s}
			}
			if args[0].Type() == STRING_OBJ {
				return args[0]
			}
			return newError("argument `secret_value` must be SECRET or STRING, got %s", args[0].Type())
		},
	},
	"secret_mask": {
		Fn: func(args ...Object) Object {
			if len(args) < 1 || len(args) > 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1 or 2", len(args))}
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
				return &String{Value: ""}
			}
			if int(visible) >= len(s) {
				return &String{Value: strings.Repeat("*", len(s))}
			}
			masked := strings.Repeat("*", len(s)-int(visible)) + s[len(s)-int(visible):]
			return &String{Value: masked}
		},
	},
	"parse_int": {
		Fn: func(args ...Object) Object {
			if len(args) < 1 || len(args) > 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1 or 2", len(args))}
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
				return &String{Value: fmt.Sprintf("ERROR: %s", err)}
			}
			return &Integer{Value: v}
		},
	},
	"parse_float": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			s, errObj := asString(args[0], "value")
			if errObj != nil {
				return errObj
			}
			v, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return &String{Value: fmt.Sprintf("ERROR: %s", err)}
			}
			return &Float{Value: v}
		},
	},
	"parse_type": {
		Fn: func(args ...Object) Object {
			if len(args) < 2 || len(args) > 4 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2..4", len(args))}
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
				return &String{Value: args[0].Inspect()}
			case "bool", "boolean":
				return toBoolObject(args[0])
			case "time", "timestamp":
				if len(args) == 2 {
					if args[0].Type() == STRING_OBJ {
						tm, err := time.Parse(time.RFC3339, args[0].(*String).Value)
						if err != nil {
							return newError("%s", err)
						}
						return &Integer{Value: tm.Unix()}
					}
					return newError("parse_type(time) with 2 args expects STRING input")
				}
				if args[0].Type() != STRING_OBJ {
					return newError("parse_type(time) expects STRING input")
				}
				if len(args) == 3 {
					format, e := asString(args[2], "format")
					if e != nil {
						return e
					}
					tm, err := time.Parse(normalizeTimeFormat(format), args[0].(*String).Value)
					if err != nil {
						return newError("%s", err)
					}
					return &Integer{Value: tm.Unix()}
				}
				format, e := asString(args[2], "format")
				if e != nil {
					return e
				}
				tz, e := asString(args[3], "timezone")
				if e != nil {
					return e
				}
				loc, errObj := loadLocationOrError(tz)
				if errObj != nil {
					return errObj
				}
				tm, err := time.ParseInLocation(normalizeTimeFormat(format), args[0].(*String).Value, loc)
				if err != nil {
					return newError("%s", err)
				}
				return &Integer{Value: tm.Unix()}
			default:
				return newError("unsupported target_type %q", target)
			}
		},
	},
	"range": {
		Fn: func(args ...Object) Object {
			if len(args) < 1 || len(args) > 3 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1..3", len(args))}
			}
			var start, end, step int64
			var errObj Object
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
				return &String{Value: "ERROR: step cannot be zero"}
			}
			elements := []Object{}
			if step > 0 {
				for i := start; i < end; i += step {
					elements = append(elements, &Integer{Value: i})
				}
			} else {
				for i := start; i > end; i += step {
					elements = append(elements, &Integer{Value: i})
				}
			}
			return &Array{Elements: elements}
		},
	},
	"contains": {
		Fn: func(args ...Object) Object {
			if len(args) != 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
			}
			switch args[0].Type() {
			case STRING_OBJ:
				if args[1].Type() != STRING_OBJ {
					return &String{Value: fmt.Sprintf("needle must be STRING for string contains, got %s", args[1].Type())}
				}
				return nativeBoolToBooleanObject(strings.Contains(args[0].(*String).Value, args[1].(*String).Value))
			case ARRAY_OBJ:
				arr := args[0].(*Array)
				target := args[1].Inspect()
				for _, el := range arr.Elements {
					if el.Inspect() == target {
						return TRUE
					}
				}
				return FALSE
			default:
				return &String{Value: fmt.Sprintf("contains not supported for %s", args[0].Type())}
			}
		},
	},
	"sort": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			if args[0].Type() != ARRAY_OBJ {
				return &String{Value: fmt.Sprintf("argument to `sort` must be ARRAY, got %s", args[0].Type())}
			}
			arr := args[0].(*Array)
			out := make([]Object, len(arr.Elements))
			copy(out, arr.Elements)
			allInt := true
			allString := true
			for _, el := range out {
				if el.Type() != INTEGER_OBJ {
					allInt = false
				}
				if el.Type() != STRING_OBJ {
					allString = false
				}
			}
			switch {
			case allInt:
				sort.Slice(out, func(i, j int) bool {
					return out[i].(*Integer).Value < out[j].(*Integer).Value
				})
			case allString:
				sort.Slice(out, func(i, j int) bool {
					return out[i].(*String).Value < out[j].(*String).Value
				})
			default:
				return &String{Value: "ERROR: sort supports only homogeneous ARRAY of INTEGER or STRING"}
			}
			return &Array{Elements: out}
		},
	},
	"uniq": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			if args[0].Type() != ARRAY_OBJ {
				return &String{Value: fmt.Sprintf("argument to `uniq` must be ARRAY, got %s", args[0].Type())}
			}
			arr := args[0].(*Array)
			seen := map[string]bool{}
			out := []Object{}
			for _, el := range arr.Elements {
				key := el.Type().String() + "::" + el.Inspect()
				if !seen[key] {
					seen[key] = true
					out = append(out, el)
				}
			}
			return &Array{Elements: out}
		},
	},
	"find": {
		Fn: func(args ...Object) Object {
			if len(args) != 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
			}
			if args[0].Type() != ARRAY_OBJ {
				return &String{Value: fmt.Sprintf("first argument to `find` must be ARRAY, got %s", args[0].Type())}
			}
			arr := args[0].(*Array)
			target := args[1].Type().String() + "::" + args[1].Inspect()
			for _, el := range arr.Elements {
				if el.Type().String()+"::"+el.Inspect() == target {
					return el
				}
			}
			return NULL
		},
	},
	"reduce": {
		Fn: func(args ...Object) Object {
			if len(args) != 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
			}
			if args[0].Type() != ARRAY_OBJ {
				return &String{Value: fmt.Sprintf("first argument to `reduce` must be ARRAY, got %s", args[0].Type())}
			}
			op, errObj := asString(args[1], "operator")
			if errObj != nil {
				return errObj
			}
			arr := args[0].(*Array)
			if len(arr.Elements) == 0 {
				return NULL
			}
			switch op {
			case "sum":
				var total int64
				for _, el := range arr.Elements {
					if el.Type() != INTEGER_OBJ {
						return &String{Value: "ERROR: reduce(sum) supports INTEGER array only"}
					}
					total += el.(*Integer).Value
				}
				return &Integer{Value: total}
			case "concat":
				var sb strings.Builder
				for _, el := range arr.Elements {
					sb.WriteString(el.Inspect())
				}
				return &String{Value: sb.String()}
			default:
				return &String{Value: fmt.Sprintf("ERROR: unsupported reduce operator %q", op)}
			}
		},
	},
	"go_async": {
		Fn: func(args ...Object) Object {
			if len(args) < 1 {
				return newError("go_async: expected at least 1 argument (function)")
			}
			fn := args[0]
			fnArgs := args[1:]
			go func() {
				applyFn(fn, fnArgs, nil, nil)
			}()
			return NULL
		},
	},
	"await_all": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return newError("await_all: expected 1 argument (array of futures), got %d", len(args))
			}
			arr, ok := args[0].(*Array)
			if !ok {
				return newError("await_all: argument must be an array, got %s", args[0].Type())
			}
			results := make([]Object, len(arr.Elements))
			for i, elem := range arr.Elements {
				if future, ok := elem.(*Future); ok {
					results[i] = future.Resolve()
				} else {
					results[i] = elem
				}
			}
			return &Array{Elements: results}
		},
	},
	"await_race": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return newError("await_race: expected 1 argument (array of futures), got %d", len(args))
			}
			arr, ok := args[0].(*Array)
			if !ok {
				return newError("await_race: argument must be an array, got %s", args[0].Type())
			}
			if len(arr.Elements) == 0 {
				return NULL
			}
			ch := make(chan Object, len(arr.Elements))
			for _, elem := range arr.Elements {
				elem := elem
				go func() {
					if future, ok := elem.(*Future); ok {
						ch <- future.Resolve()
					} else {
						ch <- elem
					}
				}()
			}
			return <-ch
		},
	},
}
