package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// Guards for the guards.
//
// Every check in this package decides something about source it walks, and
// until now the only evidence that a decision was right came from mutating a
// real file, running, and reverting. That proves the shapes its author thought
// of are caught. It proves nothing about the rest — and the rest is where the
// defects were: six review rounds on one pull request found eight faults in
// these guards and none in the code they protect, every one of them after a
// hand-run mutation had already "confirmed" the guard worked.
//
// The failure mode is specific. A guard that under-reports is invisible: the
// suite is green, the invariant is not held, and nothing distinguishes that
// from an invariant that is held. A guard that over-reports is loud but worse
// in a different way, because the cheapest fix is to weaken it.
//
// So each predicate gets a fixed decision matrix here: synthetic sources
// covering both answers, including every shape a past review found missing.
// The matrices are the regression suite for the guards themselves, and a new
// row costs one table entry rather than a mutate-run-revert cycle.

// parseGuardFixture turns a source snippet into a file the predicates can read.
// Fixtures need not type-check or even be buildable — every predicate under
// test is syntactic, and demanding compilable fixtures would rule out exactly
// the malformed shapes worth covering.
func parseGuardFixture(t *testing.T, source string) *ast.File {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), "fixture.go", "package fixture\n\n"+source, parser.ParseComments)
	if err != nil {
		t.Fatalf("fixture does not parse: %v\n%s", err, source)
	}
	return parsed
}

// sameStrings compares two lists treating nil and empty as the same answer.
// The predicates differ in which they return for "nothing found", and a matrix
// that cared about the difference would be asserting an implementation detail
// rather than a decision.
func sameStrings(got, want []string) bool {
	if len(got) == 0 && len(want) == 0 {
		return true
	}
	return reflect.DeepEqual(got, want)
}

func firstDecl(t *testing.T, source string) ast.Decl {
	t.Helper()
	parsed := parseGuardFixture(t, source)
	if len(parsed.Decls) == 0 {
		t.Fatalf("fixture declares nothing:\n%s", source)
	}
	return parsed.Decls[0]
}

// TestConstAliasPredicateMatrix covers the guard that had the
// inherited-expression hole. The last two rows are that hole.
func TestConstAliasPredicateMatrix(t *testing.T) {
	permitted := map[string]string{"Permitted": "allowed by the register"}
	for _, test := range []struct {
		name   string
		source string
		want   []string // "alias=target"
	}{
		{"explicit alias", "const (\n\tNew Status = \"x\"\n\tOld = New\n)", []string{"Old=New"}},
		{"inherited expression", "const (\n\tOne = Existing\n\tTwo\n)", []string{"One=Existing", "Two=Existing"}},
		{"inherited after literal", "const (\n\tA Status = \"x\"\n\tB\n)", nil},
		{"literal value", "const (\n\tA Status = \"x\"\n)", nil},
		{"iota member", "const (\n\tA = iota\n\tB\n)", nil},
		{"unexported alias name", "const (\n\tNew = 1\n\told = New\n)", nil},
		{"unexported target", "const (\n\tOld = existing\n)", nil},
		{"computed expression", "const (\n\tNew = 1\n\tOld = New + 1\n)", nil},
		{"permitted by register", "const (\n\tPermitted = Existing\n)", nil},
		{"var block is not const", "var (\n\tOld = Existing\n)", nil},
		{"type declaration", "type Old = Existing", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			var got []string
			for _, alias := range constAliasesIn(firstDecl(t, test.source), permitted) {
				got = append(got, alias.name+"="+alias.target)
			}
			if !sameStrings(got, test.want) {
				t.Fatalf("aliases = %v, want %v", got, test.want)
			}
		})
	}
}

// TestForwardedReceiverMethodMatrix covers the alias guard and the
// specialisation false positive a review found. The "supplies its own" rows are
// the ones that must NOT match: a guard that rejects legitimate code is fixed
// by weakening it, which is how invariants die.
func TestForwardedReceiverMethodMatrix(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
		want   string // "" means not an alias
	}{
		{"bare forward", "func (s *S) Old() { s.New() }", "New"},
		{"returned forward", "func (s *S) Old() error { return s.New() }", "New"},
		{"forwards one parameter", "func (s *S) Old(a int) error { return s.New(a) }", "New"},
		{"forwards several parameters", "func (s *S) Old(a, b int) error { return s.New(a, b) }", "New"},
		{"forwards grouped parameters", "func (s *S) Old(a int, b string) error { return s.New(a, b) }", "New"},

		{"supplies its own argument", "func (s *S) Old(a int) error { return s.New(DefaultOptions) }", ""},
		{"supplies a literal", "func (s *S) Old() error { return s.New(0) }", ""},
		{"reorders parameters", "func (s *S) Old(a, b int) error { return s.New(b, a) }", ""},
		{"drops a parameter", "func (s *S) Old(a int) error { return s.New() }", ""},
		{"unnamed parameter", "func (s *S) Old(int) error { return s.New(0) }", ""},
		{"wraps the call", "func (s *S) Old() error { return wrap(s.New()) }", ""},

		{"unexported target", "func (s *S) Old() error { return s.new() }", ""},
		{"not the receiver", "func (s *S) Old() error { return other.New() }", ""},
		{"through a field", "func (s *S) Old() error { return s.inner.New() }", ""},
		{"package function", "func (s *S) Old() error { return New() }", ""},
		{"unnamed receiver", "func (*S) Old() error { return New() }", ""},
		{"two statements", "func (s *S) Old() error {\n\tx := s.New()\n\treturn x\n}", ""},
		{"returns two results", "func (s *S) Old() (int, error) { return s.New() }", "New"},
		{"empty body", "func (s *S) Old() {}", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			function, ok := firstDecl(t, test.source).(*ast.FuncDecl)
			if !ok {
				t.Fatalf("fixture is not a function:\n%s", test.source)
			}
			// The caller guards these two before asking, so the matrix does too.
			if function.Recv == nil || function.Body == nil || len(function.Body.List) != 1 {
				if test.want != "" {
					t.Fatalf("fixture cannot reach the predicate but expects %q", test.want)
				}
				return
			}
			target, isAlias := forwardedReceiverMethod(function)
			if !isAlias {
				target = ""
			}
			if target != test.want {
				t.Fatalf("forwardedReceiverMethod = %q, want %q", target, test.want)
			}
		})
	}
}

