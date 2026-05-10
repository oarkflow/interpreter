package database

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/oarkflow/interpreter/pkg/ast"
	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/lexer"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/parser"
	"github.com/oarkflow/interpreter/pkg/token"
)

// ── QueryBuilder Object ────────────────────────────────────────────

type QueryBuilder struct {
	mu         sync.Mutex
	db         object.Object // *object.DB or *object.DBTx
	tableName  string
	selectCols []string
	whereConds []whereClause
	orderBy    []string
	limitVal   int
	offsetVal  int
	joins      []string
	groupBy    []string
	havingCond string
	built      bool
	query      string
	args       []any
}

type whereClause struct {
	column   string
	operator string
	value    any
	raw      string // for raw where clauses
}

func (qb *QueryBuilder) Type() object.ObjectType { return object.QUERY_BUILDER_OBJ }
func (qb *QueryBuilder) Inspect() string {
	q, _ := qb.buildQuery()
	return fmt.Sprintf("<query: %s>", q)
}

func (qb *QueryBuilder) buildQuery() (string, []any) {
	qb.mu.Lock()
	defer qb.mu.Unlock()

	var sb strings.Builder
	var args []any

	// SELECT
	sb.WriteString("SELECT ")
	if len(qb.selectCols) > 0 {
		sb.WriteString(strings.Join(qb.selectCols, ", "))
	} else {
		sb.WriteString("*")
	}

	// FROM
	sb.WriteString(" FROM ")
	sb.WriteString(qb.tableName)

	// JOINS
	for _, j := range qb.joins {
		sb.WriteString(" ")
		sb.WriteString(j)
	}

	// WHERE
	if len(qb.whereConds) > 0 {
		sb.WriteString(" WHERE ")
		for i, w := range qb.whereConds {
			if i > 0 {
				sb.WriteString(" AND ")
			}
			if w.raw != "" {
				sb.WriteString(w.raw)
				switch vals := w.value.(type) {
				case []any:
					args = append(args, vals...)
				case nil:
				default:
					args = append(args, vals)
				}
			} else {
				sb.WriteString(fmt.Sprintf("%s %s ?", w.column, w.operator))
				args = append(args, w.value)
			}
		}
	}

	// GROUP BY
	if len(qb.groupBy) > 0 {
		sb.WriteString(" GROUP BY ")
		sb.WriteString(strings.Join(qb.groupBy, ", "))
	}

	// HAVING
	if qb.havingCond != "" {
		sb.WriteString(" HAVING ")
		sb.WriteString(qb.havingCond)
	}

	// ORDER BY
	if len(qb.orderBy) > 0 {
		sb.WriteString(" ORDER BY ")
		sb.WriteString(strings.Join(qb.orderBy, ", "))
	}

	// LIMIT
	if qb.limitVal > 0 {
		sb.WriteString(fmt.Sprintf(" LIMIT %d", qb.limitVal))
	}

	// OFFSET
	if qb.offsetVal > 0 {
		sb.WriteString(fmt.Sprintf(" OFFSET %d", qb.offsetVal))
	}

	return sb.String(), args
}

// ── Lazy DB Query ──────────────────────────────────────────────────

type LazyDBQuery struct {
	mu        sync.Mutex
	builder   *QueryBuilder
	env       *object.Environment
	evaluated bool
	result    object.Object
}

func (lq *LazyDBQuery) Type() object.ObjectType { return object.LAZY_DB_QUERY_OBJ }
func (lq *LazyDBQuery) Inspect() string {
	lq.mu.Lock()
	defer lq.mu.Unlock()
	if lq.evaluated {
		return lq.result.Inspect()
	}
	return "<lazy query: unevaluated>"
}

func (lq *LazyDBQuery) Force() object.Object {
	lq.mu.Lock()
	defer lq.mu.Unlock()
	if lq.evaluated {
		return lq.result
	}

	query, args := lq.builder.buildQuery()
	lq.result = executeDBQuery(lq.builder.db, query, args, lq.env)
	lq.evaluated = true
	return lq.result
}

