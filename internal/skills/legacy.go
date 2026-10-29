package skills

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type legacyCatalog struct {
	Sources []legacySource `json:"sources"`
}
type legacySource struct {
	Source      string        `json:"source"`
	SourceSafe  string        `json:"sourceSafe"`
	InstalledAt string        `json:"installedAt"`
	UpdatedAt   string        `json:"updatedAt"`
	Skills      []legacySkill `json:"skills"`
}
type legacySkill struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	GlobalEnabled bool     `json:"globalEnabled"`
	InstalledAt   string   `json:"installedAt"`
	UpdatedAt     string   `json:"updatedAt"`
	ProjectPaths  []string `json:"projectPaths"`
}
type migratedLink struct{ path, target string }

func (s *Service) migrateLegacy() (result error) {
	legacy := filepath.Join(s.store.Paths.Root, "install-skills.json")
	data, err := os.ReadFile(legacy)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取旧 Skills catalog 失败：%w", err)
	}
	var catalog legacyCatalog
	if err = json.Unmarshal(data, &catalog); err != nil {
		return fmt.Errorf("解析旧 Skills catalog 失败：%w", err)
	}
	var links []migratedLink
	seen := map[string]bool{}
	for i := range catalog.Sources {
		source := &catalog.Sources[i]
		source.Source = strings.TrimSpace(source.Source)
		source.SourceSafe = strings.TrimSpace(source.SourceSafe)
		if !validComponent(source.SourceSafe) {
			return fmt.Errorf("旧 Skills 来源安全名无效：%s", source.SourceSafe)
		}
		for j := range source.Skills {
			skill := &source.Skills[j]
			skill.Name = strings.TrimSpace(skill.Name)
			skill.Description = strings.TrimSpace(skill.Description)
			if !validComponent(skill.Name) {
				return fmt.Errorf("旧 Skill 名称无效：%s", skill.Name)
			}
			for _, project := range skill.ProjectPaths {
				p := filepath.Join(project, ".agents", "skills", source.SourceSafe+"__"+skill.Name)
				if seen[p] {
					continue
				}
				seen[p] = true
				info, e := os.Lstat(p)
				if errors.Is(e, os.ErrNotExist) {
					continue
				}
				if e != nil {
					return fmt.Errorf("检查旧项目 Skill %s 失败：%w", p, e)
				}
				target := filepath.Join(s.RepositoryPath(), source.SourceSafe, skill.Name)
				actual, e := os.Readlink(p)
				if info.Mode()&os.ModeSymlink == 0 || e != nil || actual != target {
					return fmt.Errorf("旧项目路径 %s 不是匹配的受管软链，已保留", p)
				}
				links = append(links, migratedLink{p, target})
			}
		}
	}
	tx, err := s.store.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, source := range catalog.Sources {
		if _, err = tx.Exec(`INSERT INTO skill_sources(source,source_safe,installed_at,updated_at) VALUES(?,?,?,?) ON CONFLICT(source_safe) DO UPDATE SET source=excluded.source,installed_at=excluded.installed_at,updated_at=excluded.updated_at`, source.Source, source.SourceSafe, source.InstalledAt, source.UpdatedAt); err != nil {
			return fmt.Errorf("迁移来源 %s 失败：%w", source.Source, err)
		}
		var id int64
		if err = tx.QueryRow("SELECT id FROM skill_sources WHERE source_safe=?", source.SourceSafe).Scan(&id); err != nil {
			return err
		}
		sort.Slice(source.Skills, func(i, j int) bool { return source.Skills[i].Name < source.Skills[j].Name })
		for _, skill := range source.Skills {
			if _, err = tx.Exec(`INSERT INTO skills(source_id,name,description,global_enabled,installed_at,updated_at) VALUES(?,?,?,?,?,?) ON CONFLICT(source_id,name) DO UPDATE SET description=excluded.description,global_enabled=excluded.global_enabled,installed_at=excluded.installed_at,updated_at=excluded.updated_at`, id, skill.Name, skill.Description, skill.GlobalEnabled, skill.InstalledAt, skill.UpdatedAt); err != nil {
				return fmt.Errorf("迁移 Skill %s 失败：%w", skill.Name, err)
			}
		}
	}
	// Keep both the old JSON and every project link until all SQL writes can succeed.
	// Links moved aside can be restored if a later filesystem action or commit fails.
	backups := map[string]string{}
	defer func() {
		if result != nil {
			for original, backup := range backups {
				if e := os.Rename(backup, original); e != nil {
					result = errors.Join(result, fmt.Errorf("恢复迁移文件 %s 失败：%w", original, e))
				}
			}
		} else {
			for _, backup := range backups {
				_ = os.Remove(backup)
			}
		}
	}()
	for _, link := range links {
		if err = backupMigrationFile(link.path, backups); err != nil {
			return err
		}
	}
	if err = backupMigrationFile(legacy, backups); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("提交旧 Skills 迁移失败：%w", err)
	}
	return nil
}
func backupMigrationFile(p string, backups map[string]string) error {
	f, err := os.CreateTemp(filepath.Dir(p), ".agent-manager-migration-*")
	if err != nil {
		return fmt.Errorf("备份迁移文件 %s 失败：%w", p, err)
	}
	backup := f.Name()
	if err = f.Close(); err != nil {
		os.Remove(backup)
		return err
	}
	if err = os.Rename(p, backup); err != nil {
		os.Remove(backup)
		return fmt.Errorf("备份迁移文件 %s 失败：%w", p, err)
	}
	backups[p] = backup
	return nil
}
