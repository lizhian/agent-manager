// Package uiapp presents native workspaces and delegates writes to application services.
package uiapp

import (
	"github.com/egoist/mygo/ui"
	"github.com/lizhian/agent-manager/internal/agentsmd"
)

type App struct {
	Agents *AgentsWorkspace
	MCP    *MCPWorkspace
	page   string
}

func NewApp(service *agentsmd.Service) *App { return &App{Agents: NewAgentsWorkspace(service)} }

// Start installs the native UI dispatcher before starting background loading.
func (a *App) Start(post func(func())) {
	a.Agents.Load(post)
	if a.MCP != nil {
		a.MCP.Load(post)
	}
}
func (a *App) View(c *ui.Context) {
	theme := *c.Theme()
	theme.FontSize = 15
	theme.Spacing = 4
	theme.Radius = 6
	c.SetTheme(&theme)
	ui.Column(c).Fill().Children(func() {
		if a.MCP != nil {
			ui.Row(c).Padding(8).Gap(4).Children(func() {
				for _, page := range []string{"AGENTS.md", "MCP"} {
					button := ui.Button(c, page).Height(28)
					if a.page == page || (a.page == "" && page == "AGENTS.md") {
						button.Border(1, theme.Accent)
					}
					if button.Clicked() {
						a.page = page
					}
				}
			})
		}
		ui.Column(c).Grow(1).MinHeight(0).Children(func() {
			if a.page == "MCP" && a.MCP != nil {
				a.MCP.View(c)
			} else {
				a.Agents.View(c)
			}
		})
	})
}
