package config

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	testBaseDB     = "postgres://user:pass@localhost:5432/gostock?sslmode=disable"
	testProdSecret = "production-secret-with-at-least-32-bytes-long!"
)

// configEnvVars lists every environment variable read by this package.
// TestConfigEnvVars_MatchesSource keeps it in sync with the non-test source files.
var configEnvVars = []string{
	"APP_ENV",
	"ENV",
	"DATABASE_URL",
	"HTTP_PORT",
	"JWT_SECRET",
	"JWT_ISSUER",
	"JWT_AUDIENCE",
	"ACCESS_TOKEN_TTL_MINUTES",
	"REFRESH_TOKEN_TTL_DAYS",
	"REFRESH_TOKEN_GRACE_PERIOD",
	"COOKIE_SECURE",
	"COOKIE_SAMESITE",
	"CORS_ALLOWED_ORIGINS",
}

// clearConfigEnv removes every config variable inherited from the developer's
// shell or CI runner, so each test starts from a known-empty environment.
//
// Only variables that are actually set are touched. For each one, t.Setenv
// records the original value and registers a cleanup that restores it after
// the test, including when the test fails. os.Unsetenv then removes the
// variable entirely, so it reads as absent (not merely empty) to both
// os.Getenv and os.LookupEnv.
func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, name := range configEnvVars {
		if _, ok := os.LookupEnv(name); !ok {
			continue
		}
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("unset %s: %v", name, err)
		}
	}
}

// setBaseEnv isolates the test from the ambient environment and sets the
// only variable Load() requires.
func setBaseEnv(t *testing.T) {
	t.Helper()
	clearConfigEnv(t)
	t.Setenv("DATABASE_URL", testBaseDB)
}

// namedEnvReaders are os functions that read one variable whose name is an argument.
var namedEnvReaders = map[string]bool{"Getenv": true, "LookupEnv": true}

// bulkEnvReaders are os functions that can read arbitrary variables, so the
// guard cannot tell which names they depend on.
var bulkEnvReaders = map[string]bool{"Environ": true, "ExpandEnv": true}

// envScan accumulates the result of scanning Go source for environment reads.
type envScan struct {
	// names holds variables read via os.Getenv / os.LookupEnv with a string literal.
	names map[string]bool
	// unverifiable holds "file:line:col: reason" for environment access the
	// guard cannot attribute to a specific variable name.
	unverifiable []string
}

// scanEnvReads parses one Go source file and records its environment reads in
// into. src follows parser.ParseFile: nil reads filename from disk.
//
// It walks the syntax tree rather than the raw text, so comments and the
// contents of string literals are never mistaken for code. Imports of "os" are
// resolved per file, which covers aliases (import o "os"); a dot import is
// reported because calls through it cannot be attributed to the os package.
func scanEnvReads(fset *token.FileSet, filename string, src any, into *envScan) error {
	file, err := parser.ParseFile(fset, filename, src, parser.SkipObjectResolution)
	if err != nil {
		return err
	}

	report := func(n ast.Node, format string, args ...any) {
		into.unverifiable = append(into.unverifiable,
			fmt.Sprintf("%s: %s", fset.Position(n.Pos()), fmt.Sprintf(format, args...)))
	}

	// Local names under which this file imports "os".
	osNames := make(map[string]bool)
	for _, imp := range file.Imports {
		if path, err := strconv.Unquote(imp.Path.Value); err != nil || path != "os" {
			continue
		}
		switch {
		case imp.Name == nil:
			osNames["os"] = true
		case imp.Name.Name == ".":
			report(imp, `dot import of "os" hides which calls read the environment; import it by name`)
		case imp.Name.Name == "_":
			// Blank import: declares no identifier, so nothing can be referenced.
		default:
			osNames[imp.Name.Name] = true
		}
	}
	if len(osNames) == 0 {
		return nil
	}

	// Record which selectors are the function being called, so a direct call
	// such as os.Getenv("X") can be told apart from a function value such as
	// f := os.Getenv.
	calls := make(map[*ast.SelectorExpr]*ast.CallExpr)
	ast.Inspect(file, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr); ok {
				calls[sel] = call
			}
		}
		return true
	})

	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || !osNames[pkg.Name] {
			return true
		}
		qualified := "os." + sel.Sel.Name

		switch {
		case bulkEnvReaders[sel.Sel.Name]:
			report(sel, "%s can read arbitrary environment variables; "+
				"read each variable with os.Getenv or os.LookupEnv and a string literal", qualified)

		case namedEnvReaders[sel.Sel.Name]:
			call, isCall := calls[sel]
			if !isCall {
				report(sel, "%s is referenced without being called directly (for example as a function value); "+
					"call it directly with a string literal", qualified)
				return true
			}
			if len(call.Args) != 1 {
				report(call, "%s is called with %d arguments; expected one string literal", qualified, len(call.Args))
				return true
			}
			lit, ok := ast.Unparen(call.Args[0]).(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				report(call, "%s is called with a non-literal name; use a string literal so the guard can verify it", qualified)
				return true
			}
			name, err := strconv.Unquote(lit.Value)
			if err != nil {
				report(lit, "cannot decode string literal %s: %v", lit.Value, err)
				return true
			}
			into.names[name] = true
		}
		return true
	})
	return nil
}

