# Task: Upgrade Call/Put Skew Formula With Delta-Band Fallback

## Request Summary
Upgrade curve-side skew logic for both call and put:
1. Default formula remains `skew = iv25 - atm_iv`.
2. Primary `iv25` sample band uses `abs(delta) in [0.2, 0.3]`.
3. If no valid `iv25` sample exists for a side, widen that side's `iv25` band to `abs(delta) in [0.1, 0.4]`.
4. If `[0.1, 0.4]` still has no valid `iv25` sample for that same side, expand to `abs(delta) in [0, 0.5]`.

## Current Baseline
- Skew is computed in `internal/live/options_worker.py` via `_compute_side_skew`.
- Current behavior:
  - `atm_iv` from `abs(delta) in [0.45, 0.55]`.
  - `iv25` from `abs(delta) in [0.2, 0.3]`.
  - Returns `iv25 - atm_iv`, or `None` if either sample is unavailable.

## Implementation Plan
1. Refactor side-skew computation in `internal/live/options_worker.py`.
- Keep `atm_iv` logic unchanged: `abs(delta) in [0.45, 0.55]`.
- Keep primary `iv25` logic unchanged: `abs(delta) in [0.2, 0.3]`.
- Add tier-1 fallback `iv25` logic for empty/invalid primary sample: `abs(delta) in [0.1, 0.4]`.
- Add tier-2 fallback `iv25` logic when tier-1 also has no valid sample: `abs(delta) in [0, 0.5]`.
- Preserve output formula: `skew = iv25 - atm_iv`.

2. Keep call and put side isolation unchanged.
- `call_skew` continues to use call-only rows.
- `put_skew` continues to use put-only rows.
- Fallback decision is side-local (call and put can independently use primary/tier-1/tier-2 fallback).

3. Keep payload contract unchanged.
- No field renames; continue writing `call_skew` and `put_skew` into curve snapshot rows.

## Test Plan
1. Add unit coverage for skew fallback behavior (Python unit tests preferred).
- Case A: primary band has samples -> result uses `[0.2, 0.3]` (no fallback).
- Case B: primary band empty but tier-1 fallback has samples -> result uses `[0.1, 0.4]`.
- Case C: primary and tier-1 empty but tier-2 has samples -> result uses `[0, 0.5]`.
- Case D: ATM band missing -> skew remains `None` even if fallback bands exist.
- Case E: side split correctness -> call/put computed independently (fallback tier can differ by side).

2. Run validation commands.
- `python -m pytest` (or targeted test path if a dedicated test file is added).
- `python3 internal/live/options_worker.py --help` (import-time sanity check required after Python worker edits).
- `go test ./...` to ensure no regression in Go-side integration assumptions.

## Review Checklist
- Confirm formula did not change beyond requested fallback.
- Confirm fallback triggers in strict order: primary -> tier-1 -> tier-2.
- Confirm ATM band remains `[0.45, 0.55]`.
- Confirm output field names and snapshot schema remain stable.
- Confirm tests cover both normal and fallback paths.

## Risks And Mitigations
- Risk: fallback may mask data sparsity silently.
- Mitigation: keep behavior deterministic and add explicit test cases proving trigger conditions.

## Rollback Plan
- Revert only skew fallback logic in `_compute_side_skew` to prior single-band behavior.
- Re-run the same test set to validate rollback integrity.

## Structure Optimization Assessment
No project-structure refactor is needed for this task. Scope is a localized formula upgrade in the existing worker boundary.

## Approval Gate
Awaiting explicit approval before code edits and test execution.
