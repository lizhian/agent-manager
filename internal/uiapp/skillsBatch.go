package uiapp

import (
	"context"
	"fmt"
	"github.com/egoist/mygo/ui"
	"github.com/lizhian/agent-manager/internal/skills"
	"strings"
)

func (w *SkillsWorkspace) updateAll() {
	if w.writing != "" || w.post == nil {
		return
	}
	w.writing = "batch"
	w.progress = "正在准备全部更新…"
	w.err = ""
	go func() {
		result, err := w.service.UpdateAll(context.Background(), func(p skills.Progress) {
			w.post(func() {
				percent := 0
				if p.TotalSources > 0 {
					percent = (p.CompletedSources * 100) / p.TotalSources
				}
				w.progress = fmt.Sprintf("全部更新 %d/%d · 总进度 %d%% · %s：%s", p.CompletedSources, p.TotalSources, percent, p.Source, p.Message)
			})
		})
		w.post(func() {
			w.writing = ""
			if err != nil {
				w.err = err.Error()
				return
			}
			w.progress = fmt.Sprintf("全部更新完成：成功 %d，失败 %d，安装 %d，移除 %d", result.SucceededSources, result.FailedSources, result.InstalledCount, result.RemovedCount)
			var failures []string
			for _, f := range result.Failures {
				failures = append(failures, f.Source+"："+f.Error)
			}
			w.refreshSkills(result.Snapshot, "")
			w.err = strings.Join(failures, "\n")
		})
	}()
}
func (w *SkillsWorkspace) refreshSkills(snap skills.Snapshot, prefer string) {
	w.snapshot = snap
	w.status = "ready"
	if w.detail != nil {
		for _, source := range snap.Sources {
			if source.SourceSafe == w.detail.SourceSafe {
				for _, skill := range source.Skills {
					if skill.Name == w.detail.Name {
						w.selectSkill(source.SourceSafe, skill.Name)
						return
					}
				}
			}
		}
	}
	w.request++
	w.docRequest++
	w.detail = nil
	w.document = nil
	w.detailLoading = false
	w.documentLoading = false
	for _, source := range snap.Sources {
		if (prefer == "" || source.SourceSafe == prefer) && len(source.Skills) > 0 {
			w.selectSkill(source.SourceSafe, source.Skills[0].Name)
			return
		}
	}
}
func (w *SkillsWorkspace) prepareDelete(safe string) {
	if w.writing != "" || w.post == nil {
		return
	}
	w.writing = "impact"
	w.deleteSourceOpen = true
	w.deleteImpact = nil
	w.err = ""
	go func() {
		impact, err := w.service.RemoveImpact(safe)
		w.post(func() {
			w.writing = ""
			if err != nil {
				w.err = err.Error()
				return
			}
			w.deleteImpact = &impact
		})
	}()
}
func (w *SkillsWorkspace) sourceDeleteDialog(c *ui.Context) {
	wasOpen := w.deleteSourceOpen
	ui.Modal(c, &w.deleteSourceOpen, func() {
		ui.Column(c).Width(420).Padding(12).Gap(8).Background(c.Theme().Background).Children(func() {
			ui.Text(c, "删除来源？").Bold()
			if w.deleteImpact == nil {
				ui.Text(c, "正在读取删除影响…")
			} else {
				impact := *w.deleteImpact
				ui.Text(c, impact.Source)
				ui.Text(c, fmt.Sprintf("将删除 %d 个 Skills，移除 %d 个全局启用。", impact.InstalledSkills, impact.GlobalEnabledSkills))
				ui.Text(c, "本地镜像与来源记录将被删除，此操作无法撤销。").FontSize(13)
			}
			if w.err != "" {
				ui.Text(c, w.err).TextColor(c.Theme().Danger)
			}
			ui.Row(c).Gap(4).Children(func() {
				if ui.Button(c, "取消删除").AutoFocus().Height(28).Disabled(w.writing == "delete").Clicked() {
					w.deleteSourceOpen = false
				}
				if ui.Button(c, "确认删除来源").TextColor(c.Theme().Danger).Height(28).Disabled(w.writing != "" || w.deleteImpact == nil).Clicked() {
					safe := w.deleteImpact.SourceSafe
					w.writing = "delete"
					w.err = ""
					go func() {
						snap, err := w.service.RemoveSource(safe)
						w.post(func() {
							w.writing = ""
							if err != nil {
								w.err = err.Error()
								w.deleteSourceOpen = true
								return
							}
							w.deleteSourceOpen = false
							w.progress = "来源已删除"
							w.installTrigger.Focus()
							w.refreshSkills(snap, "")
						})
					}()
				}
			})
		})
	})
	if wasOpen && !w.deleteSourceOpen && w.deleteSourceTrigger != nil {
		w.deleteSourceTrigger.Focus(c)
	}
}

func (w *SkillsWorkspace) sourceDeleteHandle(safe string) *ui.Handle {
	if w.deleteHandles == nil {
		w.deleteHandles = map[string]*ui.Handle{}
	}
	if w.deleteHandles[safe] == nil {
		w.deleteHandles[safe] = new(ui.Handle)
	}
	return w.deleteHandles[safe]
}
