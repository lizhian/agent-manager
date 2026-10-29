// Package settings preserves the existing appearance and document preference contract.
package settings

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/lizhian/agent-manager/internal/storage"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type FontSize string

var Sizes = []FontSize{"extra-small", "small", "standard", "large", "extra-large"}

func (s FontSize) Pixels() float32 {
	for i, value := range Sizes {
		if value == s {
			return float32(13 + i)
		}
	}
	return 15
}

type Preview struct {
	Position string  `json:"position"`
	Ratio    float64 `json:"ratio"`
}
type Snapshot struct {
	FontSize   FontSize
	FontFamily string
	Preview    Preview
}

func Defaults() Snapshot {
	return Snapshot{FontSize: "standard", Preview: Preview{Position: "bottom", Ratio: 0.5}}
}

type Options struct {
	Now   func() string
	Fonts func() ([]string, error)
}
type Service struct {
	store *storage.Store
	now   func() string
	fonts func() ([]string, error)
}

func New(store *storage.Store, options Options) *Service {
	if options.Now == nil {
		options.Now = func() string { return fmt.Sprint(time.Now().Unix()) }
	}
	return &Service{store: store, now: options.Now, fonts: options.Fonts}
}

type querier interface {
	Query(string, ...any) (*sql.Rows, error)
}

func (s *Service) Snapshot() (Snapshot, error) { return read(s.store.DB) }
func read(q querier) (Snapshot, error) {
	snapshot := Defaults()
	rows, err := q.Query("SELECT key,value_json FROM app_settings WHERE key IN ('appearance.font_size_v3','appearance.font_family','preview.document_layout')")
	if err != nil {
		return snapshot, fmt.Errorf("读取界面设置失败：%w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err = rows.Scan(&key, &value); err != nil {
			return snapshot, err
		}
		switch key {
		case "appearance.font_size_v3":
			err = json.Unmarshal([]byte(value), &snapshot.FontSize)
		case "appearance.font_family":
			err = json.Unmarshal([]byte(value), &snapshot.FontFamily)
		case "preview.document_layout":
			err = json.Unmarshal([]byte(value), &snapshot.Preview)
			if err == nil && !validRatio(snapshot.Preview.Ratio) {
				snapshot.Preview = Defaults().Preview
			}
		}
		if err != nil {
			return snapshot, fmt.Errorf("解析设置 %s 失败：%w", key, err)
		}
	}
	if err = rows.Err(); err != nil {
		return snapshot, err
	}
	if err = validate(snapshot); err != nil {
		return snapshot, err
	}
	return snapshot, nil
}
func validRatio(r float64) bool { return !math.IsNaN(r) && !math.IsInf(r, 0) && r >= 0.2 && r <= 0.8 }
func validate(value Snapshot) error {
	found := false
	for _, size := range Sizes {
		if value.FontSize == size {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("字号 %q 无效", value.FontSize)
	}
	if utf8.RuneCountInString(strings.TrimSpace(value.FontFamily)) > 128 {
		return fmt.Errorf("字体名称不能超过 128 个字符")
	}
	for _, r := range value.FontFamily {
		if unicode.IsControl(r) {
			return fmt.Errorf("字体名称包含无效字符")
		}
	}
	if value.Preview.Position != "right" && value.Preview.Position != "bottom" {
		return fmt.Errorf("文档预览位置无效")
	}
	if !validRatio(value.Preview.Ratio) {
		return fmt.Errorf("文档预览占用比例必须在 20%% 到 80%% 之间")
	}
	return nil
}
func (s *Service) Save(value Snapshot) (Snapshot, error) {
	value.FontFamily = strings.TrimSpace(value.FontFamily)
	if err := validate(value); err != nil {
		return Snapshot{}, err
	}
	tx, err := s.store.DB.Begin()
	if err != nil {
		return Snapshot{}, fmt.Errorf("保存界面设置失败：%w", err)
	}
	defer tx.Rollback()
	entries := []struct {
		key   string
		value any
	}{{"appearance.font_size_v3", value.FontSize}, {"appearance.font_family", value.FontFamily}, {"preview.document_layout", value.Preview}}
	for _, entry := range entries {
		encoded, err := json.Marshal(entry.value)
		if err != nil {
			return Snapshot{}, err
		}
		if _, err = tx.Exec("INSERT INTO app_settings(key,value_json,updated_at) VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json,updated_at=excluded.updated_at", entry.key, string(encoded), s.now()); err != nil {
			return Snapshot{}, fmt.Errorf("保存设置 %s 失败：%w", entry.key, err)
		}
	}
	saved, err := read(tx)
	if err != nil {
		return Snapshot{}, err
	}
	if err = tx.Commit(); err != nil {
		return Snapshot{}, fmt.Errorf("提交界面设置失败：%w", err)
	}
	return saved, nil
}
func (s *Service) Fonts() ([]string, error) {
	if s.fonts == nil {
		return nil, fmt.Errorf("读取系统字体失败：未配置字体枚举")
	}
	fonts, err := s.fonts()
	if err != nil {
		return nil, fmt.Errorf("读取系统字体失败：%w", err)
	}
	if len(fonts) == 0 {
		return nil, fmt.Errorf("读取系统字体失败：系统未返回字体")
	}
	sort.Slice(fonts, func(i, j int) bool { return strings.ToLower(fonts[i]) < strings.ToLower(fonts[j]) })
	out := []string{}
	for _, font := range fonts {
		if len(out) == 0 || !strings.EqualFold(out[len(out)-1], font) {
			out = append(out, font)
		}
	}
	return out, nil
}
