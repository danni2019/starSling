# 左下聚合区优化执行方案（待审批）

## 1. 需求目标（本次范围）
1. 左下聚合分析表新增 `TIME_WINDOW` 列，逐行展示该记录的事件时间窗口：`start_time ~ end_time`（`HH:MM:SS ~ HH:MM:SS`）。
2. 左下筛选窗口新增 checkbox：`Only selected contracts`。
3. 左下筛选窗口新增 checkbox：`Only focused symbol`。
4. 当上述 checkbox 勾选后，左下聚合结果按条件实时过滤。

## 2. 口径确认（已定）
1. `Only selected contracts`：按左上“当前展示列表”中的 contracts 过滤（不是仅当前高亮行）。
2. `Only focused symbol`：复用右上 focus 规则（`contract/underlying/symbol` 任一匹配）；等价于按右上区域对应 contracts 过滤。
3. 若 `Only selected contracts=true` 且左上展示列表为空：左下显示空，并提示 `Currently no contracts are selected`。
4. 若 `Only focused symbol=true` 且当前 focus 为空：左下显示空，并提示 `Currently no focused symbol`。

## 3. 现状分析（基于当前代码）
- 左下区域数据渲染主路径：
  - 聚合计算：`/Users/daniel/projects/starSling/internal/tui/app.go` 中 `renderFlowAggregation()`
  - 设置弹窗：`/Users/daniel/projects/starSling/internal/tui/app.go` 中 `openFlowSettings()` / `applyFlowSettings()`
  - 表格结构：`/Users/daniel/projects/starSling/internal/tui/view_live.go` 中 `FlowRow` + `fillFlowTable()`
- 当前左下没有“行级时间窗口”列，只有标题显示全局窗口。
- 当前 `Flow Aggregation Settings` 仅支持 `window_size` 与 `min_analysis`，没有布尔筛选项。

## 4. 设计与实现方案

### 4.1 新增行级时间窗口列
- 目标：每条 underlying 聚合结果显示其自身事件覆盖区间，而非仅标题区间。
- 实施点：
  - 在 `flowUnderlyingAgg` 增加行级窗口字段（如 `WindowStartTS`、`WindowEndTS`）。
  - 在按 underlying 分组聚合时同步计算该组最小/最大 `TS`。
  - 新增时间格式化函数（例如 `formatFlowWindow(startTS, endTS)`），输出 `15:04:05 ~ 15:04:05`。
  - 在 `FlowRow` 增加字段（如 `TimeWindow`）。
  - 在 `fillFlowTable()` 新增 `TIME_WINDOW` 表头和对应列值。

### 4.2 左下筛选弹窗新增两个 checkbox
- 目标：在现有 `Flow Aggregation Settings` 中增加两个筛选开关并持久在 UI state。
- 实施点：
  - 在 `UI` 结构体增加：
    - `flowOnlySelectedContracts bool`
    - `flowOnlyFocusedSymbol bool`
  - 在 `openFlowSettings()` 中新增两项 checkbox：
    - `Only selected contracts`
    - `Only focused symbol`
  - `Apply` 时：
    - 继续调用现有 `applyFlowSettings(windowRaw, minRaw)`（不改函数签名）。
    - 在 `Apply` 回调中直接写回两个 bool 状态并触发 `renderFlowAggregation()`。
  - 根据新增表单项调整弹窗高度，避免截断。

### 4.3 聚合前过滤逻辑（核心）
- 原则：先剪裁窗口（现有 prune 逻辑），再应用筛选条件，再做 underlying 聚合。
- 过滤A：`Only selected contracts`
  - 数据源取左上当前展示列表：`ui.marketRows[*].Symbol`（即合约代码）。
  - 用大小写不敏感集合匹配 `event.Contract`。
  - 若该集合为空：返回空结果并显示提示 `Currently no contracts are selected`。
- 过滤B：`Only focused symbol`
  - 以 `ui.currentFocusSymbol()` 作为 focus 值。
  - 匹配逻辑抽成 helper，复用右上规则：`event.Contract` / `event.Underlying` / `event.Symbol` 任一匹配（忽略大小写）。
  - 若 focus 为空：返回空结果并显示提示 `Currently no focused symbol`。
- 两个 checkbox 同时勾选时取交集（AND）。

### 4.4 空结果与提示文案渲染
- 在 `fillFlowTable()` 增加可选提示文案参数，支持场景化空提示：
  - 默认：`Waiting for unusual events...`
  - selected-contracts 空：`Currently no contracts are selected`
  - focused-symbol 空：`Currently no focused symbol`
