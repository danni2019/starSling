# starSling 实时链路方案（JSON-RPC + Router，UI Poll + Section Diff）

> 目标：轻量化实现“行情 500ms 近实时 + 多 Python worker 派生计算 + Go TUI 稳定刷新”。
> 本文仅覆盖**数据链路 / IPC / 刷新机制 / 技术栈约束**，不再重复 UI 布局设计。

---

## 0. 边界与原则

* **只传最新**：任何链路不做队列堆积；所有计算单元仅获取/处理“最新一次快照”。
* **UI 稳定刷新**：UI 不因新数据到达而频繁重绘；采用固定 tick 拉取并按 section 独立更新。
* **协议统一**：全链路统一 JSON-RPC 2.0，复用工具链、日志、调试方式。

---

## 1. 进程与职责

### 1.1 进程列表

* `router`（Go）：

  * 维护最新快照缓存（market/curve/options/unusual/log）
  * 提供 JSON-RPC server：UI/worker 拉取最新数据；接收 live_md/worker 推送结果
  * 维护 UI 的共享控制状态（`focus_symbol` + `turnover_chg_threshold`/`turnover_ratio_threshold`）
* `live_md`（Python）：

  * 从行情源（未来 CTP）生成 `MarketSnapshot`（500ms）
  * 通过 JSON-RPC notification 推送给 router
* `py_worker_*`（Python，多进程，可扩展；当前已有 options_worker / unusual_worker）：

  * 向 router **拉取**最新 `MarketSnapshot`（不堆积）
  * 读取 router 中的 `UIState`（`focus_symbol` + unusual 阈值）
  * 产出派生快照（curve/options/unusual/log）并推送 router
* `tui`（Go UI）：

  * 每 500ms（或 1s）向 router 拉取 `ViewSnapshot`
  * 对比 section `seq`，仅更新变化的 section（section diff）
  * 高频操作：用户在行情表中改变选中合约时，调用 `SetFocusSymbol` 写入 router
  * 进入 live 界面时启动 live_md / options_worker / unusual_worker

---

## 2. 刷新与频率（核心约束）

### 2.1 live_md -> router（推送）

* 周期：**500ms**
* 语义：推送 `market.snapshot`（最新一次快照）
* router 行为：覆盖式更新 `latest_market`（不保留历史，不排队）

### 2.2 worker <- router（拉取）

* worker 计算循环：

  1. 调用 `GetLatestMarket()` 获取最新 market（带 `seq/ts`）
  2. 如与上次处理的 `seq` 相同，则 sleep（避免重复算）
  3. 读取 `GetUIState()`（`focus_symbol` + unusual 阈值）
  4. 进行派生计算（允许耗时；不追赶旧数据）
  5. 通过 notification `curve.snapshot/options.snapshot/unusual.snapshot/log.append` 推送 router
* router 行为：

  * curve/options/unusual：仅保留最新快照（单份）；options 在 `GetViewSnapshot` 中按 focus_symbol 过滤 rows
  * unusual 历史由 worker 维护（rows 最新在前）
  * logs：维护 ring buffer（有上限）

### 2.3 UI <- router（Poll + Section Diff）

* UI tick：建议 **500ms**（与行情一致）；若实际闪烁，可降为 1s。
* 每个 tick：调用 `GetViewSnapshot(focus_symbol, limits...)`
* UI 更新策略（独立 section）：

  * 每个 section 都带 `seq`（单调递增）
  * UI 记录上次渲染的 `seq`：仅当该 section `seq` 变化时才更新该 section
* UI 稳定性要求（flush & stable）：

  * 原位重绘（in-place redraw），不得向下追加输出
  * 列宽稳定（固定宽或平滑自适应）
  * 选中行稳定（基于 row_key）
  * section 更新原子化：一次 tick 内对每个 section 的更新使用 `QueueUpdateDraw` 聚合执行

### 2.4 启动与关闭时序（已定版）

* 启动顺序（同进程内）：

  1. `router` goroutine 启动并监听 localhost 端口
  2. `tui` 启动主循环（先显示 UI，market 区可为空）
  3. `live_md` 子进程启动并开始推送 `market.snapshot`
  4. `py_worker_*` 启动并进入 `get_latest_market -> compute -> push` 循环
