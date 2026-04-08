# Maintainer Worklog

本文件用于维护者/agent 的当前任务计划、审批状态、验证记录和必要的 postmortem。

规则：

- 保持精简，只记录当前活跃任务与当前任务相关的纠偏信息。
- 稳定的公开文档放到 `docs/project/` 或 `docs/release/`，不要继续堆在这里。
- 历史细节依赖 git history，不把本文件演化成长期变更日志。

## Current Task

### Name

Remove default front preset and finish local public-readiness gates

### Request Summary

将发布包中的 `front` 默认值改为完全由用户自行配置，并继续推进所有本地侧的公开前门禁，直到仓库达到 ready-for-public 状态。

### Plan

1. 将默认配置与样例配置中的 `live-md.host` 改为空字符串，`live-md.port` 保持未配置态，并同步调整 `doctor` 与相关测试。
2. 更新 README / prerelease 文档，使“用户必须自行配置 Host/Port”与新的空默认值一致。
3. 继续完成本地公开前门禁中仍可在本地收口的项：
   - 复查当前工作树和发布文档一致性；
   - 确认 `doctor`、`goreleaser` dry-run、归档契约仍全部通过；
   - 将已完成门禁写回 `docs/release/public-readiness.md`。
4. 对 GitHub public 切换后才可见/才需要复核的项保留明确提醒，不在 private 状态下假设完成。
5. 运行 `go test ./...` 和 `go run ./cmd/starsling doctor` 收尾验证。

### Approval

Approved by user in-thread on 2026-04-08 ("确认front相关实现").

### Validation

- `go test ./...` passed.
- `go run ./cmd/starsling doctor` returned `7 ok, 0 warn, 0 fail`.
- `goreleaser check` passed.
- `goreleaser release --snapshot --clean` passed.
- Extracted `dist/starsling_0.0.0-SNAPSHOT-a0071aa_darwin_arm64.tar.gz` and ran `./starsling doctor`; result was `6 ok, 1 warn, 0 fail` with only the expected bundled-python warning.

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
