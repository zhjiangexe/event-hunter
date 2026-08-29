package application

import (
	"context"
	"event-hunter/backend/internal/contexts/scenario_lab/domain"
	"testing"
	"time"
)

func TestListReturnsPersistedHistoryWithoutPolling(t *testing.T) {
	started := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	completed := started.Add(3 * time.Second)
	repository := &repositoryStub{records: []domain.RunRecord{{RunID: "run-1", ScenarioID: "S8", ExecutionMode: domain.LabInjection, Synthetic: true, CorrelationID: "LAB-S8", Status: domain.RunPassed, ExpectedEventTypes: []string{"OrderCreated"}, Actual: domain.EmptyActual(), Checks: []domain.Check{}, AcceptedAt: started.Add(-time.Second), StartedAt: &started, CompletedAt: &completed}}}
	runner := Runner{Repository: repository, Clock: fixedClock{completed}}
	filter := domain.RunFilter{ScenarioID: "S8", Status: domain.RunPassed, PageSize: 20}
	page, err := runner.List(t.Context(), filter)
	if err != nil {
		t.Fatal(err)
	}
	if repository.listCalls != 1 || len(page.Items) != 1 || page.Items[0].CurrentStep != "驗收通過" || *page.Items[0].DurationMS != 3000 {
		t.Fatalf("page = %#v", page)
	}
}

type fixedClock struct{ value time.Time }

func (clock fixedClock) Now() time.Time { return clock.value }

type repositoryStub struct {
	records   []domain.RunRecord
	listCalls int
}

func (repository *repositoryStub) Create(context.Context, domain.RunRecord) error { return nil }
func (repository *repositoryStub) Get(context.Context, string) (domain.RunRecord, error) {
	return domain.RunRecord{}, domain.ErrRunNotFound
}
func (repository *repositoryStub) List(context.Context, domain.RunFilter) ([]domain.RunRecord, error) {
	repository.listCalls++
	return repository.records, nil
}
func (repository *repositoryStub) MarkRunning(context.Context, string, time.Time) error { return nil }
func (repository *repositoryStub) Complete(context.Context, string, string, domain.Actual, []domain.Check, *string, time.Time) error {
	return nil
}
