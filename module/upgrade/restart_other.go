//go:build upgrade && !unix

package upgrade

import "errors"

func restart(string) error {
	return errors.New("restarting in place needs Linux")
}
