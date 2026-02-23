package interpreter

import (
	"bufio"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
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
			return &Integer{Value: rand.Int63n(max)}
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
}
