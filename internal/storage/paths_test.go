package storage_test

import (
	"path/filepath"
	"testing"

	"github.com/lizhian/agent-manager/internal/storage"
)

func TestRunPathsIsolateDevelopmentAndHonorExplicitDirectory(t *testing.T) {
	home, project, explicit := t.TempDir(), t.TempDir(), t.TempDir()
	for _, tc := range []struct {
		name, dataDir, wantRoot string
		development             bool
	}{
		{"normal", "", home, false},
		{"development", "", filepath.Join(project, ".mygo", "data"), true},
		{"explicit development directory", explicit, explicit, true},
		{"relative directory", "fixture", filepath.Join(project, "fixture"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := storage.PathsForRun(home, project, tc.dataDir, tc.development)
			if want := storage.PathsFromDataDir(tc.wantRoot); got != want {
				t.Fatalf("paths = %+v, want %+v", got, want)
			}
		})
	}
}
