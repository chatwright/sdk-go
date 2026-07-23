package sdk_test

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	sdk "chatwright.dev/sdk"
)

// goldenBundle builds a small, fully deterministic Bundle exercising every
// field the schema currently has, for TestBundleRoundTripIsDeterministic's
// round-trip and golden-file comparison. Every timestamp is built from
// time.Date, never time.Now, so it carries no monotonic reading and
// round-trips through JSON (which discards monotonic readings anyway) with
// full reflect.DeepEqual fidelity, not just byte-identical re-encoding. The
// Report is constructed by hand to the exact value the runtime's campaign
// assembly produced for this fixture before the split — the golden file is
// the byte-for-byte authority either way.
func goldenBundle() sdk.Bundle {
	fixedAt := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	cost := 0.3

	events := []sdk.LoopEvent{
		{
			Index: 0, At: fixedAt, TaskID: "onboarding", ObservationSequence: 1,
			Proposal: sdk.Proposal{Kind: sdk.ProposeSendText, Text: "Hi", Rationale: "start the conversation"},
			Usage:    sdk.Usage{Model: "claude-haiku-4-5", InputTokens: 5, OutputTokens: 2, Cost: &cost},
			Action:   sdk.ActionOutcome{Kind: sdk.ActionExecuted},
		},
		{
			// A Propose call that failed before it ever became a proposal —
			// LoopEvent.ProposeError, no Proposal/Usage/Validation/Action.
			// Exercises the field the run-bundle format v1 gained for
			// github.com/chatwright/runtime-go issue #4: a failed Propose
			// call now leaves a LoopEvent behind instead of vanishing from
			// the record.
			Index: 1, At: fixedAt.Add(time.Second), TaskID: "onboarding", ObservationSequence: 2,
			ProposeError: "actor/anthropic: request failed: context deadline exceeded",
		},
		{
			Index: 2, At: fixedAt.Add(2 * time.Second), TaskID: "onboarding", ObservationSequence: 2,
			Proposal: sdk.Proposal{Kind: sdk.ProposeTaskDone, Rationale: "onboarding confirmed"},
			Action:   sdk.ActionOutcome{Kind: sdk.ActionTaskCompleted},
		},
		{
			// A ProposeSendText proposal that violated the task's
			// machine-checkable content rules — blocked before it ever
			// reached the bot. Exercises ActionOutcomeKind's additive
			// "blocked-constraint-violation" value.
			Index: 3, At: fixedAt.Add(3 * time.Second), TaskID: "onboarding", ObservationSequence: 2,
			Proposal: sdk.Proposal{Kind: sdk.ProposeSendText, Text: "add a plasma TV", Rationale: "the actor tried an off-domain item"},
			Action:   sdk.ActionOutcome{Kind: sdk.ActionBlockedConstraintViolation, Detail: "text does not contain any allowed vocabulary term"},
		},
		{
			// A proposal requested strictly to measure whether the actor
			// would keep acting after its task's evidence-defined
			// completion criteria already held — recorded, never executed.
			// Exercises ActionOutcomeKind's additive "overshoot-probe"
			// value.
			Index: 4, At: fixedAt.Add(4 * time.Second), TaskID: "onboarding", ObservationSequence: 2,
			Proposal: sdk.Proposal{Kind: sdk.ProposeSendText, Text: "thanks!", Rationale: "the actor wanted to keep going"},
			Action:   sdk.ActionOutcome{Kind: sdk.ActionOvershootProbe, Detail: "requested after evidence-defined completion; recorded, never executed"},
		},
	}
	g := sdk.Goal{ID: "listus", Title: "Exercise onboarding", Tasks: []sdk.Task{
		{ID: "onboarding", Title: "Complete onboarding", SuccessCriteria: "user completes language selection"},
	}}
	report := sdk.Report{
		SchemaVersion: sdk.ReportSchemaVersion,
		GoalID:        "listus",
		GoalTitle:     "Exercise onboarding",
		StopReason:    "goal-complete",
		Steps:         2,
		Cost:          0.3,
		Tasks: []sdk.TaskOutcome{
			{
				TaskID: "onboarding", Title: "Complete onboarding",
				SuccessCriteria: "user completes language selection",
				Status:          "completed", Attempted: true,
			},
		},
		Findings: []sdk.Finding{
			{
				Kind: sdk.FindingConstraintViolation, TaskID: "onboarding",
				Summary:    `task "onboarding": the actor proposed text that violated its content rules; blocked before it reached the bot`,
				Evidence:   sdk.FindingEvidence{ObservationSequences: []int64{2}, LoopEventIndexes: []int{3}},
				Confidence: "mechanical",
			},
			{
				Kind: sdk.FindingActorOvershoot, TaskID: "onboarding",
				Summary:    `task "onboarding": the actor proposed another action after its evidence-defined completion criteria already held`,
				Evidence:   sdk.FindingEvidence{ObservationSequences: []int64{2}, LoopEventIndexes: []int{4}},
				Confidence: "mechanical",
			},
		},
		Usage: sdk.AggregateUsage{InputTokens: 5, OutputTokens: 2, Cost: 0.3, CallCount: 5},
	}

	chats := []sdk.ChatJournal{
		{
			ChatID: 42,
			Entries: []sdk.JournalEntry{
				{Direction: sdk.DirectionUser, Kind: sdk.JournalEntryMessage, MessageID: 1, Text: "Hi", At: fixedAt, FromID: 7},
				{
					Direction: sdk.DirectionBot, Kind: sdk.JournalEntryMessage, MessageID: 2, Text: "Choose your language:",
					Actions: [][]sdk.Action{{{Label: "English", ID: "act1"}}}, At: fixedAt.Add(time.Second), FromID: 1,
				},
				{Direction: sdk.DirectionUser, Kind: sdk.JournalEntryAction, RefMessageID: 2, Text: "act1", At: fixedAt.Add(2 * time.Second), FromID: 7},
				{Direction: sdk.DirectionBot, Kind: sdk.JournalEntryMessage, MessageID: 2, Version: 1, Text: "Howdy stranger", At: fixedAt.Add(3 * time.Second), FromID: 1},
			},
		},
	}

	observations := []sdk.RetainedObservation{
		{
			Sequence: 1,
			Observation: sdk.Observation{
				Sequence: 1, Chat: sdk.ChatRef{ChatID: 42},
				Messages: []sdk.VisibleMessage{{ID: "msg1", Actor: sdk.MessageActorUser, Text: "Hi"}},
			},
		},
		{
			Sequence: 2,
			Observation: sdk.Observation{
				Sequence: 2, PreviousSequence: 1, Chat: sdk.ChatRef{ChatID: 42},
				Messages: []sdk.VisibleMessage{
					{ID: "msg1", Actor: sdk.MessageActorUser, Text: "Hi"},
					{
						ID: "msg2", Actor: sdk.MessageActorBot, Text: "Choose your language:",
						Actions: []sdk.AvailableAction{{ID: "act1", Label: "English", SeenAt: 2}},
					},
				},
				Changes: []sdk.Change{{Kind: sdk.ChangeNewMessage, MessageID: "msg2", Actor: sdk.MessageActorBot}},
			},
		},
	}

	evidence := []sdk.DataStateEvidence{
		{
			Name: "onboarding-language", AttachmentPoint: sdk.AttachmentAfterMessage,
			Holder: "listusdb", Query: "SELECT language FROM users WHERE id = @userId",
			Params:  map[string]any{"userId": "u1"},
			Outcome: sdk.OutcomePassed, TotalRows: 1, ReturnedRows: 1,
			Preview: []sdk.Row{{"language": "en"}},
		},
	}

	actors := []sdk.Actor{
		{
			ID: "explorer", Type: sdk.ActorAIAgent, Name: "Explorer",
			PlatformIdentities: map[string]sdk.PlatformIdentity{
				"telegram": {UserID: 7, Username: "explorer_bot", FirstName: "Explorer"},
			},
			Provider: &sdk.ActorProvider{Name: "anthropic", ModelIDs: sdk.AggregateModelIDs(events)},
		},
		{
			ID: "bot", Type: sdk.ActorBot, Name: "Greetbot",
			PlatformIdentities: map[string]sdk.PlatformIdentity{
				"telegram": {UserID: 1, FirstName: "ChatwrightBot"},
			},
		},
	}

	bookmarks := []sdk.Bookmark{
		{ID: "language-picked", Title: "Language picked", Anchor: sdk.Anchor{ChatID: 42, EntryIndex: 2}},
	}
	annotations := []sdk.Annotation{
		{
			ID:        "note-1",
			Anchor:    sdk.Anchor{ChatID: 42, EntryIndex: 3, MessageID: 2, Version: 1},
			Author:    &sdk.Author{Name: "Ada Reviewer", Email: "ada@chatwright.dev"},
			CreatedAt: fixedAt.Add(10 * time.Second),
			Text:      "See how instead of $4 bot returned 4$",
		},
		{
			ID:        "note-2",
			Anchor:    sdk.Anchor{ChatID: 42, EntryIndex: 3, MessageID: 2, Version: 1},
			Author:    &sdk.Author{Name: "Sam Maintainer", Email: "sam@chatwright.dev"},
			CreatedAt: fixedAt.Add(20 * time.Second),
			Text:      "Good catch — filed as a display bug.",
			ReplyTo:   "note-1",
		},
	}

	run := sdk.Run{
		ID: "run-1", Platform: "telegram", EndpointProfile: sdk.EndpointProfilePlatformEmulated,
		Actors: actors, Chats: chats,
		Parts: []sdk.Part{
			{
				ID: "exploration", Title: "Shopping-list exploration", Kind: sdk.PartKindAIGoal,
				JournalBoundary: sdk.JournalBoundary{Chats: []sdk.ChatBoundary{
					{ChatID: 42, FirstEntry: 0, EntryCount: 4},
				}},
				AIGoal: &sdk.AIGoalSection{
					Goal:         g,
					ActorID:      "explorer",
					Events:       events,
					Observations: observations,
					Report:       report,
					Evidence:     evidence,
				},
			},
		},
		Bookmarks:   bookmarks,
		Annotations: annotations,
	}

	return sdk.Bundle{
		Format: sdk.FormatV1,
		Metadata: sdk.Metadata{
			CreatedAt: fixedAt,
			Author:    &sdk.Author{Name: "Ada Reviewer", Email: "ada@chatwright.dev"},
		},
		Runs: []sdk.Run{run},
	}
}

