# Roadmap

`starSling` 当前仍处于实时能力优先的 MVP / prerelease 阶段。

本路线图面向公开仓库读者，描述当前重点和大致演进顺序，不代表对交付时间的承诺。

## Current Focus

- 完成首个 `macOS` prerelease 的发布与验证闭环。
- 继续强化实时行情处理、实时分析和盘中监控体验。
- 保持项目边界清晰：先把实时观察与分析做好，再逐步扩展到账户监控、风控和执行控制。

## Roadmap

### 1. Public Prerelease Hardening

- 稳定 `macOS` prerelease 打包与发布流程。
- 完善公开仓库文档、发布说明和用户首次启动路径。
- 为 runtime/bootstrap 增加可验证的 smoke-test / self-check 路径。

### 2. Realtime Analytics Hardening

- 提升实时行情链路的稳定性、重连恢复和延迟表现。
- 持续增强盘中分析看板，包括 symbol-level 总览、期限结构、异常成交与风险暴露视角。
- 优化筛选、联动、阈值配置和告警体验。

### 3. Account Monitoring And Risk

- 接入账户、持仓、委托、成交等实时数据流。
- 增加账户监控面板，包括 PnL、Greeks 暴露、资金占用和限额告警。
- 明确交易前校验与风控门禁。

### 4. Execution Assist And Control

- 增加实时交易辅助与执行控制面板。
- 支持更完整的订单状态观察、撤改单操作和执行质量观察。
- 建立半自动执行工作流。

### 5. Automated Trading And Productionization

- 引入实时策略执行与任务调度能力。
- 补齐自动交易安全机制、故障降级和人工接管路径。
- 加强可观测性、发布管理、回滚和运行健康检查。

## Scope Notes

- 历史数据回放、回测和离线研究不是当前主线目标。
- 外部行情源、交易接入和依赖分发能力会影响具体交付顺序。
- 对生产、交易和商用场景的使用责任边界，仍以 `README`、`LICENSE` 与相关第三方条款为准。
