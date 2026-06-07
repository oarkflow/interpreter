package tools

import (
	"fmt"
	"strings"
	"time"

	builtinspkg "github.com/oarkflow/interpreter/pkg/builtins"
	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/security"
	toolspkg "github.com/oarkflow/interpreter/pkg/tools"
)

func init() {
	eval.RegisterBuiltins(map[string]*object.Builtin{
		"bulk_rename":         {Fn: builtinBulkRename},
		"file_search":         {Fn: builtinFileSearch},
		"file_locate":         {Fn: builtinFileSearch},
		"file_finder":         {Fn: builtinFileFinder},
		"file_move_plan":      {Fn: builtinFileMove},
		"file_copy_plan":      {Fn: builtinFileCopy},
		"file_dedupe":         {Fn: builtinFileDedupe},
		"file_remove_plan":    {Fn: builtinFileRemove},
		"file_organize":       {Fn: builtinFileOrganize},
		"file_checksum":       {Fn: builtinFileChecksum},
		"archive_compress":    {Fn: builtinArchiveCompress},
		"archive_extract":     {Fn: builtinArchiveExtract},
		"archive_list":        {Fn: builtinArchiveList},
		"image_convert_batch": {Fn: builtinImageConvertBatch},
		"image_optimize":      {Fn: builtinImageConvertBatch},
		"image_crop_file":     {Fn: builtinImageCropFile},
		"image_resize_file":   {Fn: builtinImageResizeFile},
		"image_thumbnail":     {Fn: builtinImageThumbnail},
		"image_info_file":     {Fn: builtinImageInfoFile},
		"secret_generate":     {Fn: builtinSecretGenerate},
		"token_generate":      {Fn: builtinTokenGenerate},
		"file_encrypt":        {Fn: builtinFileEncrypt},
		"file_decrypt":        {Fn: builtinFileDecrypt},
		"media_info":          {Fn: builtinMediaInfo},
		"media_convert":       {Fn: builtinMediaConvert},
		"ffmpeg_status":       {Fn: builtinFFmpegStatus},
		"ffmpeg_install":      {Fn: builtinFFmpegInstall},
		"system_info":         {Fn: builtinSystemInfo},
		"dns_lookup":          {Fn: builtinDNSLookup},
		"tcp_check":           {Fn: builtinTCPCheck},
		"http_probe":          {Fn: builtinHTTPProbe},
		"office_text":         {Fn: builtinOfficeText},
		"office_read":         {Fn: builtinOfficeRead},
	})

	prev := eval.DotExpressionHook
	eval.DotExpressionHook = func(left object.Object, name string) object.Object {
		if finder, ok := left.(*FileFinder); ok {
			return getFileFinderProperty(finder, name)
		}
		if prev != nil {
			return prev(left, name)
		}
		return nil
	}
}

func hooks() toolspkg.Hooks {
	return toolspkg.Hooks{
		Sanitize:   builtinspkg.SanitizePathLocal,
		CheckRead:  security.CheckFileReadAllowed,
		CheckWrite: security.CheckFileWriteAllowed,
	}
}

func builtinBulkRename(args ...object.Object) object.Object {
	if len(args) < 1 || len(args) > 2 {
		return object.NewError("bulk_rename(dir[, opts]) expects 1 or 2 arguments")
	}
	dir, errObj := asString(args[0], "dir")
	if errObj != nil {
		return errObj
	}
	opts, errObj := optsArg(args, 1)
	if errObj != nil {
		return errObj
	}
	ops, err := toolspkg.BulkRename(dir, opts, hooks())
	if err != nil {
		return object.NewError("%s", err)
	}
	return operationsObj(ops)
}

func builtinFileSearch(args ...object.Object) object.Object {
	if len(args) < 1 || len(args) > 2 {
		return object.NewError("file_search(root[, opts]) expects 1 or 2 arguments")
	}
	root, errObj := asString(args[0], "root")
	if errObj != nil {
		return errObj
	}
	opts, errObj := optsArg(args, 1)
	if errObj != nil {
		return errObj
	}
	files, err := toolspkg.Search(root, opts, hooks())
	if err != nil {
		return object.NewError("%s", err)
	}
	out := make([]object.Object, 0, len(files))
	for _, f := range files {
		out = append(out, nativeToObj(map[string]any{
			"path": f.Path, "name": f.Name, "size": f.Size, "mode": f.Mode,
			"mod_time": f.ModTime, "is_dir": f.IsDir, "mime": f.MIME,
		}))
	}
	return &object.Array{Elements: out}
}

