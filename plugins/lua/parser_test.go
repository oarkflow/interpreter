package lua

import "testing"

func TestParserEstablishedProgramShapes(t *testing.T) {
	source := `
	local function tree(depth)
	  if depth <= 0 then return {left=nil, right=nil} end
	  return {left=tree(depth-1), right=tree(depth-1)}
	end
	local sum = 0
	for i = 1, 10 do sum = sum + i end
	while sum < 100 do sum = sum + 1 end
	return tree(4), sum
	`
	chunk, err := parse(source, "shapes.lua")
	if err != nil {
		t.Fatal(err)
	}
	if len(chunk.statements) != 5 {
		t.Fatalf("statements = %d", len(chunk.statements))
	}
}

func TestParserMethodsAndTableFields(t *testing.T) {
	_, err := parse(`
	function obj:add(x, ...)
	  self.total = self.total + x
	  return self.total, ...
	end
	obj:add(2, 3)
	local t = {[1]="a"; name="lua", 42}
	`, "methods.lua")
	if err != nil {
		t.Fatal(err)
	}
}
