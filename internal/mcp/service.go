// Package mcp stores independent MCP debugging configurations, without distribution side effects.
package mcp

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/lizhian/agent-manager/internal/storage"
)

type Input struct {
	Name      string
	Transport string
	Command   string
	Args      []string
	Env       map[string]string
	URL       string
	Headers   map[string]string
}
type Server struct {
	ID int64
	Input
	CreatedAt string
	UpdatedAt string
}
type Snapshot struct{ Servers []Server }
type Service struct {
	store *storage.Store
	mu    sync.Mutex
}

func New(store *storage.Store) *Service { return &Service{store: store} }
func (s *Service) Snapshot() (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return read(s.store.DB)
}

type querier interface {
	Query(string, ...any) (*sql.Rows, error)
}

func read(db querier) (Snapshot, error) {
	snap := Snapshot{Servers: []Server{}}
	rows, err := db.Query(`SELECT id,name,transport,command,args_json,env_json,url,headers_json,created_at,updated_at FROM agent_mcp_servers ORDER BY name,id`)
	if err != nil {
		return snap, fmt.Errorf("读取 MCP 服务失败：%w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var server Server
		var args, env, headers string
		if err = rows.Scan(&server.ID, &server.Name, &server.Transport, &server.Command, &args, &env, &server.URL, &headers, &server.CreatedAt, &server.UpdatedAt); err != nil {
			return snap, fmt.Errorf("读取 MCP 服务失败：%w", err)
		}
		for _, field := range []struct {
			raw    string
			target any
			name   string
		}{{args, &server.Args, "参数"}, {env, &server.Env, "环境变量"}, {headers, &server.Headers, "请求头"}} {
			if err = json.Unmarshal([]byte(field.raw), field.target); err != nil {
				return snap, fmt.Errorf("解析 MCP %s %s 失败：配置格式无效", server.Name, field.name)
			}
		}
		snap.Servers = append(snap.Servers, server)
	}
	if err = rows.Err(); err != nil {
		return snap, fmt.Errorf("读取 MCP 服务失败：%w", err)
	}
	return snap, nil
}
func normalize(input Input) (Input, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Command = strings.TrimSpace(input.Command)
	input.URL = strings.TrimSpace(input.URL)
	if input.Name == "" || utf8.RuneCountInString(input.Name) > 80 || strings.ContainsFunc(input.Name, unicode.IsSpace) {
		return input, fmt.Errorf("MCP 名称必须为 1 到 80 个不含空白的字符")
	}
	switch input.Transport {
	case "stdio":
		if input.Command == "" {
			return input, fmt.Errorf("stdio MCP 必须填写命令")
		}
	case "http", "sse":
		if input.URL == "" {
			return input, fmt.Errorf("HTTP/SSE MCP 必须填写 URL")
		}
	default:
		return input, fmt.Errorf("MCP 传输类型必须为 stdio、http 或 sse")
	}
	if input.Args == nil {
		input.Args = []string{}
	}
	if input.Env == nil {
		input.Env = map[string]string{}
	}
	if input.Headers == nil {
		input.Headers = map[string]string{}
	}
	return input, nil
}
func (s *Service) Create(input Input) (Snapshot, error) { return s.save(0, input) }
func (s *Service) Update(id int64, input Input) (Snapshot, error) {
	if id <= 0 {
		return Snapshot{}, fmt.Errorf("更新 MCP 服务失败：无效标识 %d", id)
	}
	return s.save(id, input)
}
func (s *Service) save(id int64, input Input) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	input, err := normalize(input)
	if err != nil {
		return Snapshot{}, err
	}
	tx, err := s.store.DB.Begin()
	if err != nil {
		return Snapshot{}, fmt.Errorf("保存 MCP %s 失败：%w", input.Name, err)
	}
	defer tx.Rollback()
	var count int
	if err = tx.QueryRow(`SELECT count(*) FROM agent_mcp_servers WHERE name=? AND id<>?`, input.Name, id).Scan(&count); err != nil {
		return Snapshot{}, fmt.Errorf("检查 MCP %s 名称失败：%w", input.Name, err)
	}
	if count != 0 {
		return Snapshot{}, fmt.Errorf("MCP 名称 %s 已存在", input.Name)
	}
	args, err := json.Marshal(input.Args)
	if err != nil {
		return Snapshot{}, err
	}
	env, err := json.Marshal(input.Env)
	if err != nil {
		return Snapshot{}, err
	}
	headers, err := json.Marshal(input.Headers)
	if err != nil {
		return Snapshot{}, err
	}
	now := strconv.FormatInt(time.Now().Unix(), 10)
	if id == 0 {
		_, err = tx.Exec(`INSERT INTO agent_mcp_servers(name,transport,command,args_json,env_json,url,headers_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, input.Name, input.Transport, input.Command, string(args), string(env), input.URL, string(headers), now, now)
	} else {
		var result sql.Result
		result, err = tx.Exec(`UPDATE agent_mcp_servers SET name=?,transport=?,command=?,args_json=?,env_json=?,url=?,headers_json=?,updated_at=? WHERE id=?`, input.Name, input.Transport, input.Command, string(args), string(env), input.URL, string(headers), now, id)
		if err == nil {
			var n int64
			n, err = result.RowsAffected()
			if err == nil && n == 0 {
				return Snapshot{}, fmt.Errorf("未找到 MCP 服务 %d", id)
			}
		}
	}
	if err != nil {
		return Snapshot{}, fmt.Errorf("保存 MCP %s 失败：%w", input.Name, err)
	}
	snap, err := read(tx)
	if err != nil {
		return Snapshot{}, err
	}
	if err = tx.Commit(); err != nil {
		return Snapshot{}, fmt.Errorf("保存 MCP %s 失败：%w", input.Name, err)
	}
	return snap, nil
}
func (s *Service) Delete(id int64) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.store.DB.Begin()
	if err != nil {
		return Snapshot{}, fmt.Errorf("删除 MCP %d 失败：%w", id, err)
	}
	defer tx.Rollback()
	result, err := tx.Exec(`DELETE FROM agent_mcp_servers WHERE id=?`, id)
	if err != nil {
		return Snapshot{}, fmt.Errorf("删除 MCP %d 失败：%w", id, err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return Snapshot{}, err
	}
	if n == 0 {
		return Snapshot{}, fmt.Errorf("未找到 MCP 服务 %d", id)
	}
	snap, err := read(tx)
	if err != nil {
		return Snapshot{}, err
	}
	if err = tx.Commit(); err != nil {
		return Snapshot{}, fmt.Errorf("删除 MCP %d 失败：%w", id, err)
	}
	return snap, nil
}
