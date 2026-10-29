package uiapp_test

import (
	"github.com/egoist/mygo/ui"
	"github.com/lizhian/agent-manager/internal/agentsmd"
	"github.com/lizhian/agent-manager/internal/storage"
	"github.com/lizhian/agent-manager/internal/uiapp"
	"testing"
	"time"
)

func TestNativeFragmentWorkflowAndFailedSaveKeepsDraft(t *testing.T) {
	store, err := storage.Open(storage.PathsFromDataDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := agentsmd.New(store, agentsmd.Options{})
	app := uiapp.NewApp(service)
	updates := make(chan func(), 8)
	tt := ui.NewTester(app.View, 960, 640)
	flush := func() {
		t.Helper()
		select {
		case update := <-updates:
			update()
			tt.Frame()
		case <-time.After(3 * time.Second):
			t.Fatal("async operation did not finish")
		}
	}
	app.Start(func(fn func()) { updates <- fn })
	flush()
	if !tt.HasText("还没有片段") {
		t.Fatal("missing empty feedback")
	}
	if err = tt.Click("新增片段"); err != nil {
		t.Fatal(err)
	}
	if !tt.Focused("片段名称") {
		t.Fatal("new did not focus name")
	}
	tt.Type("中文片段")
	if err = tt.Click("Markdown 正文"); err != nil {
		t.Fatal(err)
	}
	tt.Type("# 中文正文")
	if err = tt.Click("保存片段"); err != nil {
		t.Fatal(err)
	}
	flush()
	snap, err := service.Snapshot()
	if err != nil || len(snap.Fragments) != 1 || snap.Fragments[0].Enabled {
		t.Fatal(snap, err)
	}
	if err = tt.Click("启用 中文片段"); err != nil {
		t.Fatal(err)
	}
	flush()
	snap, err = service.Snapshot()
	if err != nil || !snap.Fragments[0].Enabled {
		t.Fatal(snap, err)
	}
	if err = tt.Click("Markdown 正文"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("# 编辑未保存")
	if _, err = store.DB.Exec("CREATE TRIGGER reject_save BEFORE UPDATE ON agents_md_fragments BEGIN SELECT RAISE(ABORT,'disk unavailable'); END;"); err != nil {
		t.Fatal(err)
	}
	if err = tt.Click("保存片段"); err != nil {
		t.Fatal(err)
	}
	flush()
	if !tt.HasText("disk unavailable") {
		t.Fatal("missing actionable error")
	}
	if err = tt.Click("Markdown 正文"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Key(ui.Cmd, ui.KeyC)
	if tt.Clipboard() != "# 编辑未保存" {
		t.Fatal("failed save lost draft")
	}
	if _, err = store.DB.Exec("DROP TRIGGER reject_save"); err != nil {
		t.Fatal(err)
	}
	if err = tt.Click("删除片段"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEscape)
	snap, err = service.Snapshot()
	if err != nil || len(snap.Fragments) != 1 {
		t.Fatal("cancel deleted fragment")
	}
	if err = tt.Click("删除片段"); err != nil {
		t.Fatal(err)
	}
	if err = tt.Click("删除"); err != nil {
		t.Fatal(err)
	}
	flush()
	snap, err = service.Snapshot()
	if err != nil || len(snap.Fragments) != 0 {
		t.Fatal(snap, err)
	}
}
