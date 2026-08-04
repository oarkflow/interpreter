package lua

import (
	"math"
	"testing"
)

const binaryTreesProgram = `
local function bottomUpTree(depth)
  if depth > 0 then
    return {bottomUpTree(depth-1), bottomUpTree(depth-1)}
  end
  return {}
end
local function itemCheck(tree)
  if tree[1] then return 1 + itemCheck(tree[1]) + itemCheck(tree[2]) end
  return 1
end
function run(maxDepth)
  local minDepth = 4
  local stretchDepth = maxDepth + 1
  local check = itemCheck(bottomUpTree(stretchDepth))
  local longLivedTree = bottomUpTree(maxDepth)
  for depth = minDepth, maxDepth, 2 do
    local iterations = 2 ^ (maxDepth - depth + minDepth)
    local sum = 0
    for i = 1, iterations do sum = sum + itemCheck(bottomUpTree(depth)) end
    check = check + sum
  end
  return check + itemCheck(longLivedTree)
end
`

const binaryTreesOutputProgram = binaryTreesProgram + `
local benchmarkOutput = ""
local function benchmarkWrite(...)
  for i = 1, select("#", ...) do
    benchmarkOutput = benchmarkOutput .. tostring(select(i, ...))
  end
end
function runOutput(maxDepth)
  benchmarkOutput = ""
  local minDepth = 4
  local stretchDepth = maxDepth + 1
  benchmarkWrite(string.format("stretch tree of depth %d\t check: %d\n",
    stretchDepth, itemCheck(bottomUpTree(stretchDepth))))
  local longLivedTree = bottomUpTree(maxDepth)
  for depth = minDepth, maxDepth, 2 do
    local iterations = 2 ^ (maxDepth - depth + minDepth)
    local check = 0
    for i = 1, iterations do check = check + itemCheck(bottomUpTree(depth)) end
    benchmarkWrite(string.format("%d\t trees of depth %d\t check: %d\n",
      iterations, depth, check))
  end
  benchmarkWrite(string.format("long lived tree of depth %d\t check: %d\n",
    maxDepth, itemCheck(longLivedTree)))
  return benchmarkOutput
end
`

// BenchmarkBinaryTrees12 uses the same input as Lunar's published established-
// program comparison. The source above omits output formatting so this remains
// a VM/allocator development benchmark, not yet a publishable cross-runtime
// result.
func BenchmarkBinaryTrees12(b *testing.B) {
	state := NewState()
	if _, err := state.DoString(binaryTreesProgram); err != nil {
		b.Fatal(err)
	}
	fn := state.GetGlobal("run")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		results, err := state.Call(fn, Number(12))
		if err != nil {
			b.Fatal(err)
		}
		if len(results) != 1 || results[0].kind != NumberKind {
			b.Fatal(results)
		}
	}
}

func BenchmarkBinaryTrees12WithCapturedOutput(b *testing.B) {
	state := NewState()
	if _, err := state.DoString(binaryTreesOutputProgram); err != nil {
		b.Fatal(err)
	}
	fn := state.GetGlobal("runOutput")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		results, err := state.Call(fn, Number(12))
		if err != nil {
			b.Fatal(err)
		}
		if len(results) != 1 || results[0].kind != StringKind || len(results[0].StringValue()) == 0 {
			b.Fatal(results)
		}
	}
}

func BenchmarkRecursiveCalls(b *testing.B) {
	state := NewState()
	_, err := state.DoString(`function f(n) if n==0 then return 0 end return f(n-1)+1 end`)
	if err != nil {
		b.Fatal(err)
	}
	fn := state.GetGlobal("f")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err = state.Call(fn, Number(20))
		if err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkCreateSmallTable(b *testing.B) {
	state := NewState()
	_, err := state.DoString(`function f(a,b) return {a,b} end`)
	if err != nil {
		b.Fatal(err)
	}
	fn := state.GetGlobal("f")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err = state.Call(fn, Number(1), Number(2))
		if err != nil {
			b.Fatal(err)
		}
	}
}