type FileFinder struct {
	Root string
	Opts map[string]any
}

func (ff *FileFinder) Type() object.ObjectType { return object.FILE_FINDER_OBJ }
func (ff *FileFinder) Inspect() string {
	return fmt.Sprintf("<file_finder: %s>", ff.Root)
}

func builtinFileFinder(args ...object.Object) object.Object {
	if len(args) != 1 {
		return object.NewError("file_finder(root) expects 1 argument")
	}
	root, errObj := asString(args[0], "root")
	if errObj != nil {
		return errObj
	}
	return &FileFinder{Root: root, Opts: map[string]any{}}
}

func getFileFinderProperty(ff *FileFinder, name string) object.Object {
	switch name {
	case "files":
		return finderNoArg(ff, name, func() { ff.Opts["type"] = "file" })
	case "dirs":
		return finderNoArg(ff, name, func() { ff.Opts["type"] = "dir"; ff.Opts["include_dirs"] = true })
	case "any":
		return finderNoArg(ff, name, func() { ff.Opts["type"] = "any"; ff.Opts["include_dirs"] = true })
	case "match":
		return finderStringArg(ff, name, "match")
	case "pattern_type":
		return finderStringArg(ff, name, "pattern_type")
	case "regex":
		return finderStringArg(ff, name, "regex")
	case "name":
		return finderStringArg(ff, name, "name")
	case "path_contains":
		return finderStringArg(ff, name, "path_contains")
	case "path_regex":
		return finderStringArg(ff, name, "path_regex")
	case "content":
		return finderStringArg(ff, name, "content")
	case "content_regex":
		return finderStringArg(ff, name, "content_regex")
	case "ext":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return object.NewError("ext() expects 1 argument")
			}
			switch v := args[0].(type) {
			case *object.String:
				ff.Opts["ext"] = v.Value
			case *object.Array:
				ff.Opts["ext"] = objectToNative(v)
			default:
				return object.NewError("ext() argument must be STRING or ARRAY")
			}
			return ff
		}}
	case "size":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return object.NewError("size(min, max) expects 2 arguments")
			}
			min, errObj := asInt(args[0], "min")
			if errObj != nil {
				return errObj
			}
			max, errObj := asInt(args[1], "max")
			if errObj != nil {
				return errObj
			}
			ff.Opts["min_size"], ff.Opts["max_size"] = min, max
			return ff
		}}
	case "modified":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return object.NewError("modified(after, before) expects 2 arguments")
			}
			after, errObj := asInt(args[0], "after")
			if errObj != nil {
				return errObj
			}
			before, errObj := asInt(args[1], "before")
			if errObj != nil {
				return errObj
			}
			ff.Opts["modified_after"], ff.Opts["modified_before"] = after, before
			return ff
		}}
	case "recursive":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return object.NewError("recursive(bool) expects 1 argument")
			}
			b, ok := args[0].(*object.Boolean)
			if !ok {
				return object.NewError("recursive() argument must be BOOLEAN")
			}
			ff.Opts["recursive"] = b.Value
			return ff
		}}
	case "max_depth":
		return finderIntArg(ff, name, "max_depth")
	case "limit":
		return finderIntArg(ff, name, "limit")
	case "sort":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 || len(args) > 2 {
				return object.NewError("sort(key[, desc]) expects 1 or 2 arguments")
			}
			key, errObj := asString(args[0], "key")
			if errObj != nil {
				return errObj
			}
			ff.Opts["sort"] = key
			if len(args) == 2 {
				b, ok := args[1].(*object.Boolean)
				if !ok {
					return object.NewError("sort() desc argument must be BOOLEAN")
				}
				ff.Opts["desc"] = b.Value
			}
			return ff
		}}
	case "exec":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) != 0 {
				return object.NewError("exec() expects 0 arguments")
			}
			return fileSearchResult(ff.Root, ff.Opts)
		}}
	default:
		return nil
	}
}

