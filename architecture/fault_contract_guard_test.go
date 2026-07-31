package architecture_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// This guard cross-checks the published fault contract against the code that
// produces it. Without it the registry is maintained only by review, and a Kind
// or safe message can drift from what it promises without any test going red.
//
// It resolves three distinct (Kind, Code) pairing shapes, because a scan that
// only understood direct fault.New/Wrap calls would cover well under half the
// registry and still look green:
//
//	direct       fault.New(fault.Conflict, CodeX, "msg")
//	helper       mustXFault(fault.Conflict, CodeX, "msg")   — Kind is a parameter
//	fixedHelper  wrapNodeFault(cause, CodeX, "msg")         — Kind is in the body
//
// The scan matches adjacent argument pairs rather than callee names, so it covers
// f(kind, code, ...) and f(cause, kind, code, ...) alike without enumerating
// every helper.

var kindConstants = map[string]string{
	"InvalidArgument":    "INVALID_ARGUMENT",
	"OutOfRange":         "OUT_OF_RANGE",
	"NotFound":           "NOT_FOUND",
	"AlreadyExists":      "ALREADY_EXISTS",
	"Conflict":           "CONFLICT",
	"FailedPrecondition": "FAILED_PRECONDITION",
	"ResourceExhausted":  "RESOURCE_EXHAUSTED",
	"Canceled":           "CANCELED",
	"DeadlineExceeded":   "DEADLINE_EXCEEDED",
	"Unavailable":        "UNAVAILABLE",
	"Internal":           "INTERNAL",
}

var upperSnake = regexp.MustCompile(`^[A-Z][A-Z0-9_]{2,62}$`)

// violationCodePrefix marks the shared-kernel reason vocabulary. These codes are
// carried inside an aggregate envelope's violations and must never be the code of
// a top-level Error, so they never take part in a (Kind, Code) pairing.
const violationCodePrefix = "VALIDATION_"

type registryRow struct {
	kind    string
	message string
	line    int
}

type pairing struct {
	kind      string
	message   string
	shape     string
	site      string
	directory string
}

// prefixOwners is the inventory's explicit prefix-to-producer mapping. It is
// deliberately a fixed table and not derived from package paths: the inventory
// assigns SAMPLING_* to application/automation on purpose, so any rule inferring
// the prefix from the producing package would either reject that row or silently
// legitimise cross-context prefixes everywhere else.
var prefixOwners = map[string][]string{
	// The registry states that EXECUTION_* owns node runtime, engine, scheduling,
	// and execution-application failures, not just domain/execution.
	"EXECUTION": {
		"domain/execution", "domain/node",
		"application/engine", "application/scheduling", "application/execution",
	},
	"AUTOMATION":    {"domain/automation", "application/automation"},
	"SAMPLING":      {"domain/sampling", "application/automation"},
	"EVIDENCE":      {"domain/evidence"},
	"FINGERPRINT":   {"domain/fingerprint"},
	"INTERPOLATION": {"domain/interpolation"},
	"PARAMETER":     {"domain/parameter"},
	// The reason vocabulary belongs to the shared kernel and has no bounded context.
	"VALIDATION": {"domain/fault"},
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve module root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("module root %s has no go.mod: %v", root, err)
	}
	return root
}

// parseRegistry reads the code tables. A row qualifies only when columns 1 and 2
// are both UPPER_SNAKE, which structurally excludes the historical mapping table
// (lowercase column 1) and the violation-code table (prose column 2) without
// needing to track section headings.
func parseRegistry(t *testing.T, root string) (map[string]registryRow, []string) {
	t.Helper()
	path := filepath.Join(root, "docs", "refactor", "business-error-contract", "error-code-registry.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read registry: %v", err)
	}
	rows := map[string]registryRow{}
	var order []string
	clean := func(s string) string { return strings.Trim(strings.TrimSpace(s), "`") }
	// GFM permits a table row without the leading pipe; this parser does not. A
	// row a human reads as a published code must never be silently skipped, so
	// anything shaped like one is a hard error rather than a formatting nit.
	pipelessCodeRow := regexp.MustCompile("^`[A-Z][A-Z0-9_]{2,62}`\\s*\\|")
	for index, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") {
			if pipelessCodeRow.MatchString(trimmed) {
				t.Errorf("registry line %d looks like a code row but lacks the leading pipe, so parsing would silently skip it: %s", index+1, trimmed)
			}
			continue
		}
		columns := strings.Split(line, "|")
		if len(columns) < 4 {
			continue
		}
		code, kind := clean(columns[1]), clean(columns[2])
		if !upperSnake.MatchString(code) || !upperSnake.MatchString(kind) {
			continue
		}
		if _, duplicate := rows[code]; duplicate {
			t.Errorf("registry declares %s more than once (line %d); a published code must have exactly one row", code, index+1)
			continue
		}
		rows[code] = registryRow{kind: kind, message: clean(columns[3]), line: index + 1}
		order = append(order, code)
	}
	if len(rows) == 0 {
		t.Fatal("registry parsed to zero codes; the table shape must have changed")
	}
	return rows, order
}

