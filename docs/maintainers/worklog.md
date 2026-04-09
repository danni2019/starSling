# Maintainer Worklog

本文件用于维护者/agent 的当前任务计划、审批状态、验证记录和必要的 postmortem。

规则：

- 保持精简，只记录当前活跃任务与当前任务相关的纠偏信息。
- 稳定的公开文档放到 `docs/project/` 或 `docs/release/`，不要继续堆在这里。
- 历史细节依赖 git history，不把本文件演化成长期变更日志。

## Current Task

### Name

Gate Live entry on explicit metadata readiness flow

### Request Summary

把 metadata 准备从 runtime setup 中彻底分离，并把它变成进入 `Live market data` 前的显式门禁。目标是：应用启动时继续做一次 metadata 预热；每次进入 Live 前都检查 `missing/stale` 状态；如需刷新则弹出独立的 metadata 准备流程，完成后再自动回到 Live 链路。

### Plan

1. 在 `internal/tui` 增加独立的 metadata 准备屏幕/流程，和 Python runtime setup 分开。
2. 在 `metadata_gate` 中增加 `missing/stale` 状态检查；进入 `Live market data` 前先检查状态，缺失或过期时不直接刷新，而是提示进入 metadata 准备流程。
3. metadata 准备完成后自动回到 `Live` 链路；启动时继续保留一次 metadata 预热，形成双保险。
4. 更新相关测试和 README/release 文档。
5. 运行 `go test ./internal/tui ./internal/metadata` 与 `go test ./...`。

### Approval

Approved by user in-thread on 2026-04-09 ("确认").

### Validation

- `gofmt -w internal/tui/app.go internal/tui/metadata_flow.go internal/tui/metadata_gate.go internal/tui/metadata_gate_test.go internal/tui/view_main.go internal/tui/view_main_test.go internal/tui/view_metadata.go internal/tui/view_metadata_test.go internal/tui/view_setup.go internal/tui/view_setup_test.go` passed.
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
