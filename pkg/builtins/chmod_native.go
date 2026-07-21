//go:build !js

package builtins

import "os"

func chmodPath(path string, mode os.FileMode) error {
	return os.Chmod(path, mode)
}