// TestConfigEnvVars_MatchesSource fails when a source file in this package
// reads an environment variable that configEnvVars does not list (the test
// would then leak the developer's shell into results), when the list contains
// a name no longer read anywhere, or when environment access cannot be
// attributed to a specific variable name.
func TestConfigEnvVars_MatchesSource(t *testing.T) {
	// go test runs each package's tests with the package directory as the
	// working directory, so this finds the config package's own files.
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("list source files: %v", err)
	}

	scan := envScan{names: make(map[string]bool)}
	fset := token.NewFileSet()
	scanned := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		if err := scanEnvReads(fset, file, nil, &scan); err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		scanned++
	}
	if scanned == 0 {
		t.Fatal("found no non-test Go files; this test must run from the config package directory")
	}

	for _, problem := range scan.unverifiable {
		t.Errorf("unverifiable environment access at %s", problem)
	}

	if len(scan.names) == 0 && len(scan.unverifiable) == 0 {
		t.Fatal("found no os.Getenv/os.LookupEnv calls in the package source; " +
			"if Load() now reads variables another way, update this guard")
	}

	listed := make(map[string]bool, len(configEnvVars))
	for _, name := range configEnvVars {
		if listed[name] {
			t.Errorf("configEnvVars lists %s more than once", name)
		}
		listed[name] = true
	}

	for _, name := range slices.Sorted(maps.Keys(scan.names)) {
		if !listed[name] {
			t.Errorf("source reads %s but configEnvVars does not list it; "+
				"add it so config tests stay isolated from the shell environment", name)
		}
	}
	for _, name := range configEnvVars {
		if !scan.names[name] {
			t.Errorf("configEnvVars lists %s but no source file reads it; remove it", name)
		}
	}
}

// TestScanEnvReads pins the scanner's behavior on the cases the guard must
// handle, so a regression in the guard itself is caught.
func TestScanEnvReads(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		names []string
		// unverifiable lists one expected substring per expected report, in order.
		unverifiable []string
	}{
		{
			name:  "name only inside a comment is ignored",
			src:   "package p\nimport \"os\"\n// os.Getenv(\"IN_COMMENT\")\nvar _ = os.Args",
			names: nil,
		},
		{
			name:  "name only inside a string literal is ignored",
			src:   "package p\nimport \"os\"\nvar s = `os.Getenv(\"IN_STRING\")`\nvar _ = os.Args",
			names: nil,
		},
		{
			name:  "os.Getenv with a literal",
			src:   "package p\nimport \"os\"\nvar v = os.Getenv(\"A\")",
			names: []string{"A"},
		},
		{
			name:  "os.LookupEnv with a literal",
			src:   "package p\nimport \"os\"\nfunc f() bool { _, ok := os.LookupEnv(\"B\"); return ok }",
			names: []string{"B"},
		},
		{
			name:  "raw string literal name",
			src:   "package p\nimport \"os\"\nvar v = os.Getenv(`RAW`)",
			names: []string{"RAW"},
		},
		{
			name:  "multiline call",
			src:   "package p\nimport \"os\"\nvar v = os.Getenv(\n\t\"SPLIT\",\n)",
			names: []string{"SPLIT"},
		},
		{
			name:  "aliased import",
			src:   "package p\nimport o \"os\"\nvar v = o.Getenv(\"ALIASED\")",
			names: []string{"ALIASED"},
		},
		{
			name:         "non-literal name",
			src:          "package p\nimport \"os\"\nfunc f(k string) string { return os.Getenv(k) }",
			unverifiable: []string{"os.Getenv is called with a non-literal name"},
		},
		{
			name:         "constant name is not a literal",
			src:          "package p\nimport \"os\"\nconst k = \"K\"\nvar v = os.Getenv(k)",
			unverifiable: []string{"os.Getenv is called with a non-literal name"},
		},
		{
			name:         "function value",
			src:          "package p\nimport \"os\"\nvar getenv = os.Getenv\nvar v = getenv(\"HIDDEN\")",
			unverifiable: []string{"os.Getenv is referenced without being called directly"},
		},
		{
			name:         "os.Environ",
			src:          "package p\nimport \"os\"\nvar all = os.Environ()",
			unverifiable: []string{"os.Environ can read arbitrary environment variables"},
		},
		{
			name:         "os.ExpandEnv",
			src:          "package p\nimport \"os\"\nvar v = os.ExpandEnv(\"$HOME\")",
			unverifiable: []string{"os.ExpandEnv can read arbitrary environment variables"},
		},
		{
			name:         "dot import",
			src:          "package p\nimport . \"os\"\nvar v = Getenv(\"DOT\")",
			unverifiable: []string{`dot import of "os"`},
		},
		{
			name:  "blank import has no references",
			src:   "package p\nimport _ \"os\"",
			names: nil,
		},
		{
			name:  "Getenv method on a non-os value is ignored",
			src:   "package p\ntype e struct{}\nfunc (e) Getenv(string) string { return \"\" }\nvar x e\nvar v = x.Getenv(\"NOT_OS\")",
			names: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scan := envScan{names: make(map[string]bool)}
			if err := scanEnvReads(token.NewFileSet(), "probe.go", tc.src, &scan); err != nil {
				t.Fatalf("parse probe source: %v", err)
			}

			gotNames := slices.Sorted(maps.Keys(scan.names))
			if !slices.Equal(gotNames, tc.names) {
				t.Errorf("names = %v, want %v", gotNames, tc.names)
			}

			if len(scan.unverifiable) != len(tc.unverifiable) {
				t.Fatalf("got %d unverifiable reports %q, want %d", len(scan.unverifiable), scan.unverifiable, len(tc.unverifiable))
			}
			for i, want := range tc.unverifiable {
				if !strings.Contains(scan.unverifiable[i], want) {
					t.Errorf("report %d = %q, want it to contain %q", i, scan.unverifiable[i], want)
				}
			}
		})
	}
}

func TestClearConfigEnv_IsolatesAndRestoresAmbientEnvironment(t *testing.T) {
	// Simulate a polluted shell: every variable set to a value Load() would reject
	// or that would change its result.
	for _, name := range configEnvVars {
		t.Setenv(name, "ambient-"+name)
	}

	t.Run("clearConfigEnv removes every variable", func(t *testing.T) {
		clearConfigEnv(t)
		for _, name := range configEnvVars {
			if val, ok := os.LookupEnv(name); ok {
				t.Errorf("%s still set to %q after clearConfigEnv", name, val)
			}
		}
	})

	t.Run("setBaseEnv yields default configuration", func(t *testing.T) {
		setBaseEnv(t)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() with isolated environment: unexpected error: %v", err)
		}
		if cfg.AppEnv != EnvDevelopment {
			t.Errorf("AppEnv = %q, want %q", cfg.AppEnv, EnvDevelopment)
		}
		if cfg.DatabaseURL != testBaseDB {
			t.Errorf("DatabaseURL = %q, want test base URL", cfg.DatabaseURL)
		}
	})

	// The subtests' cleanups have run; the ambient values must be back.
	for _, name := range configEnvVars {
		if got := os.Getenv(name); got != "ambient-"+name {
			t.Errorf("%s not restored after subtest: got %q, want %q", name, got, "ambient-"+name)
		}
	}
}