func finderNoArg(ff *FileFinder, name string, apply func()) object.Object {
	return &object.Builtin{Fn: func(args ...object.Object) object.Object {
		if len(args) != 0 {
			return object.NewError("%s() expects 0 arguments", name)
		}
		apply()
		return ff
	}}
}

func finderStringArg(ff *FileFinder, name, key string) object.Object {
	return &object.Builtin{Fn: func(args ...object.Object) object.Object {
		if len(args) != 1 {
			return object.NewError("%s(value) expects 1 argument", name)
		}
		value, errObj := asString(args[0], "value")
		if errObj != nil {
			return errObj
		}
		ff.Opts[key] = value
		return ff
	}}
}

func finderIntArg(ff *FileFinder, name, key string) object.Object {
	return &object.Builtin{Fn: func(args ...object.Object) object.Object {
		if len(args) != 1 {
			return object.NewError("%s(value) expects 1 argument", name)
		}
		value, errObj := asInt(args[0], "value")
		if errObj != nil {
			return errObj
		}
		ff.Opts[key] = value
		return ff
	}}
}

func fileSearchResult(root string, opts map[string]any) object.Object {
	files, err := toolspkg.Search(root, opts, hooks())
	if err != nil {
		return object.NewError("%s", err)
	}
	out := make([]object.Object, 0, len(files))
	for _, f := range files {
		out = append(out, nativeToObj(map[string]any{
			"path": f.Path, "name": f.Name, "size": f.Size, "mode": f.Mode,
			"mod_time": f.ModTime, "is_dir": f.IsDir, "mime": f.MIME,
		}))
	}
	return &object.Array{Elements: out}
}

func builtinFileMove(args ...object.Object) object.Object {
	return copyMoveBuiltin(args, true)
}

func builtinFileCopy(args ...object.Object) object.Object {
	return copyMoveBuiltin(args, false)
}

func copyMoveBuiltin(args []object.Object, move bool) object.Object {
	if len(args) < 2 || len(args) > 3 {
		return object.NewError("file_move_plan/file_copy_plan(src, dst[, opts]) expects 2 or 3 arguments")
	}
	src, errObj := asString(args[0], "src")
	if errObj != nil {
		return errObj
	}
	dst, errObj := asString(args[1], "dst")
	if errObj != nil {
		return errObj
	}
	opts, errObj := optsArg(args, 2)
	if errObj != nil {
		return errObj
	}
	return operationObj(toolspkg.CopyOrMove(src, dst, move, boolOpt(opts, "apply"), hooks()))
}

func builtinFileDedupe(args ...object.Object) object.Object {
	if len(args) < 1 || len(args) > 2 {
		return object.NewError("file_dedupe(root[, opts]) expects 1 or 2 arguments")
	}
	root, errObj := asString(args[0], "root")
	if errObj != nil {
		return errObj
	}
	opts, errObj := optsArg(args, 1)
	if errObj != nil {
		return errObj
	}
	ops, err := toolspkg.Dedupe(root, opts, hooks())
	if err != nil {
		return object.NewError("%s", err)
	}
	return operationsObj(ops)
}

func builtinFileRemove(args ...object.Object) object.Object {
	if len(args) < 1 || len(args) > 2 {
		return object.NewError("file_remove_plan(path[, opts]) expects 1 or 2 arguments")
	}
	path, errObj := asString(args[0], "path")
	if errObj != nil {
		return errObj
	}
	opts, errObj := optsArg(args, 1)
	if errObj != nil {
		return errObj
	}
	return operationObj(toolspkg.RemovePath(path, opts, hooks()))
}

func builtinFileOrganize(args ...object.Object) object.Object {
	if len(args) < 2 || len(args) > 3 {
		return object.NewError("file_organize(src_dir, dst_dir[, opts]) expects 2 or 3 arguments")
	}
	src, errObj := asString(args[0], "src_dir")
	if errObj != nil {
		return errObj
	}
	dst, errObj := asString(args[1], "dst_dir")
	if errObj != nil {
		return errObj
	}
	opts, errObj := optsArg(args, 2)
	if errObj != nil {
		return errObj
	}
	ops, err := toolspkg.OrganizeByExt(src, dst, opts, hooks())
	if err != nil {
		return object.NewError("%s", err)
	}
	return operationsObj(ops)
}

