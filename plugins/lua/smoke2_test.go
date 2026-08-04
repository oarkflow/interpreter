package lua

import "testing"

func TestSmokeEdgeCases(t *testing.T) {
	src := `
-- number formatting (Lua 5.1: all numbers are doubles, integral floats print without decimals)
assert(tostring(10) == "10", "int tostring failed: "..tostring(10))
assert(tostring(10.5) == "10.5", "float tostring failed: "..tostring(10.5))
assert(tostring(-0.0) == "-0" or tostring(-0.0) == "0", "neg zero: "..tostring(-0.0))

-- long strings / long comments
local ls = [[
line1
line2]]
assert(ls == "line1\nline2", "long string failed")
--[[ this is
a long comment ]]
assert(true, "long comment did not break parsing")

-- string.format %q and width specifiers
local q = string.format("%q", 'he said "hi"\n')
assert(q:find('said') ~= nil, "format %q failed: "..q)
assert(string.format("%5d", 3) == "    3", "format width failed: '"..string.format("%5d",3).."'")
assert(string.format("%-5d|", 3) == "3    |", "format left-align failed")
assert(string.format("%x", 255) == "ff", "format hex failed")
assert(string.format("%05.2f", 3.14159) == "03.14", "format precision failed: "..string.format("%05.2f", 3.14159))

-- rawget/rawset/rawequal bypass metamethods
local mt = {__index = function() return "meta" end, __newindex = function() error("blocked") end}
local rt = setmetatable({}, mt)
assert(rt.missing == "meta", "index metamethod failed")
assert(rawget(rt, "missing") == nil, "rawget failed")
rawset(rt, "x", 5)
assert(rawget(rt, "x") == 5, "rawset failed")

-- length operator with holes (border semantics - just check it doesn't crash and array part length works)
local arr = {1,2,3,4,5}
assert(#arr == 5, "length failed")

-- error with non-string (table) value
local ok, errval = pcall(function() error({code=42}) end)
assert(not ok and type(errval) == "table" and errval.code == 42, "error table failed")

-- nested pcall
local ok2 = pcall(function()
  local ok3, e3 = pcall(function() error("inner") end)
  assert(not ok3 and e3:find("inner"), "nested pcall failed")
  error("outer")
end)
assert(not ok2, "outer pcall should fail")

-- multiple assignment / swap
local x, y = 1, 2
x, y = y, x
assert(x == 2 and y == 1, "swap failed")

-- table constructor with trailing call expansion
local function three() return 1,2,3 end
local t1 = {three()}
assert(#t1 == 3, "trailing call expansion failed: "..#t1)
local t2 = {three(), 10}
assert(#t2 == 2 and t2[1] == 1, "non-trailing call truncation failed")

-- string.rep with separator (5.2+, optional) - just rep basic
assert(string.rep("ab", 3) == "ababab", "rep failed")

-- goto is NOT required in 5.1, skip

-- integer for-loop negative step
local acc = {}
for i = 5, 1, -1 do acc[#acc+1] = i end
assert(table.concat(acc, ",") == "5,4,3,2,1", "negative step for failed")

return "EDGE_OK"
`
	state := NewState()
	results, err := state.DoString(src)
	if err != nil {
		t.Fatalf("script error: %v", err)
	}
	if len(results) != 1 || results[0].StringValue() != "EDGE_OK" {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func TestSmokeDebugHookOnFastLoop(t *testing.T) {
	src := `
local hookCalls = 0
local function onLine() hookCalls = hookCalls + 1 end
debug.sethook(onLine, "l")

local function tightLoop(n)
  local sum = 0
  for i = 1, n do
    sum = sum + i * 2 - 1
  end
  return sum
end

local result = tightLoop(50)
debug.sethook()
assert(result == 2500, "tight loop result wrong: "..tostring(result))
assert(hookCalls > 50, "line hook did not fire enough times: "..tostring(hookCalls))
return "HOOK_OK"
`
	state := NewState()
	results, err := state.DoString(src)
	if err != nil {
		t.Fatalf("script error: %v", err)
	}
	if len(results) != 1 || results[0].StringValue() != "HOOK_OK" {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func TestSmokeDebugHookReentrant(t *testing.T) {
	src := `
local trace = {}
local function onLine()
  trace[#trace+1] = true
  local x = 0
  for i = 1, 5 do x = x + i end
  if x ~= 15 then error("nested computation corrupted") end
end
debug.sethook(onLine, "l")

local function work(n)
  local total = 0
  for i = 1, n do
    total = total + i
  end
  return total
end

local r1 = work(30)
local r2 = work(20)
debug.sethook()
assert(r1 == 465, "r1 wrong: "..tostring(r1))
assert(r2 == 210, "r2 wrong: "..tostring(r2))
assert(#trace > 0, "hook never fired")
return "REENTRANT_OK"
`
	state := NewState()
	results, err := state.DoString(src)
	if err != nil {
		t.Fatalf("script error: %v", err)
	}
	if len(results) != 1 || results[0].StringValue() != "REENTRANT_OK" {
		t.Fatalf("unexpected results: %#v", results)
	}
}
