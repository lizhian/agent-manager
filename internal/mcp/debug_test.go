package mcp_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/lizhian/agent-manager/internal/mcp"
	"github.com/lizhian/agent-manager/internal/storage"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestMCPHelperProcess(t *testing.T) {
	if os.Getenv("MCP_TEST_HELPER") != "1" {
		return
	}
	if pidfile := os.Getenv("MCP_PID_FILE"); pidfile != "" {
		child := exec.Command("/bin/sleep", "30")
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		file, err := os.OpenFile(pidfile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			os.Exit(2)
		}
		fmt.Fprintf(file, "%d\n%d\n", os.Getpid(), child.Process.Pid)
		file.Close()
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "fixture", Version: "1"}, nil)
	sdk.AddTool(server, &sdk.Tool{Name: "echo", Description: "中文回声"}, func(ctx context.Context, req *sdk.CallToolRequest, in struct {
		Message string `json:"message"`
	}) (*sdk.CallToolResult, any, error) {
		if in.Message == "hang" {
			<-ctx.Done()
			return nil, nil, ctx.Err()
		}
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: in.Message + os.Getenv("EXPANDED")}}}, nil, nil
	})
	if err := server.Run(context.Background(), &sdk.StdioTransport{}); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}
func TestStdioDebugUsesSavedDisabledConfigAndReleasesEachProcess(t *testing.T) {
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
	snap, err := service.Create(mcp.Input{Name: "fixture", Transport: "stdio", Command: exe, Args: []string{"-test.run=^TestMCPHelperProcess$"}, Env: map[string]string{"MCP_TEST_HELPER": "1", "EXPANDED": "${MCP_EXPANSION_TEST}"}})
	if err != nil {
		t.Fatal(err)
	}
	id := snap.Servers[0].ID
	t.Setenv("MCP_EXPANSION_TEST", "resolved")
	if _, err = store.DB.Exec(`UPDATE agent_mcp_servers SET enabled=0`); err != nil {
		t.Fatal(err)
	}
	debugger := mcp.NewDebugger(service, mcp.DebugOptions{Timeout: time.Second})
	tools, err := debugger.ListTools(context.Background(), id)
	if err != nil || len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatal(tools, err)
	}
	result, err := debugger.CallTool(context.Background(), id, "echo", `{"message":"你好"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result.Result), "你好resolved") {
		t.Fatal(result)
	}
	for _, bad := range []string{`null`, `[]`, `"x"`, `{`} {
		if _, err = debugger.CallTool(context.Background(), id, "echo", bad); err == nil {
			t.Fatal("accepted", bad)
		}
	}
	start := time.Now()
	if _, err = debugger.CallTool(context.Background(), id, "echo", `{"message":"hang"}`); err == nil {
		t.Fatal("timeout accepted")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("cleanup exceeded bound")
	}
}

func TestEnvironmentPlaceholdersRemainLiteralUnlessBracedAndDefined(t *testing.T) {
	if got := mcp.ExpandEnvironment("$TOKEN ${TOKEN} ${MISSING}", map[string]string{"TOKEN": "secret"}); got != "$TOKEN secret ${MISSING}" {
		t.Fatal(got)
	}
}
func TestStdioMissingCommandAndProtocolFailure(t *testing.T) {
	store, err := storage.Open(storage.PathsFromDataDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := mcp.New(store)
	for _, command := range []string{"no-such-executable-mcp-fixture", "/usr/bin/false"} {
		snap, err := service.Create(mcp.Input{Name: strings.ReplaceAll(command, "/", "_"), Transport: "stdio", Command: command})
		if err != nil {
			t.Fatal(err)
		}
		debugger := mcp.NewDebugger(service, mcp.DebugOptions{Timeout: time.Second, UserPath: func(context.Context, string) string { return "/usr/bin:/bin" }})
		id := snap.Servers[0].ID
		for _, server := range snap.Servers {
			if server.Command == command {
				id = server.ID
			}
		}
		if _, err = debugger.ListTools(context.Background(), id); err == nil {
			t.Fatal("protocol/start failure accepted")
		}
	}
}

func TestDebugReleasesOwnedWrapperChildren(t *testing.T) {
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
	pids := filepath.Join(t.TempDir(), "pids")
	snap, err := service.Create(mcp.Input{Name: "wrapper", Transport: "stdio", Command: exe, Args: []string{"-test.run=^TestMCPHelperProcess$"}, Env: map[string]string{"MCP_TEST_HELPER": "1", "MCP_PID_FILE": pids}})
	if err != nil {
		t.Fatal(err)
	}
	debugger := mcp.NewDebugger(service, mcp.DebugOptions{Timeout: 400 * time.Millisecond, UserPath: func(context.Context, string) string { return "/bin:/usr/bin" }})
	if _, err = debugger.ListTools(context.Background(), snap.Servers[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err = debugger.CallTool(context.Background(), snap.Servers[0].ID, "echo", `{"message":"hang"}`); err == nil {
		t.Fatal("timeout missing")
	}
	data, err := os.ReadFile(pids)
	if err != nil {
		t.Fatal(err)
	}
	if len(strings.Fields(string(data))) != 4 {
		t.Fatal("expected separate owned processes for each operation", string(data))
	}
	for _, field := range strings.Fields(string(data)) {
		pid, err := strconv.Atoi(field)
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(2 * time.Second)
		for {
			err = syscall.Kill(pid, 0)
			if errors.Is(err, syscall.ESRCH) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("owned process %d remains: %v", pid, err)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}
