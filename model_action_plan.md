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

---

## Batch-4 Plan (Right Panels Data Correctness + Worker Split)

### Goal

按 `plan.md` 最新设计推进，解决“右中区域无有效 IV+Volume 图”并完成右侧三块区域的真实数据链路：

1. 右上：`VIX + forward curve`（option_worker）。
2. 右中：`IV curve + Volume bar`（option_worker，IV 按 VWAP 价格计算）。
3. 右下：`Unusual option volume`（unusual_worker，按 turnover 变化监控）。

### Scope

- [ ] 保持现有 `market + options` 行为不回归（左上只显示非期权继续保持）。
- [ ] 验证并修复右中 IV+Volume 当前“无有效显示”的问题（先做可观测性+有效性校验）。
- [ ] 按职责拆分 worker：
  - [ ] `option_worker` 负责 `curve.snapshot` + `options.snapshot`
  - [ ] `unusual_worker` 负责 `unusual.snapshot` + `log.append`
- [ ] Router 接收并缓存 `curve/unusual/log`，`get_view_snapshot` 返回完整右侧视图数据。
- [ ] TUI 右侧三区按你定义结构渲染，且保持 section diff 刷新稳定。

### Execution Checklist

- [x] **Step 1: Data-validity baseline（先定位右中无效显示）**
  - [x] 在 `option_worker` 增加计算过程诊断日志（行数、有效 strike/iv/volume 数）。
  - [x] 明确无效数据原因分类：缺字段 / 价格<=0 / tte<=0 / vwap 无法计算 / py_vollib 失败。
  - [x] 在 TUI 右中增加“空数据原因”提示文本（不再只显示空白）。

- [x] **Step 2: option_worker 实现右上/右中（Python）**
  - [x] 读取 `router.get_latest_market` + `router.get_ui_state`，latest-only，不追旧帧。
  - [x] 右中 IV curve + Volume bar：
    - [x] X 轴 = strike 升序
    - [x] Volume = 当前 `volume`
    - [x] IV 价格输入改为 VWAP（由 `bid1/ask1/bid_vol1/ask_vol1` 计算）
  - [x] 右上 VIX + forward：
    - [x] forward = 各期货合约 `last`
    - [x] VIX(underlying) = 该 underlying 下 `|delta| < 0.25` 期权 IV 均值
  - [x] 推送：`options.snapshot`（右中）+ `curve.snapshot`（右上）
  - [ ] 保留 TODO：你后续补充更细口径（如过滤细则、到期日聚合规则）。

- [x] **Step 3: unusual_worker 实现右下（Python）**
  - [x] 使用上一帧缓存计算差分（必须 stateful）：
    - [x] `turnover_chg = turnover(cur) - turnover(prev)`
    - [x] `turnover_ratio = turnover(cur)/turnover(prev) - 1`
  - [x] 当两者均超过阈值时输出异常记录，否则忽略。
  - [x] 推送 `unusual.snapshot`（最新在最上）+ `log.append`（异常/状态日志）。

- [x] **Step 4: Router 扩展（Go）**
  - [x] `internal/router/types.go` 新增 `CurveSnapshot`、`UnusualSnapshot`、`LogSnapshot`。
  - [x] `internal/router/state.go` 增加 `curve/unusual/log` 缓存、seq、stale、log ring buffer。
  - [x] `internal/router/server.go` 新增 notification 分发：`curve.snapshot`、`unusual.snapshot`、`log.append`。
  - [x] `router.get_view_snapshot` 返回右侧三块所需完整数据。

- [x] **Step 5: TUI 接入（Go）**
  - [x] 右上（VIX+forward）接 `curve.snapshot`，禁用 Enter 二级窗口。
  - [x] 右中（IV+Volume）接 `options.snapshot`，禁用 Enter 二级窗口。
  - [x] 右下（unusual）接 `unusual.snapshot`，支持 Enter 二级窗口设置阈值。
  - [x] 阈值设置通过 router UI state 回传 worker（新增 UI state 字段/方法）。
  - [x] 保持 section diff：仅变更 section 重绘，不闪烁、不追加打印。