- 当两个条件同时触发空场景时，优先使用更直接原因：
  - focus 为空优先显示 `Currently no focused symbol`；否则显示 contracts 为空提示。

### 4.5 联动刷新与时序修正
- 目标：避免筛选切换后显示旧结果或 focus 滞后一帧。
- 实施点：
  - 保留并使用现有 market 刷新中的 `renderFlowAggregation()` 触发。
  - focus 变更路径补一次 `renderFlowAggregation()`（左上选择变化、`ensureFocusSymbol()` 自动重置时均覆盖）。
  - 修正 `collecting` 分支行为：当筛选条件或数据窗口变化导致当前不可展示时，不保留旧表格内容，按当前状态显示空提示。

### 4.6 可测试性与复用
- 抽象 helper：
  - `eventMatchesFocus(event, focus string) bool`（复用右上规则）。
  - `buildSelectedContractsSet(rows []MarketRow) map[string]struct{}`。
  - `filterFlowEvents(events []flowEvent, ...flags) ([]flowEvent, emptyReason)`。
- 这样可直接做纯函数单测，减少 UI 交互耦合。

## 5. 变更文件清单
1. `/Users/daniel/projects/starSling/internal/tui/app.go`
- 新增 UI 状态字段。
- 扩展 `openFlowSettings()` 表单。
- 保持 `applyFlowSettings()` 仅处理窗口参数。
- 在 `renderFlowAggregation()` 中加入筛选、行级时间窗口、空提示分支。
- 新增过滤 helper 与 focus 匹配 helper。
- 补充 focus 相关路径的 flow 重绘。

2. `/Users/daniel/projects/starSling/internal/tui/view_live.go`
- 扩展 `FlowRow`。
- `fillFlowTable()` 新增 `TIME_WINDOW` 列。
- `fillFlowTable()` 支持自定义空提示文案。

3. `/Users/daniel/projects/starSling/internal/tui/app_test.go`
- 更新受列索引影响的断言。
- 新增 flow 行级窗口列与过滤行为单测。
- 新增“selected contracts 空提示 / focused symbol 空提示”单测。
- 新增“collecting 状态下切换筛选不保留旧结果”单测。
- 新增“focus 自动变更后 flow 同步重绘”单测。

## 6. 测试计划
1. 现有回归
- 运行：`go test ./internal/tui`

2. 新增/调整用例
- `renderFlowAggregation` 输出包含 `TIME_WINDOW` 且格式正确。
- `Only selected contracts = true` 时，仅展示左上当前可见合约对应聚合。
- `Only focused symbol = true` 时，仅展示当前 focus 品种对应聚合。
- 两个开关同时为 true 时验证交集行为。
- `Only selected contracts=true` 且左上无合约时，显示 `Currently no contracts are selected`。
- `Only focused symbol=true` 且 focus 为空时，显示 `Currently no focused symbol`。
- collecting 期间切换筛选条件，不显示旧结果。
- focus 变更（手动与自动）后，flow 过滤同帧生效。

## 7. 验收标准（逐条对应需求）
1. 左下表头可见 `TIME_WINDOW`，每行值为 `start_time ~ end_time`。
2. Flow 设置弹窗可见 `Only selected contracts` checkbox，勾选后结果仅来自左上当前展示合约。
3. Flow 设置弹窗可见 `Only focused symbol` checkbox，勾选后结果仅来自当前 focus 品种（按右上同口径）。
4. 当 selected contracts 为空时显示 `Currently no contracts are selected`。
5. 当 focused symbol 为空时显示 `Currently no focused symbol`。
6. 两个筛选支持同时开启，结果为交集，界面无崩溃、无空指针。
7. `go test ./internal/tui` 通过。

## 8. 关键风险与控制
- 风险：focus/collecting 时序导致“旧结果残留”。
- 控制：在 collecting 分支和 focus 变更分支强制按当前过滤条件重绘空状态。

- 风险：新增列导致旧断言列号偏移。
- 控制：同步修正相关单测索引并补充显式 header 断言。

- 风险：不同区域 focus 匹配规则漂移。
- 控制：抽象统一 helper，右上与左下共用同一匹配口径。

## 9. 执行顺序
1. 改 `FlowRow/fillFlowTable`，先固定列结构与空提示接口。
2. 改 `renderFlowAggregation`，补齐行级窗口、过滤、collecting 行为。
3. 改 `openFlowSettings`，接入两个 checkbox（不改 `applyFlowSettings` 签名）。
4. 增加 focus/market 相关重绘触发点与时序修正。
5. 补齐/修正测试并跑 `go test ./internal/tui`。
