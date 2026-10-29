package uiapp

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/lizhian/agent-manager/internal/agentsmd"
)

type AgentsWorkspace struct {
	viewportWidth             float32
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
	query                     string
	filter                    int
	bottom                    bool
	previewChanged            func(string, float64)
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
		sameContext := w.selected == id && w.draftMode == mode
		if mode == "new" && w.draftMode == "new" && len(w.snapshot.Fragments) > 0 {
			w.selectFragment(w.snapshot.Fragments[len(w.snapshot.Fragments)-1])
		} else if w.selected == id && w.draftMode == mode {
			for _, f := range w.snapshot.Fragments {
				if f.ID == id {
					w.selectFragment(f)
				}
			}
		}
		if sameContext {
			w.saveTrigger.Focus()
		}
	})
}
func (w *AgentsWorkspace) View(c *ui.Context) {
	t := c.Theme()
	width, _ := c.Size()
	if w.viewportWidth > 0 {
		width = w.viewportWidth
	}
	maxList := min(float32(360), width-480)
	w.listWidth = max(float32(240), min(w.listWidth, maxList))
	ui.Column(c).Fill().Padding(8).Gap(6).Children(func() {
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, "AGENTS.md").FontSize(t.FontSize + 3).Bold()
			ui.Text(c, fmt.Sprintf("%d 个片段 · 已启用 %d 字符", len(w.snapshot.Fragments), w.snapshot.EnabledContentChars)).TextColor(t.TextMuted).Grow(1)
			if ui.Button(c, "打开目录").Height(28).Disabled(w.busy).Clicked() {
				w.run(func() (agentsmd.Snapshot, error) {
					if err := w.service.OpenFolder(); err != nil {
						return agentsmd.Snapshot{}, err
					}
					return w.service.Snapshot()
				}, nil)
			}
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
					w.run(func() (agentsmd.Snapshot, error) { return w.service.Delete(id) }, func() {
						w.deleteOpen = false
						if w.selected == id && w.draftMode == "edit" {
							w.selected = 0
							w.draftMode = "none"
							w.newTrigger.Focus()
						}
					})
				}
			})
		})
	})
	if wasOpen && !w.deleteOpen {
		w.deleteTrigger.Focus(c)
	}
}
func (w *AgentsWorkspace) editor(c *ui.Context) {
	ui.Column(c).Fill().Padding(8).Gap(6).Children(func() {
		ui.Row(c).Gap(4).Children(func() {
			ui.Checkbox(c, &w.combined, "组合预览")
			if ui.Checkbox(c, &w.bottom, "底部预览").Changed() {
				w.notifyPreview()
			}
			if w.busy {
				ui.Text(c, "正在保存…").TextColor(c.Theme().TextMuted)
			}
		})
		width, height := c.Size()
		if w.viewportWidth > 0 {
			width = w.viewportWidth
		}
		room := width - w.listWidth - 26
		minimum := float32(220)
		if w.bottom {
			room = height - 100
			minimum = 160
		}
		lower := max(float32(20), min(float32(50), minimum/max(room, 1)*100))
		w.previewPercent = max(lower, min(w.previewPercent, 100-lower))
		editor := func() {
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
		}
		preview := func() {
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
		}
		var split ui.Element
		if w.bottom {
			split = ui.SplitVerticalPercent(c, &w.previewPercent, editor, preview)
		} else {
			split = ui.SplitPercent(c, &w.previewPercent, editor, preview)
		}
		if split.Grow(1).MinHeight(0).Changed() {
			w.previewPercent = max(lower, min(w.previewPercent, 100-lower))
			w.notifyPreview()
			c.Invalidate()
		}
	})
}