* 关闭顺序（反向）：

  1. 先停止 `tui` poll tick
  2. 再停止 `py_worker_*`
  3. 再停止 `live_md`
  4. 最后关闭 `router` listener 与内部 goroutine

### 2.5 故障与降级策略

* router 不可达（UI/worker/live_md 任一端）：

  * worker 已实现指数退避重连：`0.5s -> 1s -> 2s -> 4s`，上限 `5s`
  * UI/live_md 目前为固定周期重试（无指数退避），失败写入运行日志
  * 不退出主程序；UI 继续显示上一次画面
* live_md 推送中断：

  * router 保留最后一帧 `market`，并在 `ViewSnapshot.market` 标记 `stale=true`
  * UI 保持最后画面，不清空表格
* worker 推送中断：

  * 对应 section 保留最后一帧并标记 `stale=true`
  * 不影响 market 主链路
* 超时约束：

  * JSON-RPC request 默认 timeout `2s`
  * 连续 `N=3` 次 timeout 后在日志中升级为 warn（带 endpoint/method，尚未实现）

---

## 3. JSON-RPC 方法与通知（最小可用集合）

> 约定：
>
> * notification：无 id（不需要 response），用于 push
> * request/response：用于 pull 与 UI 控制
> * snapshot payloads 包含 `schema_version` + `ts`；`seq` 由 router 赋值并返回给 UI

### 3.1 live_md -> router（notification）

* `market.snapshot` (params: MarketSnapshot)

### 3.2 worker -> router（notification）

* `curve.snapshot` (params: CurveSnapshot)
* `options.snapshot` (params: OptionsSnapshot)
* `unusual.snapshot` (params: UnusualSnapshot)
* `log.append` (params: LogAppend)

### 3.3 UI -> router（request/response）

* `ui.set_focus_symbol` (params: {symbol}) -> {ok}
* `ui.set_unusual_threshold` (params: {turnover_chg_threshold, turnover_ratio_threshold}) -> {ok}
* （本期不需要 set_sort_spec，排序仅在 UI 本地生效）

### 3.4 UI/worker <- router（request/response）

* `router.get_latest_market` (params: {min_seq?}) -> MarketSnapshot

  * `min_seq` 可选：若当前最新 seq <= min_seq，则返回 {unchanged:true, seq:latest}（减少 payload）
* `router.get_ui_state` -> UIState {focus_symbol, turnover_chg_threshold, turnover_ratio_threshold}
* `router.get_view_snapshot` (params: {focus_symbol, limits}) -> ViewSnapshot

---

## 4. 数据契约（Snapshot Schemas）

### 4.1 公共字段

* `schema_version`: int
* `ts`: int64（**固定为毫秒级 epoch**）
* `seq`: int64（**由 router 为每个 section 统一单调递增**）
* `stale`?: bool（仅 router -> UI 返回时存在）

### 4.2 MarketSnapshot（用于左上行情表）

* `row_key`: string（固定为 "ctp_contract"，用于稳定选中）
* `columns`: []string（固定顺序）
* `rows`: []object（每行按 columns 提供字段；数值可为 number 或 string）

### 4.3 CurveSnapshot（右上）

* `rows`: [{ctp_contract, forward, vix, volume?, open_interest?, bid1?, ask1?, bid_vol1?, ask_vol1?}]

### 4.4 OptionsSnapshot（右中）

* `rows`: [{ctp_contract, underlying, symbol, strike, option_type, last, volume, tte, underlying_price, iv, delta, gamma, theta, vega, iv_reason, price_for_iv}]

### 4.5 UnusualSnapshot（右下滚动信息窗）

* `rows`: [{ts, time, ctp_contract, symbol, underlying, cp, strike, tte, price, volume, turnover, turnover_chg, turnover_ratio}]
* 语义：rows 必须“最新在前”（push-front）；历史由 worker 维护，UI 只做渲染/裁剪。

### 4.6 LogAppend / LogSnapshot（左下）

* append：{ts, level, source, message}
* router 内部维护 ring buffer；在 ViewSnapshot 中返回最近 N 条。

