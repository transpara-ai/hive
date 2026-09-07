# Confirmed work and peer execution hosts

Designed outcome: let an internal operator inspect the short brief before
implementation, choose Codex or Claude independently from model and reasoning
effort, recover interrupted work, and inspect a verified local result.

`POST /api/civilization/v1/intake` accepts optional `selection` fields
`provider`, `model`, and `reasoning_effort`. New HTTP intake always requests brief
confirmation. Routing ends at `awaiting_confirmation`; the reconciler excludes
that state. `POST /work/{workID}/confirm` accepts `{"brief_id":"…"}` using the
bound request's `idempotency_key`. Only a matching brief queues implementation;
duplicate confirmation is harmless. Older synchronous callers and stored work
retain their existing execution semantics.

`ProviderRouter` resolves invocation override → configured provider default →
provider default. CLI errors are final for that invocation. Both adapters use
bounded execution, restricted child environments and request-bound receipts;
the digest includes provider/model/effort and executable/settings identities.
Codex retains its existing receipt directory. Older receipts whose digest did
not bind selection are rejected visibly rather than ignored or silently rerun;
migrate inactive work only at a safe boundary. Claude uses a separate receipt
subdirectory. Do not delete historical receipts to bypass a mismatch.
The durable result carries requested and effective selection and its source.
When native output does not report a default model, the field stays empty rather
than inventing an identity. Claude reports a single observed model when available;
explicit aliases that resolve to another identifier are rejected with guidance
to use the exact identifier. No automatic fallback or Fable/advisor selection.

Both primary hosts receive the exact external skill bytes from the caller.
At startup Hive reads `CIVILIZATION_TLC_LOCK_FILE` (default
`/etc/civilization/tlc.lock.json`) and `CIVILIZATION_TLC_SKILL_FILE` (default
`/var/lib/civilization/codex/skills/tlc/SKILL.md`), checks the external digest and
passes that context to routing. This avoids host-specific discovery assumptions
without embedding a version pin or workflow body in Hive.

The existing Codex executable is also the independent verification sandbox,
regardless of primary model host. `CIVILIZATION_PROVIDER` selects the default
primary host; optional `CIVILIZATION_CODEX_MODEL` and `CIVILIZATION_CLAUDE_MODEL`
configure provider defaults. Claude registration additionally requires
`CIVILIZATION_CLAUDE_PATH`, `CIVILIZATION_CLAUDE_SHA256`,
`CIVILIZATION_CLAUDE_SETTINGS_FILE` and `CIVILIZATION_CLAUDE_SETTINGS_SHA256`.
Claude settings must enable fail-closed sandboxing and forbid unsandboxed
commands. Safe mode disables automatic customizations, plugins and hooks;
restricted mode excludes project/user configuration. Routing/review
have read tools only. Implementation may use sandboxed Bash and file tools.
Do not enable this adapter until the actual host image and managed policy pass
credential-containment and execution-isolation tests. This change neither
installs credentials nor prepares or activates that production policy.

With publication disabled, Hive independently verifies the exact reviewed
worktree, captures the bounded artifact, rechecks its digest, then persists it
with state `prepared`. `GET /work/{workID}/artifact` serves that recorded result
without repository or provider effects. The state is terminal for reconciliation;
publication still requires a separate authorized operating transition.

Verification: provider selection/CLI fixtures and receipt replay; confirmation
and invalid intake tests; real Site/Hive HTTP + Git + persisted signed EventGraph
journeys in Platform's `tests/civilizationjourney`; native `make verify`.
SQLite and PostgreSQL share the Civilization event adapter. The integration
fixture validates SQLite persistence and does not qualify production PostgreSQL,
real accounts, Claude's sandbox or repository-specific image toolchains.
