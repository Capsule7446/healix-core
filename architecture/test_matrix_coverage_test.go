package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestEveryTestFunctionAppearsInItsMatrix is the reverse of
// TestEveryDocLinkTestNameExists.
//
// That guard walks docs -> code and catches a matrix row naming a function
// that was renamed or deleted. Nothing walked code -> docs, so a *new* test
// function was invisible: the matrices stayed green while drifting further
// from the suite they claim to index. A matrix that silently omits rows is
// worse than no matrix, because "the entry point is covered" is exactly what
// a reader consults it to learn.
//
// The rule is deliberately shallow — a name must appear, nothing is asserted
// about the description beside it. Enforcing prose quality mechanically is not
// possible; enforcing that no test is missing is, and it is the half that
// actually rots.
func TestEveryTestFunctionAppearsInItsMatrix(t *testing.T) {
	root := repositoryRoot(t)
	backtick := "\x60"
	nameRe := regexp.MustCompile(backtick + `(Test[A-Za-z0-9_]*)` + backtick)

	for _, dir := range matrixDirectories(t, root) {
		matrix := filepath.Join(dir, "TEST_CASES.md")
		content, err := os.ReadFile(matrix)
		if err != nil {
			t.Fatalf("read %s: %v", matrix, err)
		}
		named := make(map[string]bool)
		for _, match := range nameRe.FindAllStringSubmatch(string(content), -1) {
			named[match[1]] = true
		}

		var missing []string
		for _, name := range topLevelTestFunctions(t, dir) {
			if !named[name] {
				missing = append(missing, name)
			}
		}
		if len(missing) == 0 {
			continue
		}
		sort.Strings(missing)
		rel, _ := filepath.Rel(root, matrix)
		t.Errorf("%s: %d test function(s) have no matrix row: %s",
			filepath.ToSlash(rel), len(missing), strings.Join(missing, ", "))
	}
}

// TestEveryProductionPackageOwnsAMatrix closes the blind spot in the guard
// above: a package with no TEST_CASES.md is not scanned, so a brand-new
// package is exempt from the row check by virtue of having skipped the matrix
// entirely. That is precisely how domain/fault went without one — the index
// carried a prose admission of the gap for as long as anyone cared to read it,
// which is not the same as a failing build.
func TestEveryProductionPackageOwnsAMatrix(t *testing.T) {
	root := repositoryRoot(t)

	var missing []string
	for _, area := range []string{"domain", "application"} {
		entries, err := os.ReadDir(filepath.Join(root, area))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			dir := filepath.Join(root, area, entry.Name())
			if !hasProductionGo(t, dir) {
				continue
			}
			if _, err := os.Stat(filepath.Join(dir, "TEST_CASES.md")); os.IsNotExist(err) {
				missing = append(missing, filepath.ToSlash(filepath.Join(area, entry.Name())))
			}
		}
	}
	if len(missing) == 0 {
		return
	}
	sort.Strings(missing)
	t.Errorf("%d package(s) with production code own no TEST_CASES.md: %s\n"+
		"add the matrix and link it from docs/testing/TEST_CASES_INDEX.md",
		len(missing), strings.Join(missing, ", "))
}

// hasProductionGo reports whether dir declares any non-test Go file directly.
func hasProductionGo(t *testing.T, dir string) bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() && strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			return true
		}
	}
	return false
}

// matrixDirectories returns every directory that owns a TEST_CASES.md. The
// index treats those directories as the unit of ownership, so a subdirectory
// without its own matrix (application/*/conformancetest) is not scanned here —
// its cases belong to the parent package's matrix and are listed there.
func matrixDirectories(t *testing.T, root string) []string {
	t.Helper()
	var dirs []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if name := entry.Name(); name == ".git" || name == "docs" {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() == "TEST_CASES.md" {
			dirs = append(dirs, filepath.Dir(path))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) == 0 {
		t.Fatal("no TEST_CASES.md found; the guard would pass vacuously")
	}
	sort.Strings(dirs)
	return dirs
}

// topLevelTestFunctions lists the Test functions declared directly in dir.
// Fuzz targets are excluded: they are seeded corpora rather than named cases,
// and the matrices index cases.
func topLevelTestFunctions(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, decl := range parsed.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Recv != nil {
				continue
			}
			if strings.HasPrefix(fn.Name.Name, "Test") {
				names = append(names, fn.Name.Name)
			}
		}
	}
	return names
}
