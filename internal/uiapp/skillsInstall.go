package uiapp

import (
	"context"
	"fmt"
	"github.com/egoist/mygo/ui"
	"github.com/lizhian/agent-manager/internal/skills"
)

func (w *SkillsWorkspace) installDialog(c *ui.Context) {
	wasOpen := w.installOpen
	ui.Modal(c, &w.installOpen, func() {
		ui.Column(c).Width(400).Padding(12).Gap(8).Background(c.Theme().Background).Children(func() {
			ui.Text(c, "安装 Skill 来源").Bold()
			ui.TextInput(c, &w.installSource).Label("GitHub 来源").Placeholder("owner/repo").Height(28).Disabled(w.writing != "").AutoFocus()
			if w.err != "" {
				ui.Text(c, w.err).TextColor(c.Theme().Danger)
			}
			if w.progress != "" {
				ui.Text(c, w.progress)
			}
			ui.Row(c).Gap(4).Children(func() {
				if ui.Button(c, "关闭").Height(28).Clicked() {
					w.installOpen = false
				}
				if ui.PrimaryButton(c, "开始安装").Height(28).Disabled(w.writing != "" || w.installSource == "").Clicked() {
					w.syncSource(w.installSource)
				}
			})
		})
	})
	if wasOpen && !w.installOpen {
		w.installTrigger.Focus(c)
	}
}
func (w *SkillsWorkspace) syncSource(source string) {
	if w.writing != "" || w.post == nil {
		return
	}
	w.writing = "source:" + source
	w.progress = "正在准备来源同步…"
	w.err = ""
	go func() {
		result, err := w.service.SyncSource(context.Background(), source, func(p skills.Progress) {
			w.post(func() {
				w.progress = p.Message
				if p.Percent >= 0 {
					w.progress = fmt.Sprintf("%s · %d%%", p.Message, p.Percent)
				}
			})
		})
		w.post(func() {
			w.writing = ""
			if err != nil {
				w.err = err.Error()
				w.progress = "来源同步失败"
				return
			}
			w.snapshot = result.Snapshot
			w.status = "ready"
			w.progress = fmt.Sprintf("同步完成：%d 个 Skills，清理 %d 个失效项", result.InstalledCount, result.RemovedCount)
			wasInstallOpen := w.installOpen
			w.installOpen = false
			if wasInstallOpen {
				w.installTrigger.Focus()
			}
			if w.detail != nil {
				for _, s := range result.Snapshot.Sources {
					if s.SourceSafe == w.detail.SourceSafe {
						for _, skill := range s.Skills {
							if skill.Name == w.detail.Name {
								w.selectSkill(s.SourceSafe, skill.Name)
								return
							}
						}
					}
				}
			}
			w.detail = nil
			for _, s := range result.Snapshot.Sources {
				if s.SourceSafe == result.SourceSafe && len(s.Skills) > 0 {
					w.selectSkill(s.SourceSafe, s.Skills[0].Name)
					return
				}
			}
		})
	}()
}

func (w *SkillsWorkspace) toggleSkill(source, name string, enabled bool) {
	if w.writing != "" || w.post == nil {
		return
	}
	w.writing = source + "/" + name
	w.err = ""
	go func() {
		snap, err := w.service.SetGlobalEnabled(source, name, enabled)
		w.post(func() {
			w.writing = ""
			if err != nil {
				w.err = err.Error()
				return
			}
			w.snapshot = snap
			if w.detail != nil && w.detail.SourceSafe == source && w.detail.Name == name {
				for _, s := range snap.Sources {
					if s.SourceSafe == source {
						for _, skill := range s.Skills {
							if skill.Name == name {
								w.detail.GlobalEnabled = skill.GlobalEnabled
							}
						}
					}
				}
			}
		})
	}()
}
