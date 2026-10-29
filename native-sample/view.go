package main

import (
	_ "embed"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

//go:embed assets/grip-vertical.svg
var gripSVG []byte

var grip = ui.MustParseSVG(gripSVG)

type app struct {
	sample         *Sample
	query          string
	combined       bool
	bottom         bool
	resetOpen      bool
	listWidth      float32
	previewPercent float32
	md             markdown.Options
}

func newApp() *app {
	return &app{sample: NewSample(), listWidth: 280, previewPercent: 50, md: markdown.Options{OnLink: func(string) {}}}
}
func (a *app) view(c *ui.Context) {
	theme := *c.Theme()
	theme.FontSize = 15
	theme.Spacing = 3
	theme.Radius = 6
	c.SetTheme(&theme)
	ui.Column(c).Fill().Padding(8).Gap(6).Children(func() {
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, "AGENTS.md").FontSize(18).Bold()
			ui.Text(c, "原生样本 · 数据仅保存在内存").TextColor(theme.TextMuted).Grow(1)
			if ui.Button(c, "重置样本").Height(28).Clicked() {
				a.resetOpen = true
			}
		})
		ui.Split(c, &a.listWidth, func() { a.list(c) }, func() { a.workspace(c) }).Grow(1).MinHeight(0)
	})
	if ui.AlertDialog(c, &a.resetOpen, "重置临时片段？", "将丢弃本次编辑和顺序；不会修改磁盘上的数据。", "取消", "重置") == 1 {
		a.sample = NewSample()
		a.query = ""
	}
}
func (a *app) list(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Gap(4).Padding(4).Children(func() {
		ui.SearchField(c, &a.query).Label("搜索片段").Placeholder("搜索名称和正文").Height(28)
		ui.Text(c, "拖动手柄，将片段放到蓝色插入线").FontSize(11).TextColor(t.TextMuted)
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).WidthPercent(100).Children(func() {
				visible := 0
				for i := range a.sample.Fragments {
					f := &a.sample.Fragments[i]
					if !strings.Contains(strings.ToLower(f.Name+f.Body), strings.ToLower(a.query)) {
						continue
					}
					visible++
					row := ui.Row(c).Key(f.ID).Height(40).Gap(4)
					row.Children(func() {
						handle := ui.Button(c, "").Children(func() { ui.Icon(c, grip).Size(16, 16) }).Size(24, 26).Label("拖动排序 " + f.Name).Tooltip("拖动排序").Disabled(a.query != "")
						if a.query == "" {
							handle.Drag(f.ID)
						}
						ui.Switch(c, &f.Enabled).Label("启用 " + f.Name)
						b := ui.Button(c, f.Name).Grow(1).Height(28)
						if f.ID == a.sample.Selected {
							b.Border(1, t.Accent)
						}
						if b.Clicked() {
							a.sample.Selected = f.ID
							a.combined = false
						}
					})
					a.dropBefore(row, f.ID)
				}
				if visible == 0 {
					ui.Text(c, "没有匹配片段，请清空搜索").FontSize(13)
				}
				a.insertion(c, 0)
			})
		})
	})
}
func (a *app) insertion(c *ui.Context, before int) {
	if a.query != "" {
		return
	}
	line := ui.Box(c).Key(struct{ Before int }{before}).Height(24).WidthPercent(100).Label("移到列表末尾")
	a.dropBefore(line, before)
}

// Each complete row accepts a drop before itself; the final target appends.
func (a *app) dropBefore(row ui.Element, before int) {
	if a.query != "" {
		return
	}
	if _, over := ui.DragOver[int](row); over {
		row.DrawOver(func(p *ui.Painter, r ui.Rect) { p.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W, H: 2}, ui.RGB(46, 110, 219), 0) })
	}
	if id, ok := ui.Drop[int](row); ok {
		a.sample.MoveBefore(id, before)
	}
}
func (a *app) workspace(c *ui.Context) {
	ui.Column(c).Fill().Padding(4).Gap(6).Children(func() {
		ui.Row(c).Gap(4).Children(func() {
			ui.Checkbox(c, &a.combined, "组合预览")
			ui.Checkbox(c, &a.bottom, "底部预览")
		})
		editor := func() { a.editor(c) }
		preview := func() {
			ui.Column(c).Fill().Padding(8).Gap(6).Children(func() {
				title, body := "单片段预览", ""
				for _, f := range a.sample.Fragments {
					if f.ID == a.sample.Selected {
						body = f.Body
					}
				}
				if a.combined {
					title = "组合预览（仅启用片段）"
					body = a.sample.Combined()
				}
				ui.Text(c, title).Bold()
				ui.Scroll(c).Grow(1).MinHeight(0).Children(func() { markdown.View(c, body, &a.md) })
			})
		}
		if a.bottom {
			ui.SplitVerticalPercent(c, &a.previewPercent, editor, preview).Grow(1).MinHeight(0)
		} else {
			ui.SplitPercent(c, &a.previewPercent, editor, preview).Grow(1).MinHeight(0)
		}
	})
}
func (a *app) editor(c *ui.Context) {
	ui.Column(c).Fill().Padding(8).Gap(6).Children(func() {
		for i := range a.sample.Fragments {
			f := &a.sample.Fragments[i]
			if f.ID != a.sample.Selected {
				continue
			}
			ui.TextInput(c.Key(f.ID), &f.Name).Label("片段名称").Height(28)
			ui.TextArea(c.Key(struct{ Body int }{f.ID}), &f.Body).Label("Markdown 正文").Fill()
		}
	})
}
