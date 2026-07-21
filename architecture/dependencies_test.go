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
	"unicode"
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

func TestWorkspacePackageIsRemoved(t *testing.T) {
	root := repositoryRoot(t)
	workspacePath := filepath.Join(root, "domain", "workspace")
	if _, err := os.Stat(workspacePath); !os.IsNotExist(err) {
		t.Fatalf("removed workspace domain directory must not exist: %v", err)
	}
}

func TestCoreOwnsNoBusinessMetricsProjection(t *testing.T) {
	root := repositoryRoot(t)
	matches, err := filepath.Glob(filepath.Join(root, "domain", "metrics", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("domain/metrics must be owned by a consuming project's read side: %v", matches)
	}
}

func TestDomainOwnsBehaviorNotReadModelsOrStoragePorts(t *testing.T) {
	root := repositoryRoot(t)
	err := walkProductionGo(filepath.Join(root, "domain"), func(path string, parsed *ast.File, fset *token.FileSet) {
		ast.Inspect(parsed, func(node ast.Node) bool {
			declaration, ok := node.(*ast.TypeSpec)
			if !ok {
				return true
			}
			contract, ok := declaration.Type.(*ast.InterfaceType)
			if !ok {
				return true
			}
			if reason, forbidden := forbiddenDomainInterfaceReason(declaration.Name.Name, contract); forbidden {
				rel, _ := filepath.Rel(root, path)
				t.Errorf("%s:%d: domain interface %s is forbidden: %s", filepath.ToSlash(rel),
					fset.Position(declaration.Pos()).Line, declaration.Name.Name, reason)
			}
			return false
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestForbiddenDomainInterfaceReason(t *testing.T) {
	tests := []struct {
		name      string
		contract  string
		forbidden bool
	}{
		{name: "FolderReader", contract: "interface { ListFolders(); GetFolder() }", forbidden: true},
		{name: "RunQuery", contract: "interface { Execute() }", forbidden: true},
		{name: "MetricsProjection", contract: "interface { Observe() }", forbidden: true},
		{name: "WorkspaceStore", contract: "interface { Commit() }", forbidden: true},
		{name: "NodeRepository", contract: "interface { Resolve() }", forbidden: true},
		{name: "Gateway", contract: "interface { FindNode() }", forbidden: true},
		{name: "Gateway", contract: "interface { SaveNode() }", forbidden: true},
		{name: "Gateway", contract: "interface { NodeRepository }", forbidden: true},
		{name: "Reader", contract: "interface { Exists(); Visible(); Text(); Attribute() }"},
		{name: "ValidationStateReader", contract: "interface { ValidationState() }"},
		{name: "SecretResolver", contract: "interface { Resolve() }"},
		{name: "FrameworkDetector", contract: "interface { Detect() }"},
		{name: "Locator", contract: "interface { Locate() }"},
		{name: "Healer", contract: "interface { Heal() }"},
	}
	for _, test := range tests {
		t.Run(test.name+test.contract, func(t *testing.T) {
			parsed, err := parser.ParseExpr(test.contract)
			if err != nil {
				t.Fatal(err)
			}
			_, forbidden := forbiddenDomainInterfaceReason(test.name, parsed.(*ast.InterfaceType))
			if forbidden != test.forbidden {
				t.Fatalf("forbidden = %v, want %v", forbidden, test.forbidden)
			}
		})
	}
}

func forbiddenDomainInterfaceReason(name string, contract *ast.InterfaceType) (string, bool) {
	forbiddenRoles := map[string]struct{}{
		"Repository": {}, "Store": {}, "Storage": {}, "Projection": {}, "Query": {}, "Queries": {},
	}
	for _, word := range camelCaseWords(name) {
		if _, forbidden := forbiddenRoles[word]; forbidden {
			return "role name belongs to application queries or infrastructure storage", true
		}
	}
	for _, field := range contract.Methods.List {
		if len(field.Names) == 0 {
			if identifier, ok := field.Type.(*ast.Ident); ok {
				for _, word := range camelCaseWords(identifier.Name) {
					if _, forbidden := forbiddenRoles[word]; forbidden {
						return "embedded interface hides an application or storage role", true
					}
				}
			}
			continue
		}
		for _, method := range field.Names {
			if persistenceOrQueryMethod(method.Name) {
				return "method is shaped like aggregate retrieval or persistence", true
			}
		}
	}
	return "", false
}

func camelCaseWords(name string) []string {
	if name == "" {
		return nil
	}
	runes := []rune(name)
	start := 0
	words := make([]string, 0, 4)
	for index := 1; index < len(runes); index++ {
		boundary := unicode.IsUpper(runes[index]) && (!unicode.IsUpper(runes[index-1]) ||
			(index+1 < len(runes) && unicode.IsLower(runes[index+1])))
		if boundary {
			words = append(words, string(runes[start:index]))
			start = index
		}
	}
	return append(words, string(runes[start:]))
}

func persistenceOrQueryMethod(name string) bool {
	verbs := []string{"Get", "List", "Find", "Search", "Query", "Load", "Lookup", "Save", "Create",
		"Update", "Delete", "Upsert", "Insert", "Remove", "Archive", "Restore"}
	for _, verb := range verbs {
		if strings.HasPrefix(name, verb) && (len(name) == len(verb) || unicode.IsUpper(rune(name[len(verb)]))) {
			return true
		}
	}
	return false
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
	case "heal", "node", "execution", "evidence":
		return "execution", true
	case "sampling", "automation":
		return "automation", true
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
