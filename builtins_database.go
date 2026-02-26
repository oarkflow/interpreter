package interpreter

import (
	"fmt"
	"strings"

	"github.com/oarkflow/squealx"
	"github.com/oarkflow/squealx/drivers/mysql"
	"github.com/oarkflow/squealx/drivers/postgres"
	"github.com/oarkflow/squealx/drivers/sqlite"
)

// dbConnect establishes a database connection
// Supported drivers: postgres, mysql, sqlite
func init() {
	registerBuiltins(map[string]*Builtin{
		"db_connect": {
			Fn: func(args ...Object) Object {
				// Returns [DB, Error]
				retErr := func(msg string) Object {
					return &Array{Elements: []Object{&Null{}, &String{Value: msg}}}
				}
				retOk := func(db *DB) Object {
					return &Array{Elements: []Object{db, &Null{}}}
				}

				if len(args) != 2 {
					return retErr("db_connect requires 2 arguments: driver and connection_string")
				}

				driver, errObj := asString(args[0], "driver")
				if errObj != nil {
					return retErr(errObj.Inspect())
				}

				connStr, errObj := asString(args[1], "connection_string")
				if errObj != nil {
					return retErr(errObj.Inspect())
				}

				var db *squealx.DB
				var err error

				switch strings.ToLower(driver) {
				case "postgres", "postgresql":
					db, err = postgres.Open(connStr, "postgres")
				case "mysql":
					db, err = mysql.Open(connStr, "mysql")
				case "sqlite", "sqlite3":
					db, err = sqlite.Open(connStr, "sqlite")
				default:
					return retErr(fmt.Sprintf("unsupported driver: %s. Supported: postgres, mysql, sqlite", driver))
				}

				if err != nil {
					return retErr(fmt.Sprintf("failed to open connection: %v", err))
				}

				if err := db.Ping(); err != nil {
					return retErr(fmt.Sprintf("failed to ping database: %v", err))
				}

				return retOk(&DB{DB: db})
			},
		},
		"db_query": {
			Fn: func(args ...Object) Object {
				// Returns [Result, Error]
				// Result is a formatted table string or an array of rows
				retErr := func(msg string) Object {
					return &Array{Elements: []Object{&Null{}, &String{Value: msg}}}
				}
				retOk := func(result interface{}) Object {
					return &Array{Elements: []Object{toObject(result), &Null{}}}
				}

				if len(args) < 2 || len(args) > 3 {
					return retErr("db_query requires 2-3 arguments: db, query [, options]")
				}

				dbObj, ok := args[0].(*DB)
				if !ok {
					return retErr("first argument must be a database connection (use db_connect)")
				}

				query, errObj := asString(args[1], "query")
				if errObj != nil {
					return retErr(errObj.Inspect())
				}

				// Check for options: format (table, array)
				format := "table"
				if len(args) == 3 {
					if args[2].Type() == STRING_OBJ {
						format = strings.ToLower(args[2].(*String).Value)
					}
				}

				rows, err := dbObj.Queryx(query)
				if err != nil {
					return retErr(fmt.Sprintf("query error: %v", err))
				}
				defer rows.Close()

				// Get column names
				columns, err := rows.Columns()
				if err != nil {
					return retErr(fmt.Sprintf("failed to get columns: %v", err))
				}

				// Fetch all rows
				var results []map[string]interface{}
				for rows.Next() {
					row := make(map[string]interface{})
					if err := rows.MapScan(row); err != nil {
						return retErr(fmt.Sprintf("failed to scan row: %v", err))
					}
					results = append(results, row)
				}

				if err := rows.Err(); err != nil {
					return retErr(fmt.Sprintf("error iterating rows: %v", err))
				}

				if len(results) == 0 {
					// No rows, return empty array
					return retOk(&Array{Elements: []Object{}})
				}

				if format == "array" {
					// Return as array of hashes
					elements := make([]Object, len(results))
					for i, row := range results {
						pairs := make(map[HashKey]HashPair)
						for col, val := range row {
							key := &String{Value: col}
							pairs[key.HashKey()] = HashPair{Key: key, Value: toObject(val)}
						}
						elements[i] = &Hash{Pairs: pairs}
					}
					return retOk(&Array{Elements: elements})
				}

				// Default: return as formatted table
				table := formatTable(columns, results)
				return retOk(&String{Value: table})
			},
		},
		"db_exec": {
			Fn: func(args ...Object) Object {
				// Returns [Result, Error]
				// Result contains affected rows and last insert id
				retErr := func(msg string) Object {
					return &Array{Elements: []Object{&Null{}, &String{Value: msg}}}
				}
				retOk := func(result map[string]interface{}) Object {
					pairs := make(map[HashKey]HashPair)
					for k, v := range result {
						key := &String{Value: k}
						pairs[key.HashKey()] = HashPair{Key: key, Value: toObject(v)}
					}
					return &Array{Elements: []Object{&Hash{Pairs: pairs}, &Null{}}}
				}

				if len(args) != 2 {
					return retErr("db_exec requires 2 arguments: db, query")
				}

				dbObj, ok := args[0].(*DB)
				if !ok {
					return retErr("first argument must be a database connection (use db_connect)")
				}

				query, errObj := asString(args[1], "query")
				if errObj != nil {
					return retErr(errObj.Inspect())
				}

				result, err := dbObj.Exec(query)
				if err != nil {
					return retErr(fmt.Sprintf("exec error: %v", err))
				}

				rowsAffected, _ := result.RowsAffected()
				lastInsertId, _ := result.LastInsertId()

				return retOk(map[string]interface{}{
					"rows_affected":  rowsAffected,
					"last_insert_id": lastInsertId,
				})
			},
		},
		"db_close": {
			Fn: func(args ...Object) Object {
				// Returns [Bool, Error]
				retErr := func(msg string) Object {
					return &Array{Elements: []Object{FALSE, &String{Value: msg}}}
				}
				retOk := func() Object {
					return &Array{Elements: []Object{TRUE, &Null{}}}
				}

				if len(args) != 1 {
					return retErr("db_close requires 1 argument: db")
				}

				dbObj, ok := args[0].(*DB)
				if !ok {
					return retErr("argument must be a database connection (use db_connect)")
				}

				// Use the embedded SQLDB to close
				if err := dbObj.SQLDB.Close(); err != nil {
					return retErr(fmt.Sprintf("failed to close connection: %v", err))
				}

				return retOk()
			},
		},
		"db_tables": {
			Fn: func(args ...Object) Object {
				// Returns [Array of table names, Error]
				retErr := func(msg string) Object {
					return &Array{Elements: []Object{&Null{}, &String{Value: msg}}}
				}
				retOk := func(tables []string) Object {
					elements := make([]Object, len(tables))
					for i, t := range tables {
						elements[i] = &String{Value: t}
					}
					return &Array{Elements: []Object{&Array{Elements: elements}, &Null{}}}
				}

				if len(args) != 1 {
					return retErr("db_tables requires 1 argument: db")
				}

				dbObj, ok := args[0].(*DB)
				if !ok {
					return retErr("argument must be a database connection (use db_connect)")
				}

				driverName := dbObj.DriverName()

				var rows *squealx.Rows
				var err error

				// Try different queries based on driver
				switch driverName {
				case "postgres", "pgx":
					rows, err = dbObj.Queryx("SELECT table_name FROM information_schema.tables WHERE table_schema = 'public'")
				case "mysql":
					rows, err = dbObj.Queryx("SHOW TABLES")
				case "sqlite", "sqlite3":
					rows, err = dbObj.Queryx("SELECT name FROM sqlite_master WHERE type='table'")
				default:
					// Try SQLite as fallback
					rows, err = dbObj.Queryx("SELECT name FROM sqlite_master WHERE type='table'")
				}

				if err != nil {
					return retErr(fmt.Sprintf("failed to get tables: %v", err))
				}
				defer rows.Close()

				var tables []string
				for rows.Next() {
					var tableName string
					if err := rows.Scan(&tableName); err != nil {
						return retErr(fmt.Sprintf("failed to scan table name: %v", err))
					}
					tables = append(tables, tableName)
				}

				if err := rows.Err(); err != nil {
					return retErr(fmt.Sprintf("error iterating tables: %v", err))
				}

				return retOk(tables)
			},
		},
	})
}

