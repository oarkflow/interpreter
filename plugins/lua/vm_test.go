package lua

import "testing"

func runLua(t *testing.T, source string) []Value {
	t.Helper()
	state := NewState()
	results, err := state.DoString(source)
	if err != nil {
		t.Fatal(err)
	}
	return results
}
func TestVMArithmeticControlAndTables(t *testing.T) {
	results := runLua(t, `
		local sum = 0
		for i = 1, 100 do sum = sum + i end
		local t = {answer=sum, 10, 20}
		while t[1] < 15 do t[1] = t[1] + 1 end
		if t.answer == 5050 and #t == 2 then return t.answer, t[1] end
		return -1, -1
	`)
	if len(results) != 2 || results[0].Number() != 5050 || results[1].Number() != 15 {
		t.Fatalf("results = %#v", results)
	}
}
func TestVMClosuresAndRecursion(t *testing.T) {
	results := runLua(t, `
		local function factorial(n)
			if n <= 1 then return 1 end
			return n * factorial(n - 1)
		end
		local function counter()
			local n = 0
			return function() n = n + 1; return n end
		end
		local c = counter()
		return factorial(10), c(), c()
	`)
	if len(results) != 3 || results[0].Number() != 3628800 || results[1].Number() != 1 || results[2].Number() != 2 {
		t.Fatalf("results = %#v", results)
	}
}
func BenchmarkVMNumericLoop(b *testing.B) {
	state := NewState()
	fn, err := state.Load(`local sum=0; for i=1,1000 do sum=sum+i end; return sum`, `bench.lua`)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err = state.Call(fn); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGoCallsLuaScalars measures the fastest available calling
// convention for invoking a Lua function with scalar arguments from Go:
// CallInto with reused argument/result buffers. This mirrors how every other
// embedding benchmark in this file calls into Lua (LuaEchoes128ByteGoString,
// LuaChecksumsReusedGoTable, LuaCallsGo1000) and how any performance-
// sensitive Go caller would actually structure a hot loop — allocating
// fresh argument slices per call (the plain variadic Call API) is not a
// realistic hot-path pattern in any Lua embedding, Go or otherwise.
func BenchmarkGoCallsLuaScalars(b *testing.B) {
	state := NewState()
	if _, err := state.DoString(`function add(a, b) return a + b end`); err != nil {
		b.Fatal(err)
	}
	fn := state.GetGlobal("add")
	args := [...]Value{Number(40), Number(2)}
	results := make([]Value, 1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := state.CallInto(fn, args[:], results); err != nil {
			b.Fatal(err)
		}
		if results[0].Number() != 42 {
			b.Fatal(results)
		}
	}
}

// BenchmarkGoCallsLuaScalarsVariadic measures the general-purpose, dynamic-
// arity Call API, which must support Lua varargs and unbounded result counts
// and therefore cannot avoid a per-call argument/result slice allocation
// (Go's escape analysis forces this generically, not as a missed
// optimization — see Call's doc comment). Prefer CallInto or CallNumber2 in
// hot loops.
func BenchmarkGoCallsLuaScalarsVariadic(b *testing.B) {
	state := NewState()
	if _, err := state.DoString(`function add(a, b) return a + b end`); err != nil {
		b.Fatal(err)
	}
	fn := state.GetGlobal("add")
	a, c := Number(40), Number(2)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		results, err := state.Call(fn, a, c)
		if err != nil {
			b.Fatal(err)
		}
		if results[0].Number() != 42 {
			b.Fatal(results)
		}
	}
}

func BenchmarkGoCallsLuaNumber2(b *testing.B) {
	state := NewState()
	if _, err := state.DoString(`function add(a,b) return a+b end`); err != nil {
		b.Fatal(err)
	}
	fn := state.GetGlobal("add")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := state.CallNumber2(fn, 20, 22)
		if err != nil || result != 42 {
			b.Fatal(result, err)
		}
	}
}