func executeDBQuery(dbObj object.Object, query string, args []any, env *object.Environment) object.Object {
	target, errObj := dbTarget(dbObj)
	if errObj != nil {
		return dbTupleResult(object.NULL, fmt.Sprintf("invalid db target: %s", errObj.Inspect()))
	}

	ctx := context.Background()
	if env != nil {
		ctx = runtimeContext(env)
	}

	// Convert args to interface slice
	ifaces := make([]any, len(args))
	copy(ifaces, args)

	rows, err := target.QueryxContext(ctx, query, ifaces...)
	if err != nil {
		return dbTupleResult(object.NULL, err.Error())
	}
	defer rows.Close()

	var results []object.Object
	for rows.Next() {
		row := make(map[string]any)
		if err := rows.MapScan(row); err != nil {
			return dbTupleResult(object.NULL, err.Error())
		}
		h := &object.Hash{Pairs: make(map[object.HashKey]object.HashPair)}
		for k, v := range row {
			key := &object.String{Value: k}
			h.Pairs[key.HashKey()] = object.HashPair{Key: key, Value: toObject(v)}
		}
		results = append(results, h)
	}
	if err := rows.Err(); err != nil {
		return dbTupleResult(object.NULL, err.Error())
	}
	return dbTupleResult(&object.Array{Elements: results}, "")
}

// ── QueryBuilder Method Dispatch ───────────────────────────────────