func parseProductionFiles(t *testing.T, root string) (*token.FileSet, map[string]*ast.File) {
	t.Helper()
	fileSet := token.NewFileSet()
	parsed := map[string]*ast.File{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if name := entry.Name(); name == ".git" || name == "docs" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fileSet, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		parsed[path] = file
		return nil
	})
	if err != nil {
		t.Fatalf("walk module: %v", err)
	}
	return fileSet, parsed
}

func faultSelector(expr ast.Expr, want string) (string, bool) {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	pkg, ok := selector.X.(*ast.Ident)
	if !ok || pkg.Name != "fault" {
		return "", false
	}
	if want != "" && selector.Sel.Name != want {
		return "", false
	}
	return selector.Sel.Name, true
}

func identifierName(expr ast.Expr) (string, bool) {
	switch typed := expr.(type) {
	case *ast.Ident:
		return typed.Name, true
	case *ast.SelectorExpr:
		return typed.Sel.Name, true
	}
	return "", false
}

func stringLiteral(expr ast.Expr) (string, bool) {
	literal, ok := expr.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(literal.Value)
	if err != nil {
		return "", false
	}
	return value, true
}

// codeConstants resolves a code-constant identifier to its code string. Lookup is
// per package directory, because the same constant name legitimately exists in
// more than one context — evidence and automation both declare
// CodeHealObservationInvalid for their own prefixed code — and a repo-wide map
// would attribute a pairing to the wrong contract row.
type codeConstants struct {
	byDirectory map[string]map[string]string
}

func (c codeConstants) lookup(filePath, name string) (string, bool) {
	if code, ok := c.byDirectory[filepath.Dir(filePath)][name]; ok {
		return code, true
	}
	// A cross-package reference resolves only when exactly one package declares the
	// name; otherwise the qualifier would be needed to disambiguate and the pairing
	// is left unattributed rather than guessed.
	found, matches := "", 0
	for _, names := range c.byDirectory {
		if code, ok := names[name]; ok {
			found, matches = code, matches+1
		}
	}
	return found, matches == 1
}

func (c codeConstants) all() map[string]bool {
	declared := map[string]bool{}
	for _, names := range c.byDirectory {
		for _, code := range names {
			declared[code] = true
		}
	}
	return declared
}

// collectCodeConstants indexes declarations of the form `CodeX fault.Code = "..."`.
// It reports same-package duplicates, and constants that sit in a fault.Code block
// but omit the type, since an untyped sibling silently escapes every type-keyed
// check.
func collectCodeConstants(parsed map[string]*ast.File) (codeConstants, []string, []string) {
	constants := codeConstants{byDirectory: map[string]map[string]string{}}
	var ambiguous []string
	var untyped []string
	for path, file := range parsed {
		for _, decl := range file.Decls {
			group, ok := decl.(*ast.GenDecl)
			if !ok || group.Tok != token.CONST {
				continue
			}
			blockDeclaresCode := false
			for _, spec := range group.Specs {
				if valueSpec, ok := spec.(*ast.ValueSpec); ok {
					if _, isCode := faultSelector(valueSpec.Type, "Code"); isCode {
						blockDeclaresCode = true
					}
				}
			}
			for _, spec := range group.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok || len(valueSpec.Values) == 0 {
					continue
				}
				text, isString := stringLiteral(valueSpec.Values[0])
				if !isString {
					continue
				}
				if _, isCode := faultSelector(valueSpec.Type, "Code"); !isCode {
					if blockDeclaresCode && valueSpec.Type == nil && upperSnake.MatchString(text) {
						untyped = append(untyped, path+": "+valueSpec.Names[0].Name)
					}
					continue
				}
				name := valueSpec.Names[0].Name
				directory := filepath.Dir(path)
				if constants.byDirectory[directory] == nil {
					constants.byDirectory[directory] = map[string]string{}
				}
				if previous, exists := constants.byDirectory[directory][name]; exists && previous != text {
					ambiguous = append(ambiguous, directory+": "+name+" -> "+previous+", "+text)
				}
				constants.byDirectory[directory][name] = text
			}
		}
	}
	return constants, ambiguous, untyped
}

