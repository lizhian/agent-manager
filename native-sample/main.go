package main

import (
	"flag"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"log"
)

func main() {
	minimum := flag.Bool("minimum", false, "使用 960×640 最小窗口启动")
	flag.Parse()
	width, height := 1180, 760
	if *minimum {
		width, height = 960, 640
	}
	a := newApp()
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{Title: "Agent Manager · 原生样本", Width: width, Height: height, MinWidth: 960, MinHeight: 640, Content: ui.View(a.view)})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// Native sample entry point.