// testDeployedOrigins is a valid CORS allowlist for staging and production tests.
const testDeployedOrigins = "https://app.gostock.example"

// deployedTestEnvs lists the environments that require an explicit JWT_SECRET
// and CORS_ALLOWED_ORIGINS. It is spelled out here rather than calling
// isDeployed so that test setup does not depend on the code under test.
var deployedTestEnvs = map[string]bool{"staging": true, "production": true, "prod": true}

// setIfUnset sets name only when it is not already set. Table-driven tests use
// it to supply settings that staging and production require, without
// overriding a value a test case configured on purpose.
func setIfUnset(t *testing.T, name, value string) {
	t.Helper()
	if _, ok := os.LookupEnv(name); !ok {
		t.Setenv(name, value)
	}
}

func TestConfig_EnvironmentResolution(t *testing.T) {
	tests := []struct {
		name           string
		appEnvVal      string
		setAppEnv      bool
		envVal         string
		setEnv         bool
		wantAppEnv     string
		wantProduction bool
		wantError      bool
		errContains    string
	}{
		{
			name:           "APP_ENV=production",
			appEnvVal:      "production",
			setAppEnv:      true,
			wantAppEnv:     "production",
			wantProduction: true,
			wantError:      false,
		},
		{
			name:           "APP_ENV=prod",
			appEnvVal:      "prod",
			setAppEnv:      true,
			wantAppEnv:     "prod",
			wantProduction: true,
			wantError:      false,
		},
		{
			name:           "APP_ENV=development",
			appEnvVal:      "development",
			setAppEnv:      true,
			wantAppEnv:     "development",
			wantProduction: false,
			wantError:      false,
		},
		{
			name:           "APP_ENV=dev",
			appEnvVal:      "dev",
			setAppEnv:      true,
			wantAppEnv:     "dev",
			wantProduction: false,
			wantError:      false,
		},
		{
			name:           "APP_ENV=test",
			appEnvVal:      "test",
			setAppEnv:      true,
			wantAppEnv:     "test",
			wantProduction: false,
			wantError:      false,
		},
		{
			name:           "APP_ENV=staging",
			appEnvVal:      "staging",
			setAppEnv:      true,
			wantAppEnv:     "staging",
			wantProduction: false,
			wantError:      false,
		},
		{
			name:           "APP_ENV unset defaults to development",
			setAppEnv:      false,
			setEnv:         false,
			wantAppEnv:     "development",
			wantProduction: false,
			wantError:      false,
		},
		{
			name:           "ENV=prod when APP_ENV unset",
			setAppEnv:      false,
			envVal:         "prod",
			setEnv:         true,
			wantAppEnv:     "prod",
			wantProduction: true,
			wantError:      false,
		},
		{
			name:           "ENV=production when APP_ENV unset",
			setAppEnv:      false,
			envVal:         "production",
			setEnv:         true,
			wantAppEnv:     "production",
			wantProduction: true,
			wantError:      false,
		},
		{
			name:           "APP_ENV takes precedence over ENV",
			appEnvVal:      "prod",
			setAppEnv:      true,
			envVal:         "development",
			setEnv:         true,
			wantAppEnv:     "prod",
			wantProduction: true,
			wantError:      false,
		},
		{
			name:           "APP_ENV takes precedence even if ENV is production",
			appEnvVal:      "test",
			setAppEnv:      true,
			envVal:         "production",
			setEnv:         true,
			wantAppEnv:     "test",
			wantProduction: false,
			wantError:      false,
		},
		{
			name:        "Unknown environment in APP_ENV rejected",
			appEnvVal:   "unknown-environment",
			setAppEnv:   true,
			wantError:   true,
			errContains: "unsupported environment",
		},
		{
			name:        "Unknown environment in ENV rejected when APP_ENV unset",
			setAppEnv:   false,
			envVal:      "custom-env",
			setEnv:      true,
			wantError:   true,
			errContains: "unsupported environment",
		},
		{
			name:           "Whitespace and mixed-case in APP_ENV normalized",
			appEnvVal:      "  PRODUCTION  ",
			setAppEnv:      true,
			wantAppEnv:     "production",
			wantProduction: true,
			wantError:      false,
		},
		{
			name:           "Whitespace and mixed-case in ENV normalized",
			setAppEnv:      false,
			envVal:         "  DeV  ",
			setEnv:         true,
			wantAppEnv:     "dev",
			wantProduction: false,
			wantError:      false,
		},
		{
			name:           "Whitespace-only APP_ENV falls back to ENV",
			appEnvVal:      "   ",
			setAppEnv:      true,
			envVal:         "staging",
			setEnv:         true,
			wantAppEnv:     "staging",
			wantProduction: false,
			wantError:      false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setBaseEnv(t)
			if tc.setAppEnv {
				t.Setenv("APP_ENV", tc.appEnvVal)
			}
			if tc.setEnv {
				t.Setenv("ENV", tc.envVal)
			}
			if deployedTestEnvs[tc.wantAppEnv] {
				setIfUnset(t, "JWT_SECRET", testProdSecret)
				setIfUnset(t, "CORS_ALLOWED_ORIGINS", testDeployedOrigins)
			}

			cfg, err := Load()
			if tc.wantError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tc.errContains)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if cfg.AppEnv != tc.wantAppEnv {
					t.Errorf("AppEnv = %q, want %q", cfg.AppEnv, tc.wantAppEnv)
				}
				if IsProduction(cfg) != tc.wantProduction {
					t.Errorf("IsProduction = %v, want %v", IsProduction(cfg), tc.wantProduction)
				}
			}
		})
	}
}