// collectFixedKindHelpers finds helpers that accept a fault.Code but no fault.Kind
// and hardcode one Kind in their body. Every code routed through such a helper
// inherits that Kind with no compiler check, which is why they are resolved here
// rather than trusted.
func collectFixedKindHelpers(parsed map[string]*ast.File) map[string]string {
	helpers := map[string]string{}
	for _, file := range parsed {
		for _, decl := range file.Decls {
			function, ok := decl.(*ast.FuncDecl)
			if !ok || function.Body == nil || function.Type.Params == nil {
				continue
			}
			takesCode, takesKind := false, false
			for _, param := range function.Type.Params.List {
				if _, ok := faultSelector(param.Type, "Code"); ok {
					takesCode = true
				}
				if _, ok := faultSelector(param.Type, "Kind"); ok {
					takesKind = true
				}
			}
			if !takesCode || takesKind {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				callee, ok := faultSelector(call.Fun, "")
				if !ok || (callee != "New" && callee != "Wrap") {
					return true
				}
				for _, arg := range call.Args {
					if name, ok := faultSelector(arg, ""); ok {
						if registryKind, valid := kindConstants[name]; valid {
							helpers[function.Name.Name] = registryKind
						}
					}
				}
				return true
			})
		}
	}
	return helpers
}

type scanResult struct {
	// pairings keeps EVERY construction site per code. Keeping only one let a
	// correct site mask a wrong Kind or a leaking message at another site of the
	// same code, and made the survivor depend on map iteration order.
	pairings         map[string][]pairing
	violationPaired  []string
	topLevelAsReason []string
	// unresolved lists constructions the scan could not statically verify. If
	// these were silently skipped, a code routed through a variable, a slice, a
	// fault.Code("...") conversion, or a non-literal message would simply drop
	// out of coverage with the suite still green.
	unresolved []string
}

// paramRoles maps an enclosing function's parameter names to the fault role they
// carry. An identifier argument that names such a parameter is not unresolvable —
// it is a helper body whose pairing is verified at every call site instead.
func paramRoles(function *ast.FuncDecl) map[string]string {
	roles := map[string]string{}
	if function == nil || function.Type.Params == nil {
		return roles
	}
	for _, param := range function.Type.Params.List {
		role := ""
		switch {
		case func() bool { _, ok := faultSelector(param.Type, "Kind"); return ok }():
			role = "kind"
		case func() bool { _, ok := faultSelector(param.Type, "Code"); return ok }():
			role = "code"
		default:
			if identifier, ok := param.Type.(*ast.Ident); ok && identifier.Name == "string" {
				role = "string"
			}
		}
		if role == "" {
			continue
		}
		for _, name := range param.Names {
			roles[name.Name] = role
		}
	}
	return roles
}

// messageStatus classifies the argument in a message position: a string literal
// is verifiable, an identifier naming a string parameter is deferred to call
// sites, a fault.With* options call means the message was absent (variadic
// options follow it), anything else cannot be verified.
func messageStatus(arg ast.Expr, roles map[string]string) (message string, verifiable bool) {
	if text, ok := stringLiteral(arg); ok {
		return text, true
	}
	if identifier, ok := arg.(*ast.Ident); ok && roles[identifier.Name] == "string" {
		return "", true
	}
	if call, ok := arg.(*ast.CallExpr); ok {
		if name, isFault := faultSelector(call.Fun, ""); isFault && strings.HasPrefix(name, "With") {
			return "", true
		}
	}
	return "", false
}

