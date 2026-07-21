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
	// Builtins is the global map of registered builtin functions. These are
	// resolvable as bare identifiers by any script, with no import required.
	Builtins = map[string]*object.Builtin{}

	pluginBuiltinsMu sync.Mutex
	// PluginBuiltins holds functions registered by optional plugin packages
	// (pdf, rules, database, ...). Unlike Builtins, these are never resolved
	// as bare global identifiers by evalIdentifier — a script must import
	// the owning std module first (e.g. `import "rules" as rules;`) and
	// call them through that namespace (`rules.service()`).
	PluginBuiltins = map[string]*object.Builtin{}
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

// RegisterPluginBuiltins merges a group of builtins into the plugin
// registry. Plugin builtins are only reachable through an explicit import
// of their owning std module (see presets_plugins.go), never as bare
// global identifiers — call this instead of RegisterBuiltins from a
// plugins/* package's init().
func RegisterPluginBuiltins(group map[string]*object.Builtin) {
	pluginBuiltinsMu.Lock()
	defer pluginBuiltinsMu.Unlock()
	for name, fn := range group {
		if _, exists := PluginBuiltins[name]; exists {
			fmt.Fprintf(os.Stderr, "warning: plugin builtin %q already exists; skipping duplicate registration\n", name)
			continue
		}
		if fn != nil && fn.Fn == nil && fn.FnWithEnv != nil {
			captured := fn
			fn.Fn = func(args ...object.Object) object.Object {
				return captured.FnWithEnv(captured.Env, args...)
			}
		}
		PluginBuiltins[name] = fn
	}
}

