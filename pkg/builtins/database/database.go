package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/security"
	"github.com/oarkflow/squealx"
	"github.com/oarkflow/squealx/drivers/mysql"
	"github.com/oarkflow/squealx/drivers/postgres"
	"github.com/oarkflow/squealx/drivers/sqlite"
)

type dbQueryable interface {
	DriverName() string
	QueryxContext(ctx context.Context, query string, args ...any) (*squealx.Rows, error)
	NamedQueryContext(ctx context.Context, query string, arg any) (*squealx.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	NamedExecContext(ctx context.Context, query string, arg any) (sql.Result, error)
}

// ── Helpers ────────────────────────────────────────────────────────

func runtimeContext(env *object.Environment) context.Context {
	if env != nil && env.RuntimeLimits != nil && env.RuntimeLimits.Ctx != nil {
		return env.RuntimeLimits.Ctx
	}
	return context.Background()
}

func objectToNative(obj object.Object) interface{} {
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
	case *object.Array:
		result := make([]interface{}, len(v.Elements))
		for i, el := range v.Elements {
			result[i] = objectToNative(el)
		}
		return result
	case *object.Hash:
		result := make(map[string]interface{})
		for _, pair := range v.Pairs {
			result[pair.Key.Inspect()] = objectToNative(pair.Value)
		}
		return result
	default:
		return obj.Inspect()
	}
}

func toObject(val interface{}) object.Object {
	if val == nil {
		return object.NULL
	}
	switch v := val.(type) {
	case object.Object:
		return v
	case bool:
		return object.NativeBoolToBooleanObject(v)
	case int:
		return &object.Integer{Value: int64(v)}
	case int8:
		return &object.Integer{Value: int64(v)}
	case int16:
		return &object.Integer{Value: int64(v)}
	case int32:
		return &object.Integer{Value: int64(v)}
	case int64:
		return &object.Integer{Value: v}
	case uint:
		return &object.Integer{Value: int64(v)}
	case uint8:
		return &object.Integer{Value: int64(v)}
	case uint16:
		return &object.Integer{Value: int64(v)}
	case uint32:
		return &object.Integer{Value: int64(v)}
	case uint64:
		return &object.Integer{Value: int64(v)}
	case float32:
		return &object.Float{Value: float64(v)}
	case float64:
		return &object.Float{Value: v}
	case string:
		return &object.String{Value: v}
	case []byte:
		return &object.String{Value: string(v)}
	default:
		return &object.String{Value: fmt.Sprintf("%v", v)}
	}
}

func asString(arg object.Object, name string) (string, object.Object) {
	if s, ok := arg.(*object.Secret); ok {
		return s.Value, nil
	}
	if arg.Type() != object.STRING_OBJ {
		return "", object.NewError("argument `%s` must be STRING, got %s", name, arg.Type())
	}
	return arg.(*object.String).Value, nil
}

func dbTupleResult(result object.Object, errMsg string) object.Object {
	if errMsg == "" {
		return &object.Array{Elements: []object.Object{result, object.NULL}}
	}
	return &object.Array{Elements: []object.Object{object.NULL, &object.String{Value: errMsg}}}
}

func dbTupleBool(ok bool, errMsg string) object.Object {
	if ok {
		return &object.Array{Elements: []object.Object{object.TRUE, object.NULL}}
	}
	return &object.Array{Elements: []object.Object{object.FALSE, &object.String{Value: errMsg}}}
}

func dbTarget(obj object.Object) (dbQueryable, object.Object) {
	switch v := obj.(type) {
	case *object.DB:
		return v, nil
	case *object.DBTx:
		return v, nil
	default:
		return nil, object.NewError("first argument must be a database connection or transaction")
	}
}

func dbTxTarget(obj object.Object) (*object.DBTx, object.Object) {
	tx, ok := obj.(*object.DBTx)
	if !ok {
		return nil, object.NewError("argument must be a database transaction (use db_begin)")
	}
	return tx, nil
}

func objectToDBNamedArgs(obj object.Object) (map[string]any, object.Object) {
	if obj == nil || obj == object.NULL {
		return nil, nil
	}
	hash, ok := obj.(*object.Hash)
	if !ok {
		return nil, object.NewError("params must be ARRAY or HASH, got %s", obj.Type())
	}
	out := make(map[string]any, len(hash.Pairs))
	for _, pair := range hash.Pairs {
		key, ok := pair.Key.(*object.String)
		if !ok {
			return nil, object.NewError("named query params require STRING keys")
		}
		out[key.Value] = objectToNative(pair.Value)
	}
	return out, nil
}