### 4.7 ViewSnapshot（UI poll 返回）

* `market`: MarketSnapshot（ViewSnapshot 中会过滤掉期权行）
* `curve`: CurveSnapshot?（由 worker 基于 focus_symbol 计算；无则为空）
* `options`: OptionsSnapshot?（按 focus_symbol 过滤 rows；无则为空）
* `unusual`: UnusualSnapshot?（无则为空）
* `logs`: {seq, items:[...]}（最近 N 条）
* 每个 section 可带 `stale`，由 router 根据最后更新时间标记。

### 4.8 序列化规则（强约束）

* 时间字段：

  * `ts` 使用 epoch 毫秒（int64）
  * `MarketSnapshot.rows` 中的 `trading_date/datetime/list_date/expiry_date` 使用 **ISO 8601 带时区字符串**（Asia/Shanghai，示例：`2026-02-04T21:05:00+08:00`）
* 浮点字段：

  * JSON 中禁止 `NaN/Inf/-Inf`
  * 统一在 python 侧转换为 `null`
* 缺失值：

  * 缺失/不可用字段统一输出 `null`，不要输出空字符串占位
* 行键要求：

  * `row_key` 固定为 `ctp_contract`
  * 若 `ctp_contract` 为空，该行直接丢弃（不进入 snapshot）

---

## 5. UI 侧排序（与 DataFrame 输出解耦）

* UI 维护 `sort_spec = {sort_by, order}`（仅本地）
* UI 每次 tick poll `ViewSnapshot` 后：

  * 对 market.rows 在 UI 内排序（两侧值可转为 float 则按数值比较，否则按字符串比较）
  * 列宽策略：固定或平滑自适应（防抖动）
* sort_spec **不写入 router**；仅 `focus_symbol` 反向传播给 router/worker。

### 5.1 默认排序与比较规则（已定版）

* 默认排序：`sort_by=volume`，`order=desc`
* 数值列判定：两侧值都能转换为 float 则按数值比较，否则按字符串比较
* 稳定排序：主键按 `sort_spec`，次键固定 `ctp_contract`（保证刷新时行顺序稳定）

### 5.2 Market 过滤（已落地）

* 支持按 exchange / product_class / symbol / ctp_contract 过滤（Enter 弹窗）
* 过滤仅影响 UI 显示，不回写 router

---

## 6. 稳定刷新（flush & stable）验收标准

* 不得出现“旧内容仍在 + 新内容打印在下方”的效果（必须原位刷新）。
* 列宽稳定：连续刷新中列宽不应频繁变化（使用固定宽或滚动窗口平滑）。
* 行稳定：排序条件不变时，行应基于 row_key 稳定更新；选中合约保持选中。
* section 独立更新：market/curve/options/unusual/log 各自依据 `seq` 变化独立刷新。
* UI redraw 节流：仅在 tick 中触发；单 tick 内聚合 `QueueUpdateDraw`，避免撕裂。

---

## 7. 严格技术栈与轻量化约束（必须遵守）

* **Go**：tcell + tview（不引入更重 UI 框架）。
* **Python**：仅依赖 pandas + 必要的 JSON-RPC 库（优先 asyncio 生态，避免引入大而全的服务框架）。
* **IPC/协议**：统一 JSON-RPC 2.0 over TCP（localhost），分帧协议固定为 **length-prefix**（外层消息体保持 JSON-RPC 结构）。
* **无消息队列**：本阶段不引入 Redis/NATS/Kafka 等。
* **无持久化要求**：router 仅维护内存最新快照 + ring buffer（有上限）。
* **无高 FPS**：UI 500ms 或 1s tick；禁止用“消息到达立即重绘”的方式。
* **可观测性**：所有 JSON-RPC 请求/通知必须带 ts/seq/schema_version，router 与 worker 需打印结构化日志（可开关）。

---

## 8. 最小联调里程碑（用于批准/验收）

