package lua

import (
	"bufio"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
)

type luaFile struct {
	file       *os.File
	reader     *bufio.Reader
	writer     *bufio.Writer
	lineBuffer bool
	closeable  bool
	closed     bool
}

func (file *luaFile) writeString(value string) (int, error) {
	if file.closed {
		return 0, runtimeError("attempt to use a closed file")
	}
	if file.writer == nil {
		return io.WriteString(file.file, value)
	}
	n, err := file.writer.WriteString(value)
	if err == nil && file.lineBuffer && strings.Contains(value, "\n") {
		err = file.writer.Flush()
	}
	return n, err
}

func (file *luaFile) flush() error {
	if file.closed {
		return runtimeError("attempt to use a closed file")
	}
	if file.writer != nil {
		if err := file.writer.Flush(); err != nil {
			return err
		}
	}
	return file.file.Sync()
}

func newLuaFile(file *os.File, closeable bool) *luaFile {
	return &luaFile{file: file, reader: bufio.NewReader(file), closeable: closeable}
}

func fileFromValue(value Value) (*luaFile, error) {
	if value.kind != UserdataKind {
		return nil, runtimeError("file expected")
	}
	file, ok := value.UserdataValue().(*luaFile)
	if !ok || file == nil {
		return nil, runtimeError("file expected")
	}
	if file.closed {
		return nil, runtimeError("attempt to use a closed file")
	}
	return file, nil
}

func ioFailure(err error) ([]Value, error) {
	if err == nil {
		return []Value{True}, nil
	}
	return []Value{Nil, String(err.Error()), Number(1)}, nil
}

