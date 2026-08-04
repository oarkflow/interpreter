package lua

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
	for i := 0; i < s.top; i++ {
		s.markValue(s.stack[i].value)
	}
	for i := range s.frames {
		for j := range s.frames[i].regs {
			s.markValue(s.frames[i].regs[j].value)
		}
		for _, v := range s.frames[i].varargs {
			s.markValue(v)
		}
		for j := 0; j < s.frames[i].multi.count; j++ {
			s.markValue(s.frames[i].multi.at(j))
		}
	}
	live := 0
	for _, t := range s.heap {
		if t.pooled {
			continue
		}
		if t.marked || t.escaped {
			t.marked = false
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
			for j := range thread.state.frames[i].regs {
				s.markValue(thread.state.frames[i].regs[j].value)
			}
		}
	}
}
func (s *State) markTable(t *Table) {
	if t == nil || t.marked {
		return
	}
	t.marked = true
	if t.meta != nil {
		s.markTable(t.meta)
	}
	for _, v := range t.array {
		s.markValue(v)
	}
	for _, field := range t.fields {
		s.markValue(field.value)
	}
	for _, v := range t.str {
		s.markValue(v)
	}
	for k, v := range t.other {
		s.markValue(tableKeyValue(k))
		s.markValue(v)
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
