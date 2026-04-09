# Maintainer Worklog

本文件用于维护者/agent 的当前任务计划、审批状态、验证记录和必要的 postmortem。

规则：

- 保持精简，只记录当前活跃任务与当前任务相关的纠偏信息。
- 稳定的公开文档放到 `docs/project/` 或 `docs/release/`，不要继续堆在这里。
- 历史细节依赖 git history，不把本文件演化成长期变更日志。

## Current Task

### Name

Ensure metadata readiness before Live startup

### Request Summary

修复 Linux release 场景下进入 `Live market data` 前未把 contract metadata 作为显式门禁的问题。目标是：进入 Live 前确保 metadata 已可用；如果本地缺失或过期则主动刷新；刷新失败时阻止进入空白 Live 并给出明确提示。

### Plan

1. 在 `internal/tui` 增加 `Live` 入口的 metadata readiness 检查：先尝试按 stale/missing 规则刷新 metadata，再在必要时强制 refresh，并在成功后重载 contract mappings 与 trade_time segments。
2. 当 metadata 仍不可用时，阻止进入 `Live market data`，改为弹出明确错误提示，而不是进入空白 Live 界面。
3. 为 metadata 门禁与刷新兜底补充测试。
4. 同步更新 release/README 文档，写明首次进入 Live 可能需要联网刷新 metadata。
5. 运行 `go test ./internal/tui ./internal/metadata` 与 `go test ./...`。

### Approval

Approved by user in-thread on 2026-04-09 ("确认，改完之后同步将发布命令也写给我").

### Validation

- `gofmt -w internal/tui/metadata_gate.go internal/tui/metadata_gate_test.go internal/tui/view_main.go internal/tui/view_main_test.go` passed.
- `go test ./internal/tui ./internal/metadata` passed.
- `go test ./...` passed.

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