- [x] **Step 6: 重连、stale、测试**
  - [x] worker/router 调用失败指数退避（0.5s -> 1s -> 2s -> 4s -> 5s）。
  - [x] request timeout=2s；连续失败日志升级但 UI 不阻塞。
  - [x] `go test ./internal/router ./internal/tui`
  - [x] `go test ./...`
  - [x] `python -m py_compile internal/live/options_worker.py`（+ unusual_worker）
  - [ ] 手工 smoke：三块右侧区域实时刷新、断线恢复、stale 可见。

- [ ] **Step 7: Review Gate**
  - [ ] 完成后先请你 code review。
  - [ ] review 通过后再 `git add/commit`。

### Target Files (Batch-4)

- [x] `internal/live/options_worker.py`
- [x] `internal/live/unusual_worker.py`（新）
- [x] `internal/router/types.go`
- [x] `internal/router/state.go`
- [x] `internal/router/server.go`
- [x] `internal/router/state_test.go`
- [x] `internal/router/server_test.go`
- [x] `internal/tui/app.go`
- [x] `internal/tui/view_live.go`
- [x] `internal/tui/app_test.go`

### Need User Confirmation (before code)

- [x] unusual 阈值默认值：
  - [x] `turnover_chg_threshold = 100000`
  - [x] `turnover_ratio_threshold = 5%`
- [x] 右上 X 轴“期货合约升序”按 `ctp_contract` 字典序实现，已确认。

---

## Batch-4.1 Patch Plan (Right-Mid Full Option Chain Table)

### Problem (confirmed)

- 当前右中区域只显示“部分期权链”，原因是渲染逻辑中存在硬限制：
  - points 上限 24
  - 明细行上限 8

### Goal

- 右中区域展示完整期权链数据表（不截断）。
- 保留 IV curve + Volume bar 的顶部概览信息。
- 支持在右中区域聚焦后滚动查看完整行（不影响其它 section diff 刷新）。
- 左上筛选新增 `contract` 条件，规则与 `symbol` 一致：
  - 大小写不敏感
  - 严格匹配（不做前缀/模糊）
  - `,` 分割多选（字段内 OR）

### Execution Checklist

- [x] 移除右中 options 渲染中的硬截断（24/8 限制）。
- [x] 将右中组件从“静态文本片段”调整为“完整可滚动表格视图”或“完整文本+滚动”，确保全量数据可访问。
- [x] 保持按 strike 升序展示。
- [x] 左上筛选弹窗新增 `contract` 输入项。
- [x] 筛选逻辑扩展 `contract` 字段（与 `symbol` 同规则：case-insensitive + strict + CSV 多选）。
- [x] 更新筛选相关单测（含 contract 单值/多值/大小写场景）。
- [x] 增补/更新 `internal/tui/app_test.go`：验证不再截断（输入 >24 行时仍保留全量）。
- [x] 回归测试：
  - [x] `go test ./internal/tui`
  - [x] `go test ./...`

### Target Files (Batch-4.1)

- [x] `internal/tui/app.go`
- [x] `internal/tui/view_live.go`（若切到 table 组件）
- [x] `internal/tui/app_test.go`

---

## Batch-4.2 Plan (Right-Mid Enter Filter: Delta Abs Upper Bound)

### Goal

在 live 页面右中区域（options panel）聚焦并按 Enter 时，弹出筛选窗口；支持按 `|delta|` 上限过滤展示的期权链数据。

示例：输入 `0.25` => 仅展示 `delta in [-0.25, 0.25]` 的合约。

### Execution Checklist

- [ ] **Step 1: 交互接入**
  - [x] 在 `handleLiveKeys` 中，当焦点位于右中 `liveOpts` 时，`Enter` 打开新的 options filter modal。
  - [x] 不改动现有左上 market filter 与右下 unusual threshold 的 Enter 行为。

- [ ] **Step 2: 右中筛选窗口**
  - [x] 新增 `openOptionsFilter()`，表单字段：`Delta |abs| <=`（单值输入）。
  - [x] 按钮：`Apply / Reset / Cancel`。
  - [x] `Apply`：仅允许正数（`>0`）；非法输入自动重置为默认值 `0.25`，并写入 runtime log 提示。
  - [x] `Reset`：清空 delta 过滤（恢复全量显示）。

