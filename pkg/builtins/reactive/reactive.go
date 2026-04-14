package reactive

import (
	"fmt"
	"sync"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
)

// ── Reactive Object Types ──────────────────────────────────────────

// ── Signal ─────────────────────────────────────────────────────────

type Signal struct {
	mu          sync.RWMutex
	value       object.Object
	subscribers []*Effect
	computeds   []*Computed
	name        string
}

func (s *Signal) Type() object.ObjectType { return object.SIGNAL_OBJ }
func (s *Signal) Inspect() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return fmt.Sprintf("<signal:%s=%s>", s.name, s.value.Inspect())
}

func (s *Signal) Get() object.Object {
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

func (s *Signal) Set(val object.Object) {
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

// GetSignalProperty provides dot-access for signals.
func GetSignalProperty(sig *Signal, name string) object.Object {
	switch name {
	case "value":
		return sig.Get()
	case "set":
		return &object.Builtin{FnWithEnv: func(env *object.Environment, args ...object.Object) object.Object {
			if len(args) < 1 {
				return object.NewError("signal.set() requires a value")
			}
			val := args[0]
			// Support updater function: signal.set(fn) where fn receives prev value
			switch val.(type) {
			case *object.Function, *object.Builtin:
				prev := sig.Get()
				if object.ApplyFunctionFn == nil {
					return object.NewError("applyFunction not available")
				}
				result := object.ApplyFunctionFn(val, []object.Object{prev}, env)
				if object.IsError(result) {
					return result
				}
				sig.Set(result)
			default:
				sig.Set(val)
			}
			return object.NULL
		}}
	case "get":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			return sig.Get()
		}}
	case "name":
		return &object.String{Value: sig.name}
	case "subscribe":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return object.NewError("subscribe() requires a callback")
			}
			e := &Effect{
				fn:  args[0],
				env: nil,
			}
			sig.mu.Lock()
			sig.subscribers = append(sig.subscribers, e)
			sig.mu.Unlock()
			return object.NULL
		}}
	default:
		return nil
	}
}

// ── Computed ───────────────────────────────────────────────────────

type Computed struct {
	mu    sync.Mutex
	fn    object.Object // computation function
	env   *object.Environment
	value object.Object
	valid bool
	deps  []*Signal
}

func (c *Computed) Type() object.ObjectType { return object.COMPUTED_OBJ }
func (c *Computed) Inspect() string {
	val := c.Get()
	return fmt.Sprintf("<computed=%s>", val.Inspect())
}

func (c *Computed) Get() object.Object {
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

func (c *Computed) compute() object.Object {
	prevComputed := currentComputed
	currentComputed = c
	defer func() { currentComputed = prevComputed }()

	switch fn := c.fn.(type) {
	case *object.Function:
		if object.ExtendFunctionEnvFn == nil || object.EvalFn == nil || object.UnwrapReturnValueFn == nil {
			return object.NULL
		}
		extEnv := object.ExtendFunctionEnvFn(fn, nil, c.env)
		result := object.EvalFn(fn.Body, extEnv)
		return object.UnwrapReturnValueFn(result)
	case *object.Builtin:
		if fn.FnWithEnv != nil {
			return fn.FnWithEnv(c.env)
		}
		return fn.Fn()
	default:
		return object.NULL
	}
}

func (c *Computed) invalidate() {
	c.mu.Lock()
	c.valid = false
	c.mu.Unlock()
}

// GetComputedProperty provides dot-access for computed values.
func GetComputedProperty(comp *Computed, name string) object.Object {
	switch name {
	case "value":
		return comp.Get()
	case "get":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
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
	fn       object.Object
	env      *object.Environment
	disposed bool
	running  bool
	deps     []*Signal
}

func (e *Effect) Type() object.ObjectType { return object.EFFECT_OBJ }
func (e *Effect) Inspect() string         { return "<effect>" }

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
	case *object.Function:
		if object.ExtendFunctionEnvFn != nil && object.EvalFn != nil {
			extEnv := object.ExtendFunctionEnvFn(f, nil, env)
			object.EvalFn(f.Body, extEnv)
		}
	case *object.Builtin:
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

// GetEffectProperty provides dot-access for effects.
func GetEffectProperty(eff *Effect, name string) object.Object {
	switch name {
	case "dispose":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			eff.dispose()
			return object.NULL
		}}
	default:
		return nil
	}
}

// ── Builtins ───────────────────────────────────────────────────────

func init() {
	eval.RegisterBuiltins(map[string]*object.Builtin{
		"signal":    {FnWithEnv: builtinSignal},
		"setSignal": {FnWithEnv: builtinSetSignal},
		"computed":  {FnWithEnv: builtinComputed},
		"effect":    {FnWithEnv: builtinEffect},
		"batch":     {FnWithEnv: builtinBatch},
	})

	// Register dot expression hook for reactive types
	prev := eval.DotExpressionHook
	eval.DotExpressionHook = func(left object.Object, name string) object.Object {
		switch obj := left.(type) {
		case *Signal:
			return GetSignalProperty(obj, name)
		case *Computed:
			return GetComputedProperty(obj, name)
		case *Effect:
			return GetEffectProperty(obj, name)
		}
		if prev != nil {
			return prev(left, name)
		}
		return nil
	}
}

// signal(initial_value) or signal(name, initial_value)
func builtinSignal(env *object.Environment, args ...object.Object) object.Object {
	if len(args) < 1 {
		return object.NewError("signal() requires an initial value")
	}

	var name string
	var value object.Object
	if len(args) >= 2 {
		if n, ok := args[0].(*object.String); ok {
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
func builtinSetSignal(env *object.Environment, args ...object.Object) object.Object {
	if len(args) < 2 {
		return object.NewError("setSignal() requires a signal and a value")
	}
	sig, ok := args[0].(*Signal)
	if !ok {
		return object.NewError("setSignal() first argument must be a signal, got %s", args[0].Type())
	}
	val := args[1]
	switch val.(type) {
	case *object.Function, *object.Builtin:
		prev := sig.Get()
		if object.ApplyFunctionFn == nil {
			return object.NewError("applyFunction not available")
		}
		result := object.ApplyFunctionFn(val, []object.Object{prev}, env)
		if object.IsError(result) {
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
func builtinComputed(env *object.Environment, args ...object.Object) object.Object {
	if len(args) < 1 {
		return object.NewError("computed() requires a function")
	}
	return &Computed{
		fn:  args[0],
		env: env,
	}
}

// effect(fn) — creates a side-effect that re-runs when tracked signals change
func builtinEffect(env *object.Environment, args ...object.Object) object.Object {
	if len(args) < 1 {
		return object.NewError("effect() requires a function")
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
func builtinBatch(env *object.Environment, args ...object.Object) object.Object {
	if len(args) < 1 {
		return object.NewError("batch() requires a function")
	}
	switch fn := args[0].(type) {
	case *object.Function:
		if object.ExtendFunctionEnvFn == nil || object.EvalFn == nil || object.UnwrapReturnValueFn == nil {
			return object.NewError("eval functions not available")
		}
		extEnv := object.ExtendFunctionEnvFn(fn, nil, env)
		return object.UnwrapReturnValueFn(object.EvalFn(fn.Body, extEnv))
	case *object.Builtin:
		if fn.FnWithEnv != nil {
			return fn.FnWithEnv(env)
		}
		return fn.Fn()
	default:
		return object.NewError("batch() argument must be a function")
	}
}
