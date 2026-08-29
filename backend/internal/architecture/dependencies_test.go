package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "event-hunter/backend"

func TestContextDependencyDirection(t *testing.T) {
	root := moduleRoot(t)
	contextsRoot := filepath.Join(root, "internal", "contexts")
	err := filepath.WalkDir(contextsRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(contextsRoot, path)
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(relative), "/")
		if len(parts) == 2 {
			t.Errorf("%s is a flat context-root source file; place behavior in domain, application, ports, or adapters", relative)
		}
		layer := contextLayer(parts)
		if layer == "" {
			return nil
		}
		for _, imported := range importsOf(t, path) {
			if reason := forbiddenImport(layer, imported); reason != "" {
				t.Errorf("%s imports %q: %s", relative, imported, reason)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCanonicalApplicationPackageMap(t *testing.T) {
	root := moduleRoot(t)
	assertApplicationDirectories(t, filepath.Join(root, "internal", "contexts", "eventcheck", "application"), map[string]bool{
		"internal": true,
	})
	assertApplicationDirectories(t, filepath.Join(root, "internal", "contexts", "investigation", "application"), map[string]bool{
		"alerts": true, "cases": true, "compatibility": true,
		"operations": true, "savedsearch": true, "search": true,
	})
}

func assertApplicationDirectories(t *testing.T, root string, expected map[string]bool) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	found := make(map[string]bool, len(expected))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if !containsProductionGo(t, filepath.Join(root, entry.Name())) {
			continue
		}
		if !expected[entry.Name()] {
			t.Errorf("%s contains unexpected application package %q; group use cases by stable business capability", root, entry.Name())
			continue
		}
		found[entry.Name()] = true
	}
	for name := range expected {
		if !found[name] {
			t.Errorf("%s is missing canonical application package %q", root, name)
		}
	}
}

func containsProductionGo(t *testing.T, root string) bool {
	t.Helper()
	found := false
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func TestCommandsContainNoPersistenceQueries(t *testing.T) {
	root := moduleRoot(t)
	commandsRoot := filepath.Join(root, "cmd")
	markers := []string{
		"INSERT INTO ", "SELECT ", "UPDATE ", "DELETE FROM ", "CREATE TABLE ",
		"FORMAT JSONEachRow", "QueryContext(", "QueryRowContext(", "ExecContext(",
	}
	err := filepath.WalkDir(commandsRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, marker := range markers {
			if strings.Contains(string(content), marker) {
				relative, _ := filepath.Rel(root, path)
				t.Errorf("%s contains persistence detail %q; move it to an outbound adapter", relative, marker)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func contextLayer(parts []string) string {
	for _, part := range parts {
		switch part {
		case "domain", "application", "ports":
			return part
		case "adapters":
			return ""
		}
	}
	return ""
}

func forbiddenImport(layer, imported string) string {
	technologyPrefixes := []string{
		"database/sql", "net/http", "github.com/twmb/franz-go", "go.opentelemetry.io",
		modulePath + "/internal/platform", modulePath + "/internal/demo",
	}
	for _, prefix := range technologyPrefixes {
		if imported == prefix || strings.HasPrefix(imported, prefix+"/") {
			return layer + " must not depend on transport, persistence, telemetry, platform, or demo implementation"
		}
	}
	if strings.Contains(imported, "/adapters/") || strings.HasSuffix(imported, "/adapters") {
		return layer + " must point inward and cannot import adapters"
	}
	if layer == "domain" {
		if strings.Contains(imported, "/application/") || strings.HasSuffix(imported, "/application") || strings.Contains(imported, "/ports/") || strings.HasSuffix(imported, "/ports") {
			return "domain cannot import application or ports"
		}
	}
	if layer == "ports" {
		if strings.Contains(imported, "/application/") || strings.HasSuffix(imported, "/application") {
			return "ports cannot import application"
		}
	}
	return ""
}

func importsOf(t *testing.T, path string) []string {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	result := make([]string, 0, len(parsed.Imports))
	for _, declaration := range parsed.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.IMPORT {
			continue
		}
		for _, spec := range general.Specs {
			value, err := strconv.Unquote(spec.(*ast.ImportSpec).Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", path, err)
			}
			result = append(result, value)
		}
	}
	return result
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve architecture test source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}
