package interpreter

import (
	"fmt"
	"strings"
	"sync"
	"time"

	builtinspkg "github.com/oarkflow/interpreter/pkg/builtins"
	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/security"
)

type Plugin interface {
	Name() string
	Register(*Runtime) error
}

type PluginFunc struct {
	PluginName string
	Fn         func(*Runtime) error
}

func (p PluginFunc) Name() string {
	if strings.TrimSpace(p.PluginName) == "" {
		return "anonymous"
	}
	return p.PluginName
}

func (p PluginFunc) Register(rt *Runtime) error {
	if p.Fn == nil {
		return nil
	}
	return p.Fn(rt)
}

func RegisterRuntimeBuiltins(group map[string]*object.Builtin) {
	eval.RegisterBuiltins(group)
}

type EmbeddedLanguageContext = eval.EmbeddedLanguageContext
type EmbeddedLanguageHandler = eval.EmbeddedLanguageHandler

func RegisterEmbeddedLanguage(tag string, handler EmbeddedLanguageHandler) error {
	return eval.RegisterEmbeddedLanguage(tag, handler)
}

func EmbeddedLanguageTags() []string {
	return eval.EmbeddedLanguageTags()
}

func RegisterStdModule(name string, exports map[string]Object) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("std module name cannot be empty")
	}
	if exports == nil {
		exports = map[string]Object{}
	}
	stdModules.mu.Lock()
	defer stdModules.mu.Unlock()
	stdModules.items[name] = exports
	return nil
}

func RegisterStdBuiltinModule(name string, builtinNames ...string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("std module name cannot be empty")
	}
	stdModules.mu.Lock()
	defer stdModules.mu.Unlock()
	stdModules.builtinItems[name] = append([]string(nil), builtinNames...)
	return nil
}

func LookupStdModule(name string) (map[string]Object, bool) {
	name = strings.TrimSpace(name)
	stdModules.mu.RLock()
	exports, ok := stdModules.items[name]
	builtinNames, builtinOK := stdModules.builtinItems[name]
	stdModules.mu.RUnlock()
	if name == "builtins" {
		names := eval.BuiltinNames()
		out := make(map[string]Object, len(names))
		for _, builtinName := range names {
			if fn, ok := eval.BuiltinByName(builtinName); ok {
				out[builtinName] = fn
			}
		}
		return out, true
	}
	if ok {
		out := make(map[string]Object, len(exports))
		for k, v := range exports {
			out[k] = v
		}
		return out, true
	}
	if builtinOK {
		out := make(map[string]Object, len(builtinNames))
		for _, builtinName := range builtinNames {
			if fn, ok := eval.BuiltinByName(builtinName); ok {
				out[builtinName] = fn
			} else if optionalBuiltinModule(name) {
				out[builtinName] = unavailableBuiltin(name, builtinName)
			}
		}
		if len(out) == 0 {
			return nil, false
		}
		return out, true
	}
	return nil, false
}

func optionalBuiltinModule(name string) bool {
	switch name {
	case "database", "images", "integrations", "tools/files", "tools/archive", "tools/images", "tools/office", "tools/secrets", "tools/media", "tools/system", "tools/network", "cryptoextra", "yaml", "config/yaml", "xql":
		return true
	default:
		return false
	}
}

func unavailableBuiltin(moduleName, builtinName string) *object.Builtin {
	return &object.Builtin{Fn: func(args ...object.Object) object.Object {
		return object.NewError("optional builtin %q from module %q is not linked into this interpreter; use cmd/interpreter-full, import the optional Go package in your embedding host, or build a custom preset", builtinName, moduleName)
	}}
}

var stdModules = struct {
	mu           sync.RWMutex
	items        map[string]map[string]Object
	builtinItems map[string][]string
}{items: map[string]map[string]Object{}, builtinItems: map[string][]string{}}

