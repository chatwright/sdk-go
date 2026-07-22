// Package schemagen generates the run-bundle format v1's JSON Schema
// (formats/run-bundle/v1/schema.json) from this module's own Go types, via
// reflection (github.com/invopop/jsonschema), so the Go types stay the
// format's single source of truth — nobody hand-maintains a second
// description of the wire shape that can drift from it.
//
// Plain reflection is not quite faithful to this module's actual encoding,
// for two documented reasons Generate corrects:
//
//   - Type-name collisions: more than one package in this module declares a
//     type with the same short name (observe.Actor, the message-side enum,
//     vs bundle.Actor, the roster entry; datastate.Evidence vs
//     campaign.Evidence). Reflected naively, the second definition silently
//     overwrites the first in the schema's $defs, and every existing $ref to
//     the clobbered type quietly points at the wrong shape instead.
//     qualifiedTypeName (via Reflector.Namer) disambiguates every $defs key
//     by package so this cannot happen.
//   - Nullable non-omitempty slices, maps and pointers: most of this
//     module's types (goal.Goal, platform.JournalEntry, observe.Observation,
//     datastate.Evidence, ...) carry no `json` tags at all, so encoding/json
//     falls back to its default behaviour — and a nil slice, map or pointer
//     field with no `omitempty` option marshals as JSON null, not as that
//     field's "empty" form ([], {} or an absent property). This is the
//     everyday, common case (e.g. any goal.Task with no DependsOn), not a
//     corner case, and it shows up throughout the golden bundle
//     ("Actions": null, "Changes": null, ...). invopop/jsonschema has no
//     notion of this at all — a plain reflected schema types every one of
//     these fields as a bare "array"/"object", which would reject a huge
//     share of bundles this module's own code legitimately produces.
//     applyNullablePatches walks the real Go type graph reachable from
//     bundle.Bundle (mirroring encoding/json's own tag rules) and widens
//     every such field to also accept null.
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

	"github.com/chatwright/chatwright/actor"
	"github.com/chatwright/chatwright/bundle"
	"github.com/chatwright/chatwright/observe"
	"github.com/chatwright/chatwright/platform"
)

// SchemaID is the run-bundle format v1 JSON Schema's own "$id" — the
// schema's stable, dereferenceable identity. Distinct from bundle.FormatV1
// (the Bundle document's own "format" field): SchemaID names the schema
// document itself, FormatV1 names the wire shape a Bundle instance claims to
// follow.
const SchemaID = "https://chatwright.dev/formats/run-bundle/v1/schema.json"

// posture is the committed schema's single top-level $comment: an
// author-facing (not consumer-validated) note stating, in one place, how
// this schema treats the two kinds of "openness" a run-bundle consumer needs
// to know about up front — see the package doc comment for the reasoning.
const posture = `Enum-constrained string fields reflected from this module's Go string-const ` +
	`enums (Direction, JournalEntryKind, Verdict, Actor, ChangeKind, ProposalKind, ` +
	`ActionOutcomeKind) are closed: schema validation rejects any value outside the listed ` +
	`set. This is stricter than bundle.Read itself, which applies no such check — a ` +
	`hand-edited bundle carrying an unrecognised value still reads. Bookmark/Annotation ` +
	`references (Annotation.replyTo, Anchor) are never validated by this schema or by ` +
	`bundle.Read — see bundle.Annotation's doc comment: a dangling reference is a ` +
	`consumer's concern, not a format violation. Every object also rejects properties this ` +
	`schema does not declare (additionalProperties: false), which is stricter than ` +
	`bundle.Read's own forward-compatible decoding (it ignores unknown fields) — this ` +
	`schema describes today's shape, not a compatibility promise for tomorrow's.`

// Generate builds the run-bundle format v1 JSON Schema (draft 2020-12) from
// bundle.Bundle's Go types. See the package doc comment for the two
// documented corrections applied on top of plain reflection.
func Generate() (*jsonschema.Schema, error) {
	r := &jsonschema.Reflector{
		Namer:          qualifiedTypeName,
		Mapper:         enumMapper,
		ExpandedStruct: true,
	}
	schema := r.Reflect(&bundle.Bundle{})
	schema.ID = jsonschema.ID(SchemaID)
	schema.Comments = posture

	if err := applyNullablePatches(schema); err != nil {
		return nil, err
	}
	return schema, nil
}

// Marshal renders schema as indented JSON terminated by a trailing newline —
// the same convention bundle.Write and actor.Cassette.Save use for their own
// checked-in/produced JSON, so the committed schema file is reviewable in a
// PR diff like any other artefact in this repository.
func Marshal(schema *jsonschema.Schema) ([]byte, error) {
	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("schemagen: encode schema: %w", err)
	}
	return append(data, '\n'), nil
}

