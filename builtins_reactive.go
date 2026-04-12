package interpreter

import (
	"fmt"
	"sync"
)

// ── Reactive Object Types ──────────────────────────────────────────

const (
	SIGNAL_OBJ   ObjectType = 106
	COMPUTED_OBJ  ObjectType = 107
	EFFECT_OBJ   ObjectType = 108
)

// ── Signal ─────────────────────────────────────────────────────────

type Signal struct {
	mu         sync.RWMutex
	value      Object
	subscribers []*Effect
	computeds  []*Computed
	name       string
}

func (s *Signal) Type() ObjectType { return SIGNAL_OBJ }
func (s *Signal) Inspect() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return fmt.Sprintf("<signal:%s=%s>", s.name, s.value.Inspect())
}

func (s *Signal) Get() Object {
	s.mu.RLock()
	val := s.value
	s.mu.RUnlock()
	// Track current effect/computed if any (outside lock to avoid deadlock)
	if currentEffect != nil {
		s.trackEffect(currentEffect)
	}
	if currentComputed != nil {
		s.trackComputed(currentComputed)
	}
	return val
}

func (s *Signal) Set(val Object) {
	s.mu.Lock()
	s.value = val
	// Copy subscribers to avoid holding lock during notifications
	effects := make([]*Effect, len(s.subscribers))
	copy(effects, s.subscribers)
	computeds := make([]*Computed, len(s.computeds))
	copy(computeds, s.computeds)
	s.mu.Unlock()

	// Invalidate computeds
	for _, c := range computeds {
		c.invalidate()
	}

	// Run effects
	for _, e := range effects {
		e.run()
	}
}

func (s *Signal) trackEffect(e *Effect) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sub := range s.subscribers {
		if sub == e {
			return
		}
	}
	s.subscribers = append(s.subscribers, e)
}

func (s *Signal) trackComputed(c *Computed) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, comp := range s.computeds {
		if comp == c {
			return
		}
	}
	s.computeds = append(s.computeds, c)
}

// GetSignalProperty provides dot-access for signals
func GetSignalProperty(sig *Signal, name string) Object {
	switch name {
	case "value":
		return sig.Get()
	case "set":
		return &Builtin{FnWithEnv: func(env *Environment, args ...Object) Object {
			if len(args) < 1 {
				return newError("signal.set() requires a value")
			}
			val := args[0]
			// Support updater function: signal.set(fn) where fn receives prev value
			switch fn := val.(type) {
			case *Function, *Builtin:
				prev := sig.Get()
				result := applyFunction(fn, []Object{prev}, env, nil)
				if isError(result) {
					return result
				}
				sig.Set(result)
			default:
				sig.Set(val)
			}
			return NULL
		}}
	case "get":
		return &Builtin{Fn: func(args ...Object) Object {
			return sig.Get()
		}}
	case "name":
		return &String{Value: sig.name}
	case "subscribe":
		return &Builtin{Fn: func(args ...Object) Object {
			if len(args) < 1 {
				return newError("subscribe() requires a callback")
			}
			// Create a lightweight effect
			e := &Effect{
				fn:  args[0],
				env: nil,
			}
			sig.mu.Lock()
			sig.subscribers = append(sig.subscribers, e)
			sig.mu.Unlock()
			return NULL
		}}
	default:
		return nil
	}
}

// ── Computed ───────────────────────────────────────────────────────

type Computed struct {
	mu       sync.Mutex
	fn       Object // computation function
	env      *Environment
	value    Object
	valid    bool
	deps     []*Signal
}

func (c *Computed) Type() ObjectType { return COMPUTED_OBJ }
func (c *Computed) Inspect() string {
	val := c.Get()
	return fmt.Sprintf("<computed=%s>", val.Inspect())
}

func (c *Computed) Get() Object {
	c.mu.Lock()
	if c.valid && c.value != nil {
		val := c.value
		c.mu.Unlock()
		return val
	}
	c.mu.Unlock()
	val := c.compute()
	c.mu.Lock()
	c.value = val
	c.valid = true
	c.mu.Unlock()
	return val
}

func (c *Computed) compute() Object {
	prevComputed := currentComputed
	currentComputed = c
	defer func() { currentComputed = prevComputed }()

	switch fn := c.fn.(type) {
	case *Function:
		extEnv := extendFunctionEnv(fn, nil, c.env, nil)
		result := Eval(fn.Body, extEnv)
		return unwrapReturnValue(result)
	case *Builtin:
		if fn.FnWithEnv != nil {
			return fn.FnWithEnv(c.env)
		}
		return fn.Fn()
	default:
		return NULL
	}
}

func (c *Computed) invalidate() {
	c.mu.Lock()
	c.valid = false
	c.mu.Unlock()
}

func GetComputedProperty(comp *Computed, name string) Object {
	switch name {
	case "value":
		return comp.Get()
	case "get":
		return &Builtin{Fn: func(args ...Object) Object {
			return comp.Get()
		}}
	default:
		return nil
	}
}