// TestUnattachedDeprecationMarkerMatrix covers the stray-marker check. The
// grouped rows are the arithmetic hole: one marker on a grouped declaration
// produced several marked symbols, and subtracting counts let a genuine stray
// cancel out. The import row is the second hole from the same guard.
func TestUnattachedDeprecationMarkerMatrix(t *testing.T) {
	const marker = "// Deprecated: leaving core\n"
	for _, test := range []struct {
		name   string
		source string
		want   int
	}{
		{"on a function", marker + "func Exported() {}", 0},
		{"on a type", marker + "type Exported struct{}", 0},
		{"on a single const", "const (\n\t" + marker + "\tExported = 1\n)", 0},
		{"on a grouped const", marker + "const (\n\tOne = 1\n\tTwo = 2\n)", 0},
		{"on a struct field", "type T struct {\n\t" + marker + "\tField string\n}", 0},
		{"trailing on a field", "type T struct {\n\tField string // Deprecated: leaving core\n}", 0},

		// A blank line detaches a comment from the declaration below it, so this
		// marker documents nothing even though it reads as though it does. The
		// pair is kept because the difference is one newline and invisible in
		// review.
		{"blank line detaches it from the next declaration", "func A() {}\n\n" + marker + "\nfunc B() {}", 1},
		{"no blank line keeps it attached", "func A() {}\n\n" + marker + "func B() {}", 0},
		{"floating at end of file", "func A() {}\n\n" + marker, 1},
		{"inside a function body", "func A() {\n\t" + marker + "\t_ = 1\n}", 1},
		{"on an import", "import (\n\t" + marker + "\t\"strings\"\n)", 1},

		// The hole itself: one grouped marker inflates the symbol count, and the
		// old count-subtraction let this stray disappear into the surplus.
		{"grouped marker plus a stray", marker + "const (\n\tOne = 1\n\tTwo = 2\n)\n\nfunc A() {}\n\n" + marker, 1},
		{"no markers at all", "func Exported() {}", 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := len(unattachedDeprecationMarkers(parseGuardFixture(t, test.source)))
			if got != test.want {
				t.Fatalf("unattached markers = %d, want %d", got, test.want)
			}
		})
	}
}

// TestMarkedSymbolsMatrix pins which names a marker is taken to cover. The
// grouped row is what makes the count-subtraction above unsound, so it is
// asserted rather than left implied.
func TestMarkedSymbolsMatrix(t *testing.T) {
	const marker = "// Deprecated: leaving core\n"
	for _, test := range []struct {
		name   string
		source string
		want   []string
	}{
		{"grouped const covers every name", marker + "const (\n\tOne = 1\n\tTwo = 2\n)", []string{"One", "Two"}},
		{"individual const", "const (\n\t" + marker + "\tOne = 1\n\tTwo = 2\n)", []string{"One"}},
		{"function", marker + "func Exported() {}", []string{"Exported"}},
		{"method is named bare", marker + "func (s *S) Exported() {}", []string{"Exported"}},
		{"struct field is qualified", "type T struct {\n\t" + marker + "\tField string\n}", []string{"T.Field"}},
		{"field does not inherit its type marker", marker + "type T struct {\n\tField string\n}", []string{"T"}},
		{"unexported is ignored", marker + "func unexported() {}", nil},
		{"unmarked is ignored", "func Exported() {}", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			var got []string
			for _, symbol := range markedSymbolsIn(parseGuardFixture(t, test.source)) {
				got = append(got, symbol.name)
			}
			sort.Strings(got)
			if !sameStrings(got, test.want) {
				t.Fatalf("marked symbols = %v, want %v", got, test.want)
			}
		})
	}
}

