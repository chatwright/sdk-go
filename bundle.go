// Package bundle defines Chatwright's run-bundle format v1: the persisted,
// self-contained artifact a Web UI player (Chatwright Studio) replays. A
// Bundle needs nothing else — no live emulator, no database, no network
// access — to show what happened during a run and why it concluded what it
// did.
//
// The artifact spans more than one package's concerns (a Goal, a chat
// journal, retained observations, loop events, a campaign.Report, optional
// datastate.Evidence), which is why it lives in its own top-level package
// rather than inside campaign, which now keeps only Report and its
// assembly.
//
// Shape: a Bundle carries one or more Runs (today's writers always emit
// exactly one). A Run carries an actors roster, a run-level, continuous,
// per-chat journal, and an ordered list of Parts — each Part names a kind
// (ai-goal today; deterministic reserved) and a JournalBoundary slicing the
// run-level journal into the entries that part covers, so parts never
// duplicate journal content. See Bundle, Run, Actor and Part for the full
// shape, and spec/ideas/hybrid-runs.md for why runs are structured this way:
// a plain campaign is exactly a single-part, single-run Bundle, and the same
// shape accommodates a run mixing deterministic and AI-goal passages without
// any schema change.
//
// Filename convention: a Bundle file is named "<anything>.chatwright.json"
// (e.g. "greetbot-language.chatwright.json") so a player, a file browser or
// a directory listing can recognize one at a glance.
//
// The full wire shape is also published as a JSON Schema, generated from
// these Go types (see internal/schemagen) and committed at
// formats/run-bundle/v1/schema.json; bundle/schema_test.go gates both that
// the schema stays in sync with these types and that a Bundle this package
// produces validates against it.
//
//go:generate go run ../internal/schemagen/gen
package bundle

import (
	"runtime/debug"
	"sort"
	"time"

	"github.com/chatwright/chatwright/actor"
	"github.com/chatwright/chatwright/campaign"
	"github.com/chatwright/chatwright/datastate"
	"github.com/chatwright/chatwright/goal"
	"github.com/chatwright/chatwright/observe"
	"github.com/chatwright/chatwright/platform"
)

// FormatV1 is the run-bundle format identifier this package reads and
// writes. It replaces an earlier draft's integer schema version with a
// namespaced URL, matching the style of other Chatwright format ids and
// leaving room for a v2 to be a different, equally explicit string. Read
// rejects any other value — see Read.
const FormatV1 = "https://chatwright.dev/formats/run-bundle/v1"

// EndpointProfilePlatformEmulated is a Run.EndpointProfile label for a run
// driven against a platform.Emulator — the only endpoint profile this module
// currently produces (see decision 0008 and docs/glossary.md's "endpoint
// profile" entry: "platform-emulated (strongest), headless engine, or future
// profiles"). Run.EndpointProfile is a plain string, not restricted to this
// constant, so a future profile never requires a schema change — only a new
// label.
const EndpointProfilePlatformEmulated = "platform-emulated"

// chatwrightModulePath is this module's own import path — see ModuleVersion.
const chatwrightModulePath = "github.com/chatwright/chatwright"

// Bundle is the top-level run-bundle document: a declared Format, caller
// Metadata, and one or more Runs. Field order below is Bundle's stable JSON
// shape (Go's encoding/json preserves struct field declaration order for
// objects, and sorts map keys deterministically for anything encoded as a
// JSON object) — see Write for the ordering guarantees this gives a
// round-tripped Bundle, and each slice field's own doc comment for the order
// its elements are stored in.
type Bundle struct {
	// Format is always FormatV1 for a Bundle this package produced. See Read.
	Format string `json:"format"`

	// Metadata carries this Bundle's caller-supplied provenance — see
	// Metadata.
	Metadata Metadata `json:"metadata"`

	// Runs is every run this Bundle carries, in the order the caller
	// assembled them. Today's writers (SingleAIGoalRun) always produce
	// exactly one; the shape accommodates a future multi-run file (e.g.
	// several campaigns bundled for one delivery) without a schema change.
	Runs []Run `json:"runs"`
}

