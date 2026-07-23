package sdk

import "time"

// Task is one trackable unit of work inside a Goal. Success is judged by
// prose SuccessCriteria — the contract never prescribes the bot commands or
// callback data used to satisfy it. DependsOn names other Task IDs in the
// same Goal that must be completed before this task becomes eligible for
// activation. Milestones names checkpoints this task's completion may reach;
// the reporting layer, not this type, interprets them.
type Task struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	DependsOn       []string `json:"dependsOn"`
	SuccessCriteria string   `json:"successCriteria"`
	Milestones      []string `json:"milestones"`
}

// Goal is one campaign's product-level intent: a natural-language outcome
// broken into Tasks, plus the Constraints and Budgets that bound how an
// actor may pursue it. A Goal describes intent, never platform mechanics —
// see the standard repository's goal-and-task-contract feature and its
// goal-does-not-leak-platform-mechanics acceptance criterion.
type Goal struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tasks       []Task   `json:"tasks"`
	Constraints []string `json:"constraints"`
	Budgets     Budgets  `json:"budgets"`
}

// Budgets bounds one campaign run. Every numeric field's zero value means
// "no limit"; a negative value is invalid. MaxCost is the one genuinely
// optional field: nil means cost is not budgeted at all.
type Budgets struct {
	// MaxSteps caps the number of steps the runtime's campaign state counts.
	// Zero means unlimited.
	MaxSteps int `json:"maxSteps"`

	// MaxDuration caps wall-clock time elapsed since the campaign started,
	// measured by the runtime's injected clock. Zero means unlimited.
	MaxDuration time.Duration `json:"maxDurationNanoseconds"`

	// MaxRepeatedFailures caps how many times a single task may fail before
	// the campaign stops. Zero means unlimited.
	MaxRepeatedFailures int `json:"maxRepeatedFailures"`

	// MaxCost optionally caps spend against the campaign (tokens, currency
	// or another caller-defined unit — whatever unit the runtime accrues).
	// Nil means cost is not budgeted.
	MaxCost *float64 `json:"maxCost"`
}
