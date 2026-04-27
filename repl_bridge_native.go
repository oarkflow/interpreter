//go:build !js

package interpreter

import (
	"fmt"

	"github.com/oarkflow/interpreter/pkg/ast"
	"github.com/oarkflow/interpreter/pkg/config"
	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/lexer"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/parser"
	"github.com/oarkflow/interpreter/pkg/repl"
	"github.com/oarkflow/interpreter/pkg/token"
)

func initReplBridge() {
	repl.BuiltinHelpTextFn = eval.BuiltinHelpText
	repl.HasBuiltinFn = eval.HasBuiltin
	repl.IsErrorFn = object.IsError
	repl.FormatCallStackFn = object.FormatCallStack
	repl.ObjectErrorStringFn = func(obj object.Object) string {
		if e, ok := obj.(*object.Error); ok {
			return e.Message
		}
		return obj.Inspect()
	}
	repl.NewLexerFn = func(input string) any { return lexer.NewLexer(input) }
	repl.LexerNextTokenFn = func(l any) (int, string) {
		tok := l.(*lexer.Lexer).NextToken()
		return int(tok.Type), tok.Literal
	}
	repl.NewParserFn = func(l any) any { return parser.NewParser(l.(*lexer.Lexer)) }
	repl.ParseProgramFn = func(p any) (any, []string) {
		pp := p.(*parser.Parser)
		prog := pp.ParseProgram()
		return prog, pp.Errors()
	}
	repl.EvalFn = func(program any, env *object.Environment) object.Object {
		return eval.Eval(program.(*ast.Program), env)
	}
	repl.LineContextFn = func(source string, line, column int) string {
		return parser.LineContext(source, line, column)
	}
	repl.LoadConfigObjectFromPathFn = func(path, format string) (object.Object, error) {
		return config.LoadConfigObjectFromPath(path, format)
	}
	repl.ResolveImportPathFn = func(path string, env *object.Environment) (string, error) {
		return resolveImportPath(path, env)
	}

	repl.TOKEN_EOF = int(token.EOF)
	repl.TOKEN_LPAREN = int(token.LPAREN)
	repl.TOKEN_RPAREN = int(token.RPAREN)
	repl.TOKEN_LBRACE = int(token.LBRACE)
	repl.TOKEN_RBRACE = int(token.RBRACE)
	repl.TOKEN_LBRACKET = int(token.LBRACKET)
	repl.TOKEN_RBRACKET = int(token.RBRACKET)
	repl.TOKEN_ASSIGN = int(token.ASSIGN)
	repl.TOKEN_PLUS = int(token.PLUS)
	repl.TOKEN_MINUS = int(token.MINUS)
	repl.TOKEN_MULTIPLY = int(token.MULTIPLY)
	repl.TOKEN_DIVIDE = int(token.DIVIDE)
	repl.TOKEN_MODULO = int(token.MODULO)
	repl.TOKEN_EQ = int(token.EQ)
	repl.TOKEN_NEQ = int(token.NEQ)
	repl.TOKEN_LT = int(token.LT)
	repl.TOKEN_GT = int(token.GT)
	repl.TOKEN_LTE = int(token.LTE)
	repl.TOKEN_GTE = int(token.GTE)
	repl.TOKEN_AND = int(token.AND)
	repl.TOKEN_OR = int(token.OR)
	repl.TOKEN_BITAND = int(token.BITAND)
	repl.TOKEN_BITOR = int(token.BITOR)
	repl.TOKEN_BITXOR = int(token.BITXOR)
	repl.TOKEN_COMMA = int(token.COMMA)
	repl.TOKEN_COLON = int(token.COLON)
	repl.TOKEN_DOT = int(token.DOT)
	repl.TOKEN_LET = int(token.LET)
	repl.TOKEN_CONST = int(token.CONST)
	repl.TOKEN_RETURN = int(token.RETURN)
	repl.TOKEN_IF = int(token.IF)
	repl.TOKEN_ELSE = int(token.ELSE)
	repl.TOKEN_FOR = int(token.FOR)
	repl.TOKEN_WHILE = int(token.WHILE)
	repl.TOKEN_FUNCTION = int(token.FUNCTION)
	repl.TOKEN_TRY = int(token.TRY)
	repl.TOKEN_CATCH = int(token.CATCH)
	repl.TOKEN_SWITCH = int(token.SWITCH)
	repl.TOKEN_CASE = int(token.CASE)
	repl.TOKEN_THROW = int(token.THROW)
	repl.TOKEN_IMPORT = int(token.IMPORT)
	repl.TOKEN_EXPORT = int(token.EXPORT)

	eval.RunReplFn = func() {
		env := object.NewEnvironment()
		fmt.Println("Welcome to SPL REPL. Type 'exit' to quit.")
		if err := repl.RunReplInteractive(env); err != nil {
			repl.RunReplBasic(env)
		}
	}
}