// TestBundleRoundTripIsDeterministic proves Write/Read round-trip a Bundle
// without loss, that writing the same Bundle twice (directly, or after
// reading it back) produces byte-identical output, and that the output
// matches a checked-in golden file — so an accidental, undeclared change to
// the schema's shape or field order is caught by a test diff rather than
// discovered by a downstream player.
func TestBundleRoundTripIsDeterministic(t *testing.T) {
	b := goldenBundle()

	var first bytes.Buffer
	if err := sdk.Write(&first, b); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	roundTripped, err := sdk.Read(bytes.NewReader(first.Bytes()))
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if !reflect.DeepEqual(b, roundTripped) {
		t.Fatalf("round-tripped bundle differs from the original:\ngot:  %+v\nwant: %+v", roundTripped, b)
	}

	var second bytes.Buffer
	if err := sdk.Write(&second, roundTripped); err != nil {
		t.Fatalf("Write(roundTripped) error = %v", err)
	}
	if first.String() != second.String() {
		t.Fatalf("Write is not deterministic across a read/write cycle:\nfirst:\n%s\nsecond:\n%s", first.String(), second.String())
	}

	const goldenPath = "testdata/bundle_golden.json"
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", goldenPath, err)
	}
	if first.String() != string(golden) {
		t.Fatalf("bundle JSON no longer matches %s — if this schema change is deliberate, update the golden file; got:\n%s", goldenPath, first.String())
	}
}