- [ ] **Step 3: 渲染过滤逻辑**
  - [x] 在 options 面板渲染前应用过滤（不修改 router 原始数据，仅 UI 视图过滤）。
  - [x] 过滤规则：先找出满足 `abs(delta) <= threshold` 的合约集合（包含边界），取这些合约的 `strike` 最小/最大值作为边界。
  - [x] 展示规则：展示该 `strike` 边界区间内的所有期权合约（包含 delta 缺失行）。
  - [x] 在右中面板标题区域增加过滤状态提示（如 `Delta|abs|<=0.25`），便于用户感知。

- [ ] **Step 4: 测试**
  - [x] 更新/新增 `internal/tui/app_test.go`：
    - [x] `abs(delta)` 过滤边界（含等号）；
    - [x] `strike` 边界内保留 delta 缺失行。
    - [x] 非法阈值输入回退默认值（0.25）。
  - [x] 执行：`go test ./internal/tui`（必要时 `go test ./...`）。

- [ ] **Step 5: Review Gate**
  - [ ] 完成后请你先做 code review。
  - [ ] review 通过后再 `git add/commit`。

### Target Files (Batch-4.2)

- [x] `internal/tui/app.go`
- [x] `internal/tui/app_test.go`

---

## Batch-4.3 Plan (Right-Mid T-Quote + Right-Bottom TTE + UI Readability)

### Goal

按你的新要求优化 UI：

1. 右中区域移除“作图/曲线”展示逻辑；
2. 右中改为期权 T 型报价（`strike` 在中间，左 Call / 右 Put，字段顺序均为：`last, iv, delta, gamma, theta, vega, vol, tte`）；
3. 右下新增 `tte` 列；
4. 全局可读性提升：列间距扩大约 50%，并处理“字体放大”诉求。

### Execution Checklist

- [ ] **Step 1: 右中去图化 + T 型结构重绘**
  - [x] 删除右中面板中的 Call/Put IV/Volume 字符画线相关渲染。
  - [x] 改为按 `strike` 聚合的 T 型行渲染：
    - [x] 左侧 `CALL`：`last, iv, delta, gamma, theta, vega, vol, tte`
    - [x] 中间：`strike`
    - [x] 右侧 `PUT`：`last, iv, delta, gamma, theta, vega, vol, tte`
  - [x] 合约对齐规则：同一 `strike` 下 call/put 分列展示；缺失侧显示 `-`。
  - [x] 保留你已确认的右中 Enter 筛选（delta 过滤）逻辑，并作用在新 T 型表输出。

- [ ] **Step 2: 右下字段扩展**
  - [x] 右下 `Unusual option volume` 表头新增 `TTE`。
  - [x] `convertUnusualTrades` 映射中补充 `tte` 字段展示（缺失时 `-`）。

- [ ] **Step 3: 可读性（列间距 + 字号诉求）**
  - [x] 将右中/右下文本列宽与间距扩大约 50%（通过 `fmt.Sprintf` 宽度与表格列内容 padding 实现）。
  - [x] 说明：终端 TUI 无法在程序内直接控制“字体点号（+2）”；可在应用内通过“更大列宽/更疏排版/更高对比”实现等效可读性提升。

- [ ] **Step 4: 测试**
  - [x] 更新 `internal/tui/app_test.go`：
    - [x] 右中输出不再包含旧图形行；
    - [x] 右中包含 T 型表头与 call/put 两侧字段；
    - [x] 右下包含 `TTE` 列并能显示值。
  - [x] 执行：`go test ./internal/tui`，必要时 `go test ./...`。

- [ ] **Step 5: Review Gate**
  - [ ] 完成后先请你 code review。
  - [ ] review 通过后再 `git add/commit`。

### Target Files (Batch-4.3)

- [x] `internal/tui/app.go`
- [x] `internal/tui/view_live.go`
- [x] `internal/tui/app_test.go`

---

## Batch-4.4 Plan (Right-Mid Columns Simplification)

