//go:build !darwin && !linux

package updater

import "errors"

func Relaunch(args []string) error {
	return errors.New("automatic relaunch is only supported on macOS and Linux")
}
