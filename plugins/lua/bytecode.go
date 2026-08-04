package lua

import "io"

// Instruction is a compact fixed-width register instruction. ABC operands
// cover hot arithmetic/table operations; Bx is used for constants and jumps.
type Instruction uint32

type opcode uint8

const (
	opMove opcode = iota
	opLoadK
	opLoadNil
	opLoadBool
	opGetUp
	opSetUp
	opGetGlobal
	opSetGlobal
	opGetTable
	opSetTable
	opNewTable
	opAdd
	opSub
	opMul
	opDiv
	opMod
	opPow
	opNeg
	opNot
	opLen
	opConcat
	opEq
	opLT
	opLE
	opJump
	opJumpFalse
	opForPrep
	opForLoop
	opTForCall
	opVararg
	opClosure
	opCall
	opCallTail
	opReturn
	opReturnTail
	opGetTableK
	opSetTableK
	opAddK
	opSubK
	opMulK
	opDivK
	opModK
	opPowK
	opEqK
	opLTK
	opLEK
	opNEK
	opGTK
	opGEK
	opJumpCompareK
	opForLoopV
	opGetArrayI
	opSetArrayI
	opGetFieldK
	opSetFieldK
	opSwapTable
	opAddTable
)

const (
	compareEQ uint8 = iota
	compareNE
	compareLT
	compareLE
	compareGT
	compareGE
)

func abc(op opcode, a, b, c uint8) Instruction {
	return Instruction(op) | Instruction(a)<<8 | Instruction(b)<<16 | Instruction(c)<<24
}
func abx(op opcode, a uint8, bx uint16) Instruction {
	return Instruction(op) | Instruction(a)<<8 | Instruction(bx)<<16
}
func (i Instruction) op() opcode { return opcode(i) }
func (i Instruction) a() int     { return int(uint8(i >> 8)) }
func (i Instruction) b() int     { return int(uint8(i >> 16)) }
func (i Instruction) c() int     { return int(uint8(i >> 24)) }
func (i Instruction) bx() int    { return int(uint16(i >> 16)) }
func (i Instruction) sbx() int   { return int(int16(i >> 16)) }

type Prototype struct {
	Source           string
	Code             []Instruction
	Constants        []Value
	Children         []*Prototype
	Upvalues         []UpvalueDescriptor
	Lines            []int
	Parameters       uint8
	MaxRegisters     uint8
	Vararg           bool
	Captured         bool
	Fast             bool
	NumericPure      bool
	NumericCode      []Instruction
	NumericRegisters uint8
	NumericFormula   *numericFormula
	FieldCaches      []fieldCache
}

// numericFormula is a compiler-produced straight-line specialization for a
// two-argument numeric function whose result is a ratio of quadratic
// polynomials. It covers common kernels without runtime bytecode dispatch.
type numericFormula struct {
	numerator   [6]float64
	denominator [6]float64
}

type UpvalueDescriptor struct {
	Index uint8
	Local bool
}

type State struct {
	globals          *Table
	stack            []cell
	top              int
	frames           []frame
	scratchArgs      [8]Value
	callArgs         []Value
	argTop           int
	Output           io.Writer
	heap             []*Table
	freeTables       []*Table
	tableBlocks      [][]Table
	tableBlockUsed   int
	tableAllocations int
	nextCollection   int
	gcGeneration     uint64
	currentThread    *Thread
	shapes           map[string]*tableShape
	randomState      uint64
}

type frame struct {
	fn         *Function
	regs       []cell
	pc         int
	varargs    []Value
	multi      callResult
	stackBase  int
	heap       bool
	returnBase int
	returnWant int
}

func NewState() *State {
	s := &State{
		globals:     NewTable(0, 64),
		stack:       make([]cell, 8192),
		frames:      make([]frame, 0, 32),
		callArgs:    make([]Value, 8192),
		randomState: 0x9e3779b97f4a7c15,
	}
	s.openLibraries()
	s.nextCollection = 1024
	return s
}

func (s *State) Globals() *Table { return s.globals }
