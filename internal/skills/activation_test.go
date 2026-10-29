package skills_test

import (
	"errors"
	"github.com/lizhian/agent-manager/internal/skills"
	"github.com/lizhian/agent-manager/internal/storage"
	"os"
	"path/filepath"
	"testing"
)

func TestGlobalActivationIsIsolatedAndPreservesManualFiles(t *testing.T) {
	home := t.TempDir()
	store, err := storage.Open(storage.PathsFromHome(home))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.DB.Exec(`INSERT INTO skill_sources VALUES(11,'a/repo','a_repo','',''),(12,'b/repo','b_repo','','');INSERT INTO skills VALUES(21,11,'demo','',0,'',''),(22,12,'demo','',0,'','')`)
	for _, source := range []string{"a_repo", "b_repo"} {
		dir := filepath.Join(store.Paths.Root, "skills", source, "demo")
		os.MkdirAll(dir, 0700)
		os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("中文"), 0600)
	}
	s := skills.New(store, skills.Options{})
	snap, err := s.SetGlobalEnabled("a_repo", "demo", true)
	if err != nil || snap.EnabledContentChars != 2 {
		t.Fatalf("enable %+v %v", snap, err)
	}
	link := filepath.Join(store.Paths.AgentsDir, "skills/a_repo__demo")
	target, err := os.Readlink(link)
	if err != nil || target != filepath.Join(store.Paths.Root, "skills/a_repo/demo") {
		t.Fatal(target, err)
	}
	if _, err = s.SetGlobalEnabled("b_repo", "demo", true); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetGlobalEnabled("a_repo", "demo", false); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Lstat(link); !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if _, err = os.Readlink(filepath.Join(store.Paths.AgentsDir, "skills/b_repo__demo")); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(link, []byte("manual"), 0600)
	if _, err = s.SetGlobalEnabled("a_repo", "demo", true); err == nil {
		t.Fatal("overwrote manual file")
	}
	data, _ := os.ReadFile(link)
	if string(data) != "manual" {
		t.Fatal("manual lost")
	}
	var enabled bool
	store.DB.QueryRow("SELECT global_enabled FROM skills WHERE id=21").Scan(&enabled)
	if enabled {
		t.Fatal("failed activation saved")
	}
	fake := skills.New(store, skills.Options{Symlink: func(string, string) error { return errors.New("fake link failure") }})
	os.Remove(link)
	if _, err = fake.SetGlobalEnabled("a_repo", "demo", true); err == nil {
		t.Fatal("expected failure")
	}
	store.DB.QueryRow("SELECT global_enabled FROM skills WHERE id=21").Scan(&enabled)
	if enabled {
		t.Fatal("SQL not rolled back")
	}
}
func TestCommitFailureRestoresOwnedLinksAndDatabase(t *testing.T) {
	home := t.TempDir()
	store, err := storage.Open(storage.PathsFromHome(home))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.DB.Exec(`INSERT INTO skill_sources VALUES(1,'owner/repo','owner_repo','','');INSERT INTO skills VALUES(1,1,'demo','',0,'','')`)
	dir := filepath.Join(store.Paths.Root, "skills/owner_repo/demo")
	os.MkdirAll(dir, 0700)
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("body"), 0600)
	s := skills.New(store, skills.Options{})
	if _, err = s.SetGlobalEnabled("owner_repo", "demo", true); err != nil {
		t.Fatal(err)
	}
	_, err = store.DB.Exec(`CREATE TABLE commit_fault(source_id INTEGER REFERENCES skill_sources(id) DEFERRABLE INITIALLY DEFERRED);CREATE TRIGGER reject_commit AFTER UPDATE ON skills BEGIN INSERT INTO commit_fault VALUES(999);END`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetGlobalEnabled("owner_repo", "demo", false); err == nil {
		t.Fatal("commit should fail")
	}
	link := filepath.Join(store.Paths.AgentsDir, "skills/owner_repo__demo")
	if actual, err := os.Readlink(link); err != nil || actual != dir {
		t.Fatal("link not restored", actual, err)
	}
	snap, err := s.Snapshot()
	if err != nil || !snap.Sources[0].Skills[0].GlobalEnabled {
		t.Fatal("database not restored", snap, err)
	}
	store.DB.Exec("DROP TRIGGER reject_commit")
	os.Remove(link)
	os.Symlink(filepath.Join(home, "manual"), link)
	if _, err = s.SetGlobalEnabled("owner_repo", "demo", false); err == nil {
		t.Fatal("removed foreign symlink")
	}
	if actual, _ := os.Readlink(link); actual != filepath.Join(home, "manual") {
		t.Fatal(actual)
	}
}
