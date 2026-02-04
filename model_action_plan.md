# Model Action Plan

## Task

执行 `plan.md` 的实时链路改造（JSON-RPC + Router + TUI Poll + Section Diff），先完成可运行最小闭环，再扩展 worker 通路。

## Execution Scope (Batch-1)

- 先落地最小可联调链路：`live_md -> router -> tui(market section)`。
- 本批不做完整 py_worker 业务计算；仅预留接口与 TODO 占位。
- 严格遵守：排序仅 UI 本地；router 只维护共享 `focus_symbol`。

## Checklist

- [x] 新增 `internal/ipc/`：length-prefix 编解码 + JSON-RPC 基础类型/错误。
- [x] 新增 `internal/router/`：内存快照缓存、seq 管理、focus_symbol 状态。
- [x] 新增 router server（localhost TCP）：实现 `market.snapshot`、`router.get_view_snapshot`、`ui.set_focus_symbol`、`router.get_ui_state`。
- [x] 修改 `cmd/starsling/main.go`：启动内嵌 router（goroutine 生命周期管理）。
- [x] 修改 `internal/live/live_md.py`：输出/推送 `MarketSnapshot`（含 NaN/Inf->null、datetime ISO8601+08:00）。
- [x] 修改 `internal/live/process.go`：启动 live_md 时注入 router 地址参数。
- [x] 修改 `internal/tui/`：500ms poll `router.get_view_snapshot`，仅更新左上 market section（原位刷新）。
- [x] 修改 `internal/tui/`：本地排序（默认 `volume desc`，稳定次键 `ctp_contract`）。
- [x] 在 `py_worker` 相关位置加入 TODO 占位（不实现业务计算逻辑）。
- [x] 补齐单测（至少：ipc/router/tui section diff 核心路径）并通过 `go test ./...`。
- [ ] 完成本批后，等待你 code review，再执行 `git add/commit`。

## Need User Confirmation

1. Batch-1 是否按“最小闭环优先（market-only 先打通）”执行？  
2. Batch-1 完成后，再进入 Batch-2（curve/options/unusual/log 与 worker 拉取-推送）的实现，是否确认？