func builtinFileChecksum(args ...object.Object) object.Object {
	if len(args) != 1 {
		return object.NewError("file_checksum(path) expects 1 argument")
	}
	path, errObj := asString(args[0], "path")
	if errObj != nil {
		return errObj
	}
	sum, err := toolspkg.FileChecksum(path, hooks())
	if err != nil {
		return object.NewError("%s", err)
	}
	return nativeToObj(sum)
}

func builtinArchiveCompress(args ...object.Object) object.Object {
	if len(args) < 2 || len(args) > 3 {
		return object.NewError("archive_compress(src, dst[, opts]) expects 2 or 3 arguments")
	}
	src, errObj := asString(args[0], "src")
	if errObj != nil {
		return errObj
	}
	dst, errObj := asString(args[1], "dst")
	if errObj != nil {
		return errObj
	}
	opts, errObj := optsArg(args, 2)
	if errObj != nil {
		return errObj
	}
	return operationObj(toolspkg.Compress(src, dst, opts, hooks()))
}

func builtinArchiveExtract(args ...object.Object) object.Object {
	if len(args) < 2 || len(args) > 3 {
		return object.NewError("archive_extract(src, dst[, opts]) expects 2 or 3 arguments")
	}
	src, errObj := asString(args[0], "src")
	if errObj != nil {
		return errObj
	}
	dst, errObj := asString(args[1], "dst")
	if errObj != nil {
		return errObj
	}
	opts, errObj := optsArg(args, 2)
	if errObj != nil {
		return errObj
	}
	return operationObj(toolspkg.Extract(src, dst, opts, hooks()))
}

func builtinArchiveList(args ...object.Object) object.Object {
	if len(args) != 1 {
		return object.NewError("archive_list(path) expects 1 argument")
	}
	path, errObj := asString(args[0], "path")
	if errObj != nil {
		return errObj
	}
	files, err := toolspkg.ArchiveList(path, hooks())
	if err != nil {
		return object.NewError("%s", err)
	}
	out := make([]object.Object, 0, len(files))
	for _, f := range files {
		out = append(out, nativeToObj(map[string]any{"path": f.Path, "name": f.Name, "size": f.Size, "is_dir": f.IsDir}))
	}
	return &object.Array{Elements: out}
}

func builtinImageConvertBatch(args ...object.Object) object.Object {
	if len(args) < 2 || len(args) > 3 {
		return object.NewError("image_convert_batch(src_dir, dst_dir[, opts]) expects 2 or 3 arguments")
	}
	src, errObj := asString(args[0], "src_dir")
	if errObj != nil {
		return errObj
	}
	dst, errObj := asString(args[1], "dst_dir")
	if errObj != nil {
		return errObj
	}
	opts, errObj := optsArg(args, 2)
	if errObj != nil {
		return errObj
	}
	ops, err := toolspkg.ImageConvertBatch(src, dst, opts, hooks())
	if err != nil {
		return object.NewError("%s", err)
	}
	return operationsObj(ops)
}

func builtinImageCropFile(args ...object.Object) object.Object {
	if len(args) < 2 || len(args) > 3 {
		return object.NewError("image_crop_file(src, dst[, opts]) expects 2 or 3 arguments")
	}
	src, errObj := asString(args[0], "src")
	if errObj != nil {
		return errObj
	}
	dst, errObj := asString(args[1], "dst")
	if errObj != nil {
		return errObj
	}
	opts, errObj := optsArg(args, 2)
	if errObj != nil {
		return errObj
	}
	return operationObj(toolspkg.CropImage(src, dst, opts, hooks()))
}

func builtinImageResizeFile(args ...object.Object) object.Object {
	if len(args) < 2 || len(args) > 3 {
		return object.NewError("image_resize_file(src, dst[, opts]) expects 2 or 3 arguments")
	}
	src, errObj := asString(args[0], "src")
	if errObj != nil {
		return errObj
	}
	dst, errObj := asString(args[1], "dst")
	if errObj != nil {
		return errObj
	}
	opts, errObj := optsArg(args, 2)
	if errObj != nil {
		return errObj
	}
	return operationObj(toolspkg.ResizeImage(src, dst, opts, hooks()))
}