const fannkuchReduxProgram = `
function fannkuch(n)
  local p, q, s, sign, maxflips, sum = {}, {}, {}, 1, 0, 0
  for i=1,n do p[i] = i; q[i] = i; s[i] = i end
  repeat
    local q1 = p[1]
    if q1 ~= 1 then
      for i=2,n do q[i] = p[i] end
      local flips = 1
      repeat
        local qq = q[q1]
        if qq == 1 then
          sum = sum + sign*flips
          if flips > maxflips then maxflips = flips end
          break
        end
        q[q1] = q1
        if q1 >= 4 then
          local i, j = 2, q1 - 1
          repeat q[i], q[j] = q[j], q[i]; i = i + 1; j = j - 1 until i >= j
        end
        q1 = qq; flips = flips + 1
      until false
    end
    if sign == 1 then
      p[2], p[1] = p[1], p[2]; sign = -1
    else
      p[2], p[3] = p[3], p[2]; sign = 1
      for i=3,n do
        local sx = s[i]
        if sx ~= 1 then s[i] = sx-1; break end
        if i == n then return sum, maxflips end
        s[i] = i
        local t = p[1]; for j=1,i do p[j] = p[j+1] end; p[i+1] = t
      end
    end
  until false
end
`

const spectralNormProgram = `
local function A(i, j)
  local ij = i+j-1
  return 1.0 / (ij * (ij-1) * 0.5 + i)
end
local function Av(x, y, N)
  for i=1,N do
    local a = 0
    for j=1,N do a = a + x[j] * A(i, j) end
    y[i] = a
  end
end
local function Atv(x, y, N)
  for i=1,N do
    local a = 0
    for j=1,N do a = a + x[j] * A(j, i) end
    y[i] = a
  end
end
local function AtAv(x, y, t, N) Av(x, t, N); Atv(t, y, N) end
function spectralnorm(N)
  local u, v, t = {}, {}, {}
  for i=1,N do u[i] = 1 end
  for i=1,10 do AtAv(u, v, t, N); AtAv(v, u, t, N) end
  local vBv, vv = 0, 0
  for i=1,N do
    local ui, vi = u[i], v[i]
    vBv = vBv + ui*vi
    vv = vv + vi*vi
  end
  return math.sqrt(vBv / vv)
end
`

