// Package schemagen generates the run-bundle format v1's JSON Schema
// (formats/run-bundle/v1/schema.json) from the sdk package's own Go types,
// via reflection (github.com/invopop/jsonschema), so the Go types stay the
// format's single source of truth — nobody hand-maintains a second
// description of the wire shape that can drift from it.
//
// Plain reflection is not quite faithful to this module's actual encoding,
// for two documented reasons Generate corrects:
//
//   - $defs key stability: the published schema's $defs keys were minted
//     when the wire types lived across several runtime packages
//     (platform.JournalEntry -> "PlatformJournalEntry", campaign.Evidence ->
//     "CampaignEvidence", ...). Now that every wire type lives in the single
//     sdk package, reflecting names naively would mint different keys and
//     silently change the published schema. defsName (via Reflector.Namer)
//     pins every $defs key to its published value through an explicit table,
//     and panics on any named struct/map type the table does not know — so
//     an accidental new type can never silently join the wire.
//   - Nullable non-omitempty slices, maps and pointers: every exported field
//     reaching bundle JSON (Goal, JournalEntry, Observation,
//     DataStateEvidence, ...) carries an explicit lower-camel-case `json`
//     tag — the whole run-bundle wire is uniformly camelCase — but most of
//     those tags carry no `omitempty` option, so encoding/json's default
//     behaviour still applies to presence: a nil slice, map or pointer field
//     with no `omitempty` marshals as JSON null, not as that field's "empty"
//     form ([], {} or an absent property). This is the everyday, common case
//     (e.g. any Task with no DependsOn), not a corner case, and it shows up
//     throughout the golden bundle ("actions": null, "changes": null, ...).
//     invopop/jsonschema has no notion of this at all — a plain reflected
//     schema types every one of these fields as a bare "array"/"object",
//     which would reject a huge share of bundles the runtime's own code
//     legitimately produces. applyNullablePatches walks the real Go type
//     graph reachable from sdk.Bundle (mirroring encoding/json's own tag
//     rules, reading each field's tagged name rather than falling back to
//     its exported Go name) and widens every such field to also accept null.
//
// See Generate.
package schemagen

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/invopop/jsonschema"
	orderedmap "github.com/pb33f/ordered-map/v2"

	sdk "chatwright.dev/sdk"
)

// SchemaID is the run-bundle format v1 JSON Schema's own "$id" — the
// schema's stable, dereferenceable identity. Distinct from sdk.FormatV1
// (the Bundle document's own "format" field): SchemaID names the schema
// document itself, FormatV1 names the wire shape a Bundle instance claims to
// follow.
const SchemaID = "https://chatwright.dev/formats/run-bundle/v1/schema.json"

// posture is the committed schema's single top-level $comment: an
// author-facing (not consumer-validated) note stating, in one place, how
// this schema treats the two kinds of "openness" a run-bundle consumer needs
// to know about up front — see the package doc comment for the reasoning.
// The enum list below names the wire's historical enum names ("Actor" is
// sdk's MessageActor); the text is part of the published schema's bytes and
// never tracks a Go-side rename.
const posture = `Enum-constrained string fields reflected from this module's Go string-const ` +
	`enums (Direction, JournalEntryKind, Freshness, Actor, ChangeKind, ProposalKind, ` +
	`ActionOutcomeKind, FindingKind) are closed: schema validation rejects any value outside the listed ` +
	`set. This is stricter than bundle.Read itself, which applies no such check — a ` +
	`hand-edited bundle carrying an unrecognised value still reads. Bookmark/Annotation ` +
	`references (Annotation.replyTo, Anchor) are never validated by this schema or by ` +
	`bundle.Read — see bundle.Annotation's doc comment: a dangling reference is a ` +
	`consumer's concern, not a format violation. Every object also rejects properties this ` +
	`schema does not declare (additionalProperties: false), which is stricter than ` +
	`bundle.Read's own forward-compatible decoding (it ignores unknown fields) — this ` +
	`schema describes today's shape, not a compatibility promise for tomorrow's.`

