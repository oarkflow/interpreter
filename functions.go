package interpreter

import (
	"bufio"
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
	"fmt"
	"math"
	"math/big"
	mrand "math/rand"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Object interface {
	Type() ObjectType
	Inspect() string
}

type BuiltinFunction func(args ...Object) Object

type Builtin struct {
	Fn BuiltinFunction
}

func (b *Builtin) Type() ObjectType { return BUILTIN_OBJ }
func (b *Builtin) Inspect() string  { return "builtin function" }

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

func asString(arg Object, name string) (string, Object) {
	if arg.Type() != STRING_OBJ {
		return "", &String{Value: fmt.Sprintf("argument `%s` must be STRING, got %s", name, arg.Type())}
	}
	return arg.(*String).Value, nil
}

func asInt(arg Object, name string) (int64, Object) {
	if arg.Type() != INTEGER_OBJ {
		return 0, &String{Value: fmt.Sprintf("argument `%s` must be INTEGER, got %s", name, arg.Type())}
	}
	return arg.(*Integer).Value, nil
}

func toStringSlice(arr *Array) ([]string, Object) {
	out := make([]string, len(arr.Elements))
	for i, el := range arr.Elements {
		if el.Type() != STRING_OBJ {
			return nil, &String{Value: fmt.Sprintf("array element at index %d must be STRING, got %s", i, el.Type())}
		}
		out[i] = el.(*String).Value
	}
	return out, nil
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

			content := args[1].(*String).Value
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
				os.Setenv(key, val)
				return NULL
			} else {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1 or 2", len(args))}
			}
		},
	},
	"exit": {
		Fn: func(args ...Object) Object {
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
		Fn: func(args ...Object) Object {
			// Whitelist of allowed commands for security
			allowedCommands := map[string]bool{
				"echo":   true,
				"date":   true,
				"ls":     true,
				"pwd":    true,
				"cat":    true,
				"grep":   true,
				"wc":     true,
				"head":   true,
				"tail":   true,
				"whoami": true,
				"sort":   true,
				"uniq":   true,
			}

			if len(args) < 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=at least 1", len(args))}
			}
			if args[0].Type() != STRING_OBJ {
				return &String{Value: fmt.Sprintf("command must be STRING, got %s", args[0].Type())}
			}

			cmdName := args[0].(*String).Value

			if !allowedCommands[cmdName] {
				return &String{Value: fmt.Sprintf("ERROR: command '%s' is not in the allowed whitelist", cmdName)}
			}

			cmdArgs := []string{}

			for i := 1; i < len(args); i++ {
				if args[i].Type() != STRING_OBJ {
					return &String{Value: fmt.Sprintf("exec argument %d must be STRING, got %s", i, args[i].Type())}
				}
				cmdArgs = append(cmdArgs, args[i].(*String).Value)
			}

			cmd := exec.Command(cmdName, cmdArgs...)
			output, err := cmd.CombinedOutput()
			if err != nil {
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
			switch arg := args[0].(type) {
			case *Integer:
				return arg
			case *String:
				val, err := strconv.ParseInt(arg.Value, 10, 64)
				if err != nil {
					return &String{Value: fmt.Sprintf("ERROR: could not convert %q to int", arg.Value)}
				}
				return &Integer{Value: val}
			case *Boolean:
				if arg.Value {
					return &Integer{Value: 1}
				}
				return &Integer{Value: 0}
			default:
				return &String{Value: fmt.Sprintf("ERROR: cannot convert %s to int", arg.Type())}
			}
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
			return &Integer{Value: mrand.Int63n(max)}
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
	"uuid_v4": {
		Fn: func(args ...Object) Object {
			if len(args) != 0 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=0", len(args))}
			}
			id, err := makeUUIDv4()
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
}
