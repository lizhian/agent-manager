# Issue tracker: GitHub

本仓库的 issue、spec 和任务使用 `lizhian/agent-manager` 的 GitHub Issues，通过 `gh` 操作。写操作显式指定 `--repo lizhian/agent-manager`；首次创建前使用 `gh repo view --json nameWithOwner,url` 核对目标。

## 常用操作

- 创建：`gh issue create --repo lizhian/agent-manager --title "标题" --body-file <正文文件> --label <标签>`。
- 读取：`gh issue view <number> --repo lizhian/agent-manager --json number,title,body,labels,comments,assignees,state`。
- 查询：`gh issue list --repo lizhian/agent-manager --state open --json number,title,body,labels,assignees`；按需添加标签过滤并注意分页。
- 更新：`gh issue edit <number> --repo lizhian/agent-manager --body-file <正文文件>`；标签通过 `--add-label` 和 `--remove-label` 调整。
- 评论：`gh issue comment <number> --repo lizhian/agent-manager --body-file <评论文件>`。
- 关闭：先发表评论说明结果，再执行 `gh issue close <number> --repo lizhian/agent-manager`；已关闭的 issue 也单独发表评论。

多行正文使用文件传递，保留原有换行。只向本项目发布用户要求的内容。

## 子任务与阻塞关系

优先使用 GitHub 原生 sub-issues 和 issue dependencies。CLI 支持时使用其原生命令；否则通过指定仓库的 `gh api`：

- 附加子任务：向 `repos/lizhian/agent-manager/issues/<parent>/sub_issues` 发送 POST，参数 `sub_issue_id` 为子任务的数据库 ID。
- 添加阻塞：向 `repos/lizhian/agent-manager/issues/<child>/dependencies/blocked_by` 发送 POST，参数 `issue_id` 为阻塞任务的数据库 ID。
- 数据库 ID 使用 `gh api repos/lizhian/agent-manager/issues/<number> --jq .id` 获取，不使用 issue 编号或 node ID。
- 原生关系不可用时，在子任务正文使用 `Part of #<parent>` 和 `Blocked by: #<number>`。只在所有阻塞任务关闭后开始任务。

## 技能约定

“publish to the issue tracker”表示创建 GitHub issue；“fetch the relevant ticket”表示读取该 issue。标签角色见 `triage-labels.md`；明确的 spec 和任务标记 `ready-for-agent`。

**PRs as a request surface: no.**

## Wayfinding operations

- Map：使用标签 `wayfinder:map` 的单一 issue，记录目标、决策和未明确范围。
- Ticket：作为 map 的子 issue，使用 `wayfinder:research`、`wayfinder:prototype`、`wayfinder:grilling` 或 `wayfinder:task`。
- Frontier：查询未关闭的子任务，排除有未关闭阻塞项或已指派的任务，按 map 顺序选择。
- Claim：通过 `gh issue edit <number> --repo lizhian/agent-manager --add-assignee @me` 认领。
- Resolve：先发表评论说明结果，再关闭任务，并向 map 追加决策摘要和链接。