func builtinImageThumbnail(args ...object.Object) object.Object {
	if len(args) < 2 || len(args) > 3 {
		return object.NewError("image_thumbnail(src, dst[, opts]) expects 2 or 3 arguments")
	}
	src, errObj := asString(args[0], "src")
	if errObj != nil {
		return errObj
	}
	dst, errObj := asString(args[1], "dst")
	if errObj != nil {
		return errObj
	}
	opts, errObj := optsArg(args, 2)
	if errObj != nil {
		return errObj
	}
	return operationObj(toolspkg.Thumbnail(src, dst, opts, hooks()))
}

func builtinImageInfoFile(args ...object.Object) object.Object {
	if len(args) != 1 {
		return object.NewError("image_info_file(path) expects 1 argument")
	}
	path, errObj := asString(args[0], "path")
	if errObj != nil {
		return errObj
	}
	info, err := toolspkg.ImageInfo(path, hooks())
	if err != nil {
		return object.NewError("%s", err)
	}
	return nativeToObj(info)
}

func builtinSecretGenerate(args ...object.Object) object.Object {
	length := int64(24)
	alphabet := ""
	if len(args) > 0 {
		var errObj object.Object
		length, errObj = asInt(args[0], "length")
		if errObj != nil {
			return errObj
		}
	}
	if len(args) > 1 {
		var errObj object.Object
		alphabet, errObj = asString(args[1], "alphabet")
		if errObj != nil {
			return errObj
		}
	}
	value, err := toolspkg.GenerateSecret(int(length), alphabet)
	if err != nil {
		return object.NewError("%s", err)
	}
	return &object.Secret{Value: value}
}

func builtinTokenGenerate(args ...object.Object) object.Object {
	n := int64(32)
	if len(args) > 0 {
		var errObj object.Object
		n, errObj = asInt(args[0], "bytes")
		if errObj != nil {
			return errObj
		}
	}
	value, err := toolspkg.EncodeToken(int(n))
	if err != nil {
		return object.NewError("%s", err)
	}
	return &object.Secret{Value: value}
}

func builtinFileEncrypt(args ...object.Object) object.Object {
	return cryptBuiltin(args, true)
}

func builtinFileDecrypt(args ...object.Object) object.Object {
	return cryptBuiltin(args, false)
}

func cryptBuiltin(args []object.Object, enc bool) object.Object {
	if len(args) < 3 || len(args) > 4 {
		return object.NewError("file_encrypt/file_decrypt(src, dst, passphrase[, opts]) expects 3 or 4 arguments")
	}
	src, errObj := asString(args[0], "src")
	if errObj != nil {
		return errObj
	}
	dst, errObj := asString(args[1], "dst")
	if errObj != nil {
		return errObj
	}
	pass, errObj := asString(args[2], "passphrase")
	if errObj != nil {
		return errObj
	}
	opts, errObj := optsArg(args, 3)
	if errObj != nil {
		return errObj
	}
	if enc {
		return operationObj(toolspkg.EncryptFile(src, dst, pass, boolOpt(opts, "apply"), hooks()))
	}
	return operationObj(toolspkg.DecryptFile(src, dst, pass, boolOpt(opts, "apply"), hooks()))
}

func builtinMediaInfo(args ...object.Object) object.Object {
	if len(args) != 1 {
		return object.NewError("media_info(path) expects 1 argument")
	}
	useFFprobe := security.CheckExecAllowed("ffprobe") == nil
	path, errObj := asString(args[0], "path")
	if errObj != nil {
		return errObj
	}
	info, err := toolspkg.MediaInfoWithFFprobe(path, hooks(), useFFprobe)
	if err != nil {
		return object.NewError("%s", err)
	}
	return nativeToObj(info)
}

