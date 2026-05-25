package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oarkflow/interpreter"
	"github.com/oarkflow/interpreter/pkg/tools"
)

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	switch args[0] {
	case "fmt":
		return runFmt(args[1:], stdin, stdout, stderr)
	case "check":
		return runCheck(args[1:], stdin, stdout, stderr)
	case "mod":
		return runMod(args[1:], stdout, stderr)
	case "config":
		return runConfig(args[1:], stdout, stderr)
	case "files":
		return runFiles(args[1:], stdout, stderr)
	case "archive":
		return runArchive(args[1:], stdout, stderr)
	case "image":
		return runImage(args[1:], stdout, stderr)
	case "secrets":
		return runSecrets(args[1:], stdout, stderr)
	case "media":
		return runMedia(args[1:], stdout, stderr)
	case "office":
		return runOffice(args[1:], stdout, stderr)
	case "symbols":
		return runSymbols(args[1:], stdin, stdout, stderr)
	case "complete":
		return runComplete(args[1:], stdin, stdout, stderr)
	case "hover":
		return runHover(args[1:], stdin, stdout, stderr)
	case "docs":
		return runDocs(args[1:], stdin, stdout, stderr)
	case "test":
		return runTest(args[1:], stdout, stderr)
	case "session":
		return runSession(args[1:], stdin, stdout, stderr)
	case "conformance":
		return runConformance(args[1:], stdout, stderr)
	case "lsp":
		return runLSP(args[1:], stdin, stdout, stderr)
	case "-h", "--help", "help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func runConfig(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "show" {
		cfg, path, err := LoadProjectConfig("")
		if err != nil {
			fmt.Fprintf(stderr, "config error: %v\n", err)
			return 1
		}
		if path == "" {
			cfg = DefaultProjectConfig()
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(map[string]any{"path": path, "config": cfg}); err != nil {
			fmt.Fprintf(stderr, "failed to encode JSON: %v\n", err)
			return 1
		}
		return 0
	}
	switch args[0] {
	case "init":
		path := "spl.config.json"
		if len(args) > 1 {
			path = args[1]
		}
		if _, err := os.Stat(path); err == nil {
			fmt.Fprintf(stderr, "%s already exists\n", path)
			return 1
		}
		cfg := DefaultProjectConfig()
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "failed to encode config: %v\n", err)
			return 1
		}
		data = append(data, '\n')
		if err := os.WriteFile(path, data, 0o644); err != nil {
			fmt.Fprintf(stderr, "failed to write %s: %v\n", path, err)
			return 1
		}
		fmt.Fprintf(stdout, "created %s\n", path)
		return 0
	default:
		fmt.Fprintln(stderr, "usage: spltool config <init|show> [path]")
		return 2
	}
}

