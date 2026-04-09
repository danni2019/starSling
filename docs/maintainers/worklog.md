# Maintainer Worklog

本文件用于维护者/agent 的当前任务计划、审批状态、验证记录和必要的 postmortem。

规则：

- 保持精简，只记录当前活跃任务与当前任务相关的纠偏信息。
- 稳定的公开文档放到 `docs/project/` 或 `docs/release/`，不要继续堆在这里。
- 历史细节依赖 git history，不把本文件演化成长期变更日志。

## Current Task

### Name

Stabilize router log round-trip test in CI

### Request Summary

修复 GitHub Actions 上 `TestRouterMarketSnapshotRoundTrip` 的时序性失败。根因是测试假设 `Notify(log.append)` 返回后服务端已经完成处理，但实际通知和后续请求走的是不同连接，CI 上会出现 `get_view_snapshot` 先于 `log.append` 落入 state 的竞态。

### Plan

1. 调整 `internal/router/server_test.go`，把 `log.append` 后对 `get_view_snapshot` 中日志可见性的断言改成带超时的轮询，而不是单次立即断言。
2. 仅修测试，不改变生产中的 router / IPC 语义。
3. 运行 `go test ./internal/router -run TestRouterMarketSnapshotRoundTrip -count=50` 与 `go test ./...`。

### Approval

Approved by user in-thread on 2026-04-09 ("ok").

### Validation

- `gofmt -w internal/router/server_test.go` passed.
- `go test ./internal/router -run TestRouterMarketSnapshotRoundTrip -count=50` passed.
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
