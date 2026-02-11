<file name=0 path=/Users/daniel/projects/starSling/model_action_plan.md># 置顶需求（必须满足，按此执行）

> 右下异常流监测到某个期权合约在当前帧与前一帧之间出现异常（成交额变化或持仓变化）
> -> 仅把该异常作为触发器（trigger）
> -> 对该期权合约做“当前帧 + 前一帧”的全字段快照分析（不是只看 chg/ratio）
> -> 形成该期权合约本次事件的交易意图汇总
> -> 在分析窗口内，结合同一 underlying 下所有期权合约的事件意图与 Greeks 做组合
> -> 输出 underlying 维度的最终汇总意图。

补充约束：
- 不再是 turnover-only，OI 必须纳入聚合分析。
- 右下 unusual 流只负责触发，不是最终分析字段全集。
- 左下可以完全重设计。
- 盘口分析是必选项，不是可选增强项。
- 必须纳入 `last` 与盘口中间价/盘口VWAP 的对比分析。

---

## 1. 业务语义硬约束（先定死）

- `turnover` 是累计成交额，按同一合约时间序列应单调不减。
- `turnover_chg = current_turnover - prev_turnover`，正常应 `>= 0`。
- `turnover_ratio = current_turnover/prev_turnover - 1`（`prev_turnover > 0`）；由于 `turnover` 单调不减，正常应 `>= 0`（若 <0 视为数据重置/异常帧）。
- `oi` 不是单调量，可升可降；`oi_chg` 允许正负。
- `oi_ratio = current_oi/prev_oi - 1（prev_oi > 0）允许正负；负值表示净减仓，属于正常语境。`
- 若出现 `turnover_chg < 0` 或 `turnover_ratio < 0`，按数据重置/异常帧处理：
  - 该条 trigger 仍可记录为异常日志
  - 但不进入强度计算主路径（防止错误放大）。

---

## 2. 架构落点（按现有项目）

落点：`/internal/tui/app.go` 左下聚合链路（本地 Go 聚合）。

保持不变：
- 不新增 router snapshot 类型。
- 不新增 worker。

需要重构：
- 左下表结构、聚合状态结构、触发事件处理流程。

---

## 3. 数据源与字段分层

### 3.1 Trigger 层（来自右下 unusual）

用途：只判定“何时需要做一次全快照分析”。

字段：
- `ts`, `time`, `ctp_contract`, `tag`
- `turnover_chg`, `turnover_ratio`, `oi_chg`, `oi_ratio`
- `price`, `volume`（trigger 时刻附带）

### 3.2 快照层（来自 market 当前帧/前一帧）

用途：做合约级“全方位分析”。

分析时读取该合约在 `curr_frame` 与 `prev_frame` 的完整可用字段：
- 价格/成交：`last`, `open`, `high`, `low`, `volume`, `turnover`, `average_price`
- 持仓：`open_interest`, `pre_open_interest`
- 盘口：`bid1`, `ask1`, `bid_vol1`, `ask_vol1`
- 边界/结算：`pre_settlement`, `pre_close`, `limit_up`, `limit_down`, `settlement`, `close`
- 合约元信息：`symbol`, `underlying`, `cp/option_type`, `strike`, `tte`, `expiry_date`, `status`, `exchange`

盘口派生字段（必算）：
- `mid_px = (bid1 + ask1) / 2`
- `book_vwap = (bid1*bid_vol1 + ask1*ask_vol1) / (bid_vol1 + ask_vol1 + eps)`
- `last_vs_mid = last - mid_px`
- `last_vs_book_vwap = last - book_vwap`

### 3.3 Greeks 层（来自已透传字段）

用途：把合约意图映射到 underlying 风险轴并组合。

字段：
- `iv`, `delta`, `gamma`, `theta`, `vega`

注意：可能为空，允许晚到升级。

---

## 4. 触发 -> 合约分析 -> Underlying 组合 的流程

### Step A: 异常触发

对于每条 unusual 事件（`TURNOVER` 或 `OI`）：
1. 做 event_id 去重。
2. 获取该合约 `curr_frame` 与 `prev_frame`。
3. 若缺前帧，降级分析（仅当前帧 + trigger 字段）。

### Step B: 合约级全字段分析（本次事件）

对同一 `ctp_contract` 计算特征组：

1) 价格行为特征
- `d_last = curr.last - prev.last`
- `ret_last = d_last / max(prev.last, eps)`
- `range_pos = (curr.last-curr.low)/max(curr.high-curr.low, eps)`

2) 流量特征
- `d_turnover = curr.turnover - prev.turnover`
- `d_volume = curr.volume - prev.volume`
- `d_oi = curr.open_interest - prev.open_interest`
- `turnover_ratio / oi_ratio（优先使用 trigger 给出的值；其中 turnover_ratio 正常应 >=0，oi_ratio 允许为负）`

