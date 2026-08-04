package lua

import (
	"fmt"
	"sync"
)

// Script owns a persistent VM and an optional module table returned by its
// chunk. Calls are serialized because a State intentionally reuses stacks.
type Script struct {
	mu     sync.Mutex
	State  *State
	Module Value
}

func LoadScript(source, name string, globals map[string]Value) (*Script, []Value, error) {
	state := NewState()
	for k, v := range globals {
		state.SetGlobal(k, v)
	}
	fn, err := state.Load(source, name)
	if err != nil {
		return nil, nil, err
	}
	results, err := state.Call(fn)
	if err != nil {
		return nil, nil, err
	}
	s := &Script{State: state}
	if len(results) > 0 && results[0].kind == TableKind {
		s.Module = results[0]
	}
	return s, results, nil
}

func (s *Script) Call(name string, args ...Value) ([]Value, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn := Nil
	if s.Module.kind == TableKind {
		fn = s.Module.Table().GetString(name)
	}
	if fn.kind == NilKind {
		fn = s.State.GetGlobal(name)
	}
	if fn.kind != FunctionKind {
		return nil, runtimeError("function %q not found", name)
	}
	return s.State.Call(fn, args...)
}
func (s *Script) Get(name string) Value {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Module.kind == TableKind {
		if v := s.Module.Table().GetString(name); v.kind != NilKind {
			return v
		}
	}
	return s.State.GetGlobal(name)
}
func (s *Script) Set(name string, value Value) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.State.SetGlobal(name, value)
}
func (s *Script) Inspect() string { return fmt.Sprintf("<native Go Lua 5.1 script %p>", s) }
