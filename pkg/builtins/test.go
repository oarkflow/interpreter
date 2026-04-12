package builtins

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/lexer"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/parser"
)

// ---------------------------------------------------------------------------
// Test statistics helpers
// ---------------------------------------------------------------------------

var testStats struct {
	total, passed, failed int
}

func resetTestStats() {
	testStats.total = 0
	testStats.passed = 0
	testStats.failed = 0
}

func assertPass() object.Object {
	testStats.total++
	testStats.passed++
	return object.TRUE
}

func assertFail(msg string) object.Object {
	testStats.total++
	testStats.failed++
	if msg == "" {
		msg = "assertion failed"
	}
	return &object.String{Value: "ERROR: " + msg}
}

func testSummaryObject() *object.Hash {
	return &object.Hash{
		Pairs: map[object.HashKey]object.HashPair{
			(&object.String{Value: "total"}).HashKey(): {
				Key:   &object.String{Value: "total"},
				Value: &object.Integer{Value: int64(testStats.total)},
			},
			(&object.String{Value: "passed"}).HashKey(): {
				Key:   &object.String{Value: "passed"},
				Value: &object.Integer{Value: int64(testStats.passed)},
			},
			(&object.String{Value: "failed"}).HashKey(): {
				Key:   &object.String{Value: "failed"},
				Value: &object.Integer{Value: int64(testStats.failed)},
			},
		},
	}
}

func init() {
	eval.RegisterBuiltins(map[string]*object.Builtin{

		// -----------------------------------------------------------------
		// help
		// -----------------------------------------------------------------
		"help": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) > 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=0 or 1", len(args))}
				}

				if len(args) == 0 {
					names := eval.BuiltinNames()
					elements := make([]object.Object, len(names))
					for i, name := range names {
						elements[i] = &object.String{Value: name}
					}
					return &object.Array{Elements: elements}
				}

				if args[0].Type() != object.STRING_OBJ {
					return &object.String{Value: fmt.Sprintf("argument to `help` must be STRING, got %s", args[0].Type())}
				}

				name := args[0].(*object.String).Value
				if !eval.HasBuiltin(name) {
					return &object.String{Value: fmt.Sprintf("ERROR: builtin %q not found", name)}
				}
				return &object.String{Value: eval.BuiltinHelpText(name)}
			},
		},

		// -----------------------------------------------------------------
		// assert_true
		// -----------------------------------------------------------------
		"assert_true": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 || len(args) > 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1 or 2", len(args))}
				}
				if object.IsTruthy(args[0]) {
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
		},

		// -----------------------------------------------------------------
		// assert_eq
		// -----------------------------------------------------------------
		"assert_eq": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 2 || len(args) > 3 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2 or 3", len(args))}
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
		},

		// -----------------------------------------------------------------
		// assert_neq
		// -----------------------------------------------------------------
		"assert_neq": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 2 || len(args) > 3 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2 or 3", len(args))}
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
		},

		// -----------------------------------------------------------------
		// assert_contains
		// -----------------------------------------------------------------
		"assert_contains": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 2 || len(args) > 3 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2 or 3", len(args))}
				}
				found := false
				switch haystack := args[0].(type) {
				case *object.String:
					needle, ok := args[1].(*object.String)
					if !ok {
						return &object.String{Value: "assert_contains: needle must be a string when haystack is a string"}
					}
					found = strings.Contains(haystack.Value, needle.Value)
				case *object.Array:
					for _, el := range haystack.Elements {
						if el.Inspect() == args[1].Inspect() {
							found = true
							break
						}
					}
				default:
					return &object.String{Value: fmt.Sprintf("assert_contains: first argument must be STRING or ARRAY, got %s", args[0].Type())}
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
		},

		// -----------------------------------------------------------------
		// assert_throws
		// -----------------------------------------------------------------
		"assert_throws": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 || len(args) > 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1 or 2", len(args))}
				}
				res := eval.ExecuteCallback(args[0], []object.Object{})
				if object.IsError(res) {
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
		},

		// -----------------------------------------------------------------
		// test_summary
		// -----------------------------------------------------------------
		"test_summary": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 0 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=0", len(args))}
				}
				return testSummaryObject()
			},
		},

		// -----------------------------------------------------------------
		// run_tests
		// -----------------------------------------------------------------
		"run_tests": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				resetTestStats()

				paths := []string{}
				switch args[0].Type() {
				case object.STRING_OBJ:
					paths = append(paths, args[0].(*object.String).Value)
				case object.ARRAY_OBJ:
					for _, el := range args[0].(*object.Array).Elements {
						if el.Type() != object.STRING_OBJ {
							return &object.String{Value: "ERROR: run_tests array elements must be STRING"}
						}
						paths = append(paths, el.(*object.String).Value)
					}
				default:
					return &object.String{Value: fmt.Sprintf("argument to `run_tests` must be STRING or ARRAY, got %s", args[0].Type())}
				}

				for _, p := range paths {
					safePath, err := SanitizePathLocal(p)
					if err != nil {
						return &object.String{Value: fmt.Sprintf("ERROR: invalid test path %q: %s", p, err)}
					}
					content, err := os.ReadFile(safePath)
					if err != nil {
						return &object.String{Value: fmt.Sprintf("ERROR: failed to read test file %q: %s", p, err)}
					}
					env := object.NewGlobalEnvironment([]string{})
					env.ModuleDir = filepath.Dir(safePath)
					l := lexer.NewLexer(string(content))
					par := parser.NewParser(l)
					program := par.ParseProgram()
					if len(par.Errors()) > 0 {
						return &object.String{Value: fmt.Sprintf("ERROR: parser errors in %q: %s", p, strings.Join(par.Errors(), "; "))}
					}
					result := eval.Eval(program, env)
					if object.IsError(result) {
						return &object.String{Value: fmt.Sprintf("ERROR: test execution failed in %q: %s", p, result.Inspect())}
					}
				}

				return testSummaryObject()
			},
		},
	})
}
