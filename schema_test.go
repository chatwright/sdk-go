package bundle_test

import (
	"os"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/chatwright/chatwright/internal/schemagen"
)

// schemaPath is repository-root-relative to this package (bundle/) — the
// committed schema internal/schemagen/gen writes and this file gates.
const schemaPath = "../formats/run-bundle/v1/schema.json"

// compileSchema compiles the committed schema file, shared by every test in
// this file (and bundle_e2e_test.go) that validates a Bundle against it.
func compileSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	sch, err := jsonschema.NewCompiler().Compile(schemaPath)
	if err != nil {
		t.Fatalf("compile %s: %v", schemaPath, err)
	}
	return sch
}

// validateBundleFile validates the Bundle JSON document at path against
// schema, failing t with the validator's own error tree on a mismatch.
func validateBundleFile(t *testing.T, schema *jsonschema.Schema, path string) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	inst, err := jsonschema.UnmarshalJSON(f)
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	if err := schema.Validate(inst); err != nil {
		t.Fatalf("%s does not validate against %s:\n%v", path, schemaPath, err)
	}
}

// TestGoldenBundleValidatesAgainstSchema proves the committed schema
// (formats/run-bundle/v1/schema.json) accepts the golden bundle this
// package's own Write produces — see bundle.go's package doc comment for how
// the schema is generated and gated. The e2e counterpart to this test —
// validating a Bundle produced by a real campaign run, not a hand-built
// fixture — lives in bundle_e2e_test.go's
// TestScriptedCampaignBundleAgainstGreetbotEndToEnd.
func TestGoldenBundleValidatesAgainstSchema(t *testing.T) {
	validateBundleFile(t, compileSchema(t), "testdata/bundle_golden.json")
}

// TestSchemaRegenerationMatchesCommittedFile is schemagen's drift guard, the
// same pattern TestBundleRoundTripIsDeterministic's own golden-file
// comparison uses: regenerating the schema from today's Go types must
// byte-for-byte match the committed file, so an undeclared type change is
// caught by a test diff rather than a stale schema silently shipping.
// Regenerate with `go generate ./bundle/...` (or, equivalently,
// `go run ./internal/schemagen/gen` from the repository root) and commit
// the result when a change is deliberate.
func TestSchemaRegenerationMatchesCommittedFile(t *testing.T) {
	schema, err := schemagen.Generate()
	if err != nil {
		t.Fatalf("schemagen.Generate() error = %v", err)
	}
	got, err := schemagen.Marshal(schema)
	if err != nil {
		t.Fatalf("schemagen.Marshal() error = %v", err)
	}

	committed, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", schemaPath, err)
	}
	if string(got) != string(committed) {
		t.Fatalf("%s is out of date relative to the Go types it is generated from — "+
			"run `go generate ./bundle/...` and commit the result", schemaPath)
	}
}
