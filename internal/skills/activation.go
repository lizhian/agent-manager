package skills

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// SetGlobalEnabled changes only the selected catalog-derived link. The SQL
// transaction stays open until that filesystem operation succeeds.
func (s *Service) SetGlobalEnabled(sourceSafe, name string, enabled bool) (snapshot Snapshot, result error) {
	if !s.writeMu.TryLock() {
		return Snapshot{}, fmt.Errorf("更新 Skill %s/%s 全局状态失败：另一个写入任务正在运行", sourceSafe, name)
	}
	defer s.writeMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	fail := func(err error) (Snapshot, error) {
		return Snapshot{}, fmt.Errorf("更新 Skill %s/%s 全局状态失败：%w", sourceSafe, name, err)
	}
	if err := s.migrateLegacy(); err != nil {
		return fail(err)
	}
	source, skill, err := s.registered(sourceSafe, name)
	if err != nil {
		return fail(err)
	}
	if !validComponent(source.SourceSafe) || !validComponent(name) {
		return fail(fmt.Errorf("受管链接名称无效"))
	}
	target := filepath.Join(s.RepositoryPath(), sourceSafe, name)
	if enabled {
		p, err := s.documentPath(source, name, name+"/SKILL.md")
		if err != nil {
			return fail(err)
		}
		expected, err := filepath.EvalSymlinks(target)
		if err != nil || !within(expected, p) {
			return fail(fmt.Errorf("Skill 镜像目录无效"))
		}
	}
	dir := s.GlobalTargetPath()
	if info, e := os.Lstat(dir); e == nil && info.Mode()&os.ModeSymlink != 0 {
		return fail(fmt.Errorf("激活目录 %s 是软链，已保留", dir))
	} else if e != nil && !errors.Is(e, os.ErrNotExist) {
		return fail(e)
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return fail(err)
	}
	link := filepath.Join(dir, sourceSafe+"__"+name)
	exists := false
	if info, e := os.Lstat(link); e == nil {
		actual, readErr := os.Readlink(link)
		if info.Mode()&os.ModeSymlink == 0 || readErr != nil || actual != target {
			return fail(fmt.Errorf("受管路径 %s 被手工文件或其他软链占用，已保留", link))
		}
		exists = true
	} else if !errors.Is(e, os.ErrNotExist) {
		return fail(e)
	}
	tx, err := s.store.DB.Begin()
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec("UPDATE skills SET global_enabled=? WHERE id=?", enabled, skill.ID); err != nil {
		return fail(err)
	}
	changed := false
	defer func() {
		if result != nil && changed {
			var rollbackErr error
			if exists {
				rollbackErr = os.Symlink(target, link)
			} else {
				rollbackErr = os.Remove(link)
			}
			if rollbackErr != nil {
				result = errors.Join(result, fmt.Errorf("恢复受管软链 %s 失败：%w", link, rollbackErr))
			}
		}
	}()
	if enabled && !exists {
		if err = s.options.Symlink(target, link); err != nil {
			return fail(err)
		}
		changed = true
	}
	if !enabled && exists {
		if err = os.Remove(link); err != nil {
			return fail(err)
		}
		changed = true
	}
	if err = tx.Commit(); err != nil {
		return fail(err)
	}
	// A committed operation is authoritative; a subsequent read failure must not
	// undo its links without also undoing the database.
	changed = false
	snapshot, err = s.snapshot()
	if err != nil {
		return fail(err)
	}
	return snapshot, nil
}
