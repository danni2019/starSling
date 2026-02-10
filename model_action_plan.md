1. 左上区域的筛选排序可用字段选取范围改为： 展示字段
2. 左上区域展示字段新增：CHG%  = CHG / LAST
3. 左上区域展示字段新增：bid vol (BIDV) / ask vol (ASKV) 位置分别在 BID 左侧和 ASK 右侧 
4. 右下区域turnover差异计算逻辑有问题：出现重复计算情形，需要检查是否保存的旧market_snapshot未被及时替换。
5. 右下区域新增OI变化，同样计算 current_oi 和 prev_oi 的CHG和RATIO，并TAG为OI
6. 右下区域筛选条件新增针对OI的筛选条件：OI Ratio，默认筛选 >= 5% 的OI变化。并且，确保当前的 Turnover Chg / Turnover Ratio 仅针对 TAG为turnover的筛选，而 OI Ratio 仅针对 Tag 为 OI 的筛选。
7. 语音播报功能优化：当前如果有两个及以上的播报合约，经常会出现播报声音重叠的问题。需要优化为在播报时以队列形式依次播报，以规避播报声音重叠的问题。

# 8. 左下区域异常成交聚合分析（Flow Aggregation Panel）设计规格

> 设计目标：
> 在不引入重型存储、不增加行情帧数、不扩展数据依赖的前提下，
> 对右下区域已有的异常成交事件流进行**短生命周期、低开销**的聚合分析，
> 并以**表格化、可排序**的方式，快速暴露「哪些品种 / 哪些合约 / ITM or OTM / 增仓还是减仓」正在被集中交易。

---

## 1. 功能定位

* 左下区域不再展示 runtime log
* 新功能仅用于：

  * 聚合右下区域筛选出的异常成交事件
  * 输出**统计性、方向性、可排序**的结果
* 不承载逐笔明细，不做历史回溯，不做长期存储

---

## 2. 数据结构设计

### 2.1 事件流缓存（deque）

* 使用本地 `deque` 保存异常成交事件
* 每个事件为**轻量对象**，只包含聚合所需字段
* 生命周期严格受时间窗口约束

**时间窗口规则**：

* 最大时间窗口：`window_size`（默认 120s，可配置 [60, 300]）
* deque 中始终只保留：

  ```text
  event.datetime >= max(event.datetime) - window_size
  ```

### 2.2 关系映射表（dict）

* 维护一个常驻内存 dict，用于 option → underlying → symbol 的快速映射
* 该映射仅用于：

  * 快速定位事件所属 symbol / underlying
  * 避免在聚合阶段重复字符串解析或查表

---

## 3. 刷新与计算流程

### 3.1 刷新周期内的处理流程

每次 UI / 数据刷新时，按以下顺序执行：

1. **追加新事件**

   * 将本次刷新中新产生的异常成交事件追加进 deque
   * 同步更新聚合桶统计

2. **过期事件清理**

   * 若 deque 头部事件时间早于：

     ```text
     max(datetime) - window_size
     ```
   * 则将其弹出
   * 并对对应聚合桶执行**反向扣减**
   * （或在实现中选择：当过期事件比例过高时，整体重算一次）

3. **最小分析窗口检查**

   * 定义最小分析窗口：`min_analysis_window`（默认 30s，可配置 [15, 60]）
   * 若当前 deque 中：

     ```text
     max(datetime) - min(datetime) < min_analysis_window
     ```
   * 则：

     * 本轮**跳过聚合分析与 UI 更新**
     * 保持上一次有效分析结果

> 该机制用于避免事件刚开始聚集时的统计抖动和误判。

---

## 4. 聚合逻辑（Underlying 级别）

### 4.1 聚合粒度

* 第一层：`symbol`
* 第二层：`underlying`
* 第三层：`option category`

  * ITM
  * OTM

### 4.2 Moneyness 分组

* 单条事件的 moneyness 定义：

  ```text
  moneyness = strike / underlying_last - 1
  ```

* 根据期权方向（CP）划分 ITM / OTM（具体判定规则在实现中固化）

* 在 underlying 层面：

  * ITM 与 OTM 各自形成一组
  * 分别计算平均 moneyness

### 4.3 分组内统计指标

对每个 underlying：

* **TOTAL_TURNOVER_SUM**

  * ITM + OTM 异常成交 TURNOVER 总和

对 ITM / OTM 各分组分别计算：

* `MONEyness`

  * 该分组内事件的平均 moneyness（百分比展示）
    计算方式： turnover加权均值，在分组内计算：
        sum(moneyness * turnover)/sum(turnover)

* `TURNOVER_SUM`

  * 该分组内异常成交 TURNOVER 总和

* `OI_CHG_SUM`

  * 该分组内异常成交对应的持仓变化总和

---

## 5. 聚合结果逻辑结构

逻辑结构如下（用于内部组织与 UI 映射）：

