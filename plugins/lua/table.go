package lua

import (
	"math"
	"strconv"
	"strings"
	"unsafe"
)

// Table is a hybrid array/hash table. Dense positive integer keys avoid
// hashing; strings have a dedicated map because they dominate object fields.
// The generic map handles the uncommon boolean/reference/non-dense keys.
type Table struct {
	array []Value
	// inline stores the overwhelmingly common one- and two-element arrays
	// inside the table itself. This avoids a second Go heap allocation for
	// tuple-like Lua tables while array transparently grows for larger values.
	inline         [2]Value
	fields         []fieldEntry
	shape          *tableShape
	str            map[string]Value
	other          map[tableKey]Value
	meta           *Table
	order          []tableKey
	markGeneration uint64
	escaped        bool
	pooled         bool
}

type fieldEntry struct {
	key   string
	value Value
}

type tableShape struct{ keys []string }
type fieldCache struct {
	shape *tableShape
	slot  int
}

func (t *Table) reset(arrayHint, hashHint int) {
	if arrayHint <= len(t.inline) {
		t.inline[0], t.inline[1] = Nil, Nil
		t.array = t.inline[:0]
	} else if cap(t.array) >= arrayHint {
		clear(t.array)
		t.array = t.array[:0]
	} else {
		t.array = make([]Value, 0, arrayHint)
	}
	if t.str != nil {
		clear(t.str)
	}
	t.fields = t.fields[:0]
	t.shape = nil
	if hashHint > 8 {
		if t.str == nil {
			t.str = make(map[string]Value, hashHint)
		}
	} else {
		t.str = nil
		if cap(t.fields) < hashHint {
			t.fields = make([]fieldEntry, 0, hashHint)
		}
	}
	if t.other != nil {
		clear(t.other)
	}
	t.meta = nil
	t.order = t.order[:0]
	t.markGeneration = 0
	t.escaped = false
	t.pooled = false
}

type tableKey struct {
	kind Kind
	n    uint64
	s    string
	ptr  unsafe.Pointer
}

func NewTable(arrayHint, hashHint int) *Table {
	t := &Table{}
	t.reset(arrayHint, hashHint)
	return t
}

func (t *Table) Len() int {
	if t == nil {
		return 0
	}
	n := len(t.array)
	for n > 0 && t.array[n-1].kind == NilKind {
		n--
	}
	return n
}
func (t *Table) Metatable() *Table {
	if t == nil {
		return nil
	}
	return t.meta
}
func (t *Table) SetMetatable(meta *Table) {
	if t != nil {
		t.meta = meta
	}
}

func (t *Table) Get(key Value) Value {
	if t == nil {
		return Nil
	}
	switch key.kind {
	case StringKind:
		name := key.StringValue()
		for i := range t.fields {
			if t.fields[i].key == name {
				return t.fields[i].value
			}
		}
		if t.str != nil {
			if v, ok := t.str[name]; ok {
				return v
			}
		}
	case NumberKind:
		if i, ok := numberKey(key.Number()); ok && i >= 1 && i <= int64(len(t.array)) {
			return t.array[i-1]
		}
	}
	if t.other != nil {
		if k, ok := makeTableKey(key); ok {
			if v, exists := t.other[k]; exists {
				return v
			}
		}
	}
	return Nil
}

func (t *Table) getDenseNumber(n float64) (Value, bool) {
	i := int(n)
	if i >= 1 && i <= len(t.array) && float64(i) == n {
		return t.array[i-1], true
	}
	return Nil, false
}

func (t *Table) setDenseNumber(n float64, value Value) bool {
	i := int(n)
	if i < 1 || float64(i) != n {
		return false
	}
	if i <= len(t.array) {
		t.array[i-1] = value
		return true
	}
	return t.appendDenseNumber(i, value)
}

func (t *Table) appendDenseNumber(i int, value Value) bool {
	if i == len(t.array)+1 {
		t.array = append(t.array, value)
		if t.other != nil {
			t.promoteDenseTail()
		}
		return true
	}
	return false
}

func (t *Table) Set(key, value Value) error {
	if key.kind == NilKind {
		return runtimeError("table index is nil")
	}
	if key.kind == NumberKind && math.IsNaN(key.Number()) {
		return runtimeError("table index is NaN")
	}
	switch key.kind {
	case StringKind:
		name := key.StringValue()
		for i := range t.fields {
			if t.fields[i].key == name {
				t.fields[i].value = value
				return nil
			}
		}
		if value.kind == NilKind {
			delete(t.str, name)
			return nil
		}
		if t.str == nil && len(t.fields) < 8 {
			t.fields = append(t.fields, fieldEntry{key: name, value: value})
			t.shape = nil
			t.order = append(t.order, tableKey{kind: StringKind, s: name})
			return nil
		}
		if t.str == nil {
			t.str = make(map[string]Value, 8)
			for _, field := range t.fields {
				if field.value.kind != NilKind {
					t.str[field.key] = field.value
				}
			}
			t.fields = t.fields[:0]
			t.shape = nil
		}
		if _, exists := t.str[name]; !exists {
			t.order = append(t.order, tableKey{kind: StringKind, s: name})
		}
		t.str[name] = value
		return nil
	case NumberKind:
		if i, ok := numberKey(key.Number()); ok && i >= 1 {
			if i <= int64(len(t.array)) {
				t.array[i-1] = value
				return nil
			}
			if i == int64(len(t.array)+1) {
				t.array = append(t.array, value)
				t.promoteDenseTail()
				return nil
			}
		}
	}
	k, ok := makeTableKey(key)
	if !ok {
		return runtimeError("invalid table key type %s", key.TypeName())
	}
	if value.kind == NilKind {
		delete(t.other, k)
		return nil
	}
	if t.other == nil {
		t.other = make(map[tableKey]Value, 8)
	}
	if _, exists := t.other[k]; !exists {
		t.order = append(t.order, k)
	}
	t.other[k] = value
	return nil
}