// Metadata declares a Bundle's provenance — independent of any one Run's own
// fidelity labels (Run.Platform, Run.EndpointProfile), which is why those
// moved out of Metadata and onto Run: a Bundle can in principle carry runs
// against different platforms or endpoint profiles, so a single
// Metadata-level label would have been misleading.
type Metadata struct {
	// CreatedAt is when this Bundle was assembled, supplied by the caller
	// (never time.Now internally — see chatwright's broader injected-clock
	// convention, e.g. goal.NewCampaignState, actor.Config.Now) so
	// assembling a Bundle is itself deterministic and testable.
	CreatedAt time.Time `json:"createdAt"`

	// ChatwrightVersion is the chatwright module's own resolved version —
	// see ModuleVersion — left empty when it cannot be determined (e.g. a
	// `go test` run inside this repository itself, which always reports
	// "(devel)"; see ModuleVersion's doc comment). "If available" is
	// load-bearing: a Bundle is still valid and complete without it.
	ChatwrightVersion string `json:"chatwrightVersion,omitempty"`

	// Author optionally attributes this Bundle to whoever assembled it.
	// Never populated automatically from git config, the OS user or any
	// other ambient environment — a caller must supply it explicitly, or
	// leave it nil. Bundles get emailed and committed to public
	// repositories, so silently harvesting an identity into one is not this
	// package's call to make.
	Author *Author `json:"author,omitempty"`
}

// Author optionally attributes provenance to a Bundle (Metadata.Author) or to
// an Annotation (Annotation.Author). Both fields are optional free-text
// strings — this package does not validate an email's shape or resolve a
// name against any identity system. See Metadata.Author's doc comment for
// why it is always caller-supplied, never auto-populated.
type Author struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
}

// Run is one run's complete, self-contained record: who was in the
// conversation (Actors), the continuous per-chat journal the whole run
// produced (Chats), and the ordered passages the run was composed of
// (Parts). Today's writers (SingleAIGoalRun) always emit a Run with exactly
// one ai-goal Part spanning the whole journal — see spec/ideas/hybrid-runs.md
// for the hybrid (deterministic + ai-goal) runs this shape exists to
// accommodate without a future schema change.
type Run struct {
	// ID is caller-supplied and only needs to be unique within this Bundle.
	ID string `json:"id"`

	// Platform is the platform name this run drove (e.g. "telegram") — see
	// platform.Platform.Name.
	Platform string `json:"platform"`

	// EndpointProfile is this run's declared endpoint profile (decision
	// 0008; docs/glossary.md's "endpoint profile" entry) — e.g.
	// EndpointProfilePlatformEmulated. Evidence is never interchangeable
	// across profiles, so a player must always have this label, never infer
	// it.
	EndpointProfile string `json:"endpointProfile"`

	// Actors is the roster of everyone who took part in this run — every
	// AI agent, human, scripted or replay actor that acted, plus the
	// bot-under-test itself — so a player can attribute every journal entry
	// to whoever produced it (see platform.JournalEntry.FromID and Actor's
	// own doc comment).
	Actors []Actor `json:"actors"`

	// Chats is the run's continuous, per-chat journal: one entry per
	// distinct chat ID, in the order the caller assembled them, each
	// carrying that chat's entire platform.JournalEntry history for the
	// whole run — never re-split or duplicated per Part. A Part slices this
	// journal by reference (see Part.JournalBoundary) rather than embedding
	// its own copy.
	Chats []ChatJournal `json:"chats"`

	// Parts is this run's ordered sequence of passages — see Part. Today's
	// writers always produce exactly one ai-goal Part; the ordered-list
	// shape is what a future hybrid run (deterministic passages interleaved
	// with ai-goal exploration) composes without any schema change.
	Parts []Part `json:"parts"`

	// Bookmarks is an optional list of manual fast-forward markers a player
	// can offer alongside this run's own derived markers — part boundaries,
	// task completions, findings and the like need no schema entry here,
	// since a player derives them directly from Parts/AIGoalSection; a
	// Bookmark is only for a marker no derivation already produces. Today's
	// writers (SingleAIGoalRun) emit none unless the caller supplies them.
	Bookmarks []Bookmark `json:"bookmarks,omitempty"`

	// Annotations is an optional list of comments attached to moments in
	// this run's conversation — see Annotation. Today's writers
	// (SingleAIGoalRun) emit none unless the caller supplies them.
	Annotations []Annotation `json:"annotations,omitempty"`
}

