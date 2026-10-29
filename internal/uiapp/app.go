// Package uiapp presents native workspaces and delegates writes to application services.
package uiapp

import (
	"github.com/egoist/mygo/ui"
	"github.com/lizhian/agent-manager/internal/agentsmd"
)

type App struct{ Agents *AgentsWorkspace }

func NewApp(service *agentsmd.Service) *App { return &App{Agents: NewAgentsWorkspace(service)} }

// Start installs the native UI dispatcher before starting background loading.
func (a *App) Start(post func(func())) { a.Agents.Load(post) }
func (a *App) View(c *ui.Context) {
	theme := *c.Theme()
	theme.FontSize = 15
	theme.Spacing = 4
	theme.Radius = 6
	c.SetTheme(&theme)
	a.Agents.View(c)
}
