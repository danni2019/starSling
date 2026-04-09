# Maintainer Worklog

本文件用于维护者/agent 的当前任务计划、审批状态、验证记录和必要的 postmortem。

规则：

- 保持精简，只记录当前活跃任务与当前任务相关的纠偏信息。
- 稳定的公开文档放到 `docs/project/` 或 `docs/release/`，不要继续堆在这里。
- 历史细节依赖 git history，不把本文件演化成长期变更日志。

## Current Task

### Name

Extend prerelease publishing to linux-x86_64

### Request Summary

在现有 macOS prerelease 已跑通的基础上，扩展 GitHub release 产物到 `linux-x86_64`，并同步更新发布配置、公开文档和验证路径。

### Plan

1. 更新 `.goreleaser.yaml`，增加 `linux/amd64` 构建与归档资产。
2. 新增 `docs/release/linux-prerelease.md`，明确 Linux prerelease 的验证范围和限制。
3. 更新 `README.md`、`docs/release/public-readiness.md`、`docs/project/roadmap.md` 中仍然偏 macOS-only 的发布表述。
4. 在本地运行 `goreleaser check` 与 `goreleaser release --snapshot --clean`，核对 Linux 归档名称与内容。
5. 运行 `go test ./...`，并把验证结果回写到本文件。

### Approval

Approved by user in-thread on 2026-04-09 ("可以，继续linux prerelease").

### Validation

- `goreleaser check` passed.
- `go test ./...` passed.
- `goreleaser release --snapshot --clean` passed when rerun outside the sandbox.
- Verified Linux snapshot archive:
  - `dist/starsling_0.1.0-alpha.1-SNAPSHOT-ce19ebc_linux_amd64.tar.gz`
  - includes `starsling`, `LICENSE`, `README.md`, `CONTRIBUTING.md`, `SECURITY.md`, config files, Python metadata, and `scripts/bootstrap_python.sh`
- Updated user-facing docs:
  - `README.md`
  - `docs/project/roadmap.md`
  - `docs/release/public-readiness.md`
  - `docs/release/linux-prerelease.md`

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
