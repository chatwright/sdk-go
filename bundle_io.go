package bundle

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ErrUnknownBundleFormat means Read decoded a document whose top-level
// "format" is not FormatV1 — older, newer, or simply unrecognised. Read
// returns it wrapped with the value actually found (see Read), rather than
// unmarshalling the rest of a shape this package's current Bundle type might
// not agree with. A newer format is exactly as rejected as an older one,
// since this package has no way to know a future shape is
// backward-compatible.
var ErrUnknownBundleFormat = errors.New("bundle: unknown bundle format")

// ErrUnknownPartKind means Read decoded a Part whose Kind is neither
// PartKindAIGoal nor PartKindDeterministic. Read returns it wrapped with the
// kind and part id actually found (see Read).
var ErrUnknownPartKind = errors.New("bundle: unknown part kind")

// ErrMissingAIGoalSection means Read decoded a Part with Kind ==
// PartKindAIGoal but no AIGoal section. Read returns it wrapped with the
// part id actually found (see Read), rather than handing back a Part whose
// AIGoal is silently nil.
var ErrMissingAIGoalSection = errors.New("bundle: ai-goal part missing its aiGoal section")

// Write writes b to w as indented, human-readable JSON, terminated by a
// trailing newline — the same style actor.Cassette.Save uses for its own
// checked-in JSON files, so a Bundle is reviewable in a PR diff and
// inspectable by hand, not just by a player. See the package doc comment for
// this format's file-naming convention ("*.chatwright.json").
//
// Output is deterministic: encoding/json always renders a struct's fields in
// their declared order (see Bundle's own doc comment for that order) and
// always sorts a map's keys, so two calls encoding equal Bundle values
// produce byte-identical output — see TestBundleRoundTripIsDeterministic.
// Write performs no reordering or canonicalisation of its own beyond what
// json.MarshalIndent already guarantees; a Bundle's slice fields carry
// whatever order the caller assembled them in (each field's own doc comment
// states what that order is expected to be).
func Write(w io.Writer, b Bundle) error {
	encoded, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("bundle: encode bundle: %w", err)
	}
	encoded = append(encoded, '\n')
	if _, err := w.Write(encoded); err != nil {
		return fmt.Errorf("bundle: write bundle: %w", err)
	}
	return nil
}

// Read reads a Bundle from r. It checks the top-level "format" before
// trusting the rest of the shape: a format other than FormatV1 returns an
// error wrapping ErrUnknownBundleFormat (naming the value actually found),
// rather than silently unmarshalling an old or newer schema's fields under
// today's meanings.
//
// Once format is confirmed, Read applies two further structural checks no
// json.Unmarshal alone can express — see ErrUnknownPartKind and
// ErrMissingAIGoalSection — over every Part of every Run. Unknown extra JSON
// fields elsewhere in the document are ignored (encoding/json's default: no
// DisallowUnknownFields), so a Bundle written by a future minor version that
// only adds fields still reads cleanly here. Read does not, and deliberately
// never will, validate Bookmark/Annotation references (Annotation.ReplyTo,
// Anchor) — see Annotation's own doc comment for why a dangling reference is
// a consumer's concern, not a Read error.
func Read(r io.Reader) (Bundle, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Bundle{}, fmt.Errorf("bundle: read bundle: %w", err)
	}

	var probe struct {
		Format string `json:"format"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return Bundle{}, fmt.Errorf("bundle: decode bundle: %w", err)
	}
	if probe.Format != FormatV1 {
		return Bundle{}, fmt.Errorf("%w: found %q, want %q", ErrUnknownBundleFormat, probe.Format, FormatV1)
	}

	var b Bundle
	if err := json.Unmarshal(data, &b); err != nil {
		return Bundle{}, fmt.Errorf("bundle: decode bundle: %w", err)
	}

	for _, run := range b.Runs {
		for _, part := range run.Parts {
			switch part.Kind {
			case PartKindAIGoal:
				if part.AIGoal == nil {
					return Bundle{}, fmt.Errorf("%w: run %q part %q", ErrMissingAIGoalSection, run.ID, part.ID)
				}
			case PartKindDeterministic:
				// Reserved — no section is modelled yet, so there is
				// nothing further to validate (see PartKindDeterministic).
			default:
				return Bundle{}, fmt.Errorf("%w: %q (run %q part %q)", ErrUnknownPartKind, part.Kind, run.ID, part.ID)
			}
		}
	}

	return b, nil
}
