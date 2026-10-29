package main

import (
	"flag"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lizhian/agent-manager/internal/agentsmd"
	"github.com/lizhian/agent-manager/internal/skills"
	"github.com/lizhian/agent-manager/internal/storage"
	"github.com/lizhian/agent-manager/internal/uiapp"
	"log"
	"os"
	"path/filepath"
)

func main() {
	dataDir := flag.String("data-dir", "", "隔离数据根目录（其中使用 .agent-manager 和 .agents）")
	minimum := flag.Bool("minimum", false, "使用960×640最小窗口")
	flag.Parse()
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	paths := storage.PathsFromHome(home)
	if *dataDir != "" {
		absolute, err := filepath.Abs(*dataDir)
		if err != nil {
			log.Fatal(err)
		}
		paths = storage.PathsFromDataDir(absolute)
	}
	store, err := storage.Open(paths)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	app := uiapp.NewApp(agentsmd.New(store, agentsmd.Options{}))
	app.Skills = uiapp.NewSkillsWorkspace(skills.New(store, skills.Options{}))
	width, height := 1180, 760
	if *minimum {
		width, height = 960, 640
	}
	mygo.App.WhenReady(func() {
		window := mygo.NewWindow(mygo.WindowOptions{Title: "Agent Manager", Width: width, Height: height, MinWidth: 960, MinHeight: 640, Content: ui.View(app.View)})
		app.Start(window.Update)
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
