package uiapp

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"github.com/lizhian/agent-manager/internal/settings"
	"strings"
)

// SystemFontNames bridges the native toolkit's enumeration into the settings service.
func SystemFontNames() ([]string, error) {
	families := ui.FontFamilies()
	out := make([]string, 0, len(families))
	for _, family := range families {
		out = append(out, family.Name)
	}
	return out, nil
}

type SettingsWorkspace struct {
	service              *settings.Service
	post                 func(func())
	applied              func(settings.Snapshot)
	snapshot, draft      settings.Snapshot
	status               string
	busy                 bool
	err, fontsErr, query string
	fonts                []string
	pending              *settings.Snapshot
}

func NewSettingsWorkspace(service *settings.Service) *SettingsWorkspace {
	return &SettingsWorkspace{service: service, status: "loading", snapshot: settings.Defaults(), draft: settings.Defaults()}
}
func (w *SettingsWorkspace) Load(post func(func()), applied func(settings.Snapshot)) {
	w.post = post
	w.applied = applied
	w.status = "loading"
	go func() {
		snap, err := w.service.Snapshot()
		post(func() {
			if err != nil {
				w.err = err.Error()
				w.status = "error"
				return
			}
			w.snapshot = snap
			w.draft = snap
			w.status = "ready"
			applied(snap)
		})
	}()
	w.loadFonts()
}
func (w *SettingsWorkspace) loadFonts() {
	w.fontsErr = ""
	go func() {
		fonts, err := w.service.Fonts()
		w.post(func() {
			if err != nil {
				w.fontsErr = err.Error()
				return
			}
			w.fonts = fonts
		})
	}()
}
func (w *SettingsWorkspace) save(next settings.Snapshot) {
	w.draft = next
	if w.busy {
		w.pending = &next
		return
	}
	w.busy = true
	w.err = ""
	go func() {
		saved, err := w.service.Save(next)
		w.post(func() {
			w.busy = false
			if err != nil {
				w.err = err.Error()
				if w.pending != nil {
					w.draft = *w.pending
					w.pending = nil
				}
				return
			}
			w.snapshot = saved
			w.applied(saved)
			if w.pending != nil {
				pending := *w.pending
				w.pending = nil
				w.save(pending)
			} else {
				w.draft = saved
			}
		})
	}()
}

// SetPreview receives a workspace gesture; writes serialize and retain the latest gesture.
func (w *SettingsWorkspace) SetPreview(position string, ratio float64) {
	next := w.draft
	next.Preview = settings.Preview{Position: position, Ratio: ratio}
	w.save(next)
}
func (w *SettingsWorkspace) View(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Padding(8).Gap(8).Children(func() {
		ui.Text(c, "设置").FontSize(t.FontSize + 3).Bold()
		if w.err != "" {
			ui.Text(c, w.err).TextColor(t.Danger)
		}
		if w.status == "loading" {
			ui.Text(c, "正在加载设置…")
			return
		}
		if w.status == "error" {
			if ui.Button(c, "重新加载设置").Clicked() {
				w.Load(w.post, w.applied)
			}
			return
		}
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).WidthPercent(100).MaxWidth(720).Gap(8).Children(func() {
				ui.Row(c).Gap(8).Children(func() {
					ui.Text(c, "界面字号").Width(100)
					size := 0
					for i, value := range settings.Sizes {
						if value == w.draft.FontSize {
							size = i
						}
					}
					if ui.Segmented(c, &size, "特小", "小", "标准", "大", "特大").Label("界面字号").Changed() {
						next := w.draft
						next.FontSize = settings.Sizes[size]
						w.save(next)
					}
				})
				ui.Row(c).Gap(8).Children(func() {
					ui.Text(c, "预览位置").Width(100)
					position := 0
					if w.draft.Preview.Position == "bottom" {
						position = 1
					}
					if ui.Segmented(c, &position, "右侧", "底部").Label("文档预览位置").Changed() {
						next := w.draft
						next.Preview.Position = "right"
						if position == 1 {
							next.Preview.Position = "bottom"
						}
						w.save(next)
					}
				})
				ui.Row(c).Gap(8).Children(func() {
					ui.Text(c, "预览比例").Width(100)
					ratio := w.draft.Preview.Ratio * 100
					if ui.Slider(c, &ratio, 20, 80).Label("文档预览比例").Width(240).Changed() {
						next := w.draft
						next.Preview.Ratio = float64(ratio) / 100
						w.save(next)
					}
					ui.Text(c, fmt.Sprintf("%.0f%%", w.draft.Preview.Ratio*100)).Width(44)
				})
				ui.Row(c).Gap(8).Children(func() {
					ui.Text(c, "界面字体").Width(100)
					family := w.draft.FontFamily
					if family == "" {
						family = "系统默认"
					}
					ui.Text(c, family).Grow(1)
					if ui.Button(c, "使用系统默认").Height(28).Clicked() {
						next := w.draft
						next.FontFamily = ""
						w.save(next)
					}
				})
				ui.SearchField(c, &w.query).Label("搜索系统字体").Placeholder("搜索系统字体").Height(28)
				if w.fontsErr != "" {
					ui.Text(c, w.fontsErr).TextColor(t.Danger)
					if ui.Button(c, "重试字体枚举").Height(28).Clicked() {
						w.loadFonts()
					}
				}
				ui.Column(c).Gap(2).Children(func() {
					count := 0
					for _, family := range w.fonts {
						if !strings.Contains(strings.ToLower(family), strings.ToLower(w.query)) {
							continue
						}
						count++
						button := ui.Button(c, family).Height(28)
						if family == w.draft.FontFamily {
							button.Border(1, t.Accent)
						}
						if button.Clicked() {
							next := w.draft
							next.FontFamily = family
							w.save(next)
						}
					}
					if len(w.fonts) > 0 && count == 0 {
						ui.Text(c, "没有匹配字体，请清空搜索。")
					}
				})
				if w.busy {
					ui.Text(c, "正在保存设置…").TextColor(t.TextMuted)
				} else if w.err != "" {
					if ui.Button(c, "重试保存设置").Height(28).Clicked() {
						w.save(w.draft)
					}
				} else {
					ui.Text(c, "设置已保存").TextColor(t.TextMuted)
				}
			})
		})
	})
}
