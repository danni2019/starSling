# Linux Prerelease Validation

本文件用于本地验证 `starSling` 的 `linux-x86_64` prerelease 发布路径。

目标不是证明“所有外部依赖都已一键打包”，而是确认：

- 发布产物结构完整；
- Linux 归档名称与资产契约正确；
- 首次用户路径与 macOS 保持一致；
- bootstrap 脚本能为 `linux-x86_64` 解析正确的平台目录。

## Scope

当前 Linux prerelease 仅覆盖：

- `linux/amd64`

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

### 2. GoReleaser Validation

在本机已安装 `goreleaser` 的前提下，执行：

```bash
goreleaser check
goreleaser release --snapshot --clean
```

预期至少生成：

- `starsling_<version>_linux_amd64.tar.gz`
- `checksums.txt`

### 3. Archive Contract Audit

Linux snapshot 归档应至少包含：

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

### 4. Runtime Bootstrap Notes

当前 Linux 用户路径与 macOS 一致：

1. 下载并解压 release 归档
2. 运行 `./starsling`
3. 若 bundled runtime 不存在，由应用内引导进入 `Setup Python runtime`
4. 首次进入 `Live market data` 时，如本地 metadata 缺失或过期，应用会尝试刷新 contract / trade_time metadata
5. 完成 bootstrap 后配置真实 `Host/Port`

如果需要手动 fallback，也可以直接执行：

```bash
./scripts/bootstrap_python.sh
```

该脚本在 Linux 上会解析到 `runtime/linux-x86_64/...`。

## Known Limits

- 当前 prerelease 不承诺外部行情依赖全部一键安装完成。
- 当前“首次初始化”仍需要通过应用内 Setup 调用 bootstrap。
- 首次进入 `Live market data` 仍依赖访问 `dict.openctp.cn` 来刷新 metadata；若网络不可达，UI 会明确阻止进入空白 Live 界面。
- 这份文档默认在非 Linux 主机上只验证归档和配置，不替代真实 Linux 环境的端到端 smoke test。
