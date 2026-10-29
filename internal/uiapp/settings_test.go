package uiapp_test

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"github.com/lizhian/agent-manager/internal/agentsmd"
	"github.com/lizhian/agent-manager/internal/settings"
	"github.com/lizhian/agent-manager/internal/storage"
	"github.com/lizhian/agent-manager/internal/uiapp"
	"testing"
	"time"
)

func TestSettingsFontFailureDoesNotBlockPreferencesAndFailedSaveRetriesDraft(t *testing.T) {
	store, err := storage.Open(storage.PathsFromDataDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := settings.New(store, settings.Options{Fonts: func() ([]string, error) { return nil, fmt.Errorf("font service unavailable") }})
	w := uiapp.NewSettingsWorkspace(service)
	updates := make(chan func(), 8)
	applied := settings.Defaults()
	w.Load(func(fn func()) { updates <- fn }, func(s settings.Snapshot) { applied = s })
	tt := ui.NewTester(w.View, 960, 640)
	flush := func() {
		t.Helper()
		select {
		case fn := <-updates:
			fn()
			tt.Frame()
		case <-time.After(3 * time.Second):
			t.Fatal("async timeout")
		}
	}
	flush()
	flush()
	if !tt.HasText("font service unavailable") {
		t.Fatal("font enumeration error was hidden")
	}
	if err = tt.Click("特大"); err != nil {
		t.Fatal(err)
	}
	flush()
	if applied.FontSize.Pixels() != 17 {
		t.Fatal(applied)
	}
	if _, err = store.DB.Exec("CREATE TRIGGER reject_settings BEFORE UPDATE ON app_settings BEGIN SELECT RAISE(ABORT,'readonly disk'); END"); err != nil {
		t.Fatal(err)
	}
	if err = tt.Click("小"); err != nil {
		t.Fatal(err)
	}
	flush()
	if !tt.HasText("readonly disk") || applied.FontSize.Pixels() != 17 {
		t.Fatal("failed preference was applied", applied)
	}
	if _, err = store.DB.Exec("DROP TRIGGER reject_settings"); err != nil {
		t.Fatal(err)
	}
	if err = tt.Click("重试保存设置"); err != nil {
		t.Fatal(err)
	}
	flush()
	if applied.FontSize.Pixels() != 14 {
		t.Fatal("failed save lost draft", applied)
	}
	if err = tt.Click("右侧"); err != nil {
		t.Fatal(err)
	}
	flush()
	if applied.Preview.Position != "right" {
		t.Fatal(applied)
	}
}

func TestWorkspacePreviewGesturePersistsSharedPreference(t *testing.T) {
	store, err := storage.Open(storage.PathsFromDataDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := settings.New(store, settings.Options{Fonts: func() ([]string, error) { return []string{"Example Sans"}, nil }})
	app := uiapp.NewApp(agentsmd.New(store, agentsmd.Options{}))
	app.Settings = uiapp.NewSettingsWorkspace(service)
	updates := make(chan func(), 8)
	app.Start(func(fn func()) { updates <- fn })
	tt := ui.NewTester(app.View, 960, 640)
	flush := func() {
		t.Helper()
		select {
		case fn := <-updates:
			fn()
			tt.Frame()
		case <-time.After(3 * time.Second):
			t.Fatal("async timeout")
		}
	}
	flush()
	flush()
	flush()
	if err = tt.Click("底部预览"); err != nil {
		t.Fatal(err)
	}
	flush()
	saved, err := service.Snapshot()
	if err != nil || saved.Preview.Position != "right" {
		t.Fatal(saved, err)
	}
	if err = tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	if err = tt.Click("Example Sans"); err != nil {
		t.Fatal(err)
	}
	flush()
	saved, err = service.Snapshot()
	if err != nil || saved.FontFamily != "Example Sans" {
		t.Fatal(saved, err)
	}
}

func TestSharedSettingsReadRetryPreservesValidPreferences(t *testing.T) {
	store, err := storage.Open(storage.PathsFromDataDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, err = store.DB.Exec(`INSERT INTO app_settings VALUES('appearance.font_size_v3','"extra-large"','old'),('appearance.font_family','"Example Sans"','old'),('preview.document_layout','broken-json','old')`)
	if err != nil {
		t.Fatal(err)
	}
	service := settings.New(store, settings.Options{Fonts: func() ([]string, error) { return []string{"Example Sans"}, nil }})
	app := uiapp.NewApp(agentsmd.New(store, agentsmd.Options{}))
	app.Settings = uiapp.NewSettingsWorkspace(service)
	updates := make(chan func(), 8)
	app.Start(func(fn func()) { updates <- fn })
	tt := ui.NewTester(app.View, 960, 640)
	flush := func() {
		t.Helper()
		select {
		case fn := <-updates:
			fn()
			tt.Frame()
		case <-time.After(3 * time.Second):
			t.Fatal("load timeout")
		}
	}
	flush()
	flush()
	flush()
	if tt.HasText("重试保存设置") {
		t.Fatal("read failure offered saving defaults")
	}
	_, err = store.DB.Exec(`UPDATE app_settings SET value_json='{"position":"right","ratio":0.7}' WHERE key='preview.document_layout'`)
	if err != nil {
		t.Fatal(err)
	}
	if err = tt.Click("重新加载设置"); err != nil {
		t.Fatal(err)
	}
	flush()
	flush()
	if tt.HasText("解析设置") {
		t.Fatal("successful retry retained stale load error")
	}
	loaded, err := service.Snapshot()
	if err != nil || loaded.FontSize != "extra-large" || loaded.FontFamily != "Example Sans" || loaded.Preview.Ratio != 0.7 {
		t.Fatal("retry overwrote valid preferences", loaded, err)
	}
}
