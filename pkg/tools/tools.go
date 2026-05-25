package tools

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

type Hooks struct {
	Sanitize   func(string) (string, error)
	CheckRead  func(string) error
	CheckWrite func(string) error
}

type Operation struct {
	Status     string `json:"status"`
	Op         string `json:"op"`
	Src        string `json:"src,omitempty"`
	Dst        string `json:"dst,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Error      string `json:"error,omitempty"`
	SizeBefore int64  `json:"size_before,omitempty"`
	SizeAfter  int64  `json:"size_after,omitempty"`
}

type FileInfo struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime int64  `json:"mod_time"`
	IsDir   bool   `json:"is_dir"`
	MIME    string `json:"mime,omitempty"`
}

func defaultHooks(h Hooks) Hooks {
	if h.Sanitize == nil {
		h.Sanitize = func(p string) (string, error) {
			return filepath.Abs(p)
		}
	}
	if h.CheckRead == nil {
		h.CheckRead = func(string) error { return nil }
	}
	if h.CheckWrite == nil {
		h.CheckWrite = func(string) error { return nil }
	}
	return h
}

func cleanPath(h Hooks, p string, write bool) (string, error) {
	h = defaultHooks(h)
	cp, err := h.Sanitize(p)
	if err != nil {
		return "", err
	}
	if write {
		return cp, h.CheckWrite(cp)
	}
	return cp, h.CheckRead(cp)
}

func boolOpt(opts map[string]any, key string) bool {
	v, ok := opts[key]
	if !ok {
		return false
	}
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x == "true" || x == "1" || x == "yes" || x == "on"
	default:
		return false
	}
}

func stringOpt(opts map[string]any, key, def string) string {
	if v, ok := opts[key]; ok {
		switch x := v.(type) {
		case string:
			if strings.TrimSpace(x) != "" {
				return x
			}
		case fmt.Stringer:
			return x.String()
		}
	}
	return def
}

func intOpt(opts map[string]any, key string, def int) int {
	if v, ok := opts[key]; ok {
		switch x := v.(type) {
		case int:
			return x
		case int64:
			return int(x)
		case float64:
			return int(x)
		case string:
			if n, err := strconv.Atoi(x); err == nil {
				return n
			}
		}
	}
	return def
}

func listOpt(opts map[string]any, key string) []string {
	v, ok := opts[key]
	if !ok {
		return nil
	}
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	case string:
		if strings.TrimSpace(x) == "" {
			return nil
		}
		return []string{x}
	default:
		return nil
	}
}

func statSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func matchFile(name, pattern string) bool {
	if strings.TrimSpace(pattern) == "" {
		pattern = "*"
	}
	ok, err := filepath.Match(pattern, name)
	return err == nil && ok
}

func ExpandTemplate(tpl string, info fs.FileInfo, seq int) string {
	ext := strings.TrimPrefix(filepath.Ext(info.Name()), ".")
	base := strings.TrimSuffix(info.Name(), filepath.Ext(info.Name()))
	out := tpl
	repls := map[string]string{
		"{name}": base,
		"{base}": base,
		"{ext}":  ext,
		"{seq}":  fmt.Sprintf("%03d", seq),
		"{date}": info.ModTime().Format("20060102"),
		"{time}": info.ModTime().Format("150405"),
	}
	for k, v := range repls {
		out = strings.ReplaceAll(out, k, v)
	}
	return filepath.Base(out)
}

func BulkRename(dir string, opts map[string]any, h Hooks) ([]Operation, error) {
	apply := boolOpt(opts, "apply")
	pattern := stringOpt(opts, "match", "*")
	tpl := stringOpt(opts, "template", "{name}_{seq}.{ext}")
	root, err := cleanPath(h, dir, false)
	if err != nil {
		return nil, err
	}
	if apply {
		if err := defaultHooks(h).CheckWrite(root); err != nil {
			return nil, err
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	var ops []Operation
	seq := 1
	for _, entry := range entries {
		if entry.IsDir() || !matchFile(entry.Name(), pattern) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			ops = append(ops, Operation{Status: "failed", Op: "rename", Src: filepath.Join(root, entry.Name()), Error: err.Error()})
			continue
		}
		dstName := ExpandTemplate(tpl, info, seq)
		seq++
		src := filepath.Join(root, entry.Name())
		dst := filepath.Join(root, dstName)
		op := Operation{Status: "planned", Op: "rename", Src: src, Dst: dst, SizeBefore: info.Size()}
		if dstName == "" || dstName == "." || dstName == string(os.PathSeparator) || filepath.Base(dstName) != dstName {
			op.Status, op.Error = "failed", "template produced invalid base filename"
		} else if _, ok := seen[dst]; ok {
			op.Status, op.Error = "failed", "destination collision in plan"
		} else if src != dst {
			if _, err := os.Stat(dst); err == nil {
				op.Status, op.Error = "failed", "destination already exists"
			}
		} else {
			op.Status, op.Reason = "skipped", "source and destination are identical"
		}
		seen[dst] = struct{}{}
		if apply && op.Status == "planned" {
			if err := os.Rename(src, dst); err != nil {
				op.Status, op.Error = "failed", err.Error()
			} else {
				op.Status = "applied"
				op.SizeAfter = statSize(dst)
			}
		}
		ops = append(ops, op)
	}
	return ops, nil
}

func CopyOrMove(src, dst string, move, apply bool, h Hooks) Operation {
	opName := "copy"
	if move {
		opName = "move"
	}
	op := Operation{Status: "planned", Op: opName, Src: src, Dst: dst}
	srcPath, err := cleanPath(h, src, false)
	if err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	dstPath, err := cleanPath(h, dst, true)
	if err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	op.Src, op.Dst, op.SizeBefore = srcPath, dstPath, statSize(srcPath)
	if !apply {
		return op
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	in, err := os.Open(srcPath)
	if err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	defer in.Close()
	out, err := os.Create(dstPath)
	if err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	if _, err = io.Copy(out, in); err != nil {
		out.Close()
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	if err = out.Close(); err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	if move {
		if err := defaultHooks(h).CheckWrite(srcPath); err != nil {
			op.Status, op.Error = "failed", err.Error()
			return op
		}
		if err := os.Remove(srcPath); err != nil {
			op.Status, op.Error = "failed", err.Error()
			return op
		}
	}
	op.Status, op.SizeAfter = "applied", statSize(dstPath)
	return op
}

func Search(root string, opts map[string]any, h Hooks) ([]FileInfo, error) {
	base, err := cleanPath(h, root, false)
	if err != nil {
		return nil, err
	}
	pattern := stringOpt(opts, "match", "*")
	contains := strings.ToLower(stringOpt(opts, "contains", ""))
	recursive := true
	if _, ok := opts["recursive"]; ok {
		recursive = boolOpt(opts, "recursive")
	}
	var out []FileInfo
	err = filepath.WalkDir(base, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path != base && d.IsDir() && !recursive {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !matchFile(name, pattern) {
			return nil
		}
		if contains != "" && !strings.Contains(strings.ToLower(name), contains) {
			return nil
		}
		if err := defaultHooks(h).CheckRead(path); err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out = append(out, FileInfo{Path: path, Name: name, Size: info.Size(), Mode: info.Mode().String(), ModTime: info.ModTime().Unix(), MIME: mime.TypeByExtension(filepath.Ext(name))})
		return nil
	})
	return out, err
}

func Dedupe(root string, opts map[string]any, h Hooks) ([]Operation, error) {
	files, err := Search(root, opts, h)
	if err != nil {
		return nil, err
	}
	seen := map[[32]byte]string{}
	var ops []Operation
	for _, file := range files {
		data, err := os.ReadFile(file.Path)
		if err != nil {
			ops = append(ops, Operation{Status: "failed", Op: "dedupe", Src: file.Path, Error: err.Error()})
			continue
		}
		sum := sha256.Sum256(data)
		if first, ok := seen[sum]; ok {
			ops = append(ops, Operation{Status: "planned", Op: "dedupe", Src: file.Path, Dst: first, Reason: "duplicate content", SizeBefore: file.Size})
		} else {
			seen[sum] = file.Path
		}
	}
	return ops, nil
}

func RemovePath(path string, opts map[string]any, h Hooks) Operation {
	apply := boolOpt(opts, "apply")
	recursive := boolOpt(opts, "recursive")
	op := Operation{Status: "planned", Op: "remove", Src: path}
	safe, err := cleanPath(h, path, true)
	if err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	op.Src, op.SizeBefore = safe, statSize(safe)
	if !apply {
		return op
	}
	if recursive {
		err = os.RemoveAll(safe)
	} else {
		err = os.Remove(safe)
	}
	if err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	op.Status = "applied"
	return op
}

func OrganizeByExt(root, dstRoot string, opts map[string]any, h Hooks) ([]Operation, error) {
	apply := boolOpt(opts, "apply")
	pattern := stringOpt(opts, "match", "*")
	srcRoot, err := cleanPath(h, root, false)
	if err != nil {
		return nil, err
	}
	dstBase, err := cleanPath(h, dstRoot, true)
	if err != nil {
		return nil, err
	}
	var ops []Operation
	err = filepath.WalkDir(srcRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !matchFile(d.Name(), pattern) {
			return nil
		}
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(d.Name())), ".")
		if ext == "" {
			ext = "no-extension"
		}
		dst := filepath.Join(dstBase, ext, d.Name())
		op := CopyOrMove(path, dst, true, apply, h)
		op.Op = "organize"
		ops = append(ops, op)
		return nil
	})
	return ops, err
}

func FileChecksum(path string, h Hooks) (map[string]any, error) {
	safe, err := cleanPath(h, path, false)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(safe)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	return map[string]any{
		"path":   safe,
		"sha256": hex.EncodeToString(sum[:]),
		"size":   int64(len(data)),
	}, nil
}

func Compress(src, dst string, opts map[string]any, h Hooks) Operation {
	format := strings.ToLower(stringOpt(opts, "format", strings.TrimPrefix(filepath.Ext(dst), ".")))
	apply := boolOpt(opts, "apply")
	if strings.TrimSpace(stringOpt(opts, "password", "")) != "" {
		return Operation{Status: "failed", Op: "compress", Src: src, Dst: dst, Error: "password-protected archives are not supported by the Go-native v1 tools"}
	}
	op := Operation{Status: "planned", Op: "compress", Src: src, Dst: dst}
	srcPath, err := cleanPath(h, src, false)
	if err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	dstPath, err := cleanPath(h, dst, true)
	if err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	op.Src, op.Dst, op.SizeBefore = srcPath, dstPath, statSize(srcPath)
	if !apply {
		return op
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	switch format {
	case "zip":
		err = writeZip(srcPath, dstPath)
	case "tar":
		err = writeTar(srcPath, dstPath)
	case "gz", "gzip":
		err = writeGzip(srcPath, dstPath)
	default:
		err = fmt.Errorf("archive format %q is unsupported; supported: zip, tar, gzip", format)
	}
	if err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	op.Status, op.SizeAfter = "applied", statSize(dstPath)
	return op
}

func writeZip(src, dst string) error {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	zw := zip.NewWriter(out)
	defer zw.Close()
	base := filepath.Dir(src)
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		base = filepath.Dir(src)
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		w, err := zw.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(w, in)
		return err
	})
}

func writeTar(src, dst string) error {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	tw := tar.NewWriter(out)
	defer tw.Close()
	base := filepath.Dir(src)
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(tw, in)
		return err
	})
}

func writeGzip(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("gzip supports a single file; use zip or tar for directories")
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	gw := gzip.NewWriter(out)
	defer gw.Close()
	_, err = io.Copy(gw, in)
	return err
}

func ArchiveList(path string, h Hooks) ([]FileInfo, error) {
	src, err := cleanPath(h, path, false)
	if err != nil {
		return nil, err
	}
	switch archiveFormat(src) {
	case "zip":
		zr, err := zip.OpenReader(src)
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		out := make([]FileInfo, 0, len(zr.File))
		for _, f := range zr.File {
			out = append(out, FileInfo{Path: f.Name, Name: filepath.Base(f.Name), Size: int64(f.UncompressedSize64), Mode: f.Mode().String(), ModTime: f.Modified.Unix(), IsDir: f.FileInfo().IsDir()})
		}
		return out, nil
	case "tar":
		in, err := os.Open(src)
		if err != nil {
			return nil, err
		}
		defer in.Close()
		tr := tar.NewReader(in)
		var out []FileInfo
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			out = append(out, FileInfo{Path: hdr.Name, Name: filepath.Base(hdr.Name), Size: hdr.Size, Mode: fs.FileMode(hdr.Mode).String(), ModTime: hdr.ModTime.Unix(), IsDir: hdr.FileInfo().IsDir()})
		}
		return out, nil
	case "gzip":
		info, err := os.Stat(src)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
		return []FileInfo{{Path: name, Name: name, Size: info.Size(), Mode: info.Mode().String(), ModTime: info.ModTime().Unix()}}, nil
	default:
		return nil, fmt.Errorf("archive list supports zip, tar, and gzip")
	}
}

func Extract(src, dst string, opts map[string]any, h Hooks) Operation {
	apply := boolOpt(opts, "apply")
	op := Operation{Status: "planned", Op: "extract", Src: src, Dst: dst}
	srcPath, err := cleanPath(h, src, false)
	if err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	dstPath, err := cleanPath(h, dst, true)
	if err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	op.Src, op.Dst, op.SizeBefore = srcPath, dstPath, statSize(srcPath)
	if !apply {
		return op
	}
	switch archiveFormat(srcPath) {
	case "zip":
		zr, err := zip.OpenReader(srcPath)
		if err != nil {
			op.Status, op.Error = "failed", err.Error()
			return op
		}
		defer zr.Close()
		for _, f := range zr.File {
			target := filepath.Clean(filepath.Join(dstPath, f.Name))
			if target != dstPath && !strings.HasPrefix(target, dstPath+string(os.PathSeparator)) {
				op.Status, op.Error = "failed", "archive contains unsafe path"
				return op
			}
			if f.FileInfo().IsDir() {
				if err := os.MkdirAll(target, f.Mode()); err != nil {
					op.Status, op.Error = "failed", err.Error()
					return op
				}
				continue
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				op.Status, op.Error = "failed", err.Error()
				return op
			}
			rc, err := f.Open()
			if err != nil {
				op.Status, op.Error = "failed", err.Error()
				return op
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, f.Mode())
			if err != nil {
				rc.Close()
				op.Status, op.Error = "failed", err.Error()
				return op
			}
			_, err = io.Copy(out, rc)
			rc.Close()
			out.Close()
			if err != nil {
				op.Status, op.Error = "failed", err.Error()
				return op
			}
		}
		op.Status = "applied"
		return op
	case "tar":
		if err := extractTar(srcPath, dstPath); err != nil {
			op.Status, op.Error = "failed", err.Error()
			return op
		}
		op.Status = "applied"
		return op
	case "gzip":
		if err := extractGzip(srcPath, dstPath); err != nil {
			op.Status, op.Error = "failed", err.Error()
			return op
		}
		op.Status = "applied"
		return op
	default:
		op.Status, op.Error = "failed", "extract supports zip, tar, and gzip"
		return op
	}
}

func archiveFormat(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return "gzip"
	case strings.HasSuffix(lower, ".zip"):
		return "zip"
	case strings.HasSuffix(lower, ".tar"):
		return "tar"
	case strings.HasSuffix(lower, ".gz"), strings.HasSuffix(lower, ".gzip"):
		return "gzip"
	default:
		return strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	}
}

func safeArchiveTarget(root, name string) (string, error) {
	target := filepath.Clean(filepath.Join(root, name))
	if target != root && !strings.HasPrefix(target, root+string(os.PathSeparator)) {
		return "", fmt.Errorf("archive contains unsafe path")
	}
	return target, nil
}

func extractTar(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tr := tar.NewReader(in)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target, err := safeArchiveTarget(dst, hdr.Name)
		if err != nil {
			return err
		}
		if hdr.FileInfo().IsDir() {
			if err := os.MkdirAll(target, hdr.FileInfo().Mode()); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, hdr.FileInfo().Mode())
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, tr)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
}

func extractGzip(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	gr, err := gzip.NewReader(in)
	if err != nil {
		return err
	}
	defer gr.Close()
	target := dst
	if info, err := os.Stat(dst); err == nil && info.IsDir() {
		name := gr.Name
		if strings.TrimSpace(name) == "" {
			name = strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
		}
		target = filepath.Join(dst, filepath.Base(name))
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	out, err := os.Create(target)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, gr)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func ImageConvertBatch(srcDir, dstDir string, opts map[string]any, h Hooks) ([]Operation, error) {
	apply := boolOpt(opts, "apply")
	to := strings.ToLower(stringOpt(opts, "to", "png"))
	from := listOpt(opts, "from")
	if len(from) == 0 {
		from = []string{"jpg", "jpeg", "png", "gif"}
	}
	allowed := map[string]struct{}{}
	for _, f := range from {
		allowed[strings.TrimPrefix(strings.ToLower(f), ".")] = struct{}{}
	}
	srcRoot, err := cleanPath(h, srcDir, false)
	if err != nil {
		return nil, err
	}
	dstRoot, err := cleanPath(h, dstDir, true)
	if err != nil {
		return nil, err
	}
	quality := intOpt(opts, "quality", 90)
	var ops []Operation
	err = filepath.WalkDir(srcRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
		if _, ok := allowed[ext]; !ok {
			return nil
		}
		rel, _ := filepath.Rel(srcRoot, path)
		dst := filepath.Join(dstRoot, strings.TrimSuffix(rel, filepath.Ext(rel))+"."+to)
		op := Operation{Status: "planned", Op: "convert", Src: path, Dst: dst, SizeBefore: statSize(path)}
		if apply {
			if err := ConvertImageFile(path, dst, to, quality); err != nil {
				op.Status, op.Error = "failed", err.Error()
			} else {
				op.Status, op.SizeAfter = "applied", statSize(dst)
			}
		}
		ops = append(ops, op)
		return nil
	})
	return ops, err
}

func ConvertImageFile(src, dst, format string, quality int) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	img, _, err := image.Decode(in)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	switch strings.ToLower(format) {
	case "jpg", "jpeg":
		return jpeg.Encode(out, img, &jpeg.Options{Quality: quality})
	case "png":
		return png.Encode(out, img)
	case "gif":
		return gif.Encode(out, img, nil)
	case "webp":
		return fmt.Errorf("webp encode is unsupported by the Go-native v1 tools")
	default:
		return fmt.Errorf("unsupported image format %q", format)
	}
}

func ImageInfo(path string, h Hooks) (map[string]any, error) {
	safe, err := cleanPath(h, path, false)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(safe)
	if err != nil {
		return nil, err
	}
	in, err := os.Open(safe)
	if err != nil {
		return nil, err
	}
	defer in.Close()
	cfg, format, err := image.DecodeConfig(in)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"path":     safe,
		"name":     info.Name(),
		"size":     info.Size(),
		"format":   format,
		"mime":     mime.TypeByExtension(filepath.Ext(safe)),
		"width":    cfg.Width,
		"height":   cfg.Height,
		"mod_time": info.ModTime().Unix(),
	}, nil
}

func ResizeImage(src, dst string, opts map[string]any, h Hooks) Operation {
	apply := boolOpt(opts, "apply")
	op := Operation{Status: "planned", Op: "resize", Src: src, Dst: dst}
	srcPath, err := cleanPath(h, src, false)
	if err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	dstPath, err := cleanPath(h, dst, true)
	if err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	op.Src, op.Dst, op.SizeBefore = srcPath, dstPath, statSize(srcPath)
	if !apply {
		return op
	}
	in, err := os.Open(srcPath)
	if err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	defer in.Close()
	img, _, err := image.Decode(in)
	if err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	width, height := intOpt(opts, "width", 0), intOpt(opts, "height", 0)
	if width <= 0 && height <= 0 {
		op.Status, op.Error = "failed", "width or height must be positive"
		return op
	}
	bounds := img.Bounds()
	if width <= 0 {
		width = bounds.Dx() * height / bounds.Dy()
	}
	if height <= 0 {
		height = bounds.Dy() * width / bounds.Dx()
	}
	dstImg := image.NewRGBA(image.Rect(0, 0, width, height))
	xdraw.CatmullRom.Scale(dstImg, dstImg.Bounds(), img, bounds, xdraw.Over, nil)
	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(dstPath)), ".")
	if format == "" {
		format = strings.ToLower(stringOpt(opts, "format", "png"))
	}
	if err := writeImage(dstPath, dstImg, format, intOpt(opts, "quality", 90)); err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	op.Status, op.SizeAfter = "applied", statSize(dstPath)
	return op
}

func Thumbnail(src, dst string, opts map[string]any, h Hooks) Operation {
	size := intOpt(opts, "size", 256)
	if _, ok := opts["width"]; !ok {
		opts["width"] = size
	}
	if _, ok := opts["height"]; !ok {
		opts["height"] = size
	}
	op := ResizeImage(src, dst, opts, h)
	op.Op = "thumbnail"
	return op
}

func writeImage(dst string, img image.Image, format string, quality int) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	switch strings.ToLower(format) {
	case "jpg", "jpeg":
		return jpeg.Encode(out, img, &jpeg.Options{Quality: quality})
	case "png":
		return png.Encode(out, img)
	case "gif":
		return gif.Encode(out, img, nil)
	case "webp":
		return fmt.Errorf("webp encode is unsupported by the Go-native v1 tools")
	default:
		return fmt.Errorf("unsupported image format %q", format)
	}
}

func CropImage(src, dst string, opts map[string]any, h Hooks) Operation {
	apply := boolOpt(opts, "apply")
	op := Operation{Status: "planned", Op: "crop", Src: src, Dst: dst}
	srcPath, err := cleanPath(h, src, false)
	if err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	dstPath, err := cleanPath(h, dst, true)
	if err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	op.Src, op.Dst, op.SizeBefore = srcPath, dstPath, statSize(srcPath)
	if !apply {
		return op
	}
	in, err := os.Open(srcPath)
	if err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	defer in.Close()
	img, _, err := image.Decode(in)
	if err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	x, y := intOpt(opts, "x", 0), intOpt(opts, "y", 0)
	w, ht := intOpt(opts, "width", 0), intOpt(opts, "height", 0)
	if w <= 0 || ht <= 0 {
		op.Status, op.Error = "failed", "width and height must be positive"
		return op
	}
	rect := image.Rect(x, y, x+w, y+ht).Intersect(img.Bounds())
	if rect.Empty() {
		op.Status, op.Error = "failed", "crop rectangle is outside image bounds"
		return op
	}
	crop, ok := img.(interface {
		SubImage(r image.Rectangle) image.Image
	})
	if !ok {
		op.Status, op.Error = "failed", "image type does not support cropping"
		return op
	}
	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(dstPath)), ".")
	if format == "" {
		format = "png"
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	out, err := os.Create(dstPath)
	if err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	defer out.Close()
	switch format {
	case "jpg", "jpeg":
		err = jpeg.Encode(out, crop.SubImage(rect), &jpeg.Options{Quality: intOpt(opts, "quality", 90)})
	case "gif":
		err = gif.Encode(out, crop.SubImage(rect), nil)
	default:
		err = png.Encode(out, crop.SubImage(rect))
	}
	if err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	op.Status, op.SizeAfter = "applied", statSize(dstPath)
	return op
}

func GenerateSecret(length int, alphabet string) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("length must be positive")
	}
	if alphabet == "" {
		alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()-_=+[]{}<>?"
	}
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, length)
	for i, b := range buf {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(out), nil
}

func EncryptFile(src, dst, passphrase string, apply bool, h Hooks) Operation {
	return cryptFile("encrypt", src, dst, passphrase, apply, h)
}

func DecryptFile(src, dst, passphrase string, apply bool, h Hooks) Operation {
	return cryptFile("decrypt", src, dst, passphrase, apply, h)
}

func cryptFile(opName, src, dst, passphrase string, apply bool, h Hooks) Operation {
	op := Operation{Status: "planned", Op: opName, Src: src, Dst: dst}
	srcPath, err := cleanPath(h, src, false)
	if err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	dstPath, err := cleanPath(h, dst, true)
	if err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	op.Src, op.Dst, op.SizeBefore = srcPath, dstPath, statSize(srcPath)
	if passphrase == "" {
		op.Status, op.Error = "failed", "passphrase is required"
		return op
	}
	if !apply {
		return op
	}
	data, err := os.ReadFile(srcPath)
	if err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	key := sha256.Sum256([]byte(passphrase))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	var out []byte
	if opName == "encrypt" {
		nonce := make([]byte, gcm.NonceSize())
		if _, err := rand.Read(nonce); err != nil {
			op.Status, op.Error = "failed", err.Error()
			return op
		}
		out = append(nonce, gcm.Seal(nil, nonce, data, nil)...)
	} else {
		if len(data) < gcm.NonceSize() {
			op.Status, op.Error = "failed", "ciphertext is too short"
			return op
		}
		nonce := data[:gcm.NonceSize()]
		out, err = gcm.Open(nil, nonce, data[gcm.NonceSize():], nil)
		if err != nil {
			op.Status, op.Error = "failed", err.Error()
			return op
		}
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	if err := os.WriteFile(dstPath, out, 0o600); err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	op.Status, op.SizeAfter = "applied", int64(len(out))
	return op
}

func MediaInfo(path string, h Hooks) (map[string]any, error) {
	return MediaInfoWithFFprobe(path, h, true)
}

func MediaInfoWithFFprobe(path string, h Hooks, useFFprobe bool) (map[string]any, error) {
	if useFFprobe {
		if ffprobe, ok := findBinary("ffprobe"); ok {
			src, err := cleanPath(h, path, false)
			if err != nil {
				return nil, err
			}
			ctx := exec.Command(ffprobe, "-v", "error", "-show_format", "-show_streams", "-of", "json", src)
			data, err := ctx.Output()
			if err == nil {
				return map[string]any{"path": src, "probe": string(data), "provider": "ffprobe"}, nil
			}
		}
	}
	src, err := cleanPath(h, path, false)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(src)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"path": src, "name": info.Name(), "size": info.Size(), "ext": strings.TrimPrefix(strings.ToLower(filepath.Ext(src)), "."), "mod_time": info.ModTime().Unix()}
	if in, err := os.Open(src); err == nil {
		defer in.Close()
		if cfg, format, err := image.DecodeConfig(in); err == nil {
			out["kind"], out["format"], out["width"], out["height"] = "image", format, cfg.Width, cfg.Height
			return out, nil
		}
	}
	out["kind"] = "binary"
	out["note"] = "audio/video codec metadata is limited to Go-native probing in v1"
	return out, nil
}

func MediaConvert(src, dst string, opts map[string]any, h Hooks) Operation {
	apply := boolOpt(opts, "apply")
	op := Operation{Status: "planned", Op: "media_convert", Src: src, Dst: dst}
	srcPath, err := cleanPath(h, src, false)
	if err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	dstPath, err := cleanPath(h, dst, true)
	if err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	op.Src, op.Dst, op.SizeBefore = srcPath, dstPath, statSize(srcPath)
	ffmpeg, ok := findBinary("ffmpeg")
	if !ok {
		if boolOpt(opts, "install") && !apply {
			install := InstallFFmpeg(false)
			op.Reason = "ffmpeg is missing; install would run: " + install.Dst
			return op
		}
		if boolOpt(opts, "install") && apply {
			install := InstallFFmpeg(true)
			if install.Status != "applied" && install.Status != "skipped" {
				op.Status, op.Error = "failed", install.Error
				if op.Error == "" {
					op.Error = install.Reason
				}
				return op
			}
			ffmpeg, ok = findBinary("ffmpeg")
		}
	}
	if !ok {
		op.Status, op.Error = "failed", "ffmpeg was not found; pass install=true, run spltool media install-ffmpeg, or install ffmpeg for your OS"
		return op
	}
	if !apply {
		return op
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		op.Status, op.Error = "failed", err.Error()
		return op
	}
	args := []string{"-y", "-i", srcPath}
	if codec := stringOpt(opts, "codec", ""); codec != "" {
		args = append(args, "-c", codec)
	}
	if crf := stringOpt(opts, "crf", ""); crf != "" {
		args = append(args, "-crf", crf)
	}
	if bitrate := stringOpt(opts, "bitrate", ""); bitrate != "" {
		args = append(args, "-b:a", bitrate)
	}
	args = append(args, dstPath)
	cmd := exec.Command(ffmpeg, args...)
	data, err := cmd.CombinedOutput()
	if err != nil {
		op.Status, op.Error = "failed", strings.TrimSpace(string(data))
		if op.Error == "" {
			op.Error = err.Error()
		}
		return op
	}
	op.Status, op.SizeAfter = "applied", statSize(dstPath)
	return op
}

func OfficeText(path string, h Hooks) (string, error) {
	safe, err := cleanPath(h, path, false)
	if err != nil {
		return "", err
	}
	switch strings.ToLower(filepath.Ext(safe)) {
	case ".txt", ".md", ".log":
		data, err := os.ReadFile(safe)
		return string(data), err
	case ".csv":
		rows, err := readCSVRows(safe)
		if err != nil {
			return "", err
		}
		var b strings.Builder
		for _, row := range rows {
			b.WriteString(strings.Join(row, "\t"))
			b.WriteByte('\n')
		}
		return b.String(), nil
	case ".json":
		data, err := os.ReadFile(safe)
		if err != nil {
			return "", err
		}
		var v any
		if err := json.Unmarshal(data, &v); err == nil {
			pretty, _ := json.MarshalIndent(v, "", "  ")
			return string(pretty), nil
		}
		return string(data), nil
	case ".docx":
		return readDocxText(safe)
	case ".xlsx":
		rows, err := readXLSXRows(safe)
		if err != nil {
			return "", err
		}
		var b strings.Builder
		for _, row := range rows {
			b.WriteString(strings.Join(row, "\t"))
			b.WriteByte('\n')
		}
		return b.String(), nil
	default:
		return "", fmt.Errorf("office_text supports txt, md, log, csv, json, docx, and xlsx")
	}
}

func OfficeRead(path string, h Hooks) (map[string]any, error) {
	safe, err := cleanPath(h, path, false)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(safe)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"path": safe, "name": info.Name(), "size": info.Size(), "ext": strings.TrimPrefix(strings.ToLower(filepath.Ext(safe)), ".")}
	switch strings.ToLower(filepath.Ext(safe)) {
	case ".csv":
		rows, err := readCSVRows(safe)
		if err != nil {
			return nil, err
		}
		out["rows"] = rows
	case ".json":
		data, err := os.ReadFile(safe)
		if err != nil {
			return nil, err
		}
		var v any
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		out["value"] = v
	case ".xlsx":
		rows, err := readXLSXRows(safe)
		if err != nil {
			return nil, err
		}
		out["rows"] = rows
	default:
		text, err := OfficeText(safe, h)
		if err != nil {
			return nil, err
		}
		out["text"] = text
	}
	return out, nil
}

func readCSVRows(path string) ([][]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	return r.ReadAll()
}

func readDocxText(path string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		defer rc.Close()
		var b strings.Builder
		dec := xml.NewDecoder(rc)
		for {
			tok, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				return "", err
			}
			if ch, ok := tok.(xml.CharData); ok {
				txt := strings.TrimSpace(string(ch))
				if txt != "" {
					if b.Len() > 0 {
						b.WriteByte(' ')
					}
					b.WriteString(txt)
				}
			}
		}
		return b.String(), nil
	}
	return "", fmt.Errorf("docx document.xml not found")
}

func readXLSXRows(path string) ([][]string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
	}
	shared, _ := readXLSXSharedStrings(files["xl/sharedStrings.xml"])
	sheet := files["xl/worksheets/sheet1.xml"]
	if sheet == nil {
		for name, f := range files {
			if strings.HasPrefix(name, "xl/worksheets/") && strings.HasSuffix(name, ".xml") {
				sheet = f
				break
			}
		}
	}
	if sheet == nil {
		return nil, fmt.Errorf("xlsx worksheet not found")
	}
	rc, err := sheet.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	type cell struct {
		Ref    string `xml:"r,attr"`
		Type   string `xml:"t,attr"`
		Value  string `xml:"v"`
		Inline string `xml:"is>t"`
	}
	type row struct {
		Cells []cell `xml:"c"`
	}
	type worksheet struct {
		Rows []row `xml:"sheetData>row"`
	}
	var ws worksheet
	if err := xml.NewDecoder(rc).Decode(&ws); err != nil {
		return nil, err
	}
	rows := make([][]string, 0, len(ws.Rows))
	for _, r := range ws.Rows {
		out := make([]string, 0, len(r.Cells))
		for _, c := range r.Cells {
			val := c.Value
			if c.Type == "s" {
				if idx, err := strconv.Atoi(c.Value); err == nil && idx >= 0 && idx < len(shared) {
					val = shared[idx]
				}
			} else if c.Type == "inlineStr" {
				val = c.Inline
			}
			out = append(out, val)
		}
		rows = append(rows, out)
	}
	return rows, nil
}

func readXLSXSharedStrings(f *zip.File) ([]string, error) {
	if f == nil {
		return nil, nil
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	type item struct {
		Texts []string `xml:"t"`
	}
	type sst struct {
		Items []item `xml:"si"`
	}
	var stringsXML sst
	if err := xml.NewDecoder(rc).Decode(&stringsXML); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(stringsXML.Items))
	for _, item := range stringsXML.Items {
		out = append(out, strings.Join(item.Texts, ""))
	}
	return out, nil
}

func FFmpegStatus() map[string]any {
	out := map[string]any{"ffmpeg": false, "ffprobe": false}
	if path, ok := findBinary("ffmpeg"); ok {
		out["ffmpeg"] = true
		out["ffmpeg_path"] = path
	}
	if path, ok := findBinary("ffprobe"); ok {
		out["ffprobe"] = true
		out["ffprobe_path"] = path
	}
	out["install_command"] = ffmpegInstallCommand()
	return out
}

func InstallFFmpeg(apply bool) Operation {
	op := Operation{Status: "planned", Op: "install_ffmpeg", Reason: "install ffmpeg and ffprobe with the detected OS package manager"}
	if _, ok := findBinary("ffmpeg"); ok {
		op.Status, op.Reason = "skipped", "ffmpeg is already installed"
		return op
	}
	cmdline := ffmpegInstallCommand()
	if len(cmdline) == 0 {
		op.Status, op.Error = "failed", "no supported package manager was found for automatic ffmpeg installation"
		return op
	}
	op.Dst = strings.Join(cmdline, " ")
	if !apply {
		return op
	}
	cmd := exec.Command(cmdline[0], cmdline[1:]...)
	data, err := cmd.CombinedOutput()
	if err != nil {
		op.Status, op.Error = "failed", strings.TrimSpace(string(data))
		if op.Error == "" {
			op.Error = err.Error()
		}
		return op
	}
	op.Status = "applied"
	return op
}

func findBinary(name string) (string, bool) {
	path, err := exec.LookPath(name)
	return path, err == nil
}

func ffmpegInstallCommand() []string {
	switch runtime.GOOS {
	case "darwin":
		if _, ok := findBinary("brew"); ok {
			return []string{"brew", "install", "ffmpeg"}
		}
	case "windows":
		if _, ok := findBinary("winget"); ok {
			return []string{"winget", "install", "--id", "Gyan.FFmpeg", "-e", "--accept-package-agreements", "--accept-source-agreements"}
		}
	default:
		candidates := [][]string{
			{"apt-get", "install", "-y", "ffmpeg"},
			{"dnf", "install", "-y", "ffmpeg"},
			{"yum", "install", "-y", "ffmpeg"},
			{"pacman", "-S", "--noconfirm", "ffmpeg"},
			{"apk", "add", "ffmpeg"},
		}
		for _, c := range candidates {
			if _, ok := findBinary(c[0]); ok {
				return c
			}
		}
	}
	return nil
}

func SystemInfo() map[string]any {
	host, _ := os.Hostname()
	wd, _ := os.Getwd()
	return map[string]any{
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"cpus":       runtime.NumCPU(),
		"go_version": runtime.Version(),
		"hostname":   host,
		"cwd":        wd,
	}
}

func DNSLookup(host string) ([]string, error) {
	return net.LookupHost(host)
}

func TCPCheck(address string, timeout time.Duration) map[string]any {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return map[string]any{"ok": false, "address": address, "error": err.Error(), "duration_ms": time.Since(start).Milliseconds()}
	}
	conn.Close()
	return map[string]any{"ok": true, "address": address, "duration_ms": time.Since(start).Milliseconds()}
}

func HTTPProbe(target string, timeout time.Duration) map[string]any {
	start := time.Now()
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest(http.MethodHead, target, nil)
	if err != nil {
		return map[string]any{"ok": false, "url": target, "error": err.Error()}
	}
	resp, err := client.Do(req)
	if err != nil {
		return map[string]any{"ok": false, "url": target, "error": err.Error(), "duration_ms": time.Since(start).Milliseconds()}
	}
	defer resp.Body.Close()
	return map[string]any{"ok": true, "url": target, "status": resp.StatusCode, "duration_ms": time.Since(start).Milliseconds()}
}

func EncodeToken(bytes int) (string, error) {
	if bytes <= 0 {
		bytes = 32
	}
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func SortOperations(ops []Operation) {
	sort.SliceStable(ops, func(i, j int) bool {
		if ops[i].Src == ops[j].Src {
			return ops[i].Dst < ops[j].Dst
		}
		return ops[i].Src < ops[j].Src
	})
}