func runFiles(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: spltool files <rename|move|copy|remove|organize|search|dedupe|checksum>")
		return 2
	}
	switch args[0] {
	case "rename":
		fs := flag.NewFlagSet("files rename", flag.ContinueOnError)
		fs.SetOutput(stderr)
		match := fs.String("match", "*", "glob pattern")
		template := fs.String("template", "{name}_{seq}.{ext}", "rename template")
		apply := fs.Bool("apply", false, "apply changes")
		jsonOut := fs.Bool("json", false, "emit JSON")
		if err := fs.Parse(normalizeToolArgs(args[1:], "match", "template")); err != nil {
			return 2
		}
		if len(fs.Args()) != 1 {
			fmt.Fprintln(stderr, "usage: spltool files rename <dir> --match '*.jpg' --template '{date}_{seq}.{ext}' [--apply]")
			return 2
		}
		ops, err := tools.BulkRename(fs.Args()[0], map[string]any{"match": *match, "template": *template, "apply": *apply}, tools.Hooks{})
		if err != nil {
			fmt.Fprintf(stderr, "rename failed: %v\n", err)
			return 1
		}
		return printOperations(stdout, stderr, ops, *jsonOut)
	case "move", "copy":
		fs := flag.NewFlagSet("files "+args[0], flag.ContinueOnError)
		fs.SetOutput(stderr)
		apply := fs.Bool("apply", false, "apply changes")
		jsonOut := fs.Bool("json", false, "emit JSON")
		if err := fs.Parse(normalizeToolArgs(args[1:])); err != nil {
			return 2
		}
		if len(fs.Args()) != 2 {
			fmt.Fprintf(stderr, "usage: spltool files %s <src> <dst> [--apply]\n", args[0])
			return 2
		}
		op := tools.CopyOrMove(fs.Args()[0], fs.Args()[1], args[0] == "move", *apply, tools.Hooks{})
		return printOperations(stdout, stderr, []tools.Operation{op}, *jsonOut)
	case "search":
		fs := flag.NewFlagSet("files search", flag.ContinueOnError)
		fs.SetOutput(stderr)
		match := fs.String("match", "*", "glob pattern")
		contains := fs.String("contains", "", "filename substring")
		jsonOut := fs.Bool("json", false, "emit JSON")
		if err := fs.Parse(normalizeToolArgs(args[1:], "match", "contains")); err != nil {
			return 2
		}
		if len(fs.Args()) != 1 {
			fmt.Fprintln(stderr, "usage: spltool files search <root> [--match '*.spl'] [--contains name]")
			return 2
		}
		files, err := tools.Search(fs.Args()[0], map[string]any{"match": *match, "contains": *contains}, tools.Hooks{})
		if err != nil {
			fmt.Fprintf(stderr, "search failed: %v\n", err)
			return 1
		}
		if *jsonOut {
			return encodeJSON(stdout, stderr, files)
		}
		for _, file := range files {
			fmt.Fprintf(stdout, "%s\t%d\n", file.Path, file.Size)
		}
		return 0
	case "dedupe":
		fs := flag.NewFlagSet("files dedupe", flag.ContinueOnError)
		fs.SetOutput(stderr)
		match := fs.String("match", "*", "glob pattern")
		jsonOut := fs.Bool("json", false, "emit JSON")
		if err := fs.Parse(normalizeToolArgs(args[1:], "match")); err != nil {
			return 2
		}
		if len(fs.Args()) != 1 {
			fmt.Fprintln(stderr, "usage: spltool files dedupe <root> [--match '*']")
			return 2
		}
		ops, err := tools.Dedupe(fs.Args()[0], map[string]any{"match": *match}, tools.Hooks{})
		if err != nil {
			fmt.Fprintf(stderr, "dedupe failed: %v\n", err)
			return 1
		}
		return printOperations(stdout, stderr, ops, *jsonOut)
	case "remove":
		fs := flag.NewFlagSet("files remove", flag.ContinueOnError)
		fs.SetOutput(stderr)
		recursive := fs.Bool("recursive", false, "remove directories recursively")
		apply := fs.Bool("apply", false, "apply changes")
		jsonOut := fs.Bool("json", false, "emit JSON")
		if err := fs.Parse(normalizeToolArgs(args[1:])); err != nil {
			return 2
		}
		if len(fs.Args()) != 1 {
			fmt.Fprintln(stderr, "usage: spltool files remove <path> [--recursive] [--apply]")
			return 2
		}
		op := tools.RemovePath(fs.Args()[0], map[string]any{"recursive": *recursive, "apply": *apply}, tools.Hooks{})
		return printOperations(stdout, stderr, []tools.Operation{op}, *jsonOut)
	case "organize":
		fs := flag.NewFlagSet("files organize", flag.ContinueOnError)
		fs.SetOutput(stderr)
		match := fs.String("match", "*", "glob pattern")
		apply := fs.Bool("apply", false, "apply changes")
		jsonOut := fs.Bool("json", false, "emit JSON")
		if err := fs.Parse(normalizeToolArgs(args[1:], "match")); err != nil {
			return 2
		}
		if len(fs.Args()) != 2 {
			fmt.Fprintln(stderr, "usage: spltool files organize <src-dir> <dst-dir> [--match '*'] [--apply]")
			return 2
		}
		ops, err := tools.OrganizeByExt(fs.Args()[0], fs.Args()[1], map[string]any{"match": *match, "apply": *apply}, tools.Hooks{})
		if err != nil {
			fmt.Fprintf(stderr, "organize failed: %v\n", err)
			return 1
		}
		return printOperations(stdout, stderr, ops, *jsonOut)
	case "checksum":
		fs := flag.NewFlagSet("files checksum", flag.ContinueOnError)
		fs.SetOutput(stderr)
		jsonOut := fs.Bool("json", false, "emit JSON")
		if err := fs.Parse(normalizeToolArgs(args[1:])); err != nil {
			return 2
		}
		if len(fs.Args()) != 1 {
			fmt.Fprintln(stderr, "usage: spltool files checksum <path>")
			return 2
		}
		sum, err := tools.FileChecksum(fs.Args()[0], tools.Hooks{})
		if err != nil {
			fmt.Fprintf(stderr, "checksum failed: %v\n", err)
			return 1
		}
		if *jsonOut {
			return encodeJSON(stdout, stderr, sum)
		}
		fmt.Fprintf(stdout, "%s  %s\n", sum["sha256"], sum["path"])
		return 0
	default:
		fmt.Fprintf(stderr, "unknown files command %q\n", args[0])
		return 2
	}
}