func scanPairings(fileSet *token.FileSet, root string, parsed map[string]*ast.File,
	constants codeConstants, fixedHelpers map[string]string) scanResult {

	result := scanResult{pairings: map[string][]pairing{}}
	site := func(pos token.Pos) string {
		position := fileSet.Position(pos)
		relative, err := filepath.Rel(root, position.Filename)
		if err != nil {
			relative = position.Filename
		}
		return filepath.ToSlash(relative) + ":" + strconv.Itoa(position.Line)
	}
	record := func(code, kind, message, shape string, pos token.Pos) {
		located := site(pos)
		result.pairings[code] = append(result.pairings[code], pairing{
			kind: kind, message: message, shape: shape, site: located,
			directory: path.Dir(located[:strings.LastIndex(located, ":")]),
		})
	}

	paths := make([]string, 0, len(parsed))
	for filePath := range parsed {
		paths = append(paths, filePath)
	}
	sort.Strings(paths)

	for _, filePath := range paths {
		file := parsed[filePath]
		for _, decl := range file.Decls {
			roles := map[string]string{}
			var body ast.Node = decl
			if function, ok := decl.(*ast.FuncDecl); ok {
				roles = paramRoles(function)
				if function.Body == nil {
					continue
				}
				body = function.Body
			}
			currentPath := filePath
			ast.Inspect(body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				callee, isFaultCall := faultSelector(call.Fun, "")
				direct := isFaultCall && (callee == "New" || callee == "Wrap")

				// A violation's reason code is its first argument and takes no Kind.
				if isFaultCall && callee == "NewViolation" && len(call.Args) > 0 {
					if name, ok := identifierName(call.Args[0]); ok {
						if code, known := constants.lookup(currentPath, name); known && !strings.HasPrefix(code, violationCodePrefix) {
							result.topLevelAsReason = append(result.topLevelAsReason, code+" at "+site(call.Lparen))
						}
					}
				}

				// Direct fault.New / fault.Wrap: the signature fixes every position,
				// so each one must be statically verifiable or explicitly deferred to
				// a helper's call sites. Silently skipping an unresolvable argument is
				// exactly how a construction escapes coverage.
				if direct {
					kindIndex := 0
					if callee == "Wrap" {
						kindIndex = 1
					}
					flag := func(what string) {
						result.unresolved = append(result.unresolved, fmt.Sprintf("fault.%s at %s: %s", callee, site(call.Lparen), what))
					}
					if len(call.Args) < kindIndex+3 {
						flag("call has fewer arguments than the signature requires")
					} else {
						kindArg := call.Args[kindIndex]
						kindName, isSelector := faultSelector(kindArg, "")
						_, validKind := kindConstants[kindName]
						kindDeferred := false
						if identifier, ok := kindArg.(*ast.Ident); ok && roles[identifier.Name] == "kind" {
							kindDeferred = true
						}
						if !(isSelector && validKind) && !kindDeferred {
							flag("Kind is not a fault.<Kind> selector and not a fault.Kind parameter")
						}
						codeArg := call.Args[kindIndex+1]
						codeDeferred := false
						if identifier, ok := codeArg.(*ast.Ident); ok && roles[identifier.Name] == "code" {
							codeDeferred = true
						}
						codeName, hasName := identifierName(codeArg)
						_, codeKnown := constants.lookup(currentPath, codeName)
						if !codeDeferred && !(hasName && codeKnown) {
							flag("Code does not resolve to a declared fault.Code constant (a fault.Code(\"...\") conversion or variable would escape the registry check)")
						}
						if _, verifiable := messageStatus(call.Args[kindIndex+2], roles); !verifiable {
							flag("safe message is not a string literal and not a string parameter, so it cannot be compared with the registry")
						}
					}
				}

				// Shapes 1 and 2: an adjacent (fault.<Kind>, Code<X>) argument pair.
				for index := 0; index+1 < len(call.Args); index++ {
					kindName, ok := faultSelector(call.Args[index], "")
					if !ok {
						continue
					}
					registryKind, valid := kindConstants[kindName]
					if !valid {
						continue
					}
					name, ok := identifierName(call.Args[index+1])
					if !ok {
						continue
					}
					code, known := constants.lookup(currentPath, name)
					if !known {
						continue
					}
					if strings.HasPrefix(code, violationCodePrefix) {
						result.violationPaired = append(result.violationPaired, code+" at "+site(call.Lparen))
						continue
					}
					message := ""
					if index+2 < len(call.Args) {
						text, verifiable := messageStatus(call.Args[index+2], roles)
						if !verifiable {
							result.unresolved = append(result.unresolved, fmt.Sprintf("%s at %s: message after the code is not statically verifiable", code, site(call.Lparen)))
						}
						message = text
					}
					shape := "helper"
					if direct {
						shape = "direct"
					}
					record(code, registryKind, message, shape, call.Lparen)
				}

				// Shape 3: a fixed-Kind helper invoked with a code constant.
				if name, ok := identifierName(call.Fun); ok {
					if kind, isFixed := fixedHelpers[name]; isFixed {
						for index, arg := range call.Args {
							identifier, ok := identifierName(arg)
							if !ok {
								continue
							}
							code, known := constants.lookup(currentPath, identifier)
							if !known || strings.HasPrefix(code, violationCodePrefix) {
								continue
							}
							message := ""
							if index+1 < len(call.Args) {
								text, verifiable := messageStatus(call.Args[index+1], roles)
								if !verifiable {
									result.unresolved = append(result.unresolved, fmt.Sprintf("%s at %s: message after the code is not statically verifiable", code, site(call.Lparen)))
								}
								message = text
							}
							record(code, kind, message, "fixedHelper", call.Lparen)
						}
					}
				}
				return true
			})
		}
	}
	return result
}