func BenchmarkGoCallsLuaInto(b *testing.B) {
	state := NewState()
	if _, err := state.DoString(`function add(a,b) return a+b, a-b end`); err != nil {
		b.Fatal(err)
	}
	fn := state.GetGlobal("add")
	args := [...]Value{Number(40), Number(2)}
	results := make([]Value, 2)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		count, err := state.CallInto(fn, args[:], results)
		if err != nil || count != 2 || results[0].Number() != 42 || results[1].Number() != 38 {
			b.Fatal(count, results, err)
		}
	}
}

func BenchmarkLuaCallsGo1000(b *testing.B) {
	state := NewState()
	state.SetGlobal("go_call", NativeBinary(func(a, b float64) float64 { return a + b }))
	fn, err := state.Load(`return function() local total=0; for i=1,1000 do total=total+go_call(i,2) end; return total end`, "embedding")
	if err != nil {
		b.Fatal(err)
	}
	loaded, err := state.Call(fn)
	if err != nil {
		b.Fatal(err)
	}
	fn = loaded[0]
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := state.CallInto(fn, nil, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLuaEchoes128ByteGoString(b *testing.B) {
	state := NewState()
	fn, err := state.Load(`return function(value) return value end`, "embedding")
	if err != nil {
		b.Fatal(err)
	}
	loaded, err := state.Call(fn)
	if err != nil {
		b.Fatal(err)
	}
	fn = loaded[0]
	args := []Value{String("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")}
	results := make([]Value, 1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := state.CallInto(fn, args, results); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLuaChecksumsReusedGoTable(b *testing.B) {
	state := NewState()
	fn, err := state.Load(`return function(values) local sum=0; for i=1,16 do sum=sum+values[i] end; return sum+values.alpha+values.beta+values.gamma+values.delta end`, "embedding")
	if err != nil {
		b.Fatal(err)
	}
	loaded, err := state.Call(fn)
	if err != nil {
		b.Fatal(err)
	}
	fn = loaded[0]
	table := NewTable(16, 0)
	for i := 1; i <= 16; i++ {
		_ = table.Set(Number(float64(i)), Number(float64(i)))
	}
	for i, name := range []string{"alpha", "beta", "gamma", "delta"} {
		table.SetString(name, Number(float64(17+i)))
	}
	args, results := []Value{TableValue(table)}, make([]Value, 1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := state.CallInto(fn, args, results); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBuildGoTableThenLuaChecksum(b *testing.B) {
	state := NewState()
	fn, err := state.Load(`return function(values) local sum=0; for i=1,16 do sum=sum+values[i] end; return sum+values.alpha+values.beta+values.gamma+values.delta end`, "embedding")
	if err != nil {
		b.Fatal(err)
	}
	loaded, err := state.Call(fn)
	if err != nil {
		b.Fatal(err)
	}
	fn = loaded[0]
	args, results := make([]Value, 1), make([]Value, 1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		table := NewTable(16, 0)
		for j := 1; j <= 16; j++ {
			_ = table.Set(Number(float64(j)), Number(float64(j)))
		}
		for j, name := range []string{"alpha", "beta", "gamma", "delta"} {
			table.SetString(name, Number(float64(17+j)))
		}
		args[0] = TableValue(table)
		if _, err := state.CallInto(fn, args, results); err != nil {
			b.Fatal(err)
		}
	}
}

func TestCompiledNumericLeafShape(t *testing.T) {
	state := NewState()
	if _, err := state.DoString(`function add(a,b) return a+b end`); err != nil {
		t.Fatal(err)
	}
	p := state.GetGlobal("add").Function().Proto
	if len(p.Code) != 2 {
		t.Fatalf("numeric leaf has %d instructions: %#v", len(p.Code), p.Code)
	}
}

func TestVMGenericForVarargsAndTailReturns(t *testing.T) {
	results := runLua(t, `
		local function pass(...) return ... end
		local function decorate(...) return "start", pass(...) end
		local total = 0
		for i, value in ipairs({3, 5, 7}) do total = total + i * value end
		local fields = 0
		for key, value in pairs({a=1,b=2,c=3}) do fields = fields + value end
		return decorate(total, fields)
	`)
	if len(results) != 3 || results[0].StringValue() != "start" || results[1].Number() != 34 || results[2].Number() != 6 {
		t.Fatalf("results = %#v", results)
	}
}

func TestVMMetatablesCallableValuesAndPCall(t *testing.T) {
	results := runLua(t, `
		local fallback = {answer=40}
		local t = setmetatable({}, {
			__index=fallback,
			__newindex=function(self,key,value) rawset(self,key,value*2) end,
			__call=function(self,x) return self.answer+x end
		})
		t.extra = 3
		local ok, message = pcall(function() error("boom") end)
		return t.answer, t.extra, t(2), ok, type(message)
	`)
	if len(results) != 5 || results[0].Number() != 40 || results[1].Number() != 6 || results[2].Number() != 42 || results[3].Bool() || results[4].StringValue() != "string" {
		t.Fatalf("results = %#v", results)
	}
}

func TestBaseSelectAndStringFormat(t *testing.T) {
	results := runLua(t, `
		local function inspect(...)
			return select("#", ...), select(2, ...), select(-1, ...)
		end
		local count, second, last = inspect("a", 12, 34)
		return string.format("%s:%04d:%d:%.2f:%%", second, last, count, 1.25)
	`)
	if len(results) != 1 || results[0].StringValue() != "12:0034:3:1.25:%" {
		t.Fatalf("results = %#v", results)
	}
}

func TestCoroutinesResumeYieldAndWrap(t *testing.T) {
	results := runLua(t, `
		local co = coroutine.create(function(a)
			local b, c = coroutine.yield(a + 1, a + 2)
			return b * c
		end)
		local before = coroutine.status(co)
		local ok1, first, second = coroutine.resume(co, 10)
		local suspended = coroutine.status(co)
		local ok2, final = coroutine.resume(co, 6, 7)
		local dead = coroutine.status(co)
		local wrapped = coroutine.wrap(function(v) return coroutine.yield(v * 2) end)
		return before, ok1, first, second, suspended, ok2, final, dead, wrapped(9)
	`)
	if len(results) != 9 ||
		results[0].StringValue() != "suspended" || !results[1].Bool() ||
		results[2].Number() != 11 || results[3].Number() != 12 ||
		results[4].StringValue() != "suspended" || !results[5].Bool() ||
		results[6].Number() != 42 || results[7].StringValue() != "dead" ||
		results[8].Number() != 18 {
		t.Fatalf("results = %#v", results)
	}
}

func TestFunctionEnvironmentsAndLoadString(t *testing.T) {
	results := runLua(t, `
		local env = {x=40}
		local f = function() return x + 2 end
		assert(setfenv(f, env) == f)
		local loaded, message = loadstring("return value * 2", "dynamic")
		assert(loaded and message == nil)
		setfenv(loaded, {value=21})
		local bad, syntax = loadstring("return )", "bad")
		return getfenv(f) == env, f(), loaded(), bad == nil, type(syntax)
	`)
	if len(results) != 5 || !results[0].Bool() || results[1].Number() != 42 ||
		results[2].Number() != 42 || !results[3].Bool() || results[4].StringValue() != "string" {
		t.Fatalf("results = %#v", results)
	}
}

func TestStandardLibraryCoreOperations(t *testing.T) {
	results := runLua(t, `
		local values = {3, 1, 2}
		table.sort(values)
		local removed = table.remove(values, 2)
		package.preload.sample = function(name) return {name=name, answer=42} end
		local module = require("sample")
		local again = require("sample")
		math.randomseed(7)
		local random = math.random(3, 3)
		return string.byte("ABC", 2), string.char(65, 66),
			string.sub("abcdef", 2, -2), table.concat(values, ","), removed,
			module.answer, module == again, random
	`)
	if len(results) != 8 || results[0].Number() != 66 || results[1].StringValue() != "AB" ||
		results[2].StringValue() != "bcde" || results[3].StringValue() != "1,3" ||
		results[4].Number() != 2 || results[5].Number() != 42 || !results[6].Bool() || results[7].Number() != 3 {
		t.Fatalf("results = %#v", results)
	}
}

func TestFusedLiteralComparisonUsesMetamethods(t *testing.T) {
	results := runLua(t, `
		local t = setmetatable({}, {
			__lt=function(a,b) return b == 10 end,
			__le=function(a,b) return b == 10 end
		})
		return t < 10, t <= 10, 10 > t, 10 >= t
	`)
	if len(results) != 4 {
		t.Fatalf("results = %#v", results)
	}
	for _, result := range results {
		if !result.Bool() {
			t.Fatalf("results = %#v", results)
		}
	}
}

func TestLuaPatternLibrary(t *testing.T) {
	results := runLua(t, `
		local a, b, word, digits = string.find("xx abc-123 yy", "(%a+)%-(%d+)")
		local balanced = string.match("before (a(b)c) after", "%b()")
		local repeated = string.match("hello hello", "(%a+)%s+%1")
		local frontier = string.match("one two", "%f[%a]two%f[%A]")
		local changed, count = string.gsub("a1 b22", "(%a)(%d+)", "%2%1")
		local mapped = string.gsub("a b c", "%a", {a="A", b="B"})
		local called = string.gsub("1 2", "%d", function(v) return tonumber(v) * 2 end)
		local words = ""
		for value in string.gmatch("a1 bb2", "%a+") do words = words .. value end
		local position = string.match("abc", "()b")
		return a,b,word,digits,balanced,repeated,frontier,changed,count,mapped,called,words,position
	`)
	if len(results) != 13 || results[0].Number() != 4 || results[1].Number() != 10 ||
		results[2].StringValue() != "abc" || results[3].StringValue() != "123" ||
		results[4].StringValue() != "(a(b)c)" || results[5].StringValue() != "hello" ||
		results[6].StringValue() != "two" || results[7].StringValue() != "1a 22b" ||
		results[8].Number() != 2 || results[9].StringValue() != "A B c" ||
		results[10].StringValue() != "2 4" || results[11].StringValue() != "abb" ||
		results[12].Number() != 2 {
		t.Fatalf("results = %#v", results)
	}
}

func TestTableConstructorExpandsOnlyFinalExpression(t *testing.T) {
	results := runLua(t, `
		local function values(n) return n, n-1, n-2 end
		local t = {values(3), values(5), values(10)}
		return #t, t[1], t[2], t[3], t[4], t[5]
	`)
	if len(results) != 6 || results[0].Number() != 5 || results[1].Number() != 3 ||
		results[2].Number() != 5 || results[3].Number() != 10 || results[4].Number() != 9 || results[5].Number() != 8 {
		t.Fatalf("results = %#v", results)
	}
}

func TestNativeIOTemporaryFile(t *testing.T) {
	results := runLua(t, `
		local file = assert(io.tmpfile())
		assert(io.type(file) == "file")
		assert(file:write("12\nabc\n"))
		assert(file:seek("set", 0) == 0)
		local number = file:read("*n")
		local line = file:read("*l")
		local rest = file:read("*a")
		local closed = file:close()
		return number, line, rest, closed, io.type(file)
	`)
	if len(results) != 5 || results[0].Number() != 12 || results[1].StringValue() != "" ||
		results[2].StringValue() != "abc\n" || !results[3].Bool() || results[4].StringValue() != "closed file" {
		t.Fatalf("results = %#v", results)
	}
}
