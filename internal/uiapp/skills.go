package uiapp

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"
	"github.com/lizhian/agent-manager/internal/skills"
)

type SkillsWorkspace struct {
	viewportWidth                                      float32
	deleteSourceOpen                                   bool
	deleteImpact                                       *skills.RemoveImpact
	deleteSourceTrigger                                *ui.Handle
	deleteHandles                                      map[string]*ui.Handle
	installOpen                                        bool
	installSource, progress                            string
	installTrigger                                     ui.Handle
	writing                                            string
	previewChanged                                     func(string, float64)
	anchors                                            map[string]string
	service                                            *skills.Service
	post                                               func(func())
	snapshot                                           skills.Snapshot
	detail                                             *skills.Detail
	document                                           *skills.Document
	status, err, query                                 string
	globalOnly, bottom, detailLoading, documentLoading bool
	listWidth, previewPercent                          float32
	request, docRequest                                int
	mainMD, documentMD                                 markdown.Options
}

func NewSkillsWorkspace(service *skills.Service) *SkillsWorkspace {
	return &SkillsWorkspace{service: service, status: "loading", anchors: map[string]string{}, deleteHandles: map[string]*ui.Handle{}, listWidth: 290, previewPercent: 50}
}
func (w *SkillsWorkspace) Load(post func(func())) {
	w.post = post
	w.request++
	w.docRequest++
	w.document = nil
	w.status = "loading"
	w.err = ""
	go func() {
		snap, err := w.service.Snapshot()
		post(func() {
			if err != nil {
				w.status = "error"
				w.err = err.Error()
				return
			}
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
			w.detail = nil
			for _, source := range snap.Sources {
				if len(source.Skills) > 0 {
					w.selectSkill(source.SourceSafe, source.Skills[0].Name)
					break
				}
			}
		})
	}()
}
func (w *SkillsWorkspace) selectSkill(source, name string) {
	w.request++
	request := w.request
	w.docRequest++
	w.detailLoading = true
	w.document = nil
	w.err = ""
	go func() {
		d, err := w.service.Detail(source, name)
		w.post(func() {
			if request != w.request {
				return
			}
			w.detailLoading = false
			if err != nil {
				w.err = err.Error()
				w.detail = nil
				return
			}
			w.detail = &d
		})
	}()
}
func (w *SkillsWorkspace) followLink(href, current string) {
	if w.detail == nil {
		return
	}
	if strings.HasPrefix(href, "#") {
		anchor, err := url.PathUnescape(strings.TrimPrefix(href, "#"))
		if err == nil {
			w.anchors[current] = strings.ToLower(anchor)
		}
		return
	}
	if strings.HasPrefix(href, "https://") || strings.HasPrefix(href, "http://") {
		w.open(func() error { return w.service.OpenURL(href) })
		return
	}
	relative, err := skills.ResolveDocumentPath(current, href)
	if err != nil {
		w.err = err.Error()
		return
	}
	source, name := w.detail.SourceSafe, w.detail.Name
	w.docRequest++
	request := w.docRequest
	w.documentLoading = true
	w.err = ""
	go func() {
		doc, err := w.service.Document(source, name, relative)
		w.post(func() {
			if request != w.docRequest {
				return
			}
			w.documentLoading = false
			if err != nil {
				w.err = err.Error()
				return
			}
			w.document = &doc
		})
	}()
}
func (w *SkillsWorkspace) open(work func() error) {
	if w.post == nil {
		return
	}
	go func() {
		err := work()
		w.post(func() {
			if err != nil {
				w.err = err.Error()
			}
		})
	}()
}
func (w *SkillsWorkspace) View(c *ui.Context) {
	width, _ := c.Size()
	if w.viewportWidth > 0 {
		width = w.viewportWidth
	}
	maxList := max(float32(250), min(float32(420), width-400))
	w.listWidth = max(float32(250), min(w.listWidth, maxList))
	ui.Column(c).Fill().Padding(8).Gap(6).Children(func() {
		ui.Row(c).Gap(6).Wrap().Children(func() {
			ui.Text(c, "技能仓库").FontSize(c.Theme().FontSize + 3).Bold()
			count, enabled := 0, 0
			for _, source := range w.snapshot.Sources {
				count += len(source.Skills)
				for _, skill := range source.Skills {
					if skill.GlobalEnabled {
						enabled++
					}
				}
			}
			ui.Text(c, fmt.Sprintf("%d 来源 · %d Skills · %d 启用 · %d 字符", len(w.snapshot.Sources), count, enabled, w.snapshot.EnabledContentChars)).FontSize(max(float32(10), c.Theme().FontSize-4)).Grow(1)
			if ui.Button(c, "全部更新").Height(28).Disabled(w.writing != "" || len(w.snapshot.Sources) == 0).Clicked() {
				w.updateAll()
			}
			if ui.Button(c, "安装来源").Bind(&w.installTrigger).Height(28).Disabled(w.writing != "").Clicked() {
				w.installOpen = true
				w.err = ""
			}
			if ui.Button(c, "技能文件夹").Height(28).Clicked() {
				w.open(w.service.OpenGlobal)
			}
			if ui.Button(c, "重新加载").Height(28).Disabled(w.status == "loading" || w.writing != "").Clicked() {
				w.Load(w.post)
			}
		})
		if w.progress != "" {
			ui.Text(c, w.progress).FontSize(max(float32(10), c.Theme().FontSize-4))
		}
		if w.err != "" {
			ui.Text(c, w.err).TextColor(c.Theme().Danger)
		}
		switch w.status {
		case "loading":
			ui.Text(c, "正在读取 Skills 仓库…")
		case "error":
			ui.Text(c, "读取失败，请重新加载。")
		case "ready":
			if ui.Split(c, &w.listWidth, func() { w.list(c) }, func() { w.preview(c) }).Grow(1).MinHeight(0).Changed() {
				w.listWidth = max(float32(250), min(w.listWidth, maxList))
				c.Invalidate()
			}
		}
	})
	w.installDialog(c)
	w.sourceDeleteDialog(c)
}
func (w *SkillsWorkspace) list(c *ui.Context) {
	ui.Column(c).Fill().Padding(4).Gap(4).Children(func() {
		ui.SearchField(c, &w.query).Label("搜索 Skills").Placeholder("搜索名称、描述和来源").Height(28)
		ui.Checkbox(c, &w.globalOnly, "只看启用")
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).WidthPercent(100).Gap(4).Children(func() {
				shown := 0
				needle := strings.ToLower(strings.TrimSpace(w.query))
				for _, source := range w.snapshot.Sources {
					source := source
					var filtered []skills.Skill
					for _, skill := range source.Skills {
						if (!w.globalOnly || skill.GlobalEnabled) && (needle == "" || strings.Contains(strings.ToLower(source.Source+" "+skill.Name+" "+skill.Description), needle)) {
							filtered = append(filtered, skill)
						}
					}
					if len(filtered) == 0 {
						continue
					}
					ui.Row(c).Gap(4).Children(func() {
						ui.Text(c, source.Source).Bold().Grow(1)
						if ui.Button(c, "更新").Label("更新 " + source.Source).Height(26).Disabled(w.writing != "").Clicked() {
							w.syncSource(source.Source)
						}
						if ui.Button(c, "删除").TextColor(c.Theme().Danger).Bind(w.sourceDeleteHandle(source.SourceSafe)).Label("删除来源 " + source.Source).Height(26).Disabled(w.writing != "").Clicked() {
							w.deleteSourceTrigger = w.sourceDeleteHandle(source.SourceSafe)
							w.prepareDelete(source.SourceSafe)
						}
					})
					for _, skill := range filtered {
						skill := skill
						shown++
						ui.Row(c).Gap(4).Height(28).Children(func() {
							enabled := skill.GlobalEnabled
							if ui.Switch(c, &enabled).Label("全局启用 " + source.SourceSafe + "/" + skill.Name).Disabled(w.writing != "").Changed() {
								w.toggleSkill(source.SourceSafe, skill.Name, enabled)
							}
							b := ui.Button(c, skill.Name).Label("查看 " + source.SourceSafe + "/" + skill.Name).Height(28).Grow(1)
							if w.detail != nil && w.detail.SourceSafe == source.SourceSafe && w.detail.Name == skill.Name {
								b.Border(1, c.Theme().Accent)
							}
							if b.Clicked() {
								w.selectSkill(source.SourceSafe, skill.Name)
							}
						})
						state := "未启用"
						if skill.GlobalEnabled {
							state = "全局启用"
						}
						ui.Text(c, state+" · "+skill.Description).FontSize(max(float32(10), c.Theme().FontSize-4)).TextColor(c.Theme().TextMuted)
					}
				}
				if shown == 0 {
					if len(w.snapshot.Sources) == 0 {
						ui.Text(c, "还没有来源，可打开技能文件夹核对现有内容。")
					} else {
						ui.Text(c, "没有匹配结果，请清空搜索或筛选。")
					}
				}
			})
		})
	})
}
func (w *SkillsWorkspace) preview(c *ui.Context) {
	ui.Column(c).Fill().Padding(8).Gap(6).Children(func() {
		if w.detailLoading {
			ui.Text(c, "正在读取 SKILL.md…")
			return
		}
		if w.detail == nil {
			ui.Text(c, "选择一个 Skill 查看正文与元数据。")
			return
		}
		d := *w.detail
		enabled := d.GlobalEnabled
		ui.Row(c).Gap(4).Children(func() {
			if ui.Switch(c, &enabled).Label("详情全局启用").Disabled(w.writing != "").Changed() {
				w.toggleSkill(d.SourceSafe, d.Name, enabled)
			}
			ui.Text(c, "全局启用").FontSize(c.Theme().FontSize)
			if w.writing != "" {
				ui.Text(c, "操作进行中…").FontSize(max(float32(10), c.Theme().FontSize-4))
			}
		})
		ui.Row(c).Gap(4).Children(func() {
			ui.Text(c, d.Name).Bold().Grow(1)
			if ui.Button(c, "打开目录").Height(28).Clicked() {
				w.open(func() error { return w.service.OpenSkill(d.SourceSafe, d.Name) })
			}
			if ui.Button(c, "打开文件").Height(28).Clicked() {
				w.open(func() error { return w.service.OpenDocument(d.SourceSafe, d.Name, d.Name+"/SKILL.md") })
			}
		})
		ui.Text(c, d.Path).FontSize(max(float32(10), c.Theme().FontSize-4)).TextColor(c.Theme().TextMuted)
		ui.Text(c, "name: "+d.MetadataName)
		ui.Text(c, "description: "+d.MetadataDescription).FontSize(c.Theme().FontSize)
		main := func() {
			w.anchorBlocks(&w.mainMD, d.Name+"/SKILL.md")
			w.mainMD.OnLink = func(href string) { w.followLink(href, d.Name+"/SKILL.md") }
			ui.Scroll(c).Grow(1).MinHeight(0).Children(func() { markdown.View(c, d.Markdown, &w.mainMD) })
		}
		if w.documentLoading {
			ui.Text(c, "正在读取关联文档…")
		}
		if w.document == nil {
			main()
			return
		}
		ui.Row(c).Gap(4).Children(func() {
			if ui.Checkbox(c, &w.bottom, "底部预览").Changed() {
				w.notifyPreview()
			}
			ui.Text(c, w.document.RelativePath).FontSize(max(float32(10), c.Theme().FontSize-4)).Grow(1)
			doc := *w.document
			if ui.Button(c, "系统打开").Height(28).Clicked() {
				w.open(func() error { return w.service.OpenDocument(doc.SourceSafe, doc.SkillName, doc.RelativePath) })
			}
			if ui.Button(c, "关闭预览").Height(28).Clicked() {
				w.docRequest++
				w.document = nil
			}
		})
		if w.document == nil {
			main()
			return
		}
		associated := func() {
			doc := *w.document
			w.anchorBlocks(&w.documentMD, doc.RelativePath)
			w.documentMD.OnLink = func(href string) { w.followLink(href, doc.RelativePath) }
			ui.Scroll(c).Grow(1).MinHeight(0).Children(func() { markdown.View(c, doc.Content, &w.documentMD) })
		}
		w.previewPercent = max(float32(20), min(float32(80), w.previewPercent))
		var split ui.Element
		if w.bottom {
			split = ui.SplitVerticalPercent(c, &w.previewPercent, main, associated)
		} else {
			split = ui.SplitPercent(c, &w.previewPercent, main, associated)
		}
		if split.Grow(1).MinHeight(0).Changed() {
			w.previewPercent = max(float32(20), min(float32(80), w.previewPercent))
			w.notifyPreview()
			c.Invalidate()
		}
	})
}

// A zero-height scroll target uses the framework renderer's own public slug contract.
func (w *SkillsWorkspace) anchorBlocks(options *markdown.Options, current string) {
	options.Block = func(c *ui.Context, d *parser.Document, i int) bool {
		if target := w.anchors[current]; target != "" && d.Block(i).Kind == parser.Heading && markdown.Slug(parser.PlainText(d.Inlines(i))) == target {
			ui.Box(c).Height(0).ScrollIntoView()
			delete(w.anchors, current)
		}
		return false
	}
}