// BuiltinHelpDescriptions provides human-readable help for builtins.
var BuiltinHelpDescriptions = map[string]string{
	"help":                "help() lists builtin names; help(\"name\") shows details for one builtin",
	"sprintf":             "sprintf(format, ...args) formats values with printf-style placeholders; supports %T for SPL type",
	"printf":              "printf(format, ...args) prints formatted text and returns it",
	"interpolate":         "interpolate(template, data[, ...positional]) replaces {key} or {index} placeholders",
	"file":                "file(path_or_url_or_data[, opts]) creates a renderable file artifact",
	"image":               "image(path_or_url_or_data[, opts]) creates a renderable image artifact",
	"render":              "render(value[, opts]) creates or updates a renderable artifact",
	"file_load":           "file_load(path_or_artifact[, opts]) loads content into FILE_VALUE",
	"file_save":           "file_save(file_value, path[, opts]) writes FILE_VALUE content to disk",
	"file_text":           "file_text(file_value) returns file content as STRING",
	"file_bytes":          "file_bytes(file_value) returns file content as base64 STRING",
	"file_name":           "file_name(file_value) returns the file name",
	"file_mime":           "file_mime(file_value) returns the MIME type",
	"file_size":           "file_size(file_value) returns the file size in bytes",
	"file_copy":           "file_copy(src, dst) copies a file path or FILE_VALUE-backed path",
	"file_move":           "file_move(src, dst) moves a file path or FILE_VALUE-backed path",
	"file_rename":         "file_rename(path, new_name) renames a file in place",
	"image_load":          "image_load(path_or_artifact[, opts]) decodes an image into IMAGE_VALUE",
	"image_resize":        "image_resize(image_value, width, height[, opts]) resizes an image",
	"image_crop":          "image_crop(image_value, x, y, width, height) crops an image",
	"image_rotate":        "image_rotate(image_value, degrees[, opts]) rotates an image",
	"image_convert":       "image_convert(image_value, format[, opts]) re-encodes an image",
	"image_save":          "image_save(image_value, path[, opts]) writes IMAGE_VALUE content to disk",
	"image_info":          "image_info(image_value) returns metadata for an image",
	"image_render":        "image_render(image_value[, opts]) creates a renderable image artifact",
	"image_resize_file":   "image_resize_file(src, dst, width, height[, opts]) resizes and saves an image",
	"image_convert_file":  "image_convert_file(src, dst, format[, opts]) converts and saves an image",
	"read_json":           "read_json(path[, opts]) loads JSON from disk",
	"write_json":          "write_json(path, value[, opts]) saves JSON to disk",
	"read_csv":            "read_csv(path[, opts]) loads CSV into TABLE_VALUE",
	"write_csv":           "write_csv(path, table_or_rows[, opts]) saves CSV to disk",
	"csv_decode":          "csv_decode(text[, opts]) decodes CSV text into TABLE_VALUE",
	"csv_encode":          "csv_encode(table_or_rows[, opts]) encodes rows as CSV text",
	"table_rows":          "table_rows(table) returns TABLE_VALUE rows as ARRAY of HASH",
	"table_columns":       "table_columns(table) returns ARRAY of column names",
	"table_select":        "table_select(table, columns) keeps selected columns",
	"table_filter":        "table_filter(table, fn) filters rows using a callback",
	"table_map":           "table_map(table, fn) maps rows using a callback that returns HASH",
	"uuid":                "uuid([version]) generates UUID, default version is 7; supports 4 or 7",
	"http_request":        "http_request(method, url[, body][, headers][, timeout_ms]) performs an HTTP request",
	"http_get":            "http_get(url[, headers][, timeout_ms]) performs HTTP GET",
	"http_post":           "http_post(url, body[, headers][, timeout_ms]) performs HTTP POST",
	"webhook":             "webhook(url, payload[, headers][, timeout_ms]) sends a webhook POST",
	"db_connect":          "db_connect(driver, connection_string) opens a database connection",
	"db_query":            "db_query(db_or_tx, query[, params][, format][, timeout_ms]) runs a query; params may be ARRAY or HASH; format is table or array; optional trailing timeout_ms bounds this call",
	"db_exec":             "db_exec(db_or_tx, query[, params][, timeout_ms]) executes a statement; params may be ARRAY or HASH; optional trailing timeout_ms bounds this call",
	"db_begin":            "db_begin(db) starts a database transaction",
	"db_commit":           "db_commit(tx) commits a database transaction",
	"db_rollback":         "db_rollback(tx) rolls back a database transaction",
	"db_tables":           "db_tables(db_or_tx) lists database tables",
	"db_close":            "db_close(db) closes a database connection",
	"bcrypt_hash":         "bcrypt_hash(password[, cost]) hashes a password with bcrypt (requires cryptoextra plugin)",
	"bcrypt_verify":       "bcrypt_verify(password, hash) verifies a password against a bcrypt hash (requires cryptoextra plugin)",
	"jwt_encode":          "jwt_encode(claims, secret[, opts]) signs a HASH of claims into a JWT string; opts supports alg (HS256/HS384/HS512) and expires_in seconds (requires cryptoextra plugin)",
	"jwt_decode":          "jwt_decode(token, secret[, opts]) verifies and decodes a JWT string into a HASH of claims; raises on invalid signature/alg/expiry (requires cryptoextra plugin)",
	"yaml_encode":         "yaml_encode(value[, opts]) encodes a value as a YAML string; opts supports indent (requires config/yaml plugin)",
	"yaml_decode":         "yaml_decode(yamlString) decodes a YAML string into the matching value (requires config/yaml plugin)",
	"smtp_send":           "smtp_send(config) sends email via SMTP",
	"ftp_list":            "ftp_list(config, remote_dir) lists directory entries over FTP",
	"ftp_get":             "ftp_get(config, remote_path, local_path) downloads file over FTP",
	"ftp_put":             "ftp_put(config, local_path, remote_path) uploads file over FTP",
	"sftp_list":           "sftp_list(config, remote_dir) lists directory entries over SFTP",
	"sftp_get":            "sftp_get(config, remote_path, local_path) downloads file over SFTP",
	"sftp_put":            "sftp_put(config, local_path, remote_path) uploads file over SFTP",
	"bulk_rename":         "bulk_rename(dir[, opts]) previews or applies bulk file renames; opts include match, template, apply",
	"file_search":         "file_search(root[, opts]) searches files/directories by glob, literal, or regex patterns plus name/path/content/size/time/sort/limit filters",
	"file_locate":         "file_locate(root[, opts]) alias for file_search",
	"file_finder":         "file_finder(root) creates a chainable filesystem finder with glob, regex, content, path, size, time, sort, and limit filters",
	"file_move_plan":      "file_move_plan(src, dst[, opts]) previews or applies a file move",
	"file_copy_plan":      "file_copy_plan(src, dst[, opts]) previews or applies a file copy",
	"file_dedupe":         "file_dedupe(root[, opts]) plans duplicate-file matches by content hash",
	"file_remove_plan":    "file_remove_plan(path[, opts]) previews or removes a file or directory",
	"file_organize":       "file_organize(src_dir, dst_dir[, opts]) previews or moves files into extension folders",
	"file_checksum":       "file_checksum(path) returns a SHA-256 checksum and file size",
	"archive_compress":    "archive_compress(src, dst[, opts]) previews or creates zip/tar/gzip archives",
	"archive_extract":     "archive_extract(src, dst[, opts]) previews or extracts supported archives",
	"archive_list":        "archive_list(path) lists supported archive entries",
	"image_convert_batch": "image_convert_batch(src_dir, dst_dir[, opts]) previews or converts image files in bulk",
	"image_optimize":      "image_optimize(src_dir, dst_dir[, opts]) previews or re-encodes images for smaller output",
	"image_crop_file":     "image_crop_file(src, dst[, opts]) previews or crops one image file",
	"image_thumbnail":     "image_thumbnail(src, dst[, opts]) previews or creates a thumbnail image",
	"image_info_file":     "image_info_file(path) returns metadata for an image file",
	"office_text":         "office_text(path) extracts text from txt, csv, json, docx, or xlsx files",
	"office_read":         "office_read(path) reads supported office/data files into structured rows, values, or text",
	"secret_generate":     "secret_generate([length][, alphabet]) returns a masked generated secret",
	"token_generate":      "token_generate([bytes]) returns a masked URL-safe token",
	"file_encrypt":        "file_encrypt(src, dst, passphrase[, opts]) previews or AES-GCM encrypts a file",
	"file_decrypt":        "file_decrypt(src, dst, passphrase[, opts]) previews or AES-GCM decrypts a file",
	"media_info":          "media_info(path) probes media metadata, using ffprobe when available",
	"media_convert":       "media_convert(src, dst[, opts]) previews or converts media with ffmpeg",
	"ffmpeg_status":       "ffmpeg_status() reports ffmpeg/ffprobe availability and installer command",
	"ffmpeg_install":      "ffmpeg_install([opts]) previews or runs the detected OS ffmpeg installer",
	"system_info":         "system_info() returns safe host/runtime metadata when system capability is allowed",
	"dns_lookup":          "dns_lookup(host) resolves host addresses under network policy",
	"tcp_check":           "tcp_check(address[, timeout_ms]) checks TCP connectivity under network policy",
	"http_probe":          "http_probe(url[, timeout_ms]) sends an HTTP HEAD probe under network policy",
	"assert_true":         "assert_true(condition[, message]) fails test when condition is false",
	"assert_eq":           "assert_eq(actual, expected[, message]) fails test when values differ",
	"assert_neq":          "assert_neq(actual, unexpected[, message]) fails test when values are equal",
	"assert_contains":     "assert_contains(haystack, needle[, message]) fails test when needle not found in haystack string or array",
	"assert_throws":       "assert_throws(fn[, message]) fails test when fn does not produce an error",
	"test_summary":        "test_summary() returns {total, passed, failed}",
	"run_tests":           "run_tests(path_or_paths) executes SPL test scripts and returns summary",
	"exec":                "exec(command, ...args[, timeout_ms]) runs a whitelisted OS command; disabled by SPL_DISABLE_EXEC or host protection",
	"config_load":         "config_load(path[, format]) loads JSON/YAML/.env config and wraps secret-like fields",
	"config_parse":        "config_parse(raw, format) parses JSON/YAML/.env string and wraps secret-like fields",
	"secret":              "secret(value) wraps a string as non-displayable secret",
	"secret_reveal":       "secret_reveal(secret_value) reveals a SECRET as plain STRING",
	"secret_mask":         "secret_mask(value[, visible]) returns masked display string",
	"Error":               "Error(message[, details]) returns structured error object with message, code, stack",
	"channel":             "channel([buffer_size]) creates a message channel",
	"send":                "send(channel, value) sends a value to channel",
	"recv":                "recv(channel) receives a value from channel",
	"go":                  "go(fn[, ...args]) runs function asynchronously and returns future",
	"generator":           "generator(fn) wraps function result as lazy iterable",
	"permissions":         "permissions(policy_hash) applies runtime allow/deny policy",
	"metric":              "metric(name, value[, labels]) records metric point",
	"trace":               "trace(name[, attrs]) emits trace event",
	"immutable":           "immutable(value) returns deeply frozen copy",
	"move":                "move(value) transfers ownership marker to current scope",
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

// BuiltinByName looks up a builtin by name, checking the global (import-free)
// registry first and falling back to the plugin registry. Std-module export
// resolution relies on this fallback to reach plugin functions that are
// intentionally absent from Builtins.
func BuiltinByName(name string) (*object.Builtin, bool) {
	builtinsMu.Lock()
	fn, ok := Builtins[name]
	builtinsMu.Unlock()
	if ok {
		return fn, true
	}
	pluginBuiltinsMu.Lock()
	defer pluginBuiltinsMu.Unlock()
	fn, ok = PluginBuiltins[name]
	return fn, ok
}

// HasBuiltin reports whether a builtin with the given name is registered,
// either as a global builtin or as a plugin builtin reachable via import.
func HasBuiltin(name string) bool {
	_, ok := BuiltinByName(name)
	return ok
}

func unwrapOwned(obj object.Object) object.Object {
	if ov, ok := obj.(*object.OwnedValue); ok {
		return ov.Inner
	}
	return obj
}

func init() {
	RegisterBuiltins(map[string]*object.Builtin{
		"stream": {FnWithEnv: func(env *object.Environment, args ...object.Object) object.Object {
			if len(args) != 1 {
				return object.NewError("stream() takes 1 argument, got %d", len(args))
			}
			arg := unwrapOwned(args[0])
			if arr, ok := arg.(*object.Array); ok {
				return &object.Stream{Elements: append([]object.Object(nil), arr.Elements...)}
			}
			return object.NewError("stream() expects an array argument")
		}},
		"stream_map": {FnWithEnv: func(env *object.Environment, args ...object.Object) object.Object {
			if len(args) != 2 {
				return object.NewError("stream_map() takes 2 arguments, got %d", len(args))
			}
			var elements []object.Object
			arg0 := unwrapOwned(args[0])
			switch s := arg0.(type) {
			case *object.Stream:
				elements = s.Elements
			case *object.Array:
				elements = s.Elements
			default:
				return object.NewError("stream_map() first argument must be a stream or array")
			}
			fn := args[1]
			result := make([]object.Object, 0, len(elements))
			for _, elem := range elements {
				mapped := ApplyFunction(fn, []object.Object{elem}, env, nil)
				if object.IsError(mapped) {
					return mapped
				}
				result = append(result, mapped)
			}
			return &object.Stream{Elements: result}
		}},
		"stream_filter": {FnWithEnv: func(env *object.Environment, args ...object.Object) object.Object {
			if len(args) != 2 {
				return object.NewError("stream_filter() takes 2 arguments, got %d", len(args))
			}
			var elements []object.Object
			arg0 := unwrapOwned(args[0])
			switch s := arg0.(type) {
			case *object.Stream:
				elements = s.Elements
			case *object.Array:
				elements = s.Elements
			default:
				return object.NewError("stream_filter() first argument must be a stream or array")
			}
			fn := args[1]
			result := make([]object.Object, 0, len(elements))
			for _, elem := range elements {
				keep := ApplyFunction(fn, []object.Object{elem}, env, nil)
				if object.IsError(keep) {
					return keep
				}
				if object.IsTruthy(keep) {
					result = append(result, elem)
				}
			}
			return &object.Stream{Elements: result}
		}},
		"stream_reduce": {FnWithEnv: func(env *object.Environment, args ...object.Object) object.Object {
			if len(args) != 3 {
				return object.NewError("stream_reduce() takes 3 arguments, got %d", len(args))
			}
			var elements []object.Object
			arg0 := unwrapOwned(args[0])
			switch s := arg0.(type) {
			case *object.Stream:
				elements = s.Elements
			case *object.Array:
				elements = s.Elements
			default:
				return object.NewError("stream_reduce() first argument must be a stream or array")
			}
			fn := args[1]
			acc := args[2]
			for _, elem := range elements {
				acc = ApplyFunction(fn, []object.Object{acc, elem}, env, nil)
				if object.IsError(acc) {
					return acc
				}
			}
			return acc
		}},
		"stream_collect": {FnWithEnv: func(env *object.Environment, args ...object.Object) object.Object {
			if len(args) != 1 {
				return object.NewError("stream_collect() takes 1 argument, got %d", len(args))
			}
			arg0 := unwrapOwned(args[0])
			switch s := arg0.(type) {
			case *object.Stream:
				return &object.Array{Elements: append([]object.Object(nil), s.Elements...)}
			case *object.Array:
				return s
			default:
				return object.NewError("stream_collect() expects a stream or array")
			}
		}},
		"stream_to_array": {FnWithEnv: func(env *object.Environment, args ...object.Object) object.Object {
			if len(args) != 1 {
				return object.NewError("stream_to_array() takes 1 argument, got %d", len(args))
			}
			arg0 := unwrapOwned(args[0])
			switch s := arg0.(type) {
			case *object.Stream:
				return &object.Array{Elements: append([]object.Object(nil), s.Elements...)}
			case *object.Array:
				return s
			default:
				return object.NewError("stream_to_array() expects a stream or array")
			}
		}},
	})
}
