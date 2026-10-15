//go:build upgrade && unix

package upgrade

import (
	"os"
	"syscall"
)

// restart runs the program at path in this process's place (same process, same arguments); it
// only returns when that failed
func restart(path string) error {
	return syscall.Exec(path, os.Args, os.Environ())
}
