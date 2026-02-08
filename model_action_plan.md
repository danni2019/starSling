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
