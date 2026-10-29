// Package uiapp presents native workspaces and delegates writes to application services.
package uiapp

import (
	"github.com/egoist/mygo/ui"
	"github.com/lizhian/agent-manager/internal/agentsmd"
	"github.com/lizhian/agent-manager/internal/settings"
)

type App struct {
	Agents      *AgentsWorkspace
	MCP         *MCPWorkspace
	Settings    *SettingsWorkspace
	preferences settings.Snapshot
	page        string
}

func NewApp(service *agentsmd.Service) *App {
	return &App{Agents: NewAgentsWorkspace(service), preferences: settings.Defaults()}
}

// Start installs the native UI dispatcher before starting background loading.
func (a *App) Start(post func(func())) {
	a.Agents.Load(post)
	if a.Settings != nil {
		a.Settings.Load(post, func(value settings.Snapshot) {
			a.preferences = value
			a.Agents.SetPreview(value.Preview.Position, value.Preview.Ratio, a.Settings.SetPreview)
		})
	}
	if a.MCP != nil {
		a.MCP.Load(post)
	}
}
func (a *App) View(c *ui.Context) {
	theme := *c.Theme()
	theme.FontSize = a.preferences.FontSize.Pixels()
	theme.Font = a.preferences.FontFamily
	theme.Spacing = 4
	theme.Radius = 6
	c.SetTheme(&theme)
	ui.Column(c).Fill().Children(func() {
		if a.MCP != nil || a.Settings != nil {
			ui.Row(c).Padding(8).Gap(4).Children(func() {
				pages := []string{"AGENTS.md"}
				if a.MCP != nil {
					pages = append(pages, "MCP")
				}
				if a.Settings != nil {
					pages = append(pages, "设置")
				}
				for _, page := range pages {
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
			if a.page == "设置" && a.Settings != nil {
				a.Settings.View(c)
			} else if a.page == "MCP" && a.MCP != nil {
				a.MCP.View(c)
			} else {
				a.Agents.View(c)
			}
		})
	})
}
