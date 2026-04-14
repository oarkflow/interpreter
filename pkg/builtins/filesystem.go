package builtins

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/security"
)

func init() {
	eval.RegisterBuiltins(map[string]*object.Builtin{
		// readdir(path) — returns array of filenames in the directory.
		"readdir": {
			Fn: func(args ...object.Object) object.Object {
				retErr := func(msg string) object.Object {
					return &object.Array{Elements: []object.Object{object.NULL, &object.String{Value: msg}}}
				}
				if len(args) != 1 {
					return retErr(fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args)))
				}
				path, errObj := asString(args[0], "path")
				if errObj != nil {
					return errObj
				}
				safePath, err := SanitizePathLocal(path)
				if err != nil {
					return retErr(fmt.Sprintf("%s", err))
				}
				if err := security.CheckFileReadAllowed(safePath); err != nil {
					return retErr(fmt.Sprintf("%s", err))
				}
				entries, err := os.ReadDir(safePath)
				if err != nil {
					return retErr(fmt.Sprintf("%s", err))
				}
				elems := make([]object.Object, 0, len(entries))
				for _, e := range entries {
					elems = append(elems, &object.String{Value: e.Name()})
				}
				return &object.Array{Elements: []object.Object{
					&object.Array{Elements: elems},
					object.NULL,
				}}
			},
		},

		// glob(pattern) — returns array of file paths matching the glob pattern.
		"glob": {
			Fn: func(args ...object.Object) object.Object {
				retErr := func(msg string) object.Object {
					return &object.Array{Elements: []object.Object{object.NULL, &object.String{Value: msg}}}
				}
				if len(args) != 1 {
					return retErr(fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args)))
				}
				pattern, errObj := asString(args[0], "pattern")
				if errObj != nil {
					return errObj
				}
				matches, err := filepath.Glob(pattern)
				if err != nil {
					return retErr(fmt.Sprintf("%s", err))
				}
				elems := make([]object.Object, 0, len(matches))
				for _, m := range matches {
					elems = append(elems, &object.String{Value: m})
				}
				return &object.Array{Elements: []object.Object{
					&object.Array{Elements: elems},
					object.NULL,
				}}
			},
		},

		// mkdir(path) or mkdir(path, permissions)
		// Creates directory (and parents). Returns [true, null] or [false, error].
		"mkdir": {
			Fn: func(args ...object.Object) object.Object {
				retErr := func(msg string) object.Object {
					return &object.Array{Elements: []object.Object{object.FALSE, &object.String{Value: msg}}}
				}
				retOk := func() object.Object {
					return &object.Array{Elements: []object.Object{object.TRUE, object.NULL}}
				}
				if len(args) < 1 || len(args) > 2 {
					return retErr(fmt.Sprintf("wrong number of arguments. got=%d, want=1 or 2", len(args)))
				}
				path, errObj := asString(args[0], "path")
				if errObj != nil {
					return errObj
				}
				safePath, err := SanitizePathLocal(path)
				if err != nil {
					return retErr(fmt.Sprintf("%s", err))
				}
				if err := security.CheckFileWriteAllowed(safePath); err != nil {
					return retErr(fmt.Sprintf("%s", err))
				}
				perm := os.FileMode(0o755)
				if len(args) == 2 {
					p, errObj := asInt(args[1], "permissions")
					if errObj != nil {
						return errObj
					}
					perm = os.FileMode(p)
				}
				if err := os.MkdirAll(safePath, perm); err != nil {
					return retErr(fmt.Sprintf("%s", err))
				}
				return retOk()
			},
		},

		// rmdir(path) — removes an empty directory. Returns [true, null] or [false, error].
		"rmdir": {
			Fn: func(args ...object.Object) object.Object {
				retErr := func(msg string) object.Object {
					return &object.Array{Elements: []object.Object{object.FALSE, &object.String{Value: msg}}}
				}
				retOk := func() object.Object {
					return &object.Array{Elements: []object.Object{object.TRUE, object.NULL}}
				}
				if len(args) != 1 {
					return retErr(fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args)))
				}
				path, errObj := asString(args[0], "path")
				if errObj != nil {
					return errObj
				}
				safePath, err := SanitizePathLocal(path)
				if err != nil {
					return retErr(fmt.Sprintf("%s", err))
				}
				if err := security.CheckFileWriteAllowed(safePath); err != nil {
					return retErr(fmt.Sprintf("%s", err))
				}
				if err := os.Remove(safePath); err != nil {
					return retErr(fmt.Sprintf("%s", err))
				}
				return retOk()
			},
		},

		// stat(path) — returns a hash with file info: name, size, mode, mod_time, is_dir.
		"stat": {
			Fn: func(args ...object.Object) object.Object {
				retErr := func(msg string) object.Object {
					return &object.Array{Elements: []object.Object{object.NULL, &object.String{Value: msg}}}
				}
				if len(args) != 1 {
					return retErr(fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args)))
				}
				path, errObj := asString(args[0], "path")
				if errObj != nil {
					return errObj
				}
				safePath, err := SanitizePathLocal(path)
				if err != nil {
					return retErr(fmt.Sprintf("%s", err))
				}
				if err := security.CheckFileReadAllowed(safePath); err != nil {
					return retErr(fmt.Sprintf("%s", err))
				}
				info, err := os.Stat(safePath)
				if err != nil {
					return retErr(fmt.Sprintf("%s", err))
				}
				pairs := make(map[object.HashKey]object.HashPair)
				setStr := func(k, v string) {
					key := &object.String{Value: k}
					pairs[key.HashKey()] = object.HashPair{Key: key, Value: &object.String{Value: v}}
				}
				setInt := func(k string, v int64) {
					key := &object.String{Value: k}
					pairs[key.HashKey()] = object.HashPair{Key: key, Value: &object.Integer{Value: v}}
				}
				setBool := func(k string, v bool) {
					key := &object.String{Value: k}
					pairs[key.HashKey()] = object.HashPair{Key: key, Value: object.NativeBoolToBooleanObject(v)}
				}
				setStr("name", info.Name())
				setInt("size", info.Size())
				setStr("mode", fmt.Sprintf("%04o", info.Mode().Perm()))
				setInt("mod_time", info.ModTime().Unix())
				setBool("is_dir", info.IsDir())
				return &object.Array{Elements: []object.Object{
					&object.Hash{Pairs: pairs},
					object.NULL,
				}}
			},
		},

		// chmod(path, mode) — changes file permissions. mode is an integer (e.g. 0o755).
		"chmod": {
			Fn: func(args ...object.Object) object.Object {
				retErr := func(msg string) object.Object {
					return &object.Array{Elements: []object.Object{object.FALSE, &object.String{Value: msg}}}
				}
				retOk := func() object.Object {
					return &object.Array{Elements: []object.Object{object.TRUE, object.NULL}}
				}
				if len(args) != 2 {
					return retErr(fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args)))
				}
				path, errObj := asString(args[0], "path")
				if errObj != nil {
					return errObj
				}
				mode, errObj := asInt(args[1], "mode")
				if errObj != nil {
					return errObj
				}
				safePath, err := SanitizePathLocal(path)
				if err != nil {
					return retErr(fmt.Sprintf("%s", err))
				}
				if err := security.CheckFileWriteAllowed(safePath); err != nil {
					return retErr(fmt.Sprintf("%s", err))
				}
				if err := os.Chmod(safePath, os.FileMode(mode)); err != nil {
					return retErr(fmt.Sprintf("%s", err))
				}
				return retOk()
			},
		},

		// basename(path) — returns the last element of the path.
		"basename": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("basename() takes 1 argument, got %d", len(args))
				}
				path, errObj := asString(args[0], "path")
				if errObj != nil {
					return errObj
				}
				return &object.String{Value: filepath.Base(path)}
			},
		},

		// dirname(path) — returns all but the last element of the path.
		"dirname": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("dirname() takes 1 argument, got %d", len(args))
				}
				path, errObj := asString(args[0], "path")
				if errObj != nil {
					return errObj
				}
				return &object.String{Value: filepath.Dir(path)}
			},
		},

		// path_join(parts...) — joins path segments.
		"path_join": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) == 0 {
					return object.NewError("path_join() requires at least 1 argument")
				}
				parts := make([]string, 0, len(args))
				for i, arg := range args {
					s, errObj := asString(arg, fmt.Sprintf("part[%d]", i))
					if errObj != nil {
						return errObj
					}
					parts = append(parts, s)
				}
				return &object.String{Value: filepath.Join(parts...)}
			},
		},
	})
}
