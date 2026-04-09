# Maintainer Worklog

本文件用于维护者/agent 的当前任务计划、审批状态、验证记录和必要的 postmortem。

规则：

- 保持精简，只记录当前活跃任务与当前任务相关的纠偏信息。
- 稳定的公开文档放到 `docs/project/` 或 `docs/release/`，不要继续堆在这里。
- 历史细节依赖 git history，不把本文件演化成长期变更日志。

## Current Task

### Name

Improve bootstrap progress visibility on Linux

### Request Summary

修复 Linux 上 `Setup Python runtime` 首次运行时“看起来像没反应”的体验问题。根因是 bootstrap 脚本文案误导且下载阶段基本静默；目标是让长耗时步骤持续输出真实进度，确保用户能看到 runtime 正在配置。

### Plan

1. 调整 `scripts/bootstrap_python.sh` 的长耗时步骤输出：把误导性的 `Press Enter...` 改成真实状态日志，并让 Python 下载阶段显示持续进度。
2. 在 `internal/tui/view_setup.go` 里规范化 `\r` 进度输出，确保 `curl` 进度条在 `Output` 面板里可见，而不是看起来空白。
3. 为 Setup 输出规范化补充测试。
4. 运行 `go test ./internal/runtime ./internal/tui` 与 `go test ./...`。

### Approval

Approved by user in-thread on 2026-04-09 ("确认。改好后给我发布命令").

### Validation

- `gofmt -w internal/tui/app.go internal/tui/view_setup.go internal/tui/view_setup_test.go` passed.
- `bash -n scripts/bootstrap_python.sh` passed.
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