func (s *State) openIOLibrary() {
	methods := NewTable(0, 12)
	meta := NewTable(0, 3)
	meta.SetString("__index", TableValue(methods))
	meta.SetString("__tostring", Native(func(_ *State, args []Value) ([]Value, error) {
		file, err := fileFromValue(args[0])
		if err != nil {
			return []Value{String("file (closed)")}, nil
		}
		return []Value{String("file (" + file.file.Name() + ")")}, nil
	}))
	wrapped := make(map[*luaFile]Value)
	wrap := func(file *luaFile) Value {
		if value, ok := wrapped[file]; ok {
			return value
		}
		value := UserdataWithMetatable(file, meta)
		wrapped[file] = value
		return value
	}

	methods.SetString("close", Native(func(_ *State, args []Value) ([]Value, error) {
		file, err := fileFromValue(firstValue(args))
		if err != nil {
			return nil, err
		}
		if !file.closeable {
			return []Value{Nil, String("cannot close standard file")}, nil
		}
		if file.writer != nil {
			err = file.writer.Flush()
		}
		if err == nil {
			err = file.file.Close()
		}
		if err == nil {
			file.closed = true
		}
		return ioFailure(err)
	}))
	methods.SetString("flush", Native(func(_ *State, args []Value) ([]Value, error) {
		file, err := fileFromValue(firstValue(args))
		if err != nil {
			return nil, err
		}
		return ioFailure(file.flush())
	}))
	methods.SetString("write", Native(func(_ *State, args []Value) ([]Value, error) {
		file, err := fileFromValue(firstValue(args))
		if err != nil {
			return nil, err
		}
		for _, value := range args[1:] {
			if value.kind != StringKind && value.kind != NumberKind {
				return nil, runtimeError("string expected")
			}
			if _, err = file.writeString(valueString(value)); err != nil {
				return ioFailure(err)
			}
		}
		return []Value{args[0]}, nil
	}))
	methods.SetString("seek", Native(func(_ *State, args []Value) ([]Value, error) {
		file, err := fileFromValue(firstValue(args))
		if err != nil {
			return nil, err
		}
		whence, offset := "cur", int64(0)
		if len(args) > 1 && args[1].kind != NilKind {
			whence, err = needString(args, 1)
			if err != nil {
				return nil, err
			}
		}
		if len(args) > 2 {
			n, numberErr := needNumber(args, 2)
			if numberErr != nil {
				return nil, numberErr
			}
			offset = int64(n)
		}
		origin := map[string]int{"set": io.SeekStart, "cur": io.SeekCurrent, "end": io.SeekEnd}[whence]
		if _, ok := map[string]int{"set": 0, "cur": 1, "end": 2}[whence]; !ok {
			return nil, runtimeError("invalid option '%s'", whence)
		}
		// Account for bytes buffered ahead when seeking relative to current.
		if origin == io.SeekCurrent {
			offset -= int64(file.reader.Buffered())
		}
		if file.writer != nil {
			if err := file.writer.Flush(); err != nil {
				return ioFailure(err)
			}
		}
		position, seekErr := file.file.Seek(offset, origin)
		if seekErr != nil {
			return ioFailure(seekErr)
		}
		file.reader.Reset(file.file)
		return []Value{Number(float64(position))}, nil
	}))
	methods.SetString("read", Native(func(_ *State, args []Value) ([]Value, error) {
		file, err := fileFromValue(firstValue(args))
		if err != nil {
			return nil, err
		}
		return file.read(args[1:])
	}))
	methods.SetString("lines", Native(func(_ *State, args []Value) ([]Value, error) {
		file, err := fileFromValue(firstValue(args))
		if err != nil {
			return nil, err
		}
		return []Value{file.lines(false)}, nil
	}))
	methods.SetString("setvbuf", Native(func(_ *State, args []Value) ([]Value, error) {
		file, err := fileFromValue(firstValue(args))
		if err != nil {
			return nil, err
		}
		mode, err := needString(args, 1)
		if err != nil {
			return nil, err
		}
		size := 8192
		if len(args) > 2 {
			n, numberErr := needNumber(args, 2)
			if numberErr != nil {
				return nil, numberErr
			}
			size = int(n)
		}
		if file.writer != nil {
			if err = file.writer.Flush(); err != nil {
				return ioFailure(err)
			}
		}
		switch mode {
		case "no":
			file.writer, file.lineBuffer = nil, false
		case "full":
			file.writer, file.lineBuffer = bufio.NewWriterSize(file.file, size), false
		case "line":
			file.writer, file.lineBuffer = bufio.NewWriterSize(file.file, size), true
		default:
			return nil, runtimeError("invalid option '%s'", mode)
		}
		return []Value{True}, nil
	}))

	stdin, stdout, stderr := newLuaFile(os.Stdin, false), newLuaFile(os.Stdout, false), newLuaFile(os.Stderr, false)
	input, output := stdin, stdout
	lib := NewTable(0, 20)
	lib.SetString("stdin", wrap(stdin))
	lib.SetString("stdout", wrap(stdout))
	lib.SetString("stderr", wrap(stderr))
	lib.SetString("open", Native(func(_ *State, args []Value) ([]Value, error) {
		name, err := needString(args, 0)
		if err != nil {
			return nil, err
		}
		mode := "r"
		if len(args) > 1 {
			mode, err = needString(args, 1)
			if err != nil {
				return nil, err
			}
		}
		file, openErr := openLuaFile(name, mode)
		if openErr != nil {
			return ioFailure(openErr)
		}
		return []Value{wrap(newLuaFile(file, true))}, nil
	}))
	lib.SetString("tmpfile", Native(func(_ *State, _ []Value) ([]Value, error) {
		file, err := os.CreateTemp("", "lua-*")
		if err != nil {
			return ioFailure(err)
		}
		return []Value{wrap(newLuaFile(file, true))}, nil
	}))
	lib.SetString("type", Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) == 0 || args[0].kind != UserdataKind {
			return []Value{Nil}, nil
		}
		file, ok := args[0].UserdataValue().(*luaFile)
		if !ok {
			return []Value{Nil}, nil
		}
		if file.closed {
			return []Value{String("closed file")}, nil
		}
		return []Value{String("file")}, nil
	}))
	lib.SetString("input", Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) > 0 {
			file, err := resolveIOFile(args[0], "r", wrap)
			if err != nil {
				return nil, err
			}
			input = file
		}
		return []Value{wrap(input)}, nil
	}))
	lib.SetString("output", Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) > 0 {
			file, err := resolveIOFile(args[0], "w", wrap)
			if err != nil {
				return nil, err
			}
			output = file
		}
		return []Value{wrap(output)}, nil
	}))
	lib.SetString("read", Native(func(_ *State, args []Value) ([]Value, error) { return input.read(args) }))
	lib.SetString("write", Native(func(state *State, args []Value) ([]Value, error) {
		if state.Output != nil {
			for _, value := range args {
				if _, err := io.WriteString(state.Output, valueString(value)); err != nil {
					return ioFailure(err)
				}
			}
			return []Value{True}, nil
		}
		if output.closed {
			return nil, runtimeError("attempt to use a closed file")
		}
		for _, value := range args {
			if _, err := output.writeString(valueString(value)); err != nil {
				return ioFailure(err)
			}
		}
		return []Value{True}, nil
	}))
	lib.SetString("close", Native(func(state *State, args []Value) ([]Value, error) {
		target := wrap(output)
		if len(args) > 0 {
			target = args[0]
		}
		result, err := state.callValue(methods.GetString("close"), []Value{target})
		if err != nil {
			return nil, err
		}
		return result.slice(), nil
	}))
	lib.SetString("flush", Native(func(_ *State, _ []Value) ([]Value, error) { return ioFailure(output.flush()) }))
	lib.SetString("lines", Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) == 0 {
			return []Value{input.lines(false)}, nil
		}
		name, err := needString(args, 0)
		if err != nil {
			return nil, err
		}
		file, err := os.Open(name)
		if err != nil {
			return nil, err
		}
		return []Value{newLuaFile(file, true).lines(true)}, nil
	}))
	s.globals.SetString("io", TableValue(lib))
}

