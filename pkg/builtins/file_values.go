package builtins

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/security"
)

func init() {
	eval.RegisterBuiltins(map[string]*object.Builtin{
		"file_load": {
			FnWithEnv: func(env *object.Environment, args ...object.Object) object.Object {
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
				file, errObj := resolveFileInput(env, args[0], "file", opts)
				if errObj != nil {
					return errObj
				}
				return file
			},
		},
		"file_save": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 2 || len(args) > 3 {
					return object.NewError("wrong number of arguments. got=%d, want=2 or 3", len(args))
				}
				file, ok := args[0].(*object.FileValue)
				if !ok {
					return object.NewError("argument `file` must be FILE_VALUE, got %s", args[0].Type())
				}
				path, errObj := asString(args[1], "path")
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
				if result := saveBytesToPath(path, file.Data); object.IsError(result) {
					return result
				}
				saved := cloneFileValue(file)
				safePath, _ := SanitizePathLocal(path)
				saved.Path = safePath
				saved.Name = firstNonEmpty(optString(opts, "name"), filepath.Base(safePath), saved.Name)
				saved.Size = int64(len(saved.Data))
				applyFileOpts(saved, opts)
				return saved
			},
		},
		"file_text": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("wrong number of arguments. got=%d, want=1", len(args))
				}
				file, ok := args[0].(*object.FileValue)
				if !ok {
					return object.NewError("argument `file` must be FILE_VALUE, got %s", args[0].Type())
				}
				return &object.String{Value: string(file.Data)}
			},
		},
		"file_bytes": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("wrong number of arguments. got=%d, want=1", len(args))
				}
				file, ok := args[0].(*object.FileValue)
				if !ok {
					return object.NewError("argument `file` must be FILE_VALUE, got %s", args[0].Type())
				}
				return &object.String{Value: base64.StdEncoding.EncodeToString(file.Data)}
			},
		},
		"file_name": {
			Fn: func(args ...object.Object) object.Object {
				file, errObj := expectFileValueArg(args, "file_name")
				if errObj != nil {
					return errObj
				}
				return &object.String{Value: file.Name}
			},
		},
		"file_mime": {
			Fn: func(args ...object.Object) object.Object {
				file, errObj := expectFileValueArg(args, "file_mime")
				if errObj != nil {
					return errObj
				}
				return &object.String{Value: file.MIME}
			},
		},
		"file_size": {
			Fn: func(args ...object.Object) object.Object {
				file, errObj := expectFileValueArg(args, "file_size")
				if errObj != nil {
					return errObj
				}
				return object.IntegerObj(file.Size)
			},
		},
		"file_copy": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return object.NewError("wrong number of arguments. got=%d, want=2", len(args))
				}
				srcPath, errObj := filePathArg(args[0], "src")
				if errObj != nil {
					return errObj
				}
				dstPath, errObj := asString(args[1], "dst")
				if errObj != nil {
					return errObj
				}
				return copyOrMovePath(srcPath, dstPath, false)
			},
		},
		"file_move": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return object.NewError("wrong number of arguments. got=%d, want=2", len(args))
				}
				srcPath, errObj := filePathArg(args[0], "src")
				if errObj != nil {
					return errObj
				}
				dstPath, errObj := asString(args[1], "dst")
				if errObj != nil {
					return errObj
				}
				return copyOrMovePath(srcPath, dstPath, true)
			},
		},
		"file_rename": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return object.NewError("wrong number of arguments. got=%d, want=2", len(args))
				}
				srcPath, errObj := filePathArg(args[0], "path")
				if errObj != nil {
					return errObj
				}
				newName, errObj := asString(args[1], "new_name")
				if errObj != nil {
					return errObj
				}
				newName = strings.TrimSpace(newName)
				if newName == "" || filepath.Base(newName) != newName || strings.ContainsRune(newName, os.PathSeparator) {
					return object.NewError("new_name must be a base filename")
				}
				return copyOrMovePath(srcPath, filepath.Join(filepath.Dir(srcPath), newName), true)
			},
		},
	})
}

func expectFileValueArg(args []object.Object, name string) (*object.FileValue, object.Object) {
	if len(args) != 1 {
		return nil, object.NewError("%s expects 1 argument, got %d", name, len(args))
	}
	file, ok := args[0].(*object.FileValue)
	if !ok {
		return nil, object.NewError("argument `file` must be FILE_VALUE, got %s", args[0].Type())
	}
	return file, nil
}

func filePathArg(arg object.Object, name string) (string, object.Object) {
	switch v := arg.(type) {
	case *object.String:
		safePath, err := SanitizePathLocal(v.Value)
		if err != nil {
			return "", object.NewError("%s", err)
		}
		return safePath, nil
	case *object.FileValue:
		if strings.TrimSpace(v.Path) == "" {
			return "", object.NewError("argument `%s` must reference a saved file path", name)
		}
		safePath, err := SanitizePathLocal(v.Path)
		if err != nil {
			return "", object.NewError("%s", err)
		}
		return safePath, nil
	default:
		return "", object.NewError("argument `%s` must be STRING or FILE_VALUE, got %s", name, arg.Type())
	}
}

func copyOrMovePath(srcPath, dstPath string, move bool) object.Object {
	safeSrc, err := SanitizePathLocal(srcPath)
	if err != nil {
		return object.NewError("%s", err)
	}
	safeDst, err := SanitizePathLocal(dstPath)
	if err != nil {
		return object.NewError("%s", err)
	}
	if err := security.CheckFileReadAllowed(safeSrc); err != nil {
		return object.NewError("%s", err)
	}
	if err := security.CheckFileWriteAllowed(safeDst); err != nil {
		return object.NewError("%s", err)
	}
	data, err := os.ReadFile(safeSrc)
	if err != nil {
		return object.NewError("%s", err)
	}
	if err := os.MkdirAll(filepath.Dir(safeDst), 0o755); err != nil {
		return object.NewError("%s", err)
	}
	if err := os.WriteFile(safeDst, data, 0o644); err != nil {
		return object.NewError("%s", err)
	}
	if move {
		if err := security.CheckFileWriteAllowed(safeSrc); err != nil {
			return object.NewError("%s", err)
		}
		if err := os.Remove(safeSrc); err != nil {
			return object.NewError("%s", err)
		}
	}
	return &object.String{Value: safeDst}
}
