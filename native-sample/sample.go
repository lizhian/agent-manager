package main

import (
	"fmt"
	"strings"
)

type Fragment struct {
	ID         int
	Name, Body string
	Enabled    bool
}

// Sample owns only disposable memory; it has no database or filesystem dependency.
type Sample struct {
	Fragments []Fragment
	Selected  int
}

func NewSample() *Sample {
	s := &Sample{Selected: 1}
	for i := 1; i <= 40; i++ {
		s.Fragments = append(s.Fragments, Fragment{ID: i, Name: fmt.Sprintf("规则片段 %02d", i), Body: fmt.Sprintf("# 规则片段 %02d\n\n中文编辑与 **Markdown** 预览。\n\n- 保护用户数据\n- 仅修改必要内容\n\n| 动作 | 结果 |\n| --- | --- |\n| 保存 | 立即生效 |\n\n```go\nfmt.Println(\"你好\")\n```\n", i), Enabled: i <= 3})
	}
	return s
}
func (s *Sample) Combined() string {
	var parts []string
	for _, f := range s.Fragments {
		if f.Enabled {
			parts = append(parts, strings.TrimSpace(f.Body))
		}
	}
	return strings.Join(parts, "\n\n")
}

// MoveBefore moves a stable ID before another; zero is the end of the list.
func (s *Sample) MoveBefore(id, before int) {
	if id == before {
		return
	}
	from, to := -1, len(s.Fragments)
	for i, f := range s.Fragments {
		if f.ID == id {
			from = i
		}
		if f.ID == before {
			to = i
		}
	}
	if from < 0 || (before != 0 && to == len(s.Fragments)) {
		return
	}
	f := s.Fragments[from]
	s.Fragments = append(s.Fragments[:from], s.Fragments[from+1:]...)
	if from < to {
		to--
	}
	s.Fragments = append(s.Fragments, Fragment{})
	copy(s.Fragments[to+1:], s.Fragments[to:])
	s.Fragments[to] = f
}
