package eval

import (
	"fmt"
	"os"
	"sort"
	"sync"

	"github.com/oarkflow/interpreter/pkg/object"
)

// ---------------------------------------------------------------------------
// Builtins registry
// ---------------------------------------------------------------------------

var (
	builtinsMu sync.Mutex
	// Builtins is the global map of registered builtin functions.
	Builtins = map[string]*object.Builtin{}
)

// RegisterBuiltins merges a group of builtins into the global registry.
func RegisterBuiltins(group map[string]*object.Builtin) {
	builtinsMu.Lock()
	defer builtinsMu.Unlock()
	for name, fn := range group {
		if _, exists := Builtins[name]; exists {
			fmt.Fprintf(os.Stderr, "warning: builtin %q already exists; skipping duplicate registration\n", name)
			continue
		}
		if fn != nil && fn.Fn == nil && fn.FnWithEnv != nil {
			captured := fn
			fn.Fn = func(args ...object.Object) object.Object {
				return captured.FnWithEnv(captured.Env, args...)
			}
		}
		Builtins[name] = fn
	}
}

// BuiltinHelpDescriptions provides human-readable help for builtins.
var BuiltinHelpDescriptions = map[string]string{
	"help":               "help() lists builtin names; help(\"name\") shows details for one builtin",
	"sprintf":            "sprintf(format, ...args) formats values with printf-style placeholders; supports %T for SPL type",
	"printf":             "printf(format, ...args) prints formatted text and returns it",
	"interpolate":        "interpolate(template, data[, ...positional]) replaces {key} or {index} placeholders",
	"file":               "file(path_or_url_or_data[, opts]) creates a renderable file artifact",
	"image":              "image(path_or_url_or_data[, opts]) creates a renderable image artifact",
	"render":             "render(value[, opts]) creates or updates a renderable artifact",
	"file_load":          "file_load(path_or_artifact[, opts]) loads content into FILE_VALUE",
	"file_save":          "file_save(file_value, path[, opts]) writes FILE_VALUE content to disk",
	"file_text":          "file_text(file_value) returns file content as STRING",
	"file_bytes":         "file_bytes(file_value) returns file content as base64 STRING",
	"file_name":          "file_name(file_value) returns the file name",
	"file_mime":          "file_mime(file_value) returns the MIME type",
	"file_size":          "file_size(file_value) returns the file size in bytes",
	"file_copy":          "file_copy(src, dst) copies a file path or FILE_VALUE-backed path",
	"file_move":          "file_move(src, dst) moves a file path or FILE_VALUE-backed path",
	"file_rename":        "file_rename(path, new_name) renames a file in place",
	"image_load":         "image_load(path_or_artifact[, opts]) decodes an image into IMAGE_VALUE",
	"image_resize":       "image_resize(image_value, width, height[, opts]) resizes an image",
	"image_crop":         "image_crop(image_value, x, y, width, height) crops an image",
	"image_rotate":       "image_rotate(image_value, degrees[, opts]) rotates an image",
	"image_convert":      "image_convert(image_value, format[, opts]) re-encodes an image",
	"image_save":         "image_save(image_value, path[, opts]) writes IMAGE_VALUE content to disk",
	"image_info":         "image_info(image_value) returns metadata for an image",
	"image_render":       "image_render(image_value[, opts]) creates a renderable image artifact",
	"image_resize_file":  "image_resize_file(src, dst, width, height[, opts]) resizes and saves an image",
	"image_convert_file": "image_convert_file(src, dst, format[, opts]) converts and saves an image",
	"read_json":          "read_json(path[, opts]) loads JSON from disk",
	"write_json":         "write_json(path, value[, opts]) saves JSON to disk",
	"read_csv":           "read_csv(path[, opts]) loads CSV into TABLE_VALUE",
	"write_csv":          "write_csv(path, table_or_rows[, opts]) saves CSV to disk",
	"csv_decode":         "csv_decode(text[, opts]) decodes CSV text into TABLE_VALUE",
	"csv_encode":         "csv_encode(table_or_rows[, opts]) encodes rows as CSV text",
	"table_rows":         "table_rows(table) returns TABLE_VALUE rows as ARRAY of HASH",
	"table_columns":      "table_columns(table) returns ARRAY of column names",
	"table_select":       "table_select(table, columns) keeps selected columns",
	"table_filter":       "table_filter(table, fn) filters rows using a callback",
	"table_map":          "table_map(table, fn) maps rows using a callback that returns HASH",
	"uuid":               "uuid([version]) generates UUID, default version is 7; supports 4 or 7",
	"http_request":       "http_request(method, url[, body][, headers][, timeout_ms]) performs an HTTP request",
	"http_get":           "http_get(url[, headers][, timeout_ms]) performs HTTP GET",
	"http_post":          "http_post(url, body[, headers][, timeout_ms]) performs HTTP POST",
	"webhook":            "webhook(url, payload[, headers][, timeout_ms]) sends a webhook POST",
	"db_connect":         "db_connect(driver, connection_string) opens a database connection",
	"db_query":           "db_query(db_or_tx, query[, params][, format]) runs a query; params may be ARRAY or HASH; format is table or array",
	"db_exec":            "db_exec(db_or_tx, query[, params]) executes a statement; params may be ARRAY or HASH",
	"db_begin":           "db_begin(db) starts a database transaction",
	"db_commit":          "db_commit(tx) commits a database transaction",
	"db_rollback":        "db_rollback(tx) rolls back a database transaction",
	"db_tables":          "db_tables(db_or_tx) lists database tables",
	"db_close":           "db_close(db) closes a database connection",
	"smtp_send":          "smtp_send(config) sends email via SMTP",
	"ftp_list":           "ftp_list(config, remote_dir) lists directory entries over FTP",
	"ftp_get":            "ftp_get(config, remote_path, local_path) downloads file over FTP",
	"ftp_put":            "ftp_put(config, local_path, remote_path) uploads file over FTP",
	"sftp_list":          "sftp_list(config, remote_dir) lists directory entries over SFTP",
	"sftp_get":           "sftp_get(config, remote_path, local_path) downloads file over SFTP",
	"sftp_put":           "sftp_put(config, local_path, remote_path) uploads file over SFTP",
	"assert_true":        "assert_true(condition[, message]) fails test when condition is false",
	"assert_eq":          "assert_eq(actual, expected[, message]) fails test when values differ",
	"assert_neq":         "assert_neq(actual, unexpected[, message]) fails test when values are equal",
	"assert_contains":    "assert_contains(haystack, needle[, message]) fails test when needle not found in haystack string or array",
	"assert_throws":      "assert_throws(fn[, message]) fails test when fn does not produce an error",
	"test_summary":       "test_summary() returns {total, passed, failed}",
	"run_tests":          "run_tests(path_or_paths) executes SPL test scripts and returns summary",
	"exec":               "exec(command, ...args[, timeout_ms]) runs a whitelisted OS command; disabled by SPL_DISABLE_EXEC or host protection",
	"config_load":        "config_load(path[, format]) loads JSON/YAML/.env config and wraps secret-like fields",
	"config_parse":       "config_parse(raw, format) parses JSON/YAML/.env string and wraps secret-like fields",
	"secret":             "secret(value) wraps a string as non-displayable secret",
	"secret_reveal":      "secret_reveal(secret_value) reveals a SECRET as plain STRING",
	"secret_mask":        "secret_mask(value[, visible]) returns masked display string",
	"Error":              "Error(message[, details]) returns structured error object with message, code, stack",
	"channel":            "channel([buffer_size]) creates a message channel",
	"send":               "send(channel, value) sends a value to channel",
	"recv":               "recv(channel) receives a value from channel",
	"go":                 "go(fn[, ...args]) runs function asynchronously and returns future",
	"generator":          "generator(fn) wraps function result as lazy iterable",
	"permissions":        "permissions(policy_hash) applies runtime allow/deny policy",
	"metric":             "metric(name, value[, labels]) records metric point",
	"trace":              "trace(name[, attrs]) emits trace event",
	"immutable":          "immutable(value) returns deeply frozen copy",
	"move":               "move(value) transfers ownership marker to current scope",
}

// BuiltinHelpText returns the help string for a named builtin.
func BuiltinHelpText(name string) string {
	if details, ok := BuiltinHelpDescriptions[name]; ok {
		return details
	}
	return fmt.Sprintf("%s(...) builtin function", name)
}

// BuiltinNames returns a sorted list of all registered builtin names.
func BuiltinNames() []string {
	builtinsMu.Lock()
	defer builtinsMu.Unlock()
	names := make([]string, 0, len(Builtins))
	for name := range Builtins {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// HasBuiltin reports whether a builtin with the given name is registered.
func HasBuiltin(name string) bool {
	builtinsMu.Lock()
	defer builtinsMu.Unlock()
	_, ok := Builtins[name]
	return ok
}