func init() {
	_ = RegisterStdBuiltinModule("std/core", "help", "sprintf", "printf", "interpolate", "len", "type")
	_ = RegisterStdBuiltinModule("core", "help", "sprintf", "printf", "interpolate", "len", "type")
	_ = RegisterStdBuiltinModule("std/fs", "read_file", "write_file", "file_exists", "remove_file", "readdir", "glob", "mkdir", "rmdir", "stat")
	_ = RegisterStdBuiltinModule("fs", "read_file", "write_file", "file_exists", "remove_file", "readdir", "glob", "mkdir", "rmdir", "stat")
	_ = RegisterStdBuiltinModule("std/render", "file", "image", "render")
	_ = RegisterStdBuiltinModule("render", "file", "image", "render")
	_ = RegisterStdBuiltinModule("std/test", "assert_true", "assert_eq", "assert_neq", "assert_contains", "assert_throws", "test_summary", "run_tests")
	_ = RegisterStdBuiltinModule("test", "assert_true", "assert_eq", "assert_neq", "assert_contains", "assert_throws", "test_summary", "run_tests")
	_ = RegisterStdBuiltinModule("std/config", "config_load", "config_parse", "secret", "secret_reveal", "secret_mask")
	_ = RegisterStdBuiltinModule("config", "config_load", "config_parse", "secret", "secret_reveal", "secret_mask")
	_ = RegisterStdBuiltinModule("std/math", "PI", "E", "INF", "NAN", "abs", "pow", "sqrt", "min", "max", "clamp", "round", "floor", "ceil", "sin", "cos", "tan", "asin", "acos", "atan", "atan2", "log", "log2", "log10", "exp", "hypot", "cbrt", "mod", "sign", "trunc", "round_to", "lerp", "normalize", "map_range", "percent", "factorial", "gcd", "lcm", "is_nan", "is_inf", "is_finite", "is_integer", "is_prime", "to_radians", "to_degrees", "mean", "median", "mode", "variance", "stddev", "percentile")
	_ = RegisterStdBuiltinModule("math", "PI", "E", "INF", "NAN", "abs", "pow", "sqrt", "min", "max", "clamp", "round", "floor", "ceil", "sin", "cos", "tan", "asin", "acos", "atan", "atan2", "log", "log2", "log10", "exp", "hypot", "cbrt", "mod", "sign", "trunc", "round_to", "lerp", "normalize", "map_range", "percent", "factorial", "gcd", "lcm", "is_nan", "is_inf", "is_finite", "is_integer", "is_prime", "to_radians", "to_degrees", "mean", "median", "mode", "variance", "stddev", "percentile")
	_ = RegisterStdBuiltinModule("std/random", "random", "seed_random", "random_range", "random_float", "random_choice", "shuffle", "sample", "random_bytes", "random_string")
	_ = RegisterStdBuiltinModule("random", "random", "seed_random", "random_range", "random_float", "random_choice", "shuffle", "sample", "random_bytes", "random_string")
	_ = RegisterStdBuiltinModule("std/string", "upper", "lower", "split", "join", "trim", "replace", "replace_n", "starts_with", "ends_with", "contains", "index_of", "last_index_of", "substring", "repeat", "title", "slug", "snake_case", "kebab_case", "camel_case", "pascal_case", "swap_case", "count_substr", "truncate", "split_lines", "trim_prefix", "trim_suffix", "trim_chars", "pad_left", "pad_right", "words", "chars", "reverse_string", "is_blank", "is_numeric", "is_alpha", "is_alnum", "escape_html", "unescape_html", "url_encode", "url_decode", "base64_encode", "base64_decode", "regex_match", "regex_replace", "regex_find_all", "regex_split")
	_ = RegisterStdBuiltinModule("string", "upper", "lower", "split", "join", "trim", "replace", "replace_n", "starts_with", "ends_with", "contains", "index_of", "last_index_of", "substring", "repeat", "title", "slug", "snake_case", "kebab_case", "camel_case", "pascal_case", "swap_case", "count_substr", "truncate", "split_lines", "trim_prefix", "trim_suffix", "trim_chars", "pad_left", "pad_right", "words", "chars", "reverse_string", "is_blank", "is_numeric", "is_alpha", "is_alnum", "escape_html", "unescape_html", "url_encode", "url_decode", "base64_encode", "base64_decode", "regex_match", "regex_replace", "regex_find_all", "regex_split")
	_ = RegisterStdBuiltinModule("std/array", "len", "first", "last", "rest", "push", "reverse", "slice", "range", "sort", "sort_by", "uniq", "unique", "find", "contains", "sum", "avg", "mean", "median", "mode", "variance", "stddev", "percentile", "compact", "flatten", "any", "all", "zip", "chunk", "partition", "take", "drop", "pluck", "index_by", "random_choice", "shuffle", "sample")
	_ = RegisterStdBuiltinModule("array", "len", "first", "last", "rest", "push", "reverse", "slice", "range", "sort", "sort_by", "uniq", "unique", "find", "contains", "sum", "avg", "mean", "median", "mode", "variance", "stddev", "percentile", "compact", "flatten", "any", "all", "zip", "chunk", "partition", "take", "drop", "pluck", "index_by", "random_choice", "shuffle", "sample")
	_ = RegisterStdBuiltinModule("std/hash", "keys", "values", "has_key", "get", "merge", "delete_key", "pick", "omit", "entries", "from_entries", "group_by", "index_by", "deep_equal", "coalesce", "default")
	_ = RegisterStdBuiltinModule("hash", "keys", "values", "has_key", "get", "merge", "delete_key", "pick", "omit", "entries", "from_entries", "group_by", "index_by", "deep_equal", "coalesce", "default")
	_ = RegisterStdBuiltinModule("std/time", "time", "now", "time_ms", "now_iso", "now_format", "format_time", "parse_time", "date_with_format", "time_add", "time_sub", "time_diff", "start_of_day", "end_of_day", "unix_to_iso", "unix_ms_to_iso", "iso_to_unix", "iso_to_unix_ms", "parse_time_tz", "format_time_tz", "to_timezone", "start_of_week", "end_of_week", "start_of_month", "end_of_month", "add_months", "parse_duration", "format_duration", "is_weekend", "weekday", "month", "year")
	_ = RegisterStdBuiltinModule("time", "time", "now", "time_ms", "now_iso", "now_format", "format_time", "parse_time", "date_with_format", "time_add", "time_sub", "time_diff", "start_of_day", "end_of_day", "unix_to_iso", "unix_ms_to_iso", "iso_to_unix", "iso_to_unix_ms", "parse_time_tz", "format_time_tz", "to_timezone", "start_of_week", "end_of_week", "start_of_month", "end_of_month", "add_months", "parse_duration", "format_duration", "is_weekend", "weekday", "month", "year")
	_ = RegisterStdBuiltinModule("std/json", "read_json", "write_json", "json_parse", "json_stringify", "json_encode", "json_decode")
	_ = RegisterStdBuiltinModule("json", "read_json", "write_json", "json_parse", "json_stringify", "json_encode", "json_decode")
	_ = RegisterStdBuiltinModule("std/csv", "read_csv", "write_csv", "csv_decode", "csv_encode", "table_rows", "table_columns", "table_select", "table_filter", "table_map")
	_ = RegisterStdBuiltinModule("csv", "read_csv", "write_csv", "csv_decode", "csv_encode", "table_rows", "table_columns", "table_select", "table_filter", "table_map")
	_ = RegisterStdBuiltinModule("std/crypto", "md5", "sha256", "sha512", "hash", "hmac", "hmac_sha256", "hmac_sha512", "password_hash", "password_verify", "encrypt", "decrypt", "constant_time_eq", "base64_encode", "base64_decode", "hex_encode", "hex_decode", "random_bytes", "random_string")
	_ = RegisterStdBuiltinModule("crypto", "md5", "sha256", "sha512", "hash", "hmac", "hmac_sha256", "hmac_sha512", "password_hash", "password_verify", "encrypt", "decrypt", "constant_time_eq", "base64_encode", "base64_decode", "hex_encode", "hex_decode", "random_bytes", "random_string")
	_ = RegisterStdBuiltinModule("std/path", "basename", "dirname", "path_join", "path_base", "path_dir", "path_ext", "path_clean", "path_abs")
	_ = RegisterStdBuiltinModule("path", "basename", "dirname", "path_join", "path_base", "path_dir", "path_ext", "path_clean", "path_abs")
	_ = RegisterStdBuiltinModule("database", "db_connect", "db_query", "db_exec", "db_begin", "db_commit", "db_rollback", "db_tables", "db_close", "query", "lazy_query")
	_ = RegisterStdBuiltinModule("images", "image_load", "image_resize", "image_crop", "image_rotate", "image_convert", "image_save", "image_info", "image_render", "image_resize_file", "image_convert_file")
	_ = RegisterStdBuiltinModule("integrations", "http_request", "http_get", "http_post", "webhook", "smtp_send", "ftp_list", "ftp_get", "ftp_put", "sftp_list", "sftp_get", "sftp_put")
	_ = RegisterStdBuiltinModule("tools/files", "bulk_rename", "file_search", "file_locate", "file_move_plan", "file_copy_plan", "file_dedupe", "file_remove_plan", "file_organize", "file_checksum")
	_ = RegisterStdBuiltinModule("tools/archive", "archive_compress", "archive_extract", "archive_list")
	_ = RegisterStdBuiltinModule("tools/images", "image_convert_batch", "image_optimize", "image_crop_file", "image_resize_file", "image_thumbnail", "image_info_file")
	_ = RegisterStdBuiltinModule("tools/office", "office_text", "office_read")
	_ = RegisterStdBuiltinModule("tools/secrets", "secret_generate", "token_generate", "file_encrypt", "file_decrypt")
	_ = RegisterStdBuiltinModule("tools/media", "media_info", "media_convert", "ffmpeg_status", "ffmpeg_install")
	_ = RegisterStdBuiltinModule("tools/system", "system_info")
	_ = RegisterStdBuiltinModule("tools/network", "dns_lookup", "tcp_check", "http_probe")
	_ = RegisterStdBuiltinModule("xql", "xql_run", "xql_connect", "xql_list_integrations")
	_ = RegisterStdModule("native/os", builtinspkg.NativeOSModule())
	_ = RegisterStdBuiltinModule("cryptoextra", "bcrypt_hash", "bcrypt_verify")
	_ = RegisterStdBuiltinModule("yaml", "config_load", "config_parse")
	_ = RegisterStdBuiltinModule("config/yaml", "config_load", "config_parse")
}

