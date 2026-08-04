package lua

import (
	"strings"
	"testing"
)

func TestSmokeComprehensive(t *testing.T) {
	src := `
-- OOP via metatables
local Animal = {}
Animal.__index = Animal
function Animal.new(name, sound)
  return setmetatable({name=name, sound=sound}, Animal)
end
function Animal:speak()
  return self.name .. " says " .. self.sound
end

local Dog = setmetatable({}, {__index = Animal})
Dog.__index = Dog
function Dog.new(name)
  local self = Animal.new(name, "Woof")
  return setmetatable(self, Dog)
end
function Dog:fetch() return self.name .. " fetches!" end

local d = Dog.new("Rex")
assert(d:speak() == "Rex says Woof", "inheritance speak failed")
assert(d:fetch() == "Rex fetches!", "subclass method failed")

-- closures/upvalues
local function counter()
  local n = 0
  return function() n = n + 1; return n end
end
local c1, c2 = counter(), counter()
assert(c1() == 1 and c1() == 2 and c2() == 1, "closures failed")

-- varargs
local function sum(...)
  local s = 0
  for _, v in ipairs({...}) do s = s + v end
  return s, select("#", ...)
end
local total, n = sum(1,2,3,4,5)
assert(total == 15 and n == 5, "varargs failed")

-- string library / patterns
local s = "Hello, World! 123"
assert(string.upper(s):sub(1,5) == "HELLO", "upper/sub failed")
assert(s:find("World") == 8, "find failed")
local digits = s:match("%d+")
assert(digits == "123", "match failed")
local replaced, count = s:gsub("o", "0")
assert(replaced == "Hell0, W0rld! 123" and count == 2, "gsub failed")
local parts = {}
for word in ("a,b,c,d"):gmatch("[^,]+") do parts[#parts+1] = word end
assert(#parts == 4 and parts[3] == "c", "gmatch failed")
assert(string.format("%d-%s-%.2f", 5, "x", 3.14159) == "5-x-3.14", "format failed")

-- table library
local t = {5,3,1,4,2}
table.sort(t)
assert(t[1]==1 and t[5]==5, "sort failed")
table.insert(t, 6)
assert(t[6]==6, "insert failed")
table.remove(t, 1)
assert(t[1]==2, "remove failed")
assert(table.concat({1,2,3}, "-") == "1-2-3", "concat failed")

-- metamethods: arithmetic, eq, tostring, call
local Vec = {}
Vec.__index = Vec
Vec.__add = function(a,b) return setmetatable({x=a.x+b.x,y=a.y+b.y}, Vec) end
Vec.__eq = function(a,b) return a.x==b.x and a.y==b.y end
Vec.__tostring = function(v) return "("..v.x..","..v.y..")" end
Vec.__call = function(self, k) return self[k] end
local v1 = setmetatable({x=1,y=2}, Vec)
local v2 = setmetatable({x=3,y=4}, Vec)
local v3 = v1 + v2
assert(v3.x==4 and v3.y==6, "__add failed")
assert(tostring(v3) == "(4,6)", "__tostring failed")
assert(v1 == setmetatable({x=1,y=2}, Vec), "__eq failed")
assert(v1("x") == 1, "__call failed")

-- error handling
local ok, err = pcall(function() error("boom") end)
assert(not ok and err:find("boom"), "pcall/error failed")
local ok2, a, b = pcall(function() return 1,2 end)
assert(ok2 and a==1 and b==2, "pcall success path failed")

-- coroutines
local co = coroutine.create(function(a)
  local b = coroutine.yield(a+1)
  local c = coroutine.yield(b+1)
  return c+1
end)
local ok3, r1 = coroutine.resume(co, 1)
local ok4, r2 = coroutine.resume(co, r1)
local ok5, r3 = coroutine.resume(co, r2)
assert(r1==2 and r2==3 and r3==4, "coroutine failed: "..tostring(r1)..","..tostring(r2)..","..tostring(r3))

-- numeric edge cases
assert(10 % 3 == 1, "mod failed")
assert(2^10 == 1024, "pow failed")
assert(math.floor(3.7) == 3 and math.ceil(3.2) == 4, "floor/ceil failed")

return "ALL_OK"
`
	state := NewState()
	results, err := state.DoString(src)
	if err != nil {
		t.Fatalf("script error: %v", err)
	}
	if len(results) != 1 || results[0].kind != StringKind || results[0].StringValue() != "ALL_OK" {
		t.Fatalf("unexpected results: %#v", results)
	}
	_ = strings.TrimSpace
}
