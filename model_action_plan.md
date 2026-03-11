# Task: Improve Contract Metadata Fetch Reliability

## Request Summary
`./starsling` startup repeatedly fails while refreshing `contract` metadata with:
- `read body: context deadline exceeded`
- downstream `metadata refresh failed`
- and `metadata stale`

Likely causes:
1. `contract` payload is much larger than other metadata sources.
2. Current timeout is globally fixed and too short for this endpoint.
3. Proxy path can further increase transfer latency.

## Current Baseline
- Metadata fetch uses one global client timeout (`15s`) for all sources.
- `contract` source uses one large URL (`futures,option` + all markets in one request).
- Any `contract` request timeout keeps old cache and reports stale metadata.

## Implementation Plan
1. Add per-source timeout support in metadata source config.
- Extend `metadata.Source` with optional `timeout_sec`.
- Keep backward compatibility: missing value falls back to existing default behavior.

2. Add split-fetch support for large sources.
- Extend `metadata.Source` with optional `urls` (batch endpoints).
- Keep backward compatibility with existing single `url`.

3. Implement multi-request aggregation for batch sources.
- For a source with `urls`, request each URL with the source timeout.
- Validate every response as JSON.
- Parse each response into rows and merge into one combined JSON array.
- Only write cache on full success across all batches (atomic success semantics).

4. Update `contract` source config to use split URLs.
- `futures` all markets in one request.
- `option` split by market (SHFE/CZCE/DCE/INE/GFEX/CFFEX).
- Set `timeout_sec` to `60` for contract source.

5. Add/expand tests for metadata parsing and source behavior.
- Cover backward-compatible source loading (`url` only).
- Cover split mode (`urls`) and timeout parsing.
- Cover merge behavior for array/envelope payload forms.

## Test Plan
1. Run targeted tests:
- `go test ./internal/metadata`

2. Run full regression:
- `go test ./...`

## Review Checklist
- `contract` no longer depends on one oversized request.
- Per-source timeout is applied; default timeout remains unchanged for other sources.
- Batch merge preserves valid JSON payload contract for downstream loaders.
- Existing sources without new fields keep working.

## Risks And Mitigations
- Risk: upstream payload shape differs across batches.
- Mitigation: implement tolerant parser supporting array payload and `{data:[...]}` envelope (already used in contract mapping).

- Risk: partial batch success could produce inconsistent cache.
- Mitigation: write cache only after all batch requests succeed and aggregate is valid.

## Rollback Plan
- Revert `metadata.Source` schema extension and split-fetch path.
- Revert `config/metadata.sources.json` contract source to single URL.

## Structure Optimization Assessment
Problem:
Metadata fetching currently mixes transport policy (timeouts, batching) and source definition in a minimal single-URL model, which makes large-source reliability fixes intrusive.

Refactor Direction:
Move toward source-driven transport config (`timeout_sec`, `urls`) so reliability policy is declared in config rather than hard-coded by source name.

Action Plan:
1. Introduce backward-compatible source fields.
2. Centralize source URL resolution and timeout selection in `internal/metadata`.
3. Keep caller API unchanged (`RefreshAll` / `RefreshIfStale`) to avoid cross-layer churn.

Validation:
- `go test ./internal/metadata`
- `go test ./...`

## Approval Gate
Approved by user in-thread ("ok, continue").

## Postmortem
- what happened:
  - A newly added failure-path test compared cached JSON using raw string equality and failed due to indentation/format changes after `MarshalIndent`.
- root cause:
  - The assertion verified serialization formatting instead of JSON semantic equivalence.
- fix:
  - Updated the test to unmarshal both payloads and compare normalized marshaled structures.
- prevention rule:
  - Add a repository test rule: JSON persistence/cache assertions must compare parsed content semantics, not raw formatting.

---

# Task: Fix Review Findings For Batched Metadata Fetch

## Request Summary
Address two review findings in `internal/metadata/metadata.go`:
1. Limit cumulative timeout for batched `urls` fetches.
2. Reject nested payload objects that omit `data` rows instead of silently accepting them.

## Implementation Plan
1. Enforce source-level timeout budget across the full batch.
- Wrap `fetchAndStore` with a per-source `context.WithTimeout(source.timeout())`.
- Reuse that scoped context for each URL fetch so total batch duration cannot exceed the configured timeout budget.

2. Harden nested payload parsing in `extractRows`.
- In the `inner[0] == '{'` branch, detect missing nested `data` explicitly (not just unmarshal success).
- Return an error when nested `data` is missing or null.

3. Add regression tests.
- Add a test that proves batched fetch uses a cumulative timeout budget (fails when combined latency exceeds budget).
- Add a test that `aggregatePayloads` fails when one payload has nested object content without `data`.

## Test Plan
1. Run targeted tests:
- `go test ./internal/metadata`

2. Run full regression:
- `go test ./...`

## Review Checklist
- A source with multiple URLs cannot exceed one configured timeout budget in total.
- Malformed nested payload objects without `data` are rejected.
- Existing single-URL sources still behave unchanged.
- Existing metadata mapping/contract tests still pass.

## Risks And Mitigations
- Risk: tighter timeout semantics may fail on very slow networks.
- Mitigation: timeout remains configurable per source (`timeout_sec`) and now has predictable upper bound behavior.

