package architecture_test

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

func TestEntryStatusMachineHasProductionCallers(t *testing.T) {
	root := repositoryRoot(t)
	calls := 0
	err := walkProductionGo(filepath.Join(root, "application", "scheduling"), func(path string, parsed *ast.File, fset *token.FileSet) {
		rel, _ := filepath.Rel(root, path)
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			name := selector.Sel.Name
			if name == "ValidateEntryStatusTransition" || name == "CanTransitionTo" {
				calls++
				t.Logf("found call to %s at %s:%d", name, filepath.ToSlash(rel), fset.Position(call.Pos()).Line)
			}
			return true
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls == 0 {
		t.Error("ValidateEntryStatusTransition or CanTransitionTo has zero production callers")
	}
}

func TestExecutionTransitionLiteralsUseValidStatusTransitions(t *testing.T) {
	root := repositoryRoot(t)
	allowed := map[[2]string]bool{
		{"EntryPending", "EntryRunning"}:   true,
		{"EntryPending", "EntrySkipped"}:   true,
		{"EntryPending", "EntryFailed"}:    true,
		{"EntryPending", "EntryCanceled"}:  true,
		{"EntryRunning", "EntrySucceeded"}: true,
		{"EntryRunning", "EntryFailed"}:    true,
		{"EntryRunning", "EntryCanceled"}:  true,
		{"EntryRunning", "EntryAborted"}:   true,
	}
	literals := 0
	err := walkAllGo(filepath.Join(root, "application", "scheduling"), func(path string, parsed *ast.File, fset *token.FileSet) {
		rel, _ := filepath.Rel(root, path)
		ast.Inspect(parsed, func(node ast.Node) bool {
			composite, ok := node.(*ast.CompositeLit)
			if !ok {
				return true
			}
			// Both spellings, and the count assertion below. Matching only
			// the qualified form made this guard vacuous: the sole package
			// that constructs these literals is the one being walked, so it
			// writes them unqualified, and the walk found nothing at all. A
			// guard whose subject set is empty passes for the wrong reason.
			switch typed := composite.Type.(type) {
			case *ast.SelectorExpr:
				if typed.Sel.Name != "ExecutionTransition" {
					return true
				}
			case *ast.Ident:
				if typed.Name != "ExecutionTransition" {
					return true
				}
			default:
				return true
			}
			literals++
			var from, to string
			for _, elt := range composite.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := kv.Key.(*ast.Ident)
				if !ok {
					continue
				}
				sel, ok := kv.Value.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				ident, ok := sel.X.(*ast.Ident)
				if !ok {
					continue
				}
				value := ident.Name + "." + sel.Sel.Name
				switch key.Name {
				case "From":
					from = value
				case "To":
					to = value
				}
			}
			if from == "" || to == "" {
				t.Errorf("%s:%d: ExecutionTransition literal missing From or To", filepath.ToSlash(rel), fset.Position(composite.Pos()).Line)
				return true
			}
			fromIdent := from[strings.IndexByte(from, '.')+1:]
			toIdent := to[strings.IndexByte(to, '.')+1:]
			if !allowed[[2]string{fromIdent, toIdent}] {
				t.Errorf("%s:%d: invalid transition %s -> %s", filepath.ToSlash(rel), fset.Position(composite.Pos()).Line, from, to)
			}
			return true
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	// Coverage gate. Without it the guard reports success when it has walked
	// past every literal it exists to check, which is how it passed while
	// matching only the qualified spelling.
	if literals == 0 {
		t.Error("no ExecutionTransition literal was found in application/scheduling; the guard is checking an empty set and cannot fail for the right reason")
	}
}
