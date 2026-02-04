# Model Action Plan (Hotfix + Batch-2 Refactor)

## Task

修复进入 `Live market data` 后进程退出（`live md exited: exit status 1`）导致界面卡住的问题，并将期权 Greeks 计算从 `internal/live/live_md.py` 中解耦为独立 Python worker（从 router 拉取市场快照 -> 计算 -> 回推 router）。

## Scope

- [ ] 修复 live-md 启动即退出/卡住问题（优先级最高）。
- [x] `live_md.py` 仅负责 CTP 行情接收、清洗、`market.snapshot` 推送；不再做 options 计算。
- [x] 补齐并对齐 router 侧职责（按 `plan.md` 的 JSON-RPC + latest-only + section seq 规范）。
- [ ] 新增独立 options worker：
  - [x] 从 router 拉取 `router.get_latest_market`（latest-only，不追旧帧）。
  - [x] 基于 `ref_only/option_calc.py` 公式计算 `iv/delta/gamma/theta/vega`。
  - [x] 将 `options.snapshot` 回推到 router。
- [x] Go 侧启动/停止 worker，并把 worker 异常写入 runtime log（不阻塞主 UI）。

## Plan (Checklist)

- [x] **Step 1: Root-cause + freeze hotfix**
  - [x] 复现并定位退出原因（优先检查：`py_vollib` 导入失败、空 instruments 被拒、异常未落日志）。
  - [x] 修正 live-md 退出路径：错误可见、UI 不假死、返回主界面可操作。

- [x] **Step 2: live_md 轻量化**
  - [x] 删除 `live_md.py` 中期权 Greeks 计算与 `options.snapshot` 推送逻辑。
  - [x] 保留并稳定 `market_snapshot` 推送节奏（500ms latest-only）。
  - [x] 保持现有 metadata/字段清洗行为不回归。

- [x] **Step 3: Router 对齐与补全（按 plan.md）**
  - [x] 校验并补全 JSON-RPC 方法契约：
    - [x] `market.snapshot`（notification）
    - [x] `options.snapshot`（notification）
    - [x] `router.get_latest_market`（支持 `min_seq` + `unchanged`）
    - [x] `router.get_ui_state`
    - [x] `router.get_view_snapshot`
    - [x] `ui.set_focus_symbol`
  - [x] 确保 router 为 `market/options` 独立维护 `seq`（递增）和 `stale` 判定（latest-only）。
  - [x] 确保 `get_view_snapshot` 中 options 返回按 `focus_symbol` 过滤（大小写不敏感）。
  - [x] 补齐 router 异常可观测性：decode/invalid params/method not found 的结构化日志。
  - [x] 保持协议为 length-prefix + JSON-RPC 2.0，不引入消息队列。

- [x] **Step 4: options worker 实现**
  - [x] 新建 `internal/live/options_worker.py`（名称可微调），实现：
    - [x] 拉取 market snapshot（JSON-RPC request）
    - [x] 解析 rows -> pandas DataFrame
    - [x] 计算 options Greeks（`product_class == "2"`）
    - [x] 推送 `options.snapshot`（JSON-RPC notification）
  - [x] 规则：
    - [x] `risk_free_rate` 暂定 `0.01`（后续在 main UI 配置入口暴露并持久化）。
    - [x] symbol 严格匹配但不区分大小写（与现有筛选策略一致）。

- [x] **Step 5: Go supervisor 接入 worker**
  - [x] live 模块新增 options worker 子进程生命周期管理（start/stop + health log）。
  - [x] 若 worker 异常退出：仅告警，不拖垮 market 主链路。
  - [x] ESC/返回上级交互不受影响。

- [x] **Step 6: Test**
  - [x] Router 单测补充（方法分发、min_seq/unchanged、focus_symbol 过滤、seq/stale 行为）。
  - [x] `go test ./...`
  - [x] `python -m py_compile internal/live/live_md.py internal/live/options_worker.py`
  - [ ] 手工 smoke：
    - [ ] 进入 live 页面不再 freeze
    - [ ] log 可见 live-md 与 worker 启停
    - [ ] options section 有数据更新（在有期权行情时）