func runArchive(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: spltool archive <compress|extract|list>")
		return 2
	}
	switch args[0] {
	case "compress":
		fs := flag.NewFlagSet("archive compress", flag.ContinueOnError)
		fs.SetOutput(stderr)
		format := fs.String("format", "", "zip, tar, or gzip")
		apply := fs.Bool("apply", false, "apply changes")
		jsonOut := fs.Bool("json", false, "emit JSON")
		if err := fs.Parse(normalizeToolArgs(args[1:], "format")); err != nil {
			return 2
		}
		if len(fs.Args()) != 2 {
			fmt.Fprintln(stderr, "usage: spltool archive compress <src> <dst> [--format zip] [--apply]")
			return 2
		}
		op := tools.Compress(fs.Args()[0], fs.Args()[1], map[string]any{"format": *format, "apply": *apply}, tools.Hooks{})
		return printOperations(stdout, stderr, []tools.Operation{op}, *jsonOut)
	case "extract":
		fs := flag.NewFlagSet("archive extract", flag.ContinueOnError)
		fs.SetOutput(stderr)
		apply := fs.Bool("apply", false, "apply changes")
		jsonOut := fs.Bool("json", false, "emit JSON")
		if err := fs.Parse(normalizeToolArgs(args[1:])); err != nil {
			return 2
		}
		if len(fs.Args()) != 2 {
			fmt.Fprintln(stderr, "usage: spltool archive extract <src.zip> <dst> [--apply]")
			return 2
		}
		op := tools.Extract(fs.Args()[0], fs.Args()[1], map[string]any{"apply": *apply}, tools.Hooks{})
		return printOperations(stdout, stderr, []tools.Operation{op}, *jsonOut)
	case "list":
		fs := flag.NewFlagSet("archive list", flag.ContinueOnError)
		fs.SetOutput(stderr)
		jsonOut := fs.Bool("json", false, "emit JSON")
		if err := fs.Parse(normalizeToolArgs(args[1:])); err != nil {
			return 2
		}
		if len(fs.Args()) != 1 {
			fmt.Fprintln(stderr, "usage: spltool archive list <archive.zip>")
			return 2
		}
		files, err := tools.ArchiveList(fs.Args()[0], tools.Hooks{})
		if err != nil {
			fmt.Fprintf(stderr, "list failed: %v\n", err)
			return 1
		}
		if *jsonOut {
			return encodeJSON(stdout, stderr, files)
		}
		for _, f := range files {
			fmt.Fprintf(stdout, "%s\t%d\n", f.Path, f.Size)
		}
		return 0
	default:
		fmt.Fprintf(stderr, "unknown archive command %q\n", args[0])
		return 2
	}
}

func runImage(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: spltool image <convert|optimize|crop|resize|thumbnail|info>")
		return 2
	}
	switch args[0] {
	case "convert", "optimize":
		fs := flag.NewFlagSet("image "+args[0], flag.ContinueOnError)
		fs.SetOutput(stderr)
		to := fs.String("to", "png", "target format")
		quality := fs.Int("quality", 90, "JPEG quality")
		apply := fs.Bool("apply", false, "apply changes")
		jsonOut := fs.Bool("json", false, "emit JSON")
		if err := fs.Parse(normalizeToolArgs(args[1:], "to", "quality")); err != nil {
			return 2
		}
		if len(fs.Args()) != 2 {
			fmt.Fprintf(stderr, "usage: spltool image %s <src-dir> <dst-dir> --to webp [--apply]\n", args[0])
			return 2
		}
		ops, err := tools.ImageConvertBatch(fs.Args()[0], fs.Args()[1], map[string]any{"to": *to, "quality": *quality, "apply": *apply}, tools.Hooks{})
		if err != nil {
			fmt.Fprintf(stderr, "image %s failed: %v\n", args[0], err)
			return 1
		}
		return printOperations(stdout, stderr, ops, *jsonOut)
	case "crop":
		fs := flag.NewFlagSet("image crop", flag.ContinueOnError)
		fs.SetOutput(stderr)
		x := fs.Int("x", 0, "left")
		y := fs.Int("y", 0, "top")
		width := fs.Int("width", 0, "width")
		height := fs.Int("height", 0, "height")
		apply := fs.Bool("apply", false, "apply changes")
		jsonOut := fs.Bool("json", false, "emit JSON")
		if err := fs.Parse(normalizeToolArgs(args[1:], "x", "y", "width", "height")); err != nil {
			return 2
		}
		if len(fs.Args()) != 2 {
			fmt.Fprintln(stderr, "usage: spltool image crop <src> <dst> --x 0 --y 0 --width 100 --height 100 [--apply]")
			return 2
		}
		op := tools.CropImage(fs.Args()[0], fs.Args()[1], map[string]any{"x": *x, "y": *y, "width": *width, "height": *height, "apply": *apply}, tools.Hooks{})
		return printOperations(stdout, stderr, []tools.Operation{op}, *jsonOut)
	case "resize":
		fs := flag.NewFlagSet("image resize", flag.ContinueOnError)
		fs.SetOutput(stderr)
		width := fs.Int("width", 0, "width")
		height := fs.Int("height", 0, "height")
		quality := fs.Int("quality", 90, "JPEG quality")
		apply := fs.Bool("apply", false, "apply changes")
		jsonOut := fs.Bool("json", false, "emit JSON")
		if err := fs.Parse(normalizeToolArgs(args[1:], "width", "height", "quality")); err != nil {
			return 2
		}
		if len(fs.Args()) != 2 {
			fmt.Fprintln(stderr, "usage: spltool image resize <src> <dst> --width 800 [--height 600] [--apply]")
			return 2
		}
		op := tools.ResizeImage(fs.Args()[0], fs.Args()[1], map[string]any{"width": *width, "height": *height, "quality": *quality, "apply": *apply}, tools.Hooks{})
		return printOperations(stdout, stderr, []tools.Operation{op}, *jsonOut)
	case "thumbnail":
		fs := flag.NewFlagSet("image thumbnail", flag.ContinueOnError)
		fs.SetOutput(stderr)
		size := fs.Int("size", 256, "thumbnail size")
		apply := fs.Bool("apply", false, "apply changes")
		jsonOut := fs.Bool("json", false, "emit JSON")
		if err := fs.Parse(normalizeToolArgs(args[1:], "size")); err != nil {
			return 2
		}
		if len(fs.Args()) != 2 {
			fmt.Fprintln(stderr, "usage: spltool image thumbnail <src> <dst> [--size 256] [--apply]")
			return 2
		}
		op := tools.Thumbnail(fs.Args()[0], fs.Args()[1], map[string]any{"size": *size, "apply": *apply}, tools.Hooks{})
		return printOperations(stdout, stderr, []tools.Operation{op}, *jsonOut)
	case "info":
		fs := flag.NewFlagSet("image info", flag.ContinueOnError)
		fs.SetOutput(stderr)
		jsonOut := fs.Bool("json", false, "emit JSON")
		if err := fs.Parse(normalizeToolArgs(args[1:])); err != nil {
			return 2
		}
		if len(fs.Args()) != 1 {
			fmt.Fprintln(stderr, "usage: spltool image info <path>")
			return 2
		}
		info, err := tools.ImageInfo(fs.Args()[0], tools.Hooks{})
		if err != nil {
			fmt.Fprintf(stderr, "image info failed: %v\n", err)
			return 1
		}
		if *jsonOut {
			return encodeJSON(stdout, stderr, info)
		}
		for k, v := range info {
			fmt.Fprintf(stdout, "%s: %v\n", k, v)
		}
		return 0
	default:
		fmt.Fprintf(stderr, "unknown image command %q\n", args[0])
		return 2
	}
}

