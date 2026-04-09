# Maintainer Worklog

本文件用于维护者/agent 的当前任务计划、审批状态、验证记录和必要的 postmortem。

规则：

- 保持精简，只记录当前活跃任务与当前任务相关的纠偏信息。
- 稳定的公开文档放到 `docs/project/` 或 `docs/release/`，不要继续堆在这里。
- 历史细节依赖 git history，不把本文件演化成长期变更日志。

## Current Task

### Name

Stream bootstrap output in Setup Python runtime

### Request Summary

修复 `Setup Python runtime` 页面在 bootstrap 运行期间 `Output` 区域无实时内容的问题，将当前“一次性收集后再显示”改成流式输出。

### Plan

1. 重构 `internal/runtime/bootstrap.go`，提供带流式回调的 bootstrap 执行路径，同时保留最终完整输出。
2. 更新 `internal/tui/view_setup.go`，在 bootstrap 运行过程中实时追加输出到 `Output` 文本框。
3. 为 runtime 输出收集和 Setup 页面流式更新补充测试。
4. 运行 `go test ./internal/runtime ./internal/tui` 与 `go test ./...`。

### Approval

Approved by user in-thread on 2026-04-09 ("对，生成实时输出。").

### Validation

- `gofmt -w internal/runtime/bootstrap.go internal/runtime/bootstrap_test.go internal/tui/app.go internal/tui/bootstrap_flow.go internal/tui/view_setup.go internal/tui/view_setup_test.go` passed.
- `go test ./internal/runtime ./internal/tui` passed.
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
