// Package processenv resolves desktop commands without assuming the GUI inherited a login shell PATH.
package processenv

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func FindExecutable(program, path string) (string, error) {
	if strings.ContainsRune(program, filepath.Separator) {
		return program, nil
	}
	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			continue
		}
		file := filepath.Join(dir, program)
		if stat, err := os.Stat(file); err == nil && !stat.IsDir() && stat.Mode()&0111 != 0 {
			return file, nil
		}
	}
	return "", &exec.Error{Name: program, Err: exec.ErrNotFound}
}

// ResolvePath preserves inherited entries after the login shell's entries and falls back on failure.
func ResolvePath(ctx context.Context, inherited string) string {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	shell := os.Getenv("SHELL")
	if !filepath.IsAbs(shell) {
		shell = "/bin/zsh"
	}
	command := exec.CommandContext(ctx, shell, "-ilc", `printf '__AGENT_MANAGER_PATH_START__%s__AGENT_MANAGER_PATH_END__' "$PATH"`)
	release, err := OwnProcessGroup(command)
	if err != nil {
		return inherited
	}
	defer release()
	command.WaitDelay = 200 * time.Millisecond
	output, err := command.Output()
	if err != nil {
		return inherited
	}
	text := string(output)
	start := strings.LastIndex(text, "__AGENT_MANAGER_PATH_START__")
	if start < 0 {
		return inherited
	}
	text = text[start+len("__AGENT_MANAGER_PATH_START__"):]
	end := strings.Index(text, "__AGENT_MANAGER_PATH_END__")
	if end < 0 {
		return inherited
	}
	var paths []string
	seen := map[string]bool{}
	for _, entry := range filepath.SplitList(text[:end] + string(os.PathListSeparator) + inherited) {
		if entry != "" && !seen[entry] {
			paths = append(paths, entry)
			seen[entry] = true
		}
	}
	return strings.Join(paths, string(os.PathListSeparator))
}
