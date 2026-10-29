package storage_test

import (
	"github.com/lizhian/agent-manager/internal/storage"
	"testing"
)

func TestRejectsFutureSchemaWithoutClearingData(t *testing.T) {
	paths := storage.PathsFromDataDir(t.TempDir())
	store, err := storage.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.DB.Exec("INSERT INTO schema_migrations VALUES(99,'future'); INSERT INTO app_settings VALUES('keep','true','1')"); err != nil {
		t.Fatal(err)
	}
	// A second open must reject the version while the original connection remains inspectable.
	if other, err := storage.Open(paths); err == nil {
		other.Close()
		t.Fatal("future schema was accepted")
	}
	var value string
	if err = store.DB.QueryRow("SELECT value_json FROM app_settings WHERE key='keep'").Scan(&value); err != nil || value != "true" {
		t.Fatal(value, err)
	}
	store.Close()
}
