package skills_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lizhian/agent-manager/internal/skills"
	"github.com/lizhian/agent-manager/internal/storage"
	"os"
	"path/filepath"
	"testing"
)

func TestBatchUpdateKeepsSuccessfulSourcesAndReportsFailures(t *testing.T) {
	home := t.TempDir()
	store, e := storage.Open(storage.PathsFromHome(home))
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	fail := false
	runner := func(ctx context.Context, dir string, args []string) ([]byte, error) {
		if args[0] == "add" {
			if fail && args[1] == "b/repo" {
				return nil, fmt.Errorf("fake download failure")
			}
			return nil, nil
		}
		p := filepath.Join(dir, "demo")
		os.MkdirAll(p, 0700)
		os.WriteFile(filepath.Join(p, "SKILL.md"), []byte("# demo"), 0600)
		return json.Marshal([]map[string]string{{"name": "demo", "path": p}})
	}
	s := skills.New(store, skills.Options{Runner: runner})
	for _, source := range []string{"a/repo", "b/repo"} {
		if _, e = s.SyncSource(context.Background(), source, nil); e != nil {
			t.Fatal(e)
		}
	}
	fail = true
	var progress skills.Progress
	result, e := s.UpdateAll(context.Background(), func(p skills.Progress) { progress = p })
	if e != nil || result.SucceededSources != 1 || result.FailedSources != 1 || len(result.Failures) != 1 || result.Failures[0].Source != "b/repo" || len(result.Snapshot.Sources) != 2 || progress.CompletedSources != 2 || progress.TotalSources != 2 {
		t.Fatal(result, progress, e)
	}
}
func TestDeleteSourceImpactFailureAndSuccessPreserveOtherObjects(t *testing.T) {
	home := t.TempDir()
	store, e := storage.Open(storage.PathsFromHome(home))
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	store.DB.Exec(`INSERT INTO skill_sources VALUES(1,'a/repo','a_repo','',''),(2,'b/repo','b_repo','','');INSERT INTO skills VALUES(1,1,'demo','',1,'',''),(2,2,'demo','',0,'','')`)
	dir := filepath.Join(store.Paths.Root, "skills/a_repo/demo")
	os.MkdirAll(dir, 0700)
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# old"), 0600)
	links := filepath.Join(store.Paths.AgentsDir, "skills")
	os.MkdirAll(links, 0700)
	link := filepath.Join(links, "a_repo__demo")
	os.Symlink(dir, link)
	manual := filepath.Join(links, "manual")
	os.WriteFile(manual, []byte("manual"), 0600)
	s := skills.New(store, skills.Options{})
	impact, e := s.RemoveImpact("a_repo")
	if e != nil || impact.InstalledSkills != 1 || impact.GlobalEnabledSkills != 1 {
		t.Fatal(impact, e)
	}
	store.DB.Exec(`CREATE TABLE delete_fault(source_id INTEGER REFERENCES skill_sources(id) DEFERRABLE INITIALLY DEFERRED);CREATE TRIGGER reject_delete AFTER DELETE ON skill_sources BEGIN INSERT INTO delete_fault VALUES(999);END`)
	if _, e = s.RemoveSource("a_repo"); e == nil {
		t.Fatal("expected commit failure")
	}
	if actual, e := os.Readlink(link); e != nil || actual != dir {
		t.Fatal("link lost", actual, e)
	}
	if _, e = os.Stat(dir); e != nil {
		t.Fatal("mirror lost", e)
	}
	store.DB.Exec("DROP TRIGGER reject_delete")
	snap, e := s.RemoveSource("a_repo")
	if e != nil || len(snap.Sources) != 1 || snap.Sources[0].SourceSafe != "b_repo" {
		t.Fatal(snap, e)
	}
	if _, e = os.Lstat(link); !os.IsNotExist(e) {
		t.Fatal("link remains")
	}
	if data, _ := os.ReadFile(manual); string(data) != "manual" {
		t.Fatal("manual lost")
	}
}
func TestEmptyBatchAndManualDeleteConflictDoNotMutateData(t *testing.T) {
	store, e := storage.Open(storage.PathsFromHome(t.TempDir()))
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	s := skills.New(store, skills.Options{})
	r, e := s.UpdateAll(context.Background(), nil)
	if e != nil || r.TotalSources != 0 || r.FailedSources != 0 {
		t.Fatal(r, e)
	}
	store.DB.Exec(`INSERT INTO skill_sources VALUES(1,'a/repo','a_repo','','');INSERT INTO skills VALUES(1,1,'demo','',0,'','')`)
	links := filepath.Join(store.Paths.AgentsDir, "skills")
	os.MkdirAll(links, 0700)
	manual := filepath.Join(links, "a_repo__demo")
	os.WriteFile(manual, []byte("manual"), 0600)
	if _, e = s.RemoveSource("a_repo"); e == nil {
		t.Fatal("deleted colliding manual path")
	}
	snap, e := s.Snapshot()
	if e != nil || len(snap.Sources) != 1 {
		t.Fatal(snap, e)
	}
	if b, _ := os.ReadFile(manual); string(b) != "manual" {
		t.Fatal("manual lost")
	}
}
