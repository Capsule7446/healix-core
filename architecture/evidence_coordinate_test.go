package architecture_test

import (
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func w2TestOccurrenceFields(t *testing.T) {
	t.Helper()
	root := repositoryRoot(t)
	evidenceDir := root + "/domain/evidence"
	violations := 0
	err := walkAllGo(evidenceDir, func(path string, file *ast.File, _ *token.FileSet) {
		if violations > 10 {
			return
		}
		ast.Inspect(file, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return true
			}
			if !ts.Name.IsExported() {
				return true
			}
			hasEntryID := false
			hasOccurrence := false
			for _, field := range st.Fields.List {
				if len(field.Names) == 0 {
					continue
				}
				name := field.Names[0].Name
				if name == "EntryID" {
					hasEntryID = true
				}
				if name == "Occurrence" {
					hasOccurrence = true
				}
			}
			if hasEntryID && !hasOccurrence {
				t.Errorf("evidence struct %s has EntryID but no Occurrence field", ts.Name.Name)
				violations++
			}
			return true
		})
	})
	if err != nil {
		t.Fatalf("walkAllGo: %v", err)
	}
}

// coordinateComponentIsDelivered learns which struct types carry the named
// component of the evidence coordinate, then requires every literal of those
// types in production code to fill it.
//
// This replaces three subtests of the form `_ = T{Component: fixture}`. That
// form is not inert -- deleting the field stops it compiling -- but a field can
// be present, exported, typed and written by nothing that ever runs, which is
// the state InvocationPath was actually in while its guard was green. A shape
// assertion cannot distinguish "declared" from "delivered". Reading every
// construction site can, and it keeps working for carriers added later.
//
// Types are keyed by package, not by bare name. Three unrelated types are
// spelled ValidationObservation or HealObservation in this repository, and
// matching on the name alone reports the ones without the field as violations.
func coordinateComponentIsDelivered(t *testing.T, component string) {
	t.Helper()
	root := repositoryRoot(t)
	module := modulePath(t, root)
	carriers := map[string]bool{}
	if err := walkAllGo(root, func(path string, file *ast.File, _ *token.FileSet) {
		ast.Inspect(file, func(n ast.Node) bool {
			spec, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			structType, ok := spec.Type.(*ast.StructType)
			if !ok {
				return true
			}
			for _, field := range structType.Fields.List {
				for _, name := range field.Names {
					if name.Name == component {
						carriers[packageDir(t, root, path)+"."+spec.Name.Name] = true
					}
				}
			}
			return true
		})
	}); err != nil {
		t.Fatalf("walkAllGo: %v", err)
	}
	if len(carriers) == 0 {
		t.Fatalf("no struct declares %s: the coordinate component vanished", component)
	}
	if err := walkGo(root, false, func(path string, file *ast.File, fset *token.FileSet) {
		imports := importedPackageDirs(file, module)
		here := packageDir(t, root, path)
		forEachCompositeLit(file, func(lit *ast.CompositeLit, typeExpr ast.Expr) {
			// An empty literal is a zero value handed back beside an error, not a
			// claim that a coordinate was observed. Positional literals already owe
			// the compiler every field.
			if len(lit.Elts) == 0 || !carriers[literalTypeKey(typeExpr, here, imports)] {
				return
			}
			for _, element := range lit.Elts {
				pair, ok := element.(*ast.KeyValueExpr)
				if !ok {
					return
				}
				if key, ok := pair.Key.(*ast.Ident); ok && key.Name == component {
					return
				}
			}
			t.Errorf("%s:%d: literal omits %s: the coordinate component is declared but not delivered",
				path, fset.Position(lit.Pos()).Line, component)
		})
	}); err != nil {
		t.Fatalf("walkGo: %v", err)
	}
}

// forEachCompositeLit reports every composite literal in the file along with the
// expression naming the type being constructed. Literals nested inside a slice
// or map literal elide their own type, so the enclosing literal's element type
// supplies it; without that, carriers built as `[]T{{...}}` would go unread.
func forEachCompositeLit(file *ast.File, visit func(*ast.CompositeLit, ast.Expr)) {
	var enclosing []*ast.CompositeLit
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			enclosing = enclosing[:len(enclosing)-1]
			return false
		}
		lit, isLiteral := n.(*ast.CompositeLit)
		if !isLiteral {
			enclosing = append(enclosing, nil)
			return true
		}
		typeExpr := lit.Type
		if typeExpr == nil {
			for index := len(enclosing) - 1; index >= 0; index-- {
				if enclosing[index] == nil {
					continue
				}
				typeExpr = literalElementType(enclosing[index].Type)
				break
			}
		}
		visit(lit, typeExpr)
		enclosing = append(enclosing, lit)
		return true
	})
}

// literalTypeKey resolves a type expression to "<package dir>.<type name>",
// which is what makes two same-named types in different packages distinct.
func literalTypeKey(expr ast.Expr, here string, imports map[string]string) string {
	switch typed := expr.(type) {
	case *ast.Ident:
		return here + "." + typed.Name
	case *ast.StarExpr:
		return literalTypeKey(typed.X, here, imports)
	case *ast.SelectorExpr:
		qualifier, ok := typed.X.(*ast.Ident)
		if !ok {
			return ""
		}
		dir, imported := imports[qualifier.Name]
		if !imported {
			return ""
		}
		return dir + "." + typed.Sel.Name
	default:
		return ""
	}
}

func literalElementType(expr ast.Expr) ast.Expr {
	switch typed := expr.(type) {
	case *ast.ArrayType:
		return typed.Elt
	case *ast.MapType:
		return typed.Value
	default:
		return nil
	}
}

// importedPackageDirs maps each local qualifier in the file to the repository
// directory it names. Imports from outside the module resolve to a path that no
// carrier key can match, which is the intended outcome.
func importedPackageDirs(file *ast.File, module string) map[string]string {
	dirs := make(map[string]string, len(file.Imports))
	for _, spec := range file.Imports {
		path := strings.Trim(spec.Path.Value, `"`)
		local := path[strings.LastIndex(path, "/")+1:]
		if spec.Name != nil {
			local = spec.Name.Name
		}
		dirs[local] = strings.TrimPrefix(strings.TrimPrefix(path, module), "/")
	}
	return dirs
}

func packageDir(t *testing.T, root, path string) string {
	t.Helper()
	relative, err := filepath.Rel(root, filepath.Dir(path))
	if err != nil {
		t.Fatalf("relative path for %s: %v", path, err)
	}
	return filepath.ToSlash(relative)
}

func modulePath(t *testing.T, root string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	for _, line := range strings.Split(string(content), "\n") {
		if declaration, found := strings.CutPrefix(strings.TrimSpace(line), "module "); found {
			return strings.TrimSpace(declaration)
		}
	}
	t.Fatal("go.mod declares no module path")
	return ""
}

func w2TestInvocationPathIsDelivered(t *testing.T) {
	t.Helper()
	coordinateComponentIsDelivered(t, "InvocationPath")
}

func w2TestOccurrenceIsDelivered(t *testing.T) {
	t.Helper()
	coordinateComponentIsDelivered(t, "Occurrence")
}

func TestEvidenceCoordinateW2(t *testing.T) {
	t.Run("evidence_occurrence_fields", w2TestOccurrenceFields)
	t.Run("invocation_path_is_delivered", w2TestInvocationPathIsDelivered)
	t.Run("occurrence_is_delivered", w2TestOccurrenceIsDelivered)
}
