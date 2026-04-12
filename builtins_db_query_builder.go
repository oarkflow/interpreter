package interpreter

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// ── QueryBuilder Object ────────────────────────────────────────────

const QUERY_BUILDER_OBJ ObjectType = 104

type QueryBuilder struct {
	mu         sync.Mutex
	db         Object // *DB or *DBTx
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

type QueryMatchCase struct {
	Pattern Pattern
	Where   []whereClause
	Select  []string
	Guard   string
}

func (qb *QueryBuilder) Type() ObjectType { return QUERY_BUILDER_OBJ }
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

const LAZY_DB_QUERY_OBJ ObjectType = 105

type LazyDBQuery struct {
	mu        sync.Mutex
	builder   *QueryBuilder
	env       *Environment
	evaluated bool
	result    Object
}

func (lq *LazyDBQuery) Type() ObjectType { return LAZY_DB_QUERY_OBJ }
func (lq *LazyDBQuery) Inspect() string {
	lq.mu.Lock()
	defer lq.mu.Unlock()
	if lq.evaluated {
		return lq.result.Inspect()
	}
	return "<lazy query: unevaluated>"
}

func (lq *LazyDBQuery) Force() Object {
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

func executeDBQuery(dbObj Object, query string, args []any, env *Environment) Object {
	target, errObj := dbTarget(dbObj)
	if errObj != nil {
		return dbTupleResult(NULL, fmt.Sprintf("invalid db target: %s", errObj.Inspect()))
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
		return dbTupleResult(NULL, err.Error())
	}
	defer rows.Close()

	var results []Object
	for rows.Next() {
		row := make(map[string]any)
		if err := rows.MapScan(row); err != nil {
			return dbTupleResult(NULL, err.Error())
		}
		h := &Hash{Pairs: make(map[HashKey]HashPair)}
		for k, v := range row {
			key := &String{Value: k}
			h.Pairs[key.HashKey()] = HashPair{Key: key, Value: goToSPLObject(v)}
		}
		results = append(results, h)
	}
	if err := rows.Err(); err != nil {
		return dbTupleResult(NULL, err.Error())
	}
	return dbTupleResult(&Array{Elements: results}, "")
}

// ── QueryBuilder Method Dispatch ───────────────────────────────────

func GetQueryBuilderProperty(qb *QueryBuilder, name string) Object {
	switch name {
	case "from":
		return &Builtin{Fn: func(args ...Object) Object {
			if len(args) < 1 {
				return newError("from() requires a table name")
			}
			s, ok := args[0].(*String)
			if !ok {
				return newError("from() argument must be a string")
			}
			qb.mu.Lock()
			qb.tableName = s.Value
			qb.mu.Unlock()
			return qb
		}}
	case "select":
		return &Builtin{Fn: func(args ...Object) Object {
			qb.mu.Lock()
			for _, arg := range args {
				switch v := arg.(type) {
				case *String:
					qb.selectCols = append(qb.selectCols, v.Value)
				case *Array:
					for _, el := range v.Elements {
						if s, ok := el.(*String); ok {
							qb.selectCols = append(qb.selectCols, s.Value)
						}
					}
				}
			}
			qb.mu.Unlock()
			return qb
		}}
	case "where":
		return &Builtin{Fn: func(args ...Object) Object {
			if len(args) < 2 {
				return newError("where() requires at least (column, value) or (column, op, value)")
			}
			col, ok := args[0].(*String)
			if !ok {
				return newError("where() column must be a string")
			}
			qb.mu.Lock()
			if len(args) == 2 {
				qb.whereConds = append(qb.whereConds, whereClause{
					column:   col.Value,
					operator: "=",
					value:    splToGoValue(args[1]),
				})
			} else {
				op, ok := args[1].(*String)
				if !ok {
					qb.mu.Unlock()
					return newError("where() operator must be a string")
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
		return &Builtin{Fn: func(args ...Object) Object {
			if len(args) < 1 {
				return newError("where_raw() requires a SQL condition")
			}
			s, ok := args[0].(*String)
			if !ok {
				return newError("where_raw() argument must be a string")
			}
			qb.mu.Lock()
			qb.whereConds = append(qb.whereConds, whereClause{raw: s.Value})
			qb.mu.Unlock()
			return qb
		}}
	case "order_by":
		return &Builtin{Fn: func(args ...Object) Object {
			for _, arg := range args {
				if s, ok := arg.(*String); ok {
					qb.mu.Lock()
					qb.orderBy = append(qb.orderBy, s.Value)
					qb.mu.Unlock()
				}
			}
			return qb
		}}
	case "limit":
		return &Builtin{Fn: func(args ...Object) Object {
			if len(args) < 1 {
				return newError("limit() requires a number")
			}
			n, ok := args[0].(*Integer)
			if !ok {
				return newError("limit() argument must be an integer")
			}
			qb.mu.Lock()
			qb.limitVal = int(n.Value)
			qb.mu.Unlock()
			return qb
		}}
	case "offset":
		return &Builtin{Fn: func(args ...Object) Object {
			if len(args) < 1 {
				return newError("offset() requires a number")
			}
			n, ok := args[0].(*Integer)
			if !ok {
				return newError("offset() argument must be an integer")
			}
			qb.mu.Lock()
			qb.offsetVal = int(n.Value)
			qb.mu.Unlock()
			return qb
		}}
	case "join":
		return &Builtin{Fn: func(args ...Object) Object {
			if len(args) < 1 {
				return newError("join() requires a join clause")
			}
			s, ok := args[0].(*String)
			if !ok {
				return newError("join() argument must be a string")
			}
			joinType := "JOIN"
			if len(args) >= 2 {
				if jt, ok := args[1].(*String); ok {
					joinType = strings.ToUpper(jt.Value)
				}
			}
			qb.mu.Lock()
			qb.joins = append(qb.joins, fmt.Sprintf("%s %s", joinType, s.Value))
			qb.mu.Unlock()
			return qb
		}}
	case "group_by":
		return &Builtin{Fn: func(args ...Object) Object {
			for _, arg := range args {
				if s, ok := arg.(*String); ok {
					qb.mu.Lock()
					qb.groupBy = append(qb.groupBy, s.Value)
					qb.mu.Unlock()
				}
			}
			return qb
		}}
	case "exec":
		return &Builtin{FnWithEnv: func(env *Environment, args ...Object) Object {
			query, qArgs := qb.buildQuery()
			return executeDBQuery(qb.db, query, qArgs, env)
		}}
	case "lazy":
		return &Builtin{FnWithEnv: func(env *Environment, args ...Object) Object {
			return &LazyDBQuery{
				builder: qb,
				env:     env,
			}
		}}
	case "sql":
		return &Builtin{Fn: func(args ...Object) Object {
			q, _ := qb.buildQuery()
			return &String{Value: q}
		}}
	case "match", "where_match":
		return &Builtin{Fn: func(args ...Object) Object {
			if len(args) < 1 {
				return newError("match() requires a pattern string")
			}
			patternArg, ok := args[0].(*String)
			if !ok {
				return newError("match() pattern must be a string")
			}
			pattern, err := ParsePatternString(patternArg.Value)
			if err != nil {
				return newError("match() invalid pattern: %s", err)
			}
			clauses, err := buildWhereClausesFromPattern(pattern, "")
			if err != nil {
				return newError("match() unsupported pattern: %s", err)
			}
			qb.mu.Lock()
			qb.whereConds = append(qb.whereConds, clauses...)
			qb.mu.Unlock()
			return qb
		}}
	case "match_case", "where_match_case":
		return &Builtin{Fn: func(args ...Object) Object {
			if len(args) < 2 {
				return newError("match_case() requires a pattern and config")
			}
			patternArg, ok := args[0].(*String)
			if !ok {
				return newError("match_case() pattern must be a string")
			}
			cfg, ok := args[1].(*Hash)
			if !ok {
				return newError("match_case() config must be a hash")
			}
			pattern, err := ParsePatternString(patternArg.Value)
			if err != nil {
				return newError("match_case() invalid pattern: %s", err)
			}
			prefix := hashStringValue(cfg, "prefix")
			clauses, err := buildWhereClausesFromPattern(pattern, prefixOrEmpty(prefix))
			if err != nil {
				return newError("match_case() unsupported pattern: %s", err)
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
		return &Builtin{Fn: func(args ...Object) Object {
			if len(args) < 1 {
				return newError("decode() requires a pattern string")
			}
			patternArg, ok := args[0].(*String)
			if !ok {
				return newError("decode() pattern must be a string")
			}
			result := executeDBQuery(qb.db, mustQuery(qb), mustArgs(qb), nil)
			arr, errMsg := unpackDBRows(result)
			if errMsg != "" {
				return newError("%s", errMsg)
			}
			pattern, err := ParsePatternString(patternArg.Value)
			if err != nil {
				return newError("decode() invalid pattern: %s", err)
			}
			decoded := make([]Object, 0, len(arr.Elements))
			for _, row := range arr.Elements {
				env := NewEnclosedEnvironment(NewGlobalEnvironment([]string{}))
				if MatchPattern(pattern, row, env) {
					decoded = append(decoded, envToHash(env))
				}
			}
			return &Array{Elements: decoded}
		}}
	default:
		return nil
	}
}

func splToGoValue(obj Object) any {
	switch v := obj.(type) {
	case *Integer:
		return v.Value
	case *Float:
		return v.Value
	case *String:
		return v.Value
	case *Boolean:
		return v.Value
	case *Null:
		return nil
	default:
		return obj.Inspect()
	}
}

func ParsePatternString(pattern string) (Pattern, error) {
	src := "match (__db__) { case " + pattern + " => true }"
	l := NewLexer(src)
	p := NewParser(l)
	program := p.ParseProgram()
	if errs := p.Errors(); len(errs) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	if len(program.Statements) == 0 {
		return nil, fmt.Errorf("empty pattern")
	}
	exprStmt, ok := program.Statements[0].(*ExpressionStatement)
	if !ok {
		return nil, fmt.Errorf("expected expression statement")
	}
	matchExpr, ok := exprStmt.Expression.(*MatchExpression)
	if !ok || len(matchExpr.Cases) == 0 {
		return nil, fmt.Errorf("expected match expression")
	}
	return matchExpr.Cases[0].Pattern, nil
}

func buildWhereClausesFromPattern(pattern Pattern, prefix string) ([]whereClause, error) {
	switch p := pattern.(type) {
	case *ObjectPattern:
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
	case *LiteralPattern:
		if prefix == "" {
			return nil, fmt.Errorf("literal pattern requires a field")
		}
		val := Eval(p.Value, NewGlobalEnvironment([]string{}))
		if isError(val) {
			return nil, fmt.Errorf("literal evaluation failed")
		}
		return []whereClause{{column: prefix, operator: "=", value: splToGoValue(val)}}, nil
	case *ComparisonPattern:
		if prefix == "" {
			return nil, fmt.Errorf("comparison pattern requires a field")
		}
		val := Eval(p.Value, NewGlobalEnvironment([]string{}))
		if isError(val) {
			return nil, fmt.Errorf("comparison evaluation failed")
		}
		return []whereClause{{column: prefix, operator: sqlOperatorForToken(p.Operator), value: splToGoValue(val)}}, nil
	case *BindingPattern, *WildcardPattern:
		return nil, nil
	case *OrPattern:
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

func sqlOperatorForToken(t TokenType) string {
	switch t {
	case TOKEN_LT:
		return "<"
	case TOKEN_GT:
		return ">"
	case TOKEN_LTE:
		return "<="
	case TOKEN_GTE:
		return ">="
	case TOKEN_NEQ:
		return "!="
	case TOKEN_EQ:
		return "="
	default:
		return tokenTypeName(t)
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

func unpackDBRows(result Object) (*Array, string) {
	tuple, ok := result.(*Array)
	if !ok || len(tuple.Elements) < 2 {
		return nil, "invalid db query result"
	}
	if tuple.Elements[1] != NULL {
		if errStr, ok := tuple.Elements[1].(*String); ok && errStr.Value != "" {
			return nil, errStr.Value
		}
	}
	rows, ok := tuple.Elements[0].(*Array)
	if !ok {
		return nil, "query result rows missing"
	}
	return rows, ""
}

func envToHash(env *Environment) *Hash {
	h := &Hash{Pairs: make(map[HashKey]HashPair)}
	for k, v := range env.store {
		key := &String{Value: k}
		h.Pairs[key.HashKey()] = HashPair{Key: key, Value: v}
	}
	return h
}

func hashStringArrayValue(h *Hash, key string) ([]string, bool) {
	v, ok := hashLookup(h, key)
	if !ok {
		return nil, false
	}
	arr, ok := v.(*Array)
	if !ok {
		return nil, false
	}
	result := make([]string, 0, len(arr.Elements))
	for _, el := range arr.Elements {
		s, ok := el.(*String)
		if !ok {
			continue
		}
		result = append(result, s.Value)
	}
	return result, true
}

func hashLookup(h *Hash, key string) (Object, bool) {
	lookup := (&String{Value: key}).HashKey()
	pair, ok := h.Pairs[lookup]
	if !ok {
		return nil, false
	}
	return pair.Value, true
}

func prefixOrEmpty(prefix string) string {
	return prefix
}

// ── Builtins ───────────────────────────────────────────────────────

func init() {
	registerBuiltins(map[string]*Builtin{
		"query":      {Fn: builtinQuery},
		"lazy_query": {FnWithEnv: builtinLazyQuery},
	})
}

// query(db) -> returns a new QueryBuilder
// query(db, table) -> QueryBuilder with table set
func builtinQuery(args ...Object) Object {
	if len(args) < 1 {
		return newError("query() requires a database connection")
	}
	db := args[0]
	// Validate it's a DB or DBTx
	switch db.(type) {
	case *DB, *DBTx:
		// ok
	default:
		return newError("query() first argument must be a database connection, got %s", db.Type())
	}

	qb := &QueryBuilder{
		db: db,
	}
	if len(args) >= 2 {
		if s, ok := args[1].(*String); ok {
			qb.tableName = s.Value
		}
	}
	return qb
}

// lazy_query(db, table) -> LazyDBQuery
func builtinLazyQuery(env *Environment, args ...Object) Object {
	if len(args) < 2 {
		return newError("lazy_query() requires (db, table)")
	}
	db := args[0]
	switch db.(type) {
	case *DB, *DBTx:
	default:
		return newError("lazy_query() first argument must be a database connection")
	}
	table, ok := args[1].(*String)
	if !ok {
		return newError("lazy_query() second argument must be a table name")
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
