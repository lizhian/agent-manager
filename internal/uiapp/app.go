// Package uiapp presents native workspaces and delegates writes to application services.
package uiapp

import (
	"github.com/egoist/mygo/ui"
	"github.com/lizhian/agent-manager/internal/agentsmd"
)

type App struct {
	Agents  *AgentsWorkspace
	Skills  *SkillsWorkspace
	section string
}

func NewApp(service *agentsmd.Service) *App { return &App{Agents: NewAgentsWorkspace(service)} }

// Start installs the native UI dispatcher before starting background loading.
func (a *App) Start(post func(func())) {
	a.Agents.Load(post)
	if a.Skills != nil {
		a.Skills.Load(post)
	}
}
func (a *App) View(c *ui.Context) {
	theme := *c.Theme()
	theme.FontSize = 15
	theme.Spacing = 4
	theme.Radius = 6
	c.SetTheme(&theme)
	ui.Column(c).Fill().Gap(4).Children(func() {
		if a.Skills != nil {
			ui.Row(c).Gap(4).Padding(8).Children(func() {
				if ui.Button(c, "Skills").Height(28).Clicked() {
					a.section = "skills"
				}
				if ui.Button(c, "AGENTS.md").Height(28).Clicked() {
					a.section = "agents"
				}
			})
		}
		ui.Column(c).Grow(1).MinHeight(0).Children(func() {
			if a.Skills != nil && a.section != "agents" {
				a.Skills.View(c)
			} else {
				a.Agents.View(c)
			}
		})
	})
}
