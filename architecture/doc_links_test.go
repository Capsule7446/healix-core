package architecture_test

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestEveryDocLinkToSourceResolves fails when a markdown link points at a Go
// file that does not exist. Renames land in the tree and the docs pointing at
// them do not: the v0.4 execution-instance rename left 182 dead links behind
// and nothing noticed, because a dead relative link in markdown is invisible
// to the compiler, to go vet, and to every test in this suite.
func TestEveryDocLinkToSourceResolves(t *testing.T) {
	root := repositoryRoot(t)
	linkRe := regexp.MustCompile(`\]\(([^)\s#]+\.go)(?:#[^)]*)?\)`)

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if d.IsDir() || filepath.Ext(d.Name()) != ".md" {
			return nil
		}

		dir := filepath.Dir(path)
		f, err := os.Open(path)
		if err != nil {
			t.Errorf("cannot open %s: %v", filepath.ToSlash(path), err)
			return nil
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			matches := linkRe.FindAllStringSubmatch(line, -1)
			for _, m := range matches {
				target := m[1]
				if len(target) > 8 && (target[:7] == "http://" || target[:8] == "https://") {
					continue
				}
				abs := filepath.Clean(filepath.Join(dir, target))
				if _, err := os.Stat(abs); os.IsNotExist(err) {
					rel, _ := filepath.Rel(root, path)
					t.Errorf("%s:%d: broken link -> %s (resolved: %s)",
						filepath.ToSlash(rel), lineNum, target, filepath.ToSlash(abs))
				}
			}
		}
		if err := scanner.Err(); err != nil {
			t.Errorf("%s: scanner error: %v", filepath.ToSlash(path), err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk error: %v", err)
	}
}

// TestEveryDocLinkTestNameExists checks that when a .go link is followed by
// a backtick-quoted name like MiddleDot TestFoo, that function actually exists
// in the target Go file. This catches test-case-matrices that reference
// renamed test functions.
func TestEveryDocLinkTestNameExists(t *testing.T) {
	root := repositoryRoot(t)
	bt := "\x60"
	pat := `\]\(([^)\s#]+\.go)(?:#[^)]*)?\)\s*` + "\xc2\xb7" + `\s*` + bt + `([^` + bt + `]+)` + bt
	linkRe := regexp.MustCompile(pat)

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if d.IsDir() || filepath.Ext(d.Name()) != ".md" {
			return nil
		}

		dir := filepath.Dir(path)
		f, err := os.Open(path)
		if err != nil {
			t.Errorf("cannot open %s: %v", filepath.ToSlash(path), err)
			return nil
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			matches := linkRe.FindAllStringSubmatch(line, -1)
			for _, m := range matches {
				target := m[1]
				funcName := m[2]
				if len(target) > 8 && (target[:7] == "http://" || target[:8] == "https://") {
					continue
				}
				if !strings.HasPrefix(funcName, "Test") {
					continue
				}
				abs := filepath.Clean(filepath.Join(dir, target))
				if _, err := os.Stat(abs); os.IsNotExist(err) {
					continue
				}
				if !testFuncExists(abs, funcName) {
					rel, _ := filepath.Rel(root, path)
					t.Errorf("%s:%d: test function %s not found in %s",
						filepath.ToSlash(rel), lineNum, funcName, target)
				}
			}
		}
		if err := scanner.Err(); err != nil {
			t.Errorf("%s: scanner error: %v", filepath.ToSlash(path), err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk error: %v", err)
	}
}

func testFuncExists(path, funcName string) bool {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return false
	}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == funcName {
			return true
		}
	}
	return false
}