func GetQueryBuilderProperty(qb *QueryBuilder, name string) object.Object {
	switch name {
	case "from":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return object.NewError("from() requires a table name")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return object.NewError("from() argument must be a string")
			}
			qb.mu.Lock()
			qb.tableName = s.Value
			qb.mu.Unlock()
			return qb
		}}
	case "select":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			qb.mu.Lock()
			for _, arg := range args {
				switch v := arg.(type) {
				case *object.String:
					qb.selectCols = append(qb.selectCols, v.Value)
				case *object.Array:
					for _, el := range v.Elements {
						if s, ok := el.(*object.String); ok {
							qb.selectCols = append(qb.selectCols, s.Value)
						}
					}
				}
			}
			qb.mu.Unlock()
			return qb
		}}
	case "where":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 2 {
				return object.NewError("where() requires at least (column, value) or (column, op, value)")
			}
			col, ok := args[0].(*object.String)
			if !ok {
				return object.NewError("where() column must be a string")
			}
			qb.mu.Lock()
			if len(args) == 2 {
				qb.whereConds = append(qb.whereConds, whereClause{
					column:   col.Value,
					operator: "=",
					value:    splToGoValue(args[1]),
				})
			} else {
				op, ok := args[1].(*object.String)
				if !ok {
					qb.mu.Unlock()
					return object.NewError("where() operator must be a string")
				}
				qb.whereConds = append(qb.whereConds, whereClause{
					column:   col.Value,
					operator: op.Value,
					value:    splToGoValue(args[2]),
				})
			}
			qb.mu.Unlock()
			return qb
		}}
	case "where_raw":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return object.NewError("where_raw() requires a SQL condition")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return object.NewError("where_raw() argument must be a string")
			}
			qb.mu.Lock()
			qb.whereConds = append(qb.whereConds, whereClause{raw: s.Value})
			qb.mu.Unlock()
			return qb
		}}
	case "order_by":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			for _, arg := range args {
				if s, ok := arg.(*object.String); ok {
					qb.mu.Lock()
					qb.orderBy = append(qb.orderBy, s.Value)
					qb.mu.Unlock()
				}
			}
			return qb
		}}
	case "limit":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return object.NewError("limit() requires a number")
			}
			n, ok := args[0].(*object.Integer)
			if !ok {
				return object.NewError("limit() argument must be an integer")
			}
			qb.mu.Lock()
			qb.limitVal = int(n.Value)
			qb.mu.Unlock()
			return qb
		}}
	case "offset":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return object.NewError("offset() requires a number")
			}
			n, ok := args[0].(*object.Integer)
			if !ok {
				return object.NewError("offset() argument must be an integer")
			}
			qb.mu.Lock()
			qb.offsetVal = int(n.Value)
			qb.mu.Unlock()
			return qb
		}}
	case "join":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return object.NewError("join() requires a join clause")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return object.NewError("join() argument must be a string")
			}
			joinType := "JOIN"
			if len(args) >= 2 {
				if jt, ok := args[1].(*object.String); ok {
					joinType = strings.ToUpper(jt.Value)
				}
			}
			qb.mu.Lock()
			qb.joins = append(qb.joins, fmt.Sprintf("%s %s", joinType, s.Value))
			qb.mu.Unlock()
			return qb
		}}
	case "group_by":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			for _, arg := range args {
				if s, ok := arg.(*object.String); ok {
					qb.mu.Lock()
					qb.groupBy = append(qb.groupBy, s.Value)
					qb.mu.Unlock()
				}
			}
			return qb
		}}
	case "exec":
		return &object.Builtin{FnWithEnv: func(env *object.Environment, args ...object.Object) object.Object {
			query, qArgs := qb.buildQuery()
			return executeDBQuery(qb.db, query, qArgs, env)
		}}
	case "lazy":
		return &object.Builtin{FnWithEnv: func(env *object.Environment, args ...object.Object) object.Object {
			return &LazyDBQuery{
				builder: qb,
				env:     env,
			}
		}}
	case "sql":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			q, _ := qb.buildQuery()
			return &object.String{Value: q}
		}}
	case "match", "where_match":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return object.NewError("match() requires a pattern string")
			}
			patternArg, ok := args[0].(*object.String)
			if !ok {
				return object.NewError("match() pattern must be a string")
			}
			pattern, err := parsePatternString(patternArg.Value)
			if err != nil {
				return object.NewError("match() invalid pattern: %s", err)
			}
			clauses, err := buildWhereClausesFromPattern(pattern, "")
			if err != nil {
				return object.NewError("match() unsupported pattern: %s", err)
			}
			qb.mu.Lock()
			qb.whereConds = append(qb.whereConds, clauses...)
			qb.mu.Unlock()
			return qb
		}}
	case "match_case", "where_match_case":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 2 {
				return object.NewError("match_case() requires a pattern and config")
			}
			patternArg, ok := args[0].(*object.String)
			if !ok {
				return object.NewError("match_case() pattern must be a string")
			}
			cfg, ok := args[1].(*object.Hash)
			if !ok {
				return object.NewError("match_case() config must be a hash")
			}
			pattern, err := parsePatternString(patternArg.Value)
			if err != nil {
				return object.NewError("match_case() invalid pattern: %s", err)
			}
			prefix := hashStringValue(cfg, "prefix")
			clauses, err := buildWhereClausesFromPattern(pattern, prefixOrEmpty(prefix))
			if err != nil {
				return object.NewError("match_case() unsupported pattern: %s", err)
			}
			qb.mu.Lock()
			qb.whereConds = append(qb.whereConds, clauses...)
			if cols, ok := hashStringArrayValue(cfg, "select"); ok && len(cols) > 0 {
				qb.selectCols = append(qb.selectCols, cols...)
			}
			qb.mu.Unlock()
			return qb
		}}
	case "decode", "decode_match":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return object.NewError("decode() requires a pattern string")
			}
			patternArg, ok := args[0].(*object.String)
			if !ok {
				return object.NewError("decode() pattern must be a string")
			}
			result := executeDBQuery(qb.db, mustQuery(qb), mustArgs(qb), nil)
			arr, errMsg := unpackDBRows(result)
			if errMsg != "" {
				return object.NewError("%s", errMsg)
			}
			pattern, err := parsePatternString(patternArg.Value)
			if err != nil {
				return object.NewError("decode() invalid pattern: %s", err)
			}
			decoded := make([]object.Object, 0, len(arr.Elements))
			for _, row := range arr.Elements {
				env := object.NewEnclosedEnvironment(object.NewGlobalEnvironment([]string{}))
				if eval.MatchPattern(pattern, row, env) {
					decoded = append(decoded, envToHash(env))
				}
			}
			return &object.Array{Elements: decoded}
		}}
	default:
		return nil
	}
}

