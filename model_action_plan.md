# Greeks/IV 价格回退最小方案

目标：只做一个最小改动，降低 ITM 合约 IV/Greeks 无效率。

## 改动范围
- 仅调整 IV 输入价格的回退顺序。
- 不改 `option_type` 逻辑，不引入其他新机制。

## 新规则
- 先用 `vwap` 作为 `option_price` 计算 IV/Greeks。
- 如果 `vwap` 计算结果无效（例如 `iv is None`），再用 `mid=(bid1+ask1)/2` 重试。
- 如果 `mid` 仍无效，再退回 `last` 重试。
- 三者都失败时，保持当前失败输出。

## 有效结果判定
- 以 IV 是否成功作为主判定：`iv != None` 视为有效。
- 若 IV 有效，则该价格来源即为最终 `price_for_iv`。

## 执行清单（Checklist）

- [ ] **Step 1：实现价格回退链路**
  - [ ] 在 `options_worker.py` 中按 `vwap -> mid -> last` 顺序尝试求解。
  - [ ] 每次尝试都调用同一套 `_compute_greeks(...)` 逻辑。

- [ ] **Step 2：补充结果来源字段**
  - [ ] 在输出行中记录最终采用的 `price_source`（`vwap`/`mid`/`last`）。
  - [ ] `price_for_iv` 保持为最终成功价格。

- [ ] **Step 3：最小验证**
  - [ ] 准备 3 组样例：`vwap` 可解、仅 `mid` 可解、仅 `last` 可解。
  - [ ] 确认回退顺序正确且最终结果符合预期。

## 验收标准
- 当 `vwap` 无效而 `mid` 或 `last` 可解时，不再直接得到 `iv=None`。
- 不改变现有 `option_type` 处理逻辑和其他计算路径。
# Batch-5 Plan (Curve Panel: Add Call/Put Skew)

## Goal

在右上区域（`VIX + forward curve`）新增两列：`CALL_SKEW`、`PUT_SKEW`，并按你定义的口径在 Python `options_worker` 计算后透传到 TUI。

## Current-State Check (Confirmed)

- 当前 VIX 计算逻辑位于 `/Users/daniel/projects/starSling/internal/live/options_worker.py` 的 `build_curve_snapshot`：
  - 先过滤 `iv`、`delta` 非空；
  - 再用 `abs(delta) <= 0.25` 的样本求均值；
  - **并不是** `abs(delta) in [0.25, 0.5]`。

## Target Calculation Rules

- call / put 两侧独立计算，互不干扰：
  - `atm_iv_side` = 该侧 `abs(delta) in [0.45, 0.55]` 的 `iv` 均值
  - `iv25_side` = 该侧 `abs(delta) in [0.2, 0.3]` 的 `iv` 均值
  - 若 `atm_iv_side` 或 `iv25_side` 任一为 None，则该侧 skew=None
  - `call_skew = call_iv25 - call_atm_iv`
  - `put_skew = put_iv25 - put_atm_iv`

## Execution Checklist

- [ ] Step 1: Extend curve snapshot payload in Python worker
  - [x] 在 `build_curve_snapshot` 为每个 `ctp_contract` 计算并写入 `call_skew`、`put_skew`
  - [x] 复用当前链路筛选（focus symbol + underlying 对齐）确保口径一致
  - [x] 缺值返回 `None`

- [ ] Step 2: Render new columns in right-top panel
  - [x] 在 `/Users/daniel/projects/starSling/internal/tui/app.go` 的 curve 表头新增 `CALL_SKEW`、`PUT_SKEW`
  - [x] 行渲染支持缺值显示 `-`
  - [x] 保持现有列宽和对齐方式

- [ ] Step 3: Tests
  - [x] `go test ./internal/tui`
  - [x] `go test ./...`
  - [x] `python3 -m py_compile /Users/daniel/projects/starSling/internal/live/options_worker.py`

- [ ] Step 4: Review Gate
  - [ ] 完成后先请你 code review
  - [ ] review 通过后再 commit

## Needs Confirmation (Before Coding)

- [x] VIX 口径是否也要同步改为 `abs(delta) in [0.25, 0.5]`？
  - 已确认并已实现：VIX 改为 `abs(delta) in [0.25, 0.5]`，同时新增 skew 两列。
