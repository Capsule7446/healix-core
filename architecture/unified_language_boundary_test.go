package architecture_test

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// The unified-domain-language refactor lists a set of conditions under which its
// work must not be merged. Several of those boundaries already hold today. They
// are pinned here so the refactor cannot quietly regress one while moving a
// different part of the model — a regression that would otherwise only surface
// once the lifecycle split was already published.
//
// Boundaries that do NOT hold yet are deliberately absent: a guard asserting a
// future state is a failing test, not a contract. They are recorded in the
// program's own plan as the work of their phase.

// structFields returns the field names of a named struct type declared anywhere
// under dir, and reports whether the type was found at all. A boundary asserted
// against a type that has been renamed away is worse than no boundary, so every
// caller checks the found flag.
func structFields(t *testing.T, dir string, typeName string) ([]string, bool) {
	t.Helper()
	var fields []string
	found := false
	err := walkProductionGo(dir, func(path string, parsed *ast.File, fset *token.FileSet) {
		for _, decl := range parsed.Decls {
			generic, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range generic.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok || typeSpec.Name.Name != typeName {
					continue
				}
				structType, ok := typeSpec.Type.(*ast.StructType)
				if !ok || structType.Fields == nil {
					continue
				}
				found = true
				for _, field := range structType.Fields.List {
					for _, name := range field.Names {
						fields = append(fields, name.Name)
					}
				}
			}
		}
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return fields, found
}

// Formal identity is Automation's to assign, and only after a publication
// succeeds. An unpublished asset that carried a Version, a Revision, or a
// Current pointer could be executed or referenced before anything published it.
func TestUnpublishedSamplingAssetsCarryNoFormalIdentity(t *testing.T) {
	root := repositoryRoot(t)
	sampling := filepath.Join(root, "domain", "sampling")

	// ExistingElementTargetID is a reference to something already published, not
	// this asset's own identity, so it is not a formal-identity field.
	forbidden := []string{"Version", "VersionNumber", "Revision", "CurrentVersionID", "ElementTargetVersionID"}
	for _, typeName := range []string{"UnpublishedElementTarget", "UnpublishedFlowFragment"} {
		fields, found := structFields(t, sampling, typeName)
		if !found {
			t.Fatalf("%s no longer exists in domain/sampling; this boundary needs to move with it", typeName)
		}
		for _, field := range fields {
			for _, banned := range forbidden {
				if field == banned {
					t.Errorf("%s.%s gives an unpublished asset formal identity, which only Automation may assign after a successful publication", typeName, field)
				}
			}
		}
	}
}

// The reverse direction: once an asset is published, nothing about the sampling
// session that produced it may survive in the immutable version. A temporary id
// reaching a formal version would make a published asset depend on a draft that
// no longer exists.
func TestPublishedVersionsCarryNoTemporaryIdentity(t *testing.T) {
	root := repositoryRoot(t)
	automation := filepath.Join(root, "domain", "automation")

	for _, typeName := range []string{"ElementTargetVersion", "FlowFragmentVersion", "ExecutionFlowVersion"} {
		fields, found := structFields(t, automation, typeName)
		if !found {
			t.Fatalf("%s no longer exists in domain/automation; this boundary needs to move with it", typeName)
		}
		for _, field := range fields {
			if strings.Contains(field, "Temporary") {
				t.Errorf("%s.%s keeps a sampling-time identity on a published immutable version", typeName, field)
			}
		}
	}
}

// An execution instance, one top-level entry within it, and one fragment call
// site inside that entry are three different things. While they were all plain
// strings, passing one where another belonged compiled fine — and the plan's
// stop conditions call that out specifically, because evidence attributed to the
// wrong level is silently wrong rather than visibly broken.
func TestExecutionCoordinatesAreDistinctTypesNotStrings(t *testing.T) {
	root := repositoryRoot(t)
	execution := filepath.Join(root, "domain", "execution")

	wanted := map[string]bool{"InstanceID": false, "EntryID": false, "InvocationPath": false}
	err := walkProductionGo(execution, func(path string, parsed *ast.File, fset *token.FileSet) {
		for _, decl := range parsed.Decls {
			generic, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range generic.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if _, tracked := wanted[typeSpec.Name.Name]; !tracked {
					continue
				}
				// A defined struct wrapping an unexported field cannot be produced
				// from a bare string by conversion, which is the property that makes
				// the three impossible to confuse.
				structType, isStruct := typeSpec.Type.(*ast.StructType)
				if !isStruct || typeSpec.Assign.IsValid() {
					t.Errorf("%s must be a defined struct type, not an alias or a named string, or the compiler cannot keep the three coordinates apart", typeSpec.Name.Name)
					continue
				}
				exported := 0
				if structType.Fields != nil {
					for _, field := range structType.Fields.List {
						for _, name := range field.Names {
							if name.IsExported() {
								exported++
							}
						}
					}
				}
				if exported != 0 {
					t.Errorf("%s exposes %d exported field(s); an exported field lets a caller build one coordinate from another's value", typeSpec.Name.Name, exported)
				}
				wanted[typeSpec.Name.Name] = true
			}
		}
	})
	if err != nil {
		t.Fatalf("walk domain/execution: %v", err)
	}
	for name, seen := range wanted {
		if !seen {
			t.Errorf("%s is not declared in domain/execution; the three execution coordinates must stay separately typed", name)
		}
	}
}

