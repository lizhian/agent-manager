package skills_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lizhian/agent-manager/internal/skills"
	"github.com/lizhian/agent-manager/internal/storage"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestInstallAndUpdateMirrorPreserveActivationAndIDs(t *testing.T) {
	home := t.TempDir()
	store, e := storage.Open(storage.PathsFromHome(home))
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	version := 1
	runner := func(ctx context.Context, dir string, args []string) ([]byte, error) {
		if args[0] == "add" {
			return nil, nil
		}
		names := []string{"keep", "gone"}
		if version == 2 {
			names = []string{"keep", "new"}
		}
		var listed []map[string]string
		for _, name := range names {
			p := filepath.Join(dir, name)
			os.MkdirAll(p, 0700)
			os.WriteFile(filepath.Join(p, "SKILL.md"), []byte(fmt.Sprintf("---\nname: %s\ndescription: 版本%d\n---\n# %s", name, version, name)), 0600)
			listed = append(listed, map[string]string{"name": name, "path": p})
		}
		return json.Marshal(listed)
	}
	s := skills.New(store, skills.Options{Runner: runner})
	result, e := s.SyncSource(context.Background(), "owner/repo", nil)
	if e != nil || result.InstalledCount != 2 || result.Updated {
		t.Fatal(result, e)
	}
	s.SetGlobalEnabled("owner_repo", "keep", true)
	s.SetGlobalEnabled("owner_repo", "gone", true)
	before, _ := s.Snapshot()
	version = 2
	manual := filepath.Join(store.Paths.AgentsDir, "skills/manual")
	os.WriteFile(manual, []byte("keep"), 0600)
	result, e = s.SyncSource(context.Background(), "owner/repo", nil)
	if e != nil || result.RemovedCount != 1 || !result.Updated {
		t.Fatal(result, e)
	}
	after := result.Snapshot
	if after.Sources[0].ID != before.Sources[0].ID {
		t.Fatal("source id changed")
	}
	var keep skills.Skill
	for _, v := range after.Sources[0].Skills {
		if v.Name == "keep" {
			keep = v
		}
	}
	if !keep.GlobalEnabled || keep.ID != before.Sources[0].Skills[1].ID {
		t.Fatal("skill identity changed", keep)
	}
	if _, e = os.Lstat(filepath.Join(store.Paths.AgentsDir, "skills/owner_repo__gone")); !os.IsNotExist(e) {
		t.Fatal("removed link remains")
	}
	if b, _ := os.ReadFile(manual); string(b) != "keep" {
		t.Fatal("manual lost")
	}
}
func TestSourceCommitFailureRestoresMirrorCatalogAndLinks(t *testing.T) {
	home := t.TempDir()
	store, e := storage.Open(storage.PathsFromHome(home))
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	version := 1
	runner := func(ctx context.Context, dir string, args []string) ([]byte, error) {
		if args[0] == "add" {
			return nil, nil
		}
		names := []string{"keep", "gone"}
		if version == 2 {
			names = []string{"keep", "new"}
		}
		var listed []map[string]string
		for _, name := range names {
			p := filepath.Join(dir, name)
			os.MkdirAll(p, 0700)
			os.WriteFile(filepath.Join(p, "SKILL.md"), []byte(fmt.Sprintf("# version%d", version)), 0600)
			listed = append(listed, map[string]string{"name": name, "path": p})
		}
		return json.Marshal(listed)
	}
	s := skills.New(store, skills.Options{Runner: runner})
	if _, e = s.SyncSource(context.Background(), "owner/repo", nil); e != nil {
		t.Fatal(e)
	}
	s.SetGlobalEnabled("owner_repo", "keep", true)
	s.SetGlobalEnabled("owner_repo", "gone", true)
	old, _ := s.Snapshot()
	version = 2
	store.DB.Exec(`CREATE TABLE sync_fault(source_id INTEGER REFERENCES skill_sources(id) DEFERRABLE INITIALLY DEFERRED);CREATE TRIGGER reject_sync AFTER UPDATE ON skills BEGIN INSERT INTO sync_fault VALUES(999);END`)
	if _, e = s.SyncSource(context.Background(), "owner/repo", nil); e == nil {
		t.Fatal("expected commit failure")
	}
	snap, _ := s.Snapshot()
	if !reflect.DeepEqual(snap, old) {
		t.Fatal("catalog changed", snap, old)
	}
	for _, name := range []string{"keep", "gone"} {
		body, e := os.ReadFile(filepath.Join(store.Paths.Root, "skills/owner_repo", name, "SKILL.md"))
		if e != nil || string(body) != "# version1" {
			t.Fatal("mirror changed", string(body), e)
		}
		if _, e = os.Readlink(filepath.Join(store.Paths.AgentsDir, "skills/owner_repo__"+name)); e != nil {
			t.Fatal("link lost", e)
		}
	}
	store.DB.Exec("DROP TRIGGER reject_sync")
	bad := skills.New(store, skills.Options{Runner: runner, Rename: func(a, b string) error {
		if strings.Contains(filepath.Base(a), ".staging-") {
			return fmt.Errorf("fake rename failure")
		}
		return os.Rename(a, b)
	}})
	if _, e = bad.SyncSource(context.Background(), "owner/repo", nil); e == nil {
		t.Fatal("expected rename failure")
	}
	if body, _ := os.ReadFile(filepath.Join(store.Paths.Root, "skills/owner_repo/gone/SKILL.md")); string(body) != "# version1" {
		t.Fatal("mirror not restored")
	}
}
func TestRejectsUntrustedSkillPathsWithoutChangingStorage(t *testing.T) {
	for _, mode := range []string{"traversal", "escape", "cycle", "command"} {
		t.Run(mode, func(t *testing.T) {
			home := t.TempDir()
			store, e := storage.Open(storage.PathsFromHome(home))
			if e != nil {
				t.Fatal(e)
			}
			defer store.Close()
			s := skills.New(store, skills.Options{Runner: func(ctx context.Context, dir string, args []string) ([]byte, error) {
				if mode == "command" {
					return nil, fmt.Errorf("fake command error")
				}
				if args[0] == "add" {
					return nil, nil
				}
				p := filepath.Join(dir, "demo")
				os.MkdirAll(p, 0700)
				os.WriteFile(filepath.Join(p, "SKILL.md"), []byte("# safe"), 0600)
				name := "demo"
				if mode == "traversal" {
					name = "../escape"
				}
				if mode == "escape" {
					os.Symlink(home, filepath.Join(p, "escape"))
				}
				if mode == "cycle" {
					os.Symlink(p, filepath.Join(p, "cycle"))
				}
				return json.Marshal([]map[string]string{{"name": name, "path": p}})
			}})
			if _, e = s.SyncSource(context.Background(), "owner/repo", nil); e == nil {
				t.Fatal("accepted unsafe source")
			}
			snap, _ := s.Snapshot()
			if len(snap.Sources) != 0 {
				t.Fatal(snap)
			}
		})
	}
}
func TestDefaultRunnerResolvesNpxFromUserShell(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	os.MkdirAll(bin, 0700)
	shell := filepath.Join(home, "shell")
	os.WriteFile(shell, []byte("#!/bin/sh\nprintf '__AGENT_MANAGER_PATH_START__%s__AGENT_MANAGER_PATH_END__' '"+bin+"'\n"), 0700)
	script := `#!/bin/sh
[ "$1" = --yes ] && [ "$2" = skills ] || exit 8
case "$3" in
add) /bin/mkdir -p demo; printf '# fixture' > demo/SKILL.md;;
list) printf '[{"name":"demo","path":"%s/demo"}]' "$PWD";;
*) exit 9;;
esac
`
	os.WriteFile(filepath.Join(bin, "npx"), []byte(script), 0700)
	t.Setenv("SHELL", shell)
	t.Setenv("PATH", filepath.Join(home, "empty"))
	store, e := storage.Open(storage.PathsFromHome(home))
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	r, e := skills.New(store, skills.Options{}).SyncSource(context.Background(), "owner/repo", nil)
	if e != nil || r.InstalledCount != 1 {
		t.Fatal(r, e)
	}
}