一致性校验（建议实现）：若 trigger 的 chg/ratio 与 curr-prev 差分在符号或量级上显著不一致，则将 q_data 从 1 降级到 0.5，并在 OptionIntent 打一个 “INCONSISTENT_SNAPSHOT” 标记用于调试。

3) 流动性/盘口特征（必选）
- `spread_curr = curr.ask1 - curr.bid1`
- `spread_prev = prev.ask1 - prev.bid1`
- `d_spread = spread_curr - spread_prev`
- `depth_imbalance_curr = (curr.bid_vol1-curr.ask_vol1)/(curr.bid_vol1+curr.ask_vol1+eps)`
- `depth_imbalance_prev = (prev.bid_vol1-prev.ask_vol1)/(prev.bid_vol1+prev.ask_vol1+eps)`
- `d_depth_imbalance = depth_imbalance_curr - depth_imbalance_prev`
- `mid_curr = (curr.bid1 + curr.ask1)/2`
- `mid_prev = (prev.bid1 + prev.ask1)/2`
- `book_vwap_curr = (curr.bid1*curr.bid_vol1 + curr.ask1*curr.ask_vol1)/(curr.bid_vol1+curr.ask_vol1+eps)`
- `book_vwap_prev = (prev.bid1*prev.bid_vol1 + prev.ask1*prev.ask_vol1)/(prev.bid_vol1+prev.ask_vol1+eps)`
- `last_vs_mid_curr = curr.last - mid_curr`
- `last_vs_mid_prev = prev.last - mid_prev`
- `d_last_vs_mid = last_vs_mid_curr - last_vs_mid_prev`
- `last_vs_book_vwap_curr = curr.last - book_vwap_curr`
- `last_vs_book_vwap_prev = prev.last - book_vwap_prev`
- `d_last_vs_book_vwap = last_vs_book_vwap_curr - last_vs_book_vwap_prev`

盘口缺失降级规则（必须实现）：
- 若 `bid1/ask1` 缺失：`mid_*` 与 `spread_*` 标记为缺失，不计算对应子分数。
- 若 `bid_vol1/ask_vol1` 缺失：`book_vwap_*` 与 `depth_imbalance_*` 标记为缺失，不计算对应子分数。
- 缺失不会直接丢弃事件，但会降低 `confidence`（见 5.2 与 5.4）。

4) Greek 风险特征
- 当前 `delta/gamma/vega/theta/iv`
- `greeks_ready` 标记

5) 事件类型特征
- `trigger_type = TURNOVER | OI`
- `trigger_strength`（见 Step C 权重）

### Step C: 生成合约交易意图摘要（OptionIntent）

每个触发事件产出一条 `OptionIntent`，至少包含：
- 标识：`event_id`, `ts`, `ctp_contract`, `underlying`, `cp`
- 事件侧：`trigger_type`, `trigger_strength`
- 核心分数：
  - `direction_score`（方向）
  - `vol_score`（波动率偏好）
  - `gamma_score`（凸性偏好）
  - `theta_score`（时间价值偏好）
  - `position_score`（增减仓倾向，主要由 `d_oi`）
  - `orderbook_score`（盘口交易压力/价格位置）
- 解释字段：`d_last`, `d_turnover`, `d_volume`, `d_oi`, `ret_last`, `spread`, `depth_imbalance`
- 必含盘口解释字段：`last_vs_mid_curr`, `last_vs_book_vwap_curr`, `d_last_vs_mid`, `d_last_vs_book_vwap`, `d_spread`, `d_depth_imbalance`
- Greeks：`delta/gamma/vega/theta/iv`
- 轴级置信度：`confidence_direction/vol/gamma/theta/position`
- `confidence`

说明：
- 这些分数是“事件级解释分数”，不是最终 underlying 结论。
- `TURNOVER` 与 `OI` 都会生成 `OptionIntent` 并参与后续组合。

#### Step C.1 分数计算口径（必须闭合，避免双重计量）

原则：
- `direction_score/vol_score/gamma_score/theta_score/position_score/orderbook_score` 都是 **“事件级 signed intensity”**（取值约束在 [-1, +1]），只表达“更像买入/卖出该风险”的方向与强弱；
- **Greeks 数值只在 Step D 做映射**（乘以 delta/vega/gamma/theta），避免在 Step C 内重复乘 Greek。

通用工具函数：
- `sgn(x)`: x>0 → +1；x<0 → -1；否则 0
- `clip(x, a, b)`
- `tanh(x)`
- `norm_by(x, scale) = tanh(x / max(scale, eps))`（把任意实数压到 (-1,1)）

(1) orderbook_score（盘口压力/成交位置，优先级最高）
- 目的：用 `last` 相对 `mid/book_vwap` 的位置与变化来推断更像“打 bid”还是“打 ask”。
- 定义两个子分数：
  - `ob_loc = 0.6*norm_by(last_vs_mid_curr, max(spread_curr, eps)) + 0.4*norm_by(last_vs_book_vwap_curr, max(spread_curr, eps))`
  - `ob_chg = 0.6*norm_by(d_last_vs_mid, max(abs(spread_curr)+abs(spread_prev), eps)) + 0.4*norm_by(d_last_vs_book_vwap, max(abs(spread_curr)+abs(spread_prev), eps))`