func (t *Table) GetString(name string) Value {
	if t == nil {
		return Nil
	}
	for i := range t.fields {
		if t.fields[i].key == name {
			return t.fields[i].value
		}
	}
	if t.str == nil {
		return Nil
	}
	return t.str[name]
}

func (t *Table) SetString(name string, value Value) {
	for i := range t.fields {
		if t.fields[i].key == name {
			t.fields[i].value = value
			return
		}
	}
	if value.kind == NilKind {
		delete(t.str, name)
		return
	}
	if t.str == nil && len(t.fields) < 8 {
		t.fields = append(t.fields, fieldEntry{key: name, value: value})
		t.shape = nil
		t.order = append(t.order, tableKey{kind: StringKind, s: name})
		return
	}
	if t.str == nil {
		t.str = make(map[string]Value, 8)
		for _, field := range t.fields {
			if field.value.kind != NilKind {
				t.str[field.key] = field.value
			}
		}
		t.fields = t.fields[:0]
		t.shape = nil
	}
	if _, exists := t.str[name]; !exists {
		t.order = append(t.order, tableKey{kind: StringKind, s: name})
	}
	t.str[name] = value
}

func (s *State) shapeFor(t *Table) *tableShape {
	if t == nil || t.str != nil || len(t.fields) == 0 {
		return nil
	}
	if t.shape != nil {
		return t.shape
	}
	var signature strings.Builder
	for _, field := range t.fields {
		signature.WriteString(strconv.Itoa(len(field.key)))
		signature.WriteByte(':')
		signature.WriteString(field.key)
		signature.WriteByte(';')
	}
	key := signature.String()
	if s.shapes == nil {
		s.shapes = make(map[string]*tableShape)
	}
	shape := s.shapes[key]
	if shape == nil {
		shape = &tableShape{keys: make([]string, len(t.fields))}
		for i := range t.fields {
			shape.keys[i] = t.fields[i].key
		}
		s.shapes[key] = shape
	}
	t.shape = shape
	return shape
}

func (s *State) cachedField(p *Prototype, pc int, t *Table, key string) (int, bool) {
	shape := s.shapeFor(t)
	if shape == nil {
		return 0, false
	}
	cache := &p.FieldCaches[pc]
	if cache.shape == shape {
		return cache.slot, true
	}
	for i, name := range shape.keys {
		if name == key {
			cache.shape = shape
			cache.slot = i
			return i, true
		}
	}
	return 0, false
}

func (t *Table) Next(previous Value) (Value, Value, bool) {
	if t == nil {
		return Nil, Nil, false
	}
	startArray := 0
	orderStart := 0
	if previous.kind != NilKind {
		if previous.kind == NumberKind {
			if i, ok := numberKey(previous.Number()); ok && i >= 1 && i <= int64(len(t.array)) {
				startArray = int(i)
				goto arraysDone
			}
		}
		startArray = len(t.array)
		if pk, ok := makeTableKey(previous); ok {
			for i, k := range t.order {
				if k == pk {
					orderStart = i + 1
					break
				}
			}
		}
	}
arraysDone:
	for i := startArray; i < len(t.array); i++ {
		if t.array[i].kind != NilKind {
			return Number(float64(i + 1)), t.array[i], true
		}
	}
	for i := orderStart; i < len(t.order); i++ {
		k := t.order[i]
		v := Nil
		if k.kind == StringKind {
			v = t.GetString(k.s)
		} else if t.other != nil {
			v = t.other[k]
		}
		if v.kind != NilKind {
			return tableKeyValue(k), v, true
		}
	}
	return Nil, Nil, false
}

// ForEach visits live entries. Iteration order is intentionally unspecified,
// matching Lua's next/pairs contract.
func (t *Table) ForEach(fn func(Value, Value) bool) {
	if t == nil {
		return
	}
	for i, v := range t.array {
		if v.kind != NilKind && !fn(Number(float64(i+1)), v) {
			return
		}
	}
	for _, field := range t.fields {
		if field.value.kind != NilKind && !fn(String(field.key), field.value) {
			return
		}
	}
	for k, v := range t.str {
		if !fn(String(k), v) {
			return
		}
	}
	for k, v := range t.other {
		if !fn(tableKeyValue(k), v) {
			return
		}
	}
}

func tableKeyValue(k tableKey) Value {
	switch k.kind {
	case BoolKind:
		return Bool(k.n != 0)
	case NumberKind:
		return Number(math.Float64frombits(k.n))
	case StringKind:
		return String(k.s)
	default:
		return Value{kind: k.kind, ptr: k.ptr}
	}
}

func (t *Table) promoteDenseTail() {
	for {
		k := tableKey{kind: NumberKind, n: math.Float64bits(float64(len(t.array) + 1))}
		v, ok := t.other[k]
		if !ok {
			return
		}
		delete(t.other, k)
		t.array = append(t.array, v)
	}
}

func makeTableKey(v Value) (tableKey, bool) {
	switch v.kind {
	case BoolKind:
		if v.Bool() {
			return tableKey{kind: BoolKind, n: 1}, true
		}
		return tableKey{kind: BoolKind}, true
	case NumberKind:
		return tableKey{kind: NumberKind, n: math.Float64bits(v.Number())}, true
	case StringKind:
		return tableKey{kind: StringKind, s: v.StringValue()}, true
	case TableKind, FunctionKind, UserdataKind, ThreadKind:
		return tableKey{kind: v.kind, ptr: v.ptr}, true
	default:
		return tableKey{}, false
	}
}