// qualifiedTypeName disambiguates $defs keys by package: reflected naively,
// two same-named types in different packages (observe.Actor vs bundle.Actor,
// datastate.Evidence vs campaign.Evidence) collide in a single, unqualified
// $defs map — see the package doc comment.
func qualifiedTypeName(t reflect.Type) string {
	pkg := t.PkgPath()
	if pkg == "" {
		return t.Name()
	}
	parts := strings.Split(pkg, "/")
	short := parts[len(parts)-1]
	return capitalize(short) + t.Name()
}

// capitalize upper-cases s's first byte — package names in this module are
// plain ASCII lower-case identifiers, so a byte-wise operation is sufficient
// (no need for the unicode-aware machinery strings.Title's replacement,
// cases.Title, would pull in).
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// enumMapper returns a closed, enum-constrained string schema for each of
// this module's exported string-const enum types that actually appears
// somewhere in Bundle's wire shape (see AGENTS.md's "JSON artefacts carry
// human-readable string constants" convention; the closed-enum posture
// itself is documented once, at the schema's top level — see posture).
// actor.Mode is deliberately absent: it selects a CassetteProvider's
// record/replay behaviour and never appears on a Bundle. Returning nil for
// every other type defers to the reflector's own default handling.
//
// A per-field Description is deliberately not attached here: invopop/
// jsonschema's field handling (structKeywordsFromTags) unconditionally
// overwrites a property schema's Description from the field's (here, absent)
// `jsonschema_description` struct tag after this Mapper runs, so anything
// set here would be silently discarded — see reflect.go's handleField. Using
// that tag instead would mean adding jsonschema-only struct tags to
// platform/observe/actor's plain, tag-free domain types purely for schema
// cosmetics, which this generator deliberately avoids.
//
// observe.Verdict is the one enum whose Go zero value ("") is itself a real,
// meaningful wire value, not an unset placeholder to reject: actor.
// ValidationOutcome.Verdict is documented as "meaningless when Checked is
// false", and the loop leaves it at "" in exactly that case (see the golden
// bundle's own "Verdict": ""). Its enum lists "" alongside "fresh"/"stale"
// so the set stays closed — every value the wire actually carries — rather
// than silently rejecting real, correct output. Every other enum below is
// unconditionally assigned one of its named constants by every producer in
// this module (verified by reading each call site, not assumed), so none of
// them need the same treatment.
func enumMapper(t reflect.Type) *jsonschema.Schema {
	switch t {
	case reflect.TypeOf(platform.Direction("")):
		return enumSchema(platform.DirectionUser, platform.DirectionBot)
	case reflect.TypeOf(platform.JournalEntryKind("")):
		return enumSchema(platform.JournalEntryMessage, platform.JournalEntryAction, platform.JournalEntryUncaptured)
	case reflect.TypeOf(observe.Verdict("")):
		return enumSchema(observe.Verdict(""), observe.VerdictFresh, observe.VerdictStale)
	case reflect.TypeOf(observe.Actor("")):
		return enumSchema(observe.ActorUser, observe.ActorBot)
	case reflect.TypeOf(observe.ChangeKind("")):
		return enumSchema(observe.ChangeNewMessage, observe.ChangeMessageEdited, observe.ChangeActionsChanged)
	case reflect.TypeOf(actor.ProposalKind("")):
		return enumSchema(actor.ProposeSendText, actor.ProposeClick, actor.ProposeTaskDone, actor.ProposeGiveUp)
	case reflect.TypeOf(actor.ActionOutcomeKind("")):
		return enumSchema(actor.ActionSkippedInvalid, actor.ActionExecuted, actor.ActionExecutedNoEffect,
			actor.ActionResolutionFailed, actor.ActionTaskCompleted, actor.ActionTaskGivenUp)
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
// nil — so its schema also accepts null, matching bundle.Write's actual
// output instead of a bare reflected type. See the package doc comment's
// second bullet. Left uncorrected, TestGoldenBundleValidatesAgainstSchema
// would fail: schema validation would reject the golden bundle's own literal
// nulls (e.g. "Actions": null, "Changes": null).
//
// It walks the real Go type graph reachable from bundle.Bundle (the same
// graph the reflector itself walked to build schema), deriving each field's
// JSON name/omitempty exactly as encoding/json (and invopop/jsonschema,
// which reads the same `json` tag) would, and wraps the already-reflected
// property schema as {oneOf: [<original>, {type: null}]} — the identical
// idiom invopop/jsonschema uses internally for its own `jsonschema:"nullable"`
// tag, so a nullable field here renders no differently than the library's
// own native mechanism would.
func applyNullablePatches(schema *jsonschema.Schema) error {
	rootName := qualifiedTypeName(reflect.TypeOf(bundle.Bundle{}))

	var walkErr error
	walkStructs(reflect.TypeOf(bundle.Bundle{}), func(t reflect.Type) {
		defName := qualifiedTypeName(t)
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
	case reflect.Slice, reflect.Map, reflect.Ptr, reflect.Interface:
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
		case reflect.Ptr:
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
