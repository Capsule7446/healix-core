package architecture_test

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// markdownFile is one documentation file, read once and shared by every doc
// link check in this package.
type markdownFile struct {
	rel   string
	dir   string
	lines []string
}

var (
	docCorpusOnce  sync.Once
	docCorpusFiles []markdownFile
	docCorpusErr   error
)

// documentationCorpus walks the repository once and returns every markdown file
// with its lines already read. Both checks below scan this same corpus, so the
// tree is walked once and each file is opened once per test binary rather than
// once per check.
func documentationCorpus(t *testing.T, root string) []markdownFile {
	t.Helper()
	docCorpusOnce.Do(func() {
		docCorpusErr = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && d.Name() == ".git" {
				return filepath.SkipDir
			}
			if d.IsDir() || filepath.Ext(d.Name()) != ".md" {
				return nil
			}
			lines, err := readLines(path)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			docCorpusFiles = append(docCorpusFiles, markdownFile{
				rel: filepath.ToSlash(rel), dir: filepath.Dir(path), lines: lines})
			return nil
		})
	})
	if docCorpusErr != nil {
		t.Fatalf("reading documentation corpus: %v", docCorpusErr)
	}
	return docCorpusFiles
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.ToSlash(path), err)
	}
	return lines, nil
}

// eachDocLink applies re to every line of every markdown file and hands each
// match to visit with the file, the 1-based line number, and the submatches.
// Submatch 1 is the link target; remote targets are skipped because only
// in-tree paths can be resolved against the working copy.
func eachDocLink(files []markdownFile, re *regexp.Regexp,
	visit func(file markdownFile, lineNum int, groups []string)) {
	for _, file := range files {
		for i, line := range file.lines {
			for _, groups := range re.FindAllStringSubmatch(line, -1) {
				if strings.HasPrefix(groups[1], "http://") || strings.HasPrefix(groups[1], "https://") {
					continue
				}
				visit(file, i+1, groups)
			}
		}
	}
}

// TestEveryDocLinkToSourceResolves fails when a markdown link points at a Go
// file that does not exist. Renames land in the tree and the docs pointing at
// them do not: the v0.4 execution-instance rename left 182 dead links behind
// and nothing noticed, because a dead relative link in markdown is invisible
// to the compiler, to go vet, and to every test in this suite.
func TestEveryDocLinkToSourceResolves(t *testing.T) {
	root := repositoryRoot(t)
	linkRe := regexp.MustCompile(`\]\(([^)\s#]+\.go)(?:#[^)]*)?\)`)

	eachDocLink(documentationCorpus(t, root), linkRe, func(file markdownFile, lineNum int, groups []string) {
		target := groups[1]
		abs := filepath.Clean(filepath.Join(file.dir, target))
		if _, err := os.Stat(abs); os.IsNotExist(err) {
			t.Errorf("%s:%d: broken link -> %s (resolved: %s)",
				file.rel, lineNum, target, filepath.ToSlash(abs))
		}
	})
}

// TestEveryDocLinkToMarkdownResolves fails when a markdown link points at a
// markdown file that does not exist. The .go guard above cannot see this class:
// moving docs/refactor/business-error-contract/error-code-registry.md to
// docs/contracts/ broke seven inbound links and every test stayed green, because
// each one names a .md target. A doc set that reorganises without this guard
// rots exactly the way the .go links did.
func TestEveryDocLinkToMarkdownResolves(t *testing.T) {
	root := repositoryRoot(t)
	linkRe := regexp.MustCompile(`\]\(([^)\s#]+\.md)(?:#[^)]*)?\)`)

	eachDocLink(documentationCorpus(t, root), linkRe, func(file markdownFile, lineNum int, groups []string) {
		target := groups[1]
		abs := filepath.Clean(filepath.Join(file.dir, target))
		if _, err := os.Stat(abs); os.IsNotExist(err) {
			t.Errorf("%s:%d: broken link -> %s (resolved: %s)",
				file.rel, lineNum, target, filepath.ToSlash(abs))
		}
	})
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
	index := newTestFuncIndex()

	eachDocLink(documentationCorpus(t, root), linkRe, func(file markdownFile, lineNum int, groups []string) {
		target, funcName := groups[1], groups[2]
		if !strings.HasPrefix(funcName, "Test") {
			return
		}
		abs := filepath.Clean(filepath.Join(file.dir, target))
		if _, err := os.Stat(abs); os.IsNotExist(err) {
			return
		}
		if !index.has(abs, funcName) {
			t.Errorf("%s:%d: test function %s not found in %s",
				file.rel, lineNum, funcName, target)
		}
	})
}

// testFuncIndex answers "does this file declare this function" while parsing
// each file at most once. The docs name far more test functions than they name
// distinct files, so parsing per link re-parsed the busiest targets dozens of
// times over.
type testFuncIndex struct {
	byPath map[string]map[string]bool
}

func newTestFuncIndex() *testFuncIndex {
	return &testFuncIndex{byPath: make(map[string]map[string]bool)}
}

// has reports whether path declares funcName. A file that fails to parse
// indexes as empty, so every name in it reads as absent.
func (idx *testFuncIndex) has(path, funcName string) bool {
	funcs, ok := idx.byPath[path]
	if !ok {
		funcs = make(map[string]bool)
		if parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0); err == nil {
			for _, decl := range parsed.Decls {
				if fn, isFunc := decl.(*ast.FuncDecl); isFunc {
					funcs[fn.Name.Name] = true
				}
			}
		}
		idx.byPath[path] = funcs
	}
	return funcs[funcName]
}
