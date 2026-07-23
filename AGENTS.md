# chatwright.dev/sdk — instructions for AI agents and humans

- This repository follows the conventions of the Chatwright standard
  repository's AGENTS.md
  ([github.com/chatwright/chatwright](https://github.com/chatwright/chatwright)).
- Docs use British English; Go code/comments may use American English; never
  mixed within a file.
- Go: `gofmt` clean, `go vet ./...`, `go test -race ./...` before pushing.
- The wire format is final: `formats/run-bundle/v1/schema.json` and
  `testdata/bundle_golden.json` are byte-for-byte authoritative — a diff in
  either means a bug in your change, never a reason to edit them.
- The drift-guard test (`TestSchemaRegenerationMatchesCommittedFile`) and
  `go generate .` are the only supported way to regenerate the schema;
  never hand-edit the committed file.
