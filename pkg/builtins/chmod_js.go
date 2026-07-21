//go:build js

package builtins

import (
	"fmt"
	"os"
)

func chmodPath(path string, mode os.FileMode) error {
	return fmt.Errorf("chmod is not supported on js/wasm")
}