// TestBundleReadRejectsUnknownFormat proves Read rejects a "format" it does
// not recognise — older, newer, or otherwise unknown — with a typed error
// naming the value found, rather than silently unmarshalling the rest of the
// payload under today's field meanings.
func TestBundleReadRejectsUnknownFormat(t *testing.T) {
	tests := map[string]string{
		"newer":   `{"format": "https://chatwright.dev/formats/run-bundle/v2"}`,
		"older":   `{"format": "https://chatwright.dev/formats/campaign-bundle/v1"}`,
		"garbage": `{"format": "not-a-format"}`,
		"missing": `{}`,
	}
	for name, payload := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := sdk.Read(strings.NewReader(payload))
			if err == nil {
				t.Fatal("Read() error = nil, want an unknown-format error")
			}
			if !errors.Is(err, sdk.ErrUnknownBundleFormat) {
				t.Fatalf("Read() error = %v, want it to wrap ErrUnknownBundleFormat", err)
			}
		})
	}
}

// TestBundleReadRejectsUnknownPartKind proves Read rejects a Part whose kind
// it does not recognise, naming the kind and part id.
func TestBundleReadRejectsUnknownPartKind(t *testing.T) {
	payload := `{
		"format": "https://chatwright.dev/formats/run-bundle/v1",
		"metadata": {"createdAt": "2026-07-22T12:00:00Z"},
		"runs": [{
			"id": "run-1", "platform": "telegram", "endpointProfile": "platform-emulated",
			"actors": [], "chats": [],
			"parts": [{"id": "mystery", "kind": "quantum-leap", "journalBoundary": {"chats": []}}]
		}]
	}`
	_, err := sdk.Read(strings.NewReader(payload))
	if err == nil {
		t.Fatal("Read() error = nil, want an unknown-part-kind error")
	}
	if !errors.Is(err, sdk.ErrUnknownPartKind) {
		t.Fatalf("Read() error = %v, want it to wrap ErrUnknownPartKind", err)
	}
	if !strings.Contains(err.Error(), "quantum-leap") || !strings.Contains(err.Error(), "mystery") {
		t.Fatalf("Read() error = %v, want it to name the kind and part id", err)
	}
}