// ── Helpers ────────────────────────────────────────────────────────

func splToGoValue(obj object.Object) any {
	switch v := obj.(type) {
	case *object.Integer:
		return v.Value
	case *object.Float:
		return v.Value
	case *object.String:
		return v.Value
	case *object.Boolean:
		return v.Value
	case *object.Null:
		return nil
	default:
		return obj.Inspect()
	}
}

func parsePatternString(pattern string) (ast.Pattern, error) {
	src := "match (__db__) { case " + pattern + " => true }"
	l := lexer.NewLexer(src)
	p := parser.NewParser(l)
	program := p.ParseProgram()
	if errs := p.Errors(); len(errs) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	if len(program.Statements) == 0 {
		return nil, fmt.Errorf("empty pattern")
	}
	exprStmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		return nil, fmt.Errorf("expected expression statement")
	}
	matchExpr, ok := exprStmt.Expression.(*ast.MatchExpression)
	if !ok || len(matchExpr.Cases) == 0 {
		return nil, fmt.Errorf("expected match expression")
	}
	return matchExpr.Cases[0].Pattern, nil
}

func buildWhereClausesFromPattern(pattern ast.Pattern, prefix string) ([]whereClause, error) {
	switch p := pattern.(type) {
	case *ast.ObjectPattern:
		var clauses []whereClause
		for i, key := range p.Keys {
			fullKey := key
			if prefix != "" {
				fullKey = prefix + "." + key
			}
			nested, err := buildWhereClausesFromPattern(p.Patterns[i], fullKey)
			if err != nil {
				return nil, err
			}
			clauses = append(clauses, nested...)
		}
		return clauses, nil
	case *ast.LiteralPattern:
		if prefix == "" {
			return nil, fmt.Errorf("literal pattern requires a field")
		}
		val := eval.Eval(p.Value, object.NewGlobalEnvironment([]string{}))
		if object.IsError(val) {
			return nil, fmt.Errorf("literal evaluation failed")
		}
		return []whereClause{{column: prefix, operator: "=", value: splToGoValue(val)}}, nil
	case *ast.ComparisonPattern:
		if prefix == "" {
			return nil, fmt.Errorf("comparison pattern requires a field")
		}
		val := eval.Eval(p.Value, object.NewGlobalEnvironment([]string{}))
		if object.IsError(val) {
			return nil, fmt.Errorf("comparison evaluation failed")
		}
		return []whereClause{{column: prefix, operator: sqlOperatorForToken(p.Operator), value: splToGoValue(val)}}, nil
	case *ast.BindingPattern, *ast.WildcardPattern:
		return nil, nil
	case *ast.OrPattern:
		var rawParts []string
		var rawArgs []any
		for _, sub := range p.Patterns {
			clauses, err := buildWhereClausesFromPattern(sub, prefix)
			if err != nil {
				return nil, err
			}
			if len(clauses) != 1 || clauses[0].raw != "" {
				return nil, fmt.Errorf("or pattern supports simple comparisons only")
			}
			rawParts = append(rawParts, fmt.Sprintf("%s %s ?", clauses[0].column, clauses[0].operator))
			rawArgs = append(rawArgs, clauses[0].value)
		}
		return []whereClause{{raw: strings.Join(rawParts, " OR "), value: rawArgs}}, nil
	default:
		return nil, fmt.Errorf("pattern %T is not supported for DB translation", pattern)
	}
}

func sqlOperatorForToken(t token.TokenType) string {
	switch t {
	case token.LT:
		return "<"
	case token.GT:
		return ">"
	case token.LTE:
		return "<="
	case token.GTE:
		return ">="
	case token.NEQ:
		return "!="
	case token.EQ:
		return "="
	default:
		return fmt.Sprintf("%d", t)
	}
}

func mustQuery(qb *QueryBuilder) string {
	q, _ := qb.buildQuery()
	return q
}

func mustArgs(qb *QueryBuilder) []any {
	_, args := qb.buildQuery()
	return args
}

