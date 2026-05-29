// Package arch contains architectural constraint tests.
// These tests enforce the import rules stated in core/CLAUDE.md so that
// violations are caught by `go test` rather than by human review.
package arch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// pkgInfo holds the subset of `go list -json` output we need.
type pkgInfo struct {
	ImportPath string
	Imports    []string
}

// loadPackages runs `go list -json -deps ./...` from coreDir and returns a
// slice of all packages in the dependency graph.
func loadPackages(t *testing.T) []pkgInfo {
	t.Helper()

	// Locate the core/ directory relative to this test file.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile: .../core/internal/arch/arch_test.go → go up two levels to core/
	// Dir(thisFile) = .../arch/ → ../.. = .../core/
	coreDir := filepath.Join(filepath.Dir(thisFile), "..", "..")
	coreDir, err := filepath.Abs(coreDir)
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}

	cmd := exec.Command("go", "list", "-json", "-deps", "./...")
	cmd.Dir = coreDir
	// Disable Go workspace so go list operates on the core/ module only.
	cmd.Env = append(os.Environ(), "GOWORK=off")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list (coreDir=%s): %v\nstderr: %s", coreDir, err, stderr.String())
	}

	var pkgs []pkgInfo
	dec := json.NewDecoder(bytes.NewReader(out))
	for dec.More() {
		var p pkgInfo
		if err := dec.Decode(&p); err != nil {
			t.Fatalf("json.Decode: %v", err)
		}
		pkgs = append(pkgs, p)
	}
	return pkgs
}

// isCore returns true if the import path belongs to the arrowhead/core module.
func isCore(path string) bool {
	return strings.HasPrefix(path, "arrowhead/core/")
}

// isCoreInternal returns true if the import path is inside arrowhead/core/internal/.
func isCoreInternal(path string) bool {
	return strings.HasPrefix(path, "arrowhead/core/internal/")
}

// isCoreCmd returns true if the import path is inside arrowhead/core/cmd/.
func isCoreCmd(path string) bool {
	return strings.HasPrefix(path, "arrowhead/core/cmd/")
}

// matchesLayer returns true when the last two segments of path are layer/pkg
// (e.g., .../authentication/model matches layer="model").
func matchesLayer(path, layer string) bool {
	parts := strings.Split(path, "/")
	return len(parts) >= 1 && parts[len(parts)-1] == layer
}

// TestNoExternalImportsOfCoreInternal asserts that no package outside the
// arrowhead/core module imports arrowhead/core/internal/.
func TestNoExternalImportsOfCoreInternal(t *testing.T) {
	pkgs := loadPackages(t)
	var failures []string
	for _, p := range pkgs {
		if isCore(p.ImportPath) {
			continue // core packages may import core/internal — that's fine
		}
		for _, imp := range p.Imports {
			if isCoreInternal(imp) {
				failures = append(failures,
					fmt.Sprintf("  %s imports %s (rule: no package outside core/ may import core/internal/)",
						p.ImportPath, imp))
			}
		}
	}
	if len(failures) > 0 {
		t.Errorf("architectural violation — external packages importing core/internal/:\n%s",
			strings.Join(failures, "\n"))
	}
}

// TestModelPackagesDoNotImportNonModelInternals asserts that model/ packages
// only import other model/ packages from core/internal/ — never service/,
// api/, or repository/ packages. Model-to-model imports are permitted because
// arrowhead/core/internal/orchestration/model is the designated shared types
// package (per core/CLAUDE.md "Shared orchestration types").
func TestModelPackagesDoNotImportNonModelInternals(t *testing.T) {
	pkgs := loadPackages(t)
	var failures []string
	for _, p := range pkgs {
		if !isCoreInternal(p.ImportPath) {
			continue
		}
		if !matchesLayer(p.ImportPath, "model") {
			continue
		}
		for _, imp := range p.Imports {
			if isCoreInternal(imp) && !matchesLayer(imp, "model") {
				failures = append(failures,
					fmt.Sprintf("  %s imports %s (rule: model/ packages must not import non-model core/internal/ packages)",
						p.ImportPath, imp))
			}
		}
	}
	if len(failures) > 0 {
		t.Errorf("architectural violation — model/ packages with non-model internal imports:\n%s",
			strings.Join(failures, "\n"))
	}
}

// TestAPIPackagesDoNotImportRepository asserts that api/ packages do not
// import repository/ packages directly (must go through service/).
func TestAPIPackagesDoNotImportRepository(t *testing.T) {
	pkgs := loadPackages(t)
	var failures []string
	for _, p := range pkgs {
		if !isCoreInternal(p.ImportPath) {
			continue
		}
		if !matchesLayer(p.ImportPath, "api") {
			continue
		}
		for _, imp := range p.Imports {
			if isCoreInternal(imp) && matchesLayer(imp, "repository") {
				failures = append(failures,
					fmt.Sprintf("  %s imports %s (rule: api/ must not import repository/ directly)",
						p.ImportPath, imp))
			}
		}
	}
	if len(failures) > 0 {
		t.Errorf("architectural violation — api/ packages importing repository/:\n%s",
			strings.Join(failures, "\n"))
	}
}

// TestServicePackagesDoNotImportAPI asserts that service/ packages do not
// import api/ packages (service layer must not depend on HTTP layer).
func TestServicePackagesDoNotImportAPI(t *testing.T) {
	pkgs := loadPackages(t)
	var failures []string
	for _, p := range pkgs {
		if !isCoreInternal(p.ImportPath) {
			continue
		}
		if !matchesLayer(p.ImportPath, "service") {
			continue
		}
		for _, imp := range p.Imports {
			if isCoreInternal(imp) && matchesLayer(imp, "api") {
				failures = append(failures,
					fmt.Sprintf("  %s imports %s (rule: service/ must not import api/)",
						p.ImportPath, imp))
			}
		}
	}
	if len(failures) > 0 {
		t.Errorf("architectural violation — service/ packages importing api/:\n%s",
			strings.Join(failures, "\n"))
	}
}

// TestNothingImportsCmdPackages asserts that no package imports arrowhead/core/cmd/.
func TestNothingImportsCmdPackages(t *testing.T) {
	pkgs := loadPackages(t)
	var failures []string
	for _, p := range pkgs {
		for _, imp := range p.Imports {
			if isCoreCmd(imp) {
				failures = append(failures,
					fmt.Sprintf("  %s imports %s (rule: no package may import cmd/)",
						p.ImportPath, imp))
			}
		}
	}
	if len(failures) > 0 {
		t.Errorf("architectural violation — packages importing cmd/:\n%s",
			strings.Join(failures, "\n"))
	}
}

// TestMain can be used for any test-level setup if needed in the future.
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