## Rollback Plan
- Revert the per-source timeout-context wrapper in `fetchAndStore`.
- Revert nested `data` presence validation in `extractRows`.
- Re-run `go test ./internal/metadata` and `go test ./...`.

## Structure Optimization Assessment
No additional project-structure refactor is needed for this follow-up; scope is constrained to reliability hardening in existing `internal/metadata` boundaries.

## Approval Gate
Approved by user in-thread ("approved").

## Postmortem
- what happened:
  - Review identified two reliability gaps in the new batched metadata path: timeout applied per request (not per batch) and nested object payloads without `data` could be accepted silently.
- root cause:
  - Batch-level control flow reused request-level assumptions and lacked explicit required-field presence validation in one parsing branch.
- fix:
  - Enforced `context.WithTimeout(source.timeout())` once per source batch and reused it for all URL fetches; tightened nested payload parsing to fail when `data` is missing/null.
- prevention rule:
  - Added a reusable guardrail to `AGENTS.md`: multi-URL metadata fetches must use one cumulative source timeout budget and missing required payload fields must fail hard.

---

# Task: Live Screen Visual Consistency + Arbitrage Expression Monitor + Overview Column Reorder

## Request Summary
1. Verify and fix font color inconsistency in right-top and right-middle live panels.
2. Replace left-bottom `Flow Aggregation` with an arbitrage monitor that supports full arithmetic expressions (not only linear sum), including operators, contracts, coefficients, constants, and parentheses.
3. Adjust left-top overview columns by reordering only: put each C/P pair adjacent (`c_inv, p_inv, c_fnt, p_fnt, ...`), without changing column count.

## Current Baseline Findings
- Right-top (`liveCurve`) and right-middle (`liveOpts`) use `TextView` text color `colorMuted`.
- Other major data regions use `colorTableRow` for body text.
- Left-bottom panel is currently flow aggregation table + flow settings modal.
- Left-top overview headers are currently grouped by side (`C_INV C_FNT C_MID C_BAK P_INV P_FNT P_MID P_BAK`).

## Implementation Plan (Item 1: Color Fix)
1. Align right-top/right-middle body color.
- In `internal/tui/view_live.go`:
  - `ui.liveCurve.SetTextColor(colorMuted)` -> `colorTableRow`
  - `ui.liveOpts.SetTextColor(colorMuted)` -> `colorTableRow`

2. Add regression check.
- Add/extend `internal/tui/app_test.go` to assert text color values after `buildLiveScreen()`.

3. Validate.
- `go test ./internal/tui -run TestBuildLiveScreen`
- `go test ./...`

## Detailed Feature Plan (Item 2: Left-Bottom Arbitrage Expression Monitor)
### A. Scope and behavior
- Enter on left-bottom opens formula editor modal (`Arbitrage Monitor Settings`).
- User enters one expression string, example:
  - `MA605 * 3 - eg2605 * 2`
  - `contract1 * 4 / contract2 * 3 - (contract3 / 2 + 100)`
- Backend compiles expression once; each market refresh re-evaluates using latest contract prices.
- Panel shows: formula, evaluated result, status, missing contracts, update time.

### B. Expression engine design
1. Token model.
- `NUMBER`: integer/decimal (`100`, `2.5`)
- `IDENT`: unquoted contract token (covers `MA605`, `eg2605`, `IF2603`)
- `LPAREN`, `RPAREN`, `PLUS`, `MINUS`, `MUL`, `DIV`
- Optional quoted contract token for special symbols (for example `'cu2605-C-72000'`) to avoid `-` operator ambiguity.

2. Grammar (operator precedence and associativity).
- `expr   := term (('+' | '-') term)*`
- `term   := unary (('*' | '/') unary)*`
- `unary  := ('+' | '-') unary | primary`
- `primary:= NUMBER | IDENT | QUOTED_IDENT | '(' expr ')'`
- Left associative for `+ - * /`.
- Parentheses override precedence.

3. Parser approach.
- Use shunting-yard (or Pratt parser) to build AST/RPN deterministically.
- Compile-time validation catches:
  - empty input
  - mismatched parentheses
  - consecutive operators / missing operands
  - invalid tokens

4. Contract extraction.
- Walk compiled AST to extract required contract set.
- Normalize contract keys to lowercase for matching.
- Preserve original token for display.

### C. Runtime evaluation model
1. Price source.
- Build `contract -> last_price` map from current market snapshot (`ui.marketRows`).
- Parse numeric from `MarketRow.Last`; non-numeric treated as missing.

2. Evaluation state.
- Each AST node returns:
  - `value` (float64, when known)
  - `known` (bool)
  - `missingContracts` (set)
  - `err` (for invalid runtime cases such as divide-by-zero)

3. Operation rules.
- If both operands known: compute numeric result.
- If one/both unknown: propagate unknown and union missing sets.
- Division by zero: runtime error status (`runtime_error`), no numeric output.
- Final statuses:
  - `ready`: value known, no missing, no runtime error
  - `partial`: no runtime error but has missing contracts
  - `invalid`: compile/parse error
  - `runtime_error`: evaluation error (e.g., divide-by-zero)

