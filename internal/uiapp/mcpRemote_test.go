package uiapp_test

import (
	"context"
	"github.com/egoist/mygo/ui"
	"github.com/lizhian/agent-manager/internal/mcp"
	"github.com/lizhian/agent-manager/internal/storage"
	"github.com/lizhian/agent-manager/internal/uiapp"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRemoteUITransportSwitchRejectsOldToolsAndCallsSSE(t *testing.T) {
	newServer := func(kind string) *sdk.Server {
		server := sdk.NewServer(&sdk.Implementation{Name: kind, Version: "1"}, nil)
		sdk.AddTool(server, &sdk.Tool{Name: "echo-" + kind, Description: kind + " 工具说明"}, func(ctx context.Context, req *sdk.CallToolRequest, input struct {
			Message string `json:"message"`
		}) (*sdk.CallToolResult, any, error) {
			return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: kind + input.Message}}}, nil, nil
		})
		return server
	}
	mux := http.NewServeMux()
	httpServer, sseServer := newServer("HTTP"), newServer("SSE")
	mux.Handle("/mcp", sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return httpServer }, nil))
	mux.Handle("/sse", sdk.NewSSEHandler(func(*http.Request) *sdk.Server { return sseServer }, nil))
	remote := httptest.NewServer(mux)
	defer remote.Close()
	store, err := storage.Open(storage.PathsFromDataDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := mcp.New(store)
	for _, input := range []mcp.Input{{Name: "a-http", Transport: "http", URL: remote.URL + "/mcp"}, {Name: "b-sse", Transport: "sse", URL: remote.URL + "/sse"}} {
		if _, err = service.Create(input); err != nil {
			t.Fatal(err)
		}
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
	click("b-sse")
	flush()
	if tt.HasText("HTTP 工具说明") {
		t.Fatal("old HTTP tools crossed transport")
	}
	click("读取工具")
	flush()
	if !tt.HasText("SSE 工具说明") {
		t.Fatal("missing SSE tools")
	}
	click("工具调用参数")
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type(`{"message":"本地成功"}`)
	click("调用工具")
	flush()
	if !tt.HasText("SSE本地成功") {
		t.Fatal("missing SSE call result")
	}
	click("a-http")
	if tt.HasText("SSE本地成功") {
		t.Fatal("old call result crossed service")
	}
	click("读取工具")
	flush()
	if !tt.HasText("HTTP 工具说明") {
		t.Fatal("missing HTTP tools")
	}
}
