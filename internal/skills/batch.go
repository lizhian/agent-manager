package skills

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type BatchFailure struct{ Source, Error string }
type BatchResult struct {
	Snapshot                                                                    Snapshot
	TotalSources, SucceededSources, FailedSources, InstalledCount, RemovedCount int
	Failures                                                                    []BatchFailure
}
type RemoveImpact struct {
	Source, SourceSafe                   string
	InstalledSkills, GlobalEnabledSkills int
}

func (s *Service) UpdateAll(ctx context.Context, progress func(Progress)) (BatchResult, error) {
	if !s.writeMu.TryLock() {
		return BatchResult{}, fmt.Errorf("全部更新失败：另一个 Skills 写入任务正在运行")
	}
	defer s.writeMu.Unlock()
	snap, err := s.Snapshot()
	if err != nil {
		return BatchResult{}, err
	}
	result := BatchResult{Snapshot: snap, TotalSources: len(snap.Sources)}
	for i, source := range snap.Sources {
		sync, err := s.syncSource(ctx, source.Source, func(p Progress) {
			p.CompletedSources = i
			p.TotalSources = result.TotalSources
			if progress != nil {
				progress(p)
			}
		})
		if err != nil {
			result.FailedSources++
			result.Failures = append(result.Failures, BatchFailure{source.Source, err.Error()})
		} else {
			result.SucceededSources++
			result.InstalledCount += sync.InstalledCount
			result.RemovedCount += sync.RemovedCount
			result.Snapshot = sync.Snapshot
		}
		if progress != nil {
			progress(Progress{Source: source.Source, Stage: "source-complete", Message: "当前来源处理完成", Percent: 100, CompletedSources: i + 1, TotalSources: result.TotalSources})
		}
	}
	return result, nil
}
func (s *Service) RemoveImpact(safe string) (RemoveImpact, error) {
	snap, err := s.Snapshot()
	if err != nil {
		return RemoveImpact{}, err
	}
	for _, source := range snap.Sources {
		if source.SourceSafe == safe {
			result := RemoveImpact{Source: source.Source, SourceSafe: safe, InstalledSkills: len(source.Skills)}
			for _, skill := range source.Skills {
				if skill.GlobalEnabled {
					result.GlobalEnabledSkills++
				}
			}
			return result, nil
		}
	}
	return RemoveImpact{}, fmt.Errorf("未找到来源 %s", safe)
}
func (s *Service) RemoveSource(safe string) (snapshot Snapshot, result error) {
	if !s.writeMu.TryLock() {
		return Snapshot{}, fmt.Errorf("删除来源 %s 失败：另一个 Skills 写入任务正在运行", safe)
	}
	defer s.writeMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	defer func() {
		if result != nil {
			result = fmt.Errorf("删除来源 %s 失败：%w", safe, result)
		}
	}()
	if !validComponent(safe) {
		return Snapshot{}, fmt.Errorf("来源安全名无效")
	}
	snap, err := s.snapshot()
	if err != nil {
		return Snapshot{}, err
	}
	var source *Source
	for _, v := range snap.Sources {
		v := v
		if v.SourceSafe == safe {
			source = &v
		}
	}
	if source == nil {
		return Snapshot{}, fmt.Errorf("未找到来源")
	}
	dir := s.GlobalTargetPath()
	if info, e := os.Lstat(dir); e == nil && info.Mode()&os.ModeSymlink != 0 {
		return Snapshot{}, fmt.Errorf("全局激活目录是软链，已保留")
	}
	owned := map[string]string{}
	for _, skill := range source.Skills {
		if !validComponent(skill.Name) {
			return Snapshot{}, fmt.Errorf("Skill 名称无效：%s", skill.Name)
		}
		link := filepath.Join(dir, safe+"__"+skill.Name)
		target := filepath.Join(s.RepositoryPath(), safe, skill.Name)
		info, e := os.Lstat(link)
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			return Snapshot{}, e
		}
		actual, e := os.Readlink(link)
		if info.Mode()&os.ModeSymlink == 0 || e != nil || actual != target {
			return Snapshot{}, fmt.Errorf("受管路径 %s 被手工内容占用，已保留", link)
		}
		owned[link] = target
	}
	final := filepath.Join(s.RepositoryPath(), safe)
	hadMirror := false
	if info, e := os.Lstat(final); e == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return Snapshot{}, fmt.Errorf("镜像路径不是受管目录，已保留：%s", final)
		}
		hadMirror = true
	} else if !errors.Is(e, os.ErrNotExist) {
		return Snapshot{}, e
	}
	tx, err := s.store.DB.Begin()
	if err != nil {
		return Snapshot{}, err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("DELETE FROM skill_sources WHERE id=?", source.ID); err != nil {
		return Snapshot{}, err
	}
	backup := ""
	if hadMirror {
		backup, err = os.MkdirTemp(s.RepositoryPath(), ".delete-*")
		if err != nil {
			return Snapshot{}, err
		}
		os.Remove(backup)
		if err = s.options.Rename(final, backup); err != nil {
			return Snapshot{}, err
		}
	}
	committed := false
	removed := map[string]string{}
	defer func() {
		if !committed {
			for link, target := range removed {
				if err := os.Symlink(target, link); err != nil {
					result = errors.Join(result, fmt.Errorf("恢复全局链接 %s 失败：%w", link, err))
				}
			}
			if hadMirror {
				if err := os.Rename(backup, final); err != nil {
					result = errors.Join(result, fmt.Errorf("恢复镜像 %s 失败：%w", final, err))
				}
			}
		} else if backup != "" {
			_ = os.RemoveAll(backup)
		}
	}()
	for link, target := range owned {
		if err = os.Remove(link); err != nil {
			return Snapshot{}, err
		}
		removed[link] = target
	}
	if err = tx.Commit(); err != nil {
		return Snapshot{}, err
	}
	committed = true
	return s.snapshot()
}
