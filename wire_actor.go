package sdk

import "time"

// ProposalKind is the typed shape of an AI provider's proposed action. It is
// a string type, not an int enum, so it marshals to human-readable JSON — in
// bundles, cassette files and everywhere else — rather than a bare,
// meaningless integer (see the Chatwright standard's "JSON artefacts carry
// human-readable string constants" convention). Its Go zero value ("") is
// itself a real, meaningful wire value, not only an unset placeholder: a
// LoopEvent whose ProposeError is set carries a zero-value Proposal (there
// was no proposal to have a Kind), and "" is what that Kind reads as.
type ProposalKind string

// Proposal kinds. See Proposal.
const (
	// ProposeSendText: send free text as the user.
	ProposeSendText ProposalKind = "send-text"
	// ProposeClick: activate a previously observed AvailableAction by its
	// opaque ID (Proposal.ActionID), as seen at Proposal.ObservationSequence.
	ProposeClick ProposalKind = "click"
	// ProposeTaskDone: the active task's success criteria are met.
	ProposeTaskDone ProposalKind = "task-done"
	// ProposeGiveUp: the active task cannot be completed; stop attempting it.
	ProposeGiveUp ProposalKind = "give-up"
)

// String renders k for diagnostics, test failure messages and reports.
func (k ProposalKind) String() string { return string(k) }

// Proposal is an AI provider's typed intent for the next action, plus its
// free-text rationale. The chatwright runtime's actor loop validates and
// executes it — nothing a provider proposes is trusted blindly.
type Proposal struct {
	Kind ProposalKind `json:"kind"`

	// Text is set for ProposeSendText: the text to send as the user.
	Text string `json:"text"`

	// ActionID is set for ProposeClick: an AvailableAction.ID drawn from the
	// observation the proposal was made against.
	ActionID string `json:"actionId"`
	// ObservationSequence is the Observation.Sequence the proposal was
	// chosen from. Required for ProposeClick (the runtime validates the
	// click against it); ignored otherwise.
	ObservationSequence int64 `json:"observationSequence"`

	// Rationale is free text explaining the choice — never private
	// chain-of-thought, just enough for a developer or the campaign report
	// to understand why the actor did this.
	Rationale string `json:"rationale"`
}

// Usage reports what one provider call cost: model identity, token counts,
// latency and, optionally, a caller-priced Cost. When Cost is set, the
// runtime feeds it to its campaign state so a configured Budgets.MaxCost is
// enforced.
type Usage struct {
	Model        string        `json:"model"`
	InputTokens  int           `json:"inputTokens"`
	OutputTokens int           `json:"outputTokens"`
	Latency      time.Duration `json:"latencyNanoseconds"`
	Cost         *float64      `json:"cost,omitempty"`
}

// ValidationOutcome is the loop's validate-step verdict for one proposal,
// carrying the runtime's own validation result verbatim when it applies.
type ValidationOutcome struct {
	// Checked is false for proposal kinds validation does not apply to
	// (ProposeSendText, ProposeTaskDone, ProposeGiveUp); Verdict and Reason
	// are meaningless when Checked is false.
	Checked bool    `json:"checked"`
	Verdict Verdict `json:"verdict"`
	Reason  string  `json:"reason"`
}

// ActionOutcomeKind classifies what happened when the loop acted on a
// proposal, or why it did not act at all. It is a string type, not an int
// enum, so it marshals to human-readable JSON (see the Chatwright standard's
// "JSON artefacts carry human-readable string constants" convention) rather
// than a bare, meaningless integer. Its Go zero value ("") is itself a real,
// meaningful wire value, not only an unset placeholder: a LoopEvent whose
// ProposeError is set carries a zero-value ActionOutcome (there was no
// action to have a Kind — the loop never got a Proposal to act on), and ""
// is what that Kind reads as.
type ActionOutcomeKind string

// Action outcome kinds. See ActionOutcome.
const (
	// ActionSkippedInvalid: the proposal failed validation (a stale click)
	// or was malformed; the loop never submitted anything to the platform.
	ActionSkippedInvalid ActionOutcomeKind = "skipped-invalid"
	// ActionExecuted: the proposed action was submitted to the platform and
	// produced an observable change (a new message, an edit, or an
	// actions-changed update).
	ActionExecuted ActionOutcomeKind = "executed"
	// ActionExecutedNoEffect: the proposed action was submitted, but the
	// next observation showed no change at all.
	ActionExecutedNoEffect ActionOutcomeKind = "executed-no-effect"
	// ActionResolutionFailed: a freshly validated proposal that the loop
	// could not resolve to a concrete platform action — e.g. no button on
	// the current message carries the validated action's label. This counts
	// as a task failure.
	ActionResolutionFailed ActionOutcomeKind = "resolution-failed"
	// ActionTaskCompleted: a ProposeTaskDone proposal was accepted; the
	// task's status moved to completed.
	ActionTaskCompleted ActionOutcomeKind = "task-completed"
	// ActionTaskGivenUp: a ProposeGiveUp proposal was accepted; the task's
	// status moved to failed.
	ActionTaskGivenUp ActionOutcomeKind = "task-given-up"
)

// String renders k for diagnostics, test failure messages and reports.
func (k ActionOutcomeKind) String() string { return string(k) }

// ActionOutcome is what actually happened when the loop tried to act on a
// Proposal.
type ActionOutcome struct {
	Kind ActionOutcomeKind `json:"kind"`
	// Detail is a human-readable explanation, set for
	// ActionSkippedInvalid/ActionResolutionFailed (why), empty otherwise.
	Detail string `json:"detail"`
}

// LoopEvent is one loop iteration's complete structured record: what was
// observed, what was proposed, how the proposal validated, what happened
// when the loop acted on it (or chose not to), and what it cost. LoopEvents
// are the loop's entire raw material for Report — nothing the report needs
// is reconstructed after the fact from logs or a transcript.
type LoopEvent struct {
	// Index is 0-based and monotonic across one loop's lifetime (not just
	// one task), so it is stable to reference from a Finding.
	Index int `json:"index"`
	// At is stamped from the loop's injected clock, never time.Now, so a
	// run's timeline is reproducible.
	At time.Time `json:"at"`
	// TaskID is the task this iteration was attempting.
	TaskID string `json:"taskId"`

	// ObservationSequence is the Observation.Sequence this iteration
	// observed before proposing — the same value a Finding's evidence links
	// back to.
	ObservationSequence int64 `json:"observationSequence"`

	Proposal Proposal `json:"proposal"`
	Usage    Usage    `json:"usage"`

	// Validation is the loop's validate-step outcome for Proposal. It is
	// only Checked for ProposeClick — the loop has nothing to validate
	// against an observation for a send-text, task-done or give-up proposal.
	Validation ValidationOutcome `json:"validation"`

	// Action is what actually happened when the loop tried to act on
	// Proposal (or why it did not).
	Action ActionOutcome `json:"action"`

	// ProposeError is set exactly when this iteration's call to the AI
	// provider's Propose failed: it carries the returned error's own
	// message (error.Error()), and Proposal, Usage, Validation and Action
	// are all their zero value — there was nothing to validate or act on.
	// Empty for every iteration that got as far as a Proposal, which is
	// most of them; this field exists so a failed Propose call still leaves
	// a LoopEvent behind (index, timestamp, task, the observation it was
	// attempting to act from) instead of vanishing from the record with
	// only a returned Go error nobody downstream of the loop ever sees
	// (github.com/chatwright/runtime-go issue #4).
	ProposeError string `json:"proposeError,omitempty"`
}
