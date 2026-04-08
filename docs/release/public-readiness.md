# Public Readiness

本文件是 `starSling` 面向公开仓库与首个 GitHub prerelease 的稳定门禁清单。

当前目标：

- 仓库已公开，继续保持公开仓库基线与社区文件完整。
- 首个正式发布仍以 `macOS` prerelease 为主，不承诺跨平台 GA。

## Completed Foundation

- [x] 项目代码许可证明确为 `BSD-3-Clause`。
- [x] 已补齐 `LICENSE`、`SECURITY.md`、`CONTRIBUTING.md`、issue / PR 模板。
- [x] 样例与默认 `live-md.host` / `live-md.port` 已改为未配置态（空 `host` + `port=0`）。
- [x] 未配置真实 `Host` / `Port` 时，UI 会阻止进入 `Live market data`。
- [x] 若 bundled runtime 缺失，主界面与 `Live market data` 入口都会引导进入 `Setup Python runtime`。
- [x] `.con` 运行时缓存文件已从版本控制中移除，并继续由 `.gitignore` 忽略。
- [x] README 已明确 `macOS` prerelease 的定位与外部依赖边界。
- [x] 已落地 `.goreleaser.yaml` 与 GitHub release workflow 骨架。

## Remaining Gates

### 1. Repository Hygiene

- [x] 已完成一轮公开前 history / tracked-files 审计。
- [ ] 明确 git 历史里的旧 front 地址是否需要清理，还是接受其作为非敏感历史信息保留。
- [x] GitHub repo 已切换为 `public`。
- [x] rulesets 已生效。
- [x] `Dependency graph` 与 `Dependabot alerts` 已启用。
- [x] `Secret protection / push protection` 已启用。
- [ ] 将本地社区文件与模板推送到默认分支后，再次复核 community profile 显示结果。

审计结论（2026-04-08）：

- 旧 front 地址仅在两个早期提交中出现：
  - `79a6aec`：`config/starsling.example.json`、`internal/config/defaults.json`、`ref_only/openctp_md.py`
  - `dc7a65b`：`ref_only/openctp_md.py`
- 审计范围内未发现凭证；出现的是 `host/port`，不是账号或密码。
- `.con` 历史对象均为 6-byte 小缓存文件，更像运行时噪音，不像可操作业务数据。
- 当前建议：如果该旧 front 地址不被视为保密运营基础设施，可接受历史保留；若你坚持公开仓库中历史也不出现该地址，再单独做 history rewrite。

### 2. macOS Prerelease Validation

- [x] 已在本地验证 `goreleaser check`。
- [x] 已在本地执行 `goreleaser release --snapshot --clean`。
- [x] 已核对 snapshot 归档内容，`darwin/arm64` 与 `darwin/amd64` tarball 均满足当前归档合约。
- [x] 已从 snapshot 归档中解压 `darwin/arm64` 包并直接运行 `./starsling doctor`，确认包内脚本与配置路径可被发现。
- [ ] 在干净环境里完整验证 runtime bootstrap 安装步骤。
- [x] 已增加 `starsling doctor` 自检入口，覆盖平台、bootstrap、metadata sources、默认配置与目录解析等基础发布检查。
- [ ] 将 `doctor` 接入本地 / CI 发布验证例程。

### 3. Release Publication

- [ ] 选定首个 prerelease tag（例如 `v0.1.0-alpha.1`）。
- [ ] 准备正式 release notes 与已知限制说明。
- [ ] 在前两类门禁完成后发布首个正式 prerelease。

## Release Artifact Contract

首个 `macOS` prerelease 归档应至少包含：

- `starsling` 可执行文件
- `LICENSE`
- `README.md`
- `CONTRIBUTING.md`
- `SECURITY.md`
- `config/starsling.example.json`
- `config/metadata.sources.json`
- `python/README.md`
- `python/requirements.txt`
- `scripts/bootstrap_python.sh`

## Validation Entry Points

- 自检命令：`go run ./cmd/starsling doctor`
- 发布验证文档：`docs/release/macos-prerelease.md`

## Positioning Notes

- 当前发布目标是“让新用户下载压缩包后先运行 `./starsling`，并按应用内引导完成首次初始化”，不是“所有依赖一键打包完成”。
- `OpenCTP` 相关环境、wheel 和服务接入仍可能需要用户自行准备。
- 公开发布前，应始终把 operator-specific 配置、私有索引和真实凭证排除在仓库之外。
- 仓库已经切换为 `public`；每次推送社区文件或仓库设置变更后，都应回到 GitHub `Security` / `Rules` / `Insights` 页面复核状态。
