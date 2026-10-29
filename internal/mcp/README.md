# 独立 MCP 配置与调试

`Service` 只操作应用 SQLite；兼容字段 `agents_json` 和 `enabled` 不参与运行，更新时保留。`Debugger` 每次从已保存配置读取，stdio 的读取工具和调用各建立独立连接。默认总操作期限 30 秒，可注入短期限验证。参数只接受 JSON 对象，结果保留完整协议结构和耗时。

桌面可执行程序通过 `internal/processenv` 的登录 shell PATH 查找；env 值中的 `${VAR}` 从本次进程环境快照解析，未知占位符保持原文，`$VAR` 不展开。不会经过 shell 执行服务命令，不打印配置、环境或服务 stderr。错误保留失败阶段并隐藏远端错误正文，防止其携带凭据或调用正文。

本轮 macOS 上为每次调试建立独立进程组；操作结束、失败、取消或超时回收组内进程，再关闭 SDK session 并等待直接子进程退出，避免 npx 等包装进程留下子进程。不会停止其他服务或创建常驻连接。Windows 进程组回收尚不支持，本轮不验收。

本地无网络验收服务：

```sh
go build -o /tmp/agent-manager-mcp-fixture ./cmd/mcp-fixture
```

在 MCP 工作区新建 stdio 配置，命令填写 `/tmp/agent-manager-mcp-fixture`，不需要参数。保存后切到“工具调试”，读取工具并调用 `echo`，参数为 `{"message":"你好"}`。可添加环境变量 `MCP_FIXTURE_VALUE` 验证值追加与占位符。输入 `[]` 可验证 JSON 错误反馈。该 fixture 不读取用户文件或访问网络。

测试通过应用服务和原生工作区边界验证保存配置、disabled 兼容、工具列表/schema、调用、无效 JSON、协议/启动失败、超时、父子进程清理、桌面 PATH 与切换服务后拒绝旧结果。真实窗口验证由集成分支记录。
