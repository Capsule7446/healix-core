package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const coreModule = "github.com/Capsule7446/healix-core/"

var forbiddenDomainImports = map[string]string{
	"encoding/json":    "domain must not know JSON encoding",
	"encoding/xml":     "domain must not know XML encoding",
	"database/sql":     "domain must not know SQL storage",
	"gopkg.in/yaml.v3": "domain must not know YAML encoding",
}

var forbiddenHostImports = map[string]string{
	"embed":         "Core must not embed host assets",
	"net/http":      "Core must not own host HTTP transport",
	"os":            "Core must not access the host filesystem or environment",
	"path/filepath": "Core must not interpret host file paths",
}

var forbiddenDomainTagKeys = []string{"json:", "yaml:", "xml:", "db:", "gorm:"}

func TestDependencyDirectionAndPurity(t *testing.T) {
	root := repositoryRoot(t)
	for _, owner := range []string{"domain", "application"} {
		err := walkProductionGo(filepath.Join(root, owner), func(path string, parsed *ast.File, fset *token.FileSet) {
			rel, _ := filepath.Rel(root, path)
			for _, spec := range parsed.Imports {
				imported := unquote(t, spec)
				if reason, banned := forbiddenHostImports[imported]; banned {
					t.Errorf("%s: forbidden host import %q (%s)", filepath.ToSlash(rel), imported, reason)
				}
				if strings.HasPrefix(owner, "domain") {
					if reason, banned := forbiddenDomainImports[imported]; banned {
						t.Errorf("%s: forbidden import %q (%s)", filepath.ToSlash(rel), imported, reason)
					}
					if !isStdlib(imported) && !strings.HasPrefix(imported, coreModule+"domain/") {
						t.Errorf("%s: domain import %q is outside the Core domain", filepath.ToSlash(rel), imported)
					}
				} else if !isStdlib(imported) && !strings.HasPrefix(imported, coreModule) {
					t.Errorf("%s: application import %q is outside Core", filepath.ToSlash(rel), imported)
				}
			}
			if owner != "domain" {
				return
			}
			ast.Inspect(parsed, func(node ast.Node) bool {
				structure, ok := node.(*ast.StructType)
				if !ok {
					return true
				}
				for _, field := range structure.Fields.List {
					if field.Tag == nil {
						continue
					}
					for _, key := range forbiddenDomainTagKeys {
						if strings.Contains(field.Tag.Value, key) {
							t.Errorf("%s:%d: forbidden domain tag %s", filepath.ToSlash(rel),
								fset.Position(field.Tag.Pos()).Line, field.Tag.Value)
						}
					}
				}
				return true
			})
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestBoundedContextIsolation(t *testing.T) {
	root := repositoryRoot(t)
	err := walkProductionGo(filepath.Join(root, "domain"), func(path string, parsed *ast.File, _ *token.FileSet) {
		ownerPackage := filepath.Base(filepath.Dir(path))
		ownerContext, known := domainContext(ownerPackage)
		if !known {
			t.Errorf("domain package %q is missing from the context map", ownerPackage)
			return
		}
		for _, spec := range parsed.Imports {
			imported := unquote(t, spec)
			const domainPrefix = coreModule + "domain/"
			if !strings.HasPrefix(imported, domainPrefix) {
				continue
			}
			importedPackage := strings.Split(strings.TrimPrefix(imported, domainPrefix), "/")[0]
			importedContext, known := domainContext(importedPackage)
			if !known {
				t.Errorf("imported domain package %q is missing from the context map", importedPackage)
				continue
			}
			if ownerContext != "shared" && importedContext != "shared" && ownerContext != importedContext {
				rel, _ := filepath.Rel(root, path)
				t.Errorf("%s: context %s must not import %s package %s", filepath.ToSlash(rel),
					ownerContext, importedContext, importedPackage)
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceExcludesHostConfiguration(t *testing.T) {
	root := repositoryRoot(t)
	forbidden := []string{"type LocalSettings", "type StorageLayout", "type SettingsStore",
		"DataDirectory", "LogDirectory", "BrowserPath", "LanguageSimplifiedChinese", "Close() error",
		"OutputRoot", "ScreenshotFormatJPEG", "ScreenshotJPEGQuality"}
	err := filepath.WalkDir(filepath.Join(root, "domain", "workspace"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, symbol := range forbidden {
			if strings.Contains(string(raw), symbol) {
				rel, _ := filepath.Rel(root, path)
				t.Errorf("%s owns host concern %q", filepath.ToSlash(rel), symbol)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMetricsIsReadOnlyAndEngineDoesNotOwnProjection(t *testing.T) {
	root := repositoryRoot(t)
	metricsSource, err := os.ReadFile(filepath.Join(root, "domain", "metrics", "metrics.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"type Repository interface", "type RunRepository interface",
		"Save(", "Create(", "Update(", "Delete(", "Record(", "Commit("} {
		if strings.Contains(string(metricsSource), forbidden) {
			t.Errorf("metrics contains write contract %q", forbidden)
		}
	}
	if !strings.Contains(string(metricsSource), "type Reader interface") ||
		!strings.Contains(string(metricsSource), "QueryHealQuality") {
		t.Fatal("metrics.Reader.QueryHealQuality contract is missing")
	}
	err = walkAllGo(filepath.Join(root, "application", "engine"), func(path string, parsed *ast.File, _ *token.FileSet) {
		for _, spec := range parsed.Imports {
			if imported := unquote(t, spec); imported == coreModule+"domain/metrics" ||
				strings.HasPrefix(imported, coreModule+"domain/metrics/") {
				t.Errorf("%s: engine must not own metrics projection", path)
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
}

func walkProductionGo(root string, visit func(string, *ast.File, *token.FileSet)) error {
	return walkGo(root, false, visit)
}

func walkAllGo(root string, visit func(string, *ast.File, *token.FileSet)) error {
	return walkGo(root, true, visit)
}

func walkGo(root string, includeTests bool, visit func(string, *ast.File, *token.FileSet)) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			(!includeTests && strings.HasSuffix(entry.Name(), "_test.go")) {
			return nil
		}
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		visit(path, parsed, fset)
		return nil
	})
}

func domainContext(packageName string) (string, bool) {
	switch packageName {
	case "heal", "node":
		return "execution", true
	case "sampling", "workspace":
		return "workspace", true
	case "metrics":
		return "projection", true
	case "fingerprint", "interpolation":
		return "shared", true
	default:
		return "", false
	}
}

func isStdlib(path string) bool {
	return !strings.Contains(strings.Split(path, "/")[0], ".")
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve architecture test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}

func unquote(t *testing.T, spec *ast.ImportSpec) string {
	t.Helper()
	value, err := strconv.Unquote(spec.Path.Value)
	if err != nil {
		t.Fatalf("unquote import %s: %v", spec.Path.Value, err)
	}
	return value
}
