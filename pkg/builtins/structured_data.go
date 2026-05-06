package builtins

import (
	"os"
	"path/filepath"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/security"
)

func init() {
	eval.RegisterBuiltins(map[string]*object.Builtin{
		"read_json": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 || len(args) > 2 {
					return object.NewError("wrong number of arguments. got=%d, want=1 or 2", len(args))
				}
				path, errObj := asString(args[0], "path")
				if errObj != nil {
					return errObj
				}
				data, _, errObj := loadTextFile(path)
				if errObj != nil {
					return errObj
				}
				out, err := ParseJSONToObject(string(data))
				if err != nil {
					return object.NewError("json decode failed: %v", err)
				}
				return out
			},
		},
		"write_json": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 2 || len(args) > 3 {
					return object.NewError("wrong number of arguments. got=%d, want=2 or 3", len(args))
				}
				path, errObj := asString(args[0], "path")
				if errObj != nil {
					return errObj
				}
				opts := map[string]object.Object(nil)
				if len(args) == 3 {
					opts, errObj = parseOptionalHash(args[2], "opts")
					if errObj != nil {
						return errObj
					}
				}
				data, errObj := jsonMarshalValue(args[1], opts)
				if errObj != nil {
					return errObj
				}
				if result := saveBytesToPath(path, data); object.IsError(result) {
					return result
				}
				return args[1]
			},
		},
		"read_csv": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 || len(args) > 2 {
					return object.NewError("wrong number of arguments. got=%d, want=1 or 2", len(args))
				}
				path, errObj := asString(args[0], "path")
				if errObj != nil {
					return errObj
				}
				opts := map[string]object.Object(nil)
				if len(args) == 2 {
					opts, errObj = parseOptionalHash(args[1], "opts")
					if errObj != nil {
						return errObj
					}
				}
				data, safePath, errObj := loadTextFile(path)
				if errObj != nil {
					return errObj
				}
				table, errObj := decodeCSVText(string(data), opts)
				if errObj != nil {
					return errObj
				}
				table.Path = safePath
				table.Name = filepath.Base(safePath)
				return table
			},
		},
		"write_csv": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 2 || len(args) > 3 {
					return object.NewError("wrong number of arguments. got=%d, want=2 or 3", len(args))
				}
				path, errObj := asString(args[0], "path")
				if errObj != nil {
					return errObj
				}
				opts := map[string]object.Object(nil)
				if len(args) == 3 {
					opts, errObj = parseOptionalHash(args[2], "opts")
					if errObj != nil {
						return errObj
					}
				}
				encoded, table, errObj := encodeTableCSV(args[1], opts)
				if errObj != nil {
					return errObj
				}
				if result := saveBytesToPath(path, []byte(encoded)); object.IsError(result) {
					return result
				}
				safePath, _ := SanitizePathLocal(path)
				table.Path = safePath
				table.Name = filepath.Base(safePath)
				return table
			},
		},
		"csv_decode": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 || len(args) > 2 {
					return object.NewError("wrong number of arguments. got=%d, want=1 or 2", len(args))
				}
				text, errObj := asString(args[0], "text")
				if errObj != nil {
					return errObj
				}
				opts := map[string]object.Object(nil)
				if len(args) == 2 {
					opts, errObj = parseOptionalHash(args[1], "opts")
					if errObj != nil {
						return errObj
					}
				}
				table, errObj := decodeCSVText(text, opts)
				if errObj != nil {
					return errObj
				}
				return table
			},
		},
		"csv_encode": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 || len(args) > 2 {
					return object.NewError("wrong number of arguments. got=%d, want=1 or 2", len(args))
				}
				opts := map[string]object.Object(nil)
				if len(args) == 2 {
					var errObj object.Object
					opts, errObj = parseOptionalHash(args[1], "opts")
					if errObj != nil {
						return errObj
					}
				}
				encoded, _, errObj := encodeTableCSV(args[0], opts)
				if errObj != nil {
					return errObj
				}
				return &object.String{Value: encoded}
			},
		},
		"table_rows": {
			Fn: func(args ...object.Object) object.Object {
				table, errObj := expectTableArg(args, "table_rows")
				if errObj != nil {
					return errObj
				}
				rows := make([]object.Object, 0, len(table.Rows))
				for _, row := range table.Rows {
					rows = append(rows, rowToHash(cloneRow(row)))
				}
				return &object.Array{Elements: rows}
			},
		},
		"table_columns": {
			Fn: func(args ...object.Object) object.Object {
				table, errObj := expectTableArg(args, "table_columns")
				if errObj != nil {
					return errObj
				}
				cols := make([]object.Object, 0, len(table.Columns))
				for _, col := range table.Columns {
					cols = append(cols, &object.String{Value: col})
				}
				return &object.Array{Elements: cols}
			},
		},
		"table_select": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return object.NewError("wrong number of arguments. got=%d, want=2", len(args))
				}
				table, ok := args[0].(*object.TableValue)
				if !ok {
					return object.NewError("argument `table` must be TABLE_VALUE, got %s", args[0].Type())
				}
				columns := []string{}
				if arr, ok := args[1].(*object.Array); ok {
					for _, el := range arr.Elements {
						col, ok := el.(*object.String)
						if !ok {
							return object.NewError("columns must be STRING values")
						}
						columns = append(columns, col.Value)
					}
				} else {
					return object.NewError("argument `columns` must be ARRAY, got %s", args[1].Type())
				}
				selected := cloneTableValue(table)
				selected.Columns = append([]string(nil), columns...)
				selected.Rows = make([]map[string]object.Object, 0, len(table.Rows))
				for _, row := range table.Rows {
					next := make(map[string]object.Object, len(columns))
					for _, col := range columns {
						if val, ok := row[col]; ok {
							next[col] = val
						} else {
							next[col] = object.NULL
						}
					}
					selected.Rows = append(selected.Rows, next)
				}
				return selected
			},
		},
		"table_filter": {
			FnWithEnv: func(env *object.Environment, args ...object.Object) object.Object {
				if len(args) != 2 {
					return object.NewError("wrong number of arguments. got=%d, want=2", len(args))
				}
				table, ok := args[0].(*object.TableValue)
				if !ok {
					return object.NewError("argument `table` must be TABLE_VALUE, got %s", args[0].Type())
				}
				if object.ApplyFunctionFn == nil {
					return object.NewError("callback execution unavailable")
				}
				filtered := cloneTableValue(table)
				filtered.Rows = filtered.Rows[:0]
				for _, row := range table.Rows {
					res := object.ApplyFunctionFn(args[1], []object.Object{rowToHash(cloneRow(row))}, env)
					if object.IsError(res) {
						return res
					}
					if object.IsTruthy(res) {
						filtered.Rows = append(filtered.Rows, cloneRow(row))
					}
				}
				return filtered
			},
		},
		"table_map": {
			FnWithEnv: func(env *object.Environment, args ...object.Object) object.Object {
				if len(args) != 2 {
					return object.NewError("wrong number of arguments. got=%d, want=2", len(args))
				}
				table, ok := args[0].(*object.TableValue)
				if !ok {
					return object.NewError("argument `table` must be TABLE_VALUE, got %s", args[0].Type())
				}
				if object.ApplyFunctionFn == nil {
					return object.NewError("callback execution unavailable")
				}
				mapped := cloneTableValue(table)
				mapped.Rows = mapped.Rows[:0]
				seen := make(map[string]struct{})
				columns := []string{}
				for _, row := range table.Rows {
					res := object.ApplyFunctionFn(args[1], []object.Object{rowToHash(cloneRow(row))}, env)
					if object.IsError(res) {
						return res
					}
					hash, ok := res.(*object.Hash)
					if !ok {
						return object.NewError("table_map callback must return HASH, got %s", res.Type())
					}
					next := hashToRow(hash)
					mapped.Rows = append(mapped.Rows, next)
					for key := range next {
						if _, exists := seen[key]; exists {
							continue
						}
						seen[key] = struct{}{}
						columns = append(columns, key)
					}
				}
				if len(columns) == 0 {
					columns = append([]string(nil), table.Columns...)
				}
				mapped.Columns = columns
				return mapped
			},
		},
	})
}

func expectTableArg(args []object.Object, name string) (*object.TableValue, object.Object) {
	if len(args) != 1 {
		return nil, object.NewError("%s expects 1 argument, got %d", name, len(args))
	}
	table, ok := args[0].(*object.TableValue)
	if !ok {
		return nil, object.NewError("argument `table` must be TABLE_VALUE, got %s", args[0].Type())
	}
	return table, nil
}

func ensureWritablePath(path string) (string, object.Object) {
	safePath, err := SanitizePathLocal(path)
	if err != nil {
		return "", object.NewError("%s", err)
	}
	if err := security.CheckFileWriteAllowed(safePath); err != nil {
		return "", object.NewError("%s", err)
	}
	if err := os.MkdirAll(filepath.Dir(safePath), 0o755); err != nil {
		return "", object.NewError("%s", err)
	}
	return safePath, nil
}
