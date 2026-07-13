# Agent Manager

用于管理 Agent Skills、MCP、Agent 配置和全局 AGENTS.md 的桌面应用。

当前已实现 Skills Manager 和 AGENTS.md Manager：

- 通过 `owner/repo` 安装和更新 GitHub Skill 来源。
- 将来源镜像到 `~/.agent-manager/skills/`，使用 `~/.agent-manager/agent-manager.db` 保存应用设置和 Skills 元数据。
- 通过单个开关将 Skill 全局启用到 `~/.agents/skills/`，点击后立即生效。
- 更新时保留仍存在 Skill 的启用状态，并清理上游已删除 Skill 的受管软链。
- 支持树状浏览、应用内查看 `SKILL.md`、逐来源更新和全部更新。
- 删除来源前展示影响范围，并同步清理全局激活链接。
- 使用 Markdown 片段维护全局规则，支持新增、查看、修改、删除、启用和拖拽排序。
- 按启用片段的顺序生成 `~/.agents/AGENTS.md`；首次接管已有文件时自动保留备份，全部禁用后恢复原文件。

## Stack

- Tauri v2
- Rust backend
- SQLite
- SvelteKit + TypeScript frontend
- Vite

## Development

```bash
npm install
npm run tauri dev
```

Skills 的安装和更新依赖本机可运行 `npx skills`。

首次启动新版本时，旧 `~/.agent-manager/install-skills.json` 会在事务导入 SQLite 成功后自动删除。数据库通过 `schema_migrations` 管理后续结构升级；当前包含 `app_settings`、`skill_sources`、`skills` 和 `agents_md_fragments` 业务表。

## Scripts

- `npm run dev` starts the frontend dev server.
- `npm run build` builds the frontend.
- `npm run check` runs Svelte type checks.
- `npm run tauri` runs the Tauri CLI.

## Quality checks

```bash
npm run check
npm run build
cargo fmt --check --manifest-path src-tauri/Cargo.toml
cargo test --manifest-path src-tauri/Cargo.toml
cargo clippy --manifest-path src-tauri/Cargo.toml -- -D warnings
```
