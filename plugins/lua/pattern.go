package lua

import (
	"strings"
)

const maxPatternCaptures = 32

type patternCapture struct {
	start, end int
	position   bool
}

type patternResult struct {
	start, end int
	captures   []patternCapture
}

type luaPattern struct {
	source, pattern string
}

func compilePattern(source, pattern string) (*luaPattern, error) {
	if err := validatePattern(pattern); err != nil {
		return nil, err
	}
	return &luaPattern{source: source, pattern: pattern}, nil
}

func validatePattern(pattern string) error {
	captures := 0
	open := make([]int, 0, 4)
	closed := [maxPatternCaptures]bool{}
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '[':
			end, ok := patternClassEnd(pattern, i)
			if !ok {
				return runtimeError("malformed pattern (missing ']')")
			}
			i = end - 1
		case '(':
			if captures >= maxPatternCaptures {
				return runtimeError("too many captures")
			}
			index := captures
			captures++
			if i+1 < len(pattern) && pattern[i+1] == ')' {
				closed[index] = true
				i++
			} else {
				open = append(open, index)
			}
		case ')':
			if len(open) == 0 {
				return runtimeError("invalid pattern capture")
			}
			index := open[len(open)-1]
			open = open[:len(open)-1]
			closed[index] = true
		case '%':
			if i+1 >= len(pattern) {
				return runtimeError("malformed pattern (ends with '%%')")
			}
			if pattern[i+1] == 'b' {
				if i+3 >= len(pattern) {
					return runtimeError("malformed pattern (missing arguments to '%%b')")
				}
				i += 3
			} else if pattern[i+1] == 'f' {
				if i+2 >= len(pattern) || pattern[i+2] != '[' {
					return runtimeError("missing '[' after '%%f' in pattern")
				}
				end, ok := patternClassEnd(pattern, i+2)
				if !ok {
					return runtimeError("malformed pattern (missing ']')")
				}
				i = end - 1
			} else if pattern[i+1] >= '0' && pattern[i+1] <= '9' {
				index := int(pattern[i+1] - '1')
				if pattern[i+1] == '0' || index < 0 || index >= captures || !closed[index] {
					return runtimeError("invalid capture index")
				}
				i++
			} else {
				i++
			}
		}
	}
	if len(open) != 0 {
		return runtimeError("unfinished capture")
	}
	return nil
}

func patternClassEnd(pattern string, start int) (int, bool) {
	i := start + 1
	if i < len(pattern) && pattern[i] == '^' {
		i++
	}
	if i < len(pattern) && pattern[i] == ']' {
		i++
	}
	for i < len(pattern) {
		if pattern[i] == '%' && i+1 < len(pattern) {
			i += 2
			continue
		}
		if pattern[i] == ']' {
			return i + 1, true
		}
		i++
	}
	return 0, false
}

func (p *luaPattern) find(init int) (patternResult, bool) {
	if init < 0 {
		init = len(p.source) + init + 1
	}
	if init < 1 {
		init = 1
	}
	if init > len(p.source)+1 {
		return patternResult{}, false
	}
	anchored := strings.HasPrefix(p.pattern, "^")
	patternStart := 0
	if anchored {
		patternStart = 1
	}
	for start := init - 1; start <= len(p.source); start++ {
		end, captures, ok := p.match(start, patternStart, nil)
		if ok {
			return patternResult{start: start, end: end, captures: captures}, true
		}
		if anchored {
			break
		}
	}
	return patternResult{}, false
}

func cloneCaptures(captures []patternCapture) []patternCapture {
	return append([]patternCapture(nil), captures...)
}

func (p *luaPattern) match(si, pi int, captures []patternCapture) (int, []patternCapture, bool) {
	if pi == len(p.pattern) {
		return si, captures, true
	}
	if p.pattern[pi] == '$' && pi+1 == len(p.pattern) {
		return si, captures, si == len(p.source)
	}
	if p.pattern[pi] == '(' {
		if len(captures) >= maxPatternCaptures {
			return 0, nil, false
		}
		next := cloneCaptures(captures)
		if pi+1 < len(p.pattern) && p.pattern[pi+1] == ')' {
			next = append(next, patternCapture{start: si, end: si, position: true})
			return p.match(si, pi+2, next)
		}
		next = append(next, patternCapture{start: si, end: -1})
		return p.match(si, pi+1, next)
	}
	if p.pattern[pi] == ')' {
		index := -1
		for i := len(captures) - 1; i >= 0; i-- {
			if captures[i].end < 0 {
				index = i
				break
			}
		}
		if index < 0 {
			return 0, nil, false
		}
		next := cloneCaptures(captures)
		next[index].end = si
		return p.match(si, pi+1, next)
	}
	end := patternItemEnd(p.pattern, pi)
	quantifier := byte(0)
	if end < len(p.pattern) && strings.ContainsRune("*+-?", rune(p.pattern[end])) {
		quantifier = p.pattern[end]
		end++
	}
	switch quantifier {
	case '?':
		if next, ok := p.matchItem(si, pi, captures); ok {
			if result, resultCaps, matched := p.match(next, end, captures); matched {
				return result, resultCaps, true
			}
		}
		return p.match(si, end, captures)
	case '*', '+':
		positions := []int{si}
		current := si
		for {
			next, ok := p.matchItem(current, pi, captures)
			if !ok || next == current {
				break
			}
			positions = append(positions, next)
			current = next
		}
		minimum := 0
		if quantifier == '+' {
			minimum = 1
		}
		for i := len(positions) - 1; i >= minimum; i-- {
			if result, resultCaps, ok := p.match(positions[i], end, cloneCaptures(captures)); ok {
				return result, resultCaps, true
			}
		}
		return 0, nil, false
	case '-':
		current := si
		for {
			if result, resultCaps, ok := p.match(current, end, cloneCaptures(captures)); ok {
				return result, resultCaps, true
			}
			next, ok := p.matchItem(current, pi, captures)
			if !ok || next == current {
				return 0, nil, false
			}
			current = next
		}
	default:
		next, ok := p.matchItem(si, pi, captures)
		if !ok {
			return 0, nil, false
		}
		return p.match(next, end, captures)
	}
}

