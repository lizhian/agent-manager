package uiapp_test

import (
	"context"
	"github.com/egoist/mygo/ui"
	"github.com/lizhian/agent-manager/internal/mcp"
	"github.com/lizhian/agent-manager/internal/storage"
	"github.com/lizhian/agent-manager/internal/uiapp"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"os"
	"testing"
	"time"
)

func TestMCPUIDebugHelper(t *testing.T) {
	if os.Getenv("MCP_UI_HELPER") != "1" {
		return
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "ui-fixture", Version: "1"}, nil)
	sdk.AddTool(server, &sdk.Tool{Name: "echo", Description: "回声说明"}, func(ctx context.Context, req *sdk.CallToolRequest, in struct {
		Message string `json:"message"`
	}) (*sdk.CallToolResult, any, error) { return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: in.Message}}}, nil, nil })
	if err := server.Run(context.Background(), &sdk.StdioTransport{}); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
func TestMCPDebugUIRejectsStaleServiceResultsAndShowsCall(t *testing.T) {
	store, err := storage.Open(storage.PathsFromDataDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := mcp.New(store)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Create(mcp.Input{Name: "a-fixture", Transport: "stdio", Command: exe, Args: []string{"-test.run=^TestMCPUIDebugHelper$"}, Env: map[string]string{"MCP_UI_HELPER": "1"}}); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Create(mcp.Input{Name: "b-other", Transport: "stdio", Command: "missing-fixture"}); err != nil {
		t.Fatal(err)
	}
	w := uiapp.NewMCPWorkspace(service)
	updates := make(chan func(), 8)
	tt := ui.NewTester(w.View, 960, 640)
	flush := func() {
		t.Helper()
		select {
		case fn := <-updates:
			fn()
			tt.Frame()
		case <-time.After(5 * time.Second):
			t.Fatal("async timeout")
		}
	}
	click := func(name string) {
		t.Helper()
		if err := tt.Click(name); err != nil {
			t.Fatal(err)
		}
	}
	w.Load(func(fn func()) { updates <- fn })
	flush()
	click("工具调试")
	click("读取工具")
	click("b-other")
	flush()
	if tt.HasText("回声说明") {
		t.Fatal("stale tools crossed service selection")
	}
	click("a-fixture")
	click("读取工具")
	flush()
	if !tt.HasText("回声说明") {
		t.Fatal("missing tools")
	}
	click("工具调用参数")
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type(`{"message":"原生调试成功"}`)
	click("调用工具")
	flush()
	if !tt.HasText("调用结果") || !tt.HasText("原生调试成功") {
		t.Fatal("missing result")
	}
	click("工具调用参数")
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("[]")
	click("调用工具")
	flush()
	if !tt.HasText("参数必须是 JSON 对象") {
		t.Fatal("missing JSON feedback")
	}
}
