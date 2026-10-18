package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/invopop/jsonschema"
)

func TestRepoRoot(t *testing.T) {
	// Normal execution in repo
	root, err := repoRoot()
	if err != nil {
		t.Fatalf("repoRoot() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("expected go.mod in repoRoot: %v", err)
	}

	// getwd error
	origGetwd := getwd
	defer func() { getwd = origGetwd }()
	getwd = func() (string, error) {
		return "", errors.New("getwd failed")
	}
	if _, err := repoRoot(); err == nil {
		t.Error("expected error when getwd fails")
	}

	// no go.mod found above root
	tempDir := t.TempDir()
	// resolve symlinks if any
	realTemp, err := filepath.EvalSymlinks(tempDir)
	if err == nil {
		tempDir = realTemp
	}
	getwd = func() (string, error) {
		return tempDir, nil
	}
	// Unless tempDir happens to be inside a git repo with go.mod above it:
	// To guarantee parent == dir without go.mod, set dir to "/"
	getwd = func() (string, error) {
		return "/", nil
	}
	if _, err := repoRoot(); err == nil {
		t.Error("expected error when root reached without go.mod")
	}
}

func TestRun_Errors(t *testing.T) {
	// schemagenGenerate error
	origGen := schemagenGenerate
	origMarshal := schemagenMarshal
	origFindRoot := findRepoRoot
	defer func() {
		schemagenGenerate = origGen
		schemagenMarshal = origMarshal
		findRepoRoot = origFindRoot
	}()

	schemagenGenerate = func() (*jsonschema.Schema, error) {
		return nil, errors.New("generate failed")
	}
	if err := run(); err == nil {
		t.Error("expected error when schemagenGenerate fails")
	}
	schemagenGenerate = origGen

	// schemagenMarshal error
	schemagenMarshal = func(*jsonschema.Schema) ([]byte, error) {
		return nil, errors.New("marshal failed")
	}
	if err := run(); err == nil {
		t.Error("expected error when schemagenMarshal fails")
	}
	schemagenMarshal = origMarshal

	// findRepoRoot error
	findRepoRoot = func() (string, error) {
		return "", errors.New("findRepoRoot failed")
	}
	if err := run(); err == nil {
		t.Error("expected error when findRepoRoot fails")
	}

	// MkdirAll error (file exists where dir should be)
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "formats")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	findRepoRoot = func() (string, error) {
		return tempDir, nil
	}
	if err := run(); err == nil {
		t.Error("expected MkdirAll error")
	}

	// WriteFile error (target path is a directory)
	tempDir2 := t.TempDir()
	targetPath := filepath.Join(tempDir2, filepath.FromSlash(outputPath))
	if err := os.MkdirAll(targetPath, 0o755); err != nil {
		t.Fatal(err)
	}
	findRepoRoot = func() (string, error) {
		return tempDir2, nil
	}
	if err := run(); err == nil {
		t.Error("expected WriteFile error when path is a directory")
	}
}

func TestMain(t *testing.T) {
	origExit := osExit
	origGen := schemagenGenerate
	defer func() {
		osExit = origExit
		schemagenGenerate = origGen
	}()

	// main error branch
	var exitCode int
	osExit = func(code int) {
		exitCode = code
	}
	schemagenGenerate = func() (*jsonschema.Schema, error) {
		return nil, errors.New("generate failed")
	}
	main()
	if exitCode != 1 {
		t.Errorf("expected exitCode 1, got %d", exitCode)
	}

	// main success branch
	exitCode = 0
	schemagenGenerate = origGen
	tempDir := t.TempDir()
	origFindRoot := findRepoRoot
	defer func() { findRepoRoot = origFindRoot }()
	findRepoRoot = func() (string, error) {
		return tempDir, nil
	}
	main()
	if exitCode != 0 {
		t.Errorf("expected exitCode 0 on success, got %d", exitCode)
	}
}
