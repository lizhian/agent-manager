package uiapp

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/lizhian/agent-manager/internal/agentsmd"
)

type AgentsWorkspace struct {
	service                   *agentsmd.Service
	post                      func(func())
	snapshot                  agentsmd.Snapshot
	status                    string
	err                       string
	busy                      bool
	selected                  int64
	draftMode                 string
	title, content            string
	combined                  bool
	deleteOpen                bool
	deleteTrigger             ui.Handle
	saveTrigger               ui.Handle
	nameInput                 ui.Handle
	newTrigger                ui.Handle
	listWidth, previewPercent float32
	md                        markdown.Options
}

func NewAgentsWorkspace(service *agentsmd.Service) *AgentsWorkspace {
	return &AgentsWorkspace{service: service, status: "loading", listWidth: 280, previewPercent: 50, md: markdown.Options{OnLink: func(string) {}}}
}
func (w *AgentsWorkspace) Load(post func(func())) {
	w.post = post
	w.status = "loading"
	w.run(w.service.Snapshot, nil)
}
func (w *AgentsWorkspace) run(work func() (agentsmd.Snapshot, error), done func()) {
	if w.busy {
		return
	}
	w.busy = true
	w.err = ""
	go func() {
		snap, err := work()
		w.post(func() {
			w.busy = false
			if err != nil {
				w.err = err.Error()
				if w.status == "loading" {
					w.status = "error"
				}
				return
			}
			w.snapshot = snap
			w.status = "ready"
			if done != nil {
				done()
			}
		})
	}()
}
func (w *AgentsWorkspace) selectFragment(f agentsmd.Fragment) {
	w.selected = f.ID
	w.title = f.Title
	w.content = f.Content
	w.draftMode = "edit"
}
func (w *AgentsWorkspace) save() {
	title, content, id, mode := w.title, w.content, w.selected, w.draftMode
	work := func() (agentsmd.Snapshot, error) {
		if mode == "new" {
			return w.service.Create(title, content)
		}
		return w.service.Update(id, title, content)
	}
	w.run(work, func() {
		if mode == "new" && w.draftMode == "new" && len(w.snapshot.Fragments) > 0 {
			w.selectFragment(w.snapshot.Fragments[len(w.snapshot.Fragments)-1])
		} else if w.selected == id && w.draftMode == mode {
			for _, f := range w.snapshot.Fragments {
				if f.ID == id {
					w.selectFragment(f)
				}
			}
		}
		w.saveTrigger.Focus()
	})
}
func (w *AgentsWorkspace) View(c *ui.Context) {
	t := c.Theme()
	width, _ := c.Size()
	maxList := min(float32(360), width-480)
	w.listWidth = max(float32(240), min(w.listWidth, maxList))
	ui.Column(c).Fill().Padding(8).Gap(6).Children(func() {
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, "AGENTS.md").FontSize(18).Bold()
			ui.Text(c, fmt.Sprintf("%d 个片段 · 已启用 %d 字符", len(w.snapshot.Fragments), w.snapshot.EnabledContentChars)).TextColor(t.TextMuted).Grow(1)
			if ui.Button(c, "新增片段").Height(28).Bind(&w.newTrigger).Disabled(w.busy).Clicked() {
				w.selected = 0
				w.title = ""
				w.content = ""
				w.draftMode = "new"
				w.err = ""
				w.nameInput.Focus(c)
			}
		})
		if w.err != "" {
			ui.Text(c, w.err).TextColor(t.Danger)
		}
		switch w.status {
		case "loading":
			ui.Text(c, "正在加载片段…")
		case "error":
			if ui.Button(c, "重新加载").Disabled(w.busy).Clicked() {
				w.Load(w.post)
			}
		case "ready":
			if ui.Split(c, &w.listWidth, func() { w.list(c) }, func() { w.editor(c) }).Grow(1).MinHeight(0).Changed() {
				w.listWidth = max(float32(240), min(w.listWidth, maxList))
				c.Invalidate()
			}
		}
	})
	wasOpen := w.deleteOpen
	ui.Modal(c, &w.deleteOpen, func() {
		ui.Column(c).Width(380).Padding(12).Gap(8).Radius(6).Background(t.Background).Children(func() {
			ui.Text(c, "删除所选片段？").Bold()
			ui.Text(c, "删除后无法撤销；已启用片段将立即从生成的 AGENTS.md 中移除。")
			if w.err != "" {
				ui.Text(c, w.err).TextColor(t.Danger)
			}
			ui.Row(c).Gap(4).Children(func() {
				if ui.Button(c, "取消").AutoFocus().Height(28).Clicked() {
					w.deleteOpen = false
				}
				label := "删除"
				if w.busy {
					label = "正在删除…"
				}
				if ui.PrimaryButton(c, label).Height(28).Disabled(w.busy).Clicked() {
					id := w.selected
					w.run(func() (agentsmd.Snapshot, error) { return w.service.Delete(id) }, func() { w.deleteOpen = false; w.selected = 0; w.draftMode = "none"; w.newTrigger.Focus() })
				}
			})
		})
	})
	if wasOpen && !w.deleteOpen {
		w.deleteTrigger.Focus(c)
	}
}
func (w *AgentsWorkspace) list(c *ui.Context) {
	ui.Column(c).Fill().Padding(4).Gap(4).Children(func() {
		ui.Text(c, w.snapshot.TargetPath).FontSize(11).TextColor(c.Theme().TextMuted)
		if len(w.snapshot.Fragments) == 0 {
			ui.Text(c, "还没有片段，点击“新增片段”开始。")
			return
		}
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).WidthPercent(100).Children(func() {
				for _, fragment := range w.snapshot.Fragments {
					f := fragment
					ui.Row(c).Key(f.ID).Height(40).Gap(4).Children(func() {
						enabled := f.Enabled
						if ui.Switch(c, &enabled).Label("启用 " + f.Title).Disabled(w.busy).Changed() {
							w.run(func() (agentsmd.Snapshot, error) { return w.service.SetEnabled(f.ID, enabled) }, nil)
						}
						b := ui.Button(c, f.Title).Grow(1).Height(28)
						if w.selected == f.ID {
							b.Border(1, c.Theme().Accent)
						}
						if b.Clicked() {
							w.selectFragment(f)
						}
					})
				}
			})
		})
	})
}
func (w *AgentsWorkspace) editor(c *ui.Context) {
	ui.Column(c).Fill().Padding(8).Gap(6).Children(func() {
		ui.Row(c).Gap(4).Children(func() {
			ui.Checkbox(c, &w.combined, "组合预览")
			if w.busy {
				ui.Text(c, "正在保存…").TextColor(c.Theme().TextMuted)
			}
		})
		width, _ := c.Size()
		room := width - w.listWidth - 26
		lower := max(float32(20), min(float32(50), 220/max(room, 1)*100))
		w.previewPercent = max(lower, min(w.previewPercent, 100-lower))
		if ui.SplitPercent(c, &w.previewPercent, func() {
			ui.Column(c).Fill().Gap(6).Padding(4).Children(func() {
				if w.draftMode == "" || w.draftMode == "none" {
					ui.Text(c, "选择片段以编辑，或新增片段。")
					return
				}
				ui.TextInput(c, &w.title).Label("片段名称").Bind(&w.nameInput).Height(28).Disabled(w.busy)
				ui.TextArea(c, &w.content).Label("Markdown 正文").Grow(1).MinHeight(0).Disabled(w.busy)
				ui.Row(c).Gap(4).Children(func() {
					label := "保存片段"
					if w.busy {
						label = "正在保存…"
					}
					if ui.PrimaryButton(c, label).Bind(&w.saveTrigger).Height(28).Disabled(w.busy).Clicked() {
						w.save()
					}
					if w.draftMode == "edit" && ui.Button(c, "删除片段").Bind(&w.deleteTrigger).Height(28).Disabled(w.busy).Clicked() {
						w.deleteOpen = true
					}
				})
			})
		}, func() {
			ui.Column(c).Fill().Padding(8).Gap(6).Children(func() {
				body, title := w.content, "单片段预览"
				if w.combined {
					body = w.snapshot.CombinedContent
					title = "组合预览"
				}
				ui.Text(c, title).Bold()
				if body == "" {
					ui.Text(c, "暂无预览内容").TextColor(c.Theme().TextMuted)
				} else {
					ui.Scroll(c).Grow(1).MinHeight(0).Children(func() { markdown.View(c, body, &w.md) })
				}
			})
		}).Grow(1).MinHeight(0).Changed() {
			w.previewPercent = max(lower, min(w.previewPercent, 100-lower))
			c.Invalidate()
		}
	})
}