// formatTable formats query results as a table string
func formatTable(columns []string, results []map[string]interface{}) string {
	var sb strings.Builder

	// Calculate column widths
	widths := make([]int, len(columns))
	for i, col := range columns {
		widths[i] = len(col)
	}

	// Calculate max width for each column
	for _, row := range results {
		for i, col := range columns {
			val := fmt.Sprintf("%v", row[col])
			if len(val) > widths[i] {
				widths[i] = len(val)
			}
		}
	}

	// Cap max width at 50 characters
	for i := range widths {
		if widths[i] > 50 {
			widths[i] = 50
		}
	}

	// Print header
	sb.WriteString("+")
	for _, w := range widths {
		sb.WriteString(strings.Repeat("-", w+2))
		sb.WriteString("+")
	}
	sb.WriteString("\n")

	// Print column names
	sb.WriteString("|")
	for i, col := range columns {
		sb.WriteString(" ")
		sb.WriteString(col)
		sb.WriteString(strings.Repeat(" ", widths[i]-len(col)+1))
		sb.WriteString("|")
	}
	sb.WriteString("\n")

	// Print separator
	sb.WriteString("+")
	for _, w := range widths {
		sb.WriteString(strings.Repeat("=", w+2))
		sb.WriteString("+")
	}
	sb.WriteString("\n")

	// Print rows
	for _, row := range results {
		sb.WriteString("|")
		for i, col := range columns {
			val := fmt.Sprintf("%v", row[col])
			// Truncate if too long
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

	// Print footer
	sb.WriteString("+")
	for _, w := range widths {
		sb.WriteString(strings.Repeat("-", w+2))
		sb.WriteString("+")
	}
	sb.WriteString("\n")

	// Add row count
	sb.WriteString(fmt.Sprintf("(%d rows)\n", len(results)))

	return sb.String()
}
