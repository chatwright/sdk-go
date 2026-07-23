# Changelog

## Unreleased

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
