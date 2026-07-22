// Command gen regenerates formats/run-bundle/v1/schema.json from this
// module's Go types via internal/schemagen. Run it from the repository root
// (e.g. via bundle/bundle.go's //go:generate directive, or by hand as
// `go run ./internal/schemagen/gen`) whenever a change to the types
// schemagen.Generate reflects — bundle.Bundle and everything it embeds —
// should be reflected in the committed schema.
//
// bundle/schema_test.go's drift-guard test fails the build if this command
// has not been re-run after such a change: it is the only supported way to
// update the committed file, deliberately never a hand-edit.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chatwright/chatwright/internal/schemagen"
)

// outputPath is repository-root-relative, matching where
// bundle/schema_test.go reads the committed file from.
const outputPath = "formats/run-bundle/v1/schema.json"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	schema, err := schemagen.Generate()
	if err != nil {
		return err
	}
	data, err := schemagen.Marshal(schema)
	if err != nil {
		return err
	}

	root, err := repoRoot()
	if err != nil {
		return err
	}
	path := filepath.Join(root, filepath.FromSlash(outputPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("gen: create %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil { //nolint:gosec // schema.json is not sensitive
		return fmt.Errorf("gen: write %s: %w", path, err)
	}
	fmt.Println("wrote", path)
	return nil
}

// repoRoot locates the module root by walking up from the current working
// directory to the nearest go.mod — go:generate runs with the working
// directory set to the file carrying the directive (bundle/), and a plain
// `go run ./internal/schemagen/gen` from the repository root also needs to
// resolve the same outputPath, so this cannot simply assume either cwd.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("gen: getwd: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("gen: no go.mod found above %s", dir)
		}
		dir = parent
	}
}