func unpackDBRows(result object.Object) (*object.Array, string) {
	tuple, ok := result.(*object.Array)
	if !ok || len(tuple.Elements) < 2 {
		return nil, "invalid db query result"
	}
	if tuple.Elements[1] != object.NULL {
		if errStr, ok := tuple.Elements[1].(*object.String); ok && errStr.Value != "" {
			return nil, errStr.Value
		}
	}
	rows, ok := tuple.Elements[0].(*object.Array)
	if !ok {
		return nil, "query result rows missing"
	}
	return rows, ""
}

func envToHash(env *object.Environment) *object.Hash {
	h := &object.Hash{Pairs: make(map[object.HashKey]object.HashPair)}
	for k, v := range env.Store {
		key := &object.String{Value: k}
		h.Pairs[key.HashKey()] = object.HashPair{Key: key, Value: v}
	}
	return h
}

func hashStringArrayValue(h *object.Hash, key string) ([]string, bool) {
	v, ok := hashLookup(h, key)
	if !ok {
		return nil, false
	}
	arr, ok := v.(*object.Array)
	if !ok {
		return nil, false
	}
	result := make([]string, 0, len(arr.Elements))
	for _, el := range arr.Elements {
		s, ok := el.(*object.String)
		if !ok {
			continue
		}
		result = append(result, s.Value)
	}
	return result, true
}

func hashLookup(h *object.Hash, key string) (object.Object, bool) {
	lookup := (&object.String{Value: key}).HashKey()
	pair, ok := h.Pairs[lookup]
	if !ok {
		return nil, false
	}
	return pair.Value, true
}

func hashStringValue(h *object.Hash, key string) string {
	if h == nil {
		return ""
	}
	pair, ok := h.Pairs[(&object.String{Value: key}).HashKey()]
	if !ok {
		return ""
	}
	if s, ok := pair.Value.(*object.String); ok {
		return strings.TrimSpace(s.Value)
	}
	return strings.TrimSpace(pair.Value.Inspect())
}

func prefixOrEmpty(prefix string) string {
	return prefix
}

// ── Builtins ───────────────────────────────────────────────────────

func init() {
	eval.RegisterBuiltins(map[string]*object.Builtin{
		"query":      {Fn: builtinQuery},
		"lazy_query": {FnWithEnv: builtinLazyQuery},
	})

	// Register dot expression hook for QueryBuilder and LazyDBQuery
	prev := eval.DotExpressionHook
	eval.DotExpressionHook = func(left object.Object, name string) object.Object {
		switch obj := left.(type) {
		case *QueryBuilder:
			return GetQueryBuilderProperty(obj, name)
		case *LazyDBQuery:
			return obj.Force()
		}
		if prev != nil {
			return prev(left, name)
		}
		return nil
	}
}

// query(db) -> returns a new QueryBuilder
// query(db, table) -> QueryBuilder with table set
func builtinQuery(args ...object.Object) object.Object {
	if len(args) < 1 {
		return object.NewError("query() requires a database connection")
	}
	db := args[0]
	// Validate it's a DB or DBTx
	switch db.(type) {
	case *object.DB, *object.DBTx:
		// ok
	default:
		return object.NewError("query() first argument must be a database connection, got %s", db.Type())
	}

	qb := &QueryBuilder{
		db: db,
	}
	if len(args) >= 2 {
		if s, ok := args[1].(*object.String); ok {
			qb.tableName = s.Value
		}
	}
	return qb
}

// lazy_query(db, table) -> LazyDBQuery
func builtinLazyQuery(env *object.Environment, args ...object.Object) object.Object {
	if len(args) < 2 {
		return object.NewError("lazy_query() requires (db, table)")
	}
	db := args[0]
	switch db.(type) {
	case *object.DB, *object.DBTx:
	default:
		return object.NewError("lazy_query() first argument must be a database connection")
	}
	table, ok := args[1].(*object.String)
	if !ok {
		return object.NewError("lazy_query() second argument must be a table name")
	}

	qb := &QueryBuilder{
		db:        db,
		tableName: table.Value,
	}
	return &LazyDBQuery{
		builder: qb,
		env:     env,
	}
}