// ── Effect ─────────────────────────────────────────────────────────

// currentEffect tracks the currently executing effect for dependency tracking.
var currentEffect *Effect

// currentComputed tracks the currently computing Computed for dependency tracking.
var currentComputed *Computed

type Effect struct {
	mu       sync.Mutex
	fn       Object
	env      *Environment
	disposed bool
	running  bool
	deps     []*Signal
}

func (e *Effect) Type() ObjectType { return EFFECT_OBJ }
func (e *Effect) Inspect() string  { return "<effect>" }

func (e *Effect) run() {
	e.mu.Lock()
	if e.disposed || e.running {
		e.mu.Unlock()
		return
	}
	e.running = true
	fn := e.fn
	env := e.env
	e.mu.Unlock()

	if fn == nil {
		e.mu.Lock()
		e.running = false
		e.mu.Unlock()
		return
	}

	prevEffect := currentEffect
	currentEffect = e
	defer func() {
		currentEffect = prevEffect
		e.mu.Lock()
		e.running = false
		e.mu.Unlock()
	}()

	switch f := fn.(type) {
	case *Function:
		extEnv := extendFunctionEnv(f, nil, env, nil)
		Eval(f.Body, extEnv)
	case *Builtin:
		if f.FnWithEnv != nil {
			f.FnWithEnv(env)
		} else {
			f.Fn()
		}
	}
}

func (e *Effect) dispose() {
	e.mu.Lock()
	e.disposed = true
	e.mu.Unlock()
}

func GetEffectProperty(eff *Effect, name string) Object {
	switch name {
	case "dispose":
		return &Builtin{Fn: func(args ...Object) Object {
			eff.dispose()
			return NULL
		}}
	default:
		return nil
	}
}

// ── Builtins ───────────────────────────────────────────────────────

func init() {
	registerBuiltins(map[string]*Builtin{
		"signal":    {FnWithEnv: builtinSignal},
		"setSignal": {FnWithEnv: builtinSetSignal},
		"computed":  {FnWithEnv: builtinComputed},
		"effect":    {FnWithEnv: builtinEffect},
		"batch":     {FnWithEnv: builtinBatch},
	})
}

// signal(initial_value) or signal(name, initial_value)
func builtinSignal(env *Environment, args ...Object) Object {
	if len(args) < 1 {
		return newError("signal() requires an initial value")
	}

	var name string
	var value Object
	if len(args) >= 2 {
		if n, ok := args[0].(*String); ok {
			name = n.Value
			value = args[1]
		} else {
			value = args[0]
		}
	} else {
		value = args[0]
	}

	return &Signal{
		value: value,
		name:  name,
	}
}

// setSignal(signal, value) or setSignal(signal, fn)
// When value is a function, it receives the previous value and its return value is used.
// This enables React-style updater patterns:
//   setSignal(mySignal, fn(prev) { return {...prev, key: newVal} })
func builtinSetSignal(env *Environment, args ...Object) Object {
	if len(args) < 2 {
		return newError("setSignal() requires a signal and a value")
	}
	sig, ok := args[0].(*Signal)
	if !ok {
		return newError("setSignal() first argument must be a signal, got %s", args[0].Type())
	}
	val := args[1]
	switch fn := val.(type) {
	case *Function, *Builtin:
		prev := sig.Get()
		result := applyFunction(fn, []Object{prev}, env, nil)
		if isError(result) {
			return result
		}
		sig.Set(result)
		return result
	default:
		sig.Set(val)
		return val
	}
}

// computed(fn) — creates a computed value that auto-tracks signal dependencies
func builtinComputed(env *Environment, args ...Object) Object {
	if len(args) < 1 {
		return newError("computed() requires a function")
	}
	return &Computed{
		fn:  args[0],
		env: env,
	}
}

// effect(fn) — creates a side-effect that re-runs when tracked signals change
func builtinEffect(env *Environment, args ...Object) Object {
	if len(args) < 1 {
		return newError("effect() requires a function")
	}
	e := &Effect{
		fn:  args[0],
		env: env,
	}
	// Run immediately to establish deps
	e.run()
	return e
}

// batch(fn) — batches signal updates (runs fn, defers effect execution)
func builtinBatch(env *Environment, args ...Object) Object {
	if len(args) < 1 {
		return newError("batch() requires a function")
	}
	// For now, just execute the function — batching is a future optimization
	switch fn := args[0].(type) {
	case *Function:
		extEnv := extendFunctionEnv(fn, nil, env, nil)
		return unwrapReturnValue(Eval(fn.Body, extEnv))
	case *Builtin:
		if fn.FnWithEnv != nil {
			return fn.FnWithEnv(env)
		}
		return fn.Fn()
	default:
		return newError("batch() argument must be a function")
	}
}
