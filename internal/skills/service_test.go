package skills_test

import (
	"fmt"
	"github.com/lizhian/agent-manager/internal/skills"
	"github.com/lizhian/agent-manager/internal/storage"
	"os"
	"path/filepath"
	"testing"
)

func TestBrowseExistingCatalogAndBoundedDocuments(t *testing.T) {
	home := t.TempDir()
	store, err := storage.Open(storage.PathsFromHome(home))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, err = store.DB.Exec(`INSERT INTO skill_sources(id,source,source_safe,installed_at) VALUES(77,'owner/repo','owner_repo','100'); INSERT INTO skills(id,source_id,name,description,global_enabled) VALUES(99,77,'demo','描述',1);`)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".agent-manager/skills/owner_repo/demo")
	os.MkdirAll(filepath.Join(dir, "references"), 0700)
	body := "---\nname: Demo\ndescription: >\n  第一行\n  第二行\n---\n# 中文"
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0600)
	os.WriteFile(filepath.Join(dir, "references/guide.md"), []byte("# 指南"), 0600)
	s := skills.New(store, skills.Options{})
	snap, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Sources) != 1 || snap.Sources[0].ID != 77 || snap.Sources[0].Skills[0].ID != 99 || snap.EnabledContentChars != 50 {
		t.Fatalf("snapshot: %+v", snap)
	}
	detail, err := s.Detail("owner_repo", "demo")
	if err != nil || detail.MetadataName != "Demo" || detail.Content != body {
		t.Fatalf("detail %+v %v", detail, err)
	}
	doc, err := s.Document("owner_repo", "demo", "demo/references/guide.md")
	if err != nil || doc.Content != "# 指南" {
		t.Fatalf("doc %+v %v", doc, err)
	}
	os.WriteFile(filepath.Join(home, "outside.md"), []byte("private"), 0600)
	os.Symlink(filepath.Join(home, "outside.md"), filepath.Join(dir, "references/escape.md"))
	for _, p := range []string{"../../../outside.md", "demo/references/escape.md", "/tmp/a.md", "demo/SKILL.txt"} {
		if _, err = s.Document("owner_repo", "demo", p); err == nil {
			t.Fatalf("accepted %s", p)
		}
	}
	var id int
	store.DB.QueryRow("SELECT id FROM skills").Scan(&id)
	if id != 99 {
		t.Fatalf("changed id %d", id)
	}
}
func TestLegacyMigrationIsAtomicAndPreservesUnrelatedData(t *testing.T) {
	home := t.TempDir()
	store, err := storage.Open(storage.PathsFromHome(home))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.DB.Exec("INSERT INTO app_settings VALUES('keep','\"value\"','1')")
	target := filepath.Join(home, ".agent-manager/skills/owner_repo/demo")
	os.MkdirAll(target, 0700)
	project := filepath.Join(home, "project")
	link := filepath.Join(project, ".agents/skills/owner_repo__demo")
	os.MkdirAll(filepath.Dir(link), 0700)
	os.Symlink(target, link)
	legacy := filepath.Join(home, ".agent-manager/install-skills.json")
	body := fmt.Sprintf(`{"sources":[{"source":"owner/repo","sourceSafe":"owner_repo","skills":[{"name":"demo","description":"legacy","globalEnabled":true,"projectPaths":[%q]}]}]}`, project)
	os.WriteFile(legacy, []byte(body), 0600)
	store.DB.Exec(`CREATE TRIGGER reject_import BEFORE INSERT ON skills BEGIN SELECT RAISE(FAIL,'fake SQL failure'); END`)
	s := skills.New(store, skills.Options{})
	if _, err = s.Snapshot(); err == nil {
		t.Fatal("expected failed import")
	}
	if _, err = os.Lstat(link); err != nil {
		t.Fatal("project link lost", err)
	}
	if _, err = os.Stat(legacy); err != nil {
		t.Fatal("legacy lost", err)
	}
	store.DB.Exec("DROP TRIGGER reject_import")
	snap, err := s.Snapshot()
	if err != nil || len(snap.Sources) != 1 || !snap.Sources[0].Skills[0].GlobalEnabled {
		t.Fatalf("import %+v %v", snap, err)
	}
	if _, err = os.Lstat(link); !os.IsNotExist(err) {
		t.Fatal("managed project link remains", err)
	}
	if _, err = os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatal("legacy remains", err)
	}
	var value string
	store.DB.QueryRow("SELECT value_json FROM app_settings WHERE key='keep'").Scan(&value)
	if value != "\"value\"" {
		t.Fatal(value)
	}
}
func TestLegacyForeignProjectFileIsPreservedAndImportAborted(t *testing.T) {
	home := t.TempDir()
	store, err := storage.Open(storage.PathsFromHome(home))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	project := filepath.Join(home, "project")
	p := filepath.Join(project, ".agents/skills/owner_repo__demo")
	os.MkdirAll(filepath.Dir(p), 0700)
	os.WriteFile(p, []byte("manual"), 0600)
	legacy := filepath.Join(store.Paths.Root, "install-skills.json")
	os.WriteFile(legacy, []byte(fmt.Sprintf(`{"sources":[{"source":"owner/repo","sourceSafe":"owner_repo","skills":[{"name":"demo","projectPaths":[%q]}]}]}`, project)), 0600)
	if _, err = skills.New(store, skills.Options{}).Snapshot(); err == nil {
		t.Fatal("accepted manual project file")
	}
	body, _ := os.ReadFile(p)
	if string(body) != "manual" {
		t.Fatal("manual data lost")
	}
	if _, err = os.Stat(legacy); err != nil {
		t.Fatal(err)
	}
	var count int
	store.DB.QueryRow("SELECT count(*) FROM skills").Scan(&count)
	if count != 0 {
		t.Fatal("partially imported")
	}
}
func TestRelativeLinksAndSystemOpeningUseSameDocumentBoundary(t *testing.T) {
	home := t.TempDir()
	store, err := storage.Open(storage.PathsFromHome(home))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.DB.Exec(`INSERT INTO skill_sources VALUES(1,'owner/repo','owner_repo','','');INSERT INTO skills VALUES(1,1,'demo','',0,'','')`)
	dir := filepath.Join(home, ".agent-manager/skills/owner_repo/demo")
	os.MkdirAll(filepath.Join(dir, "references"), 0700)
	os.WriteFile(filepath.Join(dir, "references/中文.md"), []byte("# 中文"), 0600)
	opened := ""
	s := skills.New(store, skills.Options{Open: func(p string) error { opened = p; return nil }})
	relative, err := skills.ResolveDocumentPath("demo/SKILL.md", "references/%E4%B8%AD%E6%96%87.md#说明")
	if err != nil || relative != "demo/references/中文.md" {
		t.Fatal(relative, err)
	}
	if err = s.OpenDocument("owner_repo", "demo", relative); err != nil || opened != filepath.Join(dir, "references/中文.md") {
		t.Fatal(opened, err)
	}
	if err = s.OpenDocument("owner_repo", "demo", "../../../private.md"); err == nil {
		t.Fatal("opened outside")
	}
	for _, href := range []string{"file:///tmp/test.md", "https://example.org/a.md", "//example.org/a.md"} {
		if _, err = skills.ResolveDocumentPath("demo/SKILL.md", href); err == nil {
			t.Fatal(href)
		}
	}
	if err = s.OpenGlobal(); err != nil || opened != filepath.Join(home, ".agents/skills") {
		t.Fatal(opened, err)
	}
}