// Generate builds the run-bundle format v1 JSON Schema (draft 2020-12) from
// sdk.Bundle's Go types. See the package doc comment for the two documented
// corrections applied on top of plain reflection.
func Generate() (*jsonschema.Schema, error) {
	r := &jsonschema.Reflector{
		Namer:          defsName,
		Mapper:         enumMapper,
		ExpandedStruct: true,
	}
	schema := r.Reflect(&sdk.Bundle{})
	schema.ID = jsonschema.ID(SchemaID)
	schema.Comments = posture

	if err := applyNullablePatches(schema); err != nil {
		return nil, err
	}
	return schema, nil
}

// Marshal renders schema as indented JSON terminated by a trailing newline —
// the same convention sdk.Write uses for the JSON it produces, so the
// committed schema file is reviewable in a PR diff like any other artefact
// in this repository.
func Marshal(schema *jsonschema.Schema) ([]byte, error) {
	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("schemagen: encode schema: %w", err)
	}
	return append(data, '\n'), nil
}

// defsKeys pins every type the reflector places in the schema's $defs to its
// published key. The keys were minted by the pre-split generator from each
// type's then-package (platform, goal, actor, observe, campaign, datastate,
// bundle) and are frozen wire artefacts now — a Go-side rename (MessageActor,
// FindingEvidence, DataStateEvidence) never moves them. sdk.Bundle itself is
// listed for completeness: ExpandedStruct promotes its definition to the
// schema root, so "Bundle" never appears as a $defs key in the output.
var defsKeys = map[reflect.Type]string{
	reflect.TypeOf(sdk.Bundle{}):              "Bundle",
	reflect.TypeOf(sdk.Metadata{}):            "BundleMetadata",
	reflect.TypeOf(sdk.Author{}):              "BundleAuthor",
	reflect.TypeOf(sdk.Run{}):                 "BundleRun",
	reflect.TypeOf(sdk.Actor{}):               "BundleActor",
	reflect.TypeOf(sdk.ActorProvider{}):       "BundleActorProvider",
	reflect.TypeOf(sdk.PlatformIdentity{}):    "BundlePlatformIdentity",
	reflect.TypeOf(sdk.ChatJournal{}):         "BundleChatJournal",
	reflect.TypeOf(sdk.Part{}):                "BundlePart",
	reflect.TypeOf(sdk.JournalBoundary{}):     "BundleJournalBoundary",
	reflect.TypeOf(sdk.ChatBoundary{}):        "BundleChatBoundary",
	reflect.TypeOf(sdk.AIGoalSection{}):       "BundleAIGoalSection",
	reflect.TypeOf(sdk.RetainedObservation{}): "BundleRetainedObservation",
	reflect.TypeOf(sdk.Bookmark{}):            "BundleBookmark",
	reflect.TypeOf(sdk.Annotation{}):          "BundleAnnotation",
	reflect.TypeOf(sdk.Anchor{}):              "BundleAnchor",
	reflect.TypeOf(sdk.JournalEntry{}):        "PlatformJournalEntry",
	reflect.TypeOf(sdk.Action{}):              "PlatformAction",
	reflect.TypeOf(sdk.Goal{}):                "GoalGoal",
	reflect.TypeOf(sdk.Task{}):                "GoalTask",
	reflect.TypeOf(sdk.Budgets{}):             "GoalBudgets",
	reflect.TypeOf(sdk.LoopEvent{}):           "ActorLoopEvent",
	reflect.TypeOf(sdk.Proposal{}):            "ActorProposal",
	reflect.TypeOf(sdk.Usage{}):               "ActorUsage",
	reflect.TypeOf(sdk.ValidationOutcome{}):   "ActorValidationOutcome",
	reflect.TypeOf(sdk.ActionOutcome{}):       "ActorActionOutcome",
	reflect.TypeOf(sdk.Observation{}):         "ObserveObservation",
	reflect.TypeOf(sdk.VisibleMessage{}):      "ObserveVisibleMessage",
	reflect.TypeOf(sdk.AvailableAction{}):     "ObserveAvailableAction",
	reflect.TypeOf(sdk.Change{}):              "ObserveChange",
	reflect.TypeOf(sdk.ChatRef{}):             "ObserveChatRef",
	reflect.TypeOf(sdk.Report{}):              "CampaignReport",
	reflect.TypeOf(sdk.TaskOutcome{}):         "CampaignTaskOutcome",
	reflect.TypeOf(sdk.Finding{}):             "CampaignFinding",
	reflect.TypeOf(sdk.FindingEvidence{}):     "CampaignEvidence",
	reflect.TypeOf(sdk.AggregateUsage{}):      "CampaignAggregateUsage",
	reflect.TypeOf(sdk.DataStateEvidence{}):   "DatastateEvidence",
	reflect.TypeOf(sdk.Row{}):                 "DatastateRow",
}

