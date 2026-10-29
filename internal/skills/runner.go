package skills

import (
	"context"
	"fmt"
	"github.com/lizhian/agent-manager/internal/processenv"
	"os"
	"os/exec"
	"strings"
	"time"
)

func runSkills(ctx context.Context, dir string, args []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	path := processenv.ResolvePath(ctx, os.Getenv("PATH"))
	npx, err := processenv.FindExecutable("npx", path)
	if err != nil {
		return nil, fmt.Errorf("找不到 npx，请安装 Node.js 并确保用户 PATH 包含 npx：%w", err)
	}
	cmd := exec.CommandContext(ctx, npx, append([]string{"--yes", "skills"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+path)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	data, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("npx skills %s 失败：%w：%s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return data, nil
}