- 合成：`orderbook_score = clip(0.6*ob_loc + 0.4*ob_chg, -1, 1)`

解释：
- `orderbook_score > 0`：last 更偏 ask 一侧/向上穿越 mid/vwap，倾向“主动买”
- `orderbook_score < 0`：last 更偏 bid 一侧/向下穿越 mid/vwap，倾向“主动卖”

(2) direction_score（方向，受盘口与价格行为共同驱动）
- 目的：表达“更像买入/卖出该期权合约的方向风险”。
- 价格动量子分数：`px_mom = 0.7*norm_by(ret_last, 0.003) + 0.3*norm_by(range_pos-0.5, 0.25)`
- 成交流量子分数（只用差分，不用累计）：
  - 若 `d_turnover` 可用且 >0：`flow = norm_by(d_turnover, max(prev.turnover, eps))`
  - 否则若 `d_volume` 可用且 !=0：`flow = norm_by(d_volume, max(prev.volume, eps))`
  - 否则：`flow = 0`
- 合成：
  - `direction_score = clip(0.55*orderbook_score + 0.25*px_mom + 0.20*flow, -1, 1)`

(3) position_score（增减仓倾向，只表达 BUILD/REDUCE，不表达多空）
- 目的：只表达“新增头寸 vs 减少头寸”的方向，不推断开多/开空。
- 定义：`position_score = clip(norm_by(d_oi, max(0.5*(abs(curr.open_interest)+abs(prev.open_interest)), 1)), -1, 1)`
- 解释：`>0` BUILD（净增仓），`<0` REDUCE（净减仓）

(4) vol_score / gamma_score / theta_score（风险偏好轴）
- 在字段受限且难以识别复杂组合的情况下，采取“以 direction_score 为主、用 moneyness 与到期结构做分流”的可实现近似：

先定义结构标签（只用当前 greeks）：
- `abs_delta = abs(delta)`（若 delta 缺失则这些分数置 0）
- `otm_like = 1` 若 `abs_delta <= 0.35`；`itm_like = 1` 若 `abs_delta >= 0.65`；否则为 0
- `short_tte = 1` 若 `tte <= 7`（单位为天）；`mid_tte = 1` 若 7<tte<=30；否则 long

(a) gamma_score（凸性/尾部偏好）
- 直觉：OTM + 短到期 的买入更像买 gamma/尾部；同结构的卖出更像卖 gamma。
- 定义（delta/gamma 缺失则为 0）：
  - `gamma_score = clip(direction_score * (0.7*otm_like + 0.3*short_tte), -1, 1)`

(b) vol_score（波动率偏好）
- 直觉：在无法识别 delta-hedge 时，long gamma 往往伴随更强的 long vol 倾向；ITM 替代更弱。
- 定义：
  - `vol_score = clip(direction_score * (0.6*otm_like + 0.2*short_tte + 0.2*(1-itm_like)), -1, 1)`

(c) theta_score（时间价值偏好）
- 直觉：买入期权更像付 theta（偏负），卖出更像收 theta（偏正）。
- 定义：
  - `theta_score = clip(-direction_score, -1, 1)`

备注：
- 以上三轴是“可实现且可解释”的近似；后续若增加价差/组合识别，再把三轴从近似替换为结构识别结果。

### Step D: 窗口内 Underlying 组合（最终意图）

对每个 underlying 维护窗口（沿用 UI 参数）：
- `window_sec`: 默认 `120s`，范围 `[60s, 300s]`
- `min_span_sec`: 默认 `30s`，范围 `[15s, 60s]`

窗口内把所有 `OptionIntent` 组合为：
- `under_direction = sum(weight_direction * direction_score * delta)`
- `under_vol = sum(weight_vol * vol_score * vega)`
- `under_gamma = sum(weight_gamma * gamma_score * gamma)`
- `under_theta = sum(weight_theta * theta_score * theta)`
- `under_position = sum(weight_position * position_score)`

输出 underlying 汇总意图：
- `direction_intent`（BULL/BEAR/NEUTRAL）
- `vol_intent`（LONG_VOL/SHORT_VOL/NEUTRAL）
- `gamma_intent`（LONG_GAMMA/SHORT_GAMMA/NEUTRAL）
- `carry_intent`（THETA+ / THETA- / NEUTRAL）
- `position_intent`（BUILD/REDUCE/MIXED）
- `confidence_direction/vol/gamma/theta/position`
- `confidence`

---

## 5. 权重与分数口径（统一）

### 5.1 触发强度（包含 TURNOVER 与 OI）

- `w_turn = max(turnover_chg, 0)`
- `w_oi = max(abs(oi_chg) * max(price, curr.last, 0), 0)`
- `w_trigger = w_turn + w_oi`

