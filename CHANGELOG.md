# Changelog

## 0.3.0 — 2026-07-25

- Renamed `Verdict` to `Freshness` (values unchanged: `fresh`/`stale`), and
  `ValidationOutcome.Verdict` to `ValidationOutcome.Freshness` (wire tag
  `verdict` → `freshness`). This resolves a vocabulary collision recorded in
  the `chatwright/chatwright` glossary: "verdict" now names only the
  AI-judged-assertion outcome (`passed`/`failed`/`inconclusive`/
  `unavailable`); the click-validity check this type carries is a validity
  check against the runtime's own state, not a judgement against a
  criterion, so it is "freshness". `formats/run-bundle/v1/schema.json`
  regenerated accordingly (the `freshness` property replaces `verdict`,
  still required, same enum); `testdata/bundle_golden.json` updated to
  match. This is a wire-breaking rename for any reader that only accepts
  `verdict` — readers should add `freshness` support (falling back to
  `verdict` for bundles written before this release) before upgrading to a
  runtime that writes this version.

- `ActionOutcomeKind` gained two additive values: `blocked-constraint-violation`
  (a `ProposeSendText` proposal whose text violated the active task's/goal's
  machine-checkable content rules — a vocabulary allowlist, deny-pattern or
  custom predicate — blocked before it ever reached the bot) and
  `overshoot-probe` (a proposal requested and recorded strictly to measure
  whether an actor would keep acting after its task's machine-checkable
  completion criteria already held; never submitted to the platform).
- `FindingKind` gained two additive values: `actor-overshoot` and
  `constraint-violation`, and is now itself a closed, schema-enum-constrained
  type for the first time (it was reflected as a bare open string before this
  release — an oversight relative to the rest of this module's closed-enum
  discipline, fixed here alongside the two new values it needed anyway).
  Both new finding kinds are the wire side of
  [spec/ideas/evidence-defined-completion.md](https://github.com/chatwright/chatwright/blob/main/spec/ideas/evidence-defined-completion.md)
  and
  [spec/ideas/proposal-content-constraints.md](https://github.com/chatwright/chatwright/blob/main/spec/ideas/proposal-content-constraints.md)
  in the `chatwright/chatwright` standard repository; the loop-side mechanics
  land in `chatwright/runtime-go`. `goal.StopReason`'s new
  `goal-met-by-evidence` value does NOT require a change here:
  `CampaignReport.stopReason` is a plain, unconstrained string on the wire
  (never a closed enum — see its own doc comment), so any new
  `goal.StopReason` constant is already representable without a schema
  change.
  `formats/run-bundle/v1/schema.json` regenerated accordingly (purely
  additive: two `ActionOutcomeKind` values, and `CampaignFinding.kind` newly
  closed to five values); `testdata/bundle_golden.json` extended with two
  more `LoopEvent`s (one per new `ActionOutcomeKind` value) and two more
  `Finding`s (one per new `FindingKind` value).

## 0.1.1

- `LoopEvent` gained an additive, optional `proposeError` field (`omitempty`
  string): set exactly when a provider's `Propose` call failed before it
  ever produced a proposal, so a failed call can still leave a `LoopEvent`
  behind (index, timestamp, task, observation sequence) instead of
  vanishing from the record — the wire side of fixing
  [chatwright/runtime-go#4](https://github.com/chatwright/runtime-go/issues/4).
  `Proposal`/`Usage`/`Validation`/`Action` stay their (already-existing)
  zero value in that case, so `ProposalKind` and `ActionOutcomeKind` — both
  previously assumed to always carry one of their named constants — now
  also list their Go zero value (`""`) in the schema's closed enum, the
  same treatment `Verdict` already had for "meaningless when Checked is
  false". `formats/run-bundle/v1/schema.json` regenerated accordingly
  (purely additive: one new optional property, two enums widened by one
  value each); `testdata/bundle_golden.json` extended with a third
  `LoopEvent` exercising `proposeError`.

## 0.1.0

Initial extraction from
[github.com/chatwright/chatwright](https://github.com/chatwright/chatwright).

- `chatwright.dev/sdk` now owns the run-bundle format v1 wire model: every
  type the published JSON Schema describes lives in the single root `sdk`
  package, alongside `Write`/`Read` IO and the schema generator.
- Three Go-name-only renames against the runtime's packages (JSON and schema
  are byte-identical): `observe.Actor` → `MessageActor`,
  `campaign.Evidence` → `FindingEvidence`, `datastate.Evidence` →
  `DataStateEvidence`.
- The published schema (`formats/run-bundle/v1/schema.json`) and the golden
  bundle (`testdata/bundle_golden.json`) are carried over byte-for-byte; the
  schema generator pins every published `$defs` key.
