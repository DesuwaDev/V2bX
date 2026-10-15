//go:build upgrade && unix

package upgrade

import (
	"os"
	"syscall"
)

// restart runs the program at path in this process's place (same process, same arguments); it
// only returns when that failed (a variable for the tests)
var restart = func(path string) error {
	return syscall.Exec(path, os.Args, os.Environ())
}