### D. UI and persistence integration
1. UI replacement.
- Replace `Flow Aggregation` table rendering with `Arbitrage Monitor`.
- Reuse left-bottom focus position and Enter workflow to minimize navigation changes.

2. New renderer.
- New table fill function (e.g., `fillArbMonitorTable`) with columns:
  - `FORMULA`, `VALUE`, `STATUS`, `MISSING`, `UPDATED_AT`
- Empty state: `Press Enter to input formula`.

3. Persisted settings.
- Save formula text in existing live settings store.
- Restore and auto-compile on entering live screen.
- If restore compile fails, panel shows `invalid` with error hint.

### E. Test plan for item 2
1. Tokenizer/parser unit tests.
- precedence: `a + b * c`
- parentheses: `(a + b) * c`
- unary: `-a + 2`
- mixed constants/contracts: `a/2 + 100`
- invalid syntax cases.

2. Evaluator unit tests.
- full data available => deterministic numeric output
- missing one contract => `partial` + missing list
- divide-by-zero => `runtime_error`
- contract case-insensitive lookup.

3. UI integration tests.
- Enter opens formula modal on left-bottom focus.
- Apply valid formula updates panel.
- Invalid formula shows error and does not crash refresh path.

## Implementation Plan (Item 3: Left-Top Columns Reorder Only)
1. Keep same 8 columns; reorder headers and values only.
- target order:
  - `C_INV`, `P_INV`, `C_FNT`, `P_FNT`, `C_MID`, `P_MID`, `C_BAK`, `P_BAK`

2. Change location.
- `internal/tui/view_live.go` `fillOverviewTable` header + row value sequence.
- No changes to router payload, aggregation logic, sorting/filtering semantics.

3. Tests.
- Update relevant header-order assertions in `internal/tui/app_test.go`.
- Add one regression asserting row value mapping matches new order.

## Risks And Mitigations
- Risk: contract token parsing ambiguity with `-` in contract names.
- Mitigation: support quoted identifiers for special-char contracts; keep unquoted identifiers strict.

- Risk: partial quote availability leads frequent non-numeric outcomes.
- Mitigation: explicit `partial` status + missing contract list in panel.

- Risk: replacing flow panel touches multiple input/render paths.
- Mitigation: keep focus index and key bindings stable; only swap left-bottom action/renderer.

## Rollout Sequence
1. Item 1 color fix + tests.
2. Item 2 arbitrage expression monitor (parser/evaluator/UI/persistence/tests).
3. Item 3 overview column reorder + tests.
4. Full regression: `go test ./...`.

## Structure Optimization Assessment
Problem:
Left-bottom panel behavior (render + Enter action + state) is currently hard-bound to flow aggregation, increasing replacement cost.

Refactor Direction:
Introduce a small lower-panel strategy abstraction in TUI (`panel renderer`, `enter handler`, `state refresh`), then plug arbitrage monitor in.

Action Plan:
1. Extract left-bottom panel handler interface in `internal/tui`.
2. Implement arbitrage monitor handler.
3. Remove flow-only assumptions from key handling and periodic refresh.

Validation:
- `go test ./internal/tui`
- Manual live-screen interaction check (focus cycle + Enter + refresh).

## Postmortem (Planning Correction)
- what happened:
  - Initial plan incorrectly simplified item 2 to linear `sum(coef * price)` and item 3 to 8->4 paired merge.
- root cause:
  - I over-constrained requirements before fully formalizing expression semantics and exact column-change intent.
- fix:
  - Rewrote plan with full expression grammar/evaluation design and corrected item 3 to reorder-only.
- prevention rule:
  - For formula-related features, freeze grammar/scope explicitly before proposing implementation steps.

## Approval Gate
Approved by user in-thread ("可以，批准执行").

---

# Task: Arbitrage Monitor Upgrade To Multi-Pair CRUD + Persistent Storage

## Request Summary
Enhance left-bottom arbitrage monitor from single formula to multi-pair management:
1. Enter should allow creating a new arbitrage pair.
2. Existing arbitrage pairs should be editable and deletable.
3. Arbitrage pair list should persist locally like other settings, so restart does not require re-entry.

## Current Baseline
- Current arbitrage monitor supports one formula only (`arbFormula`).
- Enter opens a single-formula modal.
- Persistence stores one field: `settings_arbitrage.formula`.
- Left-bottom table displays one row at most.

## Target Behavior
1. Multi-pair list model.
- Allow 0..N arbitrage pairs in left-bottom monitor.
- Each pair has:
  - stable `id`
  - optional display `name`
  - `formula`
  - compile/evaluation status and runtime fields

2. Enter interaction model.
- Enter on left-bottom opens management modal with actions:
  - `New`
  - `Edit` (selected existing pair)
  - `Delete` (selected existing pair, with confirmation)
- Edit/New use the same formula editor and validation path.

3. Persistent storage model.
- Save full pair list into settings store under `settings_arbitrage.pairs`.
- Keep backward compatibility:
  - old `settings_arbitrage.formula` auto-migrates into one pair on load.
  - if both old/new exist, prefer new list.

## Detailed Implementation Plan
### A. Data model refactor
1. UI state changes (`internal/tui/app.go`).
- Replace single-formula state with slice-based state:
  - `arbMonitors []arbMonitorState`
  - `arbSelectedID string` (current highlighted monitor for edit/delete target)
