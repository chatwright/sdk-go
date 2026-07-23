package sdk

import "time"

// Action is a neutral interactive action (a button) captured from a bot message.
// Telegram inline buttons and WhatsApp interactive replies both normalize to it.
type Action struct {
	Label string `json:"label"` // user-visible text (Telegram button text / WhatsApp reply title)
	ID    string `json:"id"`    // stable identifier (Telegram callback_data / WhatsApp reply id)
	URL   string `json:"url"`   // set for link actions
}

// Direction identifies which side of a conversation produced a JournalEntry.
// It is a string type, not an int enum, so a JournalEntry marshals to
// human-readable JSON (see the Chatwright standard's "JSON artefacts carry
// human-readable string constants" convention) rather than a bare,
// meaningless integer.
type Direction string

const (
	DirectionUser Direction = "user"
	DirectionBot  Direction = "bot"
)

// JournalEntryKind distinguishes what a JournalEntry records. It is a string
// type for the same reason as Direction — see Direction's doc comment.
type JournalEntryKind string

const (
	// JournalEntryMessage is an inbound user message or an outbound bot
	// send/edit; MessageID, Version, Text and Actions apply.
	JournalEntryMessage JournalEntryKind = "message"
	// JournalEntryAction is an inbound action activation (a button click or
	// equivalent interactive reply); RefMessageID names the message it
	// targeted, Text carries the platform action identifier that was
	// activated.
	JournalEntryAction JournalEntryKind = "action"
	// JournalEntryUncaptured records a bot API call the emulator does not
	// simulate — it produced no observable chat content; Method names the
	// call.
	JournalEntryUncaptured JournalEntryKind = "uncaptured"
)

// JournalEntry is one chronological, structured record from a chat's
// append-only journal — the same events the chatwright runtime's platform
// emulator renders as human-readable prose, given directly to callers that
// need to reason about them structurally instead of parsing rendered text.
// It carries the emulator's full internal record, including platform-native
// identifiers and action data (e.g. Telegram callback_data via
// Actions[*][*].ID) — this is the developer/trace-level seam, not the
// actor-facing observation surface; Observation is where raw platform
// payloads are dropped before an actor ever sees them.
type JournalEntry struct {
	Direction    Direction        `json:"direction"`
	Kind         JournalEntryKind `json:"kind"`
	MessageID    int              `json:"messageId"`    // logical message identity, shared by inbound/outbound entries in this chat; 0 when Kind has no message identity of its own
	RefMessageID int              `json:"refMessageId"` // JournalEntryAction only: the message the action targeted
	Version      int              `json:"version"`      // JournalEntryMessage only: 0 = original send/inbound, N = the Nth edit
	Text         string           `json:"text"`
	Actions      [][]Action       `json:"actions"` // JournalEntryMessage only: actions attached to this entry, in platform row/col layout
	Method       string           `json:"method"`  // JournalEntryUncaptured only: the Bot API method name that was called
	At           time.Time        `json:"at"`

	// FromID is the platform-native identity of this entry's originator:
	// the Telegram user id of the client actor for a client-originated
	// entry, or the bot's own id for a bot-originated entry. It is 0 when
	// no identity is available (e.g. a pure method-call record with no
	// resolvable sender) — a platform never invents an identity it does not
	// actually know. This is what lets a run-bundle roster (see
	// Actor.PlatformIdentities) attribute every journal entry to whoever
	// produced it.
	FromID int64 `json:"fromId"`
}
