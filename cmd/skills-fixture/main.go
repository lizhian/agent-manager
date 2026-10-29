// skills-fixture is a local, disposable stand-in for npx for UI validation.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	args := os.Args[1:]
	if len(args) < 3 || args[0] != "--yes" || args[1] != "skills" {
		fmt.Fprintln(os.Stderr, "fixture expects npx --yes skills")
		os.Exit(2)
	}
	mode := "1"
	if p := os.Getenv("AGENT_MANAGER_SKILLS_FIXTURE_MODE"); p != "" {
		if b, e := os.ReadFile(p); e == nil {
			mode = strings.TrimSpace(string(b))
		}
	}
	if mode == "fail" {
		fmt.Fprintln(os.Stderr, "本地验证：模拟 npx 来源下载失败")
		os.Exit(3)
	}
	cwd, e := os.Getwd()
	if e != nil {
		panic(e)
	}
	names := []string{"keep", "gone"}
	if mode == "2" {
		names = []string{"keep", "new"}
	}
	switch args[2] {
	case "add":
		time.Sleep(2 * time.Second)
		for _, name := range names {
			p := filepath.Join(cwd, name)
			if e = os.MkdirAll(filepath.Join(p, "references"), 0700); e != nil {
				panic(e)
			}
			body := fmt.Sprintf("---\nname: %s\ndescription: 本地验证版本 %s\n---\n# %s\n\n这是受控安装替身。\n\n[关联指南](references/guide.md)\n", name, mode, name)
			if e = os.WriteFile(filepath.Join(p, "SKILL.md"), []byte(body), 0600); e != nil {
				panic(e)
			}
			os.WriteFile(filepath.Join(p, "references/guide.md"), []byte("# 本地指南\n\n仅操作当前临时目录。"), 0600)
		}
	case "list":
		var listed []map[string]string
		for _, name := range names {
			listed = append(listed, map[string]string{"name": name, "path": filepath.Join(cwd, name)})
		}
		json.NewEncoder(os.Stdout).Encode(listed)
	default:
		fmt.Fprintln(os.Stderr, "fixture supports add/list only")
		os.Exit(2)
	}
}
