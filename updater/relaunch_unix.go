//go:build darwin || linux

package updater

import (
	"os"
	"syscall"
)

func Relaunch(args []string) error {
	executablePath, err := os.Executable()
	if err != nil {
		return err
	}
	return syscall.Exec(executablePath, args, os.Environ())
}
