package uiapp

import (
	"context"
	"fmt"
	"github.com/egoist/mygo/ui"
	"github.com/lizhian/agent-manager/internal/mcp"
)

type mcpDebugState struct {
	generation             uint64
	cancel                 context.CancelFunc
	busy                   bool
	loaded                 bool
	tools                  []mcp.Tool
	selected               int
	arguments, result, err string
	duration               int64
	showSchema             bool
}

func (w *MCPWorkspace) resetDebug() {
	if w.debug.cancel != nil {
		w.debug.cancel()
	}
	w.debug = mcpDebugState{generation: w.debug.generation + 1, arguments: "{}"}
}
func (w *MCPWorkspace) loadTools() {
	if w.debug.busy || w.selected == 0 {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.debug.cancel = cancel
	w.debug.busy = true
	w.debug.err = ""
	w.debug.result = ""
	generation, id := w.debug.generation, w.selected
	go func() {
		tools, err := w.debugger.ListTools(ctx, id)
		cancel()
		w.post(func() {
			if w.selected != id || w.debug.generation != generation {
				return
			}
			w.debug.busy = false
			w.debug.loaded = true
			w.debug.tools = tools
			w.debug.selected = 0
			w.debug.arguments = "{}"
			if err != nil {
				w.debug.err = err.Error()
			}
		})
	}()
}
func (w *MCPWorkspace) callTool() {
	if w.debug.busy || len(w.debug.tools) == 0 {
		return
	}
	tool := w.debug.tools[w.debug.selected]
	ctx, cancel := context.WithCancel(context.Background())
	w.debug.cancel = cancel
	w.debug.busy = true
	w.debug.err = ""
	w.debug.result = ""
	generation, id, args := w.debug.generation, w.selected, w.debug.arguments
	go func() {
		output, err := w.debugger.CallTool(ctx, id, tool.Name, args)
		cancel()
		w.post(func() {
			if w.selected != id || w.debug.generation != generation {
				return
			}
			w.debug.busy = false
			if err != nil {
				w.debug.err = err.Error()
				return
			}
			w.debug.result = string(output.Result)
			w.debug.duration = output.DurationMs
		})
	}()
}
func (w *MCPWorkspace) debugView(c *ui.Context) {
	if w.selected == 0 || w.mode != "edit" {
		ui.Text(c, "先选择已保存的 MCP 服务配置。")
		return
	}
	ui.Column(c).Fill().Gap(6).Children(func() {
		ui.Row(c).Gap(6).Children(func() {
			ui.Text(c, "工具调试（使用已保存配置）").Bold().Grow(1)
			label := "读取工具"
			if w.debug.busy {
				label = "正在调试…"
			}
			if ui.Button(c, label).Height(28).Disabled(w.debug.busy || w.busy).Clicked() {
				w.loadTools()
			}
		})
		if w.debug.err != "" {
			ui.Text(c, w.debug.err).TextColor(c.Theme().Danger)
		}
		if len(w.debug.tools) == 0 {
			if !w.debug.busy && w.debug.err == "" {
				if w.debug.loaded {
					ui.Text(c, "服务未提供工具，可重新读取。")
				} else {
					ui.Text(c, "读取服务工具后查看说明并调用。")
				}
			}
			return
		}
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).WidthPercent(100).Gap(6).Children(func() {
				names := make([]string, len(w.debug.tools))
				for i, tool := range w.debug.tools {
					names[i] = tool.Name
				}
				name := names[w.debug.selected]
				if ui.Select(c, &name, names).Label("选择工具").Height(28).Disabled(w.debug.busy).Changed() {
					for i, item := range names {
						if item == name {
							w.debug.selected = i
							break
						}
					}
					w.debug.arguments = "{}"
					w.debug.result = ""
					w.debug.err = ""
				}

				tool := w.debug.tools[w.debug.selected]
				if tool.Title != "" {
					ui.Text(c, tool.Title).Bold()
				}
				if tool.Description != "" {
					ui.Text(c, tool.Description)
				}
				ui.Checkbox(c, &w.debug.showSchema, "输入 Schema")
				if w.debug.showSchema {
					ui.Text(c, string(tool.InputSchema)).Selectable()
				}
				ui.Text(c, "JSON 调用参数（对象）")
				ui.TextArea(c, &w.debug.arguments).Label("工具调用参数").Height(120).Disabled(w.debug.busy)
				label := "调用工具"
				if w.debug.busy {
					label = "调用中…"
				}
				if ui.PrimaryButton(c, label).Height(28).Disabled(w.debug.busy || w.busy).Clicked() {
					w.callTool()
				}
				if w.debug.result != "" {
					ui.Text(c, fmt.Sprintf("调用结果 · %d ms", w.debug.duration)).Bold()
					ui.Text(c, w.debug.result).Selectable()
				}
			})
		})
	})
}
