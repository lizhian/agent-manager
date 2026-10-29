package uiapp

import (
	_ "embed"
	"github.com/egoist/mygo/ui"
	"github.com/lizhian/agent-manager/internal/agentsmd"
	"strings"
)

//go:embed assets/grip-vertical.svg
var agentsGripSVG []byte
var agentsGrip = ui.MustParseSVG(agentsGripSVG)

func (w *AgentsWorkspace) list(c *ui.Context) {
	ui.Column(c).Fill().Padding(4).Gap(4).Children(func() {
		ui.SearchField(c, &w.query).Label("搜索片段").Placeholder("搜索标题和正文").Height(28)
		ui.Segmented(c, &w.filter, "全部", "已启用", "未启用").Label("片段范围")
		ui.Text(c, w.snapshot.TargetPath).FontSize(max(float32(10), c.Theme().FontSize-4)).TextColor(c.Theme().TextMuted)
		if len(w.snapshot.Fragments) == 0 {
			ui.Text(c, "还没有片段，点击“新增片段”开始。")
			return
		}
		if w.query != "" || w.filter != 0 {
			ui.Text(c, "筛选时禁止排序").FontSize(c.Theme().FontSize - 3).TextColor(c.Theme().TextMuted)
		}
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).WidthPercent(100).Children(func() {
				visible := 0
				for index, fragment := range w.snapshot.Fragments {
					f := fragment
					if !strings.Contains(strings.ToLower(f.Title+"\n"+f.Content), strings.ToLower(w.query)) || w.filter == 1 && !f.Enabled || w.filter == 2 && f.Enabled {
						continue
					}
					visible++
					row := ui.Row(c).Key(f.ID).Height(40).Gap(4)
					row.Children(func() {
						handle := ui.Button(c, "").Children(func() { ui.Icon(c, agentsGrip).Size(16, 16) }).Size(24, 26).Label("拖动排序 " + f.Title).Tooltip("拖动排序").Disabled(w.busy || w.query != "" || w.filter != 0)
						if !w.busy && w.query == "" && w.filter == 0 {
							handle.Drag(f.ID)
						}
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
					if w.busy || w.query != "" || w.filter != 0 {
						continue
					}
					_, y, _ := row.PointerPosition()
					after := y > row.Bounds().H/2
					before := f.ID
					if after {
						before = 0
						if index+1 < len(w.snapshot.Fragments) {
							before = w.snapshot.Fragments[index+1].ID
						}
					}
					if _, over := ui.DragOver[int64](row); over {
						row.DrawOver(func(p *ui.Painter, r ui.Rect) {
							lineY := r.Y
							if after {
								lineY += r.H - 2
							}
							p.Fill(ui.Rect{X: r.X, Y: lineY, W: r.W, H: 2}, c.Theme().Accent, 0)
						})
					}
					if id, ok := ui.Drop[int64](row); ok {
						w.moveBefore(id, before)
					}
				}
				if visible == 0 {
					ui.Text(c, "没有匹配片段，请调整筛选或清空搜索。").FontSize(c.Theme().FontSize)
				}
			})
		})
	})
}
func (w *AgentsWorkspace) moveBefore(id, before int64) {
	if id == before {
		return
	}
	ids := make([]int64, 0, len(w.snapshot.Fragments))
	for _, f := range w.snapshot.Fragments {
		if f.ID != id {
			ids = append(ids, f.ID)
		}
	}
	at := len(ids)
	for i, item := range ids {
		if item == before {
			at = i
			break
		}
	}
	ids = append(ids, 0)
	copy(ids[at+1:], ids[at:])
	ids[at] = id
	w.run(func() (agentsmd.Snapshot, error) { return w.service.Reorder(ids) }, nil)
}

// SetPreview applies persisted preferences without initiating a second write.
func (w *AgentsWorkspace) SetPreview(position string, ratio float64, onChange func(string, float64)) {
	w.bottom = position == "bottom"
	w.previewPercent = float32((1 - ratio) * 100)
	w.previewChanged = onChange
}
func (w *AgentsWorkspace) notifyPreview() {
	if w.previewChanged != nil {
		position := "right"
		if w.bottom {
			position = "bottom"
		}
		w.previewChanged(position, 1-float64(w.previewPercent)/100)
	}
}
