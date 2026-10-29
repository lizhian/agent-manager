## Standards

评审 `main`（`41a5fff`）至集成分支 `14b336f`，依据当前 Go 版 AGENTS.md、ADR-0003 和 code-review smell baseline；未修改仓库。

- **[P2] npx 包装子进程会使取消后的 Skills 操作继续阻塞。** `internal/skills/runner.go:21–26` 的 `CommandContext` 只终止直接进程；子进程仍持有 stdout/stderr 时，`Output` 无限等待 EOF，来源写锁不能释放。违反 AGENTS.md“状态、持久化与安全”的可靠后台任务要求。已用公开 `SyncSource`、默认 Runner 和临时 shell/npx fixture 复现：取消600ms后仍阻塞，终止 fixture 子进程才返回。应管理本次进程组并为 pipe/Wait 设置有界清理。

- **[P2] 更新空镜像会保留上游已经删除的最后一个 Skill。** `internal/skills/install.go:79–80` 对成功返回的 `[]` 无条件报错，跳过已有来源的 catalog、镜像和受管链接清理。违反 AGENTS.md“领域契约”的镜像更新规则。公开服务复现：安装并启用 demo，随后 fake 外部命令成功返回空数组，更新失败且 demo 仍保持启用。应区分首次安装为空和已有来源更新为空，后者走现有一致性/回滚链路。

- **[P3] 新开发规范误述 Markdown 访问边界。** AGENTS.md“领域契约”写“限制在当前 Skill 目录”，但 `internal/skills/service.go:163–176` 校验来源目录，允许访问同来源其他 Skill 的文档。该行为沿用 `main` 的 Rust 实现与迁移 spec；建议将规范改成“当前来源目录”，避免后续维护者按错误规范破坏兼容行为。

未发现值得扩大改动范围的 Fowler smell。共3项规范问题，最高优先级为2项P2。新打包/CI未发现额外高置信度问题；`macos-15` 当前确为 ARM64 标准 runner，已核实 GitHub 官方 runner 文档。

复现：`/tmp/agent-manager-spec1/skills-review-repro_test.go` 和 `skills-review-overlay.json`；可运行 `go test -overlay=/tmp/agent-manager-spec1/skills-review-overlay.json ./internal/skills -run TestReview -v -timeout 10s`。这两个实验测试在观察到缺陷时打印 REPRODUCED 并通过，应转成修复前失败的正式回归测试。[GitHub runner reference](https://docs.github.com/en/actions/reference/runners/github-hosted-runners)


## Spec

Reviewed `git diff main...HEAD` at `14b336f18b8c7faf9ad58f3393b76a13b9fdc99b`, fixed point `41a5fffd6acc9a463be073c235591cc39a26a2e8`, against #1 with macOS ARM64-only amendment.

1. **[P2] Settings read recovery can overwrite existing preferences.** `internal/uiapp/app.go:92–96` routes every settings error to “重试保存设置” → `save(draft)`. When initial `Snapshot` fails, `draft` is still defaults; clicking this purported retry writes all three settings keys, replacing valid existing font size/preview preferences alongside the unreadable field. Spec: “沿用已有设置键和编码读取旧偏好” and “失败时保留……可修复上下文”. Read errors must retry `Load`, not save defaults. Also clear stale `w.err` on successful reload (`internal/uiapp/settings.go:36–51`), otherwise a recovered read still appears failed.

2. **[P2] MCP sanitization suppresses safe actionable failure facts.** `internal/mcp/debug.go:120–143`: outside timeout/cancellation/HTTP/lookup cases, all errors become “无法完成协议交互”. Nonzero child exit status and protocol/decode failure category are lost, although spec requires “错误包含动作、对象和根因”. Keep structured status/category while continuing to exclude payloads, arguments, credentials and environment. This carries forward the pre-review finding.

3. **[P2] Some Skills text ignores the five-size preference.** `internal/uiapp/skills.go:191,290–292` and `skillsBatch.go:103` fix progress/operation text at11 and body/warning text at13. Spec: “标题与辅助文字同步调整” and “通过同一组字号令牌同步变化”. These labels stay small under Large/Extra-large despite the rest of the native theme changing; derive their sizes from the theme with the documented minimum.

No additional delivery implementation finding: Go-only build/check/package paths, ARM64 bundle identity/version, ad hoc signing disclosure and CI match the amended scope. [GitHub’s official runner reference](https://docs.github.com/en/actions/how-tos/write-workflows/choose-where-workflows-run/choose-the-runner-for-a-job) confirms `macos-15` is a public ARM64 label. Final packaged-window evidence and full-project build measurements are being gathered by the root and are not counted as implementation failures. Controlled fake-npx testing is explicitly permitted by the spec.

Spec:3 findings; worst is settings read-retry overwriting existing preferences (P2).

## 修复核对

原评审固定点main=41a5fff，完整实现评审点14b336f；同一实现代理提交379a54f及676aad3，最终代码集成点994446f。

Standards原3项已修复：npx及登录shell采用进程组和有界管道清理；已有来源允许空更新并清理最后一个Skill；规范改为来源镜像目录边界。后续核对发现空来源被列表隐藏，已保留更新/删除入口，新增从真实空更新进入视图的回归测试，核对通过。

Spec原3项已修复：设置读取失败重试Load，成功后清除旧错误，回归验证有效偏好未被覆盖；MCP保留安全退出码、JSON/协议/网络类别，不回显敏感载荷；Skills剩余文字跟随主题字号。原评审者针对修复核对，无剩余可行动问题。

实际截图发现特大字号导航截断，改用当前字体测量和显式按钮内边距，960×640/PingFang SC/17px复验完整显示AGENTS.md。来源更新成功仅在实际关闭安装弹窗时返回焦点；原生窗口复验更新期间输入搜索、完成后仍保持搜索焦点。

格式、race测试、Vet、构建及打包通过。未维护框架分支或修改渲染/输入系统。
