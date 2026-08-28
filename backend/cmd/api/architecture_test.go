package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInvestigationHTTPHandlerDoesNotAccessPostgresDirectly(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve architecture test source path")
	}
	source, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "investigations.go"))
	if err != nil {
		t.Fatalf("read investigations.go: %v", err)
	}

	for _, forbidden := range []string{
		"api.db",
		"*sql.DB",
		"QueryContext(",
		"QueryRowContext(",
		"ExecContext(",
		"BeginTx(",
	} {
		if strings.Contains(string(source), forbidden) {
			t.Errorf("investigation HTTP handler contains direct PostgreSQL access %q; use the application service and repository adapter", forbidden)
		}
	}
}

func TestHTTPHandlersDoNotContainClickHouseQueries(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve architecture test source path")
	}
	for _, sourceName := range []string{"main.go", "investigations.go", "overview.go"} {
		source, err := os.ReadFile(filepath.Join(filepath.Dir(filename), sourceName))
		if err != nil {
			t.Fatalf("read %s: %v", sourceName, err)
		}
		for _, forbidden := range []string{"FROM forensics_events", "event_processing_attempts", "FORMAT JSONEachRow", "clickhouseQuery(", "quoteSQL("} {
			if strings.Contains(string(source), forbidden) {
				t.Errorf("%s contains ClickHouse query detail %q; use the application forensics service and ClickHouse adapter", sourceName, forbidden)
			}
		}
	}
}

func TestGrafanaWebhookHTTPAdapterDoesNotAccessPostgresDirectly(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve architecture test source path")
	}
	source, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "..", "..", "internal", "platform", "grafana", "webhook.go"))
	if err != nil {
		t.Fatalf("read Grafana webhook adapter: %v", err)
	}
	for _, forbidden := range []string{"database/sql", "BeginTx(", "QueryContext(", "QueryRowContext(", "ExecContext(", "grafana_alert_receipts", "investigation_cases", "case_evidence"} {
		if strings.Contains(string(source), forbidden) {
			t.Errorf("Grafana webhook HTTP adapter contains persistence detail %q; use the application service and repository adapter", forbidden)
		}
	}
}

func TestProductionDoesNotImportFlatInvestigationApplicationPackage(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve architecture test source path")
	}
	root := filepath.Join(filepath.Dir(filename), "..", "..")
	for _, directory := range []string{"cmd", "internal/platform"} {
		base := filepath.Join(root, directory)
		err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			source, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			if strings.Contains(string(source), `contexts/investigation/application"`) {
				t.Errorf("%s imports the flat investigation application package; import a capability package instead", path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", directory, err)
		}
	}
}