- [ ] **Step 7: Review Gate**
  - [ ] 完成后先请你做 code review。
  - [ ] review 通过后再 `git add/commit`（Conventional Commit）。

## Out-of-scope (this batch)

- [ ] 不做右中图形化 IV 曲线 + Volume 柱图精修（本批仅保证数据链路正确）。
- [ ] 不新增 main UI 通用配置入口（仅记录后续计划：暴露 risk-free rate 等参数）。

## Risks / Notes

- [ ] 由于用户本地 Python runtime 依赖可能不完整，worker 依赖错误需要明确日志并降级。
- [ ] 若 metadata 缺失导致 options 字段不足，worker 应容错并继续运行（输出空 rows 而非崩溃）。
- [ ] 仅本批完成 options 相关 router 链路；curve/unusual/log 的 worker 实际计算保持后续迭代。

## Router-related Files (target)

- [ ] `internal/router/server.go`
- [ ] `internal/router/state.go`
- [ ] `internal/router/types.go`
- [ ] `internal/router/server_test.go`
- [ ] `internal/ipc/jsonrpc.go`（仅在需要补充协议健壮性时修改）

---

## Batch-3 Plan (Market Filter UX + Non-option Market View)

### New Task

按最新需求完善 live 左上筛选/排序交互与行情展示：

1. 筛选窗口内所有输入匹配统一大小写不敏感。
2. `exchange/product_class/symbol` 均支持多选（`,` 分割，OR 语义）。
3. 左上 market 区默认只展示非期权行情（排除 `product_class == "2"`），但不影响 options worker 的全量输入。
4. 筛选窗口 `sort by / order` 改为选择组件：
   - `sort by` = 当前 market 可用 numeric 列集合
   - `order` = `desc/asc`

### Execution Checklist

- [x] **Step A: Router market view split（不破坏 worker）**
  - [x] 保留 `router.get_latest_market` 返回 full market rows（给 options worker）。
  - [x] 在 `router.get_view_snapshot` 的 `market` 返回中应用 UI 视图过滤：默认排除 `product_class == "2"`。
  - [x] 补充单测覆盖：worker 仍能拿到 full market；UI market 只拿到非期权。

- [x] **Step B: 筛选条件升级为 CSV 多选 + case-insensitive**
  - [x] `exchange/product_class/symbol` 输入按 `,` 拆分 token，去空格去重。
  - [x] 匹配规则统一 `case-insensitive exact match`（symbol 保持严格匹配，不做前缀/模糊）。
  - [x] 若某项为空，视为该维度不过滤。

- [x] **Step C: sort by/order 改为下拉选择**
  - [x] 使用 `tview.DropDown` 替代输入框。
  - [x] `sort by` 选项从当前 market rows 动态抽取 numeric 列（无可用列时 fallback `volume`）。
  - [x] `order` 固定 `desc`、`asc`。
  - [x] 保持 ESC 返回上级，不影响现有键盘导航。

- [x] **Step D: 测试**
  - [x] `go test ./internal/router ./internal/tui`
  - [x] 补充/更新 `internal/tui/app_test.go`（CSV 多选 + 大小写 + strict symbol）。
  - [x] 补充/更新 `internal/router/*_test.go`（market view 非期权过滤 + latest_market 全量）。

- [ ] **Step E: Review Gate**
  - [ ] 完成后先请你 code review。
  - [ ] review 通过后再 `git add/commit`。

### Target Files (Batch-3)

- [x] `internal/router/state.go`
- [x] `internal/router/state_test.go`
- [x] `internal/router/server_test.go`
- [x] `internal/tui/app.go`
- [x] `internal/tui/app_test.go`