```text
symbol: X
    underlying: X1
        TOTAL_TURNOVER_SUM: 777777
        ITM:
            Moneyness: -3%
            TURNOVER_SUM: 123456
            OI_CHG_SUM: 7890
        OTM:
            Moneyness: 5%
            TURNOVER_SUM: 654321
            OI_CHG_SUM: -3201
    underlying: X2
        ...
symbol: Y
    ...
```

---

## 6. UI 展现形式（左下区域）

### 6.1 表格结构

左下区域以**单一表格**形式呈现，行粒度为：

> `symbol + underlying`

**表格列定义**：

| Column             | 含义               |
| ------------------ | ---------------- |
| SYMBOL             | 品种               |
| UNDERLYING         | 合约               |
| TOTAL_TURNOVER_SUM | 异常成交总额           |
| ITM                | ITM 平均 moneyness |
| ITM_TURNOVER_SUM   | ITM 异常成交额        |
| ITM_OI_CHG_SUM     | ITM 净持仓变化        |
| OTM                | OTM 平均 moneyness |
| OTM_TURNOVER_SUM   | OTM 异常成交额        |
| OTM_OI_CHG_SUM     | OTM 净持仓变化        |

> ITM / OTM 列中仅展示 moneyness 百分比，其余为数值列。

表格上方需要标注当前分析所用数据样本的采样时间段： min_time ~ max_time

---

## 7. 排序与条件窗口

### 7.1 排序交互

* 在表格获得焦点时，按 `Enter` 呼出**条件窗口**
* 条件窗口内支持：

  * 从上述 columns 中选择 **任意一列**作为排序字段
  * 选择排序顺序：`ASC / DESC`

### 7.2 时间参数配置

条件窗口内同时支持修改：

* **时间窗口（window_size）**

  * 默认：120s
  * 可选范围：[60, 300]

* **最小分析窗口（min_analysis_window）**

  * 默认：30s
  * 可选范围：[15, 60]

### 7.3 参数校验规则

* 非法输入（非数字 / NaN）
* 超出允许范围的输入

均执行：

```text
回滚至 default 值
```

并在条件窗口中给予简要提示（不写 log）。

---

## 8. 设计约束与原则回顾

* 不新增行情帧存储
* 不引入数据库 / Redis
* 事件数据短生命周期、用完即丢
* 计算复杂度与事件数量线性相关
* UI 结果稳定、可排序、可解释

> 该模块的唯一职责：
> **在极小成本下，让异常成交真正变成“可读的市场意图”。**

---

## 9. 对应实施方案（逐条，待审批）

以下方案逐条对应文首 1~8 项，先统一给你审批，审批后再按阶段编码。

### 9.1 对应条目 1：左上筛选排序字段范围改为“展示字段”

实施策略：

- 将 `Sort By` 下拉来源从 `marketSortableColumns(ui.marketRawRows)` 改为固定“展示字段白名单”。
- 白名单与左上表头一一对应，避免隐藏字段参与排序。

排序字段白名单（计划）：

- `contract`
- `exchange`
- `last`
- `chg`
- `chg_pct`
- `bidv`
- `bid`
- `ask`
- `askv`
- `vol`
- `turnover`
- `oi`
- `oi_chg_pct`
- `ts`

涉及文件：

- `/Users/daniel/projects/starSling/internal/tui/app.go`

---

### 9.2 对应条目 2：左上新增 `CHG% = CHG / LAST`

实施策略：

- 在 `convertMarketRows` 增加 `chg_pct` 字段计算：
  - `chg = last - pre_settlement`
  - `chg_pct = chg / last`
  - 若 `last` 缺失或为 0，则显示 `-`
- 左上表头与行渲染新增 `CHG%` 列。
- 排序映射中新增 `chg_pct`。

涉及文件：

- `/Users/daniel/projects/starSling/internal/tui/mock.go`
- `/Users/daniel/projects/starSling/internal/tui/app.go`
- `/Users/daniel/projects/starSling/internal/tui/view_live.go`

---

### 9.3 对应条目 3：左上新增 `BIDV/ASKV`，位置在 BID 左侧与 ASK 右侧

实施策略：

- `MarketRow` 增加 `BidVol` / `AskVol`。
- `convertMarketRows` 读取 `bid_vol1` / `ask_vol1`。
- 左上列顺序调整为：
  - `... CHG CHG% BIDV BID ASK ASKV VOL ...`
- 排序映射新增 `bidv` / `askv`。

涉及文件：

- `/Users/daniel/projects/starSling/internal/tui/mock.go`
- `/Users/daniel/projects/starSling/internal/tui/app.go`
- `/Users/daniel/projects/starSling/internal/tui/view_live.go`

---

### 9.4 对应条目 4：右下 turnover 差异重复计算问题

实施策略：

- `unusual_worker` 从“增量更新旧 map”改为“每轮重建 current map 后整体替换”：
  - 每轮先构建 `current_turnover_map`
  - 用 `prev_turnover_map` 对比计算 `chg/ratio`
  - 成功发送后 `prev_turnover_map = current_turnover_map`
- 同轮同合约去重，只计算一次。
- 保持“RPC 失败不提交状态”语义，防止 seq 倒退和重复告警。

涉及文件：

