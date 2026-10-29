//go:build !unix

package processenv

import (
	"fmt"
	"os/exec"
)

func OwnProcessGroup(command *exec.Cmd) (func(), error) {
	return nil, fmt.Errorf("当前平台不支持受管进程组")
}
