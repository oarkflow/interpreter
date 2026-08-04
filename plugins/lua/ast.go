package lua

type node interface{ lineNumber() int }
type expression interface {
	node
	expressionNode()
}
type statement interface {
	node
	statementNode()
}

type block struct{ statements []statement }

type literalExpression struct {
	line  int
	value Value
}

func (*literalExpression) expressionNode()   {}
func (n *literalExpression) lineNumber() int { return n.line }

type nameExpression struct {
	line int
	name string
}

func (*nameExpression) expressionNode()   {}
func (n *nameExpression) lineNumber() int { return n.line }

type varargExpression struct{ line int }

func (*varargExpression) expressionNode()   {}
func (n *varargExpression) lineNumber() int { return n.line }

type unaryExpression struct {
	line     int
	operator tokenKind
	value    expression
}

func (*unaryExpression) expressionNode()   {}
func (n *unaryExpression) lineNumber() int { return n.line }

type binaryExpression struct {
	line        int
	operator    tokenKind
	left, right expression
}

func (*binaryExpression) expressionNode()   {}
func (n *binaryExpression) lineNumber() int { return n.line }

type indexExpression struct {
	line       int
	table, key expression
}

func (*indexExpression) expressionNode()   {}
func (n *indexExpression) lineNumber() int { return n.line }

type callExpression struct {
	line     int
	function expression
	receiver expression
	args     []expression
}

func (*callExpression) expressionNode()   {}
func (n *callExpression) lineNumber() int { return n.line }

type functionExpression struct {
	line       int
	parameters []string
	vararg     bool
	body       *block
}

func (*functionExpression) expressionNode()   {}
func (n *functionExpression) lineNumber() int { return n.line }

type tableField struct {
	key, value expression
	list       bool
}
type tableExpression struct {
	line   int
	fields []tableField
}

func (*tableExpression) expressionNode()   {}
func (n *tableExpression) lineNumber() int { return n.line }

type assignStatement struct {
	line            int
	targets, values []expression
}

func (*assignStatement) statementNode()    {}
func (n *assignStatement) lineNumber() int { return n.line }

type localStatement struct {
	line   int
	names  []string
	values []expression
}

func (*localStatement) statementNode()    {}
func (n *localStatement) lineNumber() int { return n.line }

type callStatement struct {
	line int
	call *callExpression
}

func (*callStatement) statementNode()    {}
func (n *callStatement) lineNumber() int { return n.line }

type returnStatement struct {
	line   int
	values []expression
}

func (*returnStatement) statementNode()    {}
func (n *returnStatement) lineNumber() int { return n.line }

type breakStatement struct{ line int }

func (*breakStatement) statementNode()    {}
func (n *breakStatement) lineNumber() int { return n.line }

type doStatement struct {
	line int
	body *block
}

func (*doStatement) statementNode()    {}
func (n *doStatement) lineNumber() int { return n.line }

type whileStatement struct {
	line      int
	condition expression
	body      *block
}

func (*whileStatement) statementNode()    {}
func (n *whileStatement) lineNumber() int { return n.line }

type repeatStatement struct {
	line      int
	body      *block
	condition expression
}

func (*repeatStatement) statementNode()    {}
func (n *repeatStatement) lineNumber() int { return n.line }

type ifBranch struct {
	condition expression
	body      *block
}
type ifStatement struct {
	line      int
	branches  []ifBranch
	otherwise *block
}

func (*ifStatement) statementNode()    {}
func (n *ifStatement) lineNumber() int { return n.line }

type numericForStatement struct {
	line                 int
	name                 string
	initial, limit, step expression
	body                 *block
}

func (*numericForStatement) statementNode()    {}
func (n *numericForStatement) lineNumber() int { return n.line }

type genericForStatement struct {
	line   int
	names  []string
	values []expression
	body   *block
}

func (*genericForStatement) statementNode()    {}
func (n *genericForStatement) lineNumber() int { return n.line }
