// Package lua is a dependency-free Lua 5.1 implementation written in Go.
package lua

import (
	"fmt"
	"math"
	"strconv"
	"unsafe"
)

// Kind is deliberately byte-sized: Value is copied in every VM instruction.
type Kind uint8

const (
	NilKind Kind = iota
	BoolKind
	NumberKind
	StringKind
	TableKind
	FunctionKind
	UserdataKind
	ThreadKind
)

// Value is a compact tagged pair. bits stores numbers, booleans, or string
// length; ptr stores reference objects or string data. Keeping every Lua value
// to three machine words materially reduces register and table traffic.
type Value struct {
	kind Kind
	bits uint64
	ptr  unsafe.Pointer
}

var Nil = Value{}
var True = Value{kind: BoolKind, bits: 1}
var False = Value{kind: BoolKind}

func Bool(v bool) Value {
	if v {
		return True
	}
	return False
}
func Number(v float64) Value { return Value{kind: NumberKind, bits: math.Float64bits(v)} }
func String(v string) Value {
	return Value{kind: StringKind, bits: uint64(len(v)), ptr: unsafe.Pointer(unsafe.StringData(v))}
}
func TableValue(v *Table) Value { return Value{kind: TableKind, ptr: unsafe.Pointer(v)} }
func FunctionValue(v *Function) Value {
	return Value{kind: FunctionKind, ptr: unsafe.Pointer(v)}
}
func ThreadValue(v *Thread) Value { return Value{kind: ThreadKind, ptr: unsafe.Pointer(v)} }
func Userdata(v any) Value {
	return Value{kind: UserdataKind, ptr: unsafe.Pointer(&userdataBox{value: v})}
}

type userdataBox struct{ value any }

func (v Value) Kind() Kind      { return v.kind }
func (v Value) Bool() bool      { return v.bits != 0 }
func (v Value) Number() float64 { return math.Float64frombits(v.bits) }
func (v Value) StringValue() string {
	return unsafe.String((*byte)(v.ptr), int(v.bits))
}
func (v Value) Table() *Table {
	if v.kind != TableKind {
		return nil
	}
	return (*Table)(v.ptr)
}
func (v Value) Function() *Function {
	if v.kind != FunctionKind {
		return nil
	}
	return (*Function)(v.ptr)
}
func (v Value) Thread() *Thread {
	if v.kind != ThreadKind {
		return nil
	}
	return (*Thread)(v.ptr)
}
func (v Value) Interface() any {
	switch v.kind {
	case NilKind:
		return nil
	case BoolKind:
		return v.Bool()
	case NumberKind:
		return v.Number()
	case StringKind:
		return v.StringValue()
	case TableKind:
		return (*Table)(v.ptr)
	case FunctionKind:
		return (*Function)(v.ptr)
	case UserdataKind:
		if v.ptr == nil {
			return nil
		}
		return (*userdataBox)(v.ptr).value
	default:
		return v.ptr
	}
}

func (v Value) Truthy() bool { return v.kind != NilKind && (v.kind != BoolKind || v.bits != 0) }

func (v Value) TypeName() string {
	switch v.kind {
	case NilKind:
		return "nil"
	case BoolKind:
		return "boolean"
	case NumberKind:
		return "number"
	case StringKind:
		return "string"
	case TableKind:
		return "table"
	case FunctionKind:
		return "function"
	case UserdataKind:
		return "userdata"
	case ThreadKind:
		return "thread"
	default:
		return "unknown"
	}
}

func (v Value) Repr() string {
	switch v.kind {
	case NilKind:
		return "nil"
	case BoolKind:
		if v.Bool() {
			return "true"
		}
		return "false"
	case NumberKind:
		return strconv.FormatFloat(v.Number(), 'g', -1, 64)
	case StringKind:
		return v.StringValue()
	case TableKind:
		return fmt.Sprintf("table: %p", v.ptr)
	case FunctionKind:
		return fmt.Sprintf("function: %p", v.ptr)
	case UserdataKind:
		return fmt.Sprintf("userdata: %p", v.ptr)
	case ThreadKind:
		return fmt.Sprintf("thread: %p", v.ptr)
	default:
		return "<invalid>"
	}
}

func equal(a, b Value) bool {
	if a.kind != b.kind {
		return false
	}
	switch a.kind {
	case NilKind:
		return true
	case BoolKind:
		return a.bits == b.bits
	case NumberKind:
		return a.Number() == b.Number()
	case StringKind:
		return a.StringValue() == b.StringValue()
	default:
		return a.ptr == b.ptr
	}
}

func toNumber(v Value) (float64, bool) {
	if v.kind == NumberKind {
		return v.Number(), true
	}
	if v.kind == StringKind {
		n, err := strconv.ParseFloat(v.StringValue(), 64)
		return n, err == nil
	}
	return 0, false
}

func numberKey(n float64) (int64, bool) {
	if math.IsNaN(n) || math.IsInf(n, 0) || math.Trunc(n) != n || n < math.MinInt64 || n > math.MaxInt64 {
		return 0, false
	}
	return int64(n), true
}

type NativeFunction func(*State, []Value) ([]Value, error)

type Function struct {
	Proto          *Prototype
	Native         NativeFunction
	NativeNumber1  func(float64) float64
	NativeNumber2  func(float64, float64) float64
	Env            *Table
	Up             []*cell
	markGeneration uint64
}

type cell struct{ value Value }
