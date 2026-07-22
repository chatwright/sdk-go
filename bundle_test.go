package bundle_test

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/chatwright/chatwright/actor"
	"github.com/chatwright/chatwright/bundle"
	"github.com/chatwright/chatwright/campaign"
	"github.com/chatwright/chatwright/datastate"
	"github.com/chatwright/chatwright/goal"
	"github.com/chatwright/chatwright/observe"
	"github.com/chatwright/chatwright/platform"
)

// goldenBundle builds a small, fully deterministic Bundle exercising every
// field the schema currently has, for TestBundleRoundTripIsDeterministic's
// round-trip and golden-file comparison. Every timestamp is built from
// time.Date, never time.Now, so it carries no monotonic reading and
// round-trips through JSON (which discards monotonic readings anyway) with
// full reflect.DeepEqual fidelity, not just byte-identical re-encoding.
func goldenBundle() bundle.Bundle {
	fixedAt := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	cost := 0.3

	events := []actor.LoopEvent{
		{
			Index: 0, At: fixedAt, TaskID: "onboarding", ObservationSequence: 1,
			Proposal: actor.Proposal{Kind: actor.ProposeSendText, Text: "Hi", Rationale: "start the conversation"},
			Usage:    actor.Usage{Model: "claude-haiku-4-5", InputTokens: 5, OutputTokens: 2, Cost: &cost},
			Action:   actor.ActionOutcome{Kind: actor.ActionExecuted},
		},
		{
			Index: 1, At: fixedAt.Add(time.Second), TaskID: "onboarding", ObservationSequence: 2,
			Proposal: actor.Proposal{Kind: actor.ProposeTaskDone, Rationale: "onboarding confirmed"},
			Action:   actor.ActionOutcome{Kind: actor.ActionTaskCompleted},
		},
	}
	g := goal.Goal{ID: "listus", Title: "Exercise onboarding", Tasks: []goal.Task{
		{ID: "onboarding", Title: "Complete onboarding", SuccessCriteria: "user completes language selection"},
	}}
	snapshot := goal.CampaignSnapshot{
		GoalID:     "listus",
		Statuses:   map[string]goal.TaskStatus{"onboarding": goal.TaskCompleted},
		Steps:      2,
		Cost:       0.3,
		Stopped:    true,
		StopReason: goal.StopGoalComplete,
	}
	report := campaign.Assemble(campaign.AssembleInput{Goal: g, Campaign: snapshot, Events: events})

	chats := []bundle.ChatJournal{
		{
			ChatID: 42,
			Entries: []platform.JournalEntry{
				{Direction: platform.DirectionUser, Kind: platform.JournalEntryMessage, MessageID: 1, Text: "Hi", At: fixedAt, FromID: 7},
				{
					Direction: platform.DirectionBot, Kind: platform.JournalEntryMessage, MessageID: 2, Text: "Choose your language:",
					Actions: [][]platform.Action{{{Label: "English", ID: "act1"}}}, At: fixedAt.Add(time.Second), FromID: 1,
				},
				{Direction: platform.DirectionUser, Kind: platform.JournalEntryAction, RefMessageID: 2, Text: "act1", At: fixedAt.Add(2 * time.Second), FromID: 7},
				{Direction: platform.DirectionBot, Kind: platform.JournalEntryMessage, MessageID: 2, Version: 1, Text: "Howdy stranger", At: fixedAt.Add(3 * time.Second), FromID: 1},
			},
		},
	}

	observations := []bundle.RetainedObservation{
		{
			Sequence: 1,
			Observation: observe.Observation{
				Sequence: 1, Chat: observe.ChatRef{ChatID: 42},
				Messages: []observe.VisibleMessage{{ID: "msg1", Actor: observe.ActorUser, Text: "Hi"}},
			},
		},
		{
			Sequence: 2,
			Observation: observe.Observation{
				Sequence: 2, PreviousSequence: 1, Chat: observe.ChatRef{ChatID: 42},
				Messages: []observe.VisibleMessage{
					{ID: "msg1", Actor: observe.ActorUser, Text: "Hi"},
					{
						ID: "msg2", Actor: observe.ActorBot, Text: "Choose your language:",
						Actions: []observe.AvailableAction{{ID: "act1", Label: "English", SeenAt: 2}},
					},
				},
				Changes: []observe.Change{{Kind: observe.ChangeNewMessage, MessageID: "msg2", Actor: observe.ActorBot}},
			},
		},
	}

	evidence := []datastate.Evidence{
		{
			Name: "onboarding-language", AttachmentPoint: datastate.AttachmentAfterMessage,
			Holder: "listusdb", Query: "SELECT language FROM users WHERE id = @userId",
			Params:  map[string]any{"userId": "u1"},
			Outcome: datastate.OutcomePassed, TotalRows: 1, ReturnedRows: 1,
			Preview: []datastate.Row{{"language": "en"}},
		},
	}

	actors := []bundle.Actor{
		{
			ID: "explorer", Type: bundle.ActorAIAgent, Name: "Explorer",
			PlatformIdentities: map[string]bundle.PlatformIdentity{
				"telegram": {UserID: 7, Username: "explorer_bot", FirstName: "Explorer"},
			},
			Provider: &bundle.ActorProvider{Name: "anthropic", ModelIDs: bundle.AggregateModelIDs(events)},
		},
		{
			ID: "bot", Type: bundle.ActorBot, Name: "Greetbot",
			PlatformIdentities: map[string]bundle.PlatformIdentity{
				"telegram": {UserID: 1, FirstName: "ChatwrightBot"},
			},
		},
	}

	bookmarks := []bundle.Bookmark{
		{ID: "language-picked", Title: "Language picked", Anchor: bundle.Anchor{ChatID: 42, EntryIndex: 2}},
	}
	annotations := []bundle.Annotation{
		{
			ID:        "note-1",
			Anchor:    bundle.Anchor{ChatID: 42, EntryIndex: 3, MessageID: 2, Version: 1},
			Author:    &bundle.Author{Name: "Ada Reviewer", Email: "ada@chatwright.dev"},
			CreatedAt: fixedAt.Add(10 * time.Second),
			Text:      "See how instead of $4 bot returned 4$",
		},
		{
			ID:        "note-2",
			Anchor:    bundle.Anchor{ChatID: 42, EntryIndex: 3, MessageID: 2, Version: 1},
			Author:    &bundle.Author{Name: "Sam Maintainer", Email: "sam@chatwright.dev"},
			CreatedAt: fixedAt.Add(20 * time.Second),
			Text:      "Good catch — filed as a display bug.",
			ReplyTo:   "note-1",
		},
	}

	run := bundle.SingleAIGoalRun(bundle.SingleAIGoalRunInput{
		RunID: "run-1", Platform: "telegram", EndpointProfile: bundle.EndpointProfilePlatformEmulated,
		Actors: actors, Chats: chats,
		PartID: "exploration", PartTitle: "Shopping-list exploration",
		ActorID:      "explorer",
		Goal:         g,
		Events:       events,
		Observations: observations,
		Report:       report,
		Evidence:     evidence,
		Bookmarks:    bookmarks,
		Annotations:  annotations,
	})

	return bundle.Bundle{
		Format: bundle.FormatV1,
		Metadata: bundle.Metadata{
			CreatedAt: fixedAt,
			Author:    &bundle.Author{Name: "Ada Reviewer", Email: "ada@chatwright.dev"},
		},
		Runs: []bundle.Run{run},
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
	if err := bundle.Write(&first, b); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	roundTripped, err := bundle.Read(bytes.NewReader(first.Bytes()))
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if !reflect.DeepEqual(b, roundTripped) {
		t.Fatalf("round-tripped bundle differs from the original:\ngot:  %+v\nwant: %+v", roundTripped, b)
	}

	var second bytes.Buffer
	if err := bundle.Write(&second, roundTripped); err != nil {
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
			_, err := bundle.Read(strings.NewReader(payload))
			if err == nil {
				t.Fatal("Read() error = nil, want an unknown-format error")
			}
			if !errors.Is(err, bundle.ErrUnknownBundleFormat) {
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
	_, err := bundle.Read(strings.NewReader(payload))
	if err == nil {
		t.Fatal("Read() error = nil, want an unknown-part-kind error")
	}
	if !errors.Is(err, bundle.ErrUnknownPartKind) {
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
	_, err := bundle.Read(strings.NewReader(payload))
	if err == nil {
		t.Fatal("Read() error = nil, want a missing-aiGoal-section error")
	}
	if !errors.Is(err, bundle.ErrMissingAIGoalSection) {
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
	decoded, err := bundle.Read(strings.NewReader(payload))
	if err != nil {
		t.Fatalf("Read() error = %v, want a reserved deterministic part to be accepted", err)
	}
	if len(decoded.Runs) != 1 || len(decoded.Runs[0].Parts) != 1 || decoded.Runs[0].Parts[0].Kind != bundle.PartKindDeterministic {
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
	decoded, err := bundle.Read(strings.NewReader(payload))
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
// names — never implies — its endpoint profile and platform (AGENTS.md's
// "fidelity is declared" principle applied to the run-bundle artifact), both
// as struct fields and as readable keys in the encoded JSON a player parses.
func TestBundleContainsProfileAndPlatformLabels(t *testing.T) {
	b := goldenBundle()

	if len(b.Runs) != 1 {
		t.Fatalf("len(b.Runs) = %d, want 1", len(b.Runs))
	}
	run := b.Runs[0]
	if run.Platform != "telegram" {
		t.Fatalf("run.Platform = %q, want %q", run.Platform, "telegram")
	}
	if run.EndpointProfile != bundle.EndpointProfilePlatformEmulated {
		t.Fatalf("run.EndpointProfile = %q, want %q", run.EndpointProfile, bundle.EndpointProfilePlatformEmulated)
	}

	var buf bytes.Buffer
	if err := bundle.Write(&buf, b); err != nil {
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