- `arbMonitorState` includes:
  - `ID`, `Name`, `Formula`
  - `Compiled`, `CompileErr`
  - latest eval snapshot (`Value`, `Status`, `Missing`, `UpdatedAt`)

2. Settings schema changes (`internal/settingsstore/store.go`).
- Extend `SettingsArbitrage`:
  - retain legacy `Formula string` for migration compatibility.
  - add `Pairs []SettingsArbitragePair`.
- Add `SettingsArbitragePair` struct:
  - `id`, `name`, `formula`.
- Normalize:
  - trim all strings
  - drop invalid/empty formula entries
  - de-duplicate IDs (regenerate duplicates on load path if needed).

### B. Backward-compatible load/migration
1. Load behavior (`internal/tui/settings_persistence.go`).
- If `pairs` non-empty: load pairs directly.
- Else if legacy `formula` non-empty: create one auto-migrated pair with generated ID and empty name.
- Compile all loaded formulas and initialize monitor state.

2. Save behavior.
- Persist current pair list into `settings_arbitrage.pairs`.
- Keep writing legacy `formula` as:
  - empty when list empty,
  - first pair formula when list non-empty (optional compatibility bridge).

### C. CRUD UI flow
1. Left-bottom table rendering (`internal/tui/view_live.go`).
- Table columns update to support multi-pair scan:
  - `ID`, `NAME`, `FORMULA`, `VALUE`, `STATUS`, `MISSING`, `UPDATED_AT`
- Keep row-1 placeholder when no pair exists.

2. Enter modal behavior (`internal/tui/app.go`).
- Replace single editor modal with manager modal:
  - top list of existing pairs (single selection)
  - action buttons: `New`, `Edit`, `Delete`, `Close`
- `New/Edit` open pair editor modal:
  - name input
  - formula input
  - Apply/Cancel
- `Delete` opens confirm modal before removal.

3. Validation rules.
- Formula compile must succeed to save (or choose relaxed mode allowing save-invalid; proposed strict save-valid for now).
- Name optional; if empty, auto-display as `PAIR-<shortid>`.

### D. Evaluation engine integration
1. Reuse current parser/evaluator for each pair independently.
2. On market snapshot update:
- evaluate every pair
- update row status independently:
  - `READY`, `PARTIAL`, `INVALID`, `RUNTIME_ERR`, `IDLE`
3. Keep rendering stable when some pairs invalid and others ready.

### E. Test plan
1. Settings store tests.
- round-trip for multiple pairs.
- normalization of empty/invalid entries.
- legacy single formula migration path.

2. TUI unit tests.
- CRUD operations:
  - create pair adds row
  - edit pair updates formula and recomputes
  - delete pair removes row
- persistence:
  - save then load restores all pairs
- render:
  - multiple rows with mixed statuses.

3. Regression tests.
- Existing single-formula behavior still works when only one pair exists.
- `go test ./internal/tui ./internal/settingsstore`
- full `go test ./...`

## Risks And Mitigations
- Risk: modal complexity increases keyboard/focus edge cases.
- Mitigation: keep interaction linear with explicit selected index and no nested event loops.

- Risk: settings migration could duplicate or lose legacy formula.
- Mitigation: deterministic migration precedence and dedicated migration tests.

- Risk: table can become noisy with many pairs.
- Mitigation: default sort by updated time/status; future optional paging/filter.

## Rollout Steps
1. Schema + migration layer.
2. UI state refactor to multi-pair data model.
3. CRUD modals and table rendering.
4. Evaluation loop for N pairs.
5. tests + full regression.

## Structure Optimization Assessment
Problem:
Arbitrage monitor currently mixes expression state, modal actions, evaluation, and table rendering directly in `app.go`, which will become harder to maintain after CRUD expansion.

Refactor Direction:
Extract a dedicated `internal/tui/arbitrage_manager.go` boundary:
- state container
- CRUD methods
- compile/evaluate orchestration
- conversion to display rows

Action Plan:
1. Introduce manager type with pure methods.
2. Keep `app.go` as orchestrator only (input routing + render call).
3. Move migration helpers close to settings persistence bridge.

Validation:
- unit-test manager independently from tview widgets.
- keep UI tests focused on interaction wiring.

## Approval Gate
Approved by user in-thread ("批准执行").

---

# Task: Arbitrage Manager Continuous Batch CRUD Mode

## Request Summary
Upgrade arbitrage manager interaction so users can perform multiple `New/Edit/Delete` operations continuously after pressing Enter, without being forced back to Live screen after each action.

## Current Gap
- Current manager opens child editor/confirm modals.
- After Apply/Delete, flow calls `closeDrilldown()`, which exits to Live.
- This breaks batch operation continuity.

## Implementation Plan
1. Introduce manager re-entry flow in `internal/tui/arbitrage_manager.go`.
- Add internal method: `openArbitrageMonitorManager(selectedID, message)`.
- Keep `openArbitrageMonitorSettings()` as wrapper that opens manager with current selection.

2. Change `New/Edit/Delete` child flows to return to manager instead of Live.
- `openArbitragePairEditor(..., returnSelectedID)`:
  - Apply: mutate + save + render, then reopen manager with updated selected ID and success message.
  - Cancel: reopen manager with prior selected ID.
