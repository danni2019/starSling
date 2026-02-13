# Roadmap

## Milestone 0: Core Foundation (Done)
- Go 主程序、终端 UI 框架、配置管理、Python runtime bootstrap 已落地。
- 本地 JSON-RPC 通信与路由服务（router）已落地。
- Metadata 拉取、缓存与交易时段基础能力已落地。

## Milestone 1: Realtime Analytics MVP (Done)
- 实时行情链路（`live_md -> router -> TUI`）已打通。
- 期权分析 worker（IV/Greeks/Curve）与异常成交 worker 已接入。
- 多面板联动、筛选/阈值/焦点控制、Flow 聚合分析已可用。

## Milestone 2: Data Persistence & Replay (Next)
- 新增快照落盘能力（market/options/unusual/flow）并按交易日分区。
- 建立回放模式（replay）驱动 router 与 UI 复盘。
- 形成盘中与盘后一致的数据基线，支持可复现实验。

## Milestone 3: Research & Signal Evaluation
- 建立统一信号特征层（term structure、skew、flow imbalance 等）。
- 建立事件研究与样本外评估流程。
- 建立参数版本化与实验记录机制。

## Milestone 4: Risk & Portfolio Layer
- 增加持仓、Greeks 暴露、PnL 归因看板。
- 增加风险限额与越界告警机制。
- 增加场景压力测试（价格、波动率、期限结构冲击）。

## Milestone 5: Paper Trading & Execution
- 实现 paper trading 订单生命周期与成交回报。
- 建立执行质量指标（滑点、成交偏离、冲击成本）。
- 建立策略上线门禁与回滚机制。

## Milestone 6: Productionization
- 加强可观测性（日志、指标、健康检查、报警）。
- 完善容错恢复与发布流程（灰度、回滚、版本管理）。
- 构建稳定的研究-风控-执行-复盘闭环。

## Guiding Principle
- 先研究与可复现，再风控与组合，最后执行自动化。
