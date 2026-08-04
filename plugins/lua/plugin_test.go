package lua

import (
	"github.com/oarkflow/interpreter"
	"github.com/oarkflow/interpreter/pkg/eval"
	"testing"
)

func TestPluginRegistrationAndSPLExecution(t *testing.T) {
	for _, name := range []string{"lua_run", "lua_eval", "lua_load", "lua_version"} {
		if !eval.HasBuiltin(name) {
			t.Fatalf("missing %s", name)
		}
	}
	result, err := interpreter.Exec(`import "lua" as lua; let [answer, lua_err] = lua.eval("6 * 7"); if (lua_err != null) { return lua_err; } answer;`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Inspect() != "42" {
		t.Fatalf("result = %s", result.Inspect())
	}
	tagged, err := interpreter.Exec("lua`local sum=0; for i=1,10 do sum=sum+i end; return sum`;", nil)
	if err != nil {
		t.Fatal(err)
	}
	if tagged.Inspect() != "55" {
		t.Fatalf("tagged = %s", tagged.Inspect())
	}
}

func TestPersistentLuaModuleFromSPL(t *testing.T) {
	result, err := interpreter.Exec(`
		import "lua" as lua;
		let [mod, load_err] = lua.load("return {add=function(a,b) return a+b end}");
		if (load_err != null) { return load_err; }
		let [answer, call_err] = mod.call("add", 20, 22);
		if (call_err != null) { return call_err; }
		answer;
	`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Inspect() != "42" {
		t.Fatalf("result = %s", result.Inspect())
	}
}

func TestPersistentLuaGlobalSetFromSPL(t *testing.T) {
	result, err := interpreter.Exec(`
		import "lua" as lua;
		let [script, load_err] = lua.load("function answer() return injected + 2 end");
		if (load_err != null) { return load_err; }
		let [set_value, set_err] = script.set("injected", 40);
		if (set_err != null) { return set_err; }
		let [answer, call_err] = script.call("answer");
		if (call_err != null) { return call_err; }
		answer;
	`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Inspect() != "42" {
		t.Fatalf("result = %s", result.Inspect())
	}
}