func runSecrets(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: spltool secrets <generate|encrypt|decrypt>")
		return 2
	}
	switch args[0] {
	case "generate":
		fs := flag.NewFlagSet("secrets generate", flag.ContinueOnError)
		fs.SetOutput(stderr)
		length := fs.Int("length", 24, "secret length")
		token := fs.Bool("token", false, "generate URL-safe token")
		if err := fs.Parse(normalizeToolArgs(args[1:], "length")); err != nil {
			return 2
		}
		var value string
		var err error
		if *token {
			value, err = tools.EncodeToken(*length)
		} else {
			value, err = tools.GenerateSecret(*length, "")
		}
		if err != nil {
			fmt.Fprintf(stderr, "generate failed: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, value)
		return 0
	case "encrypt", "decrypt":
		fs := flag.NewFlagSet("secrets "+args[0], flag.ContinueOnError)
		fs.SetOutput(stderr)
		pass := fs.String("passphrase", "", "passphrase")
		apply := fs.Bool("apply", false, "apply changes")
		jsonOut := fs.Bool("json", false, "emit JSON")
		if err := fs.Parse(normalizeToolArgs(args[1:], "passphrase")); err != nil {
			return 2
		}
		if len(fs.Args()) != 2 {
			fmt.Fprintf(stderr, "usage: spltool secrets %s <src> <dst> --passphrase secret [--apply]\n", args[0])
			return 2
		}
		var op tools.Operation
		if args[0] == "encrypt" {
			op = tools.EncryptFile(fs.Args()[0], fs.Args()[1], *pass, *apply, tools.Hooks{})
		} else {
			op = tools.DecryptFile(fs.Args()[0], fs.Args()[1], *pass, *apply, tools.Hooks{})
		}
		return printOperations(stdout, stderr, []tools.Operation{op}, *jsonOut)
	default:
		fmt.Fprintf(stderr, "unknown secrets command %q\n", args[0])
		return 2
	}
}

func runMedia(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: spltool media <info|convert|ffmpeg-status|install-ffmpeg>")
		return 2
	}
	switch args[0] {
	case "info":
		fs := flag.NewFlagSet("media info", flag.ContinueOnError)
		fs.SetOutput(stderr)
		jsonOut := fs.Bool("json", false, "emit JSON")
		if err := fs.Parse(normalizeToolArgs(args[1:])); err != nil {
			return 2
		}
		if len(fs.Args()) != 1 {
			fmt.Fprintln(stderr, "usage: spltool media info <path>")
			return 2
		}
		info, err := tools.MediaInfo(fs.Args()[0], tools.Hooks{})
		if err != nil {
			fmt.Fprintf(stderr, "media info failed: %v\n", err)
			return 1
		}
		if *jsonOut {
			return encodeJSON(stdout, stderr, info)
		}
		for k, v := range info {
			fmt.Fprintf(stdout, "%s: %v\n", k, v)
		}
		return 0
	case "convert":
		fs := flag.NewFlagSet("media convert", flag.ContinueOnError)
		fs.SetOutput(stderr)
		codec := fs.String("codec", "", "ffmpeg codec")
		crf := fs.String("crf", "", "ffmpeg CRF")
		install := fs.Bool("install", false, "install ffmpeg if missing when --apply is set")
		apply := fs.Bool("apply", false, "apply changes")
		jsonOut := fs.Bool("json", false, "emit JSON")
		if err := fs.Parse(normalizeToolArgs(args[1:], "codec", "crf")); err != nil {
			return 2
		}
		if len(fs.Args()) != 2 {
			fmt.Fprintln(stderr, "usage: spltool media convert <src> <dst> [--codec libx264] [--crf 23] [--apply]")
			return 2
		}
		op := tools.MediaConvert(fs.Args()[0], fs.Args()[1], map[string]any{"codec": *codec, "crf": *crf, "install": *install, "apply": *apply}, tools.Hooks{})
		return printOperations(stdout, stderr, []tools.Operation{op}, *jsonOut)
	case "ffmpeg-status":
		return encodeJSON(stdout, stderr, tools.FFmpegStatus())
	case "install-ffmpeg":
		fs := flag.NewFlagSet("media install-ffmpeg", flag.ContinueOnError)
		fs.SetOutput(stderr)
		apply := fs.Bool("apply", false, "install instead of previewing")
		jsonOut := fs.Bool("json", false, "emit JSON")
		if err := fs.Parse(normalizeToolArgs(args[1:])); err != nil {
			return 2
		}
		op := tools.InstallFFmpeg(*apply)
		return printOperations(stdout, stderr, []tools.Operation{op}, *jsonOut)
	default:
		fmt.Fprintf(stderr, "unknown media command %q\n", args[0])
		return 2
	}
}

