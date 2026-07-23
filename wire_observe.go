package sdk

// ChatRef identifies the chat an Observation projects. It carries
// Chatwright's own chat identity — never a raw platform chat ID scraped from
// the wire.
type ChatRef struct {
	ChatID int64 `json:"chatId"`
}

// MessageActor identifies which side of a conversation produced a
// VisibleMessage. It is a string type, not an int enum, so it marshals to
// human-readable JSON (see the Chatwright standard's "JSON artefacts carry
// human-readable string constants" convention) rather than a bare,
// meaningless integer.
//
// In the chatwright runtime this enum is observe.Actor; it is renamed here —
// Go name only, the wire values are identical — because this package's
// roster entry type already owns the name Actor.
type MessageActor string

const (
	MessageActorUser MessageActor = "user"
	MessageActorBot  MessageActor = "bot"
)

// String renders a for diagnostics and test failure messages.
func (a MessageActor) String() string { return string(a) }

// VisibleMessage is one user-visible logical message: stable identity across
// edits, a monotonic version and an edited flag, plus the actions currently
// attached to it. Only normalized text and action labels are carried — no
// platform-native message IDs, callback data or wire payloads (see the
// standard repository's observation-model/visible-conversation feature).
type VisibleMessage struct {
	ID      string            `json:"id"`      // stable synthetic Chatwright identity for this logical message, e.g. "msg7"
	Version int               `json:"version"` // monotonic version of this logical message; 0 for the original send
	Edited  bool              `json:"edited"`  // true once Version has advanced past 0
	Actor   MessageActor      `json:"actor"`   // who produced the message
	Text    string            `json:"text"`
	Actions []AvailableAction `json:"actions"` // interactions currently attached to this message
}

// AvailableAction is a generic, opaque interaction an actor can take: a
// stable Chatwright ID and its user-visible label. Platform-native callback
// data, request payloads and button coordinates are never exposed here — an
// authorised developer inspector reaches those through the platform's
// journal/transcript trace (JournalEntry), not through this type (see the
// standard repository's observation-model/actor-actions feature).
type AvailableAction struct {
	ID     string `json:"id"`     // opaque, stable Chatwright action identity
	Label  string `json:"label"`  // user-visible text
	SeenAt int64  `json:"seenAt"` // the Observation.Sequence this action was (re)issued at
}

// ChangeKind classifies one entry in an Observation's Changes feed. It is a
// string type, not an int enum, so it marshals to human-readable JSON (see
// the Chatwright standard's "JSON artefacts carry human-readable string
// constants" convention) rather than a bare, meaningless integer.
type ChangeKind string

const (
	// ChangeNewMessage: a logical message not present in the previous
	// Observation now exists.
	ChangeNewMessage ChangeKind = "new-message"
	// ChangeMessageEdited: an existing logical message's Version advanced.
	ChangeMessageEdited ChangeKind = "edited-message"
	// ChangeActionsChanged: an existing logical message's available actions
	// changed without its Version advancing.
	ChangeActionsChanged ChangeKind = "actions-changed"
)

// String renders k for diagnostics and test failure messages.
func (k ChangeKind) String() string { return string(k) }

// Change is one explicit, structured difference between an Observation and
// the previous Observation, computed by the runtime's observation engine so
// actors reason about what changed without diffing two Observations
// themselves (see the standard repository's observation-model/
// observation-lineage feature).
type Change struct {
	Kind      ChangeKind   `json:"kind"`
	MessageID string       `json:"messageId"`
	Actor     MessageActor `json:"actor"`
	// PreviousVersion is set for ChangeMessageEdited: the message's Version
	// before this change.
	PreviousVersion int `json:"previousVersion"`
	// Version is the message's Version after this change (ChangeNewMessage,
	// ChangeMessageEdited) or its current, unchanged Version
	// (ChangeActionsChanged).
	Version int `json:"version"`
}

// Observation is one platform-neutral snapshot of a chat's visible
// conversation and available actions, with explicit lineage back to the
// previous Observation. Observations are produced by the runtime's
// observation engine — actors never build or diff one by hand.
type Observation struct {
	// Sequence is monotonic per engine, starting at 1.
	Sequence int64 `json:"sequence"`
	// PreviousSequence is the Sequence of the Observation this one
	// supersedes; 0 for an engine's first Observation.
	PreviousSequence int64   `json:"previousSequence"`
	Chat             ChatRef `json:"chat"`
	// Messages is chronological, oldest to newest: one entry per currently
	// visible logical message, at its current (possibly-edited) version.
	Messages []VisibleMessage `json:"messages"`
	// Changes is empty for an engine's first Observation; otherwise the
	// explicit differences since PreviousSequence.
	Changes []Change `json:"changes"`
}

// Verdict is the deterministic outcome of validating a click proposal
// against the runtime's current journal state. It is a string type, not an
// int enum, so it marshals to human-readable JSON (see the Chatwright
// standard's "JSON artefacts carry human-readable string constants"
// convention) rather than a bare, meaningless integer.
type Verdict string

const (
	// VerdictFresh: the proposed action is present, unchanged, in the
	// engine's current projection.
	VerdictFresh Verdict = "fresh"
	// VerdictStale: the proposed action is not present in the engine's
	// current projection — its source observation is out of date, or was
	// never issued by that engine at all.
	VerdictStale Verdict = "stale"
)

// String renders v for diagnostics and test failure messages.
func (v Verdict) String() string { return string(v) }