// Bookmark is a manual fast-forward marker for a player: a caller-chosen
// point in the run's journal worth jumping straight to. Bookmark exists only
// for markers a player cannot already derive on its own — part boundaries,
// task completions and campaign.Finding entries are all recoverable from
// Run.Parts/AIGoalSection directly, so nothing here should duplicate those;
// use a Bookmark for the rest (e.g. "the moment the bug reproduced").
type Bookmark struct {
	// ID is caller-supplied and only needs to be unique within its Run.
	ID string `json:"id"`
	// Title is the human-readable label a player shows for this marker.
	Title string `json:"title"`
	// Anchor locates the marker in the run's journal — see Anchor.
	Anchor Anchor `json:"anchor"`
}

// Annotation is a comment attached to one moment of this run's conversation
// — e.g. "See how instead of $4 bot returned 4$". ReplyTo threads Annotations
// into a conversation about a message: a root Annotation leaves ReplyTo
// empty, a reply names the Annotation.ID it responds to.
//
// Read-side tolerance: a ReplyTo naming an Annotation ID this Run does not
// actually carry, and an Anchor whose EntryIndex (or MessageID/Version) does
// not resolve against the referenced chat, are both NOT errors from Read —
// bundles are hand-editable files, and a consumer must be prepared to
// surface a dangling reference rather than assume Read already validated it.
// See TestBundleReadToleratesDanglingAnnotationReferences.
type Annotation struct {
	// ID is caller-supplied and only needs to be unique within its Run.
	ID string `json:"id"`
	// Anchor locates the annotated moment in the run's journal — see Anchor.
	Anchor Anchor `json:"anchor"`
	// Author optionally attributes this Annotation — see Author's own doc
	// comment (never auto-populated).
	Author *Author `json:"author,omitempty"`
	// CreatedAt is when this Annotation was authored, supplied by the
	// caller — see Metadata.CreatedAt's own doc comment on why this package
	// never stamps a time itself.
	CreatedAt time.Time `json:"createdAt"`
	// Text is the annotation's own comment body.
	Text string `json:"text"`
	// ReplyTo optionally names another Annotation.ID in this Run that this
	// one replies to, threading both into one conversation about a message.
	// Empty for a root Annotation.
	ReplyTo string `json:"replyTo,omitempty"`
}

// Anchor locates one moment in a run's journal, shared by Bookmark and
// Annotation. ChatID and EntryIndex are required and always resolvable
// against Run.Chats for a Bundle this package wrote; MessageID and Version
// are optional and, together, pin an exact revision of an edited message —
// versioned message identity — rather than whatever its latest version
// happens to be by the time a player renders it.
type Anchor struct {
	// ChatID names the Run.Chats entry this anchor points into.
	ChatID int64 `json:"chatId"`
	// EntryIndex is the index into that ChatJournal.Entries this anchor
	// points at.
	EntryIndex int `json:"entryIndex"`
	// MessageID optionally pins the logical message (platform.JournalEntry.
	// MessageID) this anchor is about, when it is more specific than "this
	// journal entry" — e.g. an Annotation about a message that was later
	// edited, anchored to the message rather than to one particular edit.
	MessageID int `json:"messageId,omitempty"`
	// Version optionally pins the exact edit (platform.JournalEntry.
	// Version) MessageID was at, so an Annotation about "this specific
	// wording" survives a later edit instead of silently retargeting to the
	// message's newest version.
	Version int `json:"version,omitempty"`
}

// ChatJournal is one chat's complete structured journal — the same
// platform.JournalEntry records platform.Emulator.Journal returns, carried
// verbatim (including platform-native identifiers) because a Bundle is the
// developer/trace-level artifact platform.JournalEntry's own doc comment
// describes, not the actor-facing observe surface. It is run-level and
// continuous: a Part never carries its own ChatJournal, only a
// JournalBoundary referencing a slice of this one.
type ChatJournal struct {
	ChatID  int64                   `json:"chatId"`
	Entries []platform.JournalEntry `json:"entries"`
}

// ActorType classifies one roster Actor's origin.
type ActorType string

// Actor types. See Actor and ActorType.
const (
	// ActorAIAgent: an AI model proposing actions via an actor.Provider.
	ActorAIAgent ActorType = "ai-agent"
	// ActorHuman: a person driving the conversation directly.
	ActorHuman ActorType = "human"
	// ActorScripted: a fixed, deterministic proposal sequence (e.g.
	// actor.ScriptedProvider) or a deterministic scenario fragment.
	ActorScripted ActorType = "scripted"
	// ActorReplay: a recorded run replayed from an actor.Cassette.
	ActorReplay ActorType = "replay"
	// ActorBot: the bot-under-test itself, the other side of the
	// conversation from every other actor type above.
	ActorBot ActorType = "bot"
)