func TestConfig_JWTSecretPolicy(t *testing.T) {
	tests := []struct {
		name        string
		appEnv      string
		jwtSecret   string
		setSecret   bool
		wantSecret  string
		wantError   bool
		errContains string
	}{
		{
			name:        "Missing secret in production (APP_ENV=production)",
			appEnv:      "production",
			setSecret:   false,
			wantError:   true,
			errContains: "JWT_SECRET environment variable is required in production",
		},
		{
			name:        "Missing secret in production (APP_ENV=prod)",
			appEnv:      "prod",
			setSecret:   false,
			wantError:   true,
			errContains: "JWT_SECRET environment variable is required in production",
		},
		{
			name:        "Empty secret in production",
			appEnv:      "production",
			jwtSecret:   "",
			setSecret:   true,
			wantError:   true,
			errContains: "JWT_SECRET environment variable is required in production",
		},
		{
			name:        "Secret shorter than 32 bytes in production",
			appEnv:      "production",
			jwtSecret:   "too-short-secret",
			setSecret:   true,
			wantError:   true,
			errContains: "must be at least 32 bytes",
		},
		{
			name:        "Secret shorter than 32 bytes in development",
			appEnv:      "development",
			jwtSecret:   "too-short",
			setSecret:   true,
			wantError:   true,
			errContains: "must be at least 32 bytes",
		},
		{
			name:        "Default development secret in production (APP_ENV=production) rejected",
			appEnv:      "production",
			jwtSecret:   defaultDevSecret,
			setSecret:   true,
			wantError:   true,
			errContains: "cannot use default development secret in production",
		},
		{
			name:        "Default development secret in production (APP_ENV=prod) rejected",
			appEnv:      "prod",
			jwtSecret:   defaultDevSecret,
			setSecret:   true,
			wantError:   true,
			errContains: "cannot use default development secret in production",
		},
		{
			name:        "Default development secret with leading whitespace in production rejected",
			appEnv:      "production",
			jwtSecret:   "  " + defaultDevSecret,
			setSecret:   true,
			wantError:   true,
			errContains: "cannot use default development secret in production",
		},
		{
			name:        "Default development secret with trailing whitespace in production rejected",
			appEnv:      "production",
			jwtSecret:   defaultDevSecret + "  ",
			setSecret:   true,
			wantError:   true,
			errContains: "cannot use default development secret in production",
		},
		{
			name:        "Default development secret with leading and trailing whitespace in production rejected",
			appEnv:      "production",
			jwtSecret:   "  " + defaultDevSecret + "  ",
			setSecret:   true,
			wantError:   true,
			errContains: "cannot use default development secret in production",
		},
		{
			name:        "Default development secret with whitespace in prod rejected",
			appEnv:      "prod",
			jwtSecret:   "  " + defaultDevSecret + "  ",
			setSecret:   true,
			wantError:   true,
			errContains: "cannot use default development secret in production",
		},
		{
			name:        "Production secret with leading whitespace rejected",
			appEnv:      "production",
			jwtSecret:   "  " + testProdSecret,
			setSecret:   true,
			wantError:   true,
			errContains: "JWT_SECRET cannot contain leading or trailing whitespace in production",
		},
		{
			name:        "Production secret with trailing whitespace rejected",
			appEnv:      "production",
			jwtSecret:   testProdSecret + "  ",
			setSecret:   true,
			wantError:   true,
			errContains: "JWT_SECRET cannot contain leading or trailing whitespace in production",
		},
		{
			name:        "Production secret with leading and trailing whitespace rejected",
			appEnv:      "production",
			jwtSecret:   "  " + testProdSecret + "  ",
			setSecret:   true,
			wantError:   true,
			errContains: "JWT_SECRET cannot contain leading or trailing whitespace in production",
		},
		{
			name:        "Prod secret with leading and trailing whitespace rejected",
			appEnv:      "prod",
			jwtSecret:   "  " + testProdSecret + "  ",
			setSecret:   true,
			wantError:   true,
			errContains: "JWT_SECRET cannot contain leading or trailing whitespace in production",
		},
		{
			name:       "Valid production secret accepted (APP_ENV=production)",
			appEnv:     "production",
			jwtSecret:  testProdSecret,
			setSecret:  true,
			wantSecret: testProdSecret,
			wantError:  false,
		},
		{
			name:       "Valid production secret accepted (APP_ENV=prod)",
			appEnv:     "prod",
			jwtSecret:  testProdSecret,
			setSecret:  true,
			wantSecret: testProdSecret,
			wantError:  false,
		},
		{
			name:       "Development fallback to defaultDevSecret when unset",
			appEnv:     "development",
			setSecret:  false,
			wantSecret: defaultDevSecret,
			wantError:  false,
		},
		{
			name:       "Dev fallback to defaultDevSecret when unset",
			appEnv:     "dev",
			setSecret:  false,
			wantSecret: defaultDevSecret,
			wantError:  false,
		},
		{
			name:       "Test environment fallback to defaultDevSecret when unset",
			appEnv:     "test",
			setSecret:  false,
			wantSecret: defaultDevSecret,
			wantError:  false,
		},
		{
			name:        "Staging requires an explicit secret (no development fallback)",
			appEnv:      "staging",
			setSecret:   false,
			wantError:   true,
			errContains: "JWT_SECRET environment variable is required in staging",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setBaseEnv(t)
			if tc.appEnv != "" {
				t.Setenv("APP_ENV", tc.appEnv)
			}
			if tc.setSecret {
				t.Setenv("JWT_SECRET", tc.jwtSecret)
			}
			if deployedTestEnvs[tc.appEnv] {
				setIfUnset(t, "CORS_ALLOWED_ORIGINS", testDeployedOrigins)
			}

			cfg, err := Load()
			if tc.wantError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tc.errContains)
				}
				// Verify secret is not leaked in error message
				if tc.jwtSecret != "" && tc.jwtSecret != defaultDevSecret && strings.Contains(err.Error(), tc.jwtSecret) {
					t.Errorf("error message must NOT contain secret: %q", err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if cfg.JWTSecret != tc.wantSecret {
					t.Errorf("JWTSecret = %q, want %q", cfg.JWTSecret, tc.wantSecret)
				}
			}
		})
	}
}

