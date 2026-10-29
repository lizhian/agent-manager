// Package uiapp presents native workspaces and delegates writes to application services.
package uiapp

import (
	"github.com/egoist/mygo/ui"
	"github.com/lizhian/agent-manager/internal/agentsmd"
	"github.com/lizhian/agent-manager/internal/settings"
	"math"
)

type App struct {
	Agents      *AgentsWorkspace
	Skills      *SkillsWorkspace
	MCP         *MCPWorkspace
	Settings    *SettingsWorkspace
	preferences settings.Snapshot
	page        string
}

func NewApp(service *agentsmd.Service) *App {
	return &App{Agents: NewAgentsWorkspace(service), preferences: settings.Defaults()}
}
func (a *App) Start(post func(func())) {
	a.Agents.Load(post)
	if a.Skills != nil {
		a.Skills.Load(post)
	}
	if a.MCP != nil {
		a.MCP.Load(post)
	}
	if a.Settings != nil {
		a.Settings.Load(post, func(value settings.Snapshot) {
			a.preferences = value
			a.Agents.SetPreview(value.Preview.Position, value.Preview.Ratio, a.Settings.SetPreview)
			if a.Skills != nil {
				a.Skills.SetPreview(value.Preview.Position, value.Preview.Ratio, a.Settings.SetPreview)
			}
		})
	}
}
func (a *App) View(c *ui.Context) {
	theme := *c.Theme()
	theme.FontSize = a.preferences.FontSize.Pixels()
	theme.Font = a.preferences.FontFamily
	theme.Spacing = 4
	theme.Radius = 6
	c.SetTheme(&theme)
	ui.Row(c).Fill().Children(func() {
		pages := []string{}
		if a.Skills != nil {
			pages = append(pages, "Skills")
		}
		if a.MCP != nil {
			pages = append(pages, "MCP")
		}
		pages = append(pages, "AGENTS.md")
		if a.Settings != nil {
			pages = append(pages, "设置")
		}
		page := a.page
		if page == "" {
			page = pages[0]
		}
		sidebarWidth := float32(104)
		for _, name := range pages {
			textWidth, _ := c.MeasureText(0, ui.Span{Text: name, Font: theme.Font, Size: theme.FontSize})
			// Account for the explicit button padding/border and sidebar padding, on the 2px grid.
			sidebarWidth = max(sidebarWidth, float32(math.Ceil(float64(textWidth+30)/2)*2))
		}
		if len(pages) > 1 {
			ui.Column(c).Width(sidebarWidth).HeightPercent(100).Padding(6).Gap(4).Background(theme.Surface).Children(func() {
				for _, name := range pages {
					b := ui.Button(c, name).Height(28).Padding(2, 8).WidthPercent(100)
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
		width, _ := c.Size()
		available := width
		if len(pages) > 1 {
			available -= sidebarWidth
		}
		a.Agents.viewportWidth = available
		if a.Skills != nil {
			a.Skills.viewportWidth = available
		}
		if a.MCP != nil {
			a.MCP.viewportWidth = available
		}
		ui.Column(c).Grow(1).MinWidth(0).HeightPercent(100).Children(func() {
			if a.Settings != nil && a.Settings.err != "" && page != "设置" {
				ui.Row(c).PaddingX(8).Gap(4).Children(func() {
					ui.Text(c, a.Settings.err).TextColor(theme.Danger).Grow(1)
					label := "重试保存设置"
					if a.Settings.status == "error" {
						label = "重新加载设置"
					}
					if ui.Button(c, label).Height(28).Disabled(a.Settings.busy).Clicked() {
						if a.Settings.status == "error" {
							a.Settings.Load(a.Settings.post, a.Settings.applied)
						} else {
							a.Settings.save(a.Settings.draft)
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
				case "设置":
					a.Settings.View(c)
				default:
					a.Agents.View(c)
				}
			})
		})
	})
}
