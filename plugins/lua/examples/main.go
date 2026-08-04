// Command examples demonstrates embedding the native Go Lua plugin both
// directly (github.com/oarkflow/interpreter/plugins/lua) and through the
// host interpreter's SPL layer. Importing the plugin package runs its
// init(), which registers the "lua" SPL module and the lua`...`/
// lua```...``` tagged-block syntax — no separate blank import is needed
// once the package is imported for its Go API too, as it is here.
package main

import (
	"fmt"
	"log"

	"github.com/oarkflow/interpreter"
	"github.com/oarkflow/interpreter/plugins/lua"
)

func main() {
	directEmbedding()
	goCallbacksFromLua()
	persistentScript()
	splEmbedding()
}

// directEmbedding shows the plain Go API: compile once with Load, then Call
// it. DoString is Load+Call in one step for a chunk you only run once.
func directEmbedding() {
	fmt.Println("== direct embedding ==")
	state := lua.NewState()

	results, err := state.DoString(`return 6 * 7`)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("6 * 7 =", results[0].Number())

	fn, err := state.Load(`function add(a, b) return a + b end`, "add.lua")
	if err != nil {
		log.Fatal(err)
	}
	if _, err := state.Call(fn); err != nil {
		log.Fatal(err)
	}
	add := state.GetGlobal("add")

	// CallInto reuses caller-supplied argument/result buffers: no allocation
	// per call, the right choice for a hot loop. CallNumber2 is narrower
	// still — allocation-free for any two-argument numeric function.
	args, out := []lua.Value{lua.Number(19), lua.Number(23)}, make([]lua.Value, 1)
	if _, err := state.CallInto(add, args, out); err != nil {
		log.Fatal(err)
	}
	fmt.Println("add(19, 23) via CallInto =", out[0].Number())

	sum, err := state.CallNumber2(add, 100, 200)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("add(100, 200) via CallNumber2 =", sum)
}

// goCallbacksFromLua registers a Go function as a Lua global so Lua source
// can call back into Go. NativeBinary is a zero-alloc binding for the
// common float64-in/float64-out shape; Native accepts arbitrary argument and
// result counts for anything else.
func goCallbacksFromLua() {
	fmt.Println("== Go callbacks from Lua ==")
	state := lua.NewState()
	state.SetGlobal("hypot", lua.NativeBinary(func(a, b float64) float64 {
		return a*a + b*b
	}))
	state.SetGlobal("greet", lua.Native(func(_ *lua.State, args []lua.Value) ([]lua.Value, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("greet expects a name")
		}
		return []lua.Value{lua.String("hello, " + args[0].StringValue())}, nil
	}))

	results, err := state.DoString(`return hypot(3, 4), greet("Lua")`)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("hypot(3, 4) =", results[0].Number())
	fmt.Println("greet(\"Lua\") =", results[1].StringValue())
}

// persistentScript keeps one VM alive across calls, useful for stateful
// modules (counters, caches, connection-scoped config) that outlive a
// single request.
func persistentScript() {
	fmt.Println("== persistent script ==")
	script, _, err := lua.LoadScript(`
		local n = 0
		return {
			next = function(step) n = n + step; return n end,
		}
	`, "counter.lua", nil)
	if err != nil {
		log.Fatal(err)
	}
	for _, step := range []float64{1, 2, 3} {
		results, err := script.Call("next", lua.Number(step))
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("counter.next() =", results[0].Number())
	}
}

// splEmbedding drives Lua from SPL, the host interpreter's own scripting
// language, using the "lua" module (lua.eval/run/load) and tagged blocks.
// Single backtick interpolates ${expr} as SPL before the code reaches the
// Lua compiler; triple backtick reads the block raw, which matters for Lua
// source containing a literal $ or ${.
func splEmbedding() {
	fmt.Println("== SPL embedding ==")

	result, err := interpreter.Exec(`
		import "lua" as lua;
		let [answer, err] = lua.eval("6 * 7");
		if (err != null) { return err; }
		answer;
	`, nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("lua.eval(\"6 * 7\") =", result.Inspect())

	tagged, err := interpreter.Exec("lua`local sum = 0; for i = 1, 100 do sum = sum + i end; return sum`;", nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("lua`...` sum 1..100 =", tagged.Inspect())

	rawTagged, err := interpreter.Exec("lua```\n  local price = \"$\" .. tostring(5)\n  return price\n```;", nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("lua```...``` raw $ literal =", rawTagged.Inspect())
}