- `openArbitragePairDeleteConfirm(..., returnSelectedID)`:
  - Delete: mutate + save + render, then reopen manager with normalized selection and success message.
  - Cancel: reopen manager with prior selected ID.

3. Keep explicit exit path.
- Manager `Close` button remains the single explicit way to return to Live.
- `Esc` behavior remains unchanged (existing global drilldown close).

4. Add focused tests.
- Add unit-style interaction tests for helper flow where feasible:
  - manager selection normalization after delete.
  - create/edit/delete functions still produce correct selected ID and list state.
- Run:
  - `go test ./internal/tui -run 'TestUpsertArbitrageMonitorCreateEditDelete|TestRenderArbitrageMonitorMultiplePairs'`
  - `go test ./...`

## Risks And Mitigations
- Risk: nested modal transitions can desync selected pair.
- Mitigation: always pass/normalize selected ID through manager reopen path.

- Risk: repeated modal replacement might leave stale hint text.
- Mitigation: manager hint text set explicitly on every open.

## Structure Optimization Assessment
No additional structural refactor is required for this increment; the existing `arbitrage_manager.go` boundary already isolates the interaction logic needed for this change.

## Approval Gate
Approved by user in-thread ("OK，批准执行这两项").

---

# Task: Arb Monitor Add Open/High/Low Derived Values

## Request Summary
In left-bottom `Arbitrage Monitor`, extend each pair result to include formula-derived:
1. `open` value (compute once only).
2. `high` value (real-time refresh, same cadence as current `last` value).
3. `low` value (real-time refresh, same cadence as current `last` value).

Display requirement:
- Add `OPEN`, `HIGH`, `LOW` columns immediately after `VALUE`.

## Target Semantics
1. `VALUE`:
- unchanged, still evaluates formula against `last` prices each refresh.

2. `OPEN`:
- evaluate formula against `open` prices.
- when first successful known result is obtained, freeze it in monitor state.
- subsequent refreshes do not overwrite frozen open value.
- if never successfully computed, display `-`.

3. `HIGH` / `LOW`:
- evaluate formula against `high` / `low` prices every refresh.
- update display continuously like `VALUE`.
- unknown/missing/runtime-error remains `-`.

## Implementation Plan
1. Extend market row snapshot fields for formula evaluation.
- File: `internal/tui/mock.go`
- Add `Open`, `High`, `Low` fields to `MarketRow`.

2. Populate `open/high/low` values from market snapshot conversion.
- File: `internal/tui/app.go` (`convertMarketRows`)
- Parse `item["open"]`, `item["high"]`, `item["low"]` with existing optional-float helpers.
- Store formatted strings in `MarketRow`.

3. Generalize arbitrage price-map builder by price source.
- File: `internal/tui/arbitrage.go`
- Replace `buildMarketLastPriceMap` with a generalized helper:
  - `buildMarketPriceMap(rows, priceSelector)` or equivalent source enum.
- Reuse existing numeric parser and contract normalization.
- Keep `last` behavior unchanged and add support for `open/high/low`.

4. Extend monitor runtime state and rendering payload.
- File: `internal/tui/arbitrage_manager.go`
- Add state fields in `arbMonitorState`:
  - open: frozen value + frozen flag.
  - high/low: latest known values + known flags.
- In `renderArbitrageMonitor`:
  - evaluate formula using four price maps (`last/open/high/low`).
  - freeze open on first successful known result only.
  - refresh high/low each render.
  - map to display strings.

5. Extend table model and headers.
- File: `internal/tui/view_live.go`
- Extend `ArbitrageMonitorRow`:
  - add `Open`, `High`, `Low`.
- Update `fillArbitrageTable` headers/order:
  - `ID, NAME, FORMULA, VALUE, OPEN, HIGH, LOW, STATUS, MISSING, UPDATED_AT`.

6. Tests for semantics and rendering.
- File: `internal/tui/arbitrage_test.go`
  - add coverage for generalized price-map builder (`last/open/high/low`).
- File: `internal/tui/arbitrage_ui_test.go`
  - verify open is frozen after first known computation.
  - verify high/low change with updated `marketRows`.
  - verify table column order and displayed values include `OPEN/HIGH/LOW`.
- File: `internal/tui/app_test.go`
  - add/adjust `convertMarketRows` assertions to ensure open/high/low mapping exists.

## Validation Plan
1. Targeted:
- `go test ./internal/tui -run 'TestConvertMarketRows|TestBuildMarket.*PriceMap|TestRenderArbitrageMonitor.*'`

2. Full:
- `go test ./...`

## Risks And Mitigations
- Risk: formula may be ready for last but not ready for open/high/low due missing source fields.
- Mitigation: keep status semantics based on `last` path (existing behavior) while showing `-` for unavailable open/high/low; do not downgrade READY solely for missing open/high/low.

- Risk: open freeze could lock an incorrect transient value if initial open ticks are unstable.
- Mitigation: freeze only after a fully known successful formula result; expose reset path via pair edit/delete/recreate.

## Rollback Plan
1. Revert `MarketRow` added fields + `convertMarketRows` mapping.
2. Revert arbitrage evaluator/map extensions.
3. Revert table columns to prior schema.
4. Re-run `go test ./internal/tui` and `go test ./...`.

