
# Release Plan (Option B: Offline Bundle)

Goal: Produce install-ready, offline bundles for macOS arm64 and Linux x86_64 that include the Go binary plus a prebuilt Python runtime and dependencies (no network needed on first run).

## Scope
- Build Go binaries for both target platforms.
- Bootstrap and vendor Python runtime + deps into `runtime/<platform>`.
- Package per-platform tarballs with all required scripts/configs.
- Generate checksums and a short release note template.

## Execution Checklist

- [ ] **Step 1: Define release layout**
  - [ ] `dist/starsling-<platform>/starsling` (Go binary)
  - [ ] `dist/starsling-<platform>/runtime/<platform>/...` (Python runtime + venv)
  - [ ] `dist/starsling-<platform>/scripts/` (bootstrap scripts)
  - [ ] `dist/starsling-<platform>/internal/live/*.py` (workers)
  - [ ] `dist/starsling-<platform>/python/requirements.txt`
  - [ ] Optional: `config/starsling.example.json` (sample config)

- [ ] **Step 2: Build Go binaries**
  - [ ] macOS arm64 build
  - [ ] Linux x86_64 build

- [ ] **Step 3: Bootstrap Python runtime (offline)**
  - [ ] Run `scripts/bootstrap_python.sh --platform macos-arm64`
  - [ ] Run `scripts/bootstrap_python.sh --platform linux-x86_64`
  - [ ] Verify `runtime/<platform>/venv` exists and deps installed

- [ ] **Step 4: Package artifacts**
  - [ ] Assemble per-platform folders under `dist/`
  - [ ] Create `tar.gz` bundles
  - [ ] Generate `SHA256SUMS.txt`

- [ ] **Step 5: Sanity checks**
  - [ ] Unpack each tarball and run `./starsling` (no network)
  - [ ] Ensure Python runtime is detected without bootstrap

- [ ] **Step 6: Release docs**
  - [ ] Draft release notes snippet with install/run steps
  - [ ] Update `README.md` with install + offline bundle instructions

## Out of Scope
- GUI installers (`.dmg`, `.pkg`, `.deb`, `.rpm`)
- Code signing / notarization

## Target Files (if needed)
- `scripts/` (only if bootstrap needs tweaks)
- `README.md` (optional release usage snippet)
