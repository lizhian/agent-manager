package uiapp_test

import (
	"github.com/egoist/mygo/ui"
	"github.com/lizhian/agent-manager/internal/mcp"
	"github.com/lizhian/agent-manager/internal/storage"
	"github.com/lizhian/agent-manager/internal/uiapp"
	"testing"
	"time"
)

func TestMCPNativeCRUDAndFailedSaveKeepsDraft(t *testing.T) {
	store, err := storage.Open(storage.PathsFromDataDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := mcp.New(store)
	workspace := uiapp.NewMCPWorkspace(service)
	updates := make(chan func(), 8)
	tt := ui.NewTester(workspace.View, 960, 640)
	flush := func() {
		t.Helper()
		select {
		case update := <-updates:
			update()
			tt.Frame()
		case <-time.After(3 * time.Second):
			t.Fatal("async operation timed out")
		}
	}
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	workspace.Load(func(fn func()) { updates <- fn })
	flush()
	if !tt.HasText("还没有 MCP") {
		t.Fatal("missing empty state")
	}
	click("新增 MCP")
	tt.Type("本地测试")
	click("MCP 命令")
	tt.Type("fake")
	click("保存 MCP")
	flush()
	snap, err := service.Snapshot()
	if err != nil || len(snap.Servers) != 1 || snap.Servers[0].Name != "本地测试" {
		t.Fatal(snap, err)
	}
	click("MCP 名称")
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("未保存编辑")
	if _, err = store.DB.Exec(`CREATE TRIGGER fail_mcp_ui BEFORE UPDATE ON agent_mcp_servers BEGIN SELECT RAISE(FAIL,'injected write failure'); END`); err != nil {
		t.Fatal(err)
	}
	click("保存 MCP")
	flush()
	if !tt.HasText("injected write failure") {
		t.Fatal("missing error")
	}
	click("MCP 名称")
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Key(ui.Cmd, ui.KeyC)
	if tt.Clipboard() != "未保存编辑" {
		t.Fatal("lost failed draft")
	}
	click("删除 MCP")
	tt.Key(0, ui.KeyEscape)
	snap, err = service.Snapshot()
	if err != nil || len(snap.Servers) != 1 {
		t.Fatal("cancel deleted")
	}
	click("删除 MCP")
	click("确认删除 MCP")
	flush()
	snap, err = service.Snapshot()
	if err != nil || len(snap.Servers) != 0 {
		t.Fatal(snap, err)
	}
}
