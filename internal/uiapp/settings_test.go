package uiapp_test

import (
	"fmt"
	"github.com/egoist/mygo/ui"
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