- `/Users/daniel/projects/starSling/internal/live/unusual_worker.py`

---

### 9.5 对应条目 5：右下新增 OI 变化（CHG/RATIO，TAG=OI）

实施策略：

- `unusual_worker` 新增 `prev_oi_map` 并计算：
  - `oi_chg = current_oi - prev_oi`
  - `oi_ratio = current_oi / prev_oi - 1`（`prev_oi > 0`）
- 满足 OI 条件时输出事件并标记：
  - `tag = "OI"`
- 事件统一携带：
  - `turnover_chg/turnover_ratio`
  - `oi_chg/oi_ratio`
  - 右下根据 `tag` 选择显示 CHG/RATIO 值。

涉及文件：

- `/Users/daniel/projects/starSling/internal/live/unusual_worker.py`
- `/Users/daniel/projects/starSling/internal/tui/app.go`

---

### 9.6 对应条目 6：新增 OI Ratio 筛选，并与 turnover 筛选解耦

实施策略：

- UI state 扩展：
  - 新增 `oi_ratio_threshold`（默认 0.05）
- 阈值生效规则拆分：
  - `tag=TURNOVER` 仅使用 `turnover_chg_threshold + turnover_ratio_threshold`
  - `tag=OI` 仅使用 `oi_ratio_threshold`
- 右下 Enter 阈值窗口增加第三项输入：`OI Ratio >=`
- 非法输入按现有风格回退默认值并提示。

涉及文件：

- `/Users/daniel/projects/starSling/internal/router/types.go`
- `/Users/daniel/projects/starSling/internal/router/state.go`
- `/Users/daniel/projects/starSling/internal/router/server.go`
- `/Users/daniel/projects/starSling/internal/live/unusual_worker.py`
- `/Users/daniel/projects/starSling/internal/tui/app.go`

---

### 9.7 对应条目 7：语音播报改为队列串行，避免重叠

实施策略：

- 在 TUI 内新增单消费者播报队列：
  - 生产者：`maybeSpeakQuotes` 仅 enqueue
  - 消费者：独立 goroutine 串行执行 `speak`
- 队列长度设置上限（例如 64），超限做丢弃策略（建议丢弃最旧）。
- 保留现有播报触发规则：
  - 价格变化触发
  - 每合约 30s 最小间隔
  - 命令缺失一次告警后禁用

涉及文件：

- `/Users/daniel/projects/starSling/internal/tui/app.go`

---

### 9.8 对应条目 8：左下异常成交聚合分析面板（Flow Aggregation Panel）

实施策略：

- 左下区域由 `Runtime log` 改为聚合表格 `Flow Aggregation`。
- 数据源为右下异常事件流（`unusual.snapshot`），在 UI 层维护：
  - 时间窗口 deque（默认 120s）
  - 最小分析窗口（默认 30s）
  - option->underlying->symbol 映射字典
- 刷新流程：
  - 追加新事件 -> 清理过期事件 -> 最小窗口检查 -> 聚合计算 -> 刷新表格
- 聚合维度：
  - 行：`symbol + underlying`
  - 分组：`ITM` / `OTM`
- 指标：
  - `TOTAL_TURNOVER_SUM`
  - `ITM`（turnover 加权 moneyness）
  - `ITM_TURNOVER_SUM`
  - `ITM_OI_CHG_SUM`
  - `OTM`（turnover 加权 moneyness）
  - `OTM_TURNOVER_SUM`
  - `OTM_OI_CHG_SUM`
- 表格上方显示当前样本区间：`min_time ~ max_time`。
- 聚合表格 Enter 打开条件窗口：
  - 排序列选择 + ASC/DESC
  - `window_size` [60,300]
  - `min_analysis_window` [15,60]
  - 非法值回滚默认值，仅窗口提示不写 log

涉及文件：

- `/Users/daniel/projects/starSling/internal/tui/view_live.go`
- `/Users/daniel/projects/starSling/internal/tui/app.go`
- `/Users/daniel/projects/starSling/internal/tui/mock.go`

---

## 10. 分阶段执行顺序（建议）

为降低回归风险，建议按以下顺序落地：

- Phase A（数据正确性先行）：
  - 4 / 5 / 6
- Phase B（左上显示与筛选）：
  - 1 / 2 / 3
- Phase C（交互与分析）：
  - 7 / 8

当前进度：

- [x] Phase A（4 / 5 / 6）已完成开发与自动化测试
- [x] Phase B（1 / 2 / 3）已完成开发与自动化测试
- [x] Phase C（7 / 8）已完成开发与自动化测试

---

## 11. 测试与验收清单（执行后）

- [x] `go test ./internal/tui`
- [x] `go test ./internal/router`
- [x] `go test ./...`
- [x] `python3 -m py_compile /Users/daniel/projects/starSling/internal/live/unusual_worker.py`
- [ ] 手工验证：
  - [ ] 左上新增列与排序字段范围正确
  - [ ] 右下 TURNOVER/OI 双标签与阈值分流正确
  - [ ] 语音多合约播报无重叠
  - [ ] 左下聚合面板在窗口变化下稳定、可排序
