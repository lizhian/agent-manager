package processenv_test

import (
	"context"
	"github.com/lizhian/agent-manager/internal/processenv"
	"os"
	"path/filepath"
	"testing"
)

func TestDesktopPathFindsShellToolAndPreservesInheritedEntries(t *testing.T) {
	root := t.TempDir()
	shell := filepath.Join(root, "shell")
	tool := filepath.Join(root, "fixture")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\nprintf '__AGENT_MANAGER_PATH_START__"+root+":/usr/bin__AGENT_MANAGER_PATH_END__'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tool, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", shell)
	path := processenv.ResolvePath(context.Background(), "/usr/bin:/inherited")
	if path != root+":/usr/bin:/inherited" {
		t.Fatal(path)
	}
	got, err := processenv.FindExecutable("fixture", path)
	if err != nil || got != tool {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if path := processenv.ResolvePath(ctx, "/fallback"); path != "/fallback" {
		t.Fatal(path)
	}
}
