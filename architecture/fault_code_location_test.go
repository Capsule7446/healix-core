package architecture_test

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// AGENTS.md asks that a package's fault.Code constants be declared in its own
// fault_codes.go. Every domain package does. No application package did: 69
// codes sat in 17 feature files, and nothing said so.
//
// The single entry point is what the rule buys. Codes are a public
// compatibility contract — add or tombstone only, never rename or reuse — and
// an auditor checking that has to see the whole set at once. Spread across the
// features that raise them, "all of them" means grepping and trusting the grep.
//
// unconsolidated lists the packages that have not been brought into line yet.
// It exists so this guard can be true today rather than aspirational, and it
// may only shrink: adding an entry is admitting a package moved backwards.
// Consolidating one is a mechanical same-package move with no API, behaviour or
// digest consequence, so the list is a queue, not a standoff.
var unconsolidated = map[string]string{
	"application/automation": "14 codes across 4 files",
	"application/scheduling": "21 codes across 4 files",
	"application/engine":     "6 codes across 2 files",
	// These two have the same gap inside domain/, where every other package
	// already complies. automation's folders.go is the least urgent of all of
	// them: its four codes are registered for retirement and will be deleted
	// with the hierarchy, so moving them first would be churn on code that is
	// leaving.
	"domain/automation":    "codes also in folders.go, healing.go, lifecycle.go",
	"domain/interpolation": "no fault_codes.go; codes in variables.go",
}

// TestFaultCodesAreDeclaredInOneFilePerPackage holds every package outside the
// shrinking list to the rule.
func TestFaultCodesAreDeclaredInOneFilePerPackage(t *testing.T) {
	root := repositoryRoot(t)
	// package dir -> the non-fault_codes.go files that declare a code
	stray := map[string][]string{}
	consolidated := map[string]bool{}

	for _, owner := range []string{"domain", "application"} {
		err := walkProductionGo(filepath.Join(root, owner), func(path string, parsed *ast.File, _ *token.FileSet) {
			if !declaresAFaultCode(parsed) {
				return
			}
			relative, _ := filepath.Rel(root, path)
			key := filepath.ToSlash(relative)
			pkg := filepath.ToSlash(filepath.Dir(key))
			if filepath.Base(key) == "fault_codes.go" {
				consolidated[pkg] = true
				return
			}
			stray[pkg] = append(stray[pkg], filepath.Base(key))
		})
		if err != nil {
			t.Fatalf("walk %s: %v", owner, err)
		}
	}

	for _, pkg := range sortedPackages(stray) {
		if _, known := unconsolidated[pkg]; known {
			continue
		}
		files := stray[pkg]
		sort.Strings(files)
		t.Errorf("%s declares fault codes outside fault_codes.go (%v); a package's codes are a public compatibility contract and must be auditable in one place",
			pkg, files)
	}

	// A package that is both listed and clean has been consolidated without the
	// list being updated. Left alone, the list would keep permitting a
	// regression nobody intended.
	for pkg := range unconsolidated {
		if len(stray[pkg]) == 0 && consolidated[pkg] {
			t.Errorf("%s is listed as unconsolidated but declares every code in fault_codes.go; remove the entry", pkg)
		}
	}
}

// declaresAFaultCode reports whether the file declares a constant or variable
// of type fault.Code. It deliberately does not match every mention: a function
// taking a `code fault.Code` parameter, or a switch over one, uses the type
// without publishing a value, and counting those would have named almost every
// file that handles an error.
func declaresAFaultCode(parsed *ast.File) bool {
	for _, declaration := range parsed.Decls {
		generic, ok := declaration.(*ast.GenDecl)
		if !ok || (generic.Tok != token.CONST && generic.Tok != token.VAR) {
			continue
		}
		for _, spec := range generic.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok || valueSpec.Type == nil {
				continue
			}
			selector, ok := valueSpec.Type.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "Code" {
				continue
			}
			if pkg, ok := selector.X.(*ast.Ident); ok && pkg.Name == "fault" {
				return true
			}
		}
	}
	return false
}

func sortedPackages(byPackage map[string][]string) []string {
	names := make([]string, 0, len(byPackage))
	for name := range byPackage {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// TestUnconsolidatedListNamesRealPackages keeps the queue honest: an entry for
// a package that no longer exists, or that never declared a code, would sit
// there permitting nothing and never be noticed.
func TestUnconsolidatedListNamesRealPackages(t *testing.T) {
	root := repositoryRoot(t)
	for pkg := range unconsolidated {
		if !strings.HasPrefix(pkg, "application/") && !strings.HasPrefix(pkg, "domain/") {
			t.Errorf("%s is outside the owned trees", pkg)
			continue
		}
		declares := false
		err := walkProductionGo(filepath.Join(root, filepath.FromSlash(pkg)), func(_ string, parsed *ast.File, _ *token.FileSet) {
			if declaresAFaultCode(parsed) {
				declares = true
			}
		})
		if err != nil {
			t.Errorf("%s is listed as unconsolidated but cannot be walked: %v", pkg, err)
			continue
		}
		if !declares {
			t.Errorf("%s is listed as unconsolidated but declares no fault code; remove the entry", pkg)
		}
	}
}
