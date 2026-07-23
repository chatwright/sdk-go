package sdk

import "time"

// ReportSchemaVersion is the current version of Report's JSON shape. Bump it
// whenever Report changes in a way a consumer must branch on, and never
// reinterpret an old SchemaVersion's fields under a new meaning.
const ReportSchemaVersion = 1

// Report is one campaign run's complete, evidence-linked outcome: versioned,
// JSON-serialisable, and portable — a consumer needs nothing but this value
// (plus, optionally, the trace/transcript a Finding's evidence points at) to
// understand what an actor attempted, what it found, and how sure the report
// is of each conclusion. Reports are assembled by the chatwright runtime
// from a completed (or budget-stopped) actor loop run.
type Report struct {
	// SchemaVersion is always ReportSchemaVersion for a Report the runtime
	// produced; a consumer reading an older or newer value should not
	// assume today's field meanings.
	SchemaVersion int `json:"schemaVersion"`

	GoalID    string `json:"goalId"`
	GoalTitle string `json:"goalTitle"`

	// StopReason is why the campaign stopped (e.g. "goal-complete",
	// "budget-steps"), carried as a plain string so Report never imports
	// the runtime's Go type into its own JSON contract.
	StopReason string        `json:"stopReason"`
	Steps      int           `json:"steps"`
	Cost       float64       `json:"cost,omitempty"`
	Elapsed    time.Duration `json:"elapsedNanoseconds"`

	Tasks    []TaskOutcome `json:"tasks"`
	Findings []Finding     `json:"findings"`

	Usage AggregateUsage `json:"usage"`
}

// TaskOutcome is one task's result within the campaign, evidence-grounded:
// Attempted reflects whether the loop actually recorded any LoopEvent for
// this task, not merely its terminal status.
type TaskOutcome struct {
	TaskID          string `json:"taskId"`
	Title           string `json:"title,omitempty"`
	SuccessCriteria string `json:"successCriteria,omitempty"`
	// Status is the task's lifecycle status (e.g. "pending", "completed"),
	// carried as a plain string for the same reason as Report.StopReason.
	Status string `json:"status"`
	// Attempted is true once at least one LoopEvent was recorded for this
	// task.
	Attempted bool `json:"attempted"`
	// FailureCount is how many times the campaign recorded a failure for
	// this task.
	FailureCount int `json:"failureCount"`
}

// FindingKind classifies one Finding. This slice supports exactly the three
// kinds mechanical evidence (plus an explicit caller hook) can ground.
type FindingKind string

// Finding kinds. See FindingKind.
const (
	// FindingVerifiedDefect: the actor acted and the observed outcome was
	// wrong — backed by deterministic or DTQL evidence, or a
	// caller-supplied classification; mechanics alone cannot derive this
	// kind.
	FindingVerifiedDefect FindingKind = "verified-defect"
	// FindingAINavigationFailure: the task did not complete, and its
	// history shows the actor's own proposals going stale or invalid — the
	// bot was never shown to be at fault.
	FindingAINavigationFailure FindingKind = "ai-navigation-failure"
	// FindingCoverageGap: a task the campaign never attempted, or never
	// concluded, before it stopped — a gap in evidence, not a claim about
	// the bot.
	FindingCoverageGap FindingKind = "coverage-gap"
)

// FindingEvidence links a Finding back to the observations and loop events
// that ground it, so a developer can navigate from a claim to its proof.
// Both slices may be empty — e.g. a coverage-gap finding for a task that was
// never attempted has nothing to link to; that is precisely what makes it a
// gap.
//
// In the chatwright runtime this type is campaign.Evidence; it is renamed
// here — Go name only, the wire shape is identical — to keep it distinct
// from DataStateEvidence in this single package.
type FindingEvidence struct {
	// ObservationSequences are Observation.Sequence values.
	ObservationSequences []int64 `json:"observationSequences,omitempty"`
	// LoopEventIndexes are LoopEvent.Index values.
	LoopEventIndexes []int `json:"loopEventIndexes,omitempty"`
}

// Finding is one reportable outcome of the campaign: a claim, classified,
// scoped to a task, and linked to the evidence that grounds it.
type Finding struct {
	Kind    FindingKind `json:"kind"`
	TaskID  string      `json:"taskId"`
	Summary string      `json:"summary"`

	Evidence FindingEvidence `json:"evidence"`

	// Confidence distinguishes how the Finding was derived: "mechanical"
	// for the deterministic rules the runtime's report assembly applies
	// itself, or a caller's own label (e.g. "dtql-verified") for
	// caller-supplied findings.
	Confidence string `json:"confidence,omitempty"`
}

// AggregateUsage sums the Usage of every LoopEvent a Report was assembled
// from.
type AggregateUsage struct {
	InputTokens  int     `json:"inputTokens"`
	OutputTokens int     `json:"outputTokens"`
	Cost         float64 `json:"cost,omitempty"`
	// CallCount is the number of provider calls the campaign made — i.e.
	// the number of LoopEvents.
	CallCount int `json:"callCount"`
}
