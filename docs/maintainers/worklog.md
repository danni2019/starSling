# Maintainer Worklog

本文件用于维护者/agent 的当前任务计划、审批状态、验证记录和必要的 postmortem。

规则：

- 保持精简，只记录当前活跃任务与当前任务相关的纠偏信息。
- 稳定的公开文档放到 `docs/project/` 或 `docs/release/`，不要继续堆在这里。
- 历史细节依赖 git history，不把本文件演化成长期变更日志。

## Current Task

### Name

Finalize public-facing docs: README, roadmap, and legacy tracking-file cleanup

### Request Summary

按当前项目设计与公开仓库状态，重写 README、更新 roadmap，并完成旧根目录追踪文件迁移后的文档收尾。

### Plan

1. 重写 `README.md`，使其与当前公开仓库定位、当前 UI 设计、发布状态和运行方式一致。
2. 将 `docs/misc/` 下的两张项目截图嵌入 `README.md`。
3. 更新 `docs/project/roadmap.md`，按当前项目结构与公开阶段目标重写路线图。
4. 保持 `docs/maintainers/worklog.md`、`docs/project/roadmap.md`、`docs/release/public-readiness.md` 为新文档入口，并确认旧根目录追踪文件继续作为删除项保留。
5. 做文档级验证：
   - 检查旧追踪文件引用是否还残留；
   - 检查 README 图片路径和 docs 链接是否正确。

### Approval

Approved by user in-thread on 2026-04-08 ("确认").

### Validation

- `rg -n "model_action_plan\\.md|Roadmap\\.md|plan\\.md" .` returned no matches.
- Verified `README.md` references:
  - `docs/misc/screenshot_starSling.png`
  - `docs/misc/screenshot_main_panel.png`
  - `docs/project/roadmap.md`
  - `docs/release/public-readiness.md`
  - `docs/release/macos-prerelease.md`
- Rewrote `README.md` for the current public-repo state and current default UI layout.
- Updated `docs/project/roadmap.md` to reflect the public-stage roadmap.
- Updated `docs/release/public-readiness.md` to reflect that the repository is already public.
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
