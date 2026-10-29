package uiapp

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/egoist/mygo/ui"
	"github.com/lizhian/agent-manager/internal/skills"
	"github.com/lizhian/agent-manager/internal/storage"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDeleteSourceConfirmationCancelAndFailureRetainContext(t *testing.T) {
	store, err := storage.Open(storage.PathsFromHome(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.DB.Exec(`INSERT INTO skill_sources VALUES(1,'owner/repo','owner_repo','','');INSERT INTO skills VALUES(1,1,'demo','',1,'','')`)
	dir := filepath.Join(store.Paths.Root, "skills/owner_repo/demo")
	os.MkdirAll(dir, 0700)
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# Demo"), 0600)
	w := NewSkillsWorkspace(skills.New(store, skills.Options{}))
	queue := make(chan func(), 20)
	w.Load(func(f func()) { queue <- f })
	wait := func() {
		t.Helper()
		select {
		case f := <-queue:
			f()
		case <-time.After(time.Second):
			t.Fatal("timeout")
		}
	}
	wait()
	wait()
	tt := ui.NewTester(w.View, 960, 600)
	if err = tt.Click("删除来源 owner/repo"); err != nil {
		t.Fatal(err)
	}
	wait()
	tt.Frame()
	if !tt.HasText("1 个 Skills") || !tt.HasText("1 个全局启用") {
		t.Fatal("missing impact")
	}
	if err = tt.Click("取消删除"); err != nil {
		t.Fatal(err)
	}
	if len(w.snapshot.Sources) != 1 {
		t.Fatal("cancel changed data")
	}
	links := filepath.Join(store.Paths.AgentsDir, "skills")
	os.MkdirAll(links, 0700)
	os.WriteFile(filepath.Join(links, "owner_repo__demo"), []byte("manual"), 0600)
	tt.Click("删除来源 owner/repo")
	wait()
	tt.Frame()
	tt.Click("确认删除来源")
	wait()
	tt.Frame()
	if !w.deleteSourceOpen || !tt.HasText("手工内容") {
		t.Fatal("failure lost dialog/context")
	}
}
func TestBatchSummaryKeepsFailedSourceVisible(t *testing.T) {
	store, err := storage.Open(storage.PathsFromHome(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.DB.Exec(`INSERT INTO skill_sources VALUES(1,'a/repo','a_repo','',''),(2,'b/repo','b_repo','','');INSERT INTO skills VALUES(1,1,'demo','',0,'',''),(2,2,'demo','',0,'','')`)
	for _, safe := range []string{"a_repo", "b_repo"} {
		dir := filepath.Join(store.Paths.Root, "skills", safe, "demo")
		os.MkdirAll(dir, 0700)
		os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# old"), 0600)
	}
	s := skills.New(store, skills.Options{Runner: func(ctx context.Context, dir string, args []string) ([]byte, error) {
		if args[0] == "add" {
			if args[1] == "b/repo" {
				return nil, fmt.Errorf("fake batch failure")
			}
			return nil, nil
		}
		p := filepath.Join(dir, "demo")
		os.MkdirAll(p, 0700)
		os.WriteFile(filepath.Join(p, "SKILL.md"), []byte("# new"), 0600)
		return json.Marshal([]map[string]string{{"name": "demo", "path": p}})
	}})
	w := NewSkillsWorkspace(s)
	queue := make(chan func(), 20)
	w.Load(func(f func()) { queue <- f })
	wait := func() {
		t.Helper()
		select {
		case f := <-queue:
			f()
		case <-time.After(time.Second):
			t.Fatal("timeout")
		}
	}
	wait()
	wait()
	tt := ui.NewTester(w.View, 960, 600)
	if err = tt.Click("全部更新"); err != nil {
		t.Fatal(err)
	}
	for w.writing != "" || w.detailLoading {
		wait()
	}
	tt.Frame()
	if !tt.HasText("成功 1，失败 1") || !tt.HasText("fake batch failure") || len(w.snapshot.Sources) != 2 {
		t.Fatal("partial result missing", w.progress, w.err)
	}
}
