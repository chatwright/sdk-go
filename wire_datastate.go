package sdk

// AttachmentPoint names where in a scenario a data assertion runs, matching
// the data-state-assertions feature's "Assertion attachment points"
// behaviour (see the standard repository's spec): after a user message or
// action's registered application work has settled, immediately before a
// named checkpoint is published, or at the end of a branch or scenario
// fragment.
type AttachmentPoint string

const (
	// AttachmentAfterMessage is a data assertion attached after a message or
	// action and its registered application work has settled.
	AttachmentAfterMessage AttachmentPoint = "after-message"
	// AttachmentCheckpoint is a data assertion that gates a named
	// checkpoint's publication.
	AttachmentCheckpoint AttachmentPoint = "checkpoint"
	// AttachmentBranchCompletion is a data assertion run at the end of a
	// branch or scenario fragment.
	AttachmentBranchCompletion AttachmentPoint = "branch-completion"
)

// Outcome is the pass/fail result of one executed assertion.
type Outcome string

const (
	OutcomePassed Outcome = "passed"
	OutcomeFailed Outcome = "failed"
)

// Row is one returned record. Field values may themselves be nested
// map[string]any or []any, matching an embedded-document shape such as a
// parent-scoped list record and its nested `items` field.
type Row map[string]any

// DataStateEvidence is the canonical, JSON-serialisable record of one
// executed data-state assertion: the exact DTQL query and parameters, the
// holder it ran against, its pass/fail outcome, and a bounded, redacted,
// normalised preview of the rows it returned. Every exported field carries
// an explicit lower-camel-case `json` tag — this type reaches a run bundle
// (via AIGoalSection.Evidence) and the whole run-bundle wire is uniformly
// camelCase.
//
// In the chatwright runtime this type is datastate.Evidence; it is renamed
// here — Go name only, the wire shape is identical — to keep it distinct
// from FindingEvidence in this single package.
type DataStateEvidence struct {
	// Name is the triggering assertion's stable identity, correlating this
	// evidence to the message, checkpoint or branch that attached it.
	Name string `json:"name"`
	// AttachmentPoint is where in the scenario this assertion ran.
	AttachmentPoint AttachmentPoint `json:"attachmentPoint"`
	// Holder is the resolved holder name the query ran against (even an
	// unresolved request records the name that was asked for).
	Holder string `json:"holder"`
	// Query is the concrete DTQL text executed.
	Query string `json:"query"`
	// Params is a detached copy of the query's named parameters.
	Params map[string]any `json:"params"`
	// Outcome is OutcomePassed or OutcomeFailed.
	Outcome Outcome `json:"outcome"`
	// FailureMessage is set when Outcome is OutcomeFailed: holder
	// resolution, query execution or the expectation's failure message.
	FailureMessage string `json:"failureMessage"`
	// TotalRows is how many rows the query returned before any preview
	// bound was applied.
	TotalRows int `json:"totalRows"`
	// ReturnedRows is how many rows are present in Preview.
	ReturnedRows int `json:"returnedRows"`
	// Truncated is true when Preview omits rows (TotalRows > ReturnedRows)
	// or drops fields from at least one previewed row.
	Truncated bool `json:"truncated"`
	// Preview is the bounded, redacted, normalised recordset. Redacted
	// fields are present with their value replaced by the runtime's
	// redaction placeholder rather than omitted, so evidence still declares
	// which fields exist.
	Preview []Row `json:"preview"`
	// RedactedFields lists the field names configured for redaction (the
	// declared policy), regardless of whether any previewed row contained
	// them.
	RedactedFields []string `json:"redactedFields"`
	// ExcludedFields lists the field names normalisation removed from the
	// comparison basis. They remain visible in Preview: exclusion only
	// means "not part of the assertion", never "hidden from evidence".
	ExcludedFields []string `json:"excludedFields"`
}
