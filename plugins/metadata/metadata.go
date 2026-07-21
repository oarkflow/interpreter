// Package metadata exposes CSV/JSON column type inference as SPL builtins -
// useful for profiling an unfamiliar CSV/JSON data source (e.g. "is this
// column really numeric, or does it have stray text rows?") before writing
// a schema/import pipeline by hand.
//
// The inference logic below is a direct port of the pure functions in
// github.com/oarkflow/metadata's field.go (InferCSVColumnType,
// InferCSVFileTypes, InferJSONFieldType, InferJSONFileType and their
// helpers), reimplemented here rather than imported: that module's other
// files pull in a full SQL-datasource connector layer (mysql/postgres/
// mssql/sqlite via squealx) that both overlaps with plugins/database and,
// at the versions this repo currently pins, fails to build (a squealx
// subpackage those connector files import was removed upstream). Porting
// just the ~100 lines of inference logic avoids both problems.
package metadata

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/oarkflow/date"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
)

func init() {
	eval.RegisterPluginBuiltins(map[string]*object.Builtin{
		// infer_csv_types(csv_text) -> (result, err)
		// Reads raw CSV text (e.g. from file_text(file_load(path))) and
		// returns a HASH mapping each header column to its inferred type
		// ("int", "int64", "float64", "bool", "time.Time", "string", or
		// "empty" for an all-blank column), merging types across all data
		// rows.
		"infer_csv_types": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("infer_csv_types() takes 1 argument (csv_text), got %d", len(args))
				}
				text, errObj := asString(args[0], "csv_text")
				if errObj != nil {
					return errObj
				}
				types, err := inferCSVFileTypes(strings.NewReader(text))
				if err != nil {
					return tuple(object.NULL, &object.String{Value: err.Error()})
				}
				pairs := make(map[string]object.Object, len(types))
				for col, t := range types {
					pairs[col] = &object.String{Value: t}
				}
				return tuple(hashFromPairs(pairs), object.NULL)
			},
		},

		// infer_json_types(value) -> (result, err)
		// Infers a per-field type map from an already-decoded JSON value
		// (a HASH, or an ARRAY of HASH rows - the shape read_json/
		// json_decode already return), merging types across all rows/array
		// elements. Types: "int", "float64", "bool", "string", "time.Time",
		// "null", "[]<type>" for homogeneous arrays, "[]any" for mixed
		// arrays, "map[string]any" for nested objects.
		"infer_json_types": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("infer_json_types() takes 1 argument (value), got %d", len(args))
				}
				types, err := inferJSONFileType(toInferValue(args[0]))
				if err != nil {
					return tuple(object.NULL, &object.String{Value: err.Error()})
				}
				pairs := make(map[string]object.Object, len(types))
				for field, t := range types {
					pairs[field] = &object.String{Value: t}
				}
				return tuple(hashFromPairs(pairs), object.NULL)
			},
		},

		// infer_value_type(value) -> STRING
		// Infers the type of a single value directly, e.g.
		// infer_value_type(42) -> "int", infer_value_type("2026-01-01") ->
		// "time.Time". Same type vocabulary as infer_json_types.
		"infer_value_type": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("infer_value_type() takes 1 argument (value), got %d", len(args))
				}
				return &object.String{Value: inferJSONFieldType(toInferValue(args[0]))}
			},
		},
	})
}

// --- CSV type inference (ported from oarkflow/metadata's field.go) ---

func isTime(s string) bool {
	_, err := date.ParseFormat(s)
	return err == nil
}

var thousandsSeparatorRe = regexp.MustCompile(`^\d{1,3}(,\d{3})*(\.\d+)?$`)

func cleanField(s string) string {
	s = strings.TrimSpace(s)
	if thousandsSeparatorRe.MatchString(s) {
		return strings.ReplaceAll(s, ",", "")
	}
	return s
}

