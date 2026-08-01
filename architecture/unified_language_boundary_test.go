package architecture_test

import (
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"strconv"
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

// One content type, one deep copy. Sampling and Automation each had their own
// implementation for FlowFragmentStep, and they drifted: the sampling one never
// copied Validation.Assertion.ExpectedValues, so an edited draft shared that
// slice with its source. Nothing failed loudly — editing a copy just quietly
// changed the original.
//
// The guard is structural rather than behavioural because the behavioural
// version already existed in spirit (both were "correct" in their own tests) and
// still missed the drift. What has to be prevented is the second implementation.
func TestFlowFragmentStepHasExactlyOneDeepCopy(t *testing.T) {
	root := repositoryRoot(t)
	// A function is a step deep copy if it both takes and returns a step slice.
	isStepSlice := func(expr ast.Expr) bool {
		array, ok := expr.(*ast.ArrayType)
		if !ok {
			return false
		}
		switch element := array.Elt.(type) {
		case *ast.Ident:
			return element.Name == "FlowFragmentStep"
		case *ast.SelectorExpr:
			return element.Sel.Name == "FlowFragmentStep"
		}
		return false
	}

	var found []string
	for _, owner := range []string{"domain", "application"} {
		err := walkProductionGo(filepath.Join(root, owner), func(path string, parsed *ast.File, fset *token.FileSet) {
			relative, _ := filepath.Rel(root, path)
			for _, decl := range parsed.Decls {
				function, ok := decl.(*ast.FuncDecl)
				if !ok || function.Type.Params == nil || function.Type.Results == nil {
					continue
				}
				takesSteps, returnsSteps := false, false
				for _, param := range function.Type.Params.List {
					if isStepSlice(param.Type) {
						takesSteps = true
					}
				}
				for _, result := range function.Type.Results.List {
					if isStepSlice(result.Type) {
						returnsSteps = true
					}
				}
				if !takesSteps || !returnsSteps {
					continue
				}
				// A rewrite transforms content; a copy reproduces it. Only the latter
				// is what must stay unique, and only the latter is named for copying.
				name := strings.ToLower(function.Name.Name)
				if !strings.Contains(name, "clone") && !strings.Contains(name, "copy") {
					continue
				}
				found = append(found, filepath.ToSlash(relative)+":"+
					strconv.Itoa(fset.Position(function.Pos()).Line)+" "+function.Name.Name)
			}
		})
		if err != nil {
			t.Fatalf("walk %s: %v", owner, err)
		}
	}
	if len(found) != 1 {
		t.Errorf("found %d deep copies of flow fragment step content, want exactly one owned by the package that owns the type:\n  %s",
			len(found), strings.Join(found, "\n  "))
	}
}

// The entry executor runs one authorized entry. It used to take a slice and
// loop, which put the order entries run in — and the decision to stop after a
// failure — inside the executor. Both belong to Scheduling: it is the only
// component that sees the whole instance and the only one that commits terminal
// state, so an executor that also sequenced meant two components could disagree
// about what ran with no way to settle it afterwards.
func TestEntryExecutorTakesOneEntryNotACollection(t *testing.T) {
	root := repositoryRoot(t)
	checked := false
	err := walkProductionGo(filepath.Join(root, "application", "execution"), func(path string, parsed *ast.File, fset *token.FileSet) {
		for _, decl := range parsed.Decls {
			function, ok := decl.(*ast.FuncDecl)
			if !ok || function.Name.Name != "Execute" || function.Recv == nil {
				continue
			}
			receiver := ""
			if len(function.Recv.List) == 1 {
				if identifier, ok := function.Recv.List[0].Type.(*ast.Ident); ok {
					receiver = identifier.Name
				}
			}
			if receiver != "EntryExecutor" {
				continue
			}
			checked = true
			for _, param := range function.Type.Params.List {
				if _, isSlice := param.Type.(*ast.ArrayType); isSlice {
					t.Errorf("%s:%d EntryExecutor.Execute takes a collection; sequencing entries is Scheduling's authority, not the executor's",
						filepath.ToSlash(mustRelative(root, path)), fset.Position(function.Pos()).Line)
				}
			}
		}
	})
	if err != nil {
		t.Fatalf("walk application/execution: %v", err)
	}
	if !checked {
		t.Fatal("EntryExecutor.Execute was not found; this boundary needs to move with it")
	}
}

// Declaring the coordinate types was only half the work; the value arrives when
// a plan entry can no longer spell its identity as a bare string. Both halves
// are pinned, because either one alone regresses silently: the field must carry
// the type, and the pre-adoption spelling must not reappear on a neighbouring
// struct as a second, untyped way to say the same thing.
func TestExecutionEntryIdentityIsNeverSpelledAsAString(t *testing.T) {
	root := repositoryRoot(t)
	directory := filepath.Join(root, "domain", "execution")

	checked := false
	err := walkProductionGo(directory, func(path string, parsed *ast.File, fset *token.FileSet) {
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
				structType, isStruct := typeSpec.Type.(*ast.StructType)
				if !isStruct || structType.Fields == nil {
					continue
				}
				for _, field := range structType.Fields.List {
					for _, name := range field.Names {
						if name.Name == "ExecutionID" {
							t.Errorf("%s.ExecutionID at %s:%d revives the pre-adoption spelling of an entry identity; the entry identity is Entry.ID typed EntryID",
								typeSpec.Name.Name, filepath.ToSlash(mustRelative(root, path)), fset.Position(field.Pos()).Line)
						}
						if typeSpec.Name.Name != "Entry" || name.Name != "ID" {
							continue
						}
						checked = true
						if declared := types.ExprString(field.Type); declared != "EntryID" {
							t.Errorf("Entry.ID is %s, want EntryID; a bare string lets an instance id, a step execution id, or an unvalidated request field be passed where an entry identity is meant", declared)
						}
					}
				}
			}
		}
	})
	if err != nil {
		t.Fatalf("walk domain/execution: %v", err)
	}
	if !checked {
		t.Fatal("execution.Entry.ID was not found; this boundary needs to move with it")
	}
}