func runOffice(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: spltool office <text|read>")
		return 2
	}
	switch args[0] {
	case "text":
		fs := flag.NewFlagSet("office text", flag.ContinueOnError)
		fs.SetOutput(stderr)
		if err := fs.Parse(normalizeToolArgs(args[1:])); err != nil {
			return 2
		}
		if len(fs.Args()) != 1 {
			fmt.Fprintln(stderr, "usage: spltool office text <path>")
			return 2
		}
		text, err := tools.OfficeText(fs.Args()[0], tools.Hooks{})
		if err != nil {
			fmt.Fprintf(stderr, "office text failed: %v\n", err)
			return 1
		}
		fmt.Fprint(stdout, text)
		if !strings.HasSuffix(text, "\n") {
			fmt.Fprintln(stdout)
		}
		return 0
	case "read":
		fs := flag.NewFlagSet("office read", flag.ContinueOnError)
		fs.SetOutput(stderr)
		jsonOut := fs.Bool("json", false, "emit JSON")
		if err := fs.Parse(normalizeToolArgs(args[1:])); err != nil {
			return 2
		}
		if len(fs.Args()) != 1 {
			fmt.Fprintln(stderr, "usage: spltool office read <path> [--json]")
			return 2
		}
		doc, err := tools.OfficeRead(fs.Args()[0], tools.Hooks{})
		if err != nil {
			fmt.Fprintf(stderr, "office read failed: %v\n", err)
			return 1
		}
		if *jsonOut {
			return encodeJSON(stdout, stderr, doc)
		}
		if text, ok := doc["text"].(string); ok {
			fmt.Fprintln(stdout, text)
			return 0
		}
		return encodeJSON(stdout, stderr, doc)
	default:
		fmt.Fprintf(stderr, "unknown office command %q\n", args[0])
		return 2
	}
}

func runMod(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: spltool mod <init|tidy|verify>")
		return 2
	}
	switch args[0] {
	case "init":
		fs := flag.NewFlagSet("mod init", flag.ContinueOnError)
		fs.SetOutput(stderr)
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		projectDir, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "failed to get cwd: %v\n", err)
			return 1
		}
		moduleName := ""
		if len(fs.Args()) > 0 {
			moduleName = fs.Args()[0]
		}
		if _, err := interpreter.InitModuleManifest(projectDir, moduleName); err != nil {
			fmt.Fprintf(stderr, "failed to write %s: %v\n", interpreter.SPLManifestFileName, err)
			return 1
		}
		fmt.Fprintf(stdout, "created %s\n", filepath.Join(projectDir, interpreter.SPLManifestFileName))
		return 0
	case "tidy":
		fs := flag.NewFlagSet("mod tidy", flag.ContinueOnError)
		fs.SetOutput(stderr)
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		projectDir, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "failed to get cwd: %v\n", err)
			return 1
		}
		lock, err := interpreter.SyncModuleLock(projectDir)
		if err != nil {
			fmt.Fprintf(stderr, "failed to sync %s: %v\n", interpreter.SPLLockFileName, err)
			return 1
		}
		fmt.Fprintf(stdout, "synced %s with %d dependencies\n", interpreter.SPLLockFileName, len(lock.Dependencies))
		return 0
	case "verify":
		fs := flag.NewFlagSet("mod verify", flag.ContinueOnError)
		fs.SetOutput(stderr)
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		projectDir, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "failed to get cwd: %v\n", err)
			return 1
		}
		if err := interpreter.VerifyModuleLock(projectDir); err != nil {
			fmt.Fprintf(stderr, "module lock verification failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "verified %s\n", interpreter.SPLLockFileName)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown mod command %q\n", args[0])
		return 2
	}
}

