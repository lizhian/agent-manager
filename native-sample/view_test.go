package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
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

func TestSplitterLimitsKeepMinimumWindowUsable(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 960, 640)
	// Move the outer divider toward each extreme through the public pointer seam.
	for _, x := range []float32{0, 950} {
		search, _ := tt.Find("搜索片段")
		tt.Press(search.X+search.W+4, 300)
		tt.Move(x, 300)
		tt.Release(x, 300)
		if a.listWidth < 240 || a.listWidth > 360 {
			t.Fatalf("list width %v", a.listWidth)
		}
	}
	// The editor and preview divider must preserve readable panes and20–80% limits.
	for _, x := range []float32{0, 950} {
		name, _ := tt.Find("片段名称")
		tt.Press(name.X+name.W+8, 300)
		tt.Move(x, 300)
		tt.Release(x, 300)
		if a.previewPercent < 20 || a.previewPercent > 80 {
			t.Fatalf("preview percentage %v", a.previewPercent)
		}
		name, _ = tt.Find("片段名称")
		preview, _ := tt.Find("单片段预览")
		if name.W < 200 || preview.X+200 > 952 {
			t.Fatalf("unusable panes: editor%+v preview%+v", name, preview)
		}
	}
	if err := tt.Click("底部预览"); err != nil {
		t.Fatal(err)
	}
	for _, y := range []float32{0, 630} {
		body, _ := tt.Find("Markdown 正文")
		tt.Press(body.X+100, body.Y+body.H+8)
		tt.Move(body.X+100, y)
		tt.Release(body.X+100, y)
		if a.previewPercent < 20 || a.previewPercent > 80 {
			t.Fatalf("vertical preview percentage %v", a.previewPercent)
		}
		body, _ = tt.Find("Markdown 正文")
		preview, _ := tt.Find("单片段预览")
		if body.H < 100 || preview.Y+140 > 632 {
			t.Fatalf("unusable vertical panes: editor%+v preview%+v", body, preview)
		}
	}
}

func TestDraggingAtListEdgeScrollsToOffscreenFragments(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 960, 640)
	from, ok := tt.Find("拖动排序 规则片段 01")
	if !ok {
		t.Fatal("missing handle")
	}
	tt.Press(from.X+from.W/2, from.Y+from.H/2)
	tt.Move(100, 620)
	tt.Advance(2 * time.Second)
	// Row30 starts outside the window; dragging near the bottom must reveal it.
	target, ok := tt.Find("规则片段 30")
	if !ok || target.Y < 60 || target.Y+target.H > 632 {
		t.Fatalf("edge drag did not reveal row30: %+v %v", target, ok)
	}
	tt.Move(target.X+target.W/2, target.Y+target.H/2)
	tt.Release(target.X+target.W/2, target.Y+target.H/2)
	if a.sample.Fragments[28].ID != 1 || a.sample.Fragments[29].ID != 30 {
		t.Fatal("drop after edge scroll did not use current target")
	}
}

func TestEditorCompositionSelectionClipboardAndUndo(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 960, 640)
	if err := tt.Click("Markdown 正文"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Compose("zhongwen", 8)
	tt.Type("中文组合输入")
	if a.sample.Fragments[0].Body != "中文组合输入" {
		t.Fatal(a.sample.Fragments[0].Body)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Key(ui.Cmd, ui.KeyC)
	if tt.Clipboard() != "中文组合输入" {
		t.Fatal("selection copy failed")
	}
	tt.Key(0, ui.KeyEnd)
	tt.Key(ui.Cmd, ui.KeyV)
	if a.sample.Fragments[0].Body != "中文组合输入中文组合输入" {
		t.Fatal("paste failed")
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	if a.sample.Fragments[0].Body != "中文组合输入" {
		t.Fatal("undo failed")
	}
}

func TestKeyboardCanReachAndToggleFragmentAfterDialog(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 960, 640)
	if err := tt.Click("重置样本"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Key(0, ui.KeyEnter)
	if !a.resetOpen {
		t.Fatal("Enter did not reopen focused trigger")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("搜索片段") {
		t.Fatal("Tab did not reach search")
	}
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("拖动排序 规则片段 01") {
		t.Fatal("Tab did not reach handle")
	}
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("启用 规则片段 01") {
		t.Fatal("Tab did not reach switch")
	}
	tt.Key(0, ui.KeySpace)
	if a.sample.Fragments[0].Enabled {
		t.Fatal("Space did not toggle fragment")
	}
}