// Actor is one participant in a Run's conversation — the roster entry that
// lets a player attribute every platform.JournalEntry (via its FromID) and
// every actor.LoopEvent (via a Part's aiGoal.actorId) to whoever actually
// produced it, rather than leaving that to be inferred from Direction alone.
type Actor struct {
	// ID is this Bundle's own stable identity for the actor — referenced by
	// Part's aiGoal.actorId and resolvable against a
	// platform.JournalEntry.FromID through PlatformIdentities.
	ID string `json:"id"`

	// Type classifies this actor's origin — see ActorType.
	Type ActorType `json:"type"`

	// Name is an optional human-readable display name.
	Name string `json:"name,omitempty"`

	// PlatformIdentities maps a platform name (e.g. "telegram", matching
	// Run.Platform) to this actor's platform-native identity on that
	// platform. A map, not a single field, so a future actor active across
	// more than one platform needs no schema change — only a new key.
	PlatformIdentities map[string]PlatformIdentity `json:"platformIdentities,omitempty"`

	// Provider names the model/provider that proposed this actor's actions.
	// Only meaningful for ActorAIAgent and ActorReplay actors (a scripted or
	// human actor proposes nothing a "provider" describes); nil otherwise.
	Provider *ActorProvider `json:"provider,omitempty"`
}

// PlatformIdentity is one actor's platform-native identity on one platform —
// sized for what Telegram needs today (a numeric user id, plus an optional
// username and first name); a future platform reuses the same shape or
// extends it, either way without changing how Actor.PlatformIdentities is
// keyed.
type PlatformIdentity struct {
	UserID    int64  `json:"userId"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"firstName,omitempty"`
}

// ActorProvider names the model/provider behind an ActorAIAgent or
// ActorReplay Actor.
type ActorProvider struct {
	// Name is the provider's short identifier (e.g. "anthropic").
	Name string `json:"name,omitempty"`

	// ModelIDs is the aggregated set of actor.Usage.Model ids that actually
	// proposed an action for this actor during the run — see
	// AggregateModelIDs, the canonical way to compute it.
	ModelIDs []string `json:"modelIds,omitempty"`
}

// PartKind discriminates what a Part's kind-scoped section (aiGoal today)
// holds.
type PartKind string

// Part kinds. See PartKind.
const (
	// PartKindAIGoal: the actor loop ran a goal/task contract for this
	// part — see AIGoalSection.
	PartKindAIGoal PartKind = "ai-goal"
	// PartKindDeterministic: a deterministic scenario fragment executed for
	// this part. Reserved for the hybrid-runs runtime (see
	// spec/ideas/hybrid-runs.md): no Go struct or "deterministic" JSON
	// section is defined yet, and no writer in this module produces one —
	// Read still accepts the kind (so a future writer's output round-trips
	// once a section is defined) but the section itself is not modelled.
	PartKindDeterministic PartKind = "deterministic"
)

// Part is one ordered passage of a Run: a kind, a slice of the run-level
// journal this passage covers, and a kind-scoped section carrying that
// passage's own detail (aiGoal today; a future "deterministic" section is
// reserved — see PartKindDeterministic). Today's writers (SingleAIGoalRun)
// always produce exactly one Part per Run, covering the whole journal; the
// ordered-list shape on Run.Parts is what lets a future hybrid run add more
// Parts without any schema change.
type Part struct {
	// ID is caller-supplied and only needs to be unique within its Run.
	ID string `json:"id"`

	// Title is an optional human-readable label (e.g. "Shopping-list
	// exploration") for a player to show as a chapter heading.
	Title string `json:"title,omitempty"`

	// Kind discriminates which kind-scoped section below is populated — see
	// PartKind.
	Kind PartKind `json:"kind"`

	// JournalBoundary slices the run-level journal (Run.Chats) into the
	// entries this Part covers — see JournalBoundary.
	JournalBoundary JournalBoundary `json:"journalBoundary"`

	// AIGoal is populated when Kind is PartKindAIGoal, nil otherwise — see
	// AIGoalSection. Read returns ErrMissingAIGoalSection for an
	// PartKindAIGoal part with this unset, rather than silently returning a
	// half-decoded Part.
	AIGoal *AIGoalSection `json:"aiGoal,omitempty"`
}