// defsName is the Reflector.Namer: it returns t's pinned $defs key from
// defsKeys, and panics on any named struct, map, slice or array type the
// table does not know — those are exactly the kinds invopop/jsonschema
// registers as $defs entries, so an unlisted one would mint a brand-new,
// unreviewed key in the published schema. Everything else the reflector asks
// about but never registers — predeclared kinds (string, int64, ...), named
// string enums (closed inline by enumMapper), unnamed composites, and
// time.Time/time.Duration (reflected as leaf "date-time"/integer schemas) —
// falls through to the empty string, which the reflector resolves to
// t.Name() without ever creating a definition.
func defsName(t reflect.Type) string {
	if name, ok := defsKeys[t]; ok {
		return name
	}
	if t == reflect.TypeOf(time.Time{}) {
		return "" // leaf: reflected as a "date-time" string, never a $defs entry
	}
	switch t.Kind() {
	case reflect.Struct, reflect.Map, reflect.Slice, reflect.Array:
		if t.Name() != "" {
			panic(fmt.Sprintf("schemagen: type %s has no pinned $defs key — a new wire type must be added to defsKeys deliberately, never named by accident", t))
		}
	}
	return ""
}

// enumMapper returns a closed, enum-constrained string schema for each of
// the sdk package's exported string-const enum types that actually appears
// somewhere in Bundle's wire shape (see the Chatwright standard's "JSON
// artefacts carry human-readable string constants" convention; the
// closed-enum posture itself is documented once, at the schema's top level —
// see posture). Returning nil for every other type defers to the reflector's
// own default handling.
//
// A per-field Description is deliberately not attached here: invopop/
// jsonschema's field handling (structKeywordsFromTags) unconditionally
// overwrites a property schema's Description from the field's (here, absent)
// `jsonschema_description` struct tag after this Mapper runs, so anything
// set here would be silently discarded — see reflect.go's handleField. Using
// that tag instead would mean adding jsonschema-only struct tags (distinct
// from the `json` tags the wire types already carry for wire-casing) to
// those types purely for schema cosmetics, which this generator deliberately
// avoids.
//
// Three enums carry a Go zero value ("") that is itself a real, meaningful
// wire value, not an unset placeholder to reject — each lists "" alongside
// its named constants so the set stays closed (every value the wire
// actually carries) rather than silently rejecting real, correct output:
//
//   - sdk.Freshness: ValidationOutcome.Freshness is documented as
//     "meaningless when Checked is false", and the runtime's loop leaves it
//     at "" in exactly that case (see the golden bundle's own
//     "freshness": "").
//   - sdk.ProposalKind and sdk.ActionOutcomeKind: LoopEvent.Proposal and
//     LoopEvent.Action are plain (non-pointer) structs, always present on
//     the wire, so they cannot simply be omitted when
//     LoopEvent.ProposeError is set — a Propose call that failed before it
//     ever produced a proposal (see LoopEvent.ProposeError's doc comment).
//     The runtime's loop leaves both their Kind fields at "" in exactly
//     that case (see the golden bundle's own second LoopEvent).
//
// Every other enum below is unconditionally assigned one of its named
// constants by every producer in the chatwright runtime (verified by
// reading each call site before the split, not assumed), so none of them
// need the same treatment. FindingKind is the newest addition to this list
// (previously left unconstrained by an oversight predating this comment —
// closed here for the first time, alongside its two additive new values,
// actor-overshoot and constraint-violation): a campaign.Finding is always
// constructed with an explicit Kind, so it needs no "" allowance either.
func enumMapper(t reflect.Type) *jsonschema.Schema {
	switch t {
	case reflect.TypeOf(sdk.Direction("")):
		return enumSchema(sdk.DirectionUser, sdk.DirectionBot)
	case reflect.TypeOf(sdk.JournalEntryKind("")):
		return enumSchema(sdk.JournalEntryMessage, sdk.JournalEntryAction, sdk.JournalEntryUncaptured)
	case reflect.TypeOf(sdk.Freshness("")):
		return enumSchema(sdk.Freshness(""), sdk.FreshnessFresh, sdk.FreshnessStale)
	case reflect.TypeOf(sdk.MessageActor("")):
		return enumSchema(sdk.MessageActorUser, sdk.MessageActorBot)
	case reflect.TypeOf(sdk.ChangeKind("")):
		return enumSchema(sdk.ChangeNewMessage, sdk.ChangeMessageEdited, sdk.ChangeActionsChanged)
	case reflect.TypeOf(sdk.ProposalKind("")):
		return enumSchema(sdk.ProposalKind(""), sdk.ProposeSendText, sdk.ProposeClick, sdk.ProposeTaskDone, sdk.ProposeGiveUp)
	case reflect.TypeOf(sdk.ActionOutcomeKind("")):
		return enumSchema(sdk.ActionOutcomeKind(""), sdk.ActionSkippedInvalid, sdk.ActionExecuted, sdk.ActionExecutedNoEffect,
			sdk.ActionResolutionFailed, sdk.ActionTaskCompleted, sdk.ActionTaskGivenUp,
			sdk.ActionBlockedConstraintViolation, sdk.ActionOvershootProbe)
	case reflect.TypeOf(sdk.FindingKind("")):
		return enumSchema(sdk.FindingVerifiedDefect, sdk.FindingAINavigationFailure, sdk.FindingCoverageGap,
			sdk.FindingActorOvershoot, sdk.FindingConstraintViolation)
	default:
		return nil
	}
}

