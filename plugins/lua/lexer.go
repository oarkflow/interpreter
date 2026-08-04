package lua

import (
	"strconv"
	"strings"
)

type lexer struct {
	source string
	name   string
	pos    int
	line   int
}

func newLexer(source, name string) *lexer {
	l := &lexer{source: source, name: name, line: 1}
	if strings.HasPrefix(source, "\xef\xbb\xbf") {
		l.pos = 3
	}
	// Lua's file loader accepts a Unix interpreter directive on the first
	// physical line. Treat it as a comment while retaining source line numbers.
	if l.pos < len(source) && source[l.pos] == '#' {
		for l.pos < len(source) && source[l.pos] != '\n' {
			l.pos++
		}
		if l.pos < len(source) {
			l.pos++
			l.line++
		}
	}
	return l
}

func (l *lexer) next() (token, error) {
	if err := l.skipSpaceAndComments(); err != nil {
		return token{}, err
	}
	if l.pos >= len(l.source) {
		return token{kind: tEOF, line: l.line}, nil
	}
	start, line, c := l.pos, l.line, l.source[l.pos]
	if isNameStart(c) {
		l.pos++
		for l.pos < len(l.source) && isNamePart(l.source[l.pos]) {
			l.pos++
		}
		lit := l.source[start:l.pos]
		if kind, ok := keywords[lit]; ok {
			return token{kind: kind, lit: lit, line: line}, nil
		}
		return token{kind: tName, lit: lit, line: line}, nil
	}
	if isDigit(c) || (c == '.' && l.pos+1 < len(l.source) && isDigit(l.source[l.pos+1])) {
		return l.scanNumber()
	}
	if c == '\'' || c == '"' {
		return l.scanQuoted(c)
	}
	if c == '[' {
		if level, content, ok, err := l.scanLongBracket(l.pos); ok || err != nil {
			_ = level
			return token{kind: tString, lit: content, line: line}, err
		}
	}
	l.pos++
	switch c {
	case '+':
		return token{kind: tPlus, lit: "+", line: line}, nil
	case '-':
		return token{kind: tMinus, lit: "-", line: line}, nil
	case '*':
		return token{kind: tStar, lit: "*", line: line}, nil
	case '/':
		return token{kind: tSlash, lit: "/", line: line}, nil
	case '%':
		return token{kind: tPercent, lit: "%", line: line}, nil
	case '^':
		return token{kind: tCaret, lit: "^", line: line}, nil
	case '#':
		return token{kind: tHash, lit: "#", line: line}, nil
	case '=':
		if l.take('=') {
			return token{kind: tEqEq, lit: "==", line: line}, nil
		}
		return token{kind: tAssign, lit: "=", line: line}, nil
	case '~':
		if l.take('=') {
			return token{kind: tNotEq, lit: "~=", line: line}, nil
		}
	case '<':
		if l.take('=') {
			return token{kind: tLTE, lit: "<=", line: line}, nil
		}
		return token{kind: tLT, lit: "<", line: line}, nil
	case '>':
		if l.take('=') {
			return token{kind: tGTE, lit: ">=", line: line}, nil
		}
		return token{kind: tGT, lit: ">", line: line}, nil
	case '(':
		return token{kind: tLParen, lit: "(", line: line}, nil
	case ')':
		return token{kind: tRParen, lit: ")", line: line}, nil
	case '{':
		return token{kind: tLBrace, lit: "{", line: line}, nil
	case '}':
		return token{kind: tRBrace, lit: "}", line: line}, nil
	case '[':
		return token{kind: tLBracket, lit: "[", line: line}, nil
	case ']':
		return token{kind: tRBracket, lit: "]", line: line}, nil
	case ';':
		return token{kind: tSemi, lit: ";", line: line}, nil
	case ':':
		return token{kind: tColon, lit: ":", line: line}, nil
	case ',':
		return token{kind: tComma, lit: ",", line: line}, nil
	case '.':
		if l.take('.') {
			if l.take('.') {
				return token{kind: tDots, lit: "...", line: line}, nil
			}
			return token{kind: tConcat, lit: "..", line: line}, nil
		}
		return token{kind: tDot, lit: ".", line: line}, nil
	}
	return token{}, &Error{Source: l.name, Line: line, Msg: "unexpected character " + strconv.QuoteRune(rune(c))}
}