## Structure Optimization Assessment
No additional structural refactor is required for this increment; current `arbitrage.go` (expression/eval) + `arbitrage_manager.go` (state/render orchestration) boundary is suitable for adding multi-source derived fields.

## Approval Gate
Approved by user in-thread ("OK，确认该计划，开始实施").

---

# Task: Arb Monitor Add Pre-Close/Pre-Settle Once Metrics + Column Reorder (Name Only)

## Request Summary
Enhance arb monitor further:
1. If market data contains `pre_close` and `pre_settlement`, include formula-derived values for both.
2. `pre_close` and `pre_settlement` values use once-capture semantics (same as `open`: capture first valid result, then freeze).
3. Keep only `Name` (do not add `Note`).
4. Display columns in this exact order:
   - `NAME`, `VALUE`, `HIGH`, `LOW`, `OPEN`, `PRE_CLOSE`, `PRE_SETTLE`, `STATUS`, `MISSING`, `UPDATED_AT`, `FORMULA`.
5. Keep `FORMULA` as the last column to avoid long-text squeezing of key metrics.

## Target Semantics
1. Refresh behavior by metric source:
- `VALUE` (`last`): realtime refresh each market snapshot.
- `HIGH` (`high`): realtime refresh each market snapshot.
- `LOW` (`low`): realtime refresh each market snapshot.
- `OPEN` (`open`): capture once on first valid formula result; then freeze.
- `PRE_CLOSE` (`pre_close`): capture once on first valid formula result; then freeze.
- `PRE_SETTLE` (`pre_settlement`): capture once on first valid formula result; then freeze.

2. Missing source data:
- If a source field is missing or non-numeric, that source result remains `-`.
- Once-capture metrics (`open/pre_close/pre_settle`) keep `-` until first successful known computation.

3. Status semantics:
- Keep existing `STATUS/MISSING` semantics based on `last` evaluation path to avoid regression.

## Implementation Plan
1. Extend market row fields and conversion.
- File: `internal/tui/mock.go`
  - add `PreClose`, `PreSettlement` to `MarketRow`.
- File: `internal/tui/app.go` (`convertMarketRows`)
  - map and format `item["pre_close"]` and `item["pre_settlement"]` into `MarketRow`.
  - keep current `chg/chg%` logic unchanged.

2. Extend arbitrage price-map helpers.
- File: `internal/tui/arbitrage.go`
  - add:
    - `buildMarketPreClosePriceMap`
    - `buildMarketPreSettlementPriceMap`
  - reuse generic `buildMarketPriceMap`.

3. Extend arb monitor runtime state for once-capture metrics.
- File: `internal/tui/arbitrage_manager.go`
  - add per-monitor frozen fields:
    - `PreCloseValue`, `PreCloseCaptured`
    - `PreSettlementValue`, `PreSettlementCaptured`
  - include these in runtime-state reset helper.

4. Adjust rendering metrics order and computation.
- File: `internal/tui/arbitrage_manager.go`
  - compute maps for `last/high/low/open/pre_close/pre_settlement`.
  - evaluate:
    - realtime: `value/high/low`
    - once-capture: `open/pre_close/pre_settlement`
  - output row metric order:
    - `Value`, `High`, `Low`, `Open`, `PreClose`, `PreSettle`.

5. Keep Name-only edit/persistence semantics (no new Note field).
- File: `internal/tui/arbitrage_manager.go`
  - keep existing `Name(optional)` editor input and CRUD behavior.
  - manager dropdown display continues to prefer `Name`.
- File: `internal/settingsstore/store.go` + `internal/tui/settings_persistence.go`
  - no schema changes for arbitrage pair metadata; continue using existing `name`.

## Column Layout Update
Target arb monitor columns:
- `NAME`, `VALUE`, `HIGH`, `LOW`, `OPEN`, `PRE_CLOSE`, `PRE_SETTLE`, `STATUS`, `MISSING`, `UPDATED_AT`, `FORMULA`

## Test Plan
1. `internal/tui/app_test.go`
- add mapping test for `pre_close/pre_settlement` conversion.

2. `internal/tui/arbitrage_test.go`
- add map-builder tests for pre-close/pre-settlement price maps.

3. `internal/tui/arbitrage_ui_test.go`
- assert metric order is `value/high/low/open/pre_close/pre_settle`.
- assert `pre_close/pre_settlement` once-capture behavior.
- assert existing `open` freeze remains correct with new order.
- update existing status/missing column indices.

4. `internal/tui/view_live_test.go`
- assert header sequence equals required order with final `FORMULA` column.

## Validation Plan
1. Targeted:
- `go test ./internal/tui -run 'TestConvertMarketRows|TestBuildMarketPriceMaps|TestRenderArbitrageMonitor|TestFillArbitrageTable'`
- `go test ./internal/settingsstore -run 'TestSaveLoadRoundTrip|TestNormalizeFallsBackInvalidFields'`

2. Full:
- `go test ./...`

## Risks And Mitigations
- Risk: wider table could reduce readability on narrow terminals.
- Mitigation: keep fixed concise headers and preserve current truncation/padding behavior.

## Rollback Plan
1. Revert added `pre_close/pre_settlement` fields and builders.
2. Revert once-capture runtime additions.
3. Revert table headers/order changes.
4. Re-run `go test ./internal/tui ./internal/settingsstore` and `go test ./...`.

