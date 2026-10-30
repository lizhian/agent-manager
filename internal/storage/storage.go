// Package storage owns the compatible SQLite schema and explicit application paths.
package storage

import (
	"database/sql"
	_ "embed"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed schema.sql
var schema string

type Paths struct{ Root, AgentsDir string }

func PathsFromHome(home string) Paths {
	return Paths{Root: filepath.Join(home, ".agent-manager"), AgentsDir: filepath.Join(home, ".agents")}
}
func PathsFromDataDir(dir string) Paths { return PathsFromHome(dir) }

// PathsForRun keeps MyGo's development reloads on a persistent, isolated store.
// The CLI launches the application with the project directory as its working directory.
func PathsForRun(home, workingDir, dataDir string, development bool) Paths {
	if dataDir == "" {
		if !development {
			return PathsFromHome(home)
		}
		dataDir = filepath.Join(".mygo", "data")
	}
	if !filepath.IsAbs(dataDir) {
		dataDir = filepath.Join(workingDir, dataDir)
	}
	return PathsFromDataDir(dataDir)
}

type Store struct {
	DB    *sql.DB
	Paths Paths
}

func Open(paths Paths) (*Store, error) {
	if err := os.MkdirAll(paths.Root, 0700); err != nil {
		return nil, fmt.Errorf("创建数据库目录 %s 失败：%w", paths.Root, err)
	}
	u := url.URL{Scheme: "file", Path: filepath.Join(paths.Root, "agent-manager.db")}
	q := u.Query()
	q.Set("_foreign_keys", "on")
	q.Set("_busy_timeout", "5000")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite3", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	fail := func(err error) (*Store, error) {
		db.Close()
		return nil, fmt.Errorf("打开数据库 %s 失败：%w", u.Path, err)
	}
	var exists int
	if err = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'").Scan(&exists); err != nil {
		return fail(err)
	}
	if exists != 0 {
		var version int
		if err = db.QueryRow("SELECT coalesce(max(version),0) FROM schema_migrations").Scan(&version); err != nil {
			return fail(err)
		}
		if version > 4 {
			return fail(fmt.Errorf("数据库版本 %d 高于支持的版本 4", version))
		}
	}
	tx, err := db.Begin()
	if err != nil {
		return fail(err)
	}
	if _, err = tx.Exec(schema); err != nil {
		tx.Rollback()
		return fail(err)
	}
	if err = tx.Commit(); err != nil {
		return fail(err)
	}
	return &Store{DB: db, Paths: paths}, nil
}
func (s *Store) Close() error { return s.DB.Close() }