1. live_md 每 500ms 推 market.snapshot 到 router；router 更新 latest_market。
2. 至少 1 个 worker 能从 router 拉 latest_market + ui_state，并推送 unusual.snapshot。
3. Go UI 每 500ms poll ViewSnapshot：

   * 左上 market 表独立刷新
   * 右下 unusual 信息窗独立刷新（最新在最上）
   * 切换选中 symbol 后，右侧窗口在后续 tick 切换到对应 symbol 的数据
4. 所有 section 以 seq diff 的方式更新，界面无追加打印、无明显抖动。

---

## 9. 可行性评估与已确认事项

**可行性评估**

- 方案可行。JSON-RPC + router 作为“最新快照缓存层”，能保证 UI 稳定刷新与多 worker 扩展性。
- UI 端使用 tick 拉取 + section diff，符合“flush & stable”目标（避免追加输出、减少抖动）。

**已确认事项**

1. **router 进程形态**：内嵌到现有 `starsling` 进程内（goroutine）。
2. **JSON-RPC 分帧协议**：采用 length-prefix。
3. **MarketSnapshot size**：采用全量 snapshot（不做 top N/分页）。
4. **排序职责**：排序仅在 UI 左上行情表本地生效，不写入 router；唯一反向传播的是 `focus_symbol`。
5. **seq 来源**：由 router 为每个 section 统一递增。
6. **时间字段格式**：`rows` 内 datetime 字段统一为 ISO 8601 带时区（`+08:00`）。
7. **UI tick 默认频率**：固定 500ms（若现场出现闪烁，再降为 1s）。

---

## 10. 执行方案（步骤 + checklist）

### 10.1 执行步骤

1. **定义 IPC/Router 基础**
   - 新增 router 模块（Go）维护 latest snapshots + ring buffer + UI state。
   - 实现 JSON-RPC server（TCP localhost）。
   - 分帧方案固定为 length-prefix，并提供 encode/decode 公共工具。
   - 实现 request timeout（2s）与统一错误码映射。
2. **live_md 输出接入 router**
   - live_md 生成 MarketSnapshot（从 DataFrame 组装）。
   - 通过 JSON-RPC notification `market.snapshot` 推送 router。
   - MarketSnapshot 字段映射已实现（DataFrame -> columns/rows/row_key）。
   - 增加 python 侧清洗：NaN/Inf -> null、datetime -> ISO8601(+08:00)。
3. **Go UI 侧拉取与渲染**
   - UI 每 500ms poll `router.get_view_snapshot`。
   - 按 section seq diff 更新对应区域（市场表/曲线/期权/日志）。
   - 统一刷新入口，确保原位重绘、固定列宽减少抖动。
   - 仅当 section `seq` 变化时更新对应组件，避免全屏重绘。
4. **排序交互（仅 UI 本地）**
   - UI 侧维护 sort_spec（列 + 升/降序）。
   - UI 内部对 market.rows 排序，仅影响显示。
5. **联调与验收**
   - 先只打通 market + logs，确保 UI 刷新稳定。（已完成）
   - 已接入 unusual/curve/options（options_worker / unusual_worker）。
6. **故障恢复与可观测性**
   - UI/live_md/worker 统一实现指数退避重连（0.5s~5s，worker 已完成，UI/live_md 待补）。
   - 各 section 支持 stale 标记与日志告警（不清空最后画面）。

### 10.2 Checklist

- [x] router 提供 JSON-RPC server，支持 market/curve/options/unusual/log 的缓存与 UI state（含 unusual 阈值）。
- [x] live_md 能稳定推送 `market.snapshot`（500ms）。
- [x] live_md 输出满足序列化规则（datetime ISO8601+08:00，NaN/Inf 已转 null）。
- [x] UI 每 500ms poll view snapshot，market 表格无追加打印、无明显抖动。
- [x] 排序切换生效（升/降序），行选中稳定。
- [x] 默认排序为 `volume desc`，且稳定排序次键为 `ctp_contract`。
- [x] 日志与异常处理可见（stale/parse error 不崩溃，已接入 live log 面板）。
- [ ] 断线重连/backoff 与 timeout(2s) 行为已联调通过（UI/live_md 仍为固定重试）。

### 10.3 需要修改/新增的文件（初版）