func objectToDBPositionalArgs(obj object.Object) ([]any, object.Object) {
	if obj == nil || obj == object.NULL {
		return nil, nil
	}
	arr, ok := obj.(*object.Array)
	if !ok {
		return nil, object.NewError("params must be ARRAY or HASH, got %s", obj.Type())
	}
	out := make([]any, len(arr.Elements))
	for i, el := range arr.Elements {
		out[i] = objectToNative(el)
	}
	return out, nil
}

func parseDBParams(obj object.Object) ([]any, map[string]any, object.Object) {
	if obj == nil || obj == object.NULL {
		return nil, nil, nil
	}
	switch obj.Type() {
	case object.ARRAY_OBJ:
		positional, errObj := objectToDBPositionalArgs(obj)
		return positional, nil, errObj
	case object.HASH_OBJ:
		named, errObj := objectToDBNamedArgs(obj)
		return nil, named, errObj
	default:
		return nil, nil, object.NewError("params must be ARRAY or HASH, got %s", obj.Type())
	}
}

func queryRows(ctx context.Context, target dbQueryable, query string, params object.Object) (*squealx.Rows, object.Object) {
	positional, named, errObj := parseDBParams(params)
	if errObj != nil {
		return nil, errObj
	}
	if named != nil {
		if !squealx.IsNamedQuery(query) {
			return nil, object.NewError("HASH params require named placeholders like :name")
		}
		rows, err := target.NamedQueryContext(ctx, query, named)
		if err != nil {
			return nil, object.NewError("query error: %s", err)
		}
		return rows, nil
	}
	if squealx.IsNamedQuery(query) && len(positional) > 0 {
		return nil, object.NewError("named placeholders require HASH params")
	}
	rows, err := target.QueryxContext(ctx, query, positional...)
	if err != nil {
		return nil, object.NewError("query error: %s", err)
	}
	return rows, nil
}

func execStatement(ctx context.Context, target dbQueryable, query string, params object.Object) (sql.Result, object.Object) {
	positional, named, errObj := parseDBParams(params)
	if errObj != nil {
		return nil, errObj
	}
	if named != nil {
		if !squealx.IsNamedQuery(query) {
			return nil, object.NewError("HASH params require named placeholders like :name")
		}
		result, err := target.NamedExecContext(ctx, query, named)
		if err != nil {
			return nil, object.NewError("exec error: %s", err)
		}
		return result, nil
	}
	if squealx.IsNamedQuery(query) && len(positional) > 0 {
		return nil, object.NewError("named placeholders require HASH params")
	}
	result, err := target.ExecContext(ctx, query, positional...)
	if err != nil {
		return nil, object.NewError("exec error: %s", err)
	}
	return result, nil
}

func queryResultsToObject(rows *squealx.Rows, format string) (object.Object, object.Object) {
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, object.NewError("failed to get columns: %s", err)
	}

	var results []map[string]any
	for rows.Next() {
		row := make(map[string]any)
		if err := rows.MapScan(row); err != nil {
			return nil, object.NewError("failed to scan row: %s", err)
		}
		results = append(results, row)
	}

	if err := rows.Err(); err != nil {
		return nil, object.NewError("error iterating rows: %s", err)
	}

	if len(results) == 0 {
		return &object.Array{Elements: []object.Object{}}, nil
	}

	if format == "array" {
		elements := make([]object.Object, len(results))
		for i, row := range results {
			pairs := make(map[object.HashKey]object.HashPair)
			for col, val := range row {
				key := &object.String{Value: col}
				pairs[key.HashKey()] = object.HashPair{Key: key, Value: toObject(val)}
			}
			elements[i] = &object.Hash{Pairs: pairs}
		}
		return &object.Array{Elements: elements}, nil
	}

	return &object.String{Value: formatTable(columns, results)}, nil
}

// ── Builtins ───────────────────────────────────────────────────────

func init() {
	eval.RegisterBuiltins(map[string]*object.Builtin{
		"db_connect":  {FnWithEnv: builtinDBConnect},
		"db_query":    {FnWithEnv: builtinDBQuery},
		"db_exec":     {FnWithEnv: builtinDBExec},
		"db_begin":    {FnWithEnv: builtinDBBegin},
		"db_commit":   {Fn: builtinDBCommit},
		"db_rollback": {Fn: builtinDBRollback},
		"db_close":    {Fn: builtinDBClose},
		"db_tables":   {FnWithEnv: builtinDBTables},
	})
}

