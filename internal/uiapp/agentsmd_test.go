package uiapp_test

import (
	"fmt"
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

func TestSearchFilterAndDragPreserveSelectedFragment(t *testing.T) {
	store, err := storage.Open(storage.PathsFromDataDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := agentsmd.New(store, agentsmd.Options{})
	for _, name := range []string{"First", "Second", "Third"} {
		if _, err = service.Create(name, "# "+name); err != nil {
			t.Fatal(err)
		}
	}
	app := uiapp.NewApp(service)
	updates := make(chan func(), 8)
	app.Start(func(fn func()) { updates <- fn })
	tt := ui.NewTester(app.View, 960, 640)
	flush := func() {
		t.Helper()
		select {
		case fn := <-updates:
			fn()
			tt.Frame()
		case <-time.After(3 * time.Second):
			t.Fatal("async operation incomplete")
		}
	}
	flush()
	if err = tt.Click("Second"); err != nil {
		t.Fatal(err)
	}
	if err = tt.Click("搜索片段"); err != nil {
		t.Fatal(err)
	}
	tt.Type("First")
	if tt.HasText("Third") {
		t.Fatal("search failed to filter list")
	}
	from, _ := tt.Find("拖动排序 First")
	tt.Press(from.X+from.W/2, from.Y+from.H/2)
	tt.Move(100, 620)
	tt.Release(100, 620)
	snap, err := service.Snapshot()
	if err != nil || snap.Fragments[0].Title != "First" {
		t.Fatal("search changed order")
	}
	if err = tt.Click("搜索片段"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Key(0, ui.KeyBackspace)
	from, _ = tt.Find("拖动排序 First")
	to, _ := tt.Find("Third")
	tt.Press(from.X+from.W/2, from.Y+from.H/2)
	tt.Move(to.X+to.W/2, to.Y+2)
	tt.Release(to.X+to.W/2, to.Y+2)
	flush()
	snap, err = service.Snapshot()
	if err != nil || snap.Fragments[0].Title != "Second" || snap.Fragments[1].Title != "First" {
		t.Fatal(snap, err)
	}
	if err = tt.Click("片段名称"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Key(ui.Cmd, ui.KeyC)
	if tt.Clipboard() != "Second" {
		t.Fatal("search or drag changed selection")
	}
}

func TestPersistentListEdgeScrollAndEscapeCancelDrag(t *testing.T) {
	store, err := storage.Open(storage.PathsFromDataDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := agentsmd.New(store, agentsmd.Options{})
	for i := 1; i <= 40; i++ {
		if _, err = service.Create(fmt.Sprintf("Fragment %02d", i), "# Content"); err != nil {
			t.Fatal(err)
		}
	}
	app := uiapp.NewApp(service)
	updates := make(chan func(), 8)
	app.Start(func(fn func()) { updates <- fn })
	tt := ui.NewTester(app.View, 960, 640)
	select {
	case fn := <-updates:
		fn()
		tt.Frame()
	case <-time.After(3 * time.Second):
		t.Fatal("load timeout")
	}
	from, _ := tt.Find("拖动排序 Fragment 01")
	to, _ := tt.Find("Fragment 03")
	tt.Press(from.X+from.W/2, from.Y+from.H/2)
	tt.Move(to.X+to.W/2, to.Y+to.H-2)
	tt.Key(0, ui.KeyEscape)
	tt.Release(to.X+to.W/2, to.Y+to.H-2)
	select {
	case <-updates:
		t.Fatal("Escape committed cancelled drag")
	default:
	}
	from, _ = tt.Find("拖动排序 Fragment 01")
	tt.Press(from.X+from.W/2, from.Y+from.H/2)
	tt.Move(100, 620)
	tt.Advance(2 * time.Second)
	to, ok := tt.Find("Fragment 30")
	if !ok || to.Y < 100 || to.Y+to.H > 632 {
		t.Fatalf("edge drag did not reveal target: %+v %v", to, ok)
	}
	tt.Move(to.X+to.W/2, to.Y+2)
	tt.Release(to.X+to.W/2, to.Y+2)
	select {
	case fn := <-updates:
		fn()
		tt.Frame()
	case <-time.After(3 * time.Second):
		t.Fatal("reorder timeout")
	}
	snap, err := service.Snapshot()
	if err != nil || snap.Fragments[28].ID != 1 || snap.Fragments[29].ID != 30 {
		t.Fatal(snap, err)
	}
}
