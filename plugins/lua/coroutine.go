package lua

// Thread is a Lua coroutine. A suspended coroutine retains its ordinary VM
// call stack in a parked Go goroutine; resume and yield rendezvous directly,
// so no C stack, cgo shim, or copied continuation is involved.
type Thread struct {
	state          *State
	fn             Value
	resume         chan []Value
	events         chan threadEvent
	started        bool
	dead           bool
	running        bool
	markGeneration uint64
}

type threadEvent struct {
	values  []Value
	err     error
	yielded bool
}

func newCoroutineState(parent *State, thread *Thread) *State {
	return &State{
		globals:        parent.globals,
		registry:       parent.registry,
		stack:          make([]cell, len(parent.stack)),
		frames:         make([]frame, 0, 32),
		callArgs:       make([]Value, len(parent.callArgs)),
		Output:         parent.Output,
		nextCollection: 1024,
		currentThread:  thread,
		shapes:         parent.shapes,
		randomState:    parent.randomState,
		gcPercent:      parent.gcPercent,
		gcPause:        parent.gcPause,
		gcStepMul:      parent.gcStepMul,
		dumped:         parent.dumped,
		typeMetatables: parent.typeMetatables,
	}
}

func (t *Thread) run(args []Value) {
	values, err := t.state.Call(t.fn, args...)
	t.running = false
	t.dead = true
	t.events <- threadEvent{values: values, err: err}
}

func (t *Thread) Resume(args []Value) ([]Value, error) {
	if t.dead || t.running {
		return nil, runtimeError("cannot resume %s coroutine", t.status())
	}
	t.running = true
	if !t.started {
		t.started = true
		go t.run(args)
	} else {
		t.resume <- args
	}
	event := <-t.events
	if event.yielded {
		t.running = false
	}
	return event.values, event.err
}

func (t *Thread) Yield(values []Value) ([]Value, error) {
	for _, value := range values {
		t.state.escapeValue(value, map[*Table]bool{})
	}
	t.events <- threadEvent{values: values, yielded: true}
	return <-t.resume, nil
}

func (t *Thread) status() string {
	if t.dead {
		return "dead"
	}
	if t.running {
		return "running"
	}
	return "suspended"
}
