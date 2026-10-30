# Agent Manager

Go + MyGo 原生桌面工具，管理 Skills、独立 MCP 服务配置/工具调试、全局 AGENTS.md 和界面设置。界面由原生绘制，不启动 WebView；应用启动、构建和打包使用 Go，不需要 Node 或 Rust。

本轮支持并验收 macOS ARM64。Windows、Linux、macOS Intel 尚未交付；见 [平台范围决定](docs/adr/0003-macos-arm-validation-scope.md)。应用身份为 `com.lizhian.agentmanager`，版本唯一来源为 [Info.plist](assets/macos/Info.plist) 的 `CFBundleShortVersionString`。

## 启动与开发

需要 Go 1.27.3（见 `.go-version`）和 Xcode Command Line Tools；SQLite 驱动需要 CGO。MyGo 固定为 v0.7.3。

```sh
CGO_ENABLED=1 go tool mygo dev
```

CLI 与框架一起锁定为 v0.7.3。该入口启动独立的 `Agent Manager Dev`，修改 Go 文件后自动重新构建、重启；编译失败时保留上一次运行的版本。退出窗口或按 Ctrl+C 结束开发运行。

开发模式默认将数据库、镜像和受管文件放到项目的 `.mygo/data/.agent-manager` 与 `.mygo/data/.agents`，重启后保留测试数据；`.mygo` 不提交。正式打包仍使用下方脚本，以 Info.plist 为正式版本来源，mygo.json 仅配置开发入口。

需要手动指定隔离目录或最小窗口时：

```sh
go run ./cmd/agent-manager --data-dir /tmp/agent-manager-validation --minimum
# 使用真实用户配置：
go run ./cmd/agent-manager
```

默认窗口1180×760，最小960×640。Skills 安装/更新仍通过外部 `npx skills`，使用这些功能时需要 Node/npm/npx；这不是应用启动或构建依赖。

## 检查和打包

```sh
scripts/check.sh
scripts/package-macos.sh
```

检查运行 Go 格式、公开业务/原生视图测试、race 检查、vet、构建及独立历史样本验证。打包生成 `dist/Agent Manager.app`、版本化 ARM64 zip 和 SHA256。产物为本机 ad hoc 签名，未使用 Developer ID 凭据、未公证。本轮没有实现自动更新。CI 使用 macOS ARM64，tag 与版本不一致时拒绝打包；产物作为 Actions artifacts 保存，不自动发布 GitHub Release。

## 数据与行为

- 元数据与设置：`~/.agent-manager/agent-manager.db`，保留现有 SQLite schema、主键和未迁移字段。
- Skills 镜像：`~/.agent-manager/skills/`；全局激活：`~/.agents/skills/`。支持来源安装、更新、全部更新、删除影响确认、正文与关联 Markdown。仅处理受管链接，保留手工文件。
- AGENTS.md：片段保存在 SQLite；启用片段立即按顺序生成 `~/.agents/AGENTS.md`。接管手工文件先备份，全部禁用后恢复。搜索/筛选状态禁排。
- MCP：stdio、streamable HTTP、SSE 独立配置与调试，默认30秒操作超时，操作结束释放连接/拥有的进程；无需 GAAL 或外部 Agent。
- 五档字号、系统字体与文档预览位置/比例持久化，Skills 与 AGENTS.md 使用同一偏好。

`--data-dir DIR` 将上述目录分别置于 `DIR/.agent-manager` 和 `DIR/.agents`。自动测试只使用临时目录、fake 外部命令及可控本地 MCP 服务。旧 JSON catalog 由 Skills 模块在事务导入和链接迁移成功后处理；旧 Agents/分发数据、GAAL 文件与外部 Agent 配置保留。

开发边界见 [AGENTS.md](AGENTS.md)。历史样本及其证据位于 [native-sample](native-sample/README.md)；它只用于早期验证，最终集成验收另行记录。
