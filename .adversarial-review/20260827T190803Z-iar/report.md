# Internal Adversarial Review

- Gate: IAR
- Repository: transpara-ai/hive
- Draft PR: #306
- Exact reviewed head: `dfb337af0ad7601ffe39ccddbe8a8fd4fca00c26`
- Author family: Codex/OpenAI
- Result: pass
- BLOCKER_COUNT: 0
- Live-head equality: local, pushed branch and draft PR head matched at review time
- Base: authenticated PR base on `main`
- Design: `TLC51-CIVILIZATION-GATE-GOVERNANCE-DESIGN@0.4.0`, Git blob `14cc032d3252855d264d28b7fbd7cb57048fc82b`
- Factory Order: Git blob `e9f75ca5a273c22e281d9a6a05a7844fe0fca878`

## Scope and changed files

- `.tlc/tlc51-migration.blocked.json`
- `pkg/hive/events.go`
- `pkg/hive/factory_tlc51_adapters_test.go`
- `pkg/hive/factory_tlc51_client.go`
- `pkg/hive/factory_tlc51_daemon.go`
- `pkg/hive/factory_tlc51_eventgraph.go`
- `pkg/hive/factory_tlc51_startup.go`
- `pkg/hive/factory_tlc51_startup_test.go`
- `pkg/hive/factory_tlc51_work.go`
- `pkg/hive/factoryv1/tlc51.go`
- `pkg/hive/factoryv1/tlc51_effect.go`
- `pkg/hive/factoryv1/tlc51_projection.go`
- `pkg/hive/factoryv1/tlc51_scheduler.go`
- `pkg/hive/factoryv1/tlc51_test.go`
- `pkg/hive/operator_api.go`
- `pkg/hive/operator_factory_tlc51_api.go`
- `pkg/hive/operator_factory_tlc51_api_test.go`

The exact diff was checked for design/Factory Order mismatch, path-boundary drift, authority leaks, stale generated artifacts, weak validation, runtime/protected-action drift, reviewer-family assumptions and closure overclaims. The PR remained draft. The later commit containing this report is mechanical review evidence only; the implementation head above is the exact IAR subject.

## Validation

- `make verify`: pass. Full build, test and vet suite passed.
- `go test -race ./pkg/hive/factoryv1 ./pkg/hive -run 'TLC51|FactoryTLC51'`: pass. TLC 5.1 runtime seams passed targeted race tests.

## Findings and dispositions

- IAR-HIVE-TLC51-001 (blocker) — accepted_fixed: Hive rejected real TLC plans with empty adapter-selected actor-admission lists, while independent plans previously carried the author family. Evidence: The parser now admits an empty adapter-selected set, enforces non-empty allow-lists when present and always excludes author families for independent work.
- IAR-HIVE-TLC51-002 (blocker) — accepted_fixed: Effect execution did not require the effect to be planned and terminal/provider receipts were not bound to the complete operation tuple. Evidence: Effect, subject, gate receipt, operation, idempotency key, attempt, provider identity, UTC time and digest are now closed and verified, including crash recovery.
- IAR-HIVE-TLC51-003 (blocker) — accepted_fixed: An effect driver could present a receipt digest that had not been durably recorded as a passing TLC decision. Evidence: Hive now requires an exact, non-invalidated, all-predicate-pass decision event carrying authority evidence for the exact effect before invocation.

## Residual risks retained

- TLC51-RR-ORG-CONTROLS
- TLC51-RR-MUTABLE-PROVIDER-RECORDS
- TLC51-RR-APP-ENVIRONMENT-CAPABILITY
- TLC51-RR-DUAL-PROTOCOL-RUNTIME
- TLC51-RR-FACTORY-BINARY-SOURCE
- TLC51-RR-NONATOMIC-MULTIREPO-CUTOVER
- TLC51-RR-NONATOMIC-SETTINGS-API
- TLC51-RR-UNTRUSTED-GITHUB-CONTROLLER
- TLC51-RR-TLC-REPOSITORY-CONTROLS

These are implementation-verification obligations and fail-closed future stops. They are not satisfied or closed states.

## Non-authorizations

- PR readiness
- merge
- release or tag
- installation or distribution
- pilot or adoption
- workflow activation or settings enforcement
- runtime or deployment
- canary or rollout
- rollback or retirement
- deletion, archival, or issue closure
- any other protected effect

IAR is same-family evidence. It does not satisfy CFAR, create PR readiness, or authorize any protected effect.