func builtinMediaConvert(args ...object.Object) object.Object {
	if len(args) < 2 || len(args) > 3 {
		return object.NewError("media_convert(src, dst[, opts]) expects 2 or 3 arguments")
	}
	if err := security.CheckExecAllowed("ffmpeg"); err != nil {
		return object.NewError("%s", err)
	}
	src, errObj := asString(args[0], "src")
	if errObj != nil {
		return errObj
	}
	dst, errObj := asString(args[1], "dst")
	if errObj != nil {
		return errObj
	}
	opts, errObj := optsArg(args, 2)
	if errObj != nil {
		return errObj
	}
	if boolOpt(opts, "install") {
		if err := security.CheckExecAllowed("ffmpeg-install"); err != nil {
			return object.NewError("%s", err)
		}
	}
	return operationObj(toolspkg.MediaConvert(src, dst, opts, hooks()))
}

func builtinFFmpegStatus(args ...object.Object) object.Object {
	if len(args) != 0 {
		return object.NewError("ffmpeg_status() expects no arguments")
	}
	return nativeToObj(toolspkg.FFmpegStatus())
}

func builtinFFmpegInstall(args ...object.Object) object.Object {
	opts, errObj := optsArg(args, 0)
	if errObj != nil {
		return errObj
	}
	if err := security.CheckExecAllowed("ffmpeg-install"); err != nil {
		return object.NewError("%s", err)
	}
	return operationObj(toolspkg.InstallFFmpeg(boolOpt(opts, "apply")))
}

func builtinSystemInfo(args ...object.Object) object.Object {
	if len(args) != 0 {
		return object.NewError("system_info() expects no arguments")
	}
	if err := security.CheckCapabilityAllowed(security.CapabilitySystem); err != nil {
		return object.NewError("%s", err)
	}
	return nativeToObj(toolspkg.SystemInfo())
}

func builtinDNSLookup(args ...object.Object) object.Object {
	if len(args) != 1 {
		return object.NewError("dns_lookup(host) expects 1 argument")
	}
	host, errObj := asString(args[0], "host")
	if errObj != nil {
		return errObj
	}
	if err := security.CheckNetworkAllowed(host); err != nil {
		return object.NewError("%s", err)
	}
	ips, err := toolspkg.DNSLookup(host)
	if err != nil {
		return object.NewError("%s", err)
	}
	return nativeToObj(ips)
}

func builtinTCPCheck(args ...object.Object) object.Object {
	if len(args) < 1 || len(args) > 2 {
		return object.NewError("tcp_check(address[, timeout_ms]) expects 1 or 2 arguments")
	}
	addr, errObj := asString(args[0], "address")
	if errObj != nil {
		return errObj
	}
	if err := security.CheckNetworkAllowed(addr); err != nil {
		return object.NewError("%s", err)
	}
	timeout := int64(2000)
	if len(args) == 2 {
		timeout, errObj = asInt(args[1], "timeout_ms")
		if errObj != nil {
			return errObj
		}
	}
	return nativeToObj(toolspkg.TCPCheck(addr, time.Duration(timeout)*time.Millisecond))
}

func builtinHTTPProbe(args ...object.Object) object.Object {
	if len(args) < 1 || len(args) > 2 {
		return object.NewError("http_probe(url[, timeout_ms]) expects 1 or 2 arguments")
	}
	target, errObj := asString(args[0], "url")
	if errObj != nil {
		return errObj
	}
	if err := security.CheckNetworkAllowed(target); err != nil {
		return object.NewError("%s", err)
	}
	timeout := int64(5000)
	if len(args) == 2 {
		timeout, errObj = asInt(args[1], "timeout_ms")
		if errObj != nil {
			return errObj
		}
	}
	return nativeToObj(toolspkg.HTTPProbe(target, time.Duration(timeout)*time.Millisecond))
}

func builtinOfficeText(args ...object.Object) object.Object {
	if len(args) != 1 {
		return object.NewError("office_text(path) expects 1 argument")
	}
	path, errObj := asString(args[0], "path")
	if errObj != nil {
		return errObj
	}
	text, err := toolspkg.OfficeText(path, hooks())
	if err != nil {
		return object.NewError("%s", err)
	}
	return &object.String{Value: text}
}

