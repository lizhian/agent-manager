package mcp

import (
	"fmt"
	"os/exec"
)

func ownProcessGroup(command *exec.Cmd) (func(), error) {
	return nil, fmt.Errorf("当前 MCP 进程回收仅支持本轮 macOS 验收平台")
}