// TestExportedFieldsInheritTypeMarker is the opposite convention to the one
// above, and the difference is deliberate: the growth check wants a field on a
// deprecated type to count as covered, while the stray check must not let a
// type marker absorb names the register never listed.
func TestExportedFieldsInheritTypeMarker(t *testing.T) {
	const marker = "// Deprecated: leaving core\n"
	for _, test := range []struct {
		name       string
		source     string
		wantFields []string
		wantMarked bool
	}{
		{"type marker covers fields", marker + "type T struct {\n\tA string\n\tB string\n}", []string{"T.A", "T.B"}, true},
		{"field marker alone", "type T struct {\n\t" + marker + "\tA string\n}", []string{"T.A"}, true},
		{"neither marked", "type T struct {\n\tA string\n}", []string{"T.A"}, false},
		{"unexported field excluded", marker + "type T struct {\n\ta string\n}", nil, false},
		{"non-struct type", marker + "type T string", nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fields := exportedFieldsWithTypeDoc(firstDecl(t, test.source))
			var names []string
			marked := len(fields) > 0
			for _, field := range fields {
				names = append(names, field.name)
				if !strings.Contains(field.doc, "Deprecated:") {
					marked = false
				}
			}
			if !sameStrings(names, test.wantFields) {
				t.Fatalf("fields = %v, want %v", names, test.wantFields)
			}
			if marked != test.wantMarked {
				t.Fatalf("all fields marked = %v, want %v", marked, test.wantMarked)
			}
		})
	}
}

// TestDeclaresAFaultCodeMatrix covers the narrowing that stopped the fault-code
// location guard reporting every file that merely handles an error. The
// parameter and switch rows are the false positives it had on its first run.
func TestDeclaresAFaultCodeMatrix(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
		want   bool
	}{
		{"const declaration", "const X fault.Code = \"A\"", true},
		{"grouped const declaration", "const (\n\tX fault.Code = \"A\"\n)", true},
		{"var declaration", "var X fault.Code = \"A\"", true},

		{"function parameter", "func f(code fault.Code) {}", false},
		{"struct field", "type T struct{ Code fault.Code }", false},
		{"switch over a code", "func f(c fault.Code) {\n\tswitch c {\n\tcase other:\n\t}\n}", false},
		{"conversion in a call", "func f() { g(fault.Code(\"x\")) }", false},
		{"type definition", "type X fault.Code", false},
		{"another package's Code", "const X other.Code = \"A\"", false},
		{"untyped const", "const X = \"A\"", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := declaresAFaultCode(parseGuardFixture(t, test.source)); got != test.want {
				t.Fatalf("declaresAFaultCode = %v, want %v", got, test.want)
			}
		})
	}
}

// TestWeightDimensionsMatrix covers how the healer-weight enumeration check
// recognises a dimension. The shapes differ across the five sites it has to
// read, and a shape it fails to see is a dimension that escapes the digest.
func TestWeightDimensionsMatrix(t *testing.T) {
	known := map[string]bool{"Tag": true, "ID": true, "Framework": true}
	for _, test := range []struct {
		name   string
		source string
		want   []string
	}{
		{"field of a Weights field", "func f() { _ = policy.Weights.Tag }", []string{"Tag"}},
		{"nested owner", "func f() { _ = v.HealerPolicy.Weights.Tag }", []string{"Tag"}},
		{"bare weights identifier", "func f() { _ = weights.Tag }", []string{"Tag"}},
		{"lowercase weights field", "func f() { _ = s.weights.Framework }", []string{"Framework"}},
		{"address-of", "func f() { _ = []*float64{&policy.Weights.ID} }", []string{"ID"}},
		{"composite literal keys", "func f() { _ = HealerWeightsSnapshot{Tag: 1, ID: 2} }", []string{"ID", "Tag"}},
		{"qualified composite literal", "func f() { _ = execution.HealerWeightsSnapshot{Framework: 0} }", []string{"Framework"}},

		{"not a weights owner", "func f() { _ = policy.Thresholds.Tag }", nil},
		{"unknown dimension", "func f() { _ = policy.Weights.Unknown }", nil},
		{"non-weights literal", "func f() { _ = Thresholds{Tag: 1} }", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := weightDimensionsIn(firstDecl(t, test.source), known)
			if !sameStrings(got, test.want) {
				t.Fatalf("dimensions = %v, want %v", got, test.want)
			}
		})
	}
}

// TestDifferenceIsOrderedByTheVocabulary keeps the parity guard's report a
// function of the vocabulary rather than of map iteration, so a failure names
// the same missing dimension every run.
func TestDifferenceIsOrderedByTheVocabulary(t *testing.T) {
	want := []string{"Attrs", "Framework", "Tag"}
	if got := difference(want, []string{"Tag"}); !reflect.DeepEqual(got, []string{"Attrs", "Framework"}) {
		t.Fatalf("difference = %v", got)
	}
	if got := difference(want, want); got != nil {
		t.Fatalf("difference of equal sets = %v, want nil", got)
	}
	if got := difference(nil, want); got != nil {
		t.Fatalf("difference from an empty vocabulary = %v, want nil", got)
	}
}
