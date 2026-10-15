//go:build upgrade && !unix

package upgrade

import "errors"

var restart = func(string) error {
	return errors.New("restarting in place needs Linux")
}
