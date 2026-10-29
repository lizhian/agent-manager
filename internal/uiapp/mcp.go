package uiapp

import (
	"fmt"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/lizhian/agent-manager/internal/mcp"
)

type mcpPair struct{ Key, Value string }
type MCPWorkspace struct {
	service                              *mcp.Service
	post                                 func(func())
	snapshot                             mcp.Snapshot
	status                               string
	busy                                 bool
	err, notice, query                   string
	selected                             int64
	mode                                 string
	draft                                mcp.Input
	argsText                             string
	env, headers                         []mcpPair
	deleteOpen                           bool
	deleteID                             int64
	deleteName                           string
	newTrigger, deleteTrigger, nameInput ui.Handle
	listWidth                            float32
}

func NewMCPWorkspace(service *mcp.Service) *MCPWorkspace {
	return &MCPWorkspace{service: service, status: "loading", listWidth: 260}
}
func (w *MCPWorkspace) Load(post func(func())) {
	w.post = post
	w.status = "loading"
	w.run(w.service.Snapshot, func() {
		if len(w.snapshot.Servers) > 0 {
			w.selectServer(w.snapshot.Servers[0])
		}
	})
}
func (w *MCPWorkspace) run(work func() (mcp.Snapshot, error), done func()) {
	if w.busy {
		return
	}
	w.busy = true
	w.err = ""
	w.notice = ""
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
func (w *MCPWorkspace) selectServer(server mcp.Server) {
	w.selected = server.ID
	w.draft = server.Input
	w.argsText = strings.Join(server.Args, "\n")
	w.env = toPairs(server.Env)
	w.headers = toPairs(server.Headers)
	w.mode = "edit"
	w.err = ""
	w.notice = ""
}
func (w *MCPWorkspace) save() {
	input := w.draft
	input.Args = []string{}
	for _, line := range strings.Split(w.argsText, "\n") {
		if value := strings.TrimSpace(line); value != "" {
			input.Args = append(input.Args, value)
		}
	}
	input.Env = fromPairs(w.env)
	input.Headers = fromPairs(w.headers)
	id, mode := w.selected, w.mode
	w.run(func() (mcp.Snapshot, error) {
		if mode == "new" {
			return w.service.Create(input)
		}
		return w.service.Update(id, input)
	}, func() {
		if w.selected == id && w.mode == mode {
			for _, server := range w.snapshot.Servers {
				if (mode == "new" && server.Name == strings.TrimSpace(input.Name)) || (mode == "edit" && server.ID == id) {
					w.selectServer(server)
					break
				}
			}
		}
		w.notice = "MCP 已保存"
	})
}
func (w *MCPWorkspace) View(c *ui.Context) {
	t := c.Theme()
	width, _ := c.Size()
	maxList := max(float32(240), min(float32(360), width-480))
	w.listWidth = max(float32(240), min(w.listWidth, maxList))
	ui.Column(c).Fill().Padding(8).Gap(6).Children(func() {
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, "MCP 管理").FontSize(18).Bold()
			ui.Text(c, fmt.Sprintf("%d 个服务", len(w.snapshot.Servers))).TextColor(t.TextMuted).Grow(1)
			if ui.Button(c, "新增 MCP").Height(28).Bind(&w.newTrigger).Disabled(w.busy || w.status != "ready").Clicked() {
				w.selected = 0
				w.mode = "new"
				w.draft = mcp.Input{Transport: "stdio"}
				w.argsText = ""
				w.env = nil
				w.headers = nil
				w.err = ""
				w.notice = ""
				w.nameInput.Focus(c)
			}
		})
		if w.notice != "" {
			ui.Text(c, w.notice).TextColor(t.TextMuted)
		}
		switch w.status {
		case "loading":
			ui.Text(c, "正在读取 MCP 配置…")
		case "error":
			ui.Text(c, w.err).TextColor(t.Danger)
			if ui.Button(c, "重新读取 MCP").Disabled(w.busy).Clicked() {
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
		ui.Column(c).Width(400).Padding(12).Gap(8).Radius(6).Background(t.Background).Children(func() {
			ui.Text(c, "删除 MCP "+w.deleteName+"？").Bold()
			ui.Text(c, "将永久删除应用中的服务配置；不会修改外部 Agent 配置。")
			if w.err != "" {
				ui.Text(c, w.err).TextColor(t.Danger)
			}
			ui.Row(c).Gap(4).Children(func() {
				if ui.Button(c, "取消").AutoFocus().Height(28).Clicked() {
					w.deleteOpen = false
				}
				label := "确认删除 MCP"
				if w.busy {
					label = "正在删除 MCP…"
				}
				if ui.Button(c, label).Height(28).Disabled(w.busy).Clicked() {
					id := w.deleteID
					w.run(func() (mcp.Snapshot, error) { return w.service.Delete(id) }, func() {
						w.deleteOpen = false
						if w.selected == id {
							w.selected = 0
							w.mode = "none"
							if len(w.snapshot.Servers) > 0 {
								w.selectServer(w.snapshot.Servers[0])
							}
						}
						w.notice = "MCP 已删除"
						w.newTrigger.Focus()
					})
				}
			})
		})
	})
	if wasOpen && !w.deleteOpen {
		w.deleteTrigger.Focus(c)
	}
}
func (w *MCPWorkspace) list(c *ui.Context) {
	ui.Column(c).Fill().Padding(4).Gap(4).Children(func() {
		ui.SearchField(c, &w.query).Label("搜索 MCP").Placeholder("搜索名称、命令或 URL").Height(28)
		if len(w.snapshot.Servers) == 0 {
			ui.Text(c, "还没有 MCP，点击“新增 MCP”开始。")
			return
		}
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).WidthPercent(100).Children(func() {
				visible := 0
				for _, server := range w.snapshot.Servers {
					searchable := server.Name + " " + server.Transport + " " + server.Command + " " + server.URL + " " + strings.Join(server.Args, " ")
					for key := range server.Env {
						searchable += " " + key
					}
					for key := range server.Headers {
						searchable += " " + key
					}
					if !strings.Contains(strings.ToLower(searchable), strings.ToLower(strings.TrimSpace(w.query))) {
						continue
					}
					visible++
					ui.Row(c).Key(server.ID).Height(40).Gap(4).Children(func() {
						button := ui.Button(c, server.Name).Grow(1).Height(28)
						if w.selected == server.ID {
							button.Border(1, c.Theme().Accent)
						}
						if button.Clicked() {
							w.selectServer(server)
						}
						ui.Text(c, server.Transport).FontSize(11).TextColor(c.Theme().TextMuted)
					})
				}
				if visible == 0 {
					ui.Text(c, "没有匹配的 MCP")
					if ui.Button(c, "清空 MCP 搜索").Clicked() {
						w.query = ""
					}
				}
			})
		})
	})
}
func (w *MCPWorkspace) editor(c *ui.Context) {
	ui.Column(c).Fill().Padding(8).Gap(6).Children(func() {
		if w.mode != "new" && w.mode != "edit" {
			ui.Text(c, "选择一个 MCP，或新增服务配置。")
			return
		}
		ui.Row(c).Gap(4).Children(func() {
			title := "编辑 MCP"
			if w.mode == "new" {
				title = "新增 MCP 配置"
			}
			ui.Text(c, title).Bold().Grow(1)
			if w.mode == "edit" && ui.Button(c, "删除 MCP").Bind(&w.deleteTrigger).Height(28).Disabled(w.busy).Clicked() {
				w.deleteID = w.selected
				w.deleteName = w.draft.Name
				w.deleteOpen = true
				w.err = ""
			}
		})
		if w.err != "" {
			ui.Text(c, w.err).TextColor(c.Theme().Danger)
		}
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).WidthPercent(100).Gap(6).Children(func() {
				ui.Text(c, "名称")
				ui.TextInput(c, &w.draft.Name).Label("MCP 名称").Bind(&w.nameInput).Height(28).Disabled(w.busy)
				ui.Text(c, "传输")
				ui.Select(c, &w.draft.Transport, []string{"stdio", "http", "sse"}).Label("MCP 传输").Height(28).Disabled(w.busy)
				if w.draft.Transport == "stdio" {
					ui.Text(c, "命令")
					ui.TextInput(c, &w.draft.Command).Label("MCP 命令").Placeholder("npx / uvx / executable").Height(28).Disabled(w.busy)
					ui.Text(c, "参数（每行一个）")
					ui.TextArea(c, &w.argsText).Label("MCP 参数").Height(80).Disabled(w.busy)
				} else {
					ui.Text(c, "URL")
					ui.TextInput(c, &w.draft.URL).Label("MCP URL").Height(28).Disabled(w.busy)
				}
				w.pairs(c, "环境变量", &w.env)
				if w.draft.Transport != "stdio" {
					w.pairs(c, "请求头", &w.headers)
				}
			})
		})
		label := "保存 MCP"
		if w.busy {
			label = "正在保存 MCP…"
		}
		if ui.PrimaryButton(c, label).Height(28).Disabled(w.busy).Clicked() {
			w.save()
		}
	})
}
func (w *MCPWorkspace) pairs(c *ui.Context, label string, pairs *[]mcpPair) {
	ui.Column(c).Gap(4).Children(func() {
		ui.Row(c).Gap(4).Children(func() {
			ui.Text(c, label).Grow(1)
			if ui.Button(c, "新增"+label).Height(26).Disabled(w.busy).Clicked() {
				*pairs = append(*pairs, mcpPair{})
			}
		})
		remove := -1
		for i := range *pairs {
			ui.Row(c).Key(i).Gap(4).Children(func() {
				ui.TextInput(c, &(*pairs)[i].Key).Label(fmt.Sprintf("%s键 %d", label, i+1)).Placeholder("KEY").Grow(1).Height(28).Disabled(w.busy)
				ui.TextInput(c, &(*pairs)[i].Value).Label(fmt.Sprintf("%s值 %d", label, i+1)).Placeholder("VALUE 或 ${VAR}").Grow(2).Height(28).Disabled(w.busy)
				if ui.Button(c, "移除").Label(fmt.Sprintf("移除%s %d", label, i+1)).Height(26).Disabled(w.busy).Clicked() {
					remove = i
				}
			})
		}
		if remove >= 0 {
			*pairs = append((*pairs)[:remove], (*pairs)[remove+1:]...)
		}
	})
}
func toPairs(values map[string]string) []mcpPair {
	var pairs []mcpPair
	for key, value := range values {
		pairs = append(pairs, mcpPair{key, value})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Key < pairs[j].Key })
	return pairs
}
func fromPairs(pairs []mcpPair) map[string]string {
	values := map[string]string{}
	for _, pair := range pairs {
		if key := strings.TrimSpace(pair.Key); key != "" {
			values[key] = pair.Value
		}
	}
	return values
}