说明：
- TURNOVER 异常通常 `w_turn` 主导。
- OI 异常通常 `w_oi` 主导。
- 两者并存时共同增强。

### 5.2 质量因子

- q_turn = clamp(turnover_ratio/5, 0, 1)（仅当 w_turn > 0 时参与；缺失不惩罚；若 turnover_ratio < 0 则视为异常帧，不进入强度主路径）
- q_oi = clamp(abs(oi_ratio)/5, 0, 1)（仅当 w_oi > 0 时参与；缺失不惩罚；oi_ratio 允许为负，质量取绝对强度）
- `q_data`：关键字段齐全为 `1`；一致性存疑降级到 `0.5`；若该轴关键字段缺失到不可计算，则该项记为 unavailable（不进入 `U_axis`）
- `q_book`：盘口字段齐全且质量正常为 `1`；可算但质量较弱可降级到 `0.5`；若字段缺失导致不可算，则该项记为 unavailable（不进入 `U_axis`）
- 轴级可用性 gating（替代 `q_greek`）：
  - `g_direction = 1` 若 `direction_score` 与 `delta` 都有效，否则 `0`
  - `g_vol = 1` 若 `vol_score` 与 `vega` 都有效，否则 `0`
  - `g_gamma = 1` 若 `gamma_score` 与 `gamma` 都有效，否则 `0`
  - `g_theta = 1` 若 `theta_score` 与 `theta` 都有效，否则 `0`
  - `g_position = 1` 若 `position_score` 有效，否则 `0`

### 5.3 综合权重

- `w_turn_eff = w_turn`（当 `q_turn` 可用）否则 `0`
- `w_oi_eff = w_oi`（当 `q_oi` 可用）否则 `0`
- `q_trig = (w_turn_eff/(w_turn_eff+w_oi_eff+eps))*q_turn + (w_oi_eff/(w_turn_eff+w_oi_eff+eps))*q_oi`
- 主意图保持不变：`weight_axis = w_trigger * q_axis * g_axis`
- 各轴展开：
  - `weight_direction = w_trigger * q_direction * g_direction`
  - `weight_vol = w_trigger * q_vol * g_vol`
  - `weight_gamma = w_trigger * q_gamma * g_gamma`
  - `weight_theta = w_trigger * q_theta * g_theta`
  - `weight_position = w_trigger * q_position * g_position`
- 其中 `q_axis` 按“最终可用项”重分配，并加入 cap 防止单项 dominance：
  1) 设基础质量项 `Q = {trig, data, book}`，基础系数分别为 `alpha = {0.6, 0.2, 0.2}`，对应质量值 `{q_trig, q_data, q_book}`。
  2) 定义可用项矩阵（固定口径，避免实现漂移）：
     - `U_direction_base = {trig, data, book}`
     - `U_vol_base = {trig, data, book}`
     - `U_gamma_base = {trig, data, book}`
     - `U_theta_base = {trig, data, book}`
     - `U_position_base = {trig, data}`（position 轴默认不使用 book 项）
  3) 对每个轴 `axis`，仅保留最终可用于该轴聚合的项集合 `U_axis = U_axis_base ∩ avail_items`：
     - `avail_trig=1`：至少一个触发通道有效且对应 ratio 可计算
     - `avail_data=1`：该轴所需关键快照字段可计算
     - `avail_book=1`：`bid1/ask1/bid_vol1/ask_vol1` 双帧齐全
  4) 轴级 cap 支持按轴配置，但默认统一，并仅放宽少数轴：
     - `cap_quality_default = 0.70`
     - `cap_quality_axis = cap_quality_override[axis]`（若无覆盖则取默认）
     - MVP 建议仅一个放宽项：`cap_quality_override = {position: 0.80}`（其余轴沿用 `0.70`）
  5) 先做可用项重分配：`p_i = alpha_i / max(sum(alpha_j, j in U_axis), eps)`。
  6) 再做单项上限：`p_i_cap = min(p_i, cap_quality_axis)`。
  7) cap 后归一化：`p_i_norm = p_i_cap / max(sum(p_k_cap, k in U_axis), eps)`。
  8) 得到轴级质量：`q_axis = sum(p_i_norm * q_i, i in U_axis)`。
- 若 `U_axis` 为空或 `g_axis=0`，则该轴 `weight_axis=0`。

说明：q_trig 只对“实际触发的那部分强度”计入质量，不会因另一通道缺失 ratio 而被动降权。

说明：turnover_ratio 由于单调性应为非负，负值视为异常并在上游已排除；oi_ratio 允许为负，q_oi 使用绝对值只衡量强度。

说明：Step C 的各类 score 均为 [-1,+1] 的事件级 signed intensity；Greeks 的量纲只在 Step D 通过乘以 delta/vega/gamma/theta 进入聚合，避免重复计量。

### 5.4 置信度口径（轴级 + 总分）