func patternItemEnd(pattern string, pi int) int {
	if pattern[pi] == '[' {
		end, _ := patternClassEnd(pattern, pi)
		return end
	}
	if pattern[pi] == '%' && pi+1 < len(pattern) {
		if pattern[pi+1] == 'b' {
			return pi + 4
		}
		if pattern[pi+1] == 'f' {
			end, _ := patternClassEnd(pattern, pi+2)
			return end
		}
		return pi + 2
	}
	return pi + 1
}

func (p *luaPattern) matchItem(si, pi int, captures []patternCapture) (int, bool) {
	item := p.pattern[pi]
	if item == '%' {
		escape := p.pattern[pi+1]
		switch escape {
		case 'b':
			if si >= len(p.source) || p.source[si] != p.pattern[pi+2] {
				return 0, false
			}
			open, close, depth := p.pattern[pi+2], p.pattern[pi+3], 1
			for i := si + 1; i < len(p.source); i++ {
				if p.source[i] == close {
					depth--
					if depth == 0 {
						return i + 1, true
					}
				} else if p.source[i] == open {
					depth++
				}
			}
			return 0, false
		case 'f':
			end, _ := patternClassEnd(p.pattern, pi+2)
			previous := byte(0)
			if si > 0 {
				previous = p.source[si-1]
			}
			current := byte(0)
			if si < len(p.source) {
				current = p.source[si]
			}
			set := p.pattern[pi+3 : end-1]
			if matchBracketClass(previous, set) || !matchBracketClass(current, set) {
				return 0, false
			}
			return si, true
		}
		if escape >= '1' && escape <= '9' {
			index := int(escape - '1')
			if index >= len(captures) || captures[index].end < 0 || captures[index].position {
				return 0, false
			}
			text := p.source[captures[index].start:captures[index].end]
			if strings.HasPrefix(p.source[si:], text) {
				return si + len(text), true
			}
			return 0, false
		}
		if si < len(p.source) && matchPatternClass(p.source[si], escape) {
			return si + 1, true
		}
		return 0, false
	}
	if si >= len(p.source) {
		return 0, false
	}
	if item == '.' {
		return si + 1, true
	}
	if item == '[' {
		end, _ := patternClassEnd(p.pattern, pi)
		if matchBracketClass(p.source[si], p.pattern[pi+1:end-1]) {
			return si + 1, true
		}
		return 0, false
	}
	if p.source[si] == item {
		return si + 1, true
	}
	return 0, false
}

func matchPatternClass(char, class byte) bool {
	var matched bool
	switch class | 0x20 {
	case 'a':
		matched = char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z'
	case 'c':
		matched = char < 32 || char == 127
	case 'd':
		matched = char >= '0' && char <= '9'
	case 'l':
		matched = char >= 'a' && char <= 'z'
	case 'p':
		matched = char >= 33 && char <= 126 && !(char >= '0' && char <= '9') && !(char >= 'A' && char <= 'Z') && !(char >= 'a' && char <= 'z')
	case 's':
		matched = char == ' ' || char == '\t' || char == '\n' || char == '\r' || char == '\f' || char == '\v'
	case 'u':
		matched = char >= 'A' && char <= 'Z'
	case 'w':
		matched = char >= '0' && char <= '9' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z'
	case 'x':
		matched = char >= '0' && char <= '9' || char >= 'A' && char <= 'F' || char >= 'a' && char <= 'f'
	case 'z':
		matched = char == 0
	default:
		return char == class
	}
	if class >= 'A' && class <= 'Z' {
		return !matched
	}
	return matched
}

func matchBracketClass(char byte, set string) bool {
	negated := len(set) > 0 && set[0] == '^'
	if negated {
		set = set[1:]
	}
	matched := false
	for i := 0; i < len(set); {
		if set[i] == '%' && i+1 < len(set) {
			if matchPatternClass(char, set[i+1]) {
				matched = true
			}
			i += 2
			continue
		}
		if i+2 < len(set) && set[i+1] == '-' {
			if char >= set[i] && char <= set[i+2] {
				matched = true
			}
			i += 3
			continue
		}
		if char == set[i] {
			matched = true
		}
		i++
	}
	if negated {
		return !matched
	}
	return matched
}

func patternCaptureValues(source string, result patternResult) []Value {
	if len(result.captures) == 0 {
		return []Value{String(source[result.start:result.end])}
	}
	values := make([]Value, len(result.captures))
	for i, capture := range result.captures {
		if capture.position {
			values[i] = Number(float64(capture.start + 1))
		} else {
			values[i] = String(source[capture.start:capture.end])
		}
	}
	return values
}