func CapabilityPreset(name string, moduleDir string) (*SecurityPolicy, SandboxConfig, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		name = "trusted"
	}
	if strings.TrimSpace(moduleDir) == "" {
		moduleDir = "."
	}
	cfg := DefaultExecSandboxConfig()
	cfg.BaseDir = moduleDir
	policy := &SecurityPolicy{AllowEnvWrite: true}

	switch name {
	case "trusted":
		return policy, cfg, nil
	case "untrusted", "readonly":
		cfg.StrictMode = true
		cfg.ProtectHost = true
		cfg.AllowEnvWrite = false
		cfg.MaxDepth = 128
		cfg.MaxSteps = 500_000
		cfg.MaxHeapMB = 64
		cfg.MaxOutputBytes = 64 * 1024
		cfg.MaxHTTPBodyBytes = 64 * 1024
		cfg.MaxExecOutputBytes = 64 * 1024
		cfg.Timeout = 2 * time.Second
		policy = &SecurityPolicy{
			StrictMode:          true,
			ProtectHost:         true,
			AllowedCapabilities: []string{security.CapabilityFilesystemRead},
			AllowedFileReadPaths: []string{
				moduleDir,
			},
		}
	case "networked":
		policy, cfg, _ = CapabilityPreset("untrusted", moduleDir)
		policy.AllowedCapabilities = append(policy.AllowedCapabilities, security.CapabilityNetwork)
	case "data-processing":
		policy, cfg, _ = CapabilityPreset("untrusted", moduleDir)
		policy.AllowedCapabilities = append(policy.AllowedCapabilities, security.CapabilityDB)
	case "automation":
		policy, cfg, _ = CapabilityPreset("untrusted", moduleDir)
		policy.AllowedCapabilities = append(policy.AllowedCapabilities, security.CapabilityExec, security.CapabilityFilesystemWrite, security.CapabilitySystem)
	case "server":
		policy, cfg, _ = CapabilityPreset("untrusted", moduleDir)
		policy.AllowedCapabilities = append(policy.AllowedCapabilities, security.CapabilityServer, security.CapabilityNetwork)
	default:
		return nil, SandboxConfig{}, fmt.Errorf("unknown capability preset %q", name)
	}
	return policy, cfg, nil
}
