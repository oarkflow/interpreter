package lua

import "testing"

func TestWeakTableCollection(t *testing.T) {
	state := NewState()
	target := state.newTable(0, 0)
	weak := state.newTable(1, 0)
	meta := state.newTable(0, 1)
	meta.SetString("__mode", String("kv"))
	weak.SetMetatable(meta)
	if err := weak.Set(Number(1), TableValue(target)); err != nil {
		t.Fatal(err)
	}
	state.globals.SetString("weak", TableValue(weak))
	state.collectTables()
	if value := weak.Get(Number(1)); value.kind != NilKind {
		t.Fatalf("weak value survived collection: %s (target generation=%d escaped=%v)", value.Repr(), target.markGeneration, target.escaped)
	}
}

func TestWeakTableLuaRoots(t *testing.T) {
	state := NewState()
	state.SetGlobal("rootcount", Native(func(s *State, args []Value) ([]Value, error) {
		target := args[0].Table().Get(Number(1)).Table()
		count := 0
		for i := range s.frames {
			pc := s.frames[i].pc - 1
			live := int(s.frames[i].fn.Proto.LiveRegisters[pc])
			for j := 0; j < live; j++ {
				if s.frames[i].regs[j].value.kind == TableKind && s.frames[i].regs[j].value.Table() == target {
					count++
				}
			}
		}
		return []Value{Number(float64(count))}, nil
	}))
	results, err := state.DoString(`local x={[1]={}}; setmetatable(x,{__mode='kv'}); local n=rootcount(x); collectgarbage(); return n, x[1]`)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("roots=%v result=%#v", results[0].Number(), results[1])
}

func TestWeakTableAutomaticCollection(t *testing.T) {
	state := NewState()
	results, err := state.DoString(`
		local x={[1]={}}
		setmetatable(x,{__mode='kv'})
		for i=1,1000 do local garbage=i..i..i..i end
		return x[1]
	`)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].kind != NilKind {
		t.Fatalf("weak value survived automatic collection: %#v (enabled=%v ticks=%d)", results, state.weakTables, state.weakTicks)
	}
}
