# Code Review Issues Re-evaluation and Fix Plan (Pending Approval)

## 1. Scope
1. Re-evaluate the two review findings on resolver fallback order.
2. Keep this task strictly limited to regression repair and verification.
3. Do not introduce unrelated refactors in this change set.

## 2. Re-evaluation Result

### 2.1 Finding 1 (Python worker) is reasonable
1. Current code in `/Users/daniel/projects/starSling/internal/live/options_worker.py` uses:
   - `mapped -> inferred -> existing` in `_resolve_option_underlying`.
2. This means when metadata is missing/unavailable but feed `underlying` is valid, inferred value can override feed value.
3. In `build_options_snapshot`, the resolved underlying is used to map `underlying_price`; wrong underlying leads to invalid IV/Greeks.
4. Conclusion: the finding is valid and actionable.

### 2.2 Finding 2 (Go TUI) is reasonable
1. Current code in `/Users/daniel/projects/starSling/internal/tui/app.go` uses:
   - `resolver.Resolve -> resolver.Infer -> local infer -> existing` in `resolveOptionUnderlying`.
2. With metadata miss or resolver degradation, existing row-level `underlying` is not preferred.
3. This can distort flow event grouping/filtering logic that depends on resolved underlying.
4. Conclusion: the finding is valid and actionable.

## 3. Fix Strategy (Minimal and Safe)
1. Unify fallback policy to:
   - `metadata resolve -> existing field -> inference`.
2. Keep metadata-hit behavior unchanged.
3. Change only underlying resolver order; no behavior expansion for symbol/cp in this task.

## 4. Planned Code Changes (After Approval)
1. Update `/Users/daniel/projects/starSling/internal/live/options_worker.py`:
   - `_resolve_option_underlying` order from `mapped -> inferred -> existing` to `mapped -> existing -> inferred`.
2. Update `/Users/daniel/projects/starSling/internal/tui/app.go`:
   - `resolveOptionUnderlying` order to:
     - `resolver.Resolve -> existing -> resolver fallback infer -> local infer`.
3. Add targeted regression tests in `/Users/daniel/projects/starSling/internal/tui/app_test.go`:
   - existing underlying is preferred when resolver is nil.
   - existing underlying is preferred when resolver inference exists but metadata resolve misses.

## 5. Validation Plan
1. `go test ./internal/tui`
2. `go test ./internal/metadata`
3. `go test ./...`
4. Manual logic check:
   - confirm no behavior change when metadata resolve succeeds.
   - confirm existing underlying wins in fallback paths.

## 6. Risk and Rollback
1. Risk: some tests may currently encode inference-first behavior.
2. Mitigation: update only tests that assert fallback ordering semantics.
3. Rollback: revert only the two resolver-order edits if any regression appears.

## 7. Structure Optimization Assessment
1. Conclusion: no project-structure refactor is needed for this task.
2. Reason: this is a narrow logic-order regression fix; structural changes would increase risk and scope unnecessarily.

## 8. Approval Status
1. Pending explicit user approval before code edits.
