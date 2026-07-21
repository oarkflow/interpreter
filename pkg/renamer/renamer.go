// Package renamer provides reusable, preview-first file rename and move operations.
package renamer

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type DateFormat struct {
	Input  string `json:"input"`
	Output string `json:"output"`
}

type Rule struct {
	Name        string     `json:"name"`
	Pattern     string     `json:"pattern"`
	Replacement string     `json:"replacement"`
	Type        string     `json:"type"`
	Priority    int        `json:"priority,omitempty"`
	DateFormat  DateFormat `json:"date_format,omitempty"`
	Recursive   bool       `json:"recursive"`
	FileTypes   []string   `json:"file_types,omitempty"`
}

// RenameRule is retained as a descriptive alias for callers loading configs.
type RenameRule = Rule

type Category struct {
	Name       string   `json:"name"`
	Extensions []string `json:"extensions,omitempty"`
	Patterns   []string `json:"patterns,omitempty"`
	TargetDir  string   `json:"target_dir"`
}

type Config struct {
	Rules           []Rule              `json:"rules"`
	Categories      []Category          `json:"categories,omitempty"`
	ExtensionGroups map[string][]string `json:"extension_groups,omitempty"`
	GroupFolders    map[string]string   `json:"group_folders,omitempty"`
}

// Operation describes either a planned or completed filesystem change.
type Operation struct {
	Action string `json:"action"`
	From   string `json:"from"`
	To     string `json:"to"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

var DefaultExtensionGroups = map[string][]string{
	"image": {"jpg", "jpeg", "png", "gif", "bmp", "webp", "svg", "heic", "heif", "tif", "tiff", "ico", "eps", "psd", "raw", "cr2", "nef", "arw", "orf"},
	"text":  {"txt", "md", "rtf"}, "data": {"json", "ndjson", "csv", "tsv", "parquet", "orc", "avro", "db", "sqlite", "sql"},
	"video":    {"mp4", "avi", "mkv", "mov", "wmv", "flv", "webm", "3gp", "mts", "m2ts", "m4v"},
	"audio":    {"mp3", "wav", "flac", "m4a", "ogg", "aac", "opus"},
	"document": {"pdf", "doc", "docx", "xls", "xlsx", "ppt", "pptx", "odt", "ods", "pages", "key", "numbers"},
	"archive":  {"zip", "tar", "gz", "tgz", "rar", "7z", "xz", "bz2"},
	"code":     {"py", "js", "ts", "go", "java", "rb", "php", "html", "css", "c", "cpp", "h", "cs", "rs", "kt", "swift", "scala", "sh"},
	"font":     {"ttf", "otf", "woff", "woff2"}, "ebook": {"epub", "mobi"}, "logs": {"log"},
}

var DefaultGroupFolderNames = map[string]string{
	"image": "Images", "text": "Text", "data": "Data", "video": "Videos", "audio": "Audio",
	"document": "Documents", "archive": "Archives", "code": "Code", "font": "Fonts", "ebook": "Ebooks", "logs": "Logs",
}

func MergeExtensionGroups(user map[string][]string) map[string][]string {
	out := make(map[string][]string, len(DefaultExtensionGroups)+len(user))
	for k, v := range DefaultExtensionGroups {
		out[k] = append([]string(nil), v...)
	}
	for k, values := range user {
		seen := map[string]bool{}
		out[k] = nil
		for _, value := range values {
			value = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(value), "."))
			if value != "" && !seen[value] {
				out[k] = append(out[k], value)
				seen[value] = true
			}
		}
	}
	return out
}

func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	for i := range cfg.Rules {
		if cfg.Rules[i].Type == "" {
			cfg.Rules[i].Type = "simple"
		}
	}
	cfg.ExtensionGroups = MergeExtensionGroups(cfg.ExtensionGroups)
	return &cfg, nil
}

// Rename applies one rule to matching files. Dry-run operations have status "planned".
func Rename(rule Rule, targetDir string, dryRun bool, groups map[string][]string) ([]Operation, error) {
	if rule.Type == "" {
		rule.Type = "simple"
	}
	groups = MergeExtensionGroups(groups)
	allowed := expandExtensions(rule.FileTypes, groups)
	var files []string
	err := filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if !rule.Recursive && path != targetDir {
				return filepath.SkipDir
			}
			return nil
		}
		if len(allowed) > 0 && !contains(allowed, strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))) {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	ops := make([]Operation, 0)
	counter := 1
	for _, src := range files {
		name, err := ApplyRule(rule, src, groups, counter)
		if err != nil {
			ops = append(ops, Operation{Action: "rename", From: src, Status: "failed", Error: err.Error()})
			continue
		}
		if name == "" || name == filepath.Base(src) {
			continue
		}
		if filepath.Base(name) != name {
			ops = append(ops, Operation{Action: "rename", From: src, To: name, Status: "failed", Error: "replacement must be a file name"})
			continue
		}
		dst := filepath.Join(filepath.Dir(src), name)
		op := Operation{Action: "rename", From: src, To: dst, Status: "planned"}
		if _, err := os.Lstat(dst); err == nil {
			op.Status, op.Error = "failed", "destination already exists"
		} else if !os.IsNotExist(err) {
			op.Status, op.Error = "failed", err.Error()
		}
		if !dryRun && op.Status == "planned" {
			if err := os.Rename(src, dst); err != nil {
				op.Status, op.Error = "failed", err.Error()
			} else {
				op.Status = "applied"
			}
		}
		ops = append(ops, op)
		counter++
	}
	return ops, nil
}

// Move renames a file or directory to dst. It never overwrites an existing path.
func Move(src, dst string, dryRun bool) Operation {
	op := Operation{Action: "move", From: src, To: dst, Status: "planned"}
	if _, err := os.Lstat(src); err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	if _, err := os.Lstat(dst); err == nil {
		op.Status, op.Error = "failed", "destination already exists"
		return op
	} else if !os.IsNotExist(err) {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	if dryRun {
		return op
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	if err := os.Rename(src, dst); err != nil {
		op.Status, op.Error = "failed", err.Error()
	} else {
		op.Status = "applied"
	}
	return op
}

// Group moves files below targetDir into extension-group folders.
func Group(targetDir string, groups map[string][]string, folders map[string]string, dryRun bool) ([]Operation, error) {
	groups = MergeExtensionGroups(groups)
	extToGroup := map[string]string{}
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, ext := range groups[name] {
			ext = strings.ToLower(strings.TrimPrefix(ext, "."))
			if _, exists := extToGroup[ext]; !exists {
				extToGroup[ext] = name
			}
		}
	}
	var files []string
	if err := filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		files = append(files, path)
		return nil
	}); err != nil {
		return nil, err
	}
	var ops []Operation
	for _, src := range files {
		group := extToGroup[strings.ToLower(strings.TrimPrefix(filepath.Ext(src), "."))]
		if group == "" {
			group = "Other"
		}
		folder := folders[group]
		if folder == "" {
			folder = DefaultGroupFolderNames[group]
		}
		if folder == "" {
			folder = strings.ToUpper(group[:1]) + group[1:]
			if group != "Other" && !strings.HasSuffix(folder, "s") {
				folder += "s"
			}
		}
		dstDir := filepath.Join(targetDir, folder)
		rel, err := filepath.Rel(dstDir, src)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		op := Move(src, filepath.Join(dstDir, filepath.Base(src)), dryRun)
		op.Action = "group"
		ops = append(ops, op)
	}
	return ops, nil
}

// Categorize moves files matching category extensions or regular expressions.
func Categorize(targetDir string, categories []Category, groups map[string][]string, dryRun bool) ([]Operation, error) {
	groups = MergeExtensionGroups(groups)
	var ops []Operation
	for _, category := range categories {
		allowed := expandExtensions(category.Extensions, groups)
		patterns := make([]*regexp.Regexp, 0, len(category.Patterns))
		for _, raw := range category.Patterns {
			re, err := regexp.Compile(raw)
			if err != nil {
				return ops, fmt.Errorf("category %q: %w", category.Name, err)
			}
			patterns = append(patterns, re)
		}
		var files []string
		if err := filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				files = append(files, path)
			}
			return nil
		}); err != nil {
			return ops, err
		}
		for _, src := range files {
			dstDir := filepath.Join(targetDir, category.TargetDir)
			rel, _ := filepath.Rel(dstDir, src)
			if rel != "" && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
			name := filepath.Base(src)
			matched := contains(allowed, strings.ToLower(strings.TrimPrefix(filepath.Ext(name), ".")))
			if !matched {
				for _, re := range patterns {
					if re.MatchString(name) {
						matched = true
						break
					}
				}
			}
			if matched && category.TargetDir != "" {
				op := Move(src, filepath.Join(dstDir, name), dryRun)
				op.Action = "categorize"
				ops = append(ops, op)
			}
		}
	}
	return ops, nil
}

func ApplyRule(rule Rule, filePath string, groups map[string][]string, counter int) (string, error) {
	name := filepath.Base(filePath)
	switch rule.Type {
	case "", "simple":
		return applySimple(rule, name, groups, counter)
	case "regex":
		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return "", err
		}
		if !re.MatchString(name) {
			return "", nil
		}
		return re.ReplaceAllString(name, rule.Replacement), nil
	case "advanced":
		return applyAdvanced(rule, name)
	default:
		return "", fmt.Errorf("unknown rule type %q", rule.Type)
	}
}

func applySimple(rule Rule, name string, groups map[string][]string, counter int) (string, error) {
	var pattern strings.Builder
	pattern.WriteByte('^')
	tokens := []string{}
	for i := 0; i < len(rule.Pattern); {
		if rule.Pattern[i] == '*' {
			pattern.WriteString("(.*?)")
			tokens = append(tokens, "any")
			i++
			continue
		}
		if rule.Pattern[i] == '{' {
			if end := strings.IndexByte(rule.Pattern[i:], '}'); end >= 0 {
				token := rule.Pattern[i+1 : i+end]
				switch token {
				case "number":
					pattern.WriteString(`(\d+)`)
				case "text":
					pattern.WriteString(`([^\.]+)`)
				case "any":
					pattern.WriteString(`(.*?)`)
				case "date":
					pattern.WriteString(`(\d{8,14})`)
				default:
					values := groups[token]
					if len(values) == 0 {
						pattern.WriteString(regexp.QuoteMeta(rule.Pattern[i : i+end+1]))
						i += end + 1
						continue
					}
					quoted := make([]string, len(values))
					for n, value := range values {
						quoted[n] = regexp.QuoteMeta(strings.TrimPrefix(value, "."))
					}
					pattern.WriteString("(" + strings.Join(quoted, "|") + ")")
				}
				tokens = append(tokens, token)
				i += end + 1
				continue
			}
		}
		pattern.WriteString(regexp.QuoteMeta(string(rule.Pattern[i])))
		i++
	}
	pattern.WriteByte('$')
	re, err := regexp.Compile(pattern.String())
	if err != nil {
		return "", err
	}
	matches := re.FindStringSubmatch(name)
	if matches == nil {
		return "", nil
	}
	values := map[string][]string{}
	for i, token := range tokens {
		values[token] = append(values[token], matches[i+1])
	}
	indexes := map[string]int{}
	out := regexp.MustCompile(`\{[^{}]+\}|\*`).ReplaceAllStringFunc(rule.Replacement, func(raw string) string {
		token := strings.Trim(raw, "{}")
		if raw == "*" {
			token = "any"
		}
		if token == "auto" {
			return fmt.Sprintf("%03d", counter)
		}
		idx := indexes[token]
		if idx >= len(values[token]) {
			return raw
		}
		indexes[token]++
		value := values[token][idx]
		if token == "date" {
			if formatted, e := formatDate(value, rule.DateFormat); e == nil {
				return formatted
			}
		}
		return value
	})
	return out, nil
}

func applyAdvanced(rule Rule, name string) (string, error) {
	re, err := regexp.Compile(rule.Pattern)
	if err != nil {
		return "", err
	}
	match := re.FindStringSubmatch(name)
	if match == nil {
		return "", nil
	}
	out := re.ReplaceAllString(name, rule.Replacement)
	if strings.Contains(out, "{date}") {
		for _, value := range match[1:] {
			if regexp.MustCompile(`^\d{8,14}$`).MatchString(value) {
				if v, e := formatDate(value, rule.DateFormat); e == nil {
					out = strings.ReplaceAll(out, "{date}", v)
					break
				}
			}
		}
	}
	return out, nil
}

func formatDate(value string, spec DateFormat) (string, error) {
	input, output := "20060102150405", "2006-01-02"
	if spec.Input != "" {
		input = convertDateFormat(spec.Input)
	}
	if spec.Output != "" {
		output = convertDateFormat(spec.Output)
	}
	formats := []string{input, "20060102150405", "200601021504", "20060102"}
	var last error
	for _, layout := range formats {
		if len(value) > len(layout) {
			continue
		}
		t, err := time.Parse(layout[:len(value)], value)
		if err == nil {
			return t.Format(output), nil
		}
		last = err
	}
	return "", fmt.Errorf("failed to parse date %s: %v", value, last)
}

func convertDateFormat(value string) string {
	r := strings.NewReplacer("yyyy", "2006", "yy", "06", "dd", "02", "hh", "15", "MM", "01", "mm", "01", "ss", "05")
	return r.Replace(value)
}

func expandExtensions(values []string, groups map[string][]string) []string {
	var out []string
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		expanded := []string{value}
		if strings.HasPrefix(value, "{") && strings.HasSuffix(value, "}") {
			expanded = groups[strings.TrimSuffix(strings.TrimPrefix(value, "{"), "}")]
		}
		for _, ext := range expanded {
			ext = strings.ToLower(strings.TrimPrefix(ext, "."))
			if ext != "" && !seen[ext] {
				out = append(out, ext)
				seen[ext] = true
			}
		}
	}
	return out
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func WriteOperations(w io.Writer, operations []Operation) error {
	enc := json.NewEncoder(w)
	for _, operation := range operations {
		if err := enc.Encode(operation); err != nil {
			return err
		}
	}
	return nil
}