// The refactor is a straight replacement: no compatibility facade, no old name
// kept alive beside the new one. An exported type alias is the cheapest way to
// break that — `type OldName = NewName` costs one line, compiles, and leaves the
// old vocabulary reachable forever. One such alias was already in the tree
// (NodeExecutionRef, left over from the step-execution rename) before this guard
// existed, so the failure mode is demonstrated rather than hypothetical.
//
// Unexported aliases are allowed: they are internal shorthand and cannot keep an
// old public name alive.
func TestNoExportedTypeAliasKeepsAnOldNameAlive(t *testing.T) {
	root := repositoryRoot(t)
	for _, owner := range []string{"domain", "application"} {
		err := walkProductionGo(filepath.Join(root, owner), func(path string, parsed *ast.File, fset *token.FileSet) {
			relative, _ := filepath.Rel(root, path)
			for _, decl := range parsed.Decls {
				generic, ok := decl.(*ast.GenDecl)
				if !ok {
					continue
				}
				for _, spec := range generic.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok || !typeSpec.Assign.IsValid() || !typeSpec.Name.IsExported() {
						continue
					}
					t.Errorf("%s:%d declares the exported alias %s; the refactor replaces names outright, so an alias keeping an old public name reachable is a compatibility facade",
						filepath.ToSlash(relative), fset.Position(typeSpec.Pos()).Line, typeSpec.Name.Name)
				}
			}
		})
		if err != nil {
			t.Fatalf("walk %s: %v", owner, err)
		}
	}
}

// A deprecation marker is the other shape of the same promise: it says an old
// name still works. The plan offers no deprecation window — the replacement is
// the migration.
func TestNoDeprecationMarkersPromiseAnOldNameStillWorks(t *testing.T) {
	root := repositoryRoot(t)
	for _, owner := range []string{"domain", "application"} {
		err := walkProductionGo(filepath.Join(root, owner), func(path string, parsed *ast.File, fset *token.FileSet) {
			relative, _ := filepath.Rel(root, path)
			for _, group := range parsed.Comments {
				for _, comment := range group.List {
					if strings.Contains(comment.Text, "Deprecated:") {
						t.Errorf("%s:%d carries a Deprecated marker; the refactor has no deprecation window, so an old name is either gone or it was never replaced",
							filepath.ToSlash(relative), fset.Position(comment.Pos()).Line)
					}
				}
			}
		})
		if err != nil {
			t.Fatalf("walk %s: %v", owner, err)
		}
	}
}