### Goal

按最新要求，右中 T 型报价中移除 `gamma/theta/vega` 三列，仅保留：

- 左侧（CALL）：`tte, vol, delta, iv, last`
- 中间：`strike`
- 右侧（PUT）：`last, iv, delta, vol, tte`

### Execution Checklist

- [ ] **Step 1: 右中表头与行渲染**
  - [x] 更新 `renderOptionsPanel` 的左右表头，删除 `gamma/theta/vega`。
  - [x] 更新每行字段映射与列宽，保持 `strike` 中线对称布局。

- [ ] **Step 2: 回归验证**
  - [x] 确认 delta 筛选逻辑仍生效（仅改显示列，不改筛选规则）。
  - [x] 确认右中滚动、对齐、旧数据刷新行为不回归。

- [ ] **Step 3: 测试**
  - [x] 更新 `internal/tui/app_test.go`：断言右中不再出现 `GAMMA/THETA/VEGA` 表头。
  - [x] 执行：`go test ./internal/tui`（必要时 `go test ./...`）。

- [ ] **Step 4: Review Gate**
  - [ ] 完成后先请你 code review。
  - [ ] review 通过后再 `git add/commit`。

### Target Files (Batch-4.4)

- [x] `internal/tui/app.go`
- [x] `internal/tui/app_test.go`

---

## Batch-4.5 Plan (Layout Ratio Recheck + Column Spacing Halved)

### Goal

1. 确保 live 页面左右区域为 `65:35`；
2. 将当前所有表/文本列间距统一收缩到当前约 `1/2`。

### Execution Checklist

- [ ] **Step 1: 比例确认与固定**
  - [x] 复核 live root flex 比例是否为 `left=65 / right=35`。
  - [x] 若存在其他覆盖布局路径，同步改为 65:35（保持唯一来源）。

- [ ] **Step 2: 列间距统一缩减**
  - [x] 左上/右下 `tview.Table` 单元格 padding 从当前值减半。
  - [x] 右中 T-quote `fmt` 列宽与列间 gap 减半。
  - [x] 右上 curve 文本列宽同步减半，保持风格一致。

- [ ] **Step 3: 测试**
  - [x] 执行：`go test ./internal/tui`（必要时 `go test ./...`）。
  - [ ] 手工 smoke：确认 65:35 生效、各区列间距明显减半、无错位。

- [ ] **Step 4: Review Gate**
  - [ ] 完成后先请你 code review。
  - [ ] review 通过后再 `git add/commit`。

### Target Files (Batch-4.5)

- [ ] `internal/tui/view_live.go`
- [ ] `internal/tui/app.go`
- [ ] `internal/tui/app_test.go`（如需）

---

## Batch-4.6 Plan (Width Unification + Delta-by-Strike Filter + Curve OI/Volume)

### Goal

1. 所有列宽以当前右中列宽为基准统一；
2. 修正右中 delta 筛选逻辑为“按命中 delta 的 strike 集合过滤”；
3. 右上新增 `OI` 与 `Volume` 两列。

### Execution Checklist

- [ ] **Step 1: 列宽统一（以右中为准）**
  - [x] 提取统一列宽常量（以右中当前列宽为基准）。
  - [x] 右中 T-quote 使用该统一列宽。
  - [x] 右上 curve 表头/行渲染改为同一列宽。
  - [x] 左上/右下 table 的 cell padding 按同一列宽最小宽度格式化（不截断长文本）。

- [ ] **Step 2: 修正 delta 筛选逻辑**
  - [x] 将当前“delta 命中后取 strike 区间[min,max]”逻辑改为“strike 集合”逻辑：
    - [x] 先找出 `abs(delta) <= threshold` 的行对应的 strike 集合；
    - [x] 仅展示这些 strike 下的全部期权行（即 call/put 都显示）。
  - [x] 更新右中筛选状态文案（显示命中 strike 数而非区间）。

- [ ] **Step 3: 右上新增 OI/Volume**
  - [x] `options_worker.build_curve_snapshot` 输出新增 `open_interest`、`volume` 字段。
  - [x] `renderCurvePanel` 表头与行新增 `OI`、`VOL` 列。

