package uiapp

import (
	"github.com/egoist/mygo/ui"
	"github.com/lizhian/agent-manager/internal/skills"
	"github.com/lizhian/agent-manager/internal/storage"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSkillsViewSearchAndDocumentNavigation(t *testing.T) {
	home := t.TempDir()
	store, err := storage.Open(storage.PathsFromHome(home))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.DB.Exec(`INSERT INTO skill_sources VALUES(1,'owner/repo','owner_repo','','');INSERT INTO skills VALUES(1,1,'demo','中文描述',1,'','')`)
	dir := filepath.Join(home, ".agent-manager/skills/owner_repo/demo")
	os.MkdirAll(dir, 0700)
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# 示例\n\n[指南](guide.md)"), 0600)
	os.WriteFile(filepath.Join(dir, "guide.md"), []byte("# 使用指南"), 0600)
	w := NewSkillsWorkspace(skills.New(store, skills.Options{}))
	queue := make(chan func(), 10)
	w.Load(func(f func()) { queue <- f })
	wait := func() {
		t.Helper()
		select {
		case f := <-queue:
			f()
		case <-time.After(3 * time.Second):
			t.Fatal("load timeout")
		}
	}
	wait()
	wait()
	tt := ui.NewTester(w.View, 960, 600)
	if !tt.HasText("示例") {
		t.Fatal("detail not rendered")
	}
	if err = tt.Click("搜索 Skills"); err != nil {
		t.Fatal(err)
	}
	tt.Type("找不到")
	if !tt.HasText("没有匹配结果") {
		t.Fatal("no empty feedback")
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("中文描述")
	if !tt.HasText("demo") {
		t.Fatal("description search failed")
	}
	w.followLink("guide.md", "demo/SKILL.md")
	wait()
	tt.Frame()
	if !tt.HasText("使用指南") {
		t.Fatal("associated document missing")
	}
}
