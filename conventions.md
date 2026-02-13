# starSling Conventions

> 本文件从 `plan.md` 中提炼，作为后续开发的长期约定参考。
> 覆盖范围：链路职责、协议口径、字段语义、刷新机制、稳定性标准、技术约束。

---

## 1. 全局原则

- 只传最新：链路不做队列堆积，计算单元只处理最新快照。
- UI 稳定刷新：UI 使用固定 tick 拉取，按 section diff 更新，避免抖动与追加打印。
- 协议统一：全链路使用 JSON-RPC 2.0。

---

## 2. 进程职责约定

- `router`（Go）
  - 维护最新快照缓存：`market/curve/options/unusual/log`
  - 提供 JSON-RPC server（供 UI/worker 拉取）
  - 接收 live/worker 推送
  - 维护 UI 共享状态：`focus_symbol`、unusual 阈值
- `live_md`（Python）
  - 行情接入并按 500ms 推送 `market.snapshot`
- `py_worker_*`（Python）
  - 拉取最新 market + ui_state
  - 产出派生快照：`curve/options/unusual/log`
- `tui`（Go）
  - 固定 tick 拉取 `view_snapshot`
  - 按 section `seq` 做增量刷新
  - 交互变更只回写必要状态（如 `focus_symbol`、unusual 阈值）

---

## 3. 刷新频率与时序

### 3.1 频率

- `live_md -> router`：500ms 推送 `market.snapshot`
- `worker -> router`：拉取最新 market 后计算并推送派生 snapshot
- `UI -> router`：500ms poll `view_snapshot`（必要时可降至 1s）

### 3.2 启停顺序

- 启动：`router -> tui -> live_md -> py_workers`
- 关闭：`tui tick -> py_workers -> live_md -> router`

### 3.3 超时

- JSON-RPC request 默认 timeout：`2s`

---

## 4. JSON-RPC 与分帧约定

- 传输：localhost TCP
- 分帧：length-prefix
- 协议：JSON-RPC 2.0
- notification：无 `id`，用于 push
- request/response：用于 pull 与 UI 控制
- snapshot payloads：必须包含 `schema_version` + `ts`；`seq` 由 router 管理

### 4.1 方法清单（最小集合）

- live_md -> router（notification）
  - `market.snapshot`
- worker -> router（notification）
  - `curve.snapshot`
  - `options.snapshot`
  - `unusual.snapshot`
  - `log.append`
- UI -> router（request）
  - `ui.set_focus_symbol` -> `{ok}`
  - `ui.set_unusual_threshold` -> `{ok}`
- UI/worker <- router（request）
  - `router.get_latest_market`（支持 `min_seq`）
  - `router.get_ui_state`
  - `router.get_view_snapshot`

---

## 5. Snapshot 数据契约

### 5.1 公共字段

- `schema_version: int`
- `ts: int64`（epoch 毫秒）
- `seq: int64`（由 router 对各 section 独立单调递增）
- `stale?: bool`（router 返回 UI 时附带）

### 5.2 `MarketSnapshot`

- `row_key: "ctp_contract"`
- `columns: []string`
- `rows: []object`
- 说明：空 `ctp_contract` 行必须丢弃。

### 5.3 `CurveSnapshot`

- `rows: [{ctp_contract, forward, vix, volume?, open_interest?, bid1?, ask1?, bid_vol1?, ask_vol1?}]`

### 5.4 `OptionsSnapshot`

- `rows: [{ctp_contract, underlying, symbol, strike, option_type, last, volume, tte, underlying_price, iv, delta, gamma, theta, vega, iv_reason, price_for_iv}]`

### 5.5 `UnusualSnapshot`

- `rows: [{ts, time, ctp_contract, symbol, underlying, cp, strike, tte, price, volume, turnover, turnover_chg, turnover_ratio}]`
- 语义：必须最新在前（push-front），历史由 worker 维护。

### 5.6 `LogAppend / LogSnapshot`

- append: `{ts, level, source, message}`
- router 维护 ring buffer，`view_snapshot.logs` 返回最近 N 条。

### 5.7 `ViewSnapshot`

- `market`：过滤掉期权行
- `curve/options/unusual/logs`：按当前状态返回（可为空）
- options 在 router 侧按 `focus_symbol` 过滤

---

## 6. 序列化与数据清洗约定

- 时间字段
  - `ts` 使用 epoch 毫秒（int64）
  - `trading_date/datetime/list_date/expiry_date` 使用 ISO 8601 且带时区（Asia/Shanghai）
- 浮点字段
  - JSON 中禁止 `NaN/Inf/-Inf`
  - Python 侧统一转为 `null`
- 缺失值
  - 缺失统一输出 `null`，不要用空字符串占位

---

## 7. UI 行为约定

### 7.1 Section Diff 刷新

- 每个 section 用独立 `seq`
- UI 仅当 section `seq` 变化时更新该 section
- 单 tick 内聚合 `QueueUpdateDraw`，保证原子更新

### 7.2 排序与过滤

- 排序仅 UI 本地生效，不写回 router
- 默认排序：`volume desc`
- 比较规则：两侧可转 float 则按数值，否则按字符串
- 稳定排序次键：`ctp_contract`
- market 过滤仅影响 UI 显示，不回写 router

### 7.3 共享状态回写

- 允许回写：`focus_symbol`、unusual thresholds
- 不回写：`sort_spec`

---

## 8. 异常与降级约定