## Structure Optimization Assessment
No additional structural refactor is needed for this increment; existing `arbitrage.go` + `arbitrage_manager.go` split remains appropriate.

## Approval Gate
Approved by user in-thread ("确认执行").

---

# Task: Arb Monitor Use Raw Market Rows (Decouple From Left-Mid Display Filter)

## Request Summary
Fix arb monitor so formula evaluation is based on raw market snapshot rows instead of left-middle displayed rows.

Current issue:
- Arb currently reads `ui.marketRows`, which is post-filtered by left-middle filter settings.
- Contracts excluded from left-middle display become unavailable to arb evaluation.

Target behavior:
- Arb evaluation should use original market snapshot data (`ui.marketRawRows`) so it can access all contracts in feed scope.
- Left-middle display filter should only affect left-middle table, not arb calculations.

## Implementation Plan
1. Introduce raw-row price-map builders for arb.
- File: `internal/tui/arbitrage.go`
- Add helper(s) to build price maps directly from `[]map[string]any` raw rows for:
  - `last`, `high`, `low`, `open`, `pre_close`, `pre_settlement`
- Parse numeric values using existing optional-float parser patterns (`asOptionalFloat` compatible logic).
- Contract key uses normalized `ctp_contract` from raw row.

2. Switch arb monitor runtime evaluation source.
- File: `internal/tui/arbitrage_manager.go`
- In `renderArbitrageMonitor`, build all metric maps from `ui.marketRawRows` first.
- If `ui.marketRawRows` is empty but `ui.marketRows` exists (edge fallback), keep current row-based map path as fallback.
- Keep existing realtime/once-capture semantics unchanged:
  - realtime: `value/high/low`
  - once-capture: `open/pre_close/pre_settle`

3. Keep display logic unchanged.
- No changes to left-middle filters or table rendering.
- No settings schema changes.

## Test Plan
1. Add targeted regression test in `internal/tui/arbitrage_ui_test.go`.
- Construct UI state where:
  - `marketRawRows` contains contracts A and B.
  - `marketRows` contains only contract A (simulating left-middle filter).
  - formula requires A and B.
- Expect arb evaluation to be `READY` with correct computed value (proves raw rows are used).

2. Keep existing arb tests passing.
- Re-run key tests for:
  - open/pre-close/pre-settle once-capture
  - high/low realtime refresh
  - table column order.

3. Run validation:
- `go test ./internal/tui -run 'TestRenderArbitrageMonitor.*|TestBuildMarketPriceMaps.*'`
- `go test ./...`

## Risks And Mitigations
- Risk: raw rows may include contracts with missing numeric fields.
- Mitigation: preserve current map behavior (skip non-numeric values) and existing missing-contract semantics.

- Risk: raw rows could be stale while filtered rows are fresh during transition.
- Mitigation: both derive from the same market snapshot apply path; fallback to `marketRows` only when raw rows are empty.

## Rollback Plan
1. Revert arb manager back to `ui.marketRows` based maps.
2. Revert raw-row map helper additions.
3. Re-run `go test ./internal/tui` and `go test ./...`.

## Structure Optimization Assessment
Problem:
Arb evaluation currently depends on a UI projection (`marketRows`) rather than a source-of-truth data set, causing hidden coupling to display filters.

Refactor Direction:
Use source-of-truth `marketRawRows` for calculation paths and keep `marketRows` strictly as presentation data.

Action Plan:
1. Add raw-row map builders in arb module.
2. Switch arb manager to raw rows with safe fallback.
3. Add regression proving left-middle filtering no longer affects arb.

Validation:
- `go test ./internal/tui`
- `go test ./...`

## Approval Gate
Approved by user in-thread ("确认").

---

# Task: Right-Bottom Unusual Panel Symbol/Contract Multi-Filter

## Request Summary
Add filter inputs in the right-bottom panel settings (Unusual option volume area) so users can limit monitoring by:
1. `symbol`
2. `contract`
Both should support comma-separated multiple values.

## Current Baseline
- Right-bottom Enter currently opens `Unusual thresholds` modal with only:
  - `Turnover Chg >=`
  - `Turnover Ratio >=`
  - `OI Ratio >=`
- No symbol/contract filter exists in that panel-specific settings flow.

## Implementation Plan
1. Extend UI state for unusual filters.
- Add fields in `UI` state:
  - `unusualFilterSymbol string`
  - `unusualFilterContract string`

2. Extend settings schema and persistence.
- Update `internal/settingsstore/store.go` `SettingsUnusual`:
  - add `Symbol string` and `Contract string`
- Normalize by trimming spaces.
- Update round-trip/normalize tests in `internal/settingsstore/store_test.go`.
- In `internal/tui/settings_persistence.go`:
  - load values into UI state (`applyPersistedSettings`)
  - save values in `saveUnusualSettingsToStore`

3. Update right-bottom settings form (Enter on liveTrades).
- In `openUnusualThresholdSettings()` add two `InputField`s:
  - `Symbol (csv):`
  - `Contract (csv):`
- Apply action writes these fields into UI state + persistence.

4. Implement filter logic for right-bottom display.
- In `applyUnusualSnapshot()` path, before `convertUnusualTrades(...)`:
  - filter raw unusual rows by symbol and/or contract.