func TestConfig_CookieSecurityPolicy(t *testing.T) {
	tests := []struct {
		name         string
		appEnv       string
		cookieSecure string
		setSecure    bool
		sameSite     string
		setSameSite  bool
		wantSecure   bool
		wantSameSite http.SameSite
		wantError    bool
		errContains  string
	}{
		{
			name:         "Production Secure=true",
			appEnv:       "production",
			cookieSecure: "true",
			setSecure:    true,
			wantSecure:   true,
			wantSameSite: http.SameSiteLaxMode,
			wantError:    false,
		},
		{
			name:         "Prod Secure=true",
			appEnv:       "prod",
			cookieSecure: "true",
			setSecure:    true,
			wantSecure:   true,
			wantSameSite: http.SameSiteLaxMode,
			wantError:    false,
		},
		{
			name:         "Production Secure=false rejected",
			appEnv:       "production",
			cookieSecure: "false",
			setSecure:    true,
			wantError:    true,
			errContains:  "COOKIE_SECURE cannot be false in production",
		},
		{
			name:         "Prod Secure=false rejected",
			appEnv:       "prod",
			cookieSecure: "false",
			setSecure:    true,
			wantError:    true,
			errContains:  "COOKIE_SECURE cannot be false in production",
		},
		{
			name:         "Production with COOKIE_SECURE unset defaults to true",
			appEnv:       "production",
			setSecure:    false,
			wantSecure:   true,
			wantSameSite: http.SameSiteLaxMode,
			wantError:    false,
		},
		{
			name:         "Prod with COOKIE_SECURE unset defaults to true",
			appEnv:       "prod",
			setSecure:    false,
			wantSecure:   true,
			wantSameSite: http.SameSiteLaxMode,
			wantError:    false,
		},
		{
			name:         "Development Secure=false allowed",
			appEnv:       "development",
			cookieSecure: "false",
			setSecure:    true,
			wantSecure:   false,
			wantSameSite: http.SameSiteLaxMode,
			wantError:    false,
		},
		{
			name:         "Development with COOKIE_SECURE unset defaults to false",
			appEnv:       "development",
			setSecure:    false,
			wantSecure:   false,
			wantSameSite: http.SameSiteLaxMode,
			wantError:    false,
		},
		{
			name:         "SameSite=None with Secure=true succeeds in production",
			appEnv:       "production",
			cookieSecure: "true",
			setSecure:    true,
			sameSite:     "none",
			setSameSite:  true,
			wantSecure:   true,
			wantSameSite: http.SameSiteNoneMode,
			wantError:    false,
		},
		{
			name:         "SameSite=None with Secure=true succeeds in development",
			appEnv:       "development",
			cookieSecure: "true",
			setSecure:    true,
			sameSite:     "none",
			setSameSite:  true,
			wantSecure:   true,
			wantSameSite: http.SameSiteNoneMode,
			wantError:    false,
		},
		{
			name:         "SameSite=None with Secure=false rejected in development",
			appEnv:       "development",
			cookieSecure: "false",
			setSecure:    true,
			sameSite:     "none",
			setSameSite:  true,
			wantError:    true,
			errContains:  "COOKIE_SAMESITE=none requires COOKIE_SECURE=true",
		},
		{
			name:        "SameSite=None with COOKIE_SECURE unset in development rejected",
			appEnv:      "development",
			setSecure:   false, // defaults to false
			sameSite:    "none",
			setSameSite: true,
			wantError:   true,
			errContains: "COOKIE_SAMESITE=none requires COOKIE_SECURE=true",
		},
		{
			name:         "SameSite=None with COOKIE_SECURE unset in production succeeds (defaults to Secure=true)",
			appEnv:       "production",
			setSecure:    false, // defaults to true in production
			sameSite:     "none",
			setSameSite:  true,
			wantSecure:   true,
			wantSameSite: http.SameSiteNoneMode,
			wantError:    false,
		},
		{
			name:         "SameSite=Lax",
			appEnv:       "development",
			sameSite:     "lax",
			setSameSite:  true,
			wantSecure:   false,
			wantSameSite: http.SameSiteLaxMode,
			wantError:    false,
		},
		{
			name:         "SameSite=Strict",
			appEnv:       "development",
			sameSite:     "strict",
			setSameSite:  true,
			wantSecure:   false,
			wantSameSite: http.SameSiteStrictMode,
			wantError:    false,
		},
		{
			name:        "Invalid SameSite value rejected",
			appEnv:      "development",
			sameSite:    "invalid-samesite",
			setSameSite: true,
			wantError:   true,
			errContains: "COOKIE_SAMESITE must be one of 'lax', 'strict', or 'none'",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setBaseEnv(t)
			if tc.appEnv != "" {
				t.Setenv("APP_ENV", tc.appEnv)
			}
			if deployedTestEnvs[tc.appEnv] {
				setIfUnset(t, "JWT_SECRET", testProdSecret)
				setIfUnset(t, "CORS_ALLOWED_ORIGINS", testDeployedOrigins)
			}
			if tc.setSecure {
				t.Setenv("COOKIE_SECURE", tc.cookieSecure)
			}
			if tc.setSameSite {
				t.Setenv("COOKIE_SAMESITE", tc.sameSite)
			}

			cfg, err := Load()
			if tc.wantError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tc.errContains)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if cfg.CookieSecure != tc.wantSecure {
					t.Errorf("CookieSecure = %v, want %v", cfg.CookieSecure, tc.wantSecure)
				}
				if cfg.CookieSameSite != tc.wantSameSite {
					t.Errorf("CookieSameSite = %v, want %v", cfg.CookieSameSite, tc.wantSameSite)
				}
			}
		})
	}
}

