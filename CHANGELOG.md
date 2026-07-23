# Changelog

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
