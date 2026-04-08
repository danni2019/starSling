# starSling

`starSling` 是一个面向中国期货/期权市场的终端实时监控、实时分析与交易辅助 MVP。

项目定位聚焦盘中实时能力：

- 以实时行情处理、实时分析、实时监控为核心主线
- 后续演进方向优先为账户实时监控与风险看板，再到实时自动交易
- 历史落盘/回放/回测不是当前项目主线目标（仅在明确必要时再评估）

当前版本已经打通以下闭环：

- 实时行情接入（OpenCTP -> Python `live_md`）
- 本地路由缓存与 JSON-RPC 通信（Go `router`）
- 终端 UI 多面板展示（市场/波动率曲线/期权链/异常成交/Flow 聚合）
- 左上 symbol-level overview 看板（期货热度 + 期权 gamma inventory 分桶）
- 两类实时派生分析 worker（`options_worker`、`unusual_worker`）
- UI 交互筛选、阈值配置、焦点合约联动
- `Settings` 全局参数配置与本地持久化（含实时生效链路）
- Live 面板稳定性修复（异步启动、焦点/渲染去抖、终端日志污染修复）

## 项目文档

- 公开路线图：`docs/project/roadmap.md`
- 公开化 / prerelease 门禁：`docs/release/public-readiness.md`
- `macOS` prerelease 验证流程：`docs/release/macos-prerelease.md`

## MVP 功能概览

### 1. 实时总览与市场面板（左上）

- 左上区域顶部新增 symbol-level overview 单表（按 `symbol` 合并展示）
- 期货汇总列：`OI_CHG%`、`TURNOVER`
- 期权汇总列：`C_GAMMA_INV / C_GAMMA_FNT / C_GAMMA_MID / C_GAMMA_BACK` 与 `P_*`
- `gamma inventory` 采用 `sum(gamma * open_interest * multiplier * S^2)`，并按到期日分桶：
  - `front <= 30d`
  - `mid (30, 90]d`
  - `back > 90d`
- overview `Enter` 设置支持：按 `oi_chg/turnover` 升降序排序、`Option Availability` 过滤
- overview 数值格式：`OI_CHG%` 保持百分比，其余数值统一科学计数法

- 展示期货主行情字段：`LAST/CHG/BID/ASK/VOL/TURNOVER/OI` 等
- 支持排序（升降序切换）
- 支持过滤：`exchange/product_class/symbol/contract`
- 支持主力合约模式（按品种选 OI 最大合约）

### 2. 期权与波动率分析（右侧）

- `options_worker.py`：
  - 从实时市场快照计算期权链 IV/Greeks（含 `py_vollib` 不可用时的 fallback）
  - 产出 options snapshot
  - 产出 forward + VIX/偏斜曲线 snapshot
- UI 支持按焦点合约/标的联动显示期权链
- UI 支持 Delta 区间过滤（按 strike 成组筛选）

### 3. 异常成交与 Flow 聚合（左下+右下）

- `unusual_worker.py`：基于 `turnover/open_interest` 的帧间变化识别异常
- UI 可动态调整异常阈值（成交额变化、成交额变化比例、OI 变化比例）
- Go 端 Flow 聚合引擎对异常事件做多维打分：
  - `Direction / Vol / Gamma / Theta / Position / Confidence`
  - pattern overlay（straddle/strangle 识别）
  - 支持窗口化统计与聚焦过滤

### 4. 运行时辅助能力

- 主菜单 `Settings`（全局参数）：
  - `risk_free_rate`（默认 `0.01`）
  - `days_in_year`（默认 `365`，范围 `1..370`）
  - overview gamma bucket 边界（默认 `30/90`）
- 全局参数保存后：
  - gamma bucket 立即通过 router RPC 生效
  - 若 Live 正在运行，仅重启 `options_worker` 载入新参数
- Live 各面板 `Enter` 配置项支持本地持久化（`settings_<panel>`），下次进入 Live 自动恢复默认项
- 配置管理（创建/编辑/删除/设为默认）
- 本地 Python 运行时引导脚本（`scripts/bootstrap_python.sh`）
- 语音播报（可选，依赖 `say`/`espeak`/`spd-say`）
- Metadata 拉取与缓存（合约、交易时段等）
- 已清理不参与当前构建的历史 TUI 原型/占位代码，降低维护成本

## 系统结构

```text
cmd/starsling (main)
  -> internal/tui (tview UI)
     -> internal/ipc (JSON-RPC client)
     -> internal/router (snapshot state + JSON-RPC server)
     -> internal/live/live_md.py (行情接入)
     -> internal/live/options_worker.py (IV/Greeks/Curve)
     -> internal/live/unusual_worker.py (异常成交)
```

数据链路为“最新快照驱动”：

1. `live_md.py` 推送 `market.snapshot`
2. `options_worker.py` / `unusual_worker.py` 拉取最新 market 并推送派生 snapshot
3. UI 定时拉取 `view_snapshot`，按 section 更新