// enumSchema builds a closed enum schema for a Go string-const enum type —
// see enumMapper.
func enumSchema[T ~string](values ...T) *jsonschema.Schema {
	enum := make([]any, len(values))
	for i, v := range values {
		enum[i] = string(v)
	}
	return &jsonschema.Schema{Type: "string", Enum: enum}
}

// applyNullablePatches widens every field this module's real encoding can
// emit as JSON null — a non-`omitempty` slice, map or pointer field left
// nil — so its schema also accepts null, matching sdk.Write's actual output
// instead of a bare reflected type. See the package doc comment's second
// bullet. Left uncorrected, TestGoldenBundleValidatesAgainstSchema would
// fail: schema validation would reject the golden bundle's own literal
// nulls (e.g. "actions": null, "changes": null).
//
// It walks the real Go type graph reachable from sdk.Bundle (the same graph
// the reflector itself walked to build schema), deriving each field's JSON
// name/omitempty exactly as encoding/json (and invopop/jsonschema, which
// reads the same `json` tag) would, and wraps the already-reflected property
// schema as {oneOf: [<original>, {type: null}]} — the identical idiom
// invopop/jsonschema uses internally for its own `jsonschema:"nullable"`
// tag, so a nullable field here renders no differently than the library's
// own native mechanism would.
func applyNullablePatches(schema *jsonschema.Schema) error {
	rootName := defsName(reflect.TypeOf(sdk.Bundle{}))

	var walkErr error
	walkStructs(reflect.TypeOf(sdk.Bundle{}), func(t reflect.Type) {
		defName := defsName(t)
		var props *orderedmap.OrderedMap[string, *jsonschema.Schema]
		if defName == rootName {
			props = schema.Properties
		} else {
			def, ok := schema.Definitions[defName]
			if !ok {
				walkErr = fmt.Errorf("schemagen: definition %q not found for a struct type this generator's own walk reached", defName)
				return
			}
			props = def.Properties
		}

		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" && !f.Anonymous {
				continue // unexported: encoding/json never marshals it
			}
			name, omitempty, skip := jsonFieldName(f)
			if skip || omitempty || !isNilable(f.Type) {
				continue
			}
			current, ok := props.Get(name)
			if !ok {
				walkErr = fmt.Errorf("schemagen: property %q not found on definition %q for field %s.%s",
					name, defName, t.Name(), f.Name)
				return
			}
			props.Set(name, &jsonschema.Schema{OneOf: []*jsonschema.Schema{current, {Type: "null"}}})
		}
	})
	return walkErr
}

