# Repository Guidelines

## Project Structure & Module Organization
- `cmd/starsling/`: application entrypoint and process startup wiring.
- `internal/tui`: terminal UI and interaction loop.
- `internal/router` + `internal/ipc`: local JSON-RPC transport and latest-snapshot state cache.
- `internal/live`: embedded Python worker process orchestration.
- `internal/metadata`, `internal/config`, `internal/configstore`: metadata/config parsing and persistence.
- `config/`: sample config and metadata source definitions.
- `python/`: Python deps and integration notes.
- `scripts/bootstrap_python.sh`: standalone Python runtime bootstrap.
- `runtime/`: generated local runtime artifacts (Git-ignored).

## Build, Test, and Development Commands
- `go run ./cmd/starsling`: run the app locally.
- `go build ./cmd/starsling`: build the binary (`./starsling`).
- `go test ./...`: run all Go tests.
- `go test ./internal/router -run TestRouter`: iterate on router behavior quickly.
- `./scripts/bootstrap_python.sh`: install local Python 3.11 runtime and worker dependencies.
- `STARSLING_INTERNAL_DEBUG_UI=1 go run ./cmd/starsling`: run with internal UI debug logs enabled.

## Coding Style & Naming Conventions
- Keep Go code `gofmt`-clean (`gofmt -w <file.go>` before commit).
- Follow idiomatic Go naming: exported `CamelCase`, unexported `camelCase`, package names lowercase.
- Keep domain boundaries clear (`tui`, `router`, `metadata`) and avoid cross-layer shortcuts.
- Python worker edits should remain dependency-compatible with `python/requirements.txt` and preserve JSON-RPC contracts.
- Pipeline rules are mandatory: latest-snapshot processing, JSON-RPC 2.0 over TCP length-prefix, and tick-driven UI refresh (no per-message redraw path).

## Testing Guidelines
- Use Go’s standard `testing` framework with file pattern `*_test.go`.
- Name tests with `TestXxx` and prefer table-driven tests for contract-like logic.
- Add or update tests for behavior changes, especially snapshot sequencing, stale-state handling, and RPC payload compatibility.
- For JSON persistence/cache assertions, compare semantic equality (parsed JSON) instead of raw string formatting to avoid false negatives from pretty-print differences.
- Validate changes with `go test ./...` before opening a PR, plus manual TUI checks when UI behavior changes.

## Commit & Pull Request Guidelines
- Existing history mixes short `fix` commits and scoped Conventional Commit messages; prefer `type(scope): imperative summary` (example: `feat(tui): add focus symbol resync`).
- Keep commits small and single-purpose; avoid vague messages like `update`.
- `HARD SOP`: `code review before commit` is mandatory in every task. Never run any `git commit` before an explicit review step is completed and explicitly acknowledged in the thread.
- If a commit was made before review by mistake: immediately stop, undo the commit while preserving changes in working tree, document a postmortem in `docs/maintainers/worklog.md`, and resume from review stage.
- PRs should include: purpose, key files changed, test evidence (commands run), config/runtime impact, and terminal screenshots/snippets for visible TUI updates.
- Link related issues/tasks when available.

## Data Source & Protocol Requirements
- Treat `open_ctp` as the default source unless explicitly expanded.
- Do not ship new calculations using fields without documented source, meaning, units, and conversion rules.
- Keep enum mappings explicit and tested (`product_class`, `option_type`, `status`).
- Internal option-type logic must use normalized `c/p`; map source values (`1/2`) before core computations.
- New data-source onboarding must document: source name/scope, key field semantics and units/time basis, enum value mappings, and source-to-internal normalization rules.
- If mappings/semantics change, update implementation, tests, and `conventions.md` in the same PR.
- If docs and code diverge, fix documentation first, then merge code changes.
- For multi-URL metadata fetchers, enforce one cumulative timeout budget per source batch and treat missing required payload fields (for example nested `data`) as hard errors.

## Agent Workflow Requirements
- For edits larger than 3 lines, follow: `plan (docs/maintainers/worklog.md) -> approval -> code -> test -> review -> commit`.
- `HARD GATE`: no code edits before an explicit in-thread plan and explicit user approval for that plan.
- At the start of each new session (or after compact), re-read `AGENTS.md` before continuing.
- Do not use root-level progress-tracking files for new planning; public roadmap/release docs belong under `docs/`, and maintainer execution tracking belongs in `docs/maintainers/worklog.md`.
- If requirements are ambiguous, stop and confirm assumptions before implementing.
- For unspecified cases, default to the same cautious workflow: plan first, confirm when needed, then implement.
- After any mistake (wrong assumption, missed requirement, failed execution due preventable causes, or rework caused by agent error), run a short postmortem in the same task:
  - record `what happened`, `root cause`, `fix`, and `prevention rule` in `docs/maintainers/worklog.md`;
  - if the prevention can be generalized, add/update a rule in `AGENTS.md` immediately in the same change set.
- When updating `docs/maintainers/worklog.md`, keep one clearly delimited active task block and ensure any `Postmortem` stays attached to that same task block.
- When adding or changing contract-string fallback parsers, include delimiter-boundary test cases (e.g., `-C-`/`-P-`) and validate extracted tokens do not retain separators.
- After editing an embedded Python worker (`internal/live/*.py`), run an import-time sanity check (e.g., `python3 internal/live/<worker>.py --help`) in addition to syntax checks, to catch top-level initialization/runtime ordering errors that `py_compile` will not catch.
- In every task, proactively assess whether project-structure optimization would reduce future friction; if yes, provide a refactor proposal and an action plan before ending the task.

## Continuous Improvement Requirements
- Mistake-to-principle loop is mandatory:
  - each confirmed mistake must produce a concrete lesson;
  - each reusable lesson must be promoted to a behavioral rule in `AGENTS.md`;
  - avoid duplicates by refining existing rules when overlap exists.
- Structure optimization proposal format (when applicable):
  - `Problem`: current friction/inefficiency and impact;
  - `Refactor Direction`: target structure or boundary adjustment;
  - `Action Plan`: incremental steps with rollback-safe checkpoints;
  - `Validation`: tests/checks to confirm no regression.
- If no structural optimization is needed for a task, explicitly state that conclusion in the task review.
