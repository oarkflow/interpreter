package interpreter

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"syscall"
	"unicode"
	"unsafe"
)

type replEditor struct {
	in         *os.File
	out        *os.File
	fd         int
	oldState   syscall.Termios
	history    []string
	historyPos int
	candidates []string
}

type keyAction int

const (
	keyUnknown keyAction = iota
	keyUp
	keyDown
	keyLeft
	keyRight
	keyHome
	keyEnd
	keyDelete
)

func runReplInteractive(env *Environment) error {
	if !isTerminal(os.Stdin) {
		return fmt.Errorf("stdin is not a terminal")
	}

	editor, err := newReplEditor(os.Stdin, os.Stdout, replCandidates())
	if err != nil {
		return err
	}
	defer editor.close()

	for {
		line, err := editor.readLine(">> ")
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		if handleReplMetaCommand(line, editor) {
			continue
		}
		if strings.TrimSpace(line) == "exit" {
			return nil
		}
		if strings.TrimSpace(line) == "" {
			continue
		}

		input := line
		braceCount := countBraces(line)
		for braceCount > 0 {
			nextLine, err := editor.readLine(".. ")
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			input += "\n" + nextLine
			braceCount += countBraces(nextLine)
		}

		evalReplInput(input, env)
	}
}

func runReplBasic(env *Environment) {
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print(stylePrompt(">> "))
		if !scanner.Scan() {
			return
		}
		line := scanner.Text()
		if strings.TrimSpace(line) == "exit" {
			return
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		if handleReplMetaCommand(line, nil) {
			continue
		}

		input := line
		braceCount := countBraces(line)
		for braceCount > 0 {
			fmt.Print(styleContinuationPrompt(".. "))
			if !scanner.Scan() {
				return
			}
			nextLine := scanner.Text()
			input += "\n" + nextLine
			braceCount += countBraces(nextLine)
		}
		evalReplInput(input, env)
	}
}

func evalReplInput(input string, env *Environment) {
	l := NewLexer(input)
	p := NewParser(l)
	program := p.ParseProgram()
	if len(p.Errors()) != 0 {
		for _, msg := range p.Errors() {
			fmt.Println(paint(msg, colorRed))
		}
		return
	}

	evaluated := Eval(program, env)
	if evaluated != nil {
		if isError(evaluated) {
			fmt.Println(paint("ERROR: "+objectErrorString(evaluated), colorBold+colorRed))
			return
		}
		if evaluated.Type() != NULL_OBJ {
			fmt.Println(formatObjectForDisplay(evaluated))
		}
	}
}

func handleReplMetaCommand(line string, editor *replEditor) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	if trimmed == ":help" {
		fmt.Println(paint("Interactive features:", colorBold+colorCyan))
		fmt.Println("- Arrow keys: history and cursor movement")
		fmt.Println("- Tab: completion for builtins/keywords")
		fmt.Println("- Inline suggestion: gray suffix preview")
		fmt.Println("- Ctrl+C: clear current line")
		fmt.Println("- Ctrl+D: exit when line is empty")
		fmt.Println(paint("Commands:", colorBold+colorCyan))
		fmt.Println("- :builtins   list all available builtins")
		fmt.Println("- :search X   search builtins/keywords by text")
		fmt.Println("- :history    print command history")
		fmt.Println("- :clear      clear screen")
		return true
	}
	if trimmed == ":builtins" {
		names := make([]string, 0, len(builtins))
		for name := range builtins {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Println(name)
		}
		return true
	}
	if trimmed == ":history" {
		if editor == nil {
			fmt.Println("history is only available in interactive mode")
			return true
		}
		for i, h := range editor.history {
			fmt.Printf("%4d  %s\n", i+1, h)
		}
		return true
	}
	if trimmed == ":clear" {
		fmt.Print("\033[2J\033[H")
		return true
	}
	if strings.HasPrefix(trimmed, ":search ") {
		query := strings.TrimSpace(strings.TrimPrefix(trimmed, ":search "))
		if query == "" {
			fmt.Println("usage: :search <text>")
			return true
		}
		candidates := replCandidates()
		found := 0
		for _, c := range candidates {
			if strings.Contains(strings.ToLower(c), strings.ToLower(query)) {
				fmt.Println(c)
				found++
			}
		}
		if found == 0 {
			fmt.Println("no matches found")
		}
		return true
	}
	return false
}