func runFmt(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("fmt", flag.ContinueOnError)
	fs.SetOutput(stderr)
	write := fs.Bool("w", false, "write result back to files")
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	targets := fs.Args()
	if len(targets) == 0 {
		targets = []string{"-"}
	}

	reports, code := processTargets(targets, stdin, *write, true)
	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(reports); err != nil {
			fmt.Fprintf(stderr, "failed to encode JSON: %v\n", err)
			return 1
		}
		return code
	}

	for _, rep := range reports {
		if len(rep.Diagnostics) > 0 {
			for _, d := range rep.Diagnostics {
				fmt.Fprintln(stderr, formatDiagnostic(d))
			}
			continue
		}
		if *write {
			if rep.Changed {
				fmt.Fprintf(stdout, "formatted %s\n", rep.Path)
			}
			continue
		}
		if rep.Formatted != "" {
			fmt.Fprint(stdout, rep.Formatted)
		}
	}
	return code
}

func runCheck(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	targets := fs.Args()
	if len(targets) == 0 {
		targets = []string{"-"}
	}

	reports, code := processTargets(targets, stdin, false, false)
	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(reports); err != nil {
			fmt.Fprintf(stderr, "failed to encode JSON: %v\n", err)
			return 1
		}
		return code
	}

	for _, rep := range reports {
		for _, d := range rep.Diagnostics {
			fmt.Fprintln(stderr, formatDiagnostic(d))
		}
	}
	return code
}

func runSymbols(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	targets := args
	if len(targets) == 0 {
		targets = []string{"-"}
	}
	all := []Symbol{}
	for _, target := range targets {
		path, src, err := readTarget(target, stdin)
		if err != nil {
			fmt.Fprintf(stderr, "symbols error: %v\n", err)
			return 1
		}
		all = append(all, SymbolsForSource(path, src)...)
	}
	return encodeJSON(stdout, stderr, all)
}

func runComplete(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("complete", flag.ContinueOnError)
	fs.SetOutput(stderr)
	prefix := fs.String("prefix", "", "completion prefix")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	target := "-"
	if len(fs.Args()) > 0 {
		target = fs.Args()[0]
	}
	path, src, err := readTarget(target, stdin)
	if err != nil {
		fmt.Fprintf(stderr, "complete error: %v\n", err)
		return 1
	}
	return encodeJSON(stdout, stderr, CompletionItems(path, src, *prefix))
}

func runHover(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("hover", flag.ContinueOnError)
	fs.SetOutput(stderr)
	line := fs.Int("line", 1, "1-based line")
	col := fs.Int("col", 1, "1-based column")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	target := "-"
	if len(fs.Args()) > 0 {
		target = fs.Args()[0]
	}
	path, src, err := readTarget(target, stdin)
	if err != nil {
		fmt.Fprintf(stderr, "hover error: %v\n", err)
		return 1
	}
	return encodeJSON(stdout, stderr, HoverAt(path, src, *line, *col))
}

func runDocs(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	targets := args
	if len(targets) == 0 {
		targets = []string{"-"}
	}
	for i, target := range targets {
		path, src, err := readTarget(target, stdin)
		if err != nil {
			fmt.Fprintf(stderr, "docs error: %v\n", err)
			return 1
		}
		if i > 0 {
			fmt.Fprintln(stdout)
		}
		fmt.Fprint(stdout, DocsMarkdown(path, src))
	}
	return 0
}

func runSession(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: spltool session <run|debug> [flags] [files...]")
		return 2
	}
	switch args[0] {
	case "run":
		return runSessionRun(args[1:], stdin, stdout, stderr)
	case "debug":
		return runSessionDebug(args[1:], stdin, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown session command %q\n", args[0])
		return 2
	}
}

