package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/lizhian/agent-manager/internal/processenv"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const DefaultDebugTimeout = 30 * time.Second

type DebugOptions struct {
	Timeout     time.Duration
	Environment func() []string
	UserPath    func(context.Context, string) string
}
type Debugger struct {
	service *Service
	options DebugOptions
}
type Tool struct {
	Name, Title, Description string
	InputSchema              json.RawMessage
}
type CallOutput struct {
	Result     json.RawMessage
	DurationMs int64
}

func NewDebugger(service *Service, options DebugOptions) *Debugger {
	if options.Timeout <= 0 {
		options.Timeout = DefaultDebugTimeout
	}
	if options.Environment == nil {
		options.Environment = os.Environ
	}
	if options.UserPath == nil {
		options.UserPath = processenv.ResolvePath
	}
	return &Debugger{service, options}
}
func (d *Debugger) server(id int64) (Server, error) {
	snap, err := d.service.Snapshot()
	if err != nil {
		return Server{}, err
	}
	for _, server := range snap.Servers {
		if server.ID == id {
			return server, nil
		}
	}
	return Server{}, fmt.Errorf("读取 MCP 调试配置失败：未找到服务 %d", id)
}
func (d *Debugger) ListTools(ctx context.Context, id int64) ([]Tool, error) {
	server, err := d.server(id)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, d.options.Timeout)
	defer cancel()
	session, cleanup, err := d.connect(ctx, server)
	if err != nil {
		return nil, debugError("连接并读取工具", server.Name, ctx, err)
	}
	defer cleanup()
	tools := []Tool{}
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			return nil, debugError("读取工具", server.Name, ctx, err)
		}
		schema, err := json.Marshal(tool.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("读取 MCP %s 工具 schema 失败", server.Name)
		}
		tools = append(tools, Tool{tool.Name, tool.Title, tool.Description, schema})
	}
	return tools, nil
}
func (d *Debugger) CallTool(ctx context.Context, id int64, name, arguments string) (CallOutput, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(arguments), &object); err != nil || object == nil {
		return CallOutput{}, fmt.Errorf("调用 MCP 工具失败：参数必须是 JSON 对象")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return CallOutput{}, fmt.Errorf("调用 MCP 工具失败：工具名称不能为空")
	}
	server, err := d.server(id)
	if err != nil {
		return CallOutput{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, d.options.Timeout)
	defer cancel()
	started := time.Now()
	session, cleanup, err := d.connect(ctx, server)
	if err != nil {
		return CallOutput{}, debugError("连接并调用工具", server.Name, ctx, err)
	}
	defer cleanup()
	result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: object})
	if err != nil {
		return CallOutput{}, debugError("调用工具 "+name, server.Name, ctx, err)
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return CallOutput{}, fmt.Errorf("读取 MCP %s 调用结果失败", server.Name)
	}
	return CallOutput{encoded, time.Since(started).Milliseconds()}, nil
}
func debugError(action, name string, ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("MCP %s %s失败：连接或响应超时", name, action)
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return fmt.Errorf("MCP %s %s已取消", name, action)
	}
	var config *remoteConfigError
	if errors.As(err, &config) {
		return fmt.Errorf("MCP %s %s失败：%s", name, action, config.message)
	}
	var status *remoteStatusError
	if errors.As(err, &status) {
		return fmt.Errorf("MCP %s %s失败：HTTP %d，请检查 URL、凭据和服务状态", name, action, status.code)
	}
	var pathError *exec.Error
	if errors.As(err, &pathError) {
		return fmt.Errorf("MCP %s %s失败：找不到可执行命令，请检查安装与 PATH", name, action)
	}
	// Remote errors can contain credentials or user arguments; retain the failing stage, not their payload.
	return fmt.Errorf("MCP %s %s失败：无法完成协议交互，请检查命令、连接参数和服务状态", name, action)
}
func (d *Debugger) connect(ctx context.Context, server Server) (*sdk.ClientSession, func(), error) {
	if server.Transport == "http" || server.Transport == "sse" {
		return d.connectRemote(ctx, server)
	}
	if server.Transport != "stdio" {
		return nil, nil, fmt.Errorf("不支持的 MCP 传输 %s", server.Transport)
	}
	originalEnv := environmentMap(d.options.Environment())
	env := make(map[string]string, len(originalEnv))
	for key, value := range originalEnv {
		env[key] = value
	}
	path := d.options.UserPath(ctx, env["PATH"])
	env["PATH"] = path
	for key, value := range server.Env {
		env[key] = ExpandEnvironment(value, originalEnv)
	}
	executable, err := processenv.FindExecutable(server.Command, env["PATH"])
	if err != nil {
		return nil, nil, err
	}
	command := exec.CommandContext(ctx, executable, server.Args...)
	for key, value := range env {
		command.Env = append(command.Env, key+"="+value)
	}
	release, err := ownProcessGroup(command)
	if err != nil {
		return nil, nil, err
	}
	session, err := newProtocolClient().Connect(ctx, &sdk.CommandTransport{Command: command, TerminateDuration: 100 * time.Millisecond}, &sdk.ClientSessionOptions{ProtocolVersion: "2025-06-18"})
	if err != nil {
		release()
		return nil, nil, err
	}
	return session, func() { release(); _ = session.Close() }, nil
}

var placeholder = regexp.MustCompile(`\$\{([^}]+)\}`)

func ExpandEnvironment(value string, env map[string]string) string {
	return placeholder.ReplaceAllStringFunc(value, func(token string) string {
		if item, ok := env[token[2:len(token)-1]]; ok {
			return item
		}
		return token
	})
}
func environmentMap(values []string) map[string]string {
	env := map[string]string{}
	for _, item := range values {
		if key, value, ok := strings.Cut(item, "="); ok {
			env[key] = value
		}
	}
	return env
}

// SDK diagnostics may contain remote payloads; expose only sanitized application errors.
func newProtocolClient() *sdk.Client {
	return sdk.NewClient(&sdk.Implementation{Name: "agent-manager", Version: "0.2.1"}, &sdk.ClientOptions{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
}
