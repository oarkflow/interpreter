package lua

import "strings"

const tableBlockSize = 1024

// newTable allocates from a state-local arena. Lua workloads commonly create
// millions of tiny temporary tables; recycling them avoids handing every one
// to Go's tracing collector.
func (s *State) newTable(arrayHint, hashHint int) *Table {
	if s.tableAllocations >= s.nextCollection {
		s.collectTables()
	}
	s.tableAllocations++
	if n := len(s.freeTables); n > 0 {
		t := s.freeTables[n-1]
		s.freeTables = s.freeTables[:n-1]
		t.reset(arrayHint, hashHint)
		return t
	}
	if len(s.tableBlocks) == 0 || s.tableBlockUsed == len(s.tableBlocks[len(s.tableBlocks)-1]) {
		s.tableBlocks = append(s.tableBlocks, make([]Table, tableBlockSize))
		s.tableBlockUsed = 0
	}
	block := s.tableBlocks[len(s.tableBlocks)-1]
	t := &block[s.tableBlockUsed]
	s.tableBlockUsed++
	t.reset(arrayHint, hashHint)
	s.heap = append(s.heap, t)
	return t
}

func (s *State) collectTables() {
	s.gcGeneration++
	s.markTable(s.globals)
	s.markTable(s.registry)
	for _, fn := range s.dumped {
		s.markValue(FunctionValue(fn))
	}
	for _, meta := range s.typeMetatables {
		s.markTable(meta)
	}
	for i := 0; i < s.top; i++ {
		s.markValue(s.stack[i].value)
	}
	for _, value := range s.hookRoots {
		s.markValue(value)
	}
	for i := range s.frames {
		s.markValue(FunctionValue(s.frames[i].fn))
		live := len(s.frames[i].regs)
		pc := s.frames[i].pc - 1
		if proto := s.frames[i].fn.Proto; proto != nil && pc >= 0 && pc < len(proto.LiveRegisters) {
			live = int(proto.LiveRegisters[pc])
			if live > len(s.frames[i].regs) {
				live = len(s.frames[i].regs)
			}
		}
		for j := 0; j < live; j++ {
			s.markValue(s.frames[i].regs[j].value)
		}
		for _, v := range s.frames[i].varargs {
			s.markValue(v)
		}
		for j := 0; j < s.frames[i].multi.count; j++ {
			s.markValue(s.frames[i].multi.at(j))
		}
	}
	for _, t := range s.heap {
		if !t.pooled && (t.markGeneration == s.gcGeneration || t.escaped) {
			s.clearDeadWeakEntries(t)
		}
	}
	live := 0
	for _, t := range s.heap {
		if t.pooled {
			continue
		}
		if t.markGeneration == s.gcGeneration || t.escaped {
			live++
			continue
		}
		t.reset(0, 0)
		t.pooled = true
		s.freeTables = append(s.freeTables, t)
	}
	s.tableAllocations = 0
	s.nextCollection = live * 2
	if s.nextCollection < 1024 {
		s.nextCollection = 1024
	}
}
func (s *State) markValue(v Value) {
	switch v.kind {
	case TableKind:
		s.markTable(v.Table())
	case FunctionKind:
		fn := v.Function()
		if fn.markGeneration == s.gcGeneration {
			return
		}
		fn.markGeneration = s.gcGeneration
		s.markTable(fn.Env)
		for _, up := range fn.Up {
			s.markValue(up.value)
		}
	case ThreadKind:
		thread := v.Thread()
		if thread == nil || thread.markGeneration == s.gcGeneration {
			return
		}
		thread.markGeneration = s.gcGeneration
		s.markValue(thread.fn)
		for i := 0; i < thread.state.top; i++ {
			s.markValue(thread.state.stack[i].value)
		}
		for i := range thread.state.frames {
			s.markValue(FunctionValue(thread.state.frames[i].fn))
			live := len(thread.state.frames[i].regs)
			pc := thread.state.frames[i].pc - 1
			if proto := thread.state.frames[i].fn.Proto; proto != nil && pc >= 0 && pc < len(proto.LiveRegisters) {
				live = int(proto.LiveRegisters[pc])
				if live > len(thread.state.frames[i].regs) {
					live = len(thread.state.frames[i].regs)
				}
			}
			for j := 0; j < live; j++ {
				s.markValue(thread.state.frames[i].regs[j].value)
			}
		}
	case UserdataKind:
		if v.ptr != nil {
			box := (*userdataBox)(v.ptr)
			if box.markGeneration == s.gcGeneration {
				return
			}
			box.markGeneration = s.gcGeneration
			s.markTable(box.meta)
		}
	}
}
func (s *State) markTable(t *Table) {
	if t == nil || t.markGeneration == s.gcGeneration {
		return
	}
	t.markGeneration = s.gcGeneration
	if t.meta != nil {
		s.markTable(t.meta)
	}
	weakKeys, weakValues := tableWeakMode(t)
	for _, v := range t.array {
		if !weakValues {
			s.markValue(v)
		}
	}
	for _, field := range t.fields {
		if !weakValues {
			s.markValue(field.value)
		}
	}
	for _, v := range t.str {
		if !weakValues {
			s.markValue(v)
		}
	}
	for k, v := range t.other {
		key := tableKeyValue(k)
		if !weakKeys {
			s.markValue(key)
		}
		if !weakValues && (!weakKeys || !collectableValue(key) || s.valueMarked(key)) {
			s.markValue(v)
		}
	}
}

func tableWeakMode(t *Table) (bool, bool) {
	if t == nil || t.meta == nil {
		return false, false
	}
	mode := t.meta.GetString("__mode")
	if mode.kind != StringKind {
		return false, false
	}
	text := mode.StringValue()
	return strings.ContainsRune(text, 'k'), strings.ContainsRune(text, 'v')
}

func collectableValue(value Value) bool {
	return value.kind == TableKind || value.kind == FunctionKind || value.kind == UserdataKind || value.kind == ThreadKind
}

func (s *State) valueMarked(value Value) bool {
	switch value.kind {
	case TableKind:
		return value.Table().markGeneration == s.gcGeneration || value.Table().escaped
	case FunctionKind:
		return value.Function().markGeneration == s.gcGeneration
	case UserdataKind:
		return value.ptr != nil && (*userdataBox)(value.ptr).markGeneration == s.gcGeneration
	case ThreadKind:
		return value.Thread() != nil && value.Thread().markGeneration == s.gcGeneration
	default:
		return true
	}
}

func (s *State) clearDeadWeakEntries(table *Table) {
	weakKeys, weakValues := tableWeakMode(table)
	if !weakKeys && !weakValues {
		return
	}
	keys := make([]Value, 0, table.Len())
	table.ForEach(func(key, value Value) bool {
		if weakKeys && collectableValue(key) && !s.valueMarked(key) || weakValues && collectableValue(value) && !s.valueMarked(value) {
			keys = append(keys, key)
		}
		return true
	})
	for _, key := range keys {
		_ = table.Set(key, Nil)
	}
}

func (s *State) maybeCollectWeakTables() {
	if !s.weakTables {
		return
	}
	s.weakTicks++
	if s.weakTicks >= 256 {
		s.weakTicks = 0
		s.collectTables()
	}
}
func (s *State) escapeValue(v Value, seen map[*Table]bool) {
	if v.kind != TableKind {
		return
	}
	t := v.Table()
	if seen[t] {
		return
	}
	seen[t] = true
	t.escaped = true
	if t.meta != nil {
		s.escapeValue(TableValue(t.meta), seen)
	}
	t.ForEach(func(_, value Value) bool { s.escapeValue(value, seen); return true })
}
