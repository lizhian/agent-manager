package main

import "testing"

func TestEnabledPreviewFollowsSavedOrder(t *testing.T) {
	s := NewSample()
	s.Fragments = []Fragment{{ID: 1, Name: "一", Body: "# 一", Enabled: true}, {ID: 2, Name: "二", Body: "# 二", Enabled: false}, {ID: 3, Name: "三", Body: "# 三", Enabled: true}}
	if got := s.Combined(); got != "# 一\n\n# 三" {
		t.Fatal(got)
	}
	s.MoveBefore(3, 1)
	if got := s.Combined(); got != "# 三\n\n# 一" {
		t.Fatal(got)
	}
}