- 事件级轴置信度：`conf_axis_event = q_axis * g_axis`（范围 `[0,1]`）
- underlying 窗口轴置信度（按贡献绝对值加权）：
  - `contrib_direction_i = weight_direction_i * direction_score_i * delta_i`
  - `contrib_vol_i = weight_vol_i * vol_score_i * vega_i`
  - `contrib_gamma_i = weight_gamma_i * gamma_score_i * gamma_i`
  - `contrib_theta_i = weight_theta_i * theta_score_i * theta_i`
  - `contrib_position_i = weight_position_i * position_score_i`
  - `confidence_axis = sum(abs(contrib_axis_i) * conf_axis_event_i) / max(sum(abs(contrib_axis_i)), eps)`
- 总置信度：
  - `A_conf = {axis | confidence_axis > 0}`
  - `confidence = mean(confidence_axis, axis in A_conf)`；若 `A_conf` 为空则为 `0`

---

## 6. event_id 与去重/升级规则（严格）

### 6.1 归一化

- `norm_text`: trim+lower，空 -> `~`
- `norm_num`: nil/NaN/Inf -> `~`，否则 round(6) 固定 6 位
- `norm_ts`: 优先 `ts`，否则解析 `time/datetime`，失败为 `0`

### 6.2 event_id v2

`event_id = join("|",`
`"v2",`
`norm_text(ctp_contract),`
`norm_text(tag),`
`norm_ts(row),`
`norm_text(cp),`
`norm_num(strike),`
`norm_num(price),`
`norm_num(turnover_chg),`
`norm_num(oi_chg)`
`)`

说明：event_id 不包含累计型字段（如累计 volume），避免滚动历史重发时该字段漂移导致去重失效。

### 6.3 去重

- `seen[event_id]` 存：`event_ts_ms`, `greeks_ready`, `intent_ref`
- 默认同 id 跳过，不重复累计。

### 6.4 回收

- 按事件时间回收：
  - `cutoff = max_event_ts - window_sec*1000 - 5000`
  - `event_ts < cutoff` 的 seen 条目回收
- 不使用纯墙钟 TTL。

### 6.5 Greeks 晚到升级

若同一 `event_id` 首次 `greeks_ready=false`，后续重发变为 `true`：
1. 重算该事件 `OptionIntent`
2. 用新旧差值修正 underlying 累计
3. 更新 `seen[event_id]`
4. 不新增事件计数

---

## 7. 左下输出（重构后建议）

每个 underlying 一行：
- `UNDERLYING`
- `DIRECTION`
- `VOL`
- `GAMMA`
- `THETA`
- `POSITION`
- `CONFIDENCE`
- `PATTERN_HINT`
- `TOP_CONTRACTS`

补充：
- 左下仅展示结果字段；驱动与轴级诊断（如 `TURNOVER_IMPACT/OI_IMPACT/MIX_RATIO/CONF_*`）保留到详情面板或调试视图。
- 左下 Enter 呼出的筛选窗口保留两个尺度参数输入：
- `window_sec`（尺度观测窗口）默认 `120s`，范围 `[60s, 300s]`
- `min_span_sec`（最小可分析尺度）默认 `30s`，范围 `[15s, 60s]`

---

## 8. 实施顺序（编码顺序）

1. 在左下聚合状态中加入双帧缓存：`prev_frame_by_contract` / `curr_frame_by_contract`。
2. 重构 trigger ingest：右下事件仅触发，不直接当最终分析值。
3. 实现合约级全字段特征提取（curr vs prev）。
4. 实现 `OptionIntent` 生成与轴级 `weight_*` 计算（TURNOVER + OI 同时纳入，含可用项重分配与 cap）。
5. 实现 underlying 窗口组合（direction/vol/gamma/theta/position 五轴）。
6. 实现轴级 `confidence_*` 与总 `confidence` 汇总。
7. 实现 `event_id v2`、去重、按事件时间回收。
8. 实现盘口必选特征提取（含 `last-vs-mid`、`last-vs-book_vwap`）与缺失降级。
9. 实现 Greeks 晚到升级增量修正。
10. 重构左下表格字段与排序（`UNDERLYING/DIRECTION/VOL/GAMMA/THETA/POSITION/CONFIDENCE/PATTERN_HINT/TOP_CONTRACTS`）。
11. 保留左下 Enter 筛选窗口的 `window_sec/min_span_sec` 输入项，并实现默认值与范围校验（`120/[60,300]`、`30/[15,60]`）。
12. 补全测试（判重、升级、双触发、窗口回收、五轴输出、盘口必选特征、轴级置信度、筛选参数边界）。

---

## 9. 验收标准

- TURNOVER 异常与 OI 异常都能触发分析，并都进入最终聚合。
- 单次事件分析使用该合约 curr/prev 全字段，不局限于 chg/ratio。
- 同一事件重发不重复计数；Greeks 晚到可升级修正。
- Underlying 汇总能输出五轴意图、轴级置信度（`confidence_*`）与总置信度（`confidence`）。
- 左下展示与新口径一致。
- 盘口分析必选：每次事件都产出并使用 `last_vs_mid` 与 `last_vs_book_vwap`（或按缺失规则降级）。