## 快速开始

### 环境要求

- Go `1.25+`
- Bash、curl、tar（用于 Python runtime bootstrap）
- OpenCTP 可用环境（账号、行情前置、依赖包）

### 安装依赖并运行

```bash
go build ./cmd/starsling
./starsling
```

或直接运行：

```bash
go run ./cmd/starsling
```

### 初始化 Python runtime（推荐首次执行）

```bash
./scripts/bootstrap_python.sh
```

可选环境变量：

- `OPENCTP_WHEEL=/path/to/openctp.whl`
- `PIP_INDEX_URL=...`
- `PIP_EXTRA_INDEX_URL=...`
- `STARSLING_PYTHON_VERSION=3.11.x`

### 发布前 / 环境自检

```bash
go run ./cmd/starsling doctor
```

该命令会检查：

- 当前平台是否在 runtime 支持范围内
- `scripts/bootstrap_python.sh` 与 `config/metadata.sources.json` 是否可定位
- 默认 `live-md.host` / `live-md.port` 是否仍为未配置状态
- 配置目录与 metadata 目录解析是否正常

如果只看到 `bundled python` 警告，说明还没执行 bootstrap；这不会阻止 prerelease 检查通过，但会阻止后续直接进入 `Live market data`。

### 首次进入 Live 前需要先配置 Host / Port

- 仓库内置的 `live-md.host` 默认值为空，`live-md.port` 默认值为 `0`，发布包中不预设任何 front 地址。
- 进入应用后，请先在 `Config` 页面配置实际可用的 `Host` 和 `Port`。
- 未完成这两个字段配置时，UI 会阻止进入 `Live market data`。

## 运行与交互

主菜单：

- `Live market data`
- `Setup Python runtime`
- `Config`
- `Settings`
- `Quit`

Live 界面常用按键：

- `Tab / Shift+Tab`：切换焦点面板
- `← / →`：切换焦点面板（含 overview 区域）
- `Enter`：打开当前面板设置（过滤、阈值、语音等）
- `s`：切换市场排序方向
- `Esc`：返回主菜单
- `q`：退出

## 配置与数据目录

- 默认配置模板：`config/starsling.example.json`
- metadata 源：`config/metadata.sources.json`
- 用户配置目录（默认）：
  - macOS: `~/Library/Application Support/starsling/configs`
  - Linux: `${XDG_CONFIG_HOME:-~/.config}/starsling/configs`
- metadata 缓存目录（默认）：
  - macOS: `~/Library/Application Support/starsling/metadata`
  - Linux: `${XDG_CONFIG_HOME:-~/.config}/starsling/metadata`
- 全局设置与 Live 面板偏好（默认）：
  - `.../starsling/metadata/global_settings.json`
- 本地 runtime：`runtime/<platform>/...`

## 项目结构（当前）

- `cmd/starsling`：程序入口
- `internal/tui`：终端 UI 与交互逻辑
- `internal/router`：本地 JSON-RPC router 与状态缓存
- `internal/ipc`：JSON-RPC 帧协议与 client
- `internal/live`：Python worker 嵌入与进程控制
- `internal/metadata`：元数据刷新、缓存与交易时段解析
- `internal/config` / `internal/configstore`：配置定义与持久化
- `python`：Python 依赖与说明
- `scripts`：环境引导脚本

## 开发与测试

```bash
go test ./...
```

调试 UI runtime log 面板：

```bash
STARSLING_INTERNAL_DEBUG_UI=1 go run ./cmd/starsling
```

## 发布说明

- 当前发布规划优先为 `macOS` prerelease。
- 该 prerelease 的目标是先稳定分发主程序与运行时引导材料，不承诺所有外部行情依赖都能一键安装完成。
- 发布包会明确要求用户自行准备可用的 OpenCTP 环境或兼容安装来源。

## License

- 本项目代码采用 `BSD-3-Clause` 协议发布，详见 `LICENSE`。
- 第三方依赖、行情接入组件及数据服务仍分别受其各自许可证或服务条款约束。
- 将本项目用于生产、交易或商用场景前，请先自行确认相关依赖、数据源和接入环境的授权边界。

## 当前边界（MVP）

- 暂未实现真实交易执行（下单、回报、持仓管理）
- 历史数据持久化与回放回测不属于当前主线范围
- 策略注册与 session detector 仍是占位能力
- 风险控制仍以展示与人工决策为主

## 近期稳定性修复（2026-02）

- 修复进入 Live 界面时的偶发/持续卡死问题（初始空表焦点、选择回调渲染风暴等）
- 修复 Live 界面重复绘制堆叠与主屏污染问题（禁止 TUI 运行期间后台 slog 输出到终端）
- 优化 Live 启动路径为异步执行，降低 UI 线程阻塞风险
