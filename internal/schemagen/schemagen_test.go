package schemagen

import (
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/invopop/jsonschema"
	orderedmap "github.com/pb33f/ordered-map/v2"

	sdk "chatwright.dev/sdk"
)

type customStruct struct{}

type cycleA struct {
	B *cycleB
}

type cycleB struct {
	A *cycleA
}

type walkTarget struct {
	Ptr   *customStruct
	Slice []customStruct
	Arr   [1]customStruct
	Map   map[string]customStruct
	Time  time.Time
}

func TestGenerateAndMarshal(t *testing.T) {
	sch, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if sch == nil {
		t.Fatal("expected non-nil schema")
	}

	data, err := Marshal(sch)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty marshaled schema")
	}

	// Generate patchNullable error
	origPatch := patchNullable
	defer func() { patchNullable = origPatch }()
	patchNullable = func(*jsonschema.Schema) error { return errors.New("patch error") }
	if _, err := Generate(); err == nil {
		t.Fatal("expected Generate error when patchNullable fails")
	}

	// Marshal error with NaN
	badSchema := &jsonschema.Schema{
		Extras: map[string]any{
			"bad": math.NaN(),
		},
	}
	if _, err := Marshal(badSchema); err == nil {
		t.Fatal("expected Marshal error with NaN, got nil")
	}
}

func TestDefsName(t *testing.T) {
	// Keys in defsKeys
	for typ, expected := range defsKeys {
		if got := defsName(typ); got != expected {
			t.Errorf("defsName(%v) = %q, want %q", typ, got, expected)
		}
	}

	// time.Time leaf
	if got := defsName(reflect.TypeOf(time.Time{})); got != "" {
		t.Errorf("defsName(time.Time) = %q, want empty string", got)
	}

	// Anonymous struct
	anon := struct{ X int }{}
	if got := defsName(reflect.TypeOf(anon)); got != "" {
		t.Errorf("defsName(anon) = %q, want empty string", got)
	}

	// Predeclared type
	if got := defsName(reflect.TypeOf(123)); got != "" {
		t.Errorf("defsName(int) = %q, want empty string", got)
	}

	// Unregistered named struct panics
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected defsName to panic on unregistered named struct")
		}
	}()
	defsName(reflect.TypeOf(customStruct{}))
}

func TestEnumMapper(t *testing.T) {
	enumTypes := []reflect.Type{
		reflect.TypeOf(sdk.Direction("")),
		reflect.TypeOf(sdk.JournalEntryKind("")),
		reflect.TypeOf(sdk.Freshness("")),
		reflect.TypeOf(sdk.MessageActor("")),
		reflect.TypeOf(sdk.ChangeKind("")),
		reflect.TypeOf(sdk.ProposalKind("")),
		reflect.TypeOf(sdk.ActionOutcomeKind("")),
		reflect.TypeOf(sdk.FindingKind("")),
	}

	for _, typ := range enumTypes {
		sch := enumMapper(typ)
		if sch == nil {
			t.Errorf("enumMapper(%v) returned nil", typ)
		} else if sch.Type != "string" || len(sch.Enum) == 0 {
			t.Errorf("enumMapper(%v) invalid schema: %+v", typ, sch)
		}
	}

	// Unmapped type
	if sch := enumMapper(reflect.TypeOf(123)); sch != nil {
		t.Errorf("enumMapper(int) = %v, want nil", sch)
	}
}

func TestIsNilable(t *testing.T) {
	var iface any
	cases := []struct {
		typ  reflect.Type
		want bool
	}{
		{reflect.TypeOf([]string{}), true},
		{reflect.TypeOf(map[string]string{}), true},
		{reflect.TypeOf((*int)(nil)), true},
		{reflect.TypeOf(&iface).Elem(), true},
		{reflect.TypeOf(0), false},
		{reflect.TypeOf(""), false},
		{reflect.TypeOf(struct{}{}), false},
	}

	for _, c := range cases {
		if got := isNilable(c.typ); got != c.want {
			t.Errorf("isNilable(%v) = %v, want %v", c.typ, got, c.want)
		}
	}
}