func builtinDBConnect(env *object.Environment, args ...object.Object) object.Object {
	if len(args) != 2 {
		return dbTupleResult(nil, "db_connect requires 2 arguments: driver and connection_string")
	}

	driver, errObj := asString(args[0], "driver")
	if errObj != nil {
		return dbTupleResult(nil, errObj.Inspect())
	}
	connStr, errObj := asString(args[1], "connection_string")
	if errObj != nil {
		return dbTupleResult(nil, errObj.Inspect())
	}
	if err := security.CheckDBAllowed(driver, connStr); err != nil {
		return dbTupleResult(nil, fmt.Sprintf("database policy denied connection: %v", err))
	}

	var (
		db  *squealx.DB
		err error
	)
	switch strings.ToLower(driver) {
	case "postgres", "postgresql":
		db, err = postgres.Open(connStr, "postgres")
	case "mysql":
		db, err = mysql.Open(connStr, "mysql")
	case "sqlite", "sqlite3":
		db, err = sqlite.Open(connStr, "sqlite")
	default:
		return dbTupleResult(nil, fmt.Sprintf("unsupported driver: %s. Supported: postgres, mysql, sqlite", driver))
	}
	if err != nil {
		return dbTupleResult(nil, fmt.Sprintf("failed to open connection: %v", err))
	}
	if err := db.PingContext(runtimeContext(env)); err != nil {
		return dbTupleResult(nil, fmt.Sprintf("failed to ping database: %v", err))
	}
	return dbTupleResult(&object.DB{DB: db}, "")
}

func builtinDBQuery(env *object.Environment, args ...object.Object) object.Object {
	if len(args) < 2 || len(args) > 4 {
		return dbTupleResult(nil, "db_query requires 2-4 arguments: db_or_tx, query [, params] [, format]")
	}
	target, errObj := dbTarget(args[0])
	if errObj != nil {
		return dbTupleResult(nil, errObj.Inspect())
	}
	query, errObj := asString(args[1], "query")
	if errObj != nil {
		return dbTupleResult(nil, errObj.Inspect())
	}

	params := object.Object(object.NULL)
	format := "table"
	if len(args) >= 3 {
		if args[2].Type() == object.STRING_OBJ && len(args) == 3 {
			format = strings.ToLower(args[2].(*object.String).Value)
		} else {
			params = args[2]
		}
	}
	if len(args) == 4 {
		if args[3].Type() != object.STRING_OBJ {
			return dbTupleResult(nil, "format must be STRING when provided")
		}
		format = strings.ToLower(args[3].(*object.String).Value)
	}
	if format != "table" && format != "array" {
		return dbTupleResult(nil, fmt.Sprintf("unsupported db_query format %q", format))
	}

	rows, queryErr := queryRows(runtimeContext(env), target, query, params)
	if queryErr != nil {
		return dbTupleResult(nil, queryErr.Inspect())
	}
	result, formatErr := queryResultsToObject(rows, format)
	if formatErr != nil {
		return dbTupleResult(nil, formatErr.Inspect())
	}
	return dbTupleResult(result, "")
}

func builtinDBExec(env *object.Environment, args ...object.Object) object.Object {
	if len(args) < 2 || len(args) > 3 {
		return dbTupleResult(nil, "db_exec requires 2-3 arguments: db_or_tx, query [, params]")
	}
	target, errObj := dbTarget(args[0])
	if errObj != nil {
		return dbTupleResult(nil, errObj.Inspect())
	}
	query, errObj := asString(args[1], "query")
	if errObj != nil {
		return dbTupleResult(nil, errObj.Inspect())
	}
	params := object.Object(object.NULL)
	if len(args) == 3 {
		params = args[2]
	}

	result, execErr := execStatement(runtimeContext(env), target, query, params)
	if execErr != nil {
		return dbTupleResult(nil, execErr.Inspect())
	}
	rowsAffected, _ := result.RowsAffected()
	lastInsertID, _ := result.LastInsertId()

	pairs := make(map[object.HashKey]object.HashPair, 2)
	for key, value := range map[string]any{
		"rows_affected":  rowsAffected,
		"last_insert_id": lastInsertID,
	} {
		k := &object.String{Value: key}
		pairs[k.HashKey()] = object.HashPair{Key: k, Value: toObject(value)}
	}
	return dbTupleResult(&object.Hash{Pairs: pairs}, "")
}

func builtinDBBegin(env *object.Environment, args ...object.Object) object.Object {
	if len(args) != 1 {
		return dbTupleResult(nil, "db_begin requires 1 argument: db")
	}
	dbObj, ok := args[0].(*object.DB)
	if !ok {
		return dbTupleResult(nil, "argument must be a database connection (use db_connect)")
	}
	tx, err := dbObj.BeginTxx(runtimeContext(env), nil)
	if err != nil {
		return dbTupleResult(nil, fmt.Sprintf("failed to begin transaction: %v", err))
	}
	return dbTupleResult(&object.DBTx{Tx: tx}, "")
}