- [ ] **Step 4: 测试**
  - [x] 更新 `internal/tui/app_test.go`：
    - [x] delta 过滤为 strike 集合逻辑（验证同 strike 的 call/put 同时显示）；
    - [x] 不再依赖旧区间逻辑断言。
  - [x] 执行 `go test ./internal/tui`、`go test ./...`。
  - [x] 额外执行 `python3 -m py_compile internal/live/options_worker.py`。

- [ ] **Step 5: Review Gate**
  - [ ] 完成后先请你 code review。
  - [ ] review 通过后再 `git add/commit`。

### Target Files (Batch-4.6)

- [x] `internal/tui/app.go`
- [x] `internal/tui/view_live.go`
- [x] `internal/tui/app_test.go`
- [x] `internal/live/options_worker.py`

---

## Batch-4.7 Plan (Column Alignment Hard Fix)

### Goal

立即修复你指出的两点：

1. 右上区域列头与数据列严格对齐；
2. 所有区域列宽与右中区域统一。

### Execution Checklist

- [ ] **Step 1: 统一列宽基准**
  - [x] 将全局列宽常量统一为 `8`（兼容 `CONTRACT/BID_VOL` 等表头长度，避免右上错位）。
  - [x] 右中 `T-quote` 继续以该常量渲染。

- [ ] **Step 2: 右上对齐修复**
  - [x] 右上表头与每行都使用同一 `formatAlignedColumns(..., unifiedColumnWidth)` 输出。
  - [x] 确保新增 `VOL/OI` 两列和其数据列严格对齐。

- [ ] **Step 3: 全区域统一**
  - [x] 左上/右下 table 的 `padTableCell` 按同一 `unifiedColumnWidth` 处理，避免区域间视觉宽度不一致。

- [ ] **Step 4: 测试**
  - [x] `go test ./internal/tui`
  - [x] `go test ./...`

- [ ] **Step 5: Review Gate**
  - [ ] 完成后先请你 code review。
  - [ ] review 通过后再 `git add/commit`。

### Target Files (Batch-4.7)

- [x] `internal/tui/app.go`
- [x] `internal/tui/view_live.go`

---

## Batch-4.8 Plan (Layout 6:4 + Delta Range Filter)

### Goal

1. Live 页面左右比例从 `65:35` 调整为 `6:4`；
2. 右中 delta 筛选从单阈值改为区间：
   - 规则：`input1 <= abs(delta) <= input2`
   - 默认：`input1=0.25`, `input2=0.5`

### Execution Checklist

- [ ] **Step 1: 布局比例**
  - [x] 将 live root flex 从 `65:35` 改为 `6:4`。

- [ ] **Step 2: 右中筛选窗口升级**
  - [x] `openOptionsFilter()` 改为两个输入框：
    - [x] `Delta |abs| min >=`（默认 0.25）
    - [x] `Delta |abs| max <=`（默认 0.5）
  - [x] 校验：
    - [x] 两个输入都必须 `>0`
    - [x] `min <= max`
    - [x] 非法输入回退默认值并写 log

- [ ] **Step 3: 筛选逻辑升级**
  - [x] 将 strike 过滤逻辑改为区间版：
    - [x] 命中条件：`min <= abs(delta) <= max`
    - [x] 命中 strike 集合下展示 call/put 全部行
  - [x] 右中状态文案显示区间与命中 strike 数。

- [ ] **Step 4: 测试**
  - [x] 更新 `internal/tui/app_test.go`：
    - [x] 区间命中逻辑；
    - [x] `min/max` 校验回退默认值；
    - [x] 旧单阈值测试改造。
  - [x] 执行：`go test ./internal/tui`、`go test ./...`。

- [ ] **Step 5: Review Gate**
  - [ ] 完成后先请你 code review。
  - [ ] review 通过后再 `git add/commit`。

### Target Files (Batch-4.8)

- [x] `internal/tui/view_live.go`
- [x] `internal/tui/app.go`
- [x] `internal/tui/app_test.go`

---

