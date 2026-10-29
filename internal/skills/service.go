// Package skills reads the compatible catalog and confines document access to registered mirrors.
package skills

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/lizhian/agent-manager/internal/storage"
	"gopkg.in/yaml.v3"
)

type Skill struct {
	ID                     int64
	Name, Description      string
	GlobalEnabled          bool
	InstalledAt, UpdatedAt string
}
type Source struct {
	ID                                         int64
	Source, SourceSafe, InstalledAt, UpdatedAt string
	Skills                                     []Skill
}
type Snapshot struct {
	Sources                          []Source
	RepositoryPath, GlobalTargetPath string
	EnabledContentChars              int
}
type Detail struct {
	Source, SourceSafe, Name, Description, MetadataName, MetadataDescription, Path, Content, Markdown, UpdatedAt string
	GlobalEnabled                                                                                                bool
}
type Document struct{ SourceSafe, SkillName, Title, RelativePath, Content string }
type Options struct{ Open func(string) error }
type Service struct {
	store   *storage.Store
	options Options
	mu      sync.Mutex
}

func New(store *storage.Store, options Options) *Service {
	if options.Open == nil {
		options.Open = systemOpen
	}
	return &Service{store: store, options: options}
}
func (s *Service) RepositoryPath() string   { return filepath.Join(s.store.Paths.Root, "skills") }
func (s *Service) GlobalTargetPath() string { return filepath.Join(s.store.Paths.AgentsDir, "skills") }
func (s *Service) Snapshot() (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.migrateLegacy(); err != nil {
		return Snapshot{}, err
	}
	return s.snapshot()
}
func (s *Service) snapshot() (Snapshot, error) {
	result := Snapshot{RepositoryPath: s.RepositoryPath(), GlobalTargetPath: s.GlobalTargetPath()}
	rows, err := s.store.DB.Query("SELECT id,source,source_safe,installed_at,updated_at FROM skill_sources ORDER BY source")
	if err != nil {
		return result, fmt.Errorf("读取 Skills 来源失败：%w", err)
	}
	for rows.Next() {
		var v Source
		if err = rows.Scan(&v.ID, &v.Source, &v.SourceSafe, &v.InstalledAt, &v.UpdatedAt); err != nil {
			rows.Close()
			return result, err
		}
		result.Sources = append(result.Sources, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	for i := range result.Sources {
		source := &result.Sources[i]
		rows, err = s.store.DB.Query("SELECT id,name,description,global_enabled,installed_at,updated_at FROM skills WHERE source_id=? ORDER BY name", source.ID)
		if err != nil {
			return result, err
		}
		for rows.Next() {
			var v Skill
			if err = rows.Scan(&v.ID, &v.Name, &v.Description, &v.GlobalEnabled, &v.InstalledAt, &v.UpdatedAt); err != nil {
				rows.Close()
				return result, err
			}
			source.Skills = append(source.Skills, v)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return result, err
		}
		for _, v := range source.Skills {
			if v.GlobalEnabled {
				p, e := s.documentPath(*source, v.Name, v.Name+"/SKILL.md")
				if e == nil {
					if body, e := os.ReadFile(p); e == nil {
						result.EnabledContentChars += utf8.RuneCount(body)
					}
				}
			}
		}
	}
	return result, nil
}
func validComponent(v string) bool {
	return v != "" && v != "." && v != ".." && filepath.Base(v) == v && !strings.ContainsAny(v, "/\\\x00")
}
func (s *Service) registered(sourceSafe, name string) (Source, Skill, error) {
	snap, err := s.snapshot()
	if err != nil {
		return Source{}, Skill{}, err
	}
	for _, source := range snap.Sources {
		if source.SourceSafe == sourceSafe {
			for _, skill := range source.Skills {
				if skill.Name == name {
					return source, skill, nil
				}
			}
		}
	}
	return Source{}, Skill{}, fmt.Errorf("未找到 Skill %s/%s", sourceSafe, name)
}
func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
func (s *Service) documentPath(source Source, name, relative string) (string, error) {
	if !validComponent(source.SourceSafe) || !validComponent(name) || relative == "" || filepath.IsAbs(relative) || !strings.EqualFold(filepath.Ext(relative), ".md") {
		return "", fmt.Errorf("Markdown 文档路径无效：%s", relative)
	}
	root, err := filepath.EvalSymlinks(filepath.Join(s.RepositoryPath(), source.SourceSafe))
	if err != nil {
		return "", fmt.Errorf("解析来源 %s 目录失败：%w", source.Source, err)
	}
	repository, err := filepath.EvalSymlinks(s.RepositoryPath())
	if err != nil || !within(repository, root) {
		return "", fmt.Errorf("来源 %s 目录越界", source.Source)
	}
	p, err := filepath.EvalSymlinks(filepath.Join(root, relative))
	if err != nil {
		return "", fmt.Errorf("读取本地文档 %s 失败：%w", relative, err)
	}
	if !within(root, p) {
		return "", fmt.Errorf("拒绝读取来源目录之外的文档：%s", relative)
	}
	info, err := os.Stat(p)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("Markdown 文档不是普通文件：%s", relative)
	}
	return p, nil
}
func (s *Service) Detail(sourceSafe, name string) (Detail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	source, skill, err := s.registered(sourceSafe, name)
	if err != nil {
		return Detail{}, err
	}
	p, err := s.documentPath(source, name, name+"/SKILL.md")
	if err != nil {
		return Detail{}, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return Detail{}, fmt.Errorf("读取 %s/SKILL.md 失败：%w", name, err)
	}
	meta := metadata(string(data))
	if meta.Name == "" {
		meta.Name = name
	}
	return Detail{Source: source.Source, SourceSafe: sourceSafe, Name: name, Description: skill.Description, MetadataName: meta.Name, MetadataDescription: skill.Description, Path: filepath.Dir(p), Content: string(data), Markdown: markdownBody(string(data)), UpdatedAt: skill.UpdatedAt, GlobalEnabled: skill.GlobalEnabled}, nil
}
func (s *Service) Document(sourceSafe, name, relative string) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	source, _, err := s.registered(sourceSafe, name)
	if err != nil {
		return Document{}, err
	}
	p, err := s.documentPath(source, name, relative)
	if err != nil {
		return Document{}, err
	}
	body, err := os.ReadFile(p)
	if err != nil {
		return Document{}, fmt.Errorf("读取文档 %s 失败：%w", relative, err)
	}
	root, err := filepath.EvalSymlinks(filepath.Join(s.RepositoryPath(), sourceSafe))
	if err != nil {
		return Document{}, err
	}
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return Document{}, err
	}
	return Document{SourceSafe: sourceSafe, SkillName: name, Title: filepath.Base(p), RelativePath: filepath.ToSlash(rel), Content: string(body)}, nil
}
func ResolveDocumentPath(current, href string) (string, error) {
	u, err := url.Parse(href)
	if err != nil || u.IsAbs() || u.Host != "" || strings.HasPrefix(href, "/") {
		return "", fmt.Errorf("本地文档链接无效：%s", href)
	}
	p, err := url.PathUnescape(u.EscapedPath())
	if err != nil || p == "" {
		return "", fmt.Errorf("本地文档链接无效：%s", href)
	}
	return path.Clean(path.Join(path.Dir(current), p)), nil
}
func (s *Service) OpenSkill(sourceSafe, name string) error {
	d, err := s.Detail(sourceSafe, name)
	if err != nil {
		return err
	}
	return s.open(d.Path)
}
func (s *Service) OpenDocument(sourceSafe, name, relative string) error {
	d, err := s.Document(sourceSafe, name, relative)
	if err != nil {
		return err
	}
	return s.open(filepath.Join(s.RepositoryPath(), sourceSafe, filepath.FromSlash(d.RelativePath)))
}
func (s *Service) OpenGlobal() error {
	p := s.GlobalTargetPath()
	if err := os.MkdirAll(p, 0700); err != nil {
		return fmt.Errorf("创建全局 Skills 目录失败：%w", err)
	}
	return s.open(p)
}
func (s *Service) open(p string) error {
	if err := s.options.Open(p); err != nil {
		return fmt.Errorf("打开 %s 失败：%w", p, err)
	}
	return nil
}
func systemOpen(p string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("当前仅支持 macOS 系统打开")
	}
	out, err := exec.Command("/usr/bin/open", p).CombinedOutput()
	if err != nil {
		return fmt.Errorf("系统打开失败：%w：%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

type frontmatter struct{ Name, Description string }

func metadata(body string) frontmatter {
	body = strings.TrimPrefix(strings.ReplaceAll(body, "\r\n", "\n"), "\ufeff")
	lines := strings.Split(body, "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "---" {
		return frontmatter{}
	}
	end := 1
	for end < len(lines) && strings.TrimSpace(lines[end]) != "---" {
		end++
	}
	var m frontmatter
	if yaml.Unmarshal([]byte(strings.Join(lines[1:end], "\n")), &m) != nil {
		return frontmatter{}
	}
	m.Name = strings.TrimSpace(m.Name)
	m.Description = strings.TrimSpace(m.Description)
	return m
}

// OpenURL permits only ordinary web links from a document, never file or command schemes.
func (s *Service) OpenURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("外部链接无效：%s", raw)
	}
	return s.open(raw)
}

// Keep the original source for character counts and system-open; frontmatter is already shown as metadata.
func markdownBody(body string) string {
	normalized := strings.TrimPrefix(strings.ReplaceAll(body, "\r\n", "\n"), "\ufeff")
	lines := strings.Split(normalized, "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "---" {
		return body
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return strings.Join(lines[i+1:], "\n")
		}
	}
	return body
}
