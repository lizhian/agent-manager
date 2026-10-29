package compatibility_test

import (
	"context"
	"fmt"
	"github.com/lizhian/agent-manager/internal/agentsmd"
	"github.com/lizhian/agent-manager/internal/mcp"
	"github.com/lizhian/agent-manager/internal/settings"
	"github.com/lizhian/agent-manager/internal/skills"
	"github.com/lizhian/agent-manager/internal/storage"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeServicesShareLegacyDatabaseWithoutDistributionSideEffects(t *testing.T) {
	home := t.TempDir()
	paths := storage.PathsFromDataDir(home)
	store, err := storage.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, err = store.DB.Exec(`INSERT INTO agent_cache VALUES('old-agent',1,'legacy','','','','',0,0);
 INSERT INTO agent_cache_metadata VALUES(1,'old-gaal','old');
 INSERT INTO agent_mcp_servers(id,name,transport,command,agents_json,enabled,created_at,updated_at) VALUES(77,'Existing','stdio','old-command','["old-agent"]',0,'old','old');
 INSERT INTO skill_sources VALUES(31,'owner/repo','owner_repo','old','old');
 INSERT INTO skills VALUES(32,31,'demo','Existing skill',0,'old','old');
 INSERT INTO app_settings VALUES('legacy.distribution','{"keep":true}','old');`)
	if err != nil {
		t.Fatal(err)
	}
	files := []string{filepath.Join(home, ".config/gaal/config.json"), filepath.Join(home, ".old-agent/mcp.json")}
	for _, path := range files {
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, []byte("legacy manual configuration"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	calls := 0
	catalog, err := skills.New(store, skills.Options{Runner: func(context.Context, string, []string) ([]byte, error) {
		calls++
		return nil, fmt.Errorf("unexpected subprocess")
	}}).Snapshot()
	if err != nil || len(catalog.Sources) != 1 || catalog.Sources[0].ID != 31 || catalog.Sources[0].Skills[0].ID != 32 || calls != 0 {
		t.Fatal(catalog, err, calls)
	}
	if _, err = mcp.New(store).Update(77, mcp.Input{Name: "Existing", Transport: "stdio", Command: "new-command"}); err != nil {
		t.Fatal(err)
	}
	rules := agentsmd.New(store, agentsmd.Options{})
	snap, err := rules.Create("Native rule", "# Native rule")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = rules.SetEnabled(snap.Fragments[0].ID, true); err != nil {
		t.Fatal(err)
	}
	preferences := settings.Defaults()
	preferences.FontSize = "large"
	if _, err = settings.New(store, settings.Options{}).Save(preferences); err != nil {
		t.Fatal(err)
	}
	for _, query := range []struct{ sql, want string }{{"SELECT agents_json FROM agent_mcp_servers WHERE id=77", "[\"old-agent\"]"}, {"SELECT CAST(enabled AS TEXT) FROM agent_mcp_servers WHERE id=77", "0"}, {"SELECT gaal_version FROM agent_cache_metadata WHERE id=1", "old-gaal"}, {"SELECT source FROM agent_cache WHERE name='old-agent'", "legacy"}, {"SELECT value_json FROM app_settings WHERE key='legacy.distribution'", "{\"keep\":true}"}} {
		var got string
		if err = store.DB.QueryRow(query.sql).Scan(&got); err != nil || got != query.want {
			t.Fatal(query.sql, got, err)
		}
	}
	for _, path := range files {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != "legacy manual configuration" {
			t.Fatal(path, string(got), err)
		}
	}
}
