package postgres

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"event-hunter/backend/internal/contexts/investigation/domain"
	"event-hunter/backend/internal/contexts/investigation/ports"
)

func TestUnitOfWorkRollsBackCaseWhenAuditFails(t *testing.T) {
	dataSourceName := os.Getenv("EVENT_HUNTER_POSTGRES_INTEGRATION_URL")
	if dataSourceName == "" {
		t.Skip("EVENT_HUNTER_POSTGRES_INTEGRATION_URL is not set")
	}
	db, err := sql.Open("pgx", dataSourceName)
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	defer db.Close()
	if err := db.PingContext(t.Context()); err != nil {
		t.Fatalf("ping PostgreSQL: %v", err)
	}

	correlationID := "AUDIT-ROLLBACK-" + time.Now().UTC().Format("20060102150405.000000000")
	cases := NewCaseRepository(db)
	details := NewInvestigationDetailsRepository(db)
	unit := NewUnitOfWork(db)
	now := time.Now().UTC()
	err = unit.WithinTransaction(t.Context(), func(ctx context.Context) error {
		created, err := cases.Create(ctx, domain.InvestigationCase{
			Title: "audit rollback verification", Severity: domain.SeverityHigh, Status: domain.StatusOpen,
			CorrelationID: correlationID, Priority: domain.PriorityP1, Tags: []string{}, RelatedCorrelationIDs: []string{},
			IncidentWindow: domain.IncidentWindow{From: now.Add(-time.Hour), To: now, Source: domain.IncidentWindowManualDefault},
			LastUpdatedBy:  "integration-test", CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			return err
		}
		// INVALID_ROLE deliberately violates audit_logs_actor_role_chk after the
		// case insert. The whole unit of work must roll back.
		return details.RecordAudit(ctx, ports.Actor{Subject: "integration-test", Role: "INVALID_ROLE"}, "CREATE_INVESTIGATION", created.ID, "audit-rollback", map[string]any{})
	})
	if err == nil {
		t.Fatal("invalid audit role unexpectedly committed")
	}

	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM investigation_cases WHERE correlation_id=$1", correlationID).Scan(&count); err != nil {
		t.Fatalf("query rolled-back case: %v", err)
	}
	if count != 0 {
		t.Fatalf("case count = %d, want 0 after audit rollback", count)
	}
}