func TestRegistryAndProducedFaultsAgree(t *testing.T) {
	root := moduleRoot(t)
	registry, order := parseRegistry(t, root)
	fileSet, parsed := parseProductionFiles(t, root)
	constants, ambiguous, untyped := collectCodeConstants(parsed)
	fixedHelpers := collectFixedKindHelpers(parsed)
	result := scanPairings(fileSet, root, parsed, constants, fixedHelpers)

	for _, entry := range ambiguous {
		t.Errorf("code constant name resolves to more than one code string, so a pairing cannot be attributed: %s", entry)
	}
	for _, entry := range untyped {
		t.Errorf("constant sits in a fault.Code block without the type, so it escapes every type-keyed check: %s", entry)
	}

	declared := constants.all()

	// A construction the scan cannot see is a construction nothing verifies.
	for _, entry := range result.unresolved {
		t.Errorf("statically unverifiable fault construction: %s", entry)
	}

	// A code produced but unregistered is an unpublished contract; a code
	// registered but never declared is a promise with no implementation. EVERY
	// site is checked: verifying only one would let a correct site mask a wrong
	// Kind or a divergent message at another site of the same code.
	for code, sites := range result.pairings {
		row, registered := registry[code]
		if !registered {
			t.Errorf("%s is produced at %s but has no registry row", code, sites[0].site)
			continue
		}
		verifiedMessage := false
		for _, found := range sites {
			if row.kind != found.kind {
				t.Errorf("%s Kind disagrees: code produces %s at %s (%s), registry line %d declares %s. Fix the code, never the published Kind.",
					code, found.kind, found.site, found.shape, row.line, row.kind)
			}
			if found.message != "" {
				verifiedMessage = true
				if row.message != "" && found.message != row.message {
					t.Errorf("%s safe message disagrees: code produces %q at %s, registry line %d declares %q",
						code, found.message, found.site, row.line, row.message)
				}
			}
		}
		if !verifiedMessage {
			t.Errorf("%s has %d construction site(s) but not one carries a literal message the registry row can be compared against", code, len(sites))
		}
	}
	for _, code := range order {
		if !declared[code] {
			t.Errorf("%s is registered but no fault.Code constant declares it", code)
		}
	}
}

// The coverage gate is the reason this guard keeps working. Without it, a code
// later passed through a variable, slice, or map would stop being statically
// checkable and the assertions above would simply stop covering it — silently,
// with the suite still green.
func TestEveryRegisteredCodeIsStaticallyCheckable(t *testing.T) {
	root := moduleRoot(t)
	registry, order := parseRegistry(t, root)
	fileSet, parsed := parseProductionFiles(t, root)
	constants, _, _ := collectCodeConstants(parsed)
	result := scanPairings(fileSet, root, parsed, constants, collectFixedKindHelpers(parsed))

	var unpaired []string
	for _, code := range order {
		if _, ok := result.pairings[code]; !ok {
			unpaired = append(unpaired, code)
		}
	}
	// The denominator is the registry's actual row count. Hardcoding it would turn
	// the next code added or removed into a false green.
	if len(unpaired) != 0 {
		sort.Strings(unpaired)
		t.Errorf("%d of %d registered codes have no checkable (Kind, Code) pairing, so their Kind and message are unverified:\n  %s",
			len(unpaired), len(registry), strings.Join(unpaired, "\n  "))
	}
}