- **新增**：`internal/router/`（路由缓存 + JSON-RPC server）
- **新增**：`internal/ipc/`（JSON-RPC client/server 共用工具，可选）
- **新增**：`internal/live/options_worker.py` / `internal/live/unusual_worker.py`（worker 计算 + push）
- **新增**：`internal/live/options_worker.go` / `internal/live/unusual_worker.go`（worker 启动器）
- **修改**：`internal/live/live_md.py`（推送 market.snapshot）
- **修改**：`internal/live/process.go`（启动 live_md 时带 router 地址）
- **修改**：`internal/tui/`（新增 router client + poll 逻辑）
- **修改**：`cmd/starsling/main.go`（启动 router / 连接 router）

### 10.4 已完成与待办

- 已实现 `options_worker` / `unusual_worker`：从 MarketSnapshot 拉取并推送 curve/options/unusual/log（`py_worker_template.py` 仍保留为扩展入口）。
- 已实现 `live_md` 中 MarketSnapshot 的字段映射与列定义（DataFrame -> columns/rows/row_key），字段清单如下：
  - `live_md` line:301  market_snapshot = spi.md()
  - market_snapshot columns字段：
      'trading_date' -- pandas Datetime64,
      'datetime' -- pandas Datetime64, 
      'ctp_contract' -- string,
      'last' -- float | last price,
      'pre_settlement' -- float | previous trading date's settlement price,
      'pre_close' -- float | previous trading date's close price,
      'pre_open_interest' -- int | previous trading date's open interest,
      'open' -- float | opening price,
      'high' -- float,
      'low' -- float,
      'volume' -- int ,
      'turnover' -- float,
      'open_interest' -- int ,
      'close' -- float,
      'settlement' -- float,
      'limit_up' -- float,
      'limit_down' -- float,
      'bid1' -- float,
      'ask1' -- float,
      'bid_vol1' -- int,
      'ask_vol1' -- int,
      'average_price' -- float,
      'exchange' -- string, 
      'name' -- string, 
      'product_class' -- string | 商品类别（'1'-期货，'2'-期权，'3'-组合，'8'-股票，'f'-基金,'b'-债券）, 
      'symbol' -- string, 
      'multiplier' -- float, 
      'list_date' -- pandas Datetime64, 
      'expiry_date' -- pandas Datetime64, 
      'underlying' -- string, 
      'option_type' -- string | 期权类型（'1'-认购，'2'-认沽）, 
      'strike' -- null / float, 
      'status' -- string |  合约状态（'0'-未上市，'1'-上市，'2'-停牌，'3'-到期/退市）
  - market_snapshot index 已重置为pure numeric排序，无需处理。如后续需要index信息，则根据需求，重设为 ctp_contract 或其他字段
- TODO: 期权 Greeks 计算参数中的 `risk_free_rate` 当前固定为 `0.01`；
  - 后续在 main UI 新增“通用配置入口”，将该参数与其他运行参数统一暴露给用户，并支持持久化保存。

---

## 10.5 Batch-1 完成状态（已落地）

- 已打通最小闭环：`live_md -> router -> tui(left-top market)`，并补齐 `options_worker/unusual_worker -> router -> tui` 的右侧 sections + logs。
- 已完成内嵌 router 生命周期管理（main 启动/停止）。
- 已完成 UI 侧 `ui.set_focus_symbol` 回写 router，并支持 unusual 阈值同步。
- 已新增 `options_worker` / `unusual_worker`（含 Go 启动器）；`py_worker_template.py` 保留为扩展入口。
- 当前未完成项：UI/live_md 的指数退避重连与 E2E smoke；`risk_free_rate` 配置入口。

---

## 11. 测试与验收矩阵（必须执行）

### 11.1 Router / IPC 单测（Go）

- length-prefix 编解码：粘包/拆包/空包/非法长度。
- JSON-RPC：notification 与 request/response 的方法分发、错误码、timeout。
- seq 递增：每个 section 独立递增，且 diff 判断稳定。

### 11.2 live_md Payload 单测（Python）

- DataFrame -> MarketSnapshot 转换正确（字段完整、row_key 正确）。
- datetime 字段输出 ISO8601+08:00。
- NaN/Inf 处理为 null。