// TestBundleReadRejectsAIGoalPartMissingSection proves Read rejects an
// ai-goal Part with no aiGoal section, naming the part id, rather than
// handing back a Part whose AIGoal is silently nil.
func TestBundleReadRejectsAIGoalPartMissingSection(t *testing.T) {
	payload := `{
		"format": "https://chatwright.dev/formats/run-bundle/v1",
		"metadata": {"createdAt": "2026-07-22T12:00:00Z"},
		"runs": [{
			"id": "run-1", "platform": "telegram", "endpointProfile": "platform-emulated",
			"actors": [], "chats": [],
			"parts": [{"id": "exploration", "kind": "ai-goal", "journalBoundary": {"chats": []}}]
		}]
	}`
	_, err := sdk.Read(strings.NewReader(payload))
	if err == nil {
		t.Fatal("Read() error = nil, want a missing-aiGoal-section error")
	}
	if !errors.Is(err, sdk.ErrMissingAIGoalSection) {
		t.Fatalf("Read() error = %v, want it to wrap ErrMissingAIGoalSection", err)
	}
	if !strings.Contains(err.Error(), "exploration") {
		t.Fatalf("Read() error = %v, want it to name the part id", err)
	}
}

// TestBundleReadAcceptsDeterministicPartWithNoSection proves Read accepts a
// "deterministic" Part even though this package models no section for it
// yet (see PartKindDeterministic) — the kind is reserved, not rejected.
func TestBundleReadAcceptsDeterministicPartWithNoSection(t *testing.T) {
	payload := `{
		"format": "https://chatwright.dev/formats/run-bundle/v1",
		"metadata": {"createdAt": "2026-07-22T12:00:00Z"},
		"runs": [{
			"id": "run-1", "platform": "telegram", "endpointProfile": "platform-emulated",
			"actors": [], "chats": [],
			"parts": [{"id": "onboarding", "kind": "deterministic", "journalBoundary": {"chats": []}}]
		}]
	}`
	decoded, err := sdk.Read(strings.NewReader(payload))
	if err != nil {
		t.Fatalf("Read() error = %v, want a reserved deterministic part to be accepted", err)
	}
	if len(decoded.Runs) != 1 || len(decoded.Runs[0].Parts) != 1 || decoded.Runs[0].Parts[0].Kind != sdk.PartKindDeterministic {
		t.Fatalf("decoded = %+v, want one run with one deterministic part", decoded)
	}
}

