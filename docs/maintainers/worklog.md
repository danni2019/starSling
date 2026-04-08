# Maintainer Worklog

本文件用于维护者/agent 的当前任务计划、审批状态、验证记录和必要的 postmortem。

规则：

- 保持精简，只记录当前活跃任务与当前任务相关的纠偏信息。
- 稳定的公开文档放到 `docs/project/` 或 `docs/release/`，不要继续堆在这里。
- 历史细节依赖 git history，不把本文件演化成长期变更日志。

## Current Task

### Name

Guide first-run runtime bootstrap from inside the app

### Request Summary

在确认 macOS 无法提供 `openctp` 自包含 release 之后，将首次初始化路径收敛为“用户下载压缩包并先运行 `./starsling`，由应用内引导完成 Python runtime bootstrap”，而不是要求用户先阅读文档再手动执行脚本。

### Plan

1. 为主界面增加 runtime 缺失引导，并在 `Live market data` 入口优先检查 bundled runtime。
2. 复用现有 `Setup Python runtime` 页面，支持引导式自动运行 bootstrap。
3. 在 bootstrap 成功后按来源恢复流程：
   - 若来自 `Live` 入口，则自动重试进入 Live；
   - 若仅是首次启动提醒，则停留在 setup/main 流程中。
4. 更新 README / release 文档，把用户路径改成“下载 -> 解压 -> 运行 `./starsling` -> 按应用内引导完成初始化”。
5. 补充 `internal/tui` 测试并运行 `go test ./internal/tui`、`go test ./...`。

### Approval

Approved by user in-thread on 2026-04-08 ("ok").

### Validation

- `gofmt -w internal/tui/app.go internal/tui/bootstrap_flow.go internal/tui/view_main.go internal/tui/view_setup.go internal/tui/view_main_test.go` passed.
- `go test ./internal/tui` passed.
- `go test ./...` passed.
- Updated user-facing docs:
  - `README.md`
  - `docs/release/public-readiness.md`
  - `docs/release/macos-prerelease.md`

### Postmortem

- None for this task.

## Template

Use this structure for the next task:

```md
## Current Task

### Name

<task name>

### Request Summary

<one short paragraph>

### Plan

1. ...
2. ...

### Approval

Pending explicit user approval.

### Validation

- Pending.

### Postmortem

- Only include when a task-level mistake needs to be recorded.
```
