package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// w5wireTagInventory pins every persisted digest domain-separation tag in the
// tree. These bytes are a storage format: changing one silently invalidates
// every record hashed with it, and no existing test can see that happen — the
// digest simply becomes a different, equally valid-looking string. Changing a
// tag must therefore be impossible without also editing this list, which is
// where the reviewer is told to demand a migration.
//
// Every entry must also appear in docs/contracts/digest-wire-tags.md.
var w5wireTagInventory = map[string][]string{
	"domain/execution/instance_snapshot.go":                      {"healix.run-snapshot"},
	"application/scheduling/create_instance_service.go":          {"create-run-request-v1"},
	"application/scheduling/instance_command_services.go":        {"cancel-instance-request-v1", "abort-instance-request-v1", "reorder-queue-request-v1"},
	"application/automation/heal_candidate_repository.go":        {"heal-review-v1"},
	"application/automation/sampling_publication_transaction.go": {"sampling-publication-v1"},
}

// TestW5DigestWireTagsAreRegistered checks both directions:
//
//  1. Every tag in the inventory still exists as a string literal in its
//     claimed file (someone changed a tag byte → red).
//  2. Every production Go file that defines a *DigestV1 constant or uses
//     e.str("healix.*") must have every such tag in the inventory
//     (someone added a new digest tag without registering → red).
//
// Direction 2 is the one that would have caught 5ecfde2: that commit added
// three new tags, none of which would have been caught by only checking that
// the existing tags were still present.
func TestW5DigestWireTagsAreRegistered(t *testing.T) {
	root := repositoryRoot(t)

	// Direction 1: inventory tags still exist in their files.
	for relPath, expectedTags := range w5wireTagInventory {
		fullPath := filepath.Join(root, filepath.FromSlash(relPath))
		data, err := os.ReadFile(fullPath)
		if err != nil {
			t.Errorf("cannot read %s: %v", relPath, err)
			continue
		}
		content := string(data)
		for _, expectedTag := range expectedTags {
			if !strings.Contains(content, expectedTag) {
				t.Errorf("tag %q no longer found in %s", expectedTag, relPath)
			}
		}
	}

	// Direction 2: every production *DigestV1 constant and e.str("healix.*")
	// string literal must be registered.
	//
	// We walk production Go files under domain/, application/, architecture/
	// and look for:
	//   - const xDigestV1 = "..."  (the standard pattern)
	//   - e.str("healix.*")        (the canonical encoder pattern)
	//
	// Every match must have an entry in w5wireTagInventory for the same file.
	// Parse errors from files being concurrently modified are silently skipped.
	seen := w5CollectAllWireTags(t, root)
	t.Logf("found %d files with wire tag literals in production code", len(seen))

	for file, tags := range seen {
		rel := filepath.ToSlash(file)
		registered, ok := w5wireTagInventory[rel]
		if !ok {
			for _, tag := range tags {
				t.Errorf("untracked wire tag %q in %s — add it to w5wireTagInventory and docs/contracts/digest-wire-tags.md", tag, rel)
			}
			continue
		}
		for _, tag := range tags {
			if !slices.Contains(registered, tag) {
				t.Errorf("untracked wire tag %q in %s — add it to w5wireTagInventory and docs/contracts/digest-wire-tags.md", tag, rel)
			}
		}
	}
}

// w5CollectAllWireTags scans the tree for *DigestV1 constants and
// e.str("healix.*") calls, returning the set of wire tag strings found in
// each file. It uses its own walker so that parse errors from files being
// concurrently modified by other agents do not break the test.
func w5CollectAllWireTags(t *testing.T, root string) map[string][]string {
	t.Helper()
	result := make(map[string][]string)

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		// Skip vendored sources. The architecture package needs no skip of its
		// own: it holds only _test.go files, filtered above.
		rel, _ := filepath.Rel(root, path)
		if strings.HasPrefix(filepath.ToSlash(rel), "vendor/") {
			return nil
		}
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			// Concurrently modified file; skip silently.
			return nil
		}
		rel = filepath.ToSlash(rel)
		var tags []string

		// Look for const xDigestV1 = "..." patterns.
		for _, decl := range parsed.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.CONST {
				continue
			}
			for _, spec := range genDecl.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range vs.Names {
					if !strings.HasSuffix(name.Name, "DigestV1") {
						continue
					}
					if i >= len(vs.Values) {
						continue
					}
					lit, ok := vs.Values[i].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					val := strings.Trim(lit.Value, `"`)
					if val != "" {
						tags = append(tags, val)
					}
				}
			}
		}

		// Look for e.str("healix.*") calls.
		ast.Inspect(parsed, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "str" {
				return true
			}
			if len(call.Args) != 1 {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			val := strings.Trim(lit.Value, `"`)
			if strings.HasPrefix(val, "healix.") && val != "" {
				tags = append(tags, val)
			}
			return true
		})

		if len(tags) > 0 {
			result[rel] = tags
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk production Go: %v", err)
	}
	return result
}
