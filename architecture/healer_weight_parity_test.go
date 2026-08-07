package architecture_test

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The heal scorer's weight vocabulary is restated in five places that no
// compiler relates to each other: two snapshot structs, a negative-zero
// normaliser, a digest encoder, and three validation lists. Each is a
// hand-maintained enumeration, so a new dimension is carried by whichever
// ones its author remembered.
//
// Framework was the demonstration. It was added to heal.Weights and read by
// the scorer, but no snapshot carried it, nothing digested it, and no
// validation rejected it -- so the one dimension outside the run digest was
// also the only one a host could vary between two runs that sealed to the
// same digest. Reproducibility is exactly what sealing the policy buys, and
// it was silently partial.
//
// canonicalDimensions reads the vocabulary from heal.Weights, which is the
// type the scorer actually consumes. Every other site is measured against it.
func canonicalDimensions(t *testing.T, root string) []string {
	t.Helper()
	fields, found := structFields(t, filepath.Join(root, "domain", "heal"), "Weights")
	if !found {
		t.Fatal("heal.Weights was renamed away; this guard measures every other site against it")
	}
	sort.Strings(fields)
	return fields
}

// TestEveryHealerWeightSnapshotCarriesEveryScoredDimension pins the two frozen
// representations to the scorer's vocabulary. automation keeps its own copy on
// purpose -- it is an anti-corruption layer and must not import domain/heal --
// which is precisely why nothing but a guard can hold the two in step.
func TestEveryHealerWeightSnapshotCarriesEveryScoredDimension(t *testing.T) {
	root := repositoryRoot(t)
	want := canonicalDimensions(t, root)
	for _, mirror := range []struct{ dir, typeName string }{
		{filepath.Join("domain", "execution"), "HealerWeightsSnapshot"},
		{filepath.Join("domain", "automation"), "HealerWeightsSnapshotV1"},
	} {
		fields, found := structFields(t, filepath.Join(root, mirror.dir), mirror.typeName)
		if !found {
			t.Errorf("%s is missing from %s", mirror.typeName, filepath.ToSlash(mirror.dir))
			continue
		}
		sort.Strings(fields)
		if missing := difference(want, fields); len(missing) != 0 {
			t.Errorf("%s does not carry scored dimension(s) %v; a weight the scorer reads but the snapshot omits can only be set outside the run digest",
				mirror.typeName, missing)
		}
		if extra := difference(fields, want); len(extra) != 0 {
			t.Errorf("%s carries %v, which heal.Weights does not score; a frozen, digested weight nothing reads is dead policy surface",
				mirror.typeName, extra)
		}
	}
}

// TestEveryHealerWeightEnumerationIsComplete is the half the struct check
// cannot see. Adding the field is not enough: the digest encoder, the
// negative-zero normaliser and each validation list enumerate the dimensions
// by hand, and a site that omits one keeps compiling.
//
// The rule is all-or-nothing per top-level declaration, not per file. Three
// independent enumerations live in instance_snapshot.go alone -- the default
// literal, the negative-zero normaliser and the digest encoder -- so a
// file-wide check passes as long as any one of them is complete, which is
// exactly the omission being hunted.
//
// A declaration with a legitimate reason to touch a subset does not exist
// today, and if one appears the failure is the right place to argue for it.
func TestEveryHealerWeightEnumerationIsComplete(t *testing.T) {
	root := repositoryRoot(t)
	want := canonicalDimensions(t, root)
	known := make(map[string]bool, len(want))
	for _, name := range want {
		known[name] = true
	}

	for _, owner := range []string{"domain", "application"} {
		err := walkProductionGo(filepath.Join(root, owner), func(path string, parsed *ast.File, fset *token.FileSet) {
			relative, _ := filepath.Rel(root, path)
			for _, declaration := range parsed.Decls {
				// The struct definitions themselves are the previous guard's
				// job; counting their field names here would mask an
				// incomplete enumeration sharing the declaration.
				if isWeightsTypeDeclaration(declaration) {
					continue
				}
				mentioned := weightDimensionsIn(declaration, known)
				if len(mentioned) == 0 {
					continue
				}
				if missing := difference(want, mentioned); len(missing) != 0 {
					t.Errorf("%s:%d enumerates healer weights but omits %v; every digest, normalisation and validation list must cover the whole vocabulary or the omitted dimension escapes it",
						filepath.ToSlash(relative), fset.Position(declaration.Pos()).Line, missing)
				}
			}
		})
		if err != nil {
			t.Fatalf("walk %s: %v", owner, err)
		}
	}
}

// weightDimensionsIn returns the sorted dimension names one declaration names
// through a Weights value or a Weights composite literal.
func weightDimensionsIn(declaration ast.Decl, known map[string]bool) []string {
	mentioned := map[string]bool{}
	ast.Inspect(declaration, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.SelectorExpr:
			// `policy.Weights.Tag`, `v.HealerPolicy.Weights.Tag`, and the
			// scorer's own `weights.Tag` / `s.weights.Tag`.
			if known[value.Sel.Name] && namesAWeightsValue(value.X) {
				mentioned[value.Sel.Name] = true
			}
		case *ast.CompositeLit:
			if !namesAWeightsType(value.Type) {
				return true
			}
			for _, element := range value.Elts {
				pair, ok := element.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if key, ok := pair.Key.(*ast.Ident); ok && known[key.Name] {
					mentioned[key.Name] = true
				}
			}
		}
		return true
	})
	present := make([]string, 0, len(mentioned))
	for name := range mentioned {
		present = append(present, name)
	}
	sort.Strings(present)
	return present
}

func isWeightsTypeDeclaration(declaration ast.Decl) bool {
	generic, ok := declaration.(*ast.GenDecl)
	if !ok || generic.Tok != token.TYPE {
		return false
	}
	for _, spec := range generic.Specs {
		typeSpec, ok := spec.(*ast.TypeSpec)
		if ok && strings.Contains(typeSpec.Name.Name, "Weights") {
			return true
		}
	}
	return false
}

// namesAWeightsValue reports whether an expression is a Weights-typed value,
// by the only signal available without type information: it is called
// weights, or it is a field called Weights.
func namesAWeightsValue(expression ast.Expr) bool {
	switch value := expression.(type) {
	case *ast.Ident:
		return strings.EqualFold(value.Name, "weights")
	case *ast.SelectorExpr:
		return strings.EqualFold(value.Sel.Name, "weights")
	}
	return false
}

func namesAWeightsType(expression ast.Expr) bool {
	switch value := expression.(type) {
	case *ast.Ident:
		return strings.Contains(value.Name, "Weights")
	case *ast.SelectorExpr:
		return strings.Contains(value.Sel.Name, "Weights")
	}
	return false
}

// difference returns the members of want that are absent from got. Both are
// sorted, so the report is a function of the vocabulary rather than of walk
// order.
func difference(want, got []string) []string {
	present := make(map[string]bool, len(got))
	for _, name := range got {
		present[name] = true
	}
	var missing []string
	for _, name := range want {
		if !present[name] {
			missing = append(missing, name)
		}
	}
	return missing
}