// An execution coordinate crosses more packages than any other value in the
// model: a run, the node runtime driving it, every evidence observation
// attributed to it, and the scheduling commands that start and stop it all name
// the same three things. While they were strings, any of those could be handed
// the wrong one of the three, or a raw request field, and would carry it as far
// as storage before anyone noticed.
//
// This walks field spellings rather than a fixed list of types, because the
// regression to guard against is a new struct joining the chain with a plain
// string, not one of today's structs changing back.
func TestExecutionCoordinateFieldsAreNeverSpelledAsStrings(t *testing.T) {
	root := repositoryRoot(t)

	// domain/automation is absent on purpose. It is a different bounded context
	// and may not import domain/execution, so its ContributingHealFact carries
	// these as opaque correlation data it never interprets. Application types
	// that mirror such a record verbatim are exempted by name below.
	directories := []string{
		filepath.Join(root, "domain", "execution"),
		filepath.Join(root, "domain", "evidence"),
		filepath.Join(root, "domain", "node"),
		filepath.Join(root, "application"),
	}
	mirrorsAutomationRecord := map[string]bool{"HealContributionSnapshot": true}
	wantType := map[string]string{
		"RunID":           "InstanceID",
		"ExecutionID":     "EntryID",
		"NextExecutionID": "EntryID",
		"StepExecutionID": "StepExecutionID",
		"StepExecution":   "StepExecutionID",
	}

	checked := 0
	for _, directory := range directories {
		err := walkProductionGo(directory, func(path string, parsed *ast.File, fset *token.FileSet) {
			for _, decl := range parsed.Decls {
				generic, ok := decl.(*ast.GenDecl)
				if !ok {
					continue
				}
				for _, spec := range generic.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok || mirrorsAutomationRecord[typeSpec.Name.Name] {
						continue
					}
					structType, isStruct := typeSpec.Type.(*ast.StructType)
					if !isStruct || structType.Fields == nil {
						continue
					}
					for _, field := range structType.Fields.List {
						declared := types.ExprString(field.Type)
						for _, name := range field.Names {
							want, tracked := wantType[name.Name]
							if !tracked {
								continue
							}
							checked++
							if !strings.HasSuffix(declared, want) {
								t.Errorf("%s.%s at %s:%d is %s, want %s",
									typeSpec.Name.Name, name.Name, filepath.ToSlash(mustRelative(root, path)),
									fset.Position(field.Pos()).Line, declared, want)
							}
						}
					}
				}
			}
		})
		if err != nil {
			t.Fatalf("walk %s: %v", mustRelative(root, directory), err)
		}
	}
	if checked == 0 {
		t.Fatal("no coordinate field was found across execution, evidence, node, and application; this boundary needs to move with them")
	}
}

func mustRelative(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return relative
}