func builtinOfficeRead(args ...object.Object) object.Object {
	if len(args) != 1 {
		return object.NewError("office_read(path) expects 1 argument")
	}
	path, errObj := asString(args[0], "path")
	if errObj != nil {
		return errObj
	}
	doc, err := toolspkg.OfficeRead(path, hooks())
	if err != nil {
		return object.NewError("%s", err)
	}
	return nativeToObj(doc)
}

func optsArg(args []object.Object, idx int) (map[string]any, object.Object) {
	if len(args) <= idx || args[idx] == nil || args[idx] == object.NULL {
		return map[string]any{}, nil
	}
	h, ok := args[idx].(*object.Hash)
	if !ok {
		return nil, object.NewError("argument `opts` must be HASH, got %s", args[idx].Type())
	}
	out := make(map[string]any, len(h.Pairs))
	for _, pair := range h.Pairs {
		out[strings.TrimSpace(pair.Key.Inspect())] = objectToNative(pair.Value)
	}
	return out, nil
}

func asString(arg object.Object, name string) (string, object.Object) {
	switch v := arg.(type) {
	case *object.String:
		return v.Value, nil
	case *object.Secret:
		return v.Value, nil
	default:
		return "", object.NewError("argument `%s` must be STRING, got %s", name, arg.Type())
	}
}

func asInt(arg object.Object, name string) (int64, object.Object) {
	switch v := arg.(type) {
	case *object.Integer:
		return v.Value, nil
	case *object.Float:
		return int64(v.Value), nil
	default:
		return 0, object.NewError("argument `%s` must be INTEGER, got %s", name, arg.Type())
	}
}

func boolOpt(opts map[string]any, key string) bool {
	v, ok := opts[key]
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

func objectToNative(obj object.Object) any {
	switch v := obj.(type) {
	case *object.Null:
		return nil
	case *object.Boolean:
		return v.Value
	case *object.Integer:
		return v.Value
	case *object.Float:
		return v.Value
	case *object.String:
		return v.Value
	case *object.Secret:
		return v.Value
	case *object.Array:
		out := make([]any, 0, len(v.Elements))
		for _, el := range v.Elements {
			out = append(out, objectToNative(el))
		}
		return out
	case *object.Hash:
		out := map[string]any{}
		for _, pair := range v.Pairs {
			out[pair.Key.Inspect()] = objectToNative(pair.Value)
		}
		return out
	default:
		return v.Inspect()
	}
}

func nativeToObj(v any) object.Object {
	switch x := v.(type) {
	case nil:
		return object.NULL
	case bool:
		return object.NativeBoolToBooleanObject(x)
	case int:
		return &object.Integer{Value: int64(x)}
	case int64:
		return &object.Integer{Value: x}
	case float64:
		return &object.Float{Value: x}
	case string:
		return &object.String{Value: x}
	case []string:
		elems := make([]object.Object, 0, len(x))
		for _, s := range x {
			elems = append(elems, &object.String{Value: s})
		}
		return &object.Array{Elements: elems}
	case [][]string:
		rows := make([]object.Object, 0, len(x))
		for _, row := range x {
			rows = append(rows, nativeToObj(row))
		}
		return &object.Array{Elements: rows}
	case []any:
		elems := make([]object.Object, 0, len(x))
		for _, el := range x {
			elems = append(elems, nativeToObj(el))
		}
		return &object.Array{Elements: elems}
	case map[string]any:
		pairs := map[object.HashKey]object.HashPair{}
		for k, val := range x {
			key := &object.String{Value: k}
			pairs[key.HashKey()] = object.HashPair{Key: key, Value: nativeToObj(val)}
		}
		return &object.Hash{Pairs: pairs}
	default:
		return &object.String{Value: fmt.Sprint(x)}
	}
}

func operationObj(op toolspkg.Operation) object.Object {
	return nativeToObj(map[string]any{
		"status": op.Status, "op": op.Op, "src": op.Src, "dst": op.Dst,
		"reason": op.Reason, "error": op.Error, "size_before": op.SizeBefore, "size_after": op.SizeAfter,
	})
}

func operationsObj(ops []toolspkg.Operation) object.Object {
	elems := make([]object.Object, 0, len(ops))
	for _, op := range ops {
		elems = append(elems, operationObj(op))
	}
	return &object.Array{Elements: elems}
}
