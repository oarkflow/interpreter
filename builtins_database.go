package interpreter

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

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

func init() {
	registerBuiltins(map[string]*Builtin{
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

func dbTupleResult(result Object, errMsg string) Object {
	if errMsg == "" {
		return &Array{Elements: []Object{result, NULL}}
	}
	return &Array{Elements: []Object{NULL, &String{Value: errMsg}}}
}

func dbTupleBool(ok bool, errMsg string) Object {
	if ok {
		return &Array{Elements: []Object{TRUE, NULL}}
	}
	return &Array{Elements: []Object{FALSE, &String{Value: errMsg}}}
}

func dbContext(env *Environment) context.Context {
	return runtimeContext(env)
}

func dbTarget(obj Object) (dbQueryable, Object) {
	switch v := obj.(type) {
	case *DB:
		return v, nil
	case *DBTx:
		return v, nil
	default:
		return nil, newError("first argument must be a database connection or transaction")
	}
}

func dbTxTarget(obj Object) (*DBTx, Object) {
	tx, ok := obj.(*DBTx)
	if !ok {
		return nil, newError("argument must be a database transaction (use db_begin)")
	}
	return tx, nil
}

func objectToDBNamedArgs(obj Object) (map[string]any, Object) {
	if obj == nil || obj == NULL {
		return nil, nil
	}
	hash, ok := obj.(*Hash)
	if !ok {
		return nil, newError("params must be ARRAY or HASH, got %s", obj.Type())
	}
	out := make(map[string]any, len(hash.Pairs))
	for _, pair := range hash.Pairs {
		key, ok := pair.Key.(*String)
		if !ok {
			return nil, newError("named query params require STRING keys")
		}
		out[key.Value] = objectToNative(pair.Value)
	}
	return out, nil
}

func objectToDBPositionalArgs(obj Object) ([]any, Object) {
	if obj == nil || obj == NULL {
		return nil, nil
	}
	arr, ok := obj.(*Array)
	if !ok {
		return nil, newError("params must be ARRAY or HASH, got %s", obj.Type())
	}
	out := make([]any, len(arr.Elements))
	for i, el := range arr.Elements {
		out[i] = objectToNative(el)
	}
	return out, nil
}

func parseDBParams(obj Object) ([]any, map[string]any, Object) {
	if obj == nil || obj == NULL {
		return nil, nil, nil
	}
	switch obj.Type() {
	case ARRAY_OBJ:
		positional, errObj := objectToDBPositionalArgs(obj)
		return positional, nil, errObj
	case HASH_OBJ:
		named, errObj := objectToDBNamedArgs(obj)
		return nil, named, errObj
	default:
		return nil, nil, newError("params must be ARRAY or HASH, got %s", obj.Type())
	}
}

func queryRows(ctx context.Context, target dbQueryable, query string, params Object) (*squealx.Rows, Object) {
	positional, named, errObj := parseDBParams(params)
	if errObj != nil {
		return nil, errObj
	}
	if named != nil {
		if !squealx.IsNamedQuery(query) {
			return nil, newError("HASH params require named placeholders like :name")
		}
		rows, err := target.NamedQueryContext(ctx, query, named)
		if err != nil {
			return nil, newError("query error: %s", err)
		}
		return rows, nil
	}
	if squealx.IsNamedQuery(query) && len(positional) > 0 {
		return nil, newError("named placeholders require HASH params")
	}
	rows, err := target.QueryxContext(ctx, query, positional...)
	if err != nil {
		return nil, newError("query error: %s", err)
	}
	return rows, nil
}

func execStatement(ctx context.Context, target dbQueryable, query string, params Object) (sql.Result, Object) {
	positional, named, errObj := parseDBParams(params)
	if errObj != nil {
		return nil, errObj
	}
	if named != nil {
		if !squealx.IsNamedQuery(query) {
			return nil, newError("HASH params require named placeholders like :name")
		}
		result, err := target.NamedExecContext(ctx, query, named)
		if err != nil {
			return nil, newError("exec error: %s", err)
		}
		return result, nil
	}
	if squealx.IsNamedQuery(query) && len(positional) > 0 {
		return nil, newError("named placeholders require HASH params")
	}
	result, err := target.ExecContext(ctx, query, positional...)
	if err != nil {
		return nil, newError("exec error: %s", err)
	}
	return result, nil
}

func queryResultsToObject(rows *squealx.Rows, format string) (Object, Object) {
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, newError("failed to get columns: %s", err)
	}

	var results []map[string]any
	for rows.Next() {
		row := make(map[string]any)
		if err := rows.MapScan(row); err != nil {
			return nil, newError("failed to scan row: %s", err)
		}
		results = append(results, row)
	}

	if err := rows.Err(); err != nil {
		return nil, newError("error iterating rows: %s", err)
	}

	if len(results) == 0 {
		return &Array{Elements: []Object{}}, nil
	}

	if format == "array" {
		elements := make([]Object, len(results))
		for i, row := range results {
			pairs := make(map[HashKey]HashPair)
			for col, val := range row {
				key := &String{Value: col}
				pairs[key.HashKey()] = HashPair{Key: key, Value: toObject(val)}
			}
			elements[i] = &Hash{Pairs: pairs}
		}
		return &Array{Elements: elements}, nil
	}

	return &String{Value: formatTable(columns, results)}, nil
}

func builtinDBConnect(env *Environment, args ...Object) Object {
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
	if err := checkDBAllowed(driver, connStr); err != nil {
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
	if err := db.PingContext(dbContext(env)); err != nil {
		return dbTupleResult(nil, fmt.Sprintf("failed to ping database: %v", err))
	}
	return dbTupleResult(&DB{DB: db}, "")
}

func builtinDBQuery(env *Environment, args ...Object) Object {
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

	params := Object(NULL)
	format := "table"
	if len(args) >= 3 {
		if args[2].Type() == STRING_OBJ && len(args) == 3 {
			format = strings.ToLower(args[2].(*String).Value)
		} else {
			params = args[2]
		}
	}
	if len(args) == 4 {
		if args[3].Type() != STRING_OBJ {
			return dbTupleResult(nil, "format must be STRING when provided")
		}
		format = strings.ToLower(args[3].(*String).Value)
	}
	if format != "table" && format != "array" {
		return dbTupleResult(nil, fmt.Sprintf("unsupported db_query format %q", format))
	}

	rows, queryErr := queryRows(dbContext(env), target, query, params)
	if queryErr != nil {
		return dbTupleResult(nil, queryErr.Inspect())
	}
	result, formatErr := queryResultsToObject(rows, format)
	if formatErr != nil {
		return dbTupleResult(nil, formatErr.Inspect())
	}
	return dbTupleResult(result, "")
}

func builtinDBExec(env *Environment, args ...Object) Object {
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
	params := Object(NULL)
	if len(args) == 3 {
		params = args[2]
	}

	result, execErr := execStatement(dbContext(env), target, query, params)
	if execErr != nil {
		return dbTupleResult(nil, execErr.Inspect())
	}
	rowsAffected, _ := result.RowsAffected()
	lastInsertID, _ := result.LastInsertId()

	pairs := make(map[HashKey]HashPair, 2)
	for key, value := range map[string]any{
		"rows_affected":  rowsAffected,
		"last_insert_id": lastInsertID,
	} {
		k := &String{Value: key}
		pairs[k.HashKey()] = HashPair{Key: k, Value: toObject(value)}
	}
	return dbTupleResult(&Hash{Pairs: pairs}, "")
}

func builtinDBBegin(env *Environment, args ...Object) Object {
	if len(args) != 1 {
		return dbTupleResult(nil, "db_begin requires 1 argument: db")
	}
	dbObj, ok := args[0].(*DB)
	if !ok {
		return dbTupleResult(nil, "argument must be a database connection (use db_connect)")
	}
	tx, err := dbObj.BeginTxx(dbContext(env), nil)
	if err != nil {
		return dbTupleResult(nil, fmt.Sprintf("failed to begin transaction: %v", err))
	}
	return dbTupleResult(&DBTx{Tx: tx}, "")
}

func builtinDBCommit(args ...Object) Object {
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

func builtinDBRollback(args ...Object) Object {
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

func builtinDBClose(args ...Object) Object {
	if len(args) != 1 {
		return dbTupleBool(false, "db_close requires 1 argument: db")
	}
	dbObj, ok := args[0].(*DB)
	if !ok {
		return dbTupleBool(false, "argument must be a database connection (use db_connect)")
	}
	if err := dbObj.SQLDB.Close(); err != nil {
		return dbTupleBool(false, fmt.Sprintf("failed to close connection: %v", err))
	}
	return dbTupleBool(true, "")
}

func builtinDBTables(env *Environment, args ...Object) Object {
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

	rows, queryErr := queryRows(dbContext(env), target, query, NULL)
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

	elements := make([]Object, len(tables))
	for i, table := range tables {
		elements[i] = &String{Value: table}
	}
	return dbTupleResult(&Array{Elements: elements}, "")
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
