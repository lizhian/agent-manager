package skills

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Progress struct {
	Source, Stage, Message string
	Percent                int
}
type SyncResult struct {
	Snapshot                     Snapshot
	SourceSafe                   string
	InstalledCount, RemovedCount int
	Updated                      bool
}
type preparedSkill struct{ Name, Path string }

var sourcePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`)

func (s *Service) SyncSource(ctx context.Context, source string, progress func(Progress)) (result SyncResult, err error) {
	source = strings.TrimSpace(source)
	parts := strings.Split(source, "/")
	if !sourcePattern.MatchString(source) || parts[0] == "." || parts[0] == ".." || parts[1] == "." || parts[1] == ".." {
		return result, fmt.Errorf("来源格式无效，请输入 owner/repo")
	}
	if !s.writeMu.TryLock() {
		return result, fmt.Errorf("同步来源 %s 失败：另一个 Skills 写入任务正在运行", source)
	}
	defer s.writeMu.Unlock()
	defer func() {
		if err != nil {
			err = fmt.Errorf("同步来源 %s 失败：%w", source, err)
		}
	}()
	send := func(stage, message string, percent int) {
		if progress != nil {
			progress(Progress{source, stage, message, percent})
		}
	}
	send("prepare", "正在准备临时目录", 10)
	temp, err := os.MkdirTemp("", "agent-manager-skills-*")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(temp)
	temp, err = filepath.EvalSymlinks(temp)
	if err != nil {
		return result, err
	}
	send("download", "正在下载并解析来源", -1)
	if _, err = s.options.Runner(ctx, temp, []string{"add", source, "-y"}); err != nil {
		return result, err
	}
	data, err := s.options.Runner(ctx, temp, []string{"list", "--json"})
	if err != nil {
		return result, err
	}
	start := strings.IndexByte(string(data), '[')
	if start < 0 {
		return result, fmt.Errorf("npx skills list 未返回 JSON 数组")
	}
	var listed []preparedSkill
	if err = json.Unmarshal(data[start:], &listed); err != nil {
		return result, fmt.Errorf("解析 npx skills 输出失败：%w", err)
	}
	if len(listed) == 0 {
		return result, fmt.Errorf("没有可安装的 Skill")
	}
	sort.Slice(listed, func(i, j int) bool { return listed[i].Name < listed[j].Name })
	send("parse", "正在验证 Skill 路径和元信息", 48)
	seen := map[string]bool{}
	for i := range listed {
		skill := &listed[i]
		skill.Name = strings.TrimSpace(skill.Name)
		if !validComponent(skill.Name) || seen[skill.Name] {
			return result, fmt.Errorf("Skill 名称无效或重复：%s", skill.Name)
		}
		seen[skill.Name] = true
		skill.Path, err = filepath.EvalSymlinks(skill.Path)
		if err != nil || !within(temp, skill.Path) {
			return result, fmt.Errorf("Skill %s 的路径超出临时安装目录", skill.Name)
		}
	}
	if err = os.MkdirAll(s.RepositoryPath(), 0700); err != nil {
		return result, err
	}
	stage, err := os.MkdirTemp(s.RepositoryPath(), ".staging-*")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(stage)
	send("mirror", "正在准备本地镜像", 62)
	for _, skill := range listed {
		if err = copyPrepared(skill.Path, filepath.Join(stage, skill.Name), temp, map[string]bool{}); err != nil {
			return result, err
		}
		if info, e := os.Stat(filepath.Join(stage, skill.Name, "SKILL.md")); e != nil || !info.Mode().IsRegular() {
			return result, fmt.Errorf("Skill %s 缺少 SKILL.md", skill.Name)
		}
	}
	// Downloading and copying don't hold the catalog read lock: details remain usable.
	s.mu.Lock()
	defer s.mu.Unlock()
	if err = s.migrateLegacy(); err != nil {
		return result, err
	}
	old, err := s.snapshot()
	if err != nil {
		return result, err
	}
	safe := strings.ReplaceAll(source, "/", "_")
	var previous *Source
	for _, v := range old.Sources {
		v := v
		if v.SourceSafe == safe {
			if v.Source != source {
				return result, fmt.Errorf("来源安全名 %s 与 %s 冲突", safe, v.Source)
			}
			previous = &v
		}
	}
	send("catalog", "正在保存 catalog", 78)
	return s.commitSource(source, safe, stage, listed, previous, send)
}
func copyPrepared(from, to, boundary string, visiting map[string]bool) error {
	resolved, err := filepath.EvalSymlinks(from)
	if err != nil {
		return err
	}
	if !within(boundary, resolved) {
		return fmt.Errorf("拒绝复制临时目录外的路径 %s", from)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return err
	}
	if info.Mode().IsRegular() {
		data, err := os.ReadFile(resolved)
		if err != nil {
			return err
		}
		return os.WriteFile(to, data, info.Mode().Perm()|0600)
	}
	if !info.IsDir() {
		return fmt.Errorf("拒绝复制特殊文件 %s", from)
	}
	if visiting[resolved] {
		return fmt.Errorf("Skill 包含循环软链：%s", from)
	}
	visiting[resolved] = true
	defer delete(visiting, resolved)
	if err = os.MkdirAll(to, 0700); err != nil {
		return err
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err = copyPrepared(filepath.Join(resolved, entry.Name()), filepath.Join(to, entry.Name()), boundary, visiting); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) commitSource(source, safe, stage string, listed []preparedSkill, previous *Source, send func(string, string, int)) (result SyncResult, err error) {
	now := s.options.Now()
	installed := now
	if previous != nil && previous.InstalledAt != "" {
		installed = previous.InstalledAt
	}
	tx, err := s.store.DB.Begin()
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT INTO skill_sources(source,source_safe,installed_at,updated_at) VALUES(?,?,?,?) ON CONFLICT(source_safe) DO UPDATE SET updated_at=excluded.updated_at`, source, safe, installed, now); err != nil {
		return result, err
	}
	var id int64
	if err = tx.QueryRow("SELECT id FROM skill_sources WHERE source_safe=?", safe).Scan(&id); err != nil {
		return result, err
	}
	keep := map[string]bool{}
	desired := map[string]bool{}
	for _, skill := range listed {
		keep[skill.Name] = true
		enabled := false
		first := now
		if previous != nil {
			for _, old := range previous.Skills {
				if old.Name == skill.Name {
					enabled = old.GlobalEnabled
					first = old.InstalledAt
				}
			}
		}
		desired[skill.Name] = enabled
		metaData, e := os.ReadFile(filepath.Join(stage, skill.Name, "SKILL.md"))
		if e != nil {
			return result, e
		}
		meta := metadata(string(metaData))
		if _, err = tx.Exec(`INSERT INTO skills(source_id,name,description,global_enabled,installed_at,updated_at) VALUES(?,?,?,?,?,?) ON CONFLICT(source_id,name) DO UPDATE SET description=excluded.description,global_enabled=excluded.global_enabled,updated_at=excluded.updated_at`, id, skill.Name, meta.Description, enabled, first, now); err != nil {
			return result, err
		}
	}
	removed := 0
	if previous != nil {
		for _, skill := range previous.Skills {
			if !keep[skill.Name] {
				if _, err = tx.Exec("DELETE FROM skills WHERE id=?", skill.ID); err != nil {
					return result, err
				}
				removed++
			}
		}
	}
	// Preflight all owned paths before changing either mirror or links.
	names := map[string]bool{}
	for name := range keep {
		names[name] = true
	}
	if previous != nil {
		for _, skill := range previous.Skills {
			names[skill.Name] = true
		}
	}
	dir := s.GlobalTargetPath()
	if info, e := os.Lstat(dir); e == nil && info.Mode()&os.ModeSymlink != 0 {
		return result, fmt.Errorf("全局激活目录是软链，已保留")
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return result, err
	}
	owned := map[string]bool{}
	for name := range names {
		p := filepath.Join(dir, safe+"__"+name)
		info, e := os.Lstat(p)
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			return result, e
		}
		target, e := os.Readlink(p)
		if info.Mode()&os.ModeSymlink == 0 || e != nil || target != filepath.Join(s.RepositoryPath(), safe, name) {
			return result, fmt.Errorf("受管路径 %s 被手工内容占用，已保留", p)
		}
		owned[name] = true
	}
	final := filepath.Join(s.RepositoryPath(), safe)
	backup, err := os.MkdirTemp(s.RepositoryPath(), ".backup-*")
	if err != nil {
		return result, err
	}
	os.Remove(backup)
	hadMirror := false
	if info, e := os.Lstat(final); e == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return result, fmt.Errorf("来源镜像路径 %s 不是受管目录", final)
		}
		if previous == nil {
			return result, fmt.Errorf("未登记目录 %s 已存在，已保留", final)
		}
		if err = s.options.Rename(final, backup); err != nil {
			return result, err
		}
		hadMirror = true
	} else if !errors.Is(e, os.ErrNotExist) {
		return result, e
	}
	swapped := false
	changed := map[string]bool{}
	committed := false
	defer func() {
		if !committed {
			for name, was := range changed {
				p := filepath.Join(dir, safe+"__"+name)
				var e error
				if was {
					e = os.Symlink(filepath.Join(final, name), p)
				} else {
					e = os.Remove(p)
				}
				if e != nil {
					err = errors.Join(err, fmt.Errorf("恢复全局链接 %s 失败：%w", p, e))
				}
			}
			if swapped {
				if e := os.RemoveAll(final); e != nil {
					err = errors.Join(err, e)
				}
			}
			if hadMirror {
				if e := os.Rename(backup, final); e != nil {
					err = errors.Join(err, fmt.Errorf("恢复旧镜像失败：%w", e))
				}
			}
		} else {
			_ = os.RemoveAll(backup)
		}
	}()
	if err = s.options.Rename(stage, final); err != nil {
		return result, err
	}
	swapped = true
	send("global", "正在同步全局 Skills", 90)
	for name := range names {
		p := filepath.Join(dir, safe+"__"+name)
		if desired[name] && !owned[name] {
			if err = s.options.Symlink(filepath.Join(final, name), p); err != nil {
				return result, err
			}
			changed[name] = false
		} else if !desired[name] && owned[name] {
			if err = os.Remove(p); err != nil {
				return result, err
			}
			changed[name] = true
		}
	}
	if err = tx.Commit(); err != nil {
		return result, err
	}
	committed = true
	snapshot, err := s.snapshot()
	if err != nil {
		return result, err
	}
	send("complete", "来源同步完成", 100)
	return SyncResult{snapshot, safe, len(listed), removed, previous != nil}, nil
}
