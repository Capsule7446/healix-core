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

	// ExistingNodeID is a reference to something already published, not
	// this asset's own identity, so it is not a formal-identity field.
	//
	// The Saved / Existing boundary is a semantic one, not syntactic: "Saved" says
	// the asset holds a formal identity before publication, which is what this
	// guard prevents; "Existing" says the asset references something already
	// published, which is fine. Any new prefix must be evaluated against that
	// distinction — a field starting with "Saved", "Published", "Promoted", or
	// "Formal" is a formal identity leak unless proven otherwise.
	forbidden := []string{"Version", "VersionNumber", "Revision", "CurrentVersionID", "ElementTargetVersionID"}
	formalPrefixes := []string{"Saved", "Published", "Promoted", "Formal"}
	for _, typeName := range []string{"UnpublishedElementTarget", "UnpublishedFlowFragment"} {
		fields, found := structFields(t, sampling, typeName)
		if !found {
			t.Fatalf("%s no longer exists in domain/sampling; this boundary needs to move with it", typeName)
		}
		for _, field := range fields {
			// A formal identity does not stop being one because the field name says
			// the asset merely "saved" it. Substring matching against the forbidden
			// list is not enough on its own: SavedWorkflowID contains none of those
			// words, yet it is the same category of leak.
			for _, banned := range forbidden {
				if strings.Contains(field, banned) {
					t.Errorf("%s.%s gives an unpublished asset formal identity (forbidden substring %q), which only Automation may assign after a successful publication", typeName, field, banned)
				}
			}
			for _, prefix := range formalPrefixes {
				if strings.HasPrefix(field, prefix) {
					t.Errorf("%s.%s gives an unpublished asset formal identity (forbidden prefix %q), which only Automation may assign after a successful publication", typeName, field, prefix)
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

// TestNoExportedConstAliasKeepsAnOldNameAlive closes the first of two holes the
// type-alias guard above left open. `const Old Status = New` is the same
// compatibility facade in a shape typeSpec.Assign cannot see, and the tree
// carried exactly that: StatusCompleted/StatusFailed aliased StatusEnded and
// StatusInterrupted, documented as a known leftover rather than caught.
//
// The rule is narrow on purpose. Only a const whose value is a bare exported
// identifier counts -- a second name for one existing value. Computed values,
// literals, and iota members are ordinary vocabulary.
func TestNoExportedConstAliasKeepsAnOldNameAlive(t *testing.T) {
	// A "current version" pointer has the same shape as an old-name alias and
	// the opposite meaning: it names the newest member so construction sites
	// follow a schema bump without being edited. AST cannot tell the two
	// directions apart, so the exceptions are listed rather than inferred, and
	// adding one is an edit a reviewer sees.
	permitted := map[string]string{
		"InstanceSnapshotSchemaCurrent": "names the newest schema, not a retired name",
	}
	root := repositoryRoot(t)
	for _, owner := range []string{"domain", "application"} {
		err := walkProductionGo(filepath.Join(root, owner), func(path string, parsed *ast.File, fset *token.FileSet) {
			relative, _ := filepath.Rel(root, path)
			for _, decl := range parsed.Decls {
				generic, ok := decl.(*ast.GenDecl)
				if !ok || generic.Tok != token.CONST {
					continue
				}
				// A const spec with no expression list repeats the previous
				// one. `const ( New = Existing; Old )` therefore declares two
				// exported names for one value while only the first spec
				// carries the identifier, so reading each spec in isolation
				// would let the second through — the very shape being hunted,
				// spelled in the one way that costs no characters at all.
				var inherited []ast.Expr
				for _, spec := range generic.Specs {
					valueSpec, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					values := valueSpec.Values
					if len(values) == 0 {
						values = inherited
					} else {
						inherited = values
					}
					for index, name := range valueSpec.Names {
						if !name.IsExported() || index >= len(values) {
							continue
						}
						alias, ok := values[index].(*ast.Ident)
						if !ok || !alias.IsExported() {
							continue
						}
						if _, allowed := permitted[name.Name]; allowed {
							continue
						}
						t.Errorf("%s:%d declares the exported constant %s as a second name for %s; the refactor replaces names outright, so a constant alias keeping an old public name reachable is a compatibility facade",
							filepath.ToSlash(relative), fset.Position(name.Pos()).Line, name.Name, alias.Name)
					}
				}
			}
		})
		if err != nil {
			t.Fatalf("walk %s: %v", owner, err)
		}
	}
}

// TestNoExportedMethodAliasKeepsAnOldNameAlive closes the second hole. A method
// whose entire body forwards to another exported method on the same receiver is
// a second name for one operation, which is what the two guards above forbid in
// the type and constant shapes. Session.Complete forwarding to End and
// Session.Fail forwarding to Interrupt were both in the tree.
//
// Only a single-statement body counts. A method that forwards and then does
// anything else has behaviour of its own and is not an alias.
func TestNoExportedMethodAliasKeepsAnOldNameAlive(t *testing.T) {
	root := repositoryRoot(t)
	for _, owner := range []string{"domain", "application"} {
		err := walkProductionGo(filepath.Join(root, owner), func(path string, parsed *ast.File, fset *token.FileSet) {
			relative, _ := filepath.Rel(root, path)
			for _, decl := range parsed.Decls {
				function, ok := decl.(*ast.FuncDecl)
				if !ok || function.Recv == nil || !function.Name.IsExported() {
					continue
				}
				if function.Body == nil || len(function.Body.List) != 1 {
					continue
				}
				forwarded, ok := forwardedReceiverMethod(function)
				if !ok {
					continue
				}
				t.Errorf("%s:%d declares the exported method %s whose whole body forwards to %s on the same receiver; the refactor replaces names outright, so a forwarding method keeping an old public name reachable is a compatibility facade",
					filepath.ToSlash(relative), fset.Position(function.Name.Pos()).Line, function.Name.Name, forwarded)
			}
		})
		if err != nil {
			t.Fatalf("walk %s: %v", owner, err)
		}
	}
}

// forwardedReceiverMethod reports the exported method a single-statement body
// forwards to on its own receiver, in either the `return r.Other()` or bare
// `r.Other()` shape.
//
// The arguments must be this method's own parameters, in order and unchanged.
// That requirement is what separates a second name from a specialisation:
// `return r.Convert(DefaultOptions)` supplies a value of its own and therefore
// means something `Convert` alone does not, so it is not an alias however short
// it is.
//
// Known limitation: without type information a call on an exported
// function-typed field is indistinguishable from a method call, so a method
// forwarding its parameters to such a field would be reported. No such field
// exists in the tree. Should one appear, the answer is to argue it at the
// failure — the shape really is a second name for one operation — not to widen
// the rule.
func forwardedReceiverMethod(function *ast.FuncDecl) (string, bool) {
	if len(function.Recv.List) == 0 || len(function.Recv.List[0].Names) == 0 {
		return "", false
	}
	receiver := function.Recv.List[0].Names[0].Name

	var call *ast.CallExpr
	switch statement := function.Body.List[0].(type) {
	case *ast.ReturnStmt:
		if len(statement.Results) != 1 {
			return "", false
		}
		call, _ = statement.Results[0].(*ast.CallExpr)
	case *ast.ExprStmt:
		call, _ = statement.X.(*ast.CallExpr)
	}
	if call == nil {
		return "", false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !selector.Sel.IsExported() {
		return "", false
	}
	target, ok := selector.X.(*ast.Ident)
	if !ok || target.Name != receiver {
		return "", false
	}
	if !forwardsOwnParameters(function, call) {
		return "", false
	}
	return selector.Sel.Name, true
}

// forwardsOwnParameters reports whether a call passes exactly this function's
// parameters, in declaration order and untouched. A literal, a constant, a
// reordering, or a dropped parameter all mean the call is doing something of
// its own.
func forwardsOwnParameters(function *ast.FuncDecl, call *ast.CallExpr) bool {
	var parameters []string
	if function.Type.Params != nil {
		for _, field := range function.Type.Params.List {
			if len(field.Names) == 0 {
				// An unnamed parameter cannot be forwarded by name, so the call
				// cannot be passing it along.
				return false
			}
			for _, name := range field.Names {
				parameters = append(parameters, name.Name)
			}
		}
	}
	if len(call.Args) != len(parameters) {
		return false
	}
	for index, argument := range call.Args {
		identifier, ok := argument.(*ast.Ident)
		if !ok || identifier.Name != parameters[index] {
			return false
		}
	}
	return true
}

// retiringFiles is the retirement register. A Deprecated marker is allowed
// only here, and every entry must also appear in
// docs/contracts/retirement-plan.md.
//
// The distinction this register draws is the whole reason the guard below is
// not a flat ban. Two different things wear the same marker:
//
//   - A rename. The new name is in Core, so the old one buys nothing and the
//     replacement IS the migration. Still forbidden, as it always was.
//   - An ownership handoff. The capability is leaving Core entirely and its
//     replacement lives in the host. Core cannot delete it before hosts have
//     built their own, so a window is not a convenience -- it is the only
//     order the removal can happen in.
//
// wholeFile marks a file whose entire exported surface is retiring, which the
// companion guard uses to require that nothing there grows unmarked.
//
// symbols is the opposite case and must be exact. A file that is only partly
// retiring cannot be waved through wholesale: assets.go holds every automation
// aggregate, and registering the file rather than the two fields would let a
// marker appear on anything in it. Struct fields are named Type.Field.
type retirement struct {
	wholeFile bool
	symbols   []string
	reason    string
}

var retiringFiles = map[string]retirement{
	"domain/automation/folders.go":                {wholeFile: true, reason: "folder hierarchy moves to the host"},
	"application/automation/folder_service.go":    {wholeFile: true, reason: "folder hierarchy moves to the host"},
	"application/automation/folder_repository.go": {wholeFile: true, reason: "folder hierarchy moves to the host"},
	"domain/automation/assets.go": {
		symbols: []string{"ElementTarget.FolderID", "FlowFragment.FolderID"},
		reason:  "the FolderID back-reference retires with the hierarchy",
	},
}

// TestNoDeprecationMarkersPromiseAnOldNameStillWorks keeps the original rule
// for everything outside the register: a marker that says an old name still
// works is a compatibility facade, and Core does not offer that window.
func TestNoDeprecationMarkersPromiseAnOldNameStillWorks(t *testing.T) {
	root := repositoryRoot(t)
	for _, owner := range []string{"domain", "application"} {
		err := walkProductionGo(filepath.Join(root, owner), func(path string, parsed *ast.File, fset *token.FileSet) {
			relative, _ := filepath.Rel(root, path)
			key := filepath.ToSlash(relative)
			entry, retiring := retiringFiles[key]
			if retiring && entry.wholeFile {
				return
			}
			if retiring {
				// A partly retiring file is checked symbol by symbol. Skipping it
				// wholesale, as an earlier draft did, made the register's reason
				// text the only thing limiting what could be deprecated there —
				// and reason text is not a check.
				allowed := make(map[string]bool, len(entry.symbols))
				for _, symbol := range entry.symbols {
					allowed[symbol] = true
				}
				marked := markedSymbolsIn(parsed)
				for _, symbol := range marked {
					if !allowed[symbol.name] {
						t.Errorf("%s:%d marks %s as deprecated, which the retirement register does not list for this file; %s covers only %v",
							key, fset.Position(symbol.pos).Line, symbol.name, key, entry.symbols)
					}
				}
				// A marker attached to nothing the walk recognises is still a
				// marker, and it would otherwise be invisible to the loop above.
				if strays := countDeprecationMarkers(parsed) - len(marked); strays > 0 {
					t.Errorf("%s carries %d Deprecated marker(s) not attached to any exported declaration or field", key, strays)
				}
				return
			}
			for _, group := range parsed.Comments {
				for _, comment := range group.List {
					if strings.Contains(comment.Text, "Deprecated:") {
						t.Errorf("%s:%d carries a Deprecated marker outside the retirement register; a rename has no deprecation window, and a capability leaving Core must be registered in retiringFiles and docs/contracts/retirement-plan.md",
							key, fset.Position(comment.Pos()).Line)
					}
				}
			}
		})
		if err != nil {
			t.Fatalf("walk %s: %v", owner, err)
		}
	}
}

// TestRetiringSurfaceDoesNotGrowUnmarked is the half that makes the register
// safe to have. A file allowed to carry markers would otherwise be a place
// where new unmarked exports could accumulate, which is how a retirement
// quietly turns back into a permanent feature.
func TestRetiringSurfaceDoesNotGrowUnmarked(t *testing.T) {
	root := repositoryRoot(t)
	seen := map[string]bool{}
	for _, owner := range []string{"domain", "application"} {
		err := walkProductionGo(filepath.Join(root, owner), func(path string, parsed *ast.File, fset *token.FileSet) {
			relative, _ := filepath.Rel(root, path)
			key := filepath.ToSlash(relative)
			entry, retiring := retiringFiles[key]
			if !retiring {
				return
			}
			seen[key] = true
			if !entry.wholeFile {
				// The registered symbols must still exist and still carry their
				// markers. A field renamed away leaves an entry that permits a
				// name nothing declares, which silently widens nothing today and
				// blocks nothing tomorrow.
				marked := map[string]bool{}
				for _, symbol := range markedSymbolsIn(parsed) {
					marked[symbol.name] = true
				}
				for _, symbol := range entry.symbols {
					if !marked[symbol] {
						t.Errorf("%s: the retirement register lists %s, which no longer carries a Deprecated marker; %s", key, symbol, entry.reason)
					}
				}
				return
			}
			for _, declaration := range parsed.Decls {
				for _, exported := range exportedNamesWithDoc(declaration) {
					if strings.Contains(exported.doc, "Deprecated:") {
						continue
					}
					t.Errorf("%s:%d exports %s from a retiring file without a Deprecated marker; %s, so nothing here may grow unmarked",
						key, fset.Position(exported.pos).Line, exported.name, entry.reason)
				}
				// Struct fields are checked too, so the invariant above is
				// literally true rather than true of top-level names only. A
				// field inherits its type's marker, which is the common case
				// here and produces no noise; what this catches is a field
				// outliving a type whose marker was later removed.
				for _, field := range exportedFieldsWithTypeDoc(declaration) {
					if strings.Contains(field.doc, "Deprecated:") {
						continue
					}
					t.Errorf("%s:%d exports field %s from a retiring file, and neither it nor its type carries a Deprecated marker; %s",
						key, fset.Position(field.pos).Line, field.name, entry.reason)
				}
			}
		})
		if err != nil {
			t.Fatalf("walk %s: %v", owner, err)
		}
	}
	for key := range retiringFiles {
		if !seen[key] {
			t.Errorf("retirement register lists %s, which no longer exists; remove the entry once the removal has landed", key)
		}
	}
}

// TestRetirementRegisterMatchesItsPlan keeps the register and the published
// plan from drifting. The register decides what the compiler tolerates; the
// plan is what a host reads to learn what it must build before the removal
// lands. A file in one and not the other means somebody is working from a
// document that no longer describes the code.
func TestRetirementRegisterMatchesItsPlan(t *testing.T) {
	root := repositoryRoot(t)
	planPath := filepath.Join(root, "docs", "contracts", "retirement-plan.md")
	lines, err := readLines(planPath)
	if err != nil {
		t.Fatalf("read retirement plan: %v", err)
	}
	plan := strings.Join(lines, "\n")
	for key := range retiringFiles {
		if !strings.Contains(plan, key) {
			t.Errorf("%s is in the retirement register but not in docs/contracts/retirement-plan.md; a host cannot prepare for a removal it is never told about", key)
		}
	}
}

type exportedName struct {
	name string
	doc  string
	pos  token.Pos
}

// markedSymbolsIn returns every exported declaration or struct field in the
// file whose doc comment carries a Deprecated marker, named Type.Field for
// fields. Fields are walked as well as top-level declarations because a partly
// retiring file marks fields, and a check that only saw declarations would
// report every one of them as a stray.
func markedSymbolsIn(parsed *ast.File) []exportedName {
	var marked []exportedName
	for _, declaration := range parsed.Decls {
		for _, exported := range exportedNamesWithDoc(declaration) {
			if strings.Contains(exported.doc, "Deprecated:") {
				marked = append(marked, exported)
			}
		}
		generic, ok := declaration.(*ast.GenDecl)
		if !ok || generic.Tok != token.TYPE {
			continue
		}
		for _, spec := range generic.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok || structType.Fields == nil {
				continue
			}
			for _, field := range structType.Fields.List {
				doc := field.Doc.Text() + field.Comment.Text()
				if !strings.Contains(doc, "Deprecated:") {
					continue
				}
				for _, name := range field.Names {
					if name.IsExported() {
						marked = append(marked, exportedName{typeSpec.Name.Name + "." + name.Name, doc, name.Pos()})
					}
				}
			}
		}
	}
	return marked
}

// exportedFieldsWithTypeDoc returns every exported struct field in a
// declaration, named Type.Field, with the doc that would carry its marker —
// the field's own, plus its type's, because a field on a deprecated type is
// deprecated with it and repeating the marker on each one would be noise.
func exportedFieldsWithTypeDoc(declaration ast.Decl) []exportedName {
	generic, ok := declaration.(*ast.GenDecl)
	if !ok || generic.Tok != token.TYPE {
		return nil
	}
	var found []exportedName
	for _, spec := range generic.Specs {
		typeSpec, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok || structType.Fields == nil {
			continue
		}
		inherited := generic.Doc.Text() + typeSpec.Doc.Text() + typeSpec.Comment.Text()
		for _, field := range structType.Fields.List {
			doc := inherited + field.Doc.Text() + field.Comment.Text()
			for _, name := range field.Names {
				if name.IsExported() {
					found = append(found, exportedName{typeSpec.Name.Name + "." + name.Name, doc, name.Pos()})
				}
			}
		}
	}
	return found
}

// countDeprecationMarkers counts marker comment groups in the file, so a marker
// attached to nothing can be told apart from one the walk above accounted for.
func countDeprecationMarkers(parsed *ast.File) int {
	count := 0
	for _, group := range parsed.Comments {
		if strings.Contains(group.Text(), "Deprecated:") {
			count++
		}
	}
	return count
}

// exportedNamesWithDoc pairs each exported top-level name in a declaration
// with the doc comment that would carry its marker. A grouped const or var
// block puts the marker on either the group or the individual spec, and godoc
// honours both, so both are searched.
func exportedNamesWithDoc(declaration ast.Decl) []exportedName {
	var found []exportedName
	switch typed := declaration.(type) {
	case *ast.FuncDecl:
		if typed.Name.IsExported() {
			found = append(found, exportedName{typed.Name.Name, typed.Doc.Text(), typed.Name.Pos()})
		}
	case *ast.GenDecl:
		groupDoc := typed.Doc.Text()
		for _, spec := range typed.Specs {
			switch value := spec.(type) {
			case *ast.TypeSpec:
				if value.Name.IsExported() {
					found = append(found, exportedName{value.Name.Name, groupDoc + value.Doc.Text() + value.Comment.Text(), value.Name.Pos()})
				}
			case *ast.ValueSpec:
				for _, name := range value.Names {
					if name.IsExported() {
						found = append(found, exportedName{name.Name, groupDoc + value.Doc.Text() + value.Comment.Text(), name.Pos()})
					}
				}
			}
		}
	}
	return found
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
						if name.Name == "EntryID" {
							t.Errorf("%s.EntryID at %s:%d revives the pre-adoption spelling of an entry identity; the entry identity is Entry.ID typed EntryID",
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
		"InstanceID":      "InstanceID",
		"EntryID":         "EntryID",
		"NextEntryID":     "EntryID",
		"StepExecutionID": "StepExecutionID",
		"Path":            "InvocationPath",
		"ParentPath":      "InvocationPath",
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

// A fingerprint is the payload every heal decision scores against, and it holds
// a map and two slices. It had four hand-written copies, one per package that
// needed one; two of them were missing a field, and one of those two shipped a
// draft edit that silently rewrote its own source.
//
// The class, not the instances, is what has to stay dead. Any function that
// takes a fingerprint and returns one, and is named for copying, is a second
// implementation of Fingerprint.Clone waiting to drift away from it. A copy
// assembled inline as a composite literal is the same thing under no name at
// all, so this also rejects constructing a fingerprint field by field outside
// the package that owns the type.
func TestFingerprintHasExactlyOneDeepCopy(t *testing.T) {
	root := repositoryRoot(t)

	// Pointer forms count. A review probe landed in the tree as
	// func cloneFingerprint(*fingerprint.Fingerprint) *fingerprint.Fingerprint and
	// walked straight past this guard, because a *T is an ast.StarExpr wrapping
	// the name rather than the name itself.
	var isFingerprint func(ast.Expr) bool
	isFingerprint = func(expr ast.Expr) bool {
		switch typed := expr.(type) {
		case *ast.StarExpr:
			return isFingerprint(typed.X)
		case *ast.Ident:
			return typed.Name == "Fingerprint"
		case *ast.SelectorExpr:
			return typed.Sel.Name == "Fingerprint"
		}
		return false
	}

	var copies, literals, owned []string
	for _, owner := range []string{"domain", "application"} {
		err := walkProductionGo(filepath.Join(root, owner), func(path string, parsed *ast.File, fset *token.FileSet) {
			relative := filepath.ToSlash(mustRelative(root, path))
			// domain/fingerprint owns the type, so its own Clone is the one copy.
			owns := strings.HasPrefix(relative, "domain/fingerprint/")
			ast.Inspect(parsed, func(node ast.Node) bool {
				switch typed := node.(type) {
				case *ast.FuncDecl:
					if typed.Type.Params == nil && typed.Type.Results == nil {
						return true
					}
					if owns {
						// Count the owner's own copy rather than skipping it. The test
						// is named for there being exactly one, and only counting the
						// outside would let the single implementation be deleted, or a
						// second one added beside it, without the guard noticing.
						if typed.Name.Name == "Clone" && typed.Recv != nil {
							for _, receiver := range typed.Recv.List {
								if isFingerprint(receiver.Type) {
									owned = append(owned, relative+":"+strconv.Itoa(fset.Position(typed.Pos()).Line))
								}
							}
						}
						return true
					}
					if typed.Type.Params == nil || typed.Type.Results == nil {
						return true
					}
					name := strings.ToLower(typed.Name.Name)
					if !strings.Contains(name, "clone") && !strings.Contains(name, "copy") {
						return true
					}
					takes, returns := false, false
					for _, param := range typed.Type.Params.List {
						if isFingerprint(param.Type) {
							takes = true
						}
					}
					for _, result := range typed.Type.Results.List {
						if isFingerprint(result.Type) {
							returns = true
						}
					}
					if takes && returns {
						copies = append(copies, relative+":"+strconv.Itoa(fset.Position(typed.Pos()).Line)+" "+typed.Name.Name)
					}
				case *ast.CompositeLit:
					// A composite literal with named fields is someone rebuilding a
					// fingerprint by hand; Clone is the only thing allowed to do that.
					if owns || !isFingerprint(typed.Type) || len(typed.Elts) == 0 {
						return true
					}
					named := 0
					for _, element := range typed.Elts {
						if _, ok := element.(*ast.KeyValueExpr); ok {
							named++
						}
					}
					// Two or more named fields means the whole value is being assembled,
					// as opposed to a small fixture stating the one field under test.
					if named >= 2 {
						literals = append(literals, relative+":"+strconv.Itoa(fset.Position(typed.Pos()).Line))
					}
				}
				return true
			})
		})
		if err != nil {
			t.Fatalf("walk %s: %v", owner, err)
		}
	}
	if len(owned) != 1 {
		t.Errorf("domain/fingerprint declares %d Fingerprint.Clone methods, want exactly 1:\n  %s",
			len(owned), strings.Join(owned, "\n  "))
	}
	if len(copies) != 0 {
		t.Errorf("found %d hand-written fingerprint copies outside domain/fingerprint; Fingerprint.Clone is the only one:\n  %s",
			len(copies), strings.Join(copies, "\n  "))
	}
	if len(literals) != 0 {
		t.Errorf("found %d places assembling a fingerprint field by field outside domain/fingerprint; a field added to the type would be dropped by each of them:\n  %s",
			len(literals), strings.Join(literals, "\n  "))
	}
}

func mustRelative(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return relative
}