func inferTypeFromString(s string) string {
	s = cleanField(s)
	if s == "" {
		return "empty"
	}
	if _, err := strconv.Atoi(s); err == nil {
		return "int"
	}
	if _, err := strconv.ParseInt(s, 10, 64); err == nil {
		return "int64"
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return "float64"
	}
	lower := strings.ToLower(s)
	if lower == "true" || lower == "false" {
		return "bool"
	}
	if isTime(s) {
		return "time.Time"
	}
	return "string"
}

func mergeTypes(t1, t2 string) string {
	if t1 == t2 {
		return t1
	}
	if t1 == "empty" {
		return t2
	}
	if t2 == "empty" {
		return t1
	}
	if (t1 == "int" && t2 == "int64") || (t1 == "int64" && t2 == "int") {
		return "int64"
	}
	if (t1 == "int" || t1 == "int64") && t2 == "float64" ||
		(t2 == "int" || t2 == "int64") && t1 == "float64" {
		return "float64"
	}
	return "string"
}

func inferCSVColumnType(values []string) string {
	inferredType := "empty"
	timeCount := 0
	nonEmptyCount := 0
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		nonEmptyCount++
		if isTime(v) {
			timeCount++
		}
		typ := inferTypeFromString(v)
		if inferredType == "empty" {
			inferredType = typ
		} else {
			inferredType = mergeTypes(inferredType, typ)
		}
	}
	if nonEmptyCount > 0 && timeCount == nonEmptyCount {
		return "time.Time"
	}
	if inferredType == "empty" {
		return "string"
	}
	return inferredType
}

func fixRecord(record []string, expected int, affectedIndex int) ([]string, error) {
	if len(record) < expected {
		for len(record) < expected {
			record = append(record, "")
		}
		return record, nil
	}
	if len(record) > expected {
		extraTokens := len(record) - expected
		joinStart := affectedIndex
		joinEnd := affectedIndex + extraTokens + 1
		if joinEnd > len(record) {
			return nil, fmt.Errorf("cannot merge fields properly in record: %v", record)
		}
		joinedField := strings.Join(record[joinStart:joinEnd], "")
		fixed := append([]string(nil), record[:affectedIndex]...)
		fixed = append(fixed, joinedField)
		fixed = append(fixed, record[joinEnd:]...)
		if len(fixed) != expected {
			return nil, fmt.Errorf("after merging, expected %d fields but got %d: %v", expected, len(fixed), fixed)
		}
		return fixed, nil
	}
	return record, nil
}

func inferCSVFileTypes(reader io.Reader) (map[string]string, error) {
	r := csv.NewReader(reader)
	r.FieldsPerRecord = -1
	headers, err := r.Read()
	if err != nil {
		return nil, err
	}
	expectedFields := len(headers)
	columns := make(map[int][]string, len(headers))
	for i := range headers {
		columns[i] = []string{}
	}
	lineNum := 2
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error reading CSV on line %d: %v", lineNum, err)
		}
		for i := range record {
			record[i] = cleanField(record[i])
		}
		if len(record) != expectedFields {
			fixed, err := fixRecord(record, expectedFields, 2)
			if err != nil {
				return nil, fmt.Errorf("could not fix record on line %d: %v", lineNum, err)
			}
			record = fixed
		}
		if len(record) != expectedFields {
			return nil, fmt.Errorf("record on line %d: expected %d fields but got %d", lineNum, expectedFields, len(record))
		}
		for i, v := range record {
			columns[i] = append(columns[i], v)
		}
		lineNum++
	}
	result := make(map[string]string, len(headers))
	for i, header := range headers {
		result[header] = inferCSVColumnType(columns[i])
	}
	return result, nil
}

// --- JSON type inference (ported from oarkflow/metadata's field.go) ---