func builtinDBCommit(args ...object.Object) object.Object {
	if len(args) != 1 {
		return dbTupleBool(false, "db_commit requires 1 argument: tx")
	}
	tx, errObj := dbTxTarget(args[0])
	if errObj != nil {
		return dbTupleBool(false, errObj.Inspect())
	}
	if err := tx.Commit(); err != nil {
		return dbTupleBool(false, fmt.Sprintf("failed to commit transaction: %v", err))
	}
	return dbTupleBool(true, "")
}

func builtinDBRollback(args ...object.Object) object.Object {
	if len(args) != 1 {
		return dbTupleBool(false, "db_rollback requires 1 argument: tx")
	}
	tx, errObj := dbTxTarget(args[0])
	if errObj != nil {
		return dbTupleBool(false, errObj.Inspect())
	}
	if err := tx.Rollback(); err != nil {
		return dbTupleBool(false, fmt.Sprintf("failed to rollback transaction: %v", err))
	}
	return dbTupleBool(true, "")
}

func builtinDBClose(args ...object.Object) object.Object {
	if len(args) != 1 {
		return dbTupleBool(false, "db_close requires 1 argument: db")
	}
	dbObj, ok := args[0].(*object.DB)
	if !ok {
		return dbTupleBool(false, "argument must be a database connection (use db_connect)")
	}
	if err := dbObj.Close(); err != nil {
		return dbTupleBool(false, fmt.Sprintf("failed to close connection: %v", err))
	}
	return dbTupleBool(true, "")
}

func builtinDBTables(env *object.Environment, args ...object.Object) object.Object {
	if len(args) != 1 {
		return dbTupleResult(nil, "db_tables requires 1 argument: db_or_tx")
	}
	target, errObj := dbTarget(args[0])
	if errObj != nil {
		return dbTupleResult(nil, errObj.Inspect())
	}

	var query string
	switch target.DriverName() {
	case "postgres", "pgx":
		query = "SELECT table_name FROM information_schema.tables WHERE table_schema = 'public'"
	case "mysql":
		query = "SHOW TABLES"
	case "sqlite", "sqlite3":
		query = "SELECT name FROM sqlite_master WHERE type='table'"
	default:
		query = "SELECT name FROM sqlite_master WHERE type='table'"
	}

	rows, queryErr := queryRows(runtimeContext(env), target, query, object.NULL)
	if queryErr != nil {
		return dbTupleResult(nil, queryErr.Inspect())
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return dbTupleResult(nil, fmt.Sprintf("failed to scan table name: %v", err))
		}
		tables = append(tables, tableName)
	}
	if err := rows.Err(); err != nil {
		return dbTupleResult(nil, fmt.Sprintf("error iterating tables: %v", err))
	}

	elements := make([]object.Object, len(tables))
	for i, table := range tables {
		elements[i] = &object.String{Value: table}
	}
	return dbTupleResult(&object.Array{Elements: elements}, "")
}

// formatTable formats query results as a table string.
func formatTable(columns []string, results []map[string]any) string {
	var sb strings.Builder

	widths := make([]int, len(columns))
	for i, col := range columns {
		widths[i] = len(col)
	}

	for _, row := range results {
		for i, col := range columns {
			val := fmt.Sprintf("%v", row[col])
			if len(val) > widths[i] {
				widths[i] = len(val)
			}
		}
	}

	for i := range widths {
		if widths[i] > 50 {
			widths[i] = 50
		}
	}

	sb.WriteString("+")
	for _, w := range widths {
		sb.WriteString(strings.Repeat("-", w+2))
		sb.WriteString("+")
	}
	sb.WriteString("\n")

	sb.WriteString("|")
	for i, col := range columns {
		sb.WriteString(" ")
		sb.WriteString(col)
		sb.WriteString(strings.Repeat(" ", widths[i]-len(col)+1))
		sb.WriteString("|")
	}
	sb.WriteString("\n")

	sb.WriteString("+")
	for _, w := range widths {
		sb.WriteString(strings.Repeat("=", w+2))
		sb.WriteString("+")
	}
	sb.WriteString("\n")

	for _, row := range results {
		sb.WriteString("|")
		for i, col := range columns {
			val := fmt.Sprintf("%v", row[col])
			if len(val) > widths[i] {
				val = val[:widths[i]-3] + "..."
			}
			sb.WriteString(" ")
			sb.WriteString(val)
			sb.WriteString(strings.Repeat(" ", widths[i]-len(val)+1))
			sb.WriteString("|")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("+")
	for _, w := range widths {
		sb.WriteString(strings.Repeat("-", w+2))
		sb.WriteString("+")
	}
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("(%d rows)\n", len(results)))

	return sb.String()
}
