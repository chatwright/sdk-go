package sdk

import (
	"errors"
	"math"
	"runtime/debug"
	"strings"
	"testing"
)

type failWriter struct{}

func (failWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("write failed")
}

type failReader struct{}

func (failReader) Read(_ []byte) (int, error) {
	return 0, errors.New("read failed")
}

func TestStringers(t *testing.T) {
	if got := ProposeClick.String(); got != "click" {
		t.Errorf("expected 'click', got %q", got)
	}
	if got := ActionExecuted.String(); got != "executed" {
		t.Errorf("expected 'executed', got %q", got)
	}
	if got := MessageActorUser.String(); got != "user" {
		t.Errorf("expected 'user', got %q", got)
	}
	if got := ChangeNewMessage.String(); got != "new-message" {
		t.Errorf("expected 'new-message', got %q", got)
	}
	if got := FreshnessFresh.String(); got != "fresh" {
		t.Errorf("expected 'fresh', got %q", got)
	}
}

func TestModuleVersion(t *testing.T) {
	origRead := readBuildInfo
	defer func() { readBuildInfo = origRead }()

	// Test 1: normal call in build
	_ = ModuleVersion()

	// Test 2: readBuildInfo returns !ok
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return nil, false
	}
	if v := ModuleVersion(); v != "" {
		t.Errorf("expected empty string when ReadBuildInfo fails, got %q", v)
	}

	// Test 3: main module is not sdkModulePath, dep is sdkModulePath
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			Main: debug.Module{Path: "other/main"},
			Deps: []*debug.Module{
				{Path: "other/dep", Version: "v0.1.0"},
				{Path: sdkModulePath, Version: "v1.2.3"},
			},
		}, true
	}
	if v := ModuleVersion(); v != "v1.2.3" {
		t.Errorf("expected v1.2.3 from deps, got %q", v)
	}

	// Test 4: neither matches
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			Main: debug.Module{Path: "other/main"},
			Deps: []*debug.Module{
				{Path: "other/dep", Version: "v0.1.0"},
			},
		}, true
	}
	if v := ModuleVersion(); v != "" {
		t.Errorf("expected empty string when sdk not in deps, got %q", v)
	}

	if resolvedVersion("") != "" {
		t.Errorf("expected empty string")
	}
	if resolvedVersion("(devel)") != "" {
		t.Errorf("expected empty string for (devel)")
	}
	if resolvedVersion("v1.0.0") != "v1.0.0" {
		t.Errorf("expected v1.0.0, got %q", resolvedVersion("v1.0.0"))
	}
}

func TestWriteErrors(t *testing.T) {
	// Writer IO error
	err := Write(failWriter{}, Bundle{})
	if err == nil {
		t.Fatal("expected write error, got nil")
	}

	// JSON marshal error
	nan := math.NaN()
	unmarshalableBundle := Bundle{
		Runs: []Run{
			{
				Parts: []Part{
					{
						AIGoal: &AIGoalSection{
							Events: []LoopEvent{
								{
									Usage: Usage{Cost: &nan},
								},
							},
						},
					},
				},
			},
		},
	}
	if err := Write(&strings.Builder{}, unmarshalableBundle); err == nil {
		t.Fatal("expected JSON encode error for math.NaN, got nil")
	}
}

func TestReadErrors(t *testing.T) {
	// Reader error
	if _, err := Read(failReader{}); err == nil {
		t.Fatal("expected read error, got nil")
	}

	// Bad JSON
	if _, err := Read(strings.NewReader("not-json")); err == nil {
		t.Fatal("expected decode error on bad JSON, got nil")
	}

	// Unknown format
	badFormat := `{"format": "https://example.com/other"}`
	if _, err := Read(strings.NewReader(badFormat)); err == nil {
		t.Fatal("expected unknown bundle format error, got nil")
	}

	// Format matches but second decode fails
	badRuns := `{"format": "https://chatwright.dev/formats/run-bundle/v1", "runs": "not-an-array"}`
	if _, err := Read(strings.NewReader(badRuns)); err == nil {
		t.Fatal("expected decode error on bad runs array, got nil")
	}
}
