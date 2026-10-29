package agentsmd

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// Reorder accepts the entire catalog so a filtered or stale list cannot drop entries.
func (s *Service) Reorder(ids []int64) (Snapshot, error) {
	return s.mutate("保存 AGENTS.md 片段排序", func(tx *sql.Tx) error {
		snapshot, err := s.read(tx)
		if err != nil {
			return err
		}
		if len(ids) != len(snapshot.Fragments) {
			return fmt.Errorf("片段排序列表不完整，请刷新后重试")
		}
		existing := make(map[int64]bool, len(ids))
		for _, f := range snapshot.Fragments {
			existing[f.ID] = true
		}
		for _, id := range ids {
			if !existing[id] {
				return fmt.Errorf("片段排序包含重复或未知标识 %d，请刷新后重试", id)
			}
			existing[id] = false
		}
		for i, id := range ids {
			if _, err = tx.Exec("UPDATE agents_md_fragments SET sort_order=? WHERE id=?", i, id); err != nil {
				return err
			}
		}
		return nil
	})
}
func (s *Service) OpenFolder() error {
	path := s.store.Paths.AgentsDir
	if err := os.MkdirAll(path, 0700); err != nil {
		return fmt.Errorf("创建 AGENTS.md 目标目录 %s 失败：%w", path, err)
	}
	if err := s.openFolder(path); err != nil {
		return fmt.Errorf("打开 AGENTS.md 目标目录 %s 失败：%w", path, err)
	}
	return nil
}
func openFolder(path string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("当前平台不支持打开目录")
	}
	output, err := exec.Command("open", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("open 返回 %w：%s", err, output)
	}
	return nil
}