func TestJsonFieldName(t *testing.T) {
	type testFields struct {
		NoTag    string
		Ignored  string `json:"-"`
		OmitOnly string `json:",omitempty"`
		Custom   string `json:"custom_name"`
		Both     string `json:"both_name,omitempty"`
	}

	typ := reflect.TypeOf(testFields{})

	// NoTag
	name, omit, skip := jsonFieldName(typ.Field(0))
	if name != "NoTag" || omit || skip {
		t.Errorf("NoTag: got name=%q, omit=%v, skip=%v", name, omit, skip)
	}

	// Ignored
	name, omit, skip = jsonFieldName(typ.Field(1))
	if name != "" || omit || !skip {
		t.Errorf("Ignored: got name=%q, omit=%v, skip=%v", name, omit, skip)
	}

	// OmitOnly
	name, omit, skip = jsonFieldName(typ.Field(2))
	if name != "OmitOnly" || !omit || skip {
		t.Errorf("OmitOnly: got name=%q, omit=%v, skip=%v", name, omit, skip)
	}

	// Custom
	name, omit, skip = jsonFieldName(typ.Field(3))
	if name != "custom_name" || omit || skip {
		t.Errorf("Custom: got name=%q, omit=%v, skip=%v", name, omit, skip)
	}

	// Both
	name, omit, skip = jsonFieldName(typ.Field(4))
	if name != "both_name" || !omit || skip {
		t.Errorf("Both: got name=%q, omit=%v, skip=%v", name, omit, skip)
	}
}

func TestApplyNullablePatches_Errors(t *testing.T) {
	// Missing definition error
	fakeSchemaMissingDef := &jsonschema.Schema{
		Definitions: make(map[string]*jsonschema.Schema),
		Properties:  orderedmap.New[string, *jsonschema.Schema](),
	}
	if err := applyNullablePatches(fakeSchemaMissingDef); err == nil {
		t.Error("expected error when definition missing from schema")
	}

	// Missing property error
	fakeSchemaMissingProp := &jsonschema.Schema{
		Definitions: make(map[string]*jsonschema.Schema),
		Properties:  orderedmap.New[string, *jsonschema.Schema](),
	}
	// Add all required definitions, but with empty properties
	walkStructs(reflect.TypeOf(sdk.Bundle{}), func(t reflect.Type) {
		name := defsName(t)
		fakeSchemaMissingProp.Definitions[name] = &jsonschema.Schema{
			Properties: orderedmap.New[string, *jsonschema.Schema](),
		}
	})
	if err := applyNullablePatches(fakeSchemaMissingProp); err == nil {
		t.Error("expected error when property missing from definition")
	}
}

func TestWalkStructs(t *testing.T) {
	var count int
	walkStructs(reflect.TypeOf(walkTarget{}), func(_ reflect.Type) {
		count++
	})
	if count < 2 {
		t.Errorf("expected at least 2 structs visited, got %d", count)
	}

	// Cycle terminates
	var cycleCount int
	walkStructs(reflect.TypeOf(cycleA{}), func(_ reflect.Type) {
		cycleCount++
	})
	if cycleCount != 2 {
		t.Errorf("expected 2 structs in cycle, got %d", cycleCount)
	}
}

type structWithUnexported struct {
	Exported   []string
	unexported int
}

func TestApplyNullablePatches_Unexported(t *testing.T) {
	_ = structWithUnexported{}.unexported

	typ := reflect.TypeOf(structWithUnexported{})
	defsKeys[typ] = "structWithUnexported"
	defer delete(defsKeys, typ)

	schema := &jsonschema.Schema{
		Properties: orderedmap.New[string, *jsonschema.Schema](),
	}
	schema.Properties.Set("Exported", &jsonschema.Schema{Type: "array"})

	if err := applyNullablePatchesForType(schema, typ); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
