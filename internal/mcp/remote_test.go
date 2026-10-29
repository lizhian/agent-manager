package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lizhian/agent-manager/internal/mcp"
	"github.com/lizhian/agent-manager/internal/storage"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTTPAndSSEUseSavedHeadersAndCloseConnections(t *testing.T) {
	for _, transport := range []string{"http", "sse"} {
		t.Run(transport, func(t *testing.T) {
			var requests, wrongHeaders, deletes, active atomic.Int32
			server := sdk.NewServer(&sdk.Implementation{Name: "remote-fixture", Version: "1"}, nil)
			sdk.AddTool(server, &sdk.Tool{Name: "echo", Description: "远端回声"}, func(ctx context.Context, req *sdk.CallToolRequest, input struct {
				Message string `json:"message"`
			}) (*sdk.CallToolResult, any, error) {
				if input.Message == "hang" {
					select {
					case <-ctx.Done():
					case <-time.After(2 * time.Second):
					}
					return nil, nil, fmt.Errorf("cancelled fixture")
				}
				return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: input.Message}}}, nil, nil
			})
			var handler http.Handler
			if transport == "http" {
				handler = sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, nil)
			} else {
				handler = sdk.NewSSEHandler(func(*http.Request) *sdk.Server { return server }, nil)
			}
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				active.Add(1)
				defer active.Add(-1)
				if r.Header.Get("Authorization") != "Bearer local-test-value" {
					wrongHeaders.Add(1)
					w.WriteHeader(401)
					return
				}
				if r.Method == "DELETE" {
					deletes.Add(1)
				}
				handler.ServeHTTP(w, r)
			}))
			defer remote.Close()
			store, err := storage.Open(storage.PathsFromDataDir(t.TempDir()))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			service := mcp.New(store)
			snap, err := service.Create(mcp.Input{Name: "fixture", Transport: transport, URL: remote.URL, Headers: map[string]string{"Authorization": "Bearer ${TOKEN}"}})
			if err != nil {
				t.Fatal(err)
			}
			id := snap.Servers[0].ID
			if _, err = store.DB.Exec(`UPDATE agent_mcp_servers SET enabled=0`); err != nil {
				t.Fatal(err)
			}
			debug := mcp.NewDebugger(service, mcp.DebugOptions{Timeout: time.Second, Environment: func() []string { return []string{"TOKEN=local-test-value"} }})
			tools, err := debug.ListTools(context.Background(), id)
			if err != nil || len(tools) != 1 || tools[0].Name != "echo" {
				t.Fatal(tools, err)
			}
			result, err := debug.CallTool(context.Background(), id, "echo", `{"message":"远端调用成功"}`)
			if err != nil || !strings.Contains(string(result.Result), "远端调用成功") {
				t.Fatal(result, err)
			}
			if _, err = debug.CallTool(context.Background(), id, "echo", `{"message":"hang"}`); err == nil {
				t.Fatal("missing call timeout")
			}
			deadline := time.Now().Add(3 * time.Second)
			for active.Load() != 0 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if active.Load() != 0 || requests.Load() < 4 || wrongHeaders.Load() != 0 {
				t.Fatal("connection/header contract", active.Load(), requests.Load(), wrongHeaders.Load())
			}
			if transport == "http" && deletes.Load() != 3 {
				t.Fatal("each operation must terminate its session", deletes.Load())
			}
		})
	}
}

func TestRemoteErrorsAndTimeoutReleaseRequestsWithoutLeakingSecrets(t *testing.T) {
	for _, transport := range []string{"http", "sse"} {
		t.Run(transport, func(t *testing.T) {
			store, err := storage.Open(storage.PathsFromDataDir(t.TempDir()))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			service := mcp.New(store)
			var active atomic.Int32
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				active.Add(1)
				defer active.Add(-1)
				switch r.URL.Path {
				case "/denied":
					http.Error(w, "secret-value-must-not-leak", 401)
				case "/invalid":
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, "event: invalid\ndata: secret-value-must-not-leak\n\n")
				case "/hang":
					io.Copy(io.Discard, r.Body)
					<-r.Context().Done()
				}
			}))
			defer remote.Close()
			debug := mcp.NewDebugger(service, mcp.DebugOptions{Timeout: 100 * time.Millisecond})
			for _, path := range []string{"/denied", "/invalid", "/hang"} {
				snap, err := service.Create(mcp.Input{Name: strings.TrimPrefix(path, "/"), Transport: transport, URL: remote.URL + path, Headers: map[string]string{"X-Token": "secret-value-must-not-leak"}})
				if err != nil {
					t.Fatal(err)
				}
				var id int64
				for _, server := range snap.Servers {
					if server.Name == strings.TrimPrefix(path, "/") {
						id = server.ID
					}
				}
				start := time.Now()
				_, err = debug.ListTools(context.Background(), id)
				if err == nil || strings.Contains(err.Error(), "secret-value-must-not-leak") {
					t.Fatal("unsafe or missing error", err)
				}
				if time.Since(start) > 2*time.Second {
					t.Fatal("unbounded cleanup")
				}
			}
			deadline := time.Now().Add(3 * time.Second)
			for active.Load() != 0 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if active.Load() != 0 {
				t.Fatal("requests remain after error/timeout", active.Load())
			}
		})
	}
}

func TestRemoteFailuresKeepSafeProtocolAndNetworkCategories(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID any `json:"id"`
		}
		json.NewDecoder(r.Body).Decode(&request)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "error": map[string]any{"code": -32602, "message": "secret-payload"}})
	}))
	store, err := storage.Open(storage.PathsFromDataDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := mcp.New(store)
	snap, err := service.Create(mcp.Input{Name: "safe-remote", Transport: "http", URL: remote.URL})
	if err != nil {
		t.Fatal(err)
	}
	debugger := mcp.NewDebugger(service, mcp.DebugOptions{Timeout: time.Second})
	_, err = debugger.ListTools(context.Background(), snap.Servers[0].ID)
	if err == nil || !strings.Contains(err.Error(), "-32602") || strings.Contains(err.Error(), "secret-payload") {
		t.Fatal("lost safe protocol status or leaked server payload", err)
	}
	remote.Close()
	_, err = debugger.ListTools(context.Background(), snap.Servers[0].ID)
	if err == nil || !strings.Contains(err.Error(), "网络连接失败") || strings.Contains(err.Error(), remote.URL) {
		t.Fatal("lost network category or leaked endpoint", err)
	}
}