// JournalBoundary slices a run-level journal (Run.Chats) into the entries
// one Part covers, per chat.
type JournalBoundary struct {
	Chats []ChatBoundary `json:"chats"`
}

// ChatBoundary is a half-open range ([FirstEntry, FirstEntry+EntryCount)
// into one chat's ChatJournal.Entries — never a duplicated copy of the
// entries themselves.
type ChatBoundary struct {
	ChatID int64 `json:"chatId"`
	// FirstEntry is the index, into the matching ChatJournal.Entries, of
	// this Part's first entry for this chat.
	FirstEntry int `json:"firstEntry"`
	// EntryCount is how many consecutive entries, starting at FirstEntry,
	// belong to this Part.
	EntryCount int `json:"entryCount"`
}

// AIGoalSection is a PartKindAIGoal Part's kind-scoped detail: the Goal that
// part of the run pursued, which roster Actor ran the loop, and the
// evidence the loop produced — the same pieces the earlier, campaign-only
// Bundle draft carried at its top level, now scoped to one Part so a hybrid
// run can carry more than one.
type AIGoalSection struct {
	// Goal is the goal definition this part's actor loop ran — verbatim,
	// not converted to plain strings the way Report's fields are, since a
	// player needs the full Goal (task dependencies, constraints, budgets)
	// to render it, not just the outcome Report already summarises.
	Goal goal.Goal `json:"goal"`

	// ActorID references the Run.Actors entry that ran this part's loop.
	ActorID string `json:"actorId"`

	// Events is every actor.LoopEvent the loop recorded for this part, in
	// Loop.Events' own order (Index-ascending, across every task the loop
	// ran).
	Events []actor.LoopEvent `json:"events"`

	// Observations is every observe.Observation the loop retained (see
	// actor.Config.DisableObservationRetention and actor.Loop.Observations),
	// ordered ascending by Sequence — not the raw map[int64]observe.Observation
	// Loop.Observations returns, so this section's JSON stays chronologically
	// readable regardless of encoding/json's own (string-lexicographic, not
	// numeric) map-key ordering for an integer-keyed map.
	Observations []RetainedObservation `json:"observations"`

	// Report is this part's assembled campaign.Report (see campaign.Assemble).
	Report campaign.Report `json:"report"`

	// Evidence is the datastate.Evidence any data-state assertions produced
	// during this part, in the order they were run. Optional: a part with
	// no data-state assertions attached carries none.
	Evidence []datastate.Evidence `json:"evidence,omitempty"`
}

// RetainedObservation pairs one retained observe.Observation with its own
// Sequence, so AIGoalSection.Observations reads as an ordered list rather
// than a JSON object keyed by a stringified int64 (see
// AIGoalSection.Observations).
type RetainedObservation struct {
	Sequence    int64               `json:"sequence"`
	Observation observe.Observation `json:"observation"`
}

// SortObservations converts observations — as returned by
// actor.Loop.Observations — into an AIGoalSection.Observations-ready slice,
// ordered ascending by Sequence. It is the canonical way a caller turns a
// Loop's retained observations into that field: see
// AIGoalSection.Observations for why the slice form, not the map
// encoding/json would otherwise produce, is what this package stores.
func SortObservations(observations map[int64]observe.Observation) []RetainedObservation {
	out := make([]RetainedObservation, 0, len(observations))
	for seq, obs := range observations {
		out = append(out, RetainedObservation{Sequence: seq, Observation: obs})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Sequence < out[j].Sequence })
	return out
}