---

# Appendix A — Pattern Overlay（最小可行：Straddle / Strangle）

> 目的：在不改变主方案（Step A-D、权重、五轴聚合）的前提下，引入一个“高置信、低误报、可回退”的 **组合模式识别 overlay**，用于提升 `vol/gamma/theta` 轴的解释力，减少拆腿解释导致的漂移。
>
> 约束：overlay 是 **附加层**，仅在满足 gating 条件时生效；否则完全退回主方案的分流近似（`otm_like/itm_like + tte`）。

## A.1 插入位置（不改主方案）
在主方案的 Step D（窗口内 Underlying 组合）执行前，追加执行：
- `PatternOverlay.MatchAndNet(window_events)`：
  - 输入：窗口内（或最近 1–3 秒）新进入队列的 `OptionIntent` 列表（仅使用主方案已产出的字段）
  - 输出：
    1) `combo_intents[]`：识别出的组合事件（Straddle/Strangle）
    2) `netting_ops[]`：对原始腿的“净额修正操作”（delta 更新 underlying sums，而不是删除原事件）

实现方式：
- 不删除原事件，不重写主方案；
- 通过“差值修正”实现净额：
  - 对被识别为组合的两条腿 A/B：
    - 从 underlying 聚合中减去它们各自贡献的 `under_vol/under_gamma/under_theta`（仅这三轴；方向轴默认不动）
    - 再加上组合 intent 的三轴贡献
  - 组合识别失败或不满足 gating：不做任何修正。

> 注意：overlay 的净额修正应与 `event_id`/去重/Greeks 晚到升级机制兼容：
> - 组合本身也需要 `combo_id` 去重
> - 若腿的 Greeks 后续升级，需触发一次“重新匹配/重算差值”或直接忽略（MVP 建议忽略 re-match，仅对 greeks_ready=true 的腿参与组合）。

## A.2 MVP 仅识别：Straddle / Strangle
### 定义
- **Straddle**：同一 underlying、同一 expiry、同一事件时间邻域内（<= 2s），同时出现一条 Call 与一条 Put，且两腿 **moneyness 接近**（用 `abs(delta)` 接近替代）：
  - `abs(abs(delta_C) - abs(delta_P)) <= 0.10`（delta 缺失则不匹配）
- **Strangle**：同一 underlying、同一 expiry、同一时间邻域（<= 2s），一条 Call 一条 Put，但 `abs(delta)` 差异较大：
  - `abs(abs(delta_C) - abs(delta_P)) > 0.10`

### 同向性（必须）
- 两腿的 `direction_score` 必须同号且绝对值足够：
  - `sgn(direction_score_C) == sgn(direction_score_P)`
  - 且 `abs(direction_score_C) >= 0.4`、`abs(direction_score_P) >= 0.4`

解释：
- 同向买入 C+P → long vol / long gamma / pay theta
- 同向卖出 C+P → short vol / short gamma / receive theta

## A.3 Gating（高置信条件，避免误报）
仅当以下条件全部满足时才生成组合：
1) 同 `underlying`
2) expiry 相同（若主方案未显式提供 expiry，可用 `tte` 近似：`abs(tte_C - tte_P) <= 1` 天；但更推荐用 expiry）
3) 时间邻域：`abs(ts_C - ts_P) <= 2000ms`
4) 强度相近：`w_trigger`（或 `trigger_strength`）比值在 `[0.5, 2.0]`
5) 盘口质量：两腿 `q_book == 1`（或 `confidence` 中包含盘口齐全的条件）
6) Greeks 就绪：两腿 `greeks_ready == true`（MVP 强制要求；避免晚到升级带来的 re-match 复杂度）
7) 两腿 `weight_vol > 0`（确保 overlay 只在 vol 轴有效时参与）

不满足 gating：不做 overlay。

### A.3.1 配对冲突消解（单腿单组合，确定性）
- 约束：同一条腿最多参与一个组合，禁止一腿多配（避免 over-netting）。
- 步骤：
  1) 先生成所有通过 A.3 gating 的候选 `C-P` pair。
  2) 在本次匹配批次（同 underlying + expiry + 时间邻域）内，先做规模归一化：
     - `pair_size_raw = min(weight_vol_A, weight_vol_B)`
     - `size_ref = p95({pair_size_raw of all candidates in batch})`（防极值）
     - `size_score = clip(pair_size_raw / max(size_ref, eps), 0, 1)`
  3) 计算每个候选的 `pair_score`：
     - `balance_score = clip(1 - abs(log((w_trigger_A+eps)/(w_trigger_B+eps)))/log(2), 0, 1)`
     - `time_score = clip(1 - abs(ts_A-ts_B)/2000, 0, 1)`
     - `pair_score = size_score * balance_score * time_score`
  4) 按 `pair_score` 从高到低排序，贪心遍历。
  5) 若某候选的任一腿已被更高分 pair 占用，则跳过；否则锁定该 pair。
  6) 平分时的确定性 tie-break：先选 `abs(ts_A-ts_B)` 更小者；再按 `(event_id_A, event_id_B)` 字典序。