- Matching rules:
  - tokens from comma-separated input, case-insensitive, trimmed.
  - empty input => no filtering on that field.
  - both provided => AND semantics (must satisfy both dimensions).
- Data mapping:
  - `symbol` matches row `symbol`
  - `contract` matches row `ctp_contract`

5. Keep existing thresholds behavior unchanged.
- Threshold push to router remains as-is.
- Filter is UI-side monitoring restriction for the right-bottom panel display.

## Test Plan
1. Unit tests for token parsing/match:
- single token, multi token, whitespace, case-insensitive.

2. TUI tests:
- right-bottom settings apply persists symbol/contract filter.
- `applyUnusualSnapshot` filters displayed trades correctly by:
  - symbol only
  - contract only
  - both symbol+contract
  - empty filters (no restriction)

3. Full regression:
- `go test ./internal/tui ./internal/settingsstore`
- `go test ./...`

## Risks And Mitigations
- Risk: user confusion between market filter and unusual filter scopes.
- Mitigation: form labels explicitly state this is for right-bottom unusual monitoring.

- Risk: symbol field missing in some rows.
- Mitigation: missing field treated as non-match when symbol filter is active.

## Structure Optimization Assessment
No additional structure refactor required for this increment; this is a focused extension to existing unusual panel settings + display filter pipeline.

## Approval Gate
Approved by user in-thread ("OK，批准执行这两项").

---

# Task: Remove Filter Contract + Fix Filter Symbol Association in Right-Bottom Panel

## Request Summary
1. Remove `Filter Contract` from right-bottom unusual settings.
2. Fix `Filter Symbol` so inputs like `sc` or `sc,px` can correctly filter corresponding option/futures rows.

## Current Baseline Findings
- `openUnusualThresholdSettings()` still shows both inputs:
  - `Filter Symbol(csv):`
  - `Filter Contract(csv):`
- `filterUnusualRows(rows, symbolCSV, contractCSV)` currently does exact token match on:
  - `row["symbol"]`
  - `row["ctp_contract"]`
- Symbol filtering does not reuse existing contract normalization/mapping path (`contract + underlying + symbol` candidate matching), so symbol-only filters can fail when raw `symbol` is missing/non-normalized.

## Implementation Plan
1. Remove `Filter Contract` from UI and runtime filtering path.
- In `internal/tui/app.go`, remove contract input item from `openUnusualThresholdSettings()`.
- Apply action only updates `ui.unusualFilterSymbol`.
- Update unusual snapshot/render callsites to stop passing/using contract filter.

2. Rework unusual filtering into symbol-only robust matching.
- Change `filterUnusualRows` signature to `filterUnusualRows(rows []map[string]any, symbolCSV string, resolver contractResolver)`.
- For each row, evaluate symbol filter tokens against normalized candidates built from:
  - `ctp_contract`
  - `underlying`
  - `symbol`
- Reuse existing `focusMatchCandidates(...)` + resolver (`ui.metadata`) to ensure cross-panel consistent association behavior.
- Keep exact token matching semantics (case-insensitive, trimmed, comma-separated).

3. Persistence behavior (backward-compatible).
- Stop loading contract filter into UI runtime state.
- On save, keep writing `cfg.Unusual.Symbol` and force `cfg.Unusual.Contract = ""` to clear legacy value.
- Keep settings schema unchanged for compatibility with existing config files.

4. Tests update.
- `internal/tui/app_test.go`:
  - replace dual-filter tests with symbol-only tests.
  - add regression case proving symbol filter can match via underlying/contract-derived symbol (e.g. `sc`, `sc,px`).
- `internal/tui/settings_persistence_test.go`:
  - update unusual settings load/save assertions to symbol-only runtime behavior.
- Keep `internal/settingsstore` schema tests stable (no schema deletion in this task).

## Test Plan
1. `go test ./internal/tui -run 'Unusual|FilterUnusual'`
2. `go test ./internal/tui`
3. `go test ./...`

## Risks And Mitigations
- Risk: token matching may become too broad.
  - Mitigation: still require exact token equality against normalized candidate list (no fuzzy match).
- Risk: legacy contract filter values remain in local config.
  - Mitigation: save path explicitly clears contract filter value.

## Rollback Plan
1. Restore `Filter Contract` input in unusual settings form.
2. Restore old `filterUnusualRows(rows, symbolCSV, contractCSV)` API and callsites.
3. Restore persistence load/save behavior for contract filter.
4. Re-run `go test ./internal/tui` and `go test ./...`.

## Structure Optimization Assessment
Problem:
- Filtering semantics are split across simple field matchers and normalized identity matchers, leading to inconsistent UX across panels.

Refactor Direction:
- Introduce a shared row identity matching helper (contract/underlying/symbol normalization) reused by market/options/unusual filters.

Action Plan:
1. This task: fix unusual filter by reusing existing normalization helper.
2. Follow-up task: extract shared matcher utility and migrate duplicated matching paths.
3. Add cross-panel consistency tests for identical symbol inputs.

Validation:
- `go test ./internal/tui`
- Manual check for inputs: `sc`, `sc,px`, mixed case, and empty filter.

## Approval Gate
Approved by user in-thread ("确认").