func (l *lexer) skipSpaceAndComments() error {
	for l.pos < len(l.source) {
		c := l.source[l.pos]
		if c == ' ' || c == '\t' || c == '\f' || c == '\v' {
			l.pos++
			continue
		}
		if c == '\n' || c == '\r' {
			l.consumeNewline()
			l.line++
			continue
		}
		if c != '-' || l.pos+1 >= len(l.source) || l.source[l.pos+1] != '-' {
			return nil
		}
		l.pos += 2
		if l.pos < len(l.source) && l.source[l.pos] == '[' {
			if _, _, ok, err := l.scanLongBracket(l.pos); ok || err != nil {
				if err != nil {
					return err
				}
				continue
			}
		}
		for l.pos < len(l.source) && l.source[l.pos] != '\n' && l.source[l.pos] != '\r' {
			l.pos++
		}
	}
	return nil
}

func (l *lexer) scanNumber() (token, error) {
	start, line := l.pos, l.line
	if l.pos+2 <= len(l.source) && l.source[l.pos] == '0' && l.pos+1 < len(l.source) && (l.source[l.pos+1] == 'x' || l.source[l.pos+1] == 'X') {
		l.pos += 2
		for l.pos < len(l.source) && isHex(l.source[l.pos]) {
			l.pos++
		}
		i, err := strconv.ParseUint(l.source[start+2:l.pos], 16, 64)
		if err != nil {
			return token{}, &Error{Source: l.name, Line: line, Msg: "malformed hexadecimal number"}
		}
		return token{kind: tNumber, lit: l.source[start:l.pos], num: float64(i), line: line}, nil
	}
	seenDot := false
	if l.source[l.pos] == '.' {
		seenDot = true
		l.pos++
	}
	for l.pos < len(l.source) {
		c := l.source[l.pos]
		if isDigit(c) {
			l.pos++
			continue
		}
		if c == '.' && !seenDot && !(l.pos+1 < len(l.source) && l.source[l.pos+1] == '.') {
			seenDot = true
			l.pos++
			continue
		}
		break
	}
	if l.pos < len(l.source) && (l.source[l.pos] == 'e' || l.source[l.pos] == 'E') {
		l.pos++
		if l.pos < len(l.source) && (l.source[l.pos] == '+' || l.source[l.pos] == '-') {
			l.pos++
		}
		for l.pos < len(l.source) && isDigit(l.source[l.pos]) {
			l.pos++
		}
	}
	lit := l.source[start:l.pos]
	n, err := strconv.ParseFloat(lit, 64)
	if err != nil {
		if numberErr, ok := err.(*strconv.NumError); !ok || numberErr.Err != strconv.ErrRange {
			return token{}, &Error{Source: l.name, Line: line, Msg: "malformed number " + lit}
		}
	}
	return token{kind: tNumber, lit: lit, num: n, line: line}, nil
}