func firstValue(args []Value) Value {
	if len(args) == 0 {
		return Nil
	}
	return args[0]
}

func openLuaFile(name, mode string) (*os.File, error) {
	flags := 0
	switch mode {
	case "r", "rb":
		flags = os.O_RDONLY
	case "w", "wb":
		flags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	case "a", "ab":
		flags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	case "r+", "r+b", "rb+":
		flags = os.O_RDWR
	case "w+", "w+b", "wb+":
		flags = os.O_CREATE | os.O_RDWR | os.O_TRUNC
	case "a+", "a+b", "ab+":
		flags = os.O_CREATE | os.O_RDWR | os.O_APPEND
	default:
		return nil, runtimeError("invalid mode '%s'", mode)
	}
	return os.OpenFile(name, flags, 0o666)
}

func resolveIOFile(value Value, mode string, wrap func(*luaFile) Value) (*luaFile, error) {
	_ = wrap
	if value.kind == StringKind {
		file, err := openLuaFile(value.StringValue(), mode)
		if err != nil {
			return nil, err
		}
		return newLuaFile(file, true), nil
	}
	return fileFromValue(value)
}

func (file *luaFile) lines(closeAtEnd bool) Value {
	return Native(func(_ *State, _ []Value) ([]Value, error) {
		values, err := file.read(nil)
		if err != nil {
			return nil, err
		}
		if len(values) == 0 || values[0].kind == NilKind {
			if closeAtEnd && !file.closed {
				_ = file.file.Close()
				file.closed = true
			}
			return nil, nil
		}
		return values, nil
	})
}

func (file *luaFile) read(formats []Value) ([]Value, error) {
	if file.closed {
		return nil, runtimeError("attempt to use a closed file")
	}
	if len(formats) == 0 {
		formats = []Value{String("*l")}
	}
	results := make([]Value, 0, len(formats))
	for _, format := range formats {
		value, err := file.readOne(format)
		if err != nil {
			return nil, err
		}
		results = append(results, value)
		if value.kind == NilKind {
			break
		}
	}
	return results, nil
}

func (file *luaFile) readOne(format Value) (Value, error) {
	if format.kind == NumberKind {
		count := int(format.Number())
		if count < 0 {
			return Nil, runtimeError("invalid format")
		}
		if count == 0 {
			if _, err := file.reader.Peek(1); err != nil {
				return Nil, nil
			}
			return String(""), nil
		}
		buffer := make([]byte, count)
		n, err := io.ReadFull(file.reader, buffer)
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
			return Nil, err
		}
		if n == 0 {
			return Nil, nil
		}
		return String(string(buffer[:n])), nil
	}
	option, err := needString([]Value{format}, 0)
	if err != nil {
		return Nil, err
	}
	switch option {
	case "*a", "*all":
		data, err := io.ReadAll(file.reader)
		if err != nil {
			return Nil, err
		}
		return String(string(data)), nil
	case "*l", "*line":
		line, err := file.reader.ReadString('\n')
		if len(line) == 0 && err != nil {
			return Nil, nil
		}
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")
		return String(line), nil
	case "*n", "*number":
		var token strings.Builder
		for {
			char, err := file.reader.ReadByte()
			if err != nil {
				return Nil, nil
			}
			if !strings.ContainsRune(" \t\r\n\f\v", rune(char)) {
				token.WriteByte(char)
				break
			}
		}
		for {
			char, err := file.reader.ReadByte()
			if err != nil {
				break
			}
			if strings.ContainsRune(" \t\r\n\f\v", rune(char)) {
				_ = file.reader.UnreadByte()
				break
			}
			token.WriteByte(char)
		}
		number, err := strconv.ParseFloat(token.String(), 64)
		if err != nil {
			return Nil, nil
		}
		return Number(number), nil
	default:
		return Nil, runtimeError("invalid format")
	}
}
