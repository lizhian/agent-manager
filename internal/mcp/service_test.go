package mcp_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lizhian/agent-manager/internal/mcp"
	"github.com/lizhian/agent-manager/internal/storage"
)

func TestCRUDPreservesCompatibilityAndHasNoDistributionSideEffects(t *testing.T) {
	root := t.TempDir()
	store, err := storage.Open(storage.PathsFromDataDir(root))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sentinel := filepath.Join(store.Paths.Root, "mcp", "gaal.yaml")
	if err = os.MkdirAll(filepath.Dir(sentinel), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(sentinel, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = store.DB.Exec(`INSERT INTO app_settings VALUES('mcp.defaultAgents','invalid unused JSON','1'); INSERT INTO agent_mcp_servers VALUES(41,'old','stdio','fake','[]','{}','','{}','["codex"]',0,'1','2')`)
	if err != nil {
		t.Fatal(err)
	}
	service := mcp.New(store)
	snap, err := service.Update(41, mcp.Input{Name: "renamed", Transport: "http", URL: "https://example.invalid/mcp", Headers: map[string]string{"Authorization": "${TOKEN}"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Servers) != 1 || snap.Servers[0].ID != 41 || snap.Servers[0].CreatedAt != "1" {
		t.Fatal(snap)
	}
	var agents string
	var enabled int
	if err = store.DB.QueryRow(`SELECT agents_json,enabled FROM agent_mcp_servers WHERE id=41`).Scan(&agents, &enabled); err != nil {
		t.Fatal(err)
	}
	if agents != `["codex"]` || enabled != 0 {
		t.Fatal(agents, enabled)
	}
	snap, err = service.Create(mcp.Input{Name: "new", Transport: "sse", URL: "https://example.invalid/sse"})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Servers) != 2 {
		t.Fatal(snap)
	}
	if err = store.DB.QueryRow(`SELECT agents_json,enabled FROM agent_mcp_servers WHERE name='new'`).Scan(&agents, &enabled); err != nil {
		t.Fatal(err)
	}
	if agents != "[]" || enabled != 1 {
		t.Fatal(agents, enabled)
	}
	restarted := mcp.New(store)
	snap, err = restarted.Snapshot()
	if err != nil || len(snap.Servers) != 2 {
		t.Fatal(snap, err)
	}
	snap, err = service.Delete(41)
	if err != nil || len(snap.Servers) != 1 {
		t.Fatal(snap, err)
	}
	content, err := os.ReadFile(sentinel)
	if err != nil || string(content) != "unchanged" {
		t.Fatal(string(content), err)
	}
}

func TestValidationAndWriteFailurePreservePreviousSnapshot(t *testing.T) {
	store, err := storage.Open(storage.PathsFromDataDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := mcp.New(store)
	if _, err = service.Create(mcp.Input{Name: "合法中文", Transport: "stdio", Command: "fake", Args: []string{"a b"}, Env: map[string]string{"TOKEN": "${SECRET}"}}); err != nil {
		t.Fatal(err)
	}
	for _, input := range []mcp.Input{{Name: "bad name", Transport: "stdio", Command: "fake"}, {Name: "a", Transport: "stdio"}, {Name: "a", Transport: "http"}, {Name: "a", Transport: "sse"}, {Name: "a", Transport: "unknown"}, {Name: "合法中文", Transport: "stdio", Command: "fake"}} {
		if _, err = service.Create(input); err == nil {
			t.Fatal("accepted invalid input", input.Name)
		}
	}
	if _, err = store.DB.Exec(`CREATE TRIGGER fail_mcp BEFORE UPDATE ON agent_mcp_servers BEGIN SELECT RAISE(FAIL,'injected'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Update(1, mcp.Input{Name: "changed", Transport: "stdio", Command: "other"}); err == nil {
		t.Fatal("accepted failed write")
	}
	snap, err := service.Snapshot()
	if err != nil || len(snap.Servers) != 1 || snap.Servers[0].Name != "合法中文" || snap.Servers[0].Args[0] != "a b" {
		t.Fatal(snap, err)
	}
	if _, err = service.Delete(999); err == nil {
		t.Fatal("deleted unknown ID")
	}
}