// AggregateModelIDs returns the sorted, deduplicated set of every non-empty
// actor.Usage.Model value across events. It is the canonical way an
// ActorProvider.ModelIDs is computed, so two callers assembling a roster
// entry from the same events always produce the same aggregated identity
// list, regardless of how many times — or in what order — any one model was
// actually used.
func AggregateModelIDs(events []actor.LoopEvent) []string {
	seen := make(map[string]struct{})
	for _, e := range events {
		if e.Usage.Model == "" {
			continue
		}
		seen[e.Usage.Model] = struct{}{}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// SingleAIGoalRunInput is everything SingleAIGoalRun needs to assemble a
// single-part, single-run Bundle.Run from a plain campaign's own pieces.
type SingleAIGoalRunInput struct {
	// RunID is caller-supplied — see Run.ID.
	RunID string
	// Platform is the platform name the run drove — see Run.Platform.
	Platform string
	// EndpointProfile is the run's declared endpoint profile — see
	// Run.EndpointProfile, EndpointProfilePlatformEmulated.
	EndpointProfile string
	// Actors is the run's roster — see Run.Actors.
	Actors []Actor
	// Chats is the run's continuous per-chat journal — see Run.Chats.
	Chats []ChatJournal

	// PartID and PartTitle name the single ai-goal Part this run gets — see
	// Part.ID, Part.Title.
	PartID    string
	PartTitle string

	// ActorID references the Actors entry that ran the loop — see
	// AIGoalSection.ActorID.
	ActorID string

	// Goal, Events, Observations, Report and Evidence become the part's
	// AIGoalSection verbatim — see AIGoalSection's own fields.
	Goal         goal.Goal
	Events       []actor.LoopEvent
	Observations []RetainedObservation
	Report       campaign.Report
	Evidence     []datastate.Evidence

	// Bookmarks and Annotations become the Run's own fields verbatim — see
	// Run.Bookmarks, Run.Annotations. Both are optional: a caller with
	// nothing to attach leaves them nil, and the resulting Run carries none
	// (SingleAIGoalRun never invents one).
	Bookmarks   []Bookmark
	Annotations []Annotation
}

// SingleAIGoalRun builds a Run containing exactly one ai-goal Part whose
// JournalBoundary spans each chat's entire journal — the "plain campaign is
// a single ai-goal part" path spec/ideas/hybrid-runs.md's MVP scope
// describes, and the one every writer in this module uses today. A future
// hybrid run assembles its Run directly (Run{Parts: []Part{...}}) instead,
// once a runtime produces more than one Part; this helper only ever emits
// one.
func SingleAIGoalRun(in SingleAIGoalRunInput) Run {
	boundary := JournalBoundary{Chats: make([]ChatBoundary, 0, len(in.Chats))}
	for _, chat := range in.Chats {
		boundary.Chats = append(boundary.Chats, ChatBoundary{
			ChatID:     chat.ChatID,
			FirstEntry: 0,
			EntryCount: len(chat.Entries),
		})
	}

	part := Part{
		ID:              in.PartID,
		Title:           in.PartTitle,
		Kind:            PartKindAIGoal,
		JournalBoundary: boundary,
		AIGoal: &AIGoalSection{
			Goal:         in.Goal,
			ActorID:      in.ActorID,
			Events:       in.Events,
			Observations: in.Observations,
			Report:       in.Report,
			Evidence:     in.Evidence,
		},
	}

	return Run{
		ID:              in.RunID,
		Platform:        in.Platform,
		EndpointProfile: in.EndpointProfile,
		Actors:          in.Actors,
		Chats:           in.Chats,
		Parts:           []Part{part},
		Bookmarks:       in.Bookmarks,
		Annotations:     in.Annotations,
	}
}

// ModuleVersion returns the chatwright module's own resolved version, read
// from the currently running binary's runtime/debug build info, or "" when
// it cannot be determined.
//
// A Bundle is normally produced by a bot's own test binary, which imports
// chatwright as a dependency rather than being chatwright itself. In that
// (expected) case it is chatwright's entry in the binary's
// debug.BuildInfo.Deps that carries the meaningful resolved version (a git
// tag or pseudo-version), so Deps is searched for chatwrightModulePath. The
// less common case — the running binary IS chatwright's own module, e.g. a
// test run inside this repository — is also covered, via
// debug.BuildInfo.Main, checked first.
//
// Either way, "(devel)" (Go's placeholder for "no resolvable version") and
// "" are both treated as "not available" — mirroring
// cmd/chatwright/main.go's own cliVersion fallback for the same reason: a
// plain `go build`/`go test` inside this repository, or inside a consumer
// module that has not pinned a chatwright version, never has one.
func ModuleVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	if bi.Main.Path == chatwrightModulePath {
		return resolvedVersion(bi.Main.Version)
	}
	for _, dep := range bi.Deps {
		if dep.Path == chatwrightModulePath {
			return resolvedVersion(dep.Version)
		}
	}
	return ""
}

// resolvedVersion returns v, or "" when v is Go's own placeholder for "no
// real version" ("(devel)") or already empty.
func resolvedVersion(v string) string {
	if v == "" || v == "(devel)" {
		return ""
	}
	return v
}