- 结果：若单腿有多个候选，只会保留 `pair_score` 最大的那一对，且结果可复现。

## A.4 组合 intent 的生成（不改主方案的 score 定义）
对 A.3.1 锁定后的组合（两腿 A/B），生成一个 `ComboIntent`：
- `combo_id`：`join("|", "combo_v1", underlying, expiry_date_or_tte_bucket, min(tsA,tsB), max(tsA,tsB), "C+P")`
- 其中 expiry_date_or_tte_bucket：优先用两腿的 expiry_date（相同才匹配）；若缺失则用 tte 生成短/中/长桶作为近似。
- `combo_type`：`STRADDLE` 或 `STRANGLE`
- `combo_strength`：`min(trigger_strength_A, trigger_strength_B)`（保守）
- 组合使用轴级权重（与主方案 Step D 一致）：
  - `combo_weight_vol = min(weight_vol_A, weight_vol_B)`（保守）
  - `combo_weight_gamma = min(weight_gamma_A, weight_gamma_B)`（保守）
  - `combo_weight_theta = min(weight_theta_A, weight_theta_B)`（保守）

组合的三轴 score（只输出事件级 signed intensity，仍在 [-1,+1]）：
- `combo_direction_score = 0`（组合本质方向中性；方向轴不由 overlay 主导）
- `combo_vol_score = sgn(direction_score_A)`（买为 +，卖为 -）
- `combo_gamma_score = sgn(direction_score_A)`
- `combo_theta_score = -sgn(direction_score_A)`（买付 theta，卖收 theta）

组合的 Greeks：
- `combo_vega = vega_A + vega_B`
- `combo_gamma = gamma_A + gamma_B`
- `combo_theta = theta_A + theta_B`
- `combo_delta = delta_A + delta_B`（通常接近 0；但不用于 overlay 的方向轴）

## A.5 净额修正（只修正 vol/gamma/theta 三轴）
对 underlying 窗口内的累积量（主方案 Step D 的 sums）执行差值修正：

1) 计算两腿原始贡献（主方案公式）：
- `leg_vol = weight_vol * vol_score * vega`
- `leg_gamma = weight_gamma * gamma_score * gamma`
- `leg_theta = weight_theta * theta_score * theta`

2) 计算组合贡献（同主方案映射口径）：
- `combo_vol = combo_weight_vol * combo_vol_score * combo_vega`
- `combo_gamma = combo_weight_gamma * combo_gamma_score * combo_gamma`
- `combo_theta = combo_weight_theta * combo_theta_score * combo_theta`

3) 对 sums 做：
- `under_vol += combo_vol - (leg_vol_A + leg_vol_B)`
- `under_gamma += combo_gamma - (leg_gamma_A + leg_gamma_B)`
- `under_theta += combo_theta - (leg_theta_A + leg_theta_B)`

> 方向轴默认不改：
> - `under_direction` 仍由主方案腿级贡献决定（避免 overlay 改变核心方向判断的稳定性）。

## A.6 与去重/回收/升级的交互（MVP 简化规则）
- `combo_id` 也需要 `seen_combo` 去重，回收规则与 `seen[event_id]` 同步（按事件时间回收）。
- MVP 要求两腿 greeks_ready=true 才匹配，因此：
  - 若腿后续 greeks 才变 true：允许在后续重发时首次形成组合（一次性 overlay）。
  - 若腿 greeks 数值后续漂移：MVP 不 re-match、不反复修正（只在首次匹配时锁定）。

## A.7 UI 展示建议（可选）
在左下 underlying 行增加一个小字段（不影响主指标）：
- `PATTERN_HINT`：例如 `STRADDLE×2 / STRANGLE×1`（窗口内 overlay 命中的计数）
用于解释为何 `vol/gamma/theta` 轴出现跳变。

## A.8 验收标准（overlay）
- 在明确出现同 expiry 同时段的 C+P 同向大额事件时：
  - overlay 能输出 `PATTERN_HINT`
  - 且 `under_vol/under_gamma/under_theta` 的解释更接近“纯 vol/gamma 意图”而非拆腿分流结果
- overlay 不应显著增加误报：
  - gating 不满足时完全不生效
  - 同一条腿最多参与一个组合（无一腿多配导致的 over-netting）
  - 命中率不追求高，优先低误报与可解释。

---

## 10. 具体执行步骤 Checklist（待审批）