### 11.3 UI 渲染单测/行为测试（Go）

- section diff：仅变化 section 更新，未变化 section 不重绘。
- market 本地排序：`volume desc` 默认生效，升降序切换正确。
- 稳定性：同一 sort_spec 下，选中行基于 `ctp_contract` 保持稳定。

### 11.4 E2E Smoke（手工/脚本）

1. 启动 starsling（内嵌 router）后，UI 可进入 live panel 且无崩溃。
2. live_md 推送后，左上 market 表可持续刷新且无追加打印。
3. 断开 live_md 或 router 后，UI 显示 stale/断线状态并自动重连。
4. 恢复连接后，数据继续刷新，排序与选中状态不丢失。

---

## 12. Batch-2 准备计划（Ready）

### 12.1 目标

1. 打通 `py_worker -> router -> tui` 的右侧 sections（curve/options/unusual）与左下 logs。（已完成）
2. 完成 worker 拉取 market + focus_symbol，并推送派生结果。（已完成）
3. 完成断线重连/backoff 联调与稳定性验收。（待验证）

### 12.2 执行步骤

1. 在 `python/py_worker_template.py` 基础上实现 JSON-RPC client 循环（拉取 market/ui_state，推送派生 snapshots）。（已完成）
2. 扩展 router notification 处理：`curve.snapshot`、`options.snapshot`、`unusual.snapshot`、`log.append`。（已完成）
3. 扩展 `router.get_view_snapshot`：按 `focus_symbol` 返回右侧 sections + logs。（已完成）
4. TUI 按 section `seq` 做 diff 渲染，确保仅更新变化 section。（已完成）
5. 完成断线重连与 stale 标记联调（worker/router/live_md 三端）。（待验证）

### 12.3 Batch-2 Checklist

- [x] worker 能稳定拉取 `router.get_latest_market` 与 `router.get_ui_state`。
- [x] worker 能推送 `curve/options/unusual/log` 四类 notification。
- [x] router 已支持接收并缓存 `options.snapshot`，并按 `focus_symbol` 返回 options 快照。
- [x] UI 右中 options section 已按 seq diff 刷新（IV/Volume 文本图 + 明细预览）。
- [x] UI 左上 market 已支持 Enter 弹窗进行筛选/排序（exchange/product_class/symbol + sort_by/order）。
- [x] 左下 logs 与右上/右下 section 的真实 worker 数据链路已接入。
- [ ] 断线重连/backoff 与 stale 行为通过 E2E smoke。


### 右侧区域设计基础
- 右上：VIX curve + forward Curve ｜ 对应option_worker
  - X轴：期货合约，从左至右升序
  - VIX：underlying期货合约的VIX = 该期货合约对应期权链所有delta绝对值小于 0.25 的期权IV的均值
  - forward curve: 各个期货合约的现价 last
  - 是否提供Enter键弹出二级窗口：支持（语音播报设置）

- 右中：IV curve + Volume Bar | 对应option_worker
  - X轴：期权strike，从左至右升序
  - IV：期权当前的vwap价格对应的IV，利用以下四个字段计算vwap：
      'bid1' -- float,
      'ask1' -- float,
      'bid_vol1' -- int,
      'ask_vol1' -- int,
  - Volume： 期权当前的成交量volume
  - 是否提供Enter键弹出二级窗口：支持（delta 过滤）

- 右下：异常成交 Unusual Volume ｜ 对应unusual_worker
  - 针对期权合约的异常成交监控
  - 信息窗独立刷新（最新在最上）
  - 因为行情字段中的volume, turnover都是当日累计值，所以unusual worker里必须保存一个上一帧的行情数据用来计算差值
  - 虽然名称是Volume，但实际上是通过turnover来计算异常成交
  - 计算方式：
      turnover_chg = turnover(current_frame) - turnover(prev_frame)
      turnover_ratio = turnover(current_frame) / turnover(prev_frame) - 1
  - 展示方式：如果以上两项均超过了设定的threshold，则展示在窗口内，否则忽略
  - 是否提供Enter键弹出二级窗口：支持。二级窗口用来设置以上两项的阈值。
