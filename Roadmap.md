# Roadmap

## Current Status (2026-02-23)
- 当前处于 `Milestone 1` 完成后的增强迭代阶段，下一阶段重点是继续强化实时行情处理与盘中分析能力。
- 已完成本轮 Live TUI / router 优化：
  - 左上 overview 改为按 `symbol` 合并的单表（与 watchlist 上下 50:50 分区）
  - 新增期权 `gamma inventory` 到期日分桶（`front/mid/back`，`C/P` 分列）
  - overview 筛选与排序支持 `Option Availability` 与 `oi_chg/turnover` 升降序
  - overview 数值格式统一（`OI_CHG%` 百分比，其余科学计数法）
  - 新增主菜单 `Settings` 与全局参数持久化（`risk_free_rate`、`days_in_year`、gamma bucket）
  - Live `Enter` 面板配置持久化（`settings_<panel>`）并在进入 Live 时自动恢复
  - 保存 Settings 后支持 runtime 生效（router gamma bucket 更新 + `options_worker` 定向重启）
  - Live 界面稳定性修复：异步启动、焦点/重绘去抖、终端日志污染导致的屏幕堆叠修复
  - 清理历史 TUI 原型/占位代码，降低维护和阅读成本
- 项目路线已明确聚焦实时能力：实时行情处理与分析 -> 账户实时监控 -> 实时自动交易。

## Milestone 0: Core Foundation (Done)
- Go 主程序、终端 UI 框架、配置管理、Python runtime bootstrap 已落地。
- 本地 JSON-RPC 通信与路由服务（router）已落地。
- Metadata 拉取、缓存与交易时段基础能力已落地。

## Milestone 1: Realtime Analytics MVP (Done)
- 实时行情链路（`live_md -> router -> TUI`）已打通。
- 期权分析 worker（IV/Greeks/Curve）与异常成交 worker 已接入。
- 多面板联动、筛选/阈值/焦点控制、Flow 聚合分析已可用。

## Milestone 2: Realtime Analytics Hardening (Next)
- 继续强化实时行情处理链路的稳定性、延迟与容错能力。
- 扩展盘中实时分析看板（symbol-level、期限结构、成交/持仓异动、期权风险暴露）。
- 增强实时筛选、联动与告警能力，服务盘中观察与决策。

## Milestone 3: Account Realtime Monitoring & Risk
- 接入账户/持仓/委托/成交实时数据流。
- 增加账户实时监控面板（持仓、PnL、Greeks 暴露、资金占用）。
- 增加风险限额与越界告警（仓位、Greeks、成交异常、撤单异常等）。

## Milestone 4: Execution Assist & Trading Control
- 增加实时交易辅助与执行控制面板（订单状态、撤改单、执行质量观察）。
- 建立交易前校验与风控门禁（参数、仓位、限价/滑点约束）。
- 支持半自动执行工作流（人工确认 + 程序化执行）。

## Milestone 5: Realtime Automated Trading
- 引入实时策略执行引擎与任务调度（以盘中实时信号为核心）。
- 打通订单生命周期、成交回报、持仓与风控反馈闭环。
- 增加自动交易安全机制（熔断、限额、故障降级、人工接管）。

## Milestone 6: Productionization
- 加强可观测性（日志、指标、健康检查、报警）。
- 完善容错恢复与发布流程（灰度、回滚、版本管理）。
- 构建稳定的实时分析-账户监控-交易执行闭环。

## Guiding Principle
- 实时优先：先做盘中行情处理与分析，再做账户实时监控与风控，最后推进自动化执行。