### 10.1 审批前冻结项（必须确认）
- [x] 冻结左下 Enter 筛选参数：`window_sec=120s`（`[60,300]`），`min_span_sec=30s`（`[15,60]`）。
- [x] 冻结轴级质量 cap：`cap_quality_default=0.70`，仅 `position=0.80` 放宽。
- [x] 冻结 overlay 配对规则：单腿仅参与一个组合，按 `pair_score` 贪心锁定。
- [x] 确认 `TOP_CONTRACTS` 展示规则：默认 `top1,top2` 形式，按 `abs(net_contrib)` 降序。
- [x] 确认 `INCONSISTENT_SNAPSHOT` 判定阈值（建议：符号不一致直接触发；量级偏差 `>50%` 触发）。
- [x] 确认五轴 intent 文本阈值（建议：`abs(axis_value) < neutral_th` 判 `NEUTRAL`，默认 `neutral_th=0.15`）。

### 10.2 实施阶段 1：数据与状态改造
- [x] 在 `/internal/tui/app.go` 增加 `curr_frame_by_contract/prev_frame_by_contract` 双帧缓存。
- [x] 重构 unusual ingest：仅触发分析，不直接作为最终输出字段。
- [x] 落地 `event_id v2` 归一化与构造函数（含 `norm_text/norm_num/norm_ts`）。
- [x] 落地 `seen[event_id]` 去重结构（含 `event_ts_ms` 与 `greeks_ready` 升级路径）。
- [x] 实现按事件时间回收（`cutoff = max_event_ts - window_sec*1000 - 5000`）。

### 10.3 实施阶段 2：特征与评分
- [x] 实现 Step B 全字段特征提取（价格/流量/盘口/Greeks）。
- [x] 实现盘口必选特征与缺失降级（`last_vs_mid/last_vs_book_vwap/d_spread/d_depth_imbalance`）。
- [x] 实现 Step C 各评分函数：`orderbook_score/direction_score/position_score/vol/gamma/theta`。
- [x] 实现 `INCONSISTENT_SNAPSHOT` 判定与 `q_data` 降级路径（内部暴露）。

### 10.4 实施阶段 3：轴级权重与聚合
- [x] 实现 `w_turn/w_oi/w_trigger` 与 `q_turn/q_oi`。
- [x] 实现轴级 gating：`g_direction/g_vol/g_gamma/g_theta/g_position`。
- [x] 实现 `U_axis` 可用项矩阵与可用项重分配。
- [x] 实现轴级 cap 与归一化（`cap_quality_default + override`）。
- [x] 实现五轴聚合：`under_direction/under_vol/under_gamma/under_theta/under_position`。
- [x] 实现轴级置信度与总置信度：`confidence_*` 与 `confidence`。
- [x] 实现 Greeks 晚到升级增量修正（同 `event_id` 差值更新，不增计数）。

### 10.5 实施阶段 4：Overlay（Appendix A）
- [x] 实现 Straddle/Strangle 候选生成（同 underlying + expiry + 时间邻域 + 同向性）。
- [x] 实现 A.3 gating（含 `q_book==1`、`greeks_ready==true`、`weight_vol>0`）。
- [x] 实现 A.3.1 配对冲突消解（候选全集 -> 归一化 `pair_score` -> 贪心锁定 -> tie-break）。
- [x] 实现 `combo_id` 去重（匹配批次内）与窗口回收兼容。
- [x] 实现 A.5 净额修正（仅 `vol/gamma/theta` 三轴，按轴级权重）。
- [x] 实现 `PATTERN_HINT` 统计与输出（窗口内命中计数）。

### 10.6 实施阶段 5：左下 UI 与交互
- [x] 左下表格仅展示：`UNDERLYING/DIRECTION/VOL/GAMMA/THETA/POSITION/CONFIDENCE/PATTERN_HINT/TOP_CONTRACTS`。
- [x] 左下 Enter 筛选窗口保留 `window_sec/min_span_sec` 两个输入。
- [x] 实现输入默认值、范围校验与非法值回退。
- [x] 不在前端成品端展示详情/调试窗口；相关驱动与诊断字段仅保留内部开发暴露。

### 10.7 测试与验收门禁
- [x] 单元测试：event_id 归一化、去重、回收、Greeks 升级差值修正。
- [x] 单元测试：Step C 各评分函数边界与缺失降级。
- [x] 单元测试：轴级权重重分配、cap、生效与 `weight_axis=0` 分支。
- [x] 单元测试：overlay 候选、贪心配对唯一性、tie-break 确定性、净额修正一致性。
- [x] 集成测试：TURNOVER/OI 双触发与窗口聚合结果稳定性。
- [x] UI 测试：左下展示字段与 Enter 参数边界输入。
- [x] 验收对照：本文件第 9 节与 A.8 全量通过。

### 10.8 交付与回滚
- [ ] 在 PR 描述中附“参数冻结表 + 验收截图 + 核心测试结果”。
- [ ] 上线开关：支持按 underlying/全局关闭 overlay（回退主方案不影响主链路）。
- [ ] 发布后观察项：误报率、命中率、性能开销、left panel 稳定性。