func replCandidates() []string {
	kw := []string{
		"let", "if", "else", "while", "for", "break", "continue", "function", "return",
		"print", "const", "import", "export", "true", "false", "null",
		"exit", ":help", ":builtins", ":search", ":history", ":clear",
	}
	all := make(map[string]struct{}, len(builtins)+len(kw))
	for name := range builtins {
		all[name] = struct{}{}
	}
	for _, k := range kw {
		all[k] = struct{}{}
	}
	out := make([]string, 0, len(all))
	for k := range all {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func newReplEditor(in, out *os.File, candidates []string) (*replEditor, error) {
	fd := int(in.Fd())
	state, err := tcget(fd)
	if err != nil {
		return nil, err
	}
	raw := state
	raw.Lflag &^= syscall.ICANON | syscall.ECHO | syscall.IEXTEN
	raw.Iflag &^= syscall.ICRNL | syscall.IXON
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0
	if err := tcset(fd, &raw); err != nil {
		return nil, err
	}
	return &replEditor{
		in:         in,
		out:        out,
		fd:         fd,
		oldState:   state,
		history:    make([]string, 0, 256),
		historyPos: 0,
		candidates: candidates,
	}, nil
}

func (e *replEditor) close() {
	_ = tcset(e.fd, &e.oldState)
}

func (e *replEditor) readLine(prompt string) (string, error) {
	buf := make([]rune, 0, 128)
	cursor := 0
	e.historyPos = len(e.history)
	render := func() {
		_, _ = fmt.Fprint(e.out, "\r\033[2K")
		line := string(buf)
		styledPrompt := stylePrompt(prompt)
		if strings.HasPrefix(prompt, "..") {
			styledPrompt = styleContinuationPrompt(prompt)
		}
		_, _ = fmt.Fprint(e.out, styledPrompt, colorizeInputLine(line))
		if cursor == len(buf) {
			prefix, _, _, ok := currentToken(buf, cursor)
			if ok && prefix != "" {
				if suggestion, ok := firstCompletion(prefix, e.candidates); ok && suggestion != prefix {
					suffix := suggestion[len(prefix):]
					_, _ = fmt.Fprint(e.out, paint(suffix, colorGray))
				}
			}
		}
		_, _ = fmt.Fprint(e.out, "\r")
		_, _ = fmt.Fprintf(e.out, "\033[%dC", len([]rune(prompt))+cursor)
	}

	render()
	var b [1]byte
	for {
		_, err := e.in.Read(b[:])
		if err != nil {
			return "", err
		}
		ch := b[0]
		switch ch {
		case '\r', '\n':
			_, _ = fmt.Fprint(e.out, "\r\n")
			line := string(buf)
			if strings.TrimSpace(line) != "" {
				if len(e.history) == 0 || e.history[len(e.history)-1] != line {
					e.history = append(e.history, line)
				}
			}
			return line, nil
		case 3: // Ctrl+C
			buf = buf[:0]
			cursor = 0
			_, _ = fmt.Fprint(e.out, "^C\r\n")
			return "", nil
		case 4: // Ctrl+D
			if len(buf) == 0 {
				_, _ = fmt.Fprint(e.out, "\r\n")
				return "", io.EOF
			}
		case 1: // Ctrl+A
			cursor = 0
		case 5: // Ctrl+E
			cursor = len(buf)
		case 9: // Tab
			buf, cursor = e.applyCompletion(buf, cursor, prompt)
		case 127, 8: // backspace
			if cursor > 0 {
				buf = append(buf[:cursor-1], buf[cursor:]...)
				cursor--
			}
		case 27: // escape sequence
			switch e.readEscapeAction() {
			case keyUp:
				if len(e.history) > 0 && e.historyPos > 0 {
					e.historyPos--
					buf = []rune(e.history[e.historyPos])
					cursor = len(buf)
				}
			case keyDown:
				if len(e.history) > 0 && e.historyPos < len(e.history)-1 {
					e.historyPos++
					buf = []rune(e.history[e.historyPos])
					cursor = len(buf)
				} else if e.historyPos >= len(e.history)-1 {
					e.historyPos = len(e.history)
					buf = buf[:0]
					cursor = 0
				}
			case keyRight:
				if cursor < len(buf) {
					cursor++
				}
			case keyLeft:
				if cursor > 0 {
					cursor--
				}
			case keyHome:
				cursor = 0
			case keyEnd:
				cursor = len(buf)
			case keyDelete:
				if cursor < len(buf) {
					buf = append(buf[:cursor], buf[cursor+1:]...)
				}
			}
		default:
			if ch >= 32 {
				r := rune(ch)
				if cursor == len(buf) {
					buf = append(buf, r)
				} else {
					buf = append(buf[:cursor], append([]rune{r}, buf[cursor:]...)...)
				}
				cursor++
			}
		}
		render()
	}
}

func (e *replEditor) readEscapeAction() keyAction {
	var b [1]byte
	if _, err := e.in.Read(b[:]); err != nil {
		return keyUnknown
	}

	switch b[0] {
	case '[':
		return e.readCSIAction()
	case 'O':
		// SS3 sequences: arrows/home/end in many terminals.
		if _, err := e.in.Read(b[:]); err != nil {
			return keyUnknown
		}
		switch b[0] {
		case 'A':
			return keyUp
		case 'B':
			return keyDown
		case 'C':
			return keyRight
		case 'D':
			return keyLeft
		case 'H':
			return keyHome
		case 'F':
			return keyEnd
		default:
			return keyUnknown
		}
	default:
		return keyUnknown
	}
}

func (e *replEditor) readCSIAction() keyAction {
	var b [1]byte
	if _, err := e.in.Read(b[:]); err != nil {
		return keyUnknown
	}

	// Simple one-byte CSI forms.
	switch b[0] {
	case 'A':
		return keyUp
	case 'B':
		return keyDown
	case 'C':
		return keyRight
	case 'D':
		return keyLeft
	case 'H':
		return keyHome
	case 'F':
		return keyEnd
	}

	// Extended forms: e.g. [1~, [4~, [7~, [8~, [3~, [1;5C
	seq := []byte{b[0]}
	for {
		if _, err := e.in.Read(b[:]); err != nil {
			break
		}
		seq = append(seq, b[0])
		// Final CSI byte is usually letter or '~'.
		if (b[0] >= 'A' && b[0] <= 'Z') || (b[0] >= 'a' && b[0] <= 'z') || b[0] == '~' {
			break
		}
		if len(seq) >= 8 {
			break
		}
	}
	s := string(seq)

	switch {
	case strings.HasSuffix(s, "A"):
		return keyUp
	case strings.HasSuffix(s, "B"):
		return keyDown
	case strings.HasSuffix(s, "C"):
		return keyRight
	case strings.HasSuffix(s, "D"):
		return keyLeft
	case strings.HasSuffix(s, "H"):
		return keyHome
	case strings.HasSuffix(s, "F"):
		return keyEnd
	case strings.HasPrefix(s, "1~"), strings.HasPrefix(s, "7~"):
		return keyHome
	case strings.HasPrefix(s, "4~"), strings.HasPrefix(s, "8~"):
		return keyEnd
	case strings.HasPrefix(s, "3~"):
		return keyDelete
	default:
		return keyUnknown
	}
}

func (e *replEditor) applyCompletion(buf []rune, cursor int, prompt string) ([]rune, int) {
	prefix, start, end, ok := currentToken(buf, cursor)
	if !ok || prefix == "" {
		return buf, cursor
	}
	matches := findCompletions(prefix, e.candidates)
	if len(matches) == 0 {
		return buf, cursor
	}
	if len(matches) == 1 {
		completion := []rune(matches[0])
		newBuf := append([]rune{}, buf[:start]...)
		newBuf = append(newBuf, completion...)
		newBuf = append(newBuf, buf[end:]...)
		return newBuf, start + len(completion)
	}

	common := longestCommonPrefix(matches)
	if len(common) > len(prefix) {
		completion := []rune(common)
		newBuf := append([]rune{}, buf[:start]...)
		newBuf = append(newBuf, completion...)
		newBuf = append(newBuf, buf[end:]...)
		return newBuf, start + len(completion)
	}

	_, _ = fmt.Fprint(e.out, "\r\n")
	for _, m := range matches {
		_, _ = fmt.Fprintln(e.out, m)
	}
	if strings.HasPrefix(prompt, "..") {
		_, _ = fmt.Fprint(e.out, styleContinuationPrompt(prompt))
	} else {
		_, _ = fmt.Fprint(e.out, stylePrompt(prompt))
	}
	return buf, cursor
}

func currentToken(buf []rune, cursor int) (prefix string, start int, end int, ok bool) {
	if cursor < 0 || cursor > len(buf) {
		return "", 0, 0, false
	}
	start = cursor
	for start > 0 && isTokenRune(buf[start-1]) {
		start--
	}
	end = cursor
	for end < len(buf) && isTokenRune(buf[end]) {
		end++
	}
	if start == end {
		return "", 0, 0, false
	}
	return string(buf[start:cursor]), start, end, true
}

func isTokenRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == ':'
}

func findCompletions(prefix string, candidates []string) []string {
	out := make([]string, 0, 8)
	for _, c := range candidates {
		if strings.HasPrefix(c, prefix) {
			out = append(out, c)
		}
	}
	return out
}

func firstCompletion(prefix string, candidates []string) (string, bool) {
	for _, c := range candidates {
		if strings.HasPrefix(c, prefix) {
			return c, true
		}
	}
	return "", false
}

func longestCommonPrefix(items []string) string {
	if len(items) == 0 {
		return ""
	}
	prefix := items[0]
	for _, s := range items[1:] {
		for !strings.HasPrefix(s, prefix) {
			if prefix == "" {
				return ""
			}
			prefix = prefix[:len(prefix)-1]
		}
	}
	return prefix
}

func tcget(fd int) (syscall.Termios, error) {
	var state syscall.Termios
	_, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TCGETS), uintptr(unsafe.Pointer(&state)), 0, 0, 0)
	if errno != 0 {
		return state, errno
	}
	return state, nil
}

func tcset(fd int, state *syscall.Termios) error {
	_, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TCSETS), uintptr(unsafe.Pointer(state)), 0, 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
}
