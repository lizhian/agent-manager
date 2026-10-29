// Package uiapp presents native workspaces and delegates writes to application services.
package uiapp

import (
	"github.com/egoist/mygo/ui"
	"github.com/lizhian/agent-manager/internal/agentsmd"
)

type App struct {
	Agents *AgentsWorkspace
	Skills *SkillsWorkspace
	MCP    *MCPWorkspace
	page   string
}

func NewApp(service *agentsmd.Service) *App { return &App{Agents: NewAgentsWorkspace(service)} }

// Start installs the native UI dispatcher before starting background loading.
func (a *App) Start(post func(func())) {
	a.Agents.Load(post)
	if a.Skills != nil {
		a.Skills.Load(post)
	}
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
		pages := []string{}
		if a.Skills != nil {
			pages = append(pages, "Skills")
		}
		if a.MCP != nil {
			pages = append(pages, "MCP")
		}
		pages = append(pages, "AGENTS.md")
		page := a.page
		if page == "" {
			page = pages[0]
		}
		if len(pages) > 1 {
			ui.Row(c).Padding(8).Gap(4).Children(func() {
				for _, name := range pages {
					b := ui.Button(c, name).Height(28)
					if name == page {
						b.Border(1, theme.Accent)
					}
					if b.Clicked() {
						a.page = name
						page = name
					}
				}
			})
		}
		ui.Column(c).Grow(1).MinHeight(0).Children(func() {
			switch page {
			case "Skills":
				a.Skills.View(c)
			case "MCP":
				a.MCP.View(c)
			default:
				a.Agents.View(c)
			}
		})
	})
}