- router 不可达
  - worker：指数退避 `0.5s -> 1s -> 2s -> 4s`，最大 `5s`
  - UI/live_md：固定重试并记录日志（后续可升级 backoff）
- 推送中断
  - router 保留最后一帧，并在对应 section 标记 `stale=true`
  - UI 不清空旧画面，显示 stale 状态

---

## 9. 技术栈与工程约束

- Go UI：`tcell + tview`
- Python：`pandas + 必要 JSON-RPC 依赖`
- IPC：JSON-RPC 2.0 over TCP + length-prefix
- 不引入消息队列（Redis/NATS/Kafka）
- router 仅维护内存最新快照 + ring buffer
- 禁止“消息到达即重绘”；必须 tick 驱动刷新

---

## 10. 市场字段语义（MarketSnapshot rows）

### 10.1 当前数据源

- 当前项目默认行情数据源：`open_ctp`
- 以下枚举释义与映射，当前均以 `open_ctp` 字段口径为准。

### 10.2 通用字段语义

- `trading_date`: 交易日
- `datetime`: 行情时间戳
- `ctp_contract`: 合约代码（主键）
- `last`: 最新价
- `pre_settlement`: 昨结算
- `pre_close`: 昨收
- `pre_open_interest`: 昨持仓
- `open/high/low/close`: 开高低收
- `volume`: 成交量（日累计）
- `turnover`: 成交额（日累计）
- `open_interest`: 持仓量
- `settlement`: 当日结算价
- `limit_up/limit_down`: 涨跌停价
- `bid1/ask1`: 一档买卖价
- `bid_vol1/ask_vol1`: 一档买卖量
- `average_price`: 均价
- `exchange`: 交易所
- `name`: 合约名称
- `symbol`: 品种代码
- `multiplier`: 合约乘数
- `list_date/expiry_date`: 上市日/到期日
- `underlying`: 期权标的合约
- `strike`: 行权价

### 10.2.1 合约映射优先级（强制）

- 所有以下转换都必须先查 `contract` metadata（`InstrumentID/ProductID/UnderlyingInstrID`）：
  - `option contract -> underlying contract`
  - `option contract -> symbol`
  - `underlying contract -> symbol`
  - `future contract -> symbol`
- 仅当 metadata 缺失该合约映射时，才允许回退到字符串推断（如按合约前缀或 C/P 位置解析）。

### 10.3 open_ctp 枚举字段映射（必须遵守）

- `product_class`（open_ctp 原始值 -> 含义）
  - `'1'` -> 期货
  - `'2'` -> 期权
  - `'3'` -> 组合
  - `'8'` -> 股票
  - `'f'` -> 基金
  - `'b'` -> 债券
- `option_type`（open_ctp 原始值 -> 含义）
  - `'1'` -> 认购（Call）
  - `'2'` -> 认沽（Put）
- `status`（open_ctp 原始值 -> 含义）
  - `'0'` -> 未上市
  - `'1'` -> 上市
  - `'2'` -> 停牌
  - `'3'` -> 到期/退市

### 10.4 内部计算口径映射（严格备注）

- 期权计算（尤其 Greeks/Flow）内部标准口径使用 `c/p`，不是 `1/2`。
- 强制映射规则：
  - `open_ctp.option_type == '1'` -> internal `c`
  - `open_ctp.option_type == '2'` -> internal `p`
- 任何新增计算逻辑都必须使用统一后的 `c/p`，不得直接混用 `1/2`。

### 10.5 新增数据源接入要求（强制）

- 后续若新增任意数据源，必须按本文件同样模式补齐“数据源约定”：
  - 明确数据源名称（source id）与适用模块
  - 列出关键字段语义与单位
  - 列出所有枚举字段的原始值 -> 业务含义映射
  - 列出该数据源到内部标准字段/标准值的转换规则
- 未完成上述标注前，不允许将新数据源字段直接用于核心计算逻辑。

---

## 11. 右侧分析面板口径

### 11.1 Curve（右上）

- X 轴：期货合约升序
- forward：期货 `last`
- VIX：对应期权链中指定 delta 区间 IV 的均值（当前实现以 worker 口径为准）

### 11.2 Options（右中）

- X 轴：strike 升序
- IV：基于报价（优先 vwap/mid/last fallback）反推
- Volume：当前成交量
- 期权类型口径：展示可保留数据源值，但计算必须使用内部标准 `c/p`（`open_ctp: '1'/'2' -> c/p`）

### 11.3 Unusual（右下）

- 使用日累计量的帧间差：
  - `turnover_chg = turnover(curr) - turnover(prev)`
  - `turnover_ratio = turnover(curr) / turnover(prev) - 1`
- 超阈值才展示
- 信息窗最新在上

---

## 12. 测试基线（后续回归最低要求）

- Router/IPC
  - length-prefix 编解码（含异常帧）
  - JSON-RPC 分发与错误路径
  - section `seq` 单调性
- live_md payload
  - DataFrame -> MarketSnapshot 字段完整
  - datetime 时区格式正确
  - NaN/Inf 正确置 null
- UI
  - section diff 正确
  - 默认排序/切换排序正确
  - 刷新稳定，不追加打印
- E2E smoke
  - 启动可进入 live
  - market 持续刷新
  - 断连 stale 与恢复行为正确

---

## 13. 当前已知待完善项（约定保留）

- `UI/live_md` 侧指数退避重连尚未统一到 worker 水平。
- `risk_free_rate` 当前固定为 `0.01`，后续应进入统一配置并支持持久化。
