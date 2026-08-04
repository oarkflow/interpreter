package lua

type tokenKind uint8

const (
	tEOF tokenKind = iota
	tName
	tNumber
	tString
	tAnd
	tBreak
	tDo
	tElse
	tElseIf
	tEnd
	tFalse
	tFor
	tFunction
	tIf
	tIn
	tLocal
	tNil
	tNot
	tOr
	tRepeat
	tReturn
	tThen
	tTrue
	tUntil
	tWhile
	tPlus
	tMinus
	tStar
	tSlash
	tPercent
	tCaret
	tHash
	tEqEq
	tNotEq
	tLTE
	tGTE
	tLT
	tGT
	tAssign
	tLParen
	tRParen
	tLBrace
	tRBrace
	tLBracket
	tRBracket
	tSemi
	tColon
	tComma
	tDot
	tConcat
	tDots
)

type token struct {
	kind tokenKind
	lit  string
	num  float64
	line int
}

var keywords = map[string]tokenKind{
	"and": tAnd, "break": tBreak, "do": tDo, "else": tElse,
	"elseif": tElseIf, "end": tEnd, "false": tFalse, "for": tFor,
	"function": tFunction, "if": tIf, "in": tIn, "local": tLocal,
	"nil": tNil, "not": tNot, "or": tOr, "repeat": tRepeat,
	"return": tReturn, "then": tThen, "true": tTrue, "until": tUntil,
	"while": tWhile,
}
