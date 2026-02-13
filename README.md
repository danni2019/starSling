# starSling

`starSling` 是一个面向中国期货/期权市场的终端实时监控与量化分析 MVP。

当前版本已经打通以下闭环：

- 实时行情接入（OpenCTP -> Python `live_md`）
- 本地路由缓存与 JSON-RPC 通信（Go `router`）
- 终端 UI 多面板展示（市场/波动率曲线/期权链/异常成交/Flow 聚合）
- 两类实时派生分析 worker（`options_worker`、`unusual_worker`）
- UI 交互筛选、阈值配置、焦点合约联动

## MVP 功能概览

### 1. 实时市场面板（左上）

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

- 配置管理（创建/编辑/删除/设为默认）
- 本地 Python 运行时引导脚本（`scripts/bootstrap_python.sh`）
- 语音播报（可选，依赖 `say`/`espeak`/`spd-say`）
- Metadata 拉取与缓存（合约、交易时段等）

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

## 运行与交互

主菜单：

- `Live market data`
- `Setup Python runtime`
- `Config`
- `Quit`

Live 界面常用按键：

- `Tab / Shift+Tab`：切换焦点面板
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

## 当前边界（MVP）

- 暂未实现真实交易执行（下单、回报、持仓管理）
- 暂未实现历史数据持久化与回放回测
- 策略注册与 session detector 仍是占位能力
- 风险控制仍以展示与人工决策为主