// A prefix names the bounded context that owns the meaning of a code, and only
// that context's producers may mint one. Without this, any package could start
// emitting another context's codes and the prefix would stop meaning anything.
func TestCodePrefixesAreProducedOnlyByTheirOwners(t *testing.T) {
	root := moduleRoot(t)
	registry, _ := parseRegistry(t, root)
	fileSet, parsed := parseProductionFiles(t, root)
	constants, _, _ := collectCodeConstants(parsed)
	result := scanPairings(fileSet, root, parsed, constants, collectFixedKindHelpers(parsed))

	for code, sites := range result.pairings {
		if _, registered := registry[code]; !registered {
			continue // already reported as unregistered by the agreement test
		}
		prefix := code
		if index := strings.Index(code, "_"); index > 0 {
			prefix = code[:index]
		}
		owners, known := prefixOwners[prefix]
		if !known {
			t.Errorf("%s uses prefix %s, which no context claims in the inventory's mapping", code, prefix)
			continue
		}
		for _, found := range sites {
			allowed := false
			for _, owner := range owners {
				if found.directory == owner {
					allowed = true
					break
				}
			}
			if !allowed {
				t.Errorf("%s is produced from %s at %s, but the %s prefix is owned by %s",
					code, found.directory, found.site, prefix, strings.Join(owners, ", "))
			}
		}
	}
}

// A code constant that is not exported cannot be named by a host, so the code is
// published in the registry while remaining unusable for matching.
func TestCodeConstantsAreExported(t *testing.T) {
	root := moduleRoot(t)
	_, parsed := parseProductionFiles(t, root)
	constants, _, _ := collectCodeConstants(parsed)

	for directory, names := range constants.byDirectory {
		for name, code := range names {
			if !ast.IsExported(name) {
				relative, err := filepath.Rel(root, directory)
				if err != nil {
					relative = directory
				}
				t.Errorf("%s declares %s for %s unexported, so a host cannot name it", filepath.ToSlash(relative), name, code)
			}
		}
	}
}

// An exported sentinel error is a second, untyped contract: callers start matching
// on the variable instead of the code, and it can never be changed afterwards.
func TestNoExportedSentinelErrors(t *testing.T) {
	root := moduleRoot(t)
	fileSet, parsed := parseProductionFiles(t, root)

	for path, file := range parsed {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			relative = path
		}
		relative = filepath.ToSlash(relative)
		if !strings.HasPrefix(relative, "domain/") && !strings.HasPrefix(relative, "application/") {
			continue
		}
		for _, decl := range file.Decls {
			group, ok := decl.(*ast.GenDecl)
			if !ok || group.Tok != token.VAR {
				continue
			}
			for _, spec := range group.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for index, name := range valueSpec.Names {
					if !ast.IsExported(name.Name) || !declaresError(valueSpec, index) {
						continue
					}
					line := fileSet.Position(name.Pos()).Line
					t.Errorf("%s:%d exports sentinel error %s; business failures must travel as a registered fault.Code", relative, line, name.Name)
				}
			}
		}
	}
}

// declaresError reports whether a var spec entry is an error value, either by its
// declared type or by being built from errors.New / fmt.Errorf.
func declaresError(spec *ast.ValueSpec, index int) bool {
	if identifier, ok := spec.Type.(*ast.Ident); ok && identifier.Name == "error" {
		return true
	}
	if index >= len(spec.Values) {
		return false
	}
	call, ok := spec.Values[index].(*ast.CallExpr)
	if !ok {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	if !ok {
		return false
	}
	return (pkg.Name == "errors" && selector.Sel.Name == "New") ||
		(pkg.Name == "fmt" && selector.Sel.Name == "Errorf")
}

// The reason vocabulary and the top-level codes are disjoint by construction:
// a reason code answers "why did this field fail", a top-level code names the
// aggregate that rejected the input. Mixing them would make one code mean both.
func TestViolationReasonCodesStayOutOfTopLevelFaults(t *testing.T) {
	root := moduleRoot(t)
	fileSet, parsed := parseProductionFiles(t, root)
	constants, _, _ := collectCodeConstants(parsed)
	result := scanPairings(fileSet, root, parsed, constants, collectFixedKindHelpers(parsed))

	for _, entry := range result.violationPaired {
		t.Errorf("violation reason code used as a top-level fault code: %s", entry)
	}
	for _, entry := range result.topLevelAsReason {
		t.Errorf("top-level code used as a violation reason code: %s", entry)
	}
}
