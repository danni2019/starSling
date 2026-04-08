# Roadmap

`starSling` 已进入公开仓库阶段，但整体仍处于 `MVP / prerelease`。

本路线图描述的是当前优先级和结构化演进方向，不代表对外承诺的交付时间。

## Current State

- GitHub 仓库已公开
- `BSD-3-Clause` 许可证、社区文件和发布基线已建立
- 当前主线能力已覆盖：
  - 实时行情接入
  - symbol-level overview
  - options / curve / unusual 分析
  - arbitrage monitor
  - 配置持久化、runtime bootstrap 与 `doctor` 自检

## Near-Term Priorities

### 1. Public Prerelease Hardening

- 完成首个正式 `macOS prerelease` tag 与 release notes
- 将 `doctor` 接入更稳定的本地 / CI 验证例程
- 在干净环境中完成 runtime bootstrap 的端到端复核
- 持续保持公开仓库文档、community profile 与发布资产契约一致

### 2. Realtime Analytics Hardening

- 继续提升实时行情链路的稳定性、重连与可观测性
- 强化 symbol-level 总览、期限结构、异常成交与风险暴露视图
- 优化筛选、焦点联动、阈值配置和盘中可读性

### 3. Account Monitoring And Risk

- 接入账户、持仓、委托、成交等实时数据流
- 增加账户监控与风险暴露看板
- 建立交易前校验、限额和越界告警

### 4. Execution Assist And Control

- 增加执行观察、撤改单与状态跟踪面板
- 补齐半自动执行工作流
- 明确执行前校验和人工确认边界

### 5. Automated Trading And Productionization

- 引入实时策略执行与任务调度能力
- 增加自动化安全机制、降级与人工接管路径
- 完善日志、指标、健康检查、版本管理与回滚流程

## Persistent Design Direction

- 实时优先：先把盘中观察与分析做深，再扩展到账户和执行
- latest-snapshot 优先：避免链路堆积，强调最新状态一致性
- 本地可验证：配置、runtime、发布路径都应有可执行的检查入口

## Non-Goals For The Current Stage

- 不是通用量化研究平台
- 不是历史回放 / 回测框架
- 不是已经完成交易执行闭环的生产系统
