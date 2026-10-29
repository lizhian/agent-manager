//go:build unix

package processenv

import (
	"errors"
	"os/exec"
	"syscall"
)

// OwnProcessGroup bounds cleanup of this command and its wrapper children.
func OwnProcessGroup(command *exec.Cmd) (func(), error) {
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
