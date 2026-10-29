//go:build unix

package mcp

import (
	"errors"
	"os/exec"
	"syscall"
)

func ownProcessGroup(command *exec.Cmd) (func(), error) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	kill := func() error {
		if command.Process == nil {
			return nil
		}
		err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return err
	}
	command.Cancel = kill
	return func() { _ = kill() }, nil
}
