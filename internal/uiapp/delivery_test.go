package uiapp_test

import (
	"github.com/egoist/mygo/ui"
	"github.com/lizhian/agent-manager/internal/agentsmd"
	"github.com/lizhian/agent-manager/internal/mcp"
	"github.com/lizhian/agent-manager/internal/settings"
	"github.com/lizhian/agent-manager/internal/skills"
	"github.com/lizhian/agent-manager/internal/storage"
	"github.com/lizhian/agent-manager/internal/uiapp"
	"testing"
	"time"
)

func TestFormalSidebarAndToolbarFitAllSizes(t *testing.T) {
	for _, width := range []int{960, 1180} {
		for _, size := range settings.Sizes {
			t.Run(string(size), func(t *testing.T) {
				store, err := storage.Open(storage.PathsFromDataDir(t.TempDir()))
				if err != nil {
					t.Fatal(err)
				}
				defer store.Close()
				prefs := settings.New(store, settings.Options{Fonts: func() ([]string, error) { return []string{"Example Sans"}, nil }})
				value := settings.Defaults()
				value.FontSize = size
				value.FontFamily = "PingFang SC"
				if _, err = prefs.Save(value); err != nil {
					t.Fatal(err)
				}
				app := uiapp.NewApp(agentsmd.New(store, agentsmd.Options{}))
				app.MCP = uiapp.NewMCPWorkspace(mcp.New(store))
				app.Skills = uiapp.NewSkillsWorkspace(skills.New(store, skills.Options{}))
				app.Settings = uiapp.NewSettingsWorkspace(prefs)
				updates := make(chan func(), 16)
				app.Start(func(fn func()) { updates <- fn })
				height := 640
				if width == 1180 {
					height = 760
				}
				tt := ui.NewTester(app.View, width, height)
				for i := 0; i < 5; i++ {
					select {
					case fn := <-updates:
						fn()
						tt.Frame()
					case <-time.After(3 * time.Second):
						t.Fatal("load timeout")
					}
				}
				previousY := float32(-1)

				for _, label := range []string{"Skills", "MCP", "AGENTS.md", "设置"} {
					r, ok := tt.Find(label)
					if !ok {
						t.Fatal("missing navigation", label)
					}

					if r.X < 6 || r.X+r.W > 160 || r.Y <= previousY {
						t.Fatal("navigation must remain a vertical sidebar", label, r)
					}
					previousY = r.Y
				}
				for _, label := range []string{"安装来源", "全部更新", "技能文件夹", "重新加载"} {
					r, ok := tt.Find(label)
					if !ok || r.X < 104 || r.X+r.W > float32(width) {
						t.Fatal("toolbar leaves workspace", label, r)
					}
				}
			})
		}
	}
}