// isNilable reports whether a Go value of type t can be the nil zero value —
// the condition under which a field with no `omitempty` json tag option
// marshals as JSON null instead of its type's normal form.
func isNilable(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Slice, reflect.Map, reflect.Pointer, reflect.Interface:
		return true
	default:
		return false
	}
}

// jsonFieldName derives f's JSON property name, whether it carries
// `omitempty`, and whether it is skipped entirely (`json:"-"`) — the same
// rules encoding/json itself applies (and that invopop/jsonschema's own
// field-name derivation reads from the same `json` tag), reimplemented here
// because reflect.StructTag exposes no ready-made parse for them.
func jsonFieldName(f reflect.StructField) (name string, omitempty, skip bool) {
	tag, ok := f.Tag.Lookup("json")
	if !ok || tag == "" {
		return f.Name, false, false
	}
	parts := strings.Split(tag, ",")
	if parts[0] == "-" && len(parts) == 1 {
		return "", false, true
	}
	name = parts[0]
	if name == "" {
		name = f.Name
	}
	for _, opt := range parts[1:] {
		if opt == "omitempty" {
			omitempty = true
		}
	}
	return name, omitempty, false
}

// walkStructs calls visit once for every distinct struct type reachable from
// root by following struct fields, slice/array elements, map values and
// pointer targets — exactly the shapes invopop/jsonschema's own reflection
// descends through for this module's types (none of which use an
// interface-typed field directly; map[string]any's `any` values are dynamic
// content this generator does not (and need not) type further, matching how
// the reflector itself leaves them as an open "object" schema). time.Time is
// excluded: it is a leaf the reflector maps to a "date-time" string, not a
// struct this generator should walk into or emit a definition for.
func walkStructs(root reflect.Type, visit func(reflect.Type)) {
	visited := make(map[reflect.Type]bool)
	var walk func(t reflect.Type)
	walk = func(t reflect.Type) {
		switch t.Kind() {
		case reflect.Pointer:
			walk(t.Elem())
		case reflect.Slice, reflect.Array:
			walk(t.Elem())
		case reflect.Map:
			walk(t.Elem())
		case reflect.Struct:
			if t == reflect.TypeOf(time.Time{}) || visited[t] {
				return
			}
			visited[t] = true
			visit(t)
			for i := 0; i < t.NumField(); i++ {
				walk(t.Field(i).Type)
			}
		}
	}
	walk(root)
}
