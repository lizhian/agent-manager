package settings_test

import (
	"github.com/lizhian/agent-manager/internal/settings"
	"github.com/lizhian/agent-manager/internal/storage"
	"testing"
)

func TestLegacySettingsSaveReloadAndRollback(t *testing.T) {
	store, err := storage.Open(storage.PathsFromDataDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s := settings.New(store, settings.Options{Now: func() string { return "123" }})
	value, err := s.Snapshot()
	if err != nil || value.FontSize != "standard" || value.FontSize.Pixels() != 15 || value.Preview.Position != "bottom" || value.Preview.Ratio != 0.5 {
		t.Fatal(value, err)
	}
	_, err = store.DB.Exec(`INSERT INTO app_settings VALUES('appearance.font_size_v3','"small"','old'),('appearance.font_family','"Example Sans"','old'),('preview.document_layout','{"position":"right","ratio":0.62}','old'),('unknown','true','old')`)
	if err != nil {
		t.Fatal(err)
	}
	value, err = s.Snapshot()
	if err != nil || value.FontSize.Pixels() != 14 || value.FontFamily != "Example Sans" || value.Preview.Ratio != 0.62 {
		t.Fatal(value, err)
	}
	value.FontSize = "extra-large"
	value.FontFamily = "中文字体"
	value.Preview = settings.Preview{Position: "bottom", Ratio: 0.8}
	saved, err := s.Save(value)
	if err != nil || saved != value {
		t.Fatal(saved, err)
	}
	reloaded, err := settings.New(store, settings.Options{}).Snapshot()
	if err != nil || reloaded != value {
		t.Fatal(reloaded, err)
	}
	_, err = store.DB.Exec("CREATE TRIGGER reject_setting BEFORE UPDATE ON app_settings WHEN NEW.key='preview.document_layout' BEGIN SELECT RAISE(ABORT,'disk unavailable'); END")
	if err != nil {
		t.Fatal(err)
	}
	changed := value
	changed.FontSize = "extra-small"
	if _, err = s.Save(changed); err == nil {
		t.Fatal("expected save failure")
	}
	reloaded, err = s.Snapshot()
	if err != nil || reloaded != value {
		t.Fatal("partial settings write", reloaded, err)
	}
	var unknown string
	store.DB.QueryRow("SELECT value_json FROM app_settings WHERE key='unknown'").Scan(&unknown)
	if unknown != "true" {
		t.Fatal("unknown data changed")
	}
}