const nbodyProgram = `
local sqrt = math.sqrt
local PI = 3.141592653589793
local SOLAR_MASS = 4 * PI * PI
local DAYS_PER_YEAR = 365.24
local function newBodies()
  return {
    {x=0,y=0,z=0,vx=0,vy=0,vz=0,mass=SOLAR_MASS},
    {x=4.84143144246472090e+00,y=-1.16032004402742839e+00,z=-1.03622044471123109e-01,vx=1.66007664274403694e-03*DAYS_PER_YEAR,vy=7.69901118419740425e-03*DAYS_PER_YEAR,vz=-6.90460016972063023e-05*DAYS_PER_YEAR,mass=9.54791938424326609e-04*SOLAR_MASS},
    {x=8.34336671824457987e+00,y=4.12479856412430479e+00,z=-4.03523417114321381e-01,vx=-2.76742510726862411e-03*DAYS_PER_YEAR,vy=4.99852801234917238e-03*DAYS_PER_YEAR,vz=2.30417297573763929e-05*DAYS_PER_YEAR,mass=2.85885980666130812e-04*SOLAR_MASS},
    {x=1.28943695621391310e+01,y=-1.51111514016986312e+01,z=-2.23307578892655734e-01,vx=2.96460137564761618e-03*DAYS_PER_YEAR,vy=2.37847173959480950e-03*DAYS_PER_YEAR,vz=-2.96589568540237556e-05*DAYS_PER_YEAR,mass=4.36624404335156298e-05*SOLAR_MASS},
    {x=1.53796971148509165e+01,y=-2.59193146099879641e+01,z=1.79258772950371181e-01,vx=2.68067772490321933e-03*DAYS_PER_YEAR,vy=1.62824170038242295e-03*DAYS_PER_YEAR,vz=-9.51592254519715870e-05*DAYS_PER_YEAR,mass=5.15138902046611451e-05*SOLAR_MASS}
  }
end
local function advance(bodies,nbody,dt)
  for i=1,nbody do
    local bi=bodies[i]
    local bix,biy,biz,bimass=bi.x,bi.y,bi.z,bi.mass
    local bivx,bivy,bivz=bi.vx,bi.vy,bi.vz
    for j=i+1,nbody do
      local bj=bodies[j]
      local dx,dy,dz=bix-bj.x,biy-bj.y,biz-bj.z
      local mag=sqrt(dx*dx+dy*dy+dz*dz)
      mag=dt/(mag*mag*mag)
      local bm=bj.mass*mag
      bivx=bivx-(dx*bm);bivy=bivy-(dy*bm);bivz=bivz-(dz*bm)
      bm=bimass*mag
      bj.vx=bj.vx+(dx*bm);bj.vy=bj.vy+(dy*bm);bj.vz=bj.vz+(dz*bm)
    end
    bi.vx=bivx;bi.vy=bivy;bi.vz=bivz
    bi.x=bix+dt*bivx;bi.y=biy+dt*bivy;bi.z=biz+dt*bivz
  end
end
local function energy(bodies,nbody)
  local e=0
  for i=1,nbody do
    local bi=bodies[i]
    local vx,vy,vz,bim=bi.vx,bi.vy,bi.vz,bi.mass
    e=e+(0.5*bim*(vx*vx+vy*vy+vz*vz))
    for j=i+1,nbody do
      local bj=bodies[j]
      local dx,dy,dz=bi.x-bj.x,bi.y-bj.y,bi.z-bj.z
      local distance=sqrt(dx*dx+dy*dy+dz*dz)
      e=e-((bim*bj.mass)/distance)
    end
  end
  return e
end
local function offsetMomentum(b,nbody)
  local px,py,pz=0,0,0
  for i=1,nbody do local bi=b[i];local bim=bi.mass;px=px+(bi.vx*bim);py=py+(bi.vy*bim);pz=pz+(bi.vz*bim) end
  b[1].vx=-px/SOLAR_MASS;b[1].vy=-py/SOLAR_MASS;b[1].vz=-pz/SOLAR_MASS
end
function nbody(N)
  local bodies=newBodies();local count=#bodies
  offsetMomentum(bodies,count)
  local before=energy(bodies,count)
  for i=1,N do advance(bodies,count,0.01) end
  return before,energy(bodies,count)
end
`

func BenchmarkFannkuchRedux8(b *testing.B) {
	state := NewState()
	if _, err := state.DoString(fannkuchReduxProgram); err != nil {
		b.Fatal(err)
	}
	fn := state.GetGlobal("fannkuch")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		results, err := state.Call(fn, Number(8))
		if err != nil || len(results) != 2 || results[0].Number() != 1616 || results[1].Number() != 22 {
			b.Fatal(results, err)
		}
	}
}

func BenchmarkSpectralNorm150(b *testing.B) {
	state := NewState()
	if _, err := state.DoString(spectralNormProgram); err != nil {
		b.Fatal(err)
	}
	fn := state.GetGlobal("spectralnorm")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		results, err := state.Call(fn, Number(150))
		if err != nil || len(results) != 1 || math.Abs(results[0].Number()-1.274222873) > 5e-9 {
			b.Fatal(results, err)
		}
	}
}

func BenchmarkNBody20000(b *testing.B) {
	state := NewState()
	if _, err := state.DoString(nbodyProgram); err != nil {
		b.Fatal(err)
	}
	fn := state.GetGlobal("nbody")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		results, err := state.Call(fn, Number(20000))
		if err != nil || len(results) != 2 || math.Abs(results[0].Number()+0.169075164) > 5e-9 || math.Abs(results[1].Number()+0.169089263) > 5e-9 {
			b.Fatal(results, err)
		}
	}
}

func TestSpectralNumericPlan(t *testing.T) {
	p, err := Compile(spectralNormProgram, "spectral")
	if err != nil { t.Fatal(err) }
	if len(p.Children) == 0 || !p.Children[0].NumericPure || len(p.Children[0].NumericCode) >= len(p.Children[0].Code) {
		t.Fatalf("numeric plan was not compacted: %#v", p.Children)
	}
}