## Batch-4.9 Plan (Unusual TTE Fallback from Dates)

### Goal

修复右下 `TTE` 为空的问题：当 `row["tte"]` 缺失时，按你指定逻辑使用  
`expiry_date - trading_date` 计算天数并展示。

### Execution Checklist

- [ ] **Step 1: unusual worker 增加 TTE 回退计算**
  - [x] 在 `internal/live/unusual_worker.py` 增加日期解析函数（兼容常见日期/时间字符串）。
  - [x] 新增 `compute_tte_days(row)`：
    - [x] 优先使用已有 `row["tte"]`（>=0）。
    - [x] 若缺失，则尝试 `expiry_date - trading_date`。
    - [x] 若 `trading_date` 缺失，回退用 `datetime` 的日期部分。
    - [x] 结果 `<0` 则置为 `0`。
  - [x] `unusual.snapshot` 输出时统一写入计算后的 `tte`。

- [ ] **Step 2: UI链路确认**
  - [x] 保持 `convertUnusualTrades` 读取 `row["tte"]` 显示，不改字段名。
  - [x] 确认无值时仍显示 `-`（仅在 worker无法算出时）。

- [ ] **Step 3: 验证**
  - [x] `python3 -m py_compile internal/live/unusual_worker.py`
  - [x] `go test ./internal/tui`（回归）
  - [x] `go test ./...`（回归）

- [ ] **Step 4: Review Gate**
  - [ ] 完成后先请你 code review。
  - [ ] review 通过后再 `git add/commit`。

### Target Files (Batch-4.9)

- [x] `internal/live/unusual_worker.py`
- [ ] `internal/tui/app.go`（如无需改动则不动）

---

## Batch-4.10 Plan (Options T-Quote: Focus Guard + No-Chain Fallback)

### Goal

修复右中 T-quote 误合并多条期权链的问题：

- 当 focus 为空时，不渲染 T‑quote，提示 “Select a contract”；
- 只有在 focus 可用且过滤到单链时，才按 strike 聚合。

### Execution Checklist

- [ ] **Step 1: UI 侧强制 focus 过滤**
  - [x] 在 `renderOptionsPanel` 进入 strike 聚合前过滤 rows：
    - [x] 仅保留与 `focusSymbol` 匹配的 `ctp_contract/underlying/symbol` 行；
    - [x] 若 focus 为空或过滤后为空，返回提示 `Select a contract`。

- [ ] **Step 2: router 请求携带 focus**
  - [x] `router.get_view_snapshot` 调用传入 `FocusSymbol`（来自 `currentFocusSymbol()`）。
  - [x] 保留 UI 侧过滤作为兜底（避免 focus 未同步时混链）。

- [ ] **Step 3: 测试**
  - [x] `internal/tui/app_test.go`：
    - [x] focus 为空时渲染提示；
    - [x] 多链输入下仅渲染 focus 链。
  - [x] `go test ./internal/tui`、`go test ./...`。

- [ ] **Step 4: Review Gate**
  - [ ] 完成后先请你 code review。
  - [ ] review 通过后再 `git add/commit`。

### Target Files (Batch-4.10)

- [x] `internal/tui/app.go`
- [x] `internal/tui/app_test.go`

---

## Batch-4.11 Plan (Options Worker: product_class Guard)

### Goal

修复 `build_curve_snapshot` 在 `product_class` 缺失时触发 `IndexingError` 的问题，确保 options worker 不会中断。

### Execution Checklist

- [ ] **Step 1: 防护逻辑**
  - [x] 在 `build_curve_snapshot` 中，当 `product_class` 列缺失或为空时，直接返回空 curve（保持稳定）。
  - [x] 或使用 `pd.Series("", index=df.index)` 作为 fallback，确保布尔索引对齐。

- [ ] **Step 2: 验证**
  - [x] `python3 -m py_compile internal/live/options_worker.py`
  - [x] `go test ./...`

- [ ] **Step 3: Review Gate**
  - [ ] 完成后先请你 code review。
  - [ ] review 通过后再 `git add/commit`。

### Target Files (Batch-4.11)

- [x] `internal/live/options_worker.py`
