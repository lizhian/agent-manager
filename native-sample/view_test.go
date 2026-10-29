package main

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestViewCanEditAndCancelReset(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 960, 640)
	if err := tt.Click("片段名称"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("中文修改")
	if a.sample.Fragments[0].Name != "中文修改" {
		t.Fatal(a.sample.Fragments[0].Name)
	}
	if err := tt.Click("重置样本"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEscape)
	if !tt.Focused("重置样本") {
		t.Fatal("dialog did not restore focus to reset trigger")
	}
	if a.resetOpen || a.sample.Fragments[0].Name != "中文修改" {
		t.Fatal("Escape must preserve edits")
	}
}

func TestHandleReordersBeforeTargetRow(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 960, 640)
	from, ok := tt.Find("拖动排序 规则片段 01")
	if !ok {
		t.Fatal("missing handle")
	}
	to, ok := tt.Find("规则片段 03")
	if !ok {
		t.Fatal("missing target")
	}
	tt.Press(from.X+from.W/2, from.Y+from.H/2)
	tt.Move(to.X+to.W/2, to.Y+to.H/2)
	tt.Release(to.X+to.W/2, to.Y+to.H/2)
	if a.sample.Fragments[0].ID != 2 || a.sample.Fragments[1].ID != 1 || a.sample.Fragments[2].ID != 3 {
		t.Fatal(a.sample.Fragments[:3])
	}
	if a.sample.Selected != 1 {
		t.Fatal("reordering lost selection")
	}
}

func TestSearchHasEmptyFeedbackWithoutChangingOrder(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 960, 640)
	if err := tt.Click("搜索片段"); err != nil {
		t.Fatal(err)
	}
	tt.Type("找不到的内容")
	if !tt.HasText("没有匹配片段") {
		t.Fatal("missing empty feedback")
	}
	if a.sample.Fragments[0].ID != 1 {
		t.Fatal("search changed order")
	}
}