// TestBundleReadToleratesDanglingAnnotationReferences proves Read accepts a
// Bundle whose Annotation.ReplyTo names an Annotation ID this Run does not
// carry, and whose Anchor.EntryIndex is out of range for the chat it names —
// bundles are hand-editable files, and Annotation's own doc comment declares
// that surfacing a dangling reference is a consumer's concern, never a Read
// error.
func TestBundleReadToleratesDanglingAnnotationReferences(t *testing.T) {
	payload := `{
		"format": "https://chatwright.dev/formats/run-bundle/v1",
		"metadata": {"createdAt": "2026-07-22T12:00:00Z"},
		"runs": [{
			"id": "run-1", "platform": "telegram", "endpointProfile": "platform-emulated",
			"actors": [], "chats": [{"chatId": 42, "entries": []}],
			"parts": [],
			"annotations": [
				{
					"id": "note-1",
					"anchor": {"chatId": 42, "entryIndex": 999},
					"createdAt": "2026-07-22T12:00:00Z",
					"text": "replies to a note that does not exist",
					"replyTo": "note-does-not-exist"
				}
			]
		}]
	}`
	decoded, err := sdk.Read(strings.NewReader(payload))
	if err != nil {
		t.Fatalf("Read() error = %v, want a dangling replyTo/out-of-range anchor to be accepted", err)
	}
	if len(decoded.Runs) != 1 || len(decoded.Runs[0].Annotations) != 1 {
		t.Fatalf("decoded = %+v, want one run with one annotation", decoded)
	}
	got := decoded.Runs[0].Annotations[0]
	if got.ReplyTo != "note-does-not-exist" || got.Anchor.EntryIndex != 999 {
		t.Fatalf("decoded annotation = %+v, want the dangling references carried through verbatim", got)
	}
}

// TestBundleContainsProfileAndPlatformLabels proves a Bundle's Run always
// names — never implies — its endpoint profile and platform (the Chatwright
// standard's "fidelity is declared" principle applied to the run-bundle
// artifact), both as struct fields and as readable keys in the encoded JSON
// a player parses.
func TestBundleContainsProfileAndPlatformLabels(t *testing.T) {
	b := goldenBundle()

	if len(b.Runs) != 1 {
		t.Fatalf("len(b.Runs) = %d, want 1", len(b.Runs))
	}
	run := b.Runs[0]
	if run.Platform != "telegram" {
		t.Fatalf("run.Platform = %q, want %q", run.Platform, "telegram")
	}
	if run.EndpointProfile != sdk.EndpointProfilePlatformEmulated {
		t.Fatalf("run.EndpointProfile = %q, want %q", run.EndpointProfile, sdk.EndpointProfilePlatformEmulated)
	}

	var buf bytes.Buffer
	if err := sdk.Write(&buf, b); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	encoded := buf.String()
	if !strings.Contains(encoded, `"platform": "telegram"`) {
		t.Fatalf("encoded bundle does not carry a readable platform label: %s", encoded)
	}
	if !strings.Contains(encoded, `"endpointProfile": "platform-emulated"`) {
		t.Fatalf("encoded bundle does not carry a readable endpointProfile label: %s", encoded)
	}
}