func inferJSONFieldType(value interface{}) string {
	switch v := value.(type) {
	case json.Number:
		if _, err := v.Int64(); err == nil {
			if strings.Contains(v.String(), ".") {
				return "float64"
			}
			return "int"
		}
		if f, err := v.Float64(); err == nil {
			if f == float64(int64(f)) {
				return "int"
			}
			return "float64"
		}
	case bool:
		return "bool"
	case string:
		if isTime(v) {
			return "time.Time"
		}
		return "string"
	case nil:
		return "null"
	case []interface{}:
		if len(v) == 0 {
			return "[]any"
		}
		var unifiedType string
		homogeneous := true
		for i, elem := range v {
			t := inferJSONFieldType(elem)
			if i == 0 {
				unifiedType = t
			} else {
				unifiedType = mergeJSONTypes(unifiedType, t)
				if unifiedType == "any" {
					homogeneous = false
				}
			}
		}
		if homogeneous {
			return "[]" + unifiedType
		}
		return "[]any"
	case map[string]interface{}:
		return "map[string]any"
	}
	return "unknown"
}

func mergeJSONTypes(t1, t2 string) string {
	if t1 == t2 {
		return t1
	}
	if t1 == "null" {
		return t2
	}
	if t2 == "null" {
		return t1
	}
	if strings.HasPrefix(t1, "[]") && strings.HasPrefix(t2, "[]") {
		return "[]" + mergeJSONTypes(t1[2:], t2[2:])
	}
	if (t1 == "int" && t2 == "float64") || (t1 == "float64" && t2 == "int") {
		return "float64"
	}
	return "any"
}

func inferJSONFileType(data interface{}) (map[string]string, error) {
	result := make(map[string]string)
	switch v := data.(type) {
	case map[string]interface{}:
		for key, val := range v {
			result[key] = inferJSONFieldType(val)
		}
		return result, nil
	case []interface{}:
		for _, item := range v {
			record, ok := item.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("array contains non-object items")
			}
			for key, val := range record {
				t := inferJSONFieldType(val)
				if existing, exists := result[key]; exists {
					result[key] = mergeJSONTypes(existing, t)
				} else {
					result[key] = t
				}
			}
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported top-level value for infer_json_types: expected HASH or ARRAY of HASH, got %T", data)
	}
}

// toInferValue converts an SPL object into the plain Go value shape
// inferJSONFieldType/inferJSONFileType expect. Numbers must become
// json.Number (the switch only special-cases json.Number, not native
// int/float64) to be recognized as numeric rather than falling through to
// "unknown".
func toInferValue(obj object.Object) interface{} {
	switch v := obj.(type) {
	case *object.Integer:
		return json.Number(strconv.FormatInt(v.Value, 10))
	case *object.Float:
		return json.Number(strconv.FormatFloat(v.Value, 'f', -1, 64))
	case *object.String:
		return v.Value
	case *object.Secret:
		return v.Value
	case *object.Boolean:
		return v.Value
	case *object.Null:
		return nil
	case nil:
		return nil
	case *object.Array:
		elements := make([]interface{}, len(v.Elements))
		for i, el := range v.Elements {
			elements[i] = toInferValue(el)
		}
		return elements
	case *object.Hash:
		m := make(map[string]interface{}, len(v.Pairs))
		for _, pair := range v.Pairs {
			key := pair.Key.Inspect()
			if s, ok := pair.Key.(*object.String); ok {
				key = s.Value
			}
			m[key] = toInferValue(pair.Value)
		}
		return m
	default:
		return obj.Inspect()
	}
}

func hashFromPairs(pairs map[string]object.Object) *object.Hash {
	out := make(map[object.HashKey]object.HashPair, len(pairs))
	for k, v := range pairs {
		key := &object.String{Value: k}
		out[key.HashKey()] = object.HashPair{Key: key, Value: v}
	}
	return &object.Hash{Pairs: out}
}

func asString(arg object.Object, name string) (string, object.Object) {
	if s, ok := arg.(*object.Secret); ok {
		return s.Value, nil
	}
	if arg == nil {
		return "", object.NewError("argument `%s` must be STRING, got <nil>", name)
	}
	if arg.Type() != object.STRING_OBJ {
		return "", object.NewError("argument `%s` must be STRING, got %s", name, arg.Type())
	}
	return arg.(*object.String).Value, nil
}

func tuple(values ...object.Object) *object.Array {
	return &object.Array{Elements: values}
}
