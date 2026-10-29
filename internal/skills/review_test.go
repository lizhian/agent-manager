package skills_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/lizhian/agent-manager/internal/skills"
	"github.com/lizhian/agent-manager/internal/storage"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestEmptyUpstreamUpdateRemovesLastEnabledSkill(t *testing.T) {
	store, err := storage.Open(storage.PathsFromDataDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	empty := false
	service := skills.New(store, skills.Options{Runner: func(ctx context.Context, dir string, args []string) ([]byte, error) {
		if args[0] == "add" {
			return nil, nil
		}
		if empty {
			return []byte("[]"), nil
		}
		p := filepath.Join(dir, "demo")
		os.Mkdir(p, 0700)
		os.WriteFile(filepath.Join(p, "SKILL.md"), []byte("# demo"), 0600)
		return json.Marshal([]map[string]string{{"name": "demo", "path": p}})
	}})
	if _, err = service.SyncSource(context.Background(), "owner/repo", nil); err != nil {
		t.Fatal(err)
	}
	if _, err = service.SetGlobalEnabled("owner_repo", "demo", true); err != nil {
		t.Fatal(err)
	}
	empty = true
	result, err := service.SyncSource(context.Background(), "owner/repo", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.RemovedCount != 1 || len(result.Snapshot.Sources) != 1 || len(result.Snapshot.Sources[0].Skills) != 0 {
		t.Fatal(result)
	}
	if _, err = os.Lstat(filepath.Join(store.Paths.AgentsDir, "skills/owner_repo__demo")); !os.IsNotExist(err) {
		t.Fatal("stale activation remains", err)
	}
	if _, err = service.SyncSource(context.Background(), "new/empty", nil); err == nil {
		t.Fatal("initial empty install should remain an actionable error")
	}
}
func TestDefaultRunnerCancellationReleasesWrapperChildrenAndWriteLock(t *testing.T) {
	root := t.TempDir()
	store, err := storage.Open(storage.PathsFromDataDir(root))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	bin := filepath.Join(root, "bin")
	os.Mkdir(bin, 0700)
	pidfile := filepath.Join(root, "pids")
	shell := filepath.Join(bin, "shell")
	os.WriteFile(shell, []byte("#!/bin/sh\nprintf '__AGENT_MANAGER_PATH_START__"+bin+":/bin:/usr/bin__AGENT_MANAGER_PATH_END__'\n"), 0700)
	os.WriteFile(filepath.Join(bin, "npx"), []byte("#!/bin/sh\nsleep 30 &\necho $$ $! > '"+pidfile+"'\nwait\n"), 0700)
	t.Setenv("SHELL", shell)
	service := skills.New(store, skills.Options{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := service.SyncSource(ctx, "owner/repo", nil); done <- err }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err = os.Stat(pidfile); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fixture never started")
		}
		time.Sleep(10 * time.Millisecond)
	}
	data, _ := os.ReadFile(pidfile)
	var pids []int
	for _, field := range strings.Fields(string(data)) {
		pid, _ := strconv.Atoi(field)
		pids = append(pids, pid)
	}
	defer func() {
		for _, pid := range pids {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}()
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancel succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("cancel blocked on child-held pipes")
	}
	for _, pid := range pids {
		deadline = time.Now().Add(2 * time.Second)
		for {
			if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("owned process remains", pid)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	// A second operation reaches the runner rather than remaining stuck on writeMu.
	os.WriteFile(filepath.Join(bin, "npx"), []byte("#!/bin/sh\nexit 7\n"), 0700)
	_, err = service.SyncSource(context.Background(), "owner/repo", nil)
	if err == nil || !strings.Contains(err.Error(), "7") {
		t.Fatal(fmt.Sprint(err))
	}
}