func TestConfig_HTTPPortValidation(t *testing.T) {
	tests := []struct {
		name      string
		portEnv   string
		wantPort  string
		wantError bool
	}{
		{name: "default port when empty", portEnv: "", wantPort: "8080", wantError: false},
		{name: "valid port 80", portEnv: "80", wantPort: "80", wantError: false},
		{name: "valid port 8080", portEnv: "8080", wantPort: "8080", wantError: false},
		{name: "valid port min 1", portEnv: "1", wantPort: "1", wantError: false},
		{name: "valid port max 65535", portEnv: "65535", wantPort: "65535", wantError: false},
		{name: "invalid zero port", portEnv: "0", wantError: true},
		{name: "invalid negative port", portEnv: "-80", wantError: true},
		{name: "invalid out-of-range port 65536", portEnv: "65536", wantError: true},
		{name: "invalid out-of-range port 99999", portEnv: "99999", wantError: true},
		{name: "invalid non-numeric port", portEnv: "http", wantError: true},
		{name: "invalid mixed alphanumeric port", portEnv: "8080abc", wantError: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setBaseEnv(t)
			if tc.portEnv != "" {
				t.Setenv("HTTP_PORT", tc.portEnv)
			}

			cfg, err := Load()
			if tc.wantError {
				if err == nil {
					t.Fatalf("expected error for HTTP_PORT=%q, got nil", tc.portEnv)
				}
				if !strings.Contains(err.Error(), "HTTP_PORT") {
					t.Errorf("error %q should mention HTTP_PORT", err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if cfg.HTTPPort != tc.wantPort {
					t.Errorf("HTTPPort = %q, want %q", cfg.HTTPPort, tc.wantPort)
				}
			}
		})
	}
}

func TestConfig_TokenTTLValidation(t *testing.T) {
	t.Run("default token TTLs", func(t *testing.T) {
		setBaseEnv(t)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.AccessTokenTTL != 15*time.Minute {
			t.Errorf("AccessTokenTTL = %v, want 15m", cfg.AccessTokenTTL)
		}
		if cfg.RefreshTokenTTL != 7*24*time.Hour {
			t.Errorf("RefreshTokenTTL = %v, want 7d", cfg.RefreshTokenTTL)
		}
	})

	t.Run("valid custom token TTLs", func(t *testing.T) {
		setBaseEnv(t)
		t.Setenv("ACCESS_TOKEN_TTL_MINUTES", "30")
		t.Setenv("REFRESH_TOKEN_TTL_DAYS", "14")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.AccessTokenTTL != 30*time.Minute {
			t.Errorf("AccessTokenTTL = %v, want 30m", cfg.AccessTokenTTL)
		}
		if cfg.RefreshTokenTTL != 14*24*time.Hour {
			t.Errorf("RefreshTokenTTL = %v, want 14d", cfg.RefreshTokenTTL)
		}
	})

	t.Run("invalid negative or zero access token TTL", func(t *testing.T) {
		setBaseEnv(t)
		t.Setenv("ACCESS_TOKEN_TTL_MINUTES", "-1")
		_, err := Load()
		if err == nil {
			t.Fatal("expected error for negative access token TTL, got nil")
		}

		t.Setenv("ACCESS_TOKEN_TTL_MINUTES", "0")
		_, err2 := Load()
		if err2 == nil {
			t.Fatal("expected error for zero access token TTL, got nil")
		}
	})

	t.Run("invalid negative or zero refresh token TTL", func(t *testing.T) {
		setBaseEnv(t)
		t.Setenv("REFRESH_TOKEN_TTL_DAYS", "-5")
		_, err := Load()
		if err == nil {
			t.Fatal("expected error for negative refresh token TTL, got nil")
		}

		t.Setenv("REFRESH_TOKEN_TTL_DAYS", "0")
		_, err2 := Load()
		if err2 == nil {
			t.Fatal("expected error for zero refresh token TTL, got nil")
		}
	})
}

func TestConfig_GracePeriodParsing(t *testing.T) {
	t.Run("default grace period", func(t *testing.T) {
		setBaseEnv(t)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.RefreshTokenGracePeriod != 15*time.Second {
			t.Errorf("RefreshTokenGracePeriod = %v, want 15s", cfg.RefreshTokenGracePeriod)
		}
	})

	t.Run("parsed as duration string", func(t *testing.T) {
		setBaseEnv(t)
		t.Setenv("REFRESH_TOKEN_GRACE_PERIOD", "30s")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.RefreshTokenGracePeriod != 30*time.Second {
			t.Errorf("RefreshTokenGracePeriod = %v, want 30s", cfg.RefreshTokenGracePeriod)
		}
	})

	t.Run("parsed as integer seconds", func(t *testing.T) {
		setBaseEnv(t)
		t.Setenv("REFRESH_TOKEN_GRACE_PERIOD", "45")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.RefreshTokenGracePeriod != 45*time.Second {
			t.Errorf("RefreshTokenGracePeriod = %v, want 45s", cfg.RefreshTokenGracePeriod)
		}
	})

	t.Run("invalid string rejected", func(t *testing.T) {
		setBaseEnv(t)
		t.Setenv("REFRESH_TOKEN_GRACE_PERIOD", "invalid-duration")
		_, err := Load()
		if err == nil {
			t.Fatal("expected error for invalid grace period, got nil")
		}
	})
}

func TestConfig_MissingDatabaseURL(t *testing.T) {
	// Clear DATABASE_URL even if the shell sets it for integration tests.
	clearConfigEnv(t)
	_, err := Load()
	if err == nil {
		t.Fatal("expected error when DATABASE_URL is missing, got nil")
	}
}

// TestConfig_JWTSecretByEnvironment checks every secret shape in every
// environment. Development and test rows pin today's behavior unchanged;
// staging and production rows pin the deployed-environment rules.
func TestConfig_JWTSecretByEnvironment(t *testing.T) {
	secret32 := "leak-canary-" + strings.Repeat("Q7", 10) // exactly 32 bytes
	secret31 := secret32[:31]

	const (
		required   = "JWT_SECRET environment variable is required in {env}"
		tooShort   = "JWT_SECRET must be at least 32 bytes"
		whitespace = "JWT_SECRET cannot contain leading or trailing whitespace in {env}"
		devSecret  = "cannot use default development secret in {env}"
	)

	type outcome struct {
		wantSecret  string // expected AppConfig.JWTSecret on success
		errContains string // expected error substring; "{env}" is replaced by the environment label
	}

	cases := []struct {
		name     string
		value    string
		set      bool
		dev      outcome // development, dev, test
		deployed outcome // staging, production, prod
	}{
		{"unset", "", false, outcome{wantSecret: defaultDevSecret}, outcome{errContains: required}},
		{"empty", "", true, outcome{wantSecret: defaultDevSecret}, outcome{errContains: required}},
		{"whitespace only", "   ", true, outcome{errContains: tooShort}, outcome{errContains: required}},
		{"31 bytes", secret31, true, outcome{errContains: tooShort}, outcome{errContains: tooShort}},
		{"exactly 32 bytes", secret32, true, outcome{wantSecret: secret32}, outcome{wantSecret: secret32}},
		{"leading space", " " + secret32, true, outcome{wantSecret: " " + secret32}, outcome{errContains: whitespace}},
		{"trailing newline", secret32 + "\n", true, outcome{wantSecret: secret32 + "\n"}, outcome{errContains: whitespace}},
		{"leading tab", "\t" + secret32, true, outcome{wantSecret: "\t" + secret32}, outcome{errContains: whitespace}},
		{"development secret", defaultDevSecret, true, outcome{wantSecret: defaultDevSecret}, outcome{errContains: devSecret}},
		{"padded development secret", "  " + defaultDevSecret + "  ", true, outcome{wantSecret: "  " + defaultDevSecret + "  "}, outcome{errContains: devSecret}},
		{"valid", testProdSecret, true, outcome{wantSecret: testProdSecret}, outcome{wantSecret: testProdSecret}},
	}

	envs := []struct {
		appEnv string
		label  string
	}{
		{"development", "development"},
		{"dev", "dev"},
		{"test", "test"},
		{"staging", "staging"},
		{"production", "production"},
		{"prod", "production"},
	}

	for _, env := range envs {
		for _, tc := range cases {
			t.Run(env.appEnv+"/"+tc.name, func(t *testing.T) {
				setBaseEnv(t)
				t.Setenv("APP_ENV", env.appEnv)
				if tc.set {
					t.Setenv("JWT_SECRET", tc.value)
				}
				want := tc.dev
				if deployedTestEnvs[env.appEnv] {
					want = tc.deployed
					setIfUnset(t, "CORS_ALLOWED_ORIGINS", testDeployedOrigins)
				}

				cfg, err := Load()

				if want.errContains != "" {
					if err == nil {
						t.Fatalf("expected error containing %q, got nil", want.errContains)
					}
					wantMsg := strings.ReplaceAll(want.errContains, "{env}", env.label)
					if !strings.Contains(err.Error(), wantMsg) {
						t.Errorf("error %q should contain %q", err.Error(), wantMsg)
					}
					if want.errContains == required && !strings.Contains(err.Error(), "openssl rand -base64 48") {
						t.Errorf("missing-secret error should recommend openssl rand -base64 48, got %q", err.Error())
					}
					if secret := strings.TrimSpace(tc.value); secret != "" && strings.Contains(err.Error(), secret) {
						t.Errorf("error message must not contain the secret value: %q", err.Error())
					}
					return
				}
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if cfg.JWTSecret != want.wantSecret {
					t.Errorf("JWTSecret = %q, want %q", cfg.JWTSecret, want.wantSecret)
				}
			})
		}
	}
}

func TestConfig_CORSAllowedOrigins(t *testing.T) {
	tests := []struct {
		name        string
		appEnv      string // APP_ENV value; empty leaves it unset (development)
		env         string // ENV value, used when APP_ENV is unset
		origins     string
		setOrigins  bool
		want        []string
		errContains []string
	}{
		// Development and test: localhost defaults and http are allowed.
		{name: "development unset uses localhost defaults", want: []string{"http://localhost:5173", "http://localhost:3000"}},
		{name: "test unset uses localhost defaults", appEnv: "test", want: []string{"http://localhost:5173", "http://localhost:3000"}},
		{name: "development explicit http origin", origins: "http://localhost:8081", setOrigins: true, want: []string{"http://localhost:8081"}},
		{
			name:       "entries are normalized",
			origins:    "HTTPS://App.Example.com:443/, http://LOCALHOST:3000",
			setOrigins: true,
			want:       []string{"https://app.example.com", "http://localhost:3000"},
		},
		{
			name:       "duplicates removed after normalization, order preserved",
			origins:    "https://b.example.com, https://a.example.com/, HTTPS://B.EXAMPLE.COM:443",
			setOrigins: true,
			want:       []string{"https://b.example.com", "https://a.example.com"},
		},
		{name: "empty segments skipped", origins: ",https://a.example.com,, ,", setOrigins: true, want: []string{"https://a.example.com"}},
		{name: "commas only rejected", origins: " , , ", setOrigins: true, errContains: []string{"CORS_ALLOWED_ORIGINS is set but contains no origins"}},
		{name: "whitespace only rejected", origins: "   ", setOrigins: true, errContains: []string{"contains no origins"}},
		{name: "development explicitly empty rejected (no fallback to defaults)", origins: "", setOrigins: true, errContains: []string{"CORS_ALLOWED_ORIGINS is set but contains no origins"}},
		{name: "wildcard rejected in development", origins: "*", setOrigins: true, errContains: []string{`entry "*" is invalid`, "wildcards"}},
		{
			name:        "one invalid entry fails the whole list in development",
			origins:     "http://localhost:5173,https://x.example.com/path",
			setOrigins:  true,
			errContains: []string{`entry "https://x.example.com/path" is invalid`, "path"},
		},

		// Staging: explicit https allowlist, no loopback.
		{name: "staging unset rejected", appEnv: "staging", errContains: []string{"CORS_ALLOWED_ORIGINS is required in staging"}},
		{name: "staging http rejected", appEnv: "staging", origins: "http://app.example.com", setOrigins: true, errContains: []string{"not allowed in staging", "must use https"}},
		{name: "staging localhost rejected", appEnv: "staging", origins: "https://localhost", setOrigins: true, errContains: []string{"not allowed in staging", "localhost hosts"}},
		{name: "staging loopback IPv4 rejected", appEnv: "staging", origins: "https://127.0.0.1:8443", setOrigins: true, errContains: []string{"loopback addresses"}},
		{name: "staging unspecified rejected", appEnv: "staging", origins: "https://0.0.0.0", setOrigins: true, errContains: []string{"unspecified addresses"}},
		{name: "staging valid https", appEnv: "staging", origins: "https://staging.example.com", setOrigins: true, want: []string{"https://staging.example.com"}},

		// Production: explicit https allowlist, no loopback or unspecified hosts.
		{name: "production unset rejected", appEnv: "production", errContains: []string{"CORS_ALLOWED_ORIGINS is required in production"}},
		{name: "prod alias unset rejected", appEnv: "prod", errContains: []string{"CORS_ALLOWED_ORIGINS is required in production"}},
		{name: "production commas only rejected", appEnv: "production", origins: ",", setOrigins: true, errContains: []string{"contains no origins"}},
		{name: "production explicitly empty rejected", appEnv: "production", origins: "", setOrigins: true, errContains: []string{"CORS_ALLOWED_ORIGINS is set but contains no origins"}},
		{
			name:        "production http rejected even alongside https",
			appEnv:      "production",
			origins:     "https://app.example.com,http://app.example.com",
			setOrigins:  true,
			errContains: []string{`entry "http://app.example.com" is not allowed in production`, "must use https"},
		},
		{name: "production localhost rejected", appEnv: "production", origins: "https://localhost", setOrigins: true, errContains: []string{"localhost hosts"}},
		{name: "production .localhost subdomain rejected", appEnv: "production", origins: "https://api.localhost", setOrigins: true, errContains: []string{"localhost hosts"}},
		{name: "production IPv4 loopback rejected", appEnv: "production", origins: "https://127.1.2.3", setOrigins: true, errContains: []string{"loopback addresses"}},
		{name: "production IPv6 loopback rejected", appEnv: "production", origins: "https://[::1]", setOrigins: true, errContains: []string{"loopback addresses"}},
		{name: "production 0.0.0.0 rejected", appEnv: "production", origins: "https://0.0.0.0", setOrigins: true, errContains: []string{"unspecified addresses"}},
		{name: "production [::] rejected", appEnv: "production", origins: "https://[::]", setOrigins: true, errContains: []string{"unspecified addresses"}},
		{name: "production invalid entry rejected", appEnv: "production", origins: "https://app.example.com,https://*.example.com", setOrigins: true, errContains: []string{"wildcards"}},
		{name: "ENV=prod alias applies production rules", env: "prod", origins: "http://app.example.com", setOrigins: true, errContains: []string{"not allowed in production", "must use https"}},
		{
			name:       "production valid list normalized",
			appEnv:     "production",
			origins:    "https://App.Example.com/, https://admin.example.com:8443, https://localhost.example.com",
			setOrigins: true,
			want:       []string{"https://app.example.com", "https://admin.example.com:8443", "https://localhost.example.com"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setBaseEnv(t)
			if tc.appEnv != "" {
				t.Setenv("APP_ENV", tc.appEnv)
			}
			if tc.env != "" {
				t.Setenv("ENV", tc.env)
			}
			if deployedTestEnvs[tc.appEnv] || deployedTestEnvs[tc.env] {
				setIfUnset(t, "JWT_SECRET", testProdSecret)
			}
			if tc.setOrigins {
				t.Setenv("CORS_ALLOWED_ORIGINS", tc.origins)
			}

			cfg, err := Load()

			if len(tc.errContains) > 0 {
				if err == nil {
					t.Fatalf("expected error, got nil (origins: %v)", cfg.AllowedOrigins)
				}
				for _, want := range tc.errContains {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q should contain %q", err.Error(), want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !slices.Equal(cfg.AllowedOrigins, tc.want) {
				t.Errorf("AllowedOrigins = %q, want %q", cfg.AllowedOrigins, tc.want)
			}
		})
	}
}

func TestLoadDatabaseURL(t *testing.T) {
	t.Run("missing DATABASE_URL rejected", func(t *testing.T) {
		clearConfigEnv(t)
		_, err := LoadDatabaseURL()
		if err == nil || !strings.Contains(err.Error(), "DATABASE_URL environment variable is required") {
			t.Fatalf("expected missing DATABASE_URL error, got %v", err)
		}
	})

	t.Run("returns DATABASE_URL", func(t *testing.T) {
		setBaseEnv(t)
		got, err := LoadDatabaseURL()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != testBaseDB {
			t.Errorf("LoadDatabaseURL() = %q, want %q", got, testBaseDB)
		}
	})

	// Only DATABASE_URL is set: the server's Load must still fail in deployed
	// environments, while LoadDatabaseURL (used by cmd/createadmin) succeeds.
	for _, env := range []string{"staging", "production"} {
		t.Run("does not require server settings in "+env, func(t *testing.T) {
			setBaseEnv(t)
			t.Setenv("APP_ENV", env)

			got, err := LoadDatabaseURL()
			if err != nil {
				t.Fatalf("LoadDatabaseURL: unexpected error: %v", err)
			}
			if got != testBaseDB {
				t.Errorf("LoadDatabaseURL() = %q, want %q", got, testBaseDB)
			}

			if _, err := Load(); err == nil {
				t.Fatalf("Load() in %s without JWT_SECRET and CORS_ALLOWED_ORIGINS: expected error, got nil", env)
			}
		})
	}
}
