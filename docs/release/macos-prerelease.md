# macOS Prerelease Validation

本文件用于本地验证 `starSling` 的 `macOS` prerelease 发布路径。

目标不是证明“所有外部依赖都已一键打包”，而是确认：

- 发布产物结构完整；
- 运行时引导材料齐全；
- 首次用户路径已经收敛为“下载后先运行 `./starsling`，再按应用内引导进入 Setup”；
- 仓库默认值不会把真实运营配置带入公开版本。

## Scope

当前 prerelease 仅优先覆盖 `macOS`：

- `darwin/arm64`
- `darwin/amd64`

`OpenCTP` 兼容环境、wheel 和账号接入仍可能需要用户自行准备。

## Validation Flow

### 1. Repository Baseline

先在仓库根目录执行：

```bash
go test ./...
go run ./cmd/starsling doctor
```

`doctor` 应至少确认：

- 支持的平台解析正常；
- `scripts/bootstrap_python.sh` 可定位；
- `config/metadata.sources.json` 可定位；
- 默认配置仍是未配置态 `Host/Port`；
- 配置目录与 metadata 目录解析正常。

如果 `doctor` 仅提示 `bundled python` 缺失，这是允许的：当前 prerelease 仍依赖 bootstrap 步骤来准备本地 Python runtime。

### 2. First-Run Bootstrap Flow

先按最终用户路径验证：

```bash
./starsling
```

预期行为：

- 如果 bundled runtime 尚未准备好，主界面会提示进入 `Setup Python runtime`
- 从 `Live market data` 入口进入时，如 runtime 缺失，也会被引导到 Setup
- 应用启动时会先做一次 metadata 预热
- 从 `Live market data` 入口进入时，如本地 metadata 缺失或过期，也会被引导到独立的 `Refresh market metadata`
- bootstrap 完成后，可继续配置 `Host/Port` 并进入 Live

如需验证手动 fallback 路径，也可执行：

```bash
./scripts/bootstrap_python.sh
go run ./cmd/starsling doctor
```

第二次执行 `doctor` 时，`bundled python` 应能被定位到。

### 3. GoReleaser Validation

在本机已安装 `goreleaser` 的前提下，按官方 `v2` 命令流验证配置：

```bash
goreleaser check
goreleaser release --snapshot --clean
```

`--snapshot` 用于本地 dry-run，不创建正式 GitHub release。

### 4. Archive Contract Audit

本地 snapshot 归档应至少包含：

- `starsling`
- `LICENSE`
- `README.md`
- `CONTRIBUTING.md`
- `SECURITY.md`
- `config/starsling.example.json`
- `config/metadata.sources.json`
- `python/README.md`
- `python/requirements.txt`
- `scripts/bootstrap_python.sh`

## Known Limits

- 当前 prerelease 不承诺外部行情依赖全部一键安装完成。
- 当前“首次初始化”仍需要通过应用内 Setup 调用 bootstrap；只是不再要求用户先 clone 仓库或手动寻找脚本入口。
- 默认 `live-md.host` 为空、`live-md.port` 为 `0`；用户必须先在 `Config` 页面配置真实值。
- metadata 预热与 `Live` 前刷新都依赖访问 `dict.openctp.cn`；若网络不可达，UI 会明确阻止进入空白 Live 界面。
- 如果 `goreleaser` 尚未安装，本文件中的发布 dry-run 步骤无法在本机执行，但 `doctor` 与 `go test ./...` 仍应先通过。

## Observed Local Result

2026-04-08 本地已完成一次 dry-run：

- `goreleaser check`
- `goreleaser release --snapshot --clean`
- 从 `darwin/arm64` snapshot 归档解压后直接运行 `./starsling doctor`

生成产物：

- `dist/starsling_0.0.0-SNAPSHOT-a0071aa_darwin_arm64.tar.gz`
- `dist/starsling_0.0.0-SNAPSHOT-a0071aa_darwin_amd64.tar.gz`
- `dist/checksums.txt`

归档内二进制自检结果：

- `bootstrap script` 与 `config/metadata.sources.json` 可被正确发现
- `bundled python` 按预期显示为 `warn`，因为 prerelease 归档本身不内置 runtime
- 默认 `host` 为空、`port` 为 `0`