func runSessionRun(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("session run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	profile := fs.String("profile", "trusted", "execution profile")
	checkpoint := fs.String("checkpoint", "", "checkpoint name to save after execution")
	persist := fs.Bool("persist", false, "persist session metadata and input log")
	dir := fs.String("dir", "", "session persistence directory")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	targets := fs.Args()
	if len(targets) == 0 {
		targets = []string{"-"}
	}
	var captured bytes.Buffer
	out := io.Writer(stdout)
	if *jsonOut {
		out = &captured
	}
	rt, err := interpreter.NewRuntime(interpreter.RuntimeOptions{Profile: *profile, Output: out, AllowInProcessFallback: true})
	if err != nil {
		fmt.Fprintf(stderr, "session error: %v\n", err)
		return 1
	}
	sess, err := rt.NewSession(interpreter.SessionOptions{
		ID:             "spltool-session",
		Profile:        *profile,
		Output:         out,
		Persist:        *persist,
		PersistenceDir: *dir,
	})
	if err != nil {
		fmt.Fprintf(stderr, "session error: %v\n", err)
		return 1
	}
	results := []interpreter.ExecutionResult{}
	for _, target := range targets {
		path, src, err := readTarget(target, stdin)
		if err != nil {
			fmt.Fprintf(stderr, "session read error: %v\n", err)
			return 1
		}
		res := sess.Execute(interpreter.ExecutionRequest{Source: src, Path: path})
		results = append(results, res)
		if !*jsonOut {
			if res.OK {
				fmt.Fprintf(stdout, "OK %s (%s)\n", res.Path, res.Metrics.Duration)
			} else {
				fmt.Fprintf(stdout, "ERROR %s\n%s\n", res.Path, res.Error)
			}
		}
	}
	var snap any
	if *checkpoint != "" {
		s, err := sess.Checkpoint(*checkpoint)
		if err != nil {
			fmt.Fprintf(stderr, "checkpoint error: %v\n", err)
			return 1
		}
		snap = s
	}
	report := map[string]any{
		"ok":         allSessionResultsOK(results),
		"output":     captured.String(),
		"results":    results,
		"checkpoint": snap,
		"session":    sess.Inspect(),
	}
	if *jsonOut {
		return encodeJSON(stdout, stderr, report)
	}
	return boolExit(allSessionResultsOK(results))
}

func runSessionDebug(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("session debug", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	target := "-"
	if len(fs.Args()) > 0 {
		target = fs.Args()[0]
	}
	path, src, err := readTarget(target, stdin)
	if err != nil {
		fmt.Fprintf(stderr, "session debug read error: %v\n", err)
		return 1
	}
	sess, err := interpreter.NewSession(interpreter.SessionOptions{ID: "spltool-debug", SourcePath: path, ModuleDir: ModuleDirForPath(path)})
	if err != nil {
		fmt.Fprintf(stderr, "session debug error: %v\n", err)
		return 1
	}
	trace := sess.Debug(src, path)
	if *jsonOut {
		return encodeJSON(stdout, stderr, trace)
	}
	for _, step := range trace.Steps {
		status := "ok"
		if !step.OK {
			status = "error"
		}
		detail := step.Result
		if detail == "" {
			detail = step.Error
		}
		fmt.Fprintf(stdout, "[%d] %s %s => %s\n", step.Index, status, step.Statement, detail)
	}
	return boolExit(trace.OK)
}

func allSessionResultsOK(results []interpreter.ExecutionResult) bool {
	for _, result := range results {
		if !result.OK {
			return false
		}
	}
	return true
}

func boolExit(ok bool) int {
	if ok {
		return 0
	}
	return 1
}

type TestReport struct {
	OK       bool             `json:"ok"`
	Total    int              `json:"total"`
	Passed   int              `json:"passed"`
	Failed   int              `json:"failed"`
	Duration int64            `json:"duration_ms"`
	Results  []TestFileResult `json:"results"`
}

type TestFileResult struct {
	Path       string `json:"path"`
	OK         bool   `json:"ok"`
	Error      string `json:"error,omitempty"`
	DurationMS int64  `json:"duration_ms"`
}

func runTest(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	filter := fs.String("filter", "", "substring filter for discovered test files")
	profile := fs.String("profile", "trusted", "execution profile: trusted or untrusted")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	switch strings.ToLower(strings.TrimSpace(*profile)) {
	case "trusted", "untrusted":
	default:
		fmt.Fprintf(stderr, "invalid --profile %q\n", *profile)
		return 2
	}
	targets := fs.Args()
	if len(targets) == 0 {
		targets = []string{"."}
	}
	files, err := discoverTestFiles(targets, *filter)
	if err != nil {
		fmt.Fprintf(stderr, "test discovery error: %v\n", err)
		return 1
	}
	report := TestReport{OK: true, Total: len(files)}
	start := time.Now()
	for _, file := range files {
		itemStart := time.Now()
		_, err := interpreter.ExecFileWithOptions(file, nil, interpreter.ExecOptions{
			ModuleDir: filepath.Dir(file),
			Profile:   strings.ToLower(strings.TrimSpace(*profile)),
			MaxSteps:  2_000_000,
			MaxDepth:  256,
			Timeout:   10 * time.Second,
		})
		item := TestFileResult{Path: file, OK: err == nil, DurationMS: time.Since(itemStart).Milliseconds()}
		if err != nil {
			item.Error = err.Error()
			report.OK = false
			report.Failed++
		} else {
			report.Passed++
		}
		report.Results = append(report.Results, item)
	}
	report.Duration = time.Since(start).Milliseconds()
	if *jsonOut {
		return encodeJSON(stdout, stderr, report)
	}
	for _, result := range report.Results {
		if result.OK {
			fmt.Fprintf(stdout, "PASS %s (%dms)\n", result.Path, result.DurationMS)
		} else {
			fmt.Fprintf(stdout, "FAIL %s (%dms)\n%s\n", result.Path, result.DurationMS, result.Error)
		}
	}
	fmt.Fprintf(stdout, "\n%d passed, %d failed, %d total\n", report.Passed, report.Failed, report.Total)
	if !report.OK {
		return 1
	}
	return 0
}

func runConformance(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("conformance", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	profile := fs.String("profile", "trusted", "execution profile: trusted or untrusted")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	targets := fs.Args()
	if len(targets) == 0 {
		targets = []string{"testdata/conformance"}
	}
	runArgs := []string{"--profile", *profile}
	if *jsonOut {
		runArgs = append(runArgs, "--json")
	}
	runArgs = append(runArgs, targets...)
	return runTest(runArgs, stdout, stderr)
}

func runLSP(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("lsp", flag.ContinueOnError)
	fs.SetOutput(stderr)
	stdio := fs.Bool("stdio", false, "run the SPL language server over stdio")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *stdio {
		return runLSPServer(stdin, stdout, stderr)
	}
	return runLSPInfo(stdout)
}

func runLSPInfo(stdout io.Writer) int {
	_ = json.NewEncoder(stdout).Encode(map[string]any{
		"status": "available",
		"commands": []string{
			"spltool lsp --stdio",
			"spltool check --json",
			"spltool symbols",
			"spltool complete --prefix <text>",
			"spltool hover --line <n> --col <n>",
			"spltool fmt",
		},
		"note": "These JSON surfaces are stable building blocks for an editor language server.",
	})
	return 0
}

func processTargets(targets []string, stdin io.Reader, write bool, format bool) ([]Report, int) {
	reports := make([]Report, 0, len(targets))
	exitCode := 0
	for _, target := range targets {
		path, src, err := readTarget(target, stdin)
		if err != nil {
			exitCode = 1
			reports = append(reports, Report{
				Path: target,
				OK:   false,
				Diagnostics: []Diagnostic{{
					Severity: SeverityError,
					Path:     target,
					Message:  err.Error(),
				}},
			})
			continue
		}
		if format {
			report := FormatSource(path, src)
			reports = append(reports, report)
			if len(report.Diagnostics) > 0 {
				exitCode = 1
				continue
			}
			if write && report.Changed && target != "-" {
				if err := os.WriteFile(path, []byte(report.Formatted), 0o644); err != nil {
					exitCode = 1
					reports[len(reports)-1].OK = false
					reports[len(reports)-1].Diagnostics = append(reports[len(reports)-1].Diagnostics, Diagnostic{
						Severity: SeverityError,
						Path:     path,
						Message:  err.Error(),
					})
				}
			}
			continue
		}
		report := CheckSource(path, src)
		reports = append(reports, report)
		if !report.OK {
			exitCode = 1
		}
	}
	return reports, exitCode
}

func readTarget(target string, stdin io.Reader) (string, string, error) {
	if target == "-" {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return "", "", err
		}
		return DefaultStdinPath(), string(data), nil
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return "", "", err
	}
	return filepath.Clean(target), string(data), nil
}

func formatDiagnostic(d Diagnostic) string {
	var b strings.Builder
	b.WriteString(string(d.Severity))
	b.WriteString(": ")
	if d.Path != "" {
		b.WriteString(d.Path)
		if d.Line > 0 {
			fmt.Fprintf(&b, ":%d", d.Line)
			if d.Column > 0 {
				fmt.Fprintf(&b, ":%d", d.Column)
			}
		}
		b.WriteString(": ")
	}
	b.WriteString(d.Message)
	if d.Snippet != "" {
		b.WriteByte('\n')
		b.WriteString(d.Snippet)
	}
	return b.String()
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: spltool <fmt|check|files|archive|image|secrets|media|office|symbols|complete|hover|docs|test|session|conformance|config|mod|lsp> [flags] [files...]")
	fmt.Fprintln(w, "Use '-' to read from stdin.")
}

func printOperations(stdout, stderr io.Writer, ops []tools.Operation, jsonOut bool) int {
	if jsonOut {
		return encodeJSON(stdout, stderr, ops)
	}
	code := 0
	for _, op := range ops {
		if op.Status == "failed" {
			code = 1
		}
		line := strings.TrimSpace(fmt.Sprintf("%s %s %s -> %s", op.Status, op.Op, op.Src, op.Dst))
		if op.Reason != "" {
			line += " (" + op.Reason + ")"
		}
		if op.Error != "" {
			line += " ERROR: " + op.Error
		}
		fmt.Fprintln(stdout, line)
	}
	return code
}

func normalizeToolArgs(args []string, valueFlags ...string) []string {
	value := map[string]struct{}{}
	for _, flag := range valueFlags {
		value["-"+flag] = struct{}{}
		value["--"+flag] = struct{}{}
	}
	flags := []string{}
	positionals := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") && arg != "-" {
			flags = append(flags, arg)
			name := arg
			if idx := strings.IndexByte(name, '='); idx >= 0 {
				name = name[:idx]
			}
			if _, needsValue := value[name]; needsValue && !strings.Contains(arg, "=") && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		positionals = append(positionals, arg)
	}
	return append(flags, positionals...)
}

func encodeJSON(stdout, stderr io.Writer, v any) int {
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(stderr, "failed to encode JSON: %v\n", err)
		return 1
	}
	return 0
}

func discoverTestFiles(targets []string, filter string) ([]string, error) {
	seen := map[string]struct{}{}
	files := []string{}
	add := func(path string) {
		path = filepath.Clean(path)
		if filter != "" && !strings.Contains(path, filter) {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		files = append(files, path)
	}
	for _, target := range targets {
		info, err := os.Stat(target)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			if strings.HasSuffix(target, ".spl") {
				add(target)
			}
			continue
		}
		if err := filepath.WalkDir(target, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == ".git" || d.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(d.Name(), "_test.spl") || strings.Contains(filepath.ToSlash(path), "/tests/") && strings.HasSuffix(d.Name(), ".spl") {
				add(path)
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}
	return files, nil
}