func (l *lexer) scanQuoted(quote byte) (token, error) {
	line := l.line
	l.pos++
	var out strings.Builder
	for l.pos < len(l.source) {
		c := l.source[l.pos]
		l.pos++
		if c == quote {
			return token{kind: tString, lit: out.String(), line: line}, nil
		}
		if c == '\n' || c == '\r' {
			return token{}, &Error{Source: l.name, Line: l.line, Msg: "unfinished string"}
		}
		if c != '\\' {
			out.WriteByte(c)
			continue
		}
		if l.pos >= len(l.source) {
			break
		}
		c = l.source[l.pos]
		l.pos++
		switch c {
		case 'a':
			out.WriteByte('\a')
		case 'b':
			out.WriteByte('\b')
		case 'f':
			out.WriteByte('\f')
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 't':
			out.WriteByte('\t')
		case 'v':
			out.WriteByte('\v')
		case '\\', '\'', '"':
			out.WriteByte(c)
		case '\n', '\r':
			if c == '\r' && l.pos < len(l.source) && l.source[l.pos] == '\n' {
				l.pos++
			} else if c == '\n' && l.pos < len(l.source) && l.source[l.pos] == '\r' {
				l.pos++
			}
			l.line++
			out.WriteByte('\n')
		default:
			if isDigit(c) {
				value, digits := int(c-'0'), 1
				for digits < 3 && l.pos < len(l.source) && isDigit(l.source[l.pos]) {
					value = value*10 + int(l.source[l.pos]-'0')
					l.pos++
					digits++
				}
				if value > 255 {
					return token{}, &Error{Source: l.name, Line: l.line, Msg: "escape sequence too large"}
				}
				out.WriteByte(byte(value))
			} else {
				out.WriteByte(c)
			}
		}
	}
	return token{}, &Error{Source: l.name, Line: line, Msg: "unfinished string"}
}

func (l *lexer) scanLongBracket(start int) (level int, content string, ok bool, err error) {
	if start >= len(l.source) || l.source[start] != '[' {
		return 0, "", false, nil
	}
	i := start + 1
	for i < len(l.source) && l.source[i] == '=' {
		i++
	}
	if i >= len(l.source) || l.source[i] != '[' {
		return 0, "", false, nil
	}
	level = i - start - 1
	contentStart := i + 1
	if contentStart < len(l.source) && (l.source[contentStart] == '\n' || l.source[contentStart] == '\r') {
		first := l.source[contentStart]
		contentStart++
		if contentStart < len(l.source) && (first == '\n' && l.source[contentStart] == '\r' || first == '\r' && l.source[contentStart] == '\n') {
			contentStart++
		}
		l.line++
	}
	close := "]" + strings.Repeat("=", level) + "]"
	endRel := strings.Index(l.source[contentStart:], close)
	if endRel < 0 {
		return level, "", true, &Error{Source: l.name, Line: l.line, Msg: "unfinished long string or comment"}
	}
	end := contentStart + endRel
	content, lineCount := normalizeLuaNewlines(l.source[contentStart:end])
	l.line += lineCount
	l.pos = end + len(close)
	return level, content, true, nil
}

func (l *lexer) consumeNewline() {
	first := l.source[l.pos]
	l.pos++
	if l.pos < len(l.source) && (first == '\n' && l.source[l.pos] == '\r' || first == '\r' && l.source[l.pos] == '\n') {
		l.pos++
	}
}

func normalizeLuaNewlines(value string) (string, int) {
	if !strings.ContainsAny(value, "\r\n") {
		return value, 0
	}
	var out strings.Builder
	out.Grow(len(value))
	lines := 0
	for i := 0; i < len(value); i++ {
		if value[i] != '\r' && value[i] != '\n' {
			out.WriteByte(value[i])
			continue
		}
		first := value[i]
		if i+1 < len(value) && (first == '\n' && value[i+1] == '\r' || first == '\r' && value[i+1] == '\n') {
			i++
		}
		out.WriteByte('\n')
		lines++
	}
	return out.String(), lines
}

func (l *lexer) take(c byte) bool {
	if l.pos < len(l.source) && l.source[l.pos] == c {
		l.pos++
		return true
	}
	return false
}
func isNameStart(c byte) bool { return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isNamePart(c byte) bool  { return isNameStart(c) || isDigit(c) }
func isDigit(c byte) bool     { return c >= '0' && c <= '9' }
func isHex(c byte) bool       { return isDigit(c) || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F' }
