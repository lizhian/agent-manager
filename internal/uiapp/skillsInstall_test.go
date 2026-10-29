package uiapp

import (
	"context"
	"encoding/json"
	"github.com/egoist/mygo/ui"
	"github.com/lizhian/agent-manager/internal/skills"
	"github.com/lizhian/agent-manager/internal/storage"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInstallSourceFormRefreshesCatalog(t *testing.T) {
	store, err := storage.Open(storage.PathsFromHome(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s := skills.New(store, skills.Options{Runner: func(ctx context.Context, dir string, args []string) ([]byte, error) {
		if args[0] == "add" {
			return nil, nil
		}
		p := filepath.Join(dir, "demo")
		os.MkdirAll(p, 0700)
		os.WriteFile(filepath.Join(p, "SKILL.md"), []byte("# 安装正文"), 0600)
		return json.Marshal([]map[string]string{{"name": "demo", "path": p}})
	}})
	w := NewSkillsWorkspace(s)
	queue := make(chan func(), 20)
	w.Load(func(f func()) { queue <- f })
	select {
	case f := <-queue:
		f()
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
	tt := ui.NewTester(w.View, 960, 600)
	if err = tt.Click("安装来源"); err != nil {
		t.Fatal(err)
	}
	tt.Click("GitHub 来源")
	tt.Type("owner/repo")
	if err = tt.Click("开始安装"); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(3 * time.Second)
	for w.writing != "" || w.detailLoading {
		select {
		case f := <-queue:
			f()
		case <-deadline:
			t.Fatal("install timeout")
		}
	}
	tt.Frame()
	if !tt.HasText("demo") || !tt.HasText("安装正文") {
		t.Fatal("installed result absent")
	}
}
