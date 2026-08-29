package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	attachchecksnapshot "event-hunter/backend/internal/contexts/eventcheck/application/attach_check_snapshot"
	classifycheckfinding "event-hunter/backend/internal/contexts/eventcheck/application/classify_check_finding"
	evaluateeventcheck "event-hunter/backend/internal/contexts/eventcheck/application/evaluate_event_check"
	getchecksnapshot "event-hunter/backend/internal/contexts/eventcheck/application/get_check_snapshot"
	listcheckmodels "event-hunter/backend/internal/contexts/eventcheck/application/list_check_models"
	listchecksnapshots "event-hunter/backend/internal/contexts/eventcheck/application/list_check_snapshots"
	savechecksnapshot "event-hunter/backend/internal/contexts/eventcheck/application/save_check_snapshot"
	"event-hunter/backend/internal/contexts/investigation/application/alert_intake"
	businessjourney "event-hunter/backend/internal/contexts/investigation/application/business_journey"
	caselifecycle "event-hunter/backend/internal/contexts/investigation/application/case_lifecycle"
	eventsearch "event-hunter/backend/internal/contexts/investigation/application/event_search"
	evidenceattachment "event-hunter/backend/internal/contexts/investigation/application/evidence_attachment"
	"event-hunter/backend/internal/contexts/investigation/application/forensics"
	generateevidencemanifest "event-hunter/backend/internal/contexts/investigation/application/generate_evidence_manifest"
	getinvestigationsummary "event-hunter/backend/internal/contexts/investigation/application/get_investigation_summary"
	ingestionissues "event-hunter/backend/internal/contexts/investigation/application/ingestion_issues"
	journeyprofiles "event-hunter/backend/internal/contexts/investigation/application/journey_profiles"
	"event-hunter/backend/internal/contexts/investigation/application/overview"
	patternanalysis "event-hunter/backend/internal/contexts/investigation/application/pattern_analysis"
	patterneffectiveness "event-hunter/backend/internal/contexts/investigation/application/pattern_effectiveness"
	patternfeedback "event-hunter/backend/internal/contexts/investigation/application/pattern_feedback"
	savedsearch "event-hunter/backend/internal/contexts/investigation/application/saved_search"
	platformclickhouse "event-hunter/backend/internal/platform/clickhouse"
	"event-hunter/backend/internal/platform/config"
	"event-hunter/backend/internal/platform/grafana"
	platformhealth "event-hunter/backend/internal/platform/health"
	platformpostgres "event-hunter/backend/internal/platform/postgres"
	"event-hunter/backend/internal/platform/sourcehealth"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load configuration", "error", err)
		os.Exit(1)
	}
	db, err := sql.Open("pgx", postgresURL(cfg.PostgresQueryTimeout))
	if err != nil {
		slog.Error("open control plane database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	server := &http.Server{
		Addr: cfg.HTTPAddress,
		Handler: newServerWithDependencies(
			cfg,
			db,
			getenv("DEMO_SESSION_SECRET", "demo_session_local_only"),
			getenv("GRAFANA_WEBHOOK_SECRET", "grafana_webhook_local_only"),
		),
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       cfg.HTTPRequestTimeout,
		WriteTimeout:      cfg.HTTPRequestTimeout + time.Second,
		IdleTimeout:       time.Minute,
	}
	slog.Info("Event Hunter API listening", "address", cfg.HTTPAddress, "temporal_enabled", cfg.TemporalEnabled)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serverError := make(chan error, 1)
	go func() {
		serverError <- server.ListenAndServe()
	}()

	select {
	case err := <-serverError:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("HTTP server stopped", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		slog.Info("Event Hunter API shutdown requested")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("graceful HTTP shutdown failed", "error", err)
			os.Exit(1)
		}
		if err := <-serverError; err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("HTTP server stopped during shutdown", "error", err)
			os.Exit(1)
		}
		slog.Info("Event Hunter API shutdown complete")
	}
}

func newServer(cfg config.Config) http.Handler {
	return protectAPI(cfg, defaultJSONContentType(newServeMuxWithWebhook(nil, nil, newForensicsService(cfg))))
}

func newServerWithDependencies(cfg config.Config, db *sql.DB, sessionSecret, grafanaWebhookSecret string) http.Handler {
	sessions := sessionManager{secret: []byte(sessionSecret)}
	clickHouseReadModel := newClickHouseReadModel(cfg)
	forensics := forensics.NewForensicsService(clickHouseReadModel)
	grafanaAlerts := alertintake.NewGrafanaAlertService(platformpostgres.NewGrafanaAlertRepository(db))
	readinessClient := &http.Client{Timeout: 1500 * time.Millisecond}
	readinessProbes := []platformhealth.Probe{
		platformhealth.Database("postgres", db),
		platformhealth.HTTPStatus("clickhouse", strings.TrimRight(getenv("CLICKHOUSE_URL", "http://localhost:28317"), "/")+"/ping", readinessClient),
	}
	switch strings.ToLower(strings.TrimSpace(getenv("EVENT_HUNTER_ATTEMPTS_INGESTION_MODE", "clickhouse-mv"))) {
	case "legacy", "shadow":
		readinessProbes = append(readinessProbes, platformhealth.RedpandaConnect(
			"processing_attempt_ingestion",
			getenv("REDPANDA_CONNECT_ATTEMPTS_READY_URL", "http://localhost:28344/ready"),
			readinessClient,
		))
	case "clickhouse-mv":
		readinessProbes = append(readinessProbes, platformhealth.KafkaConnectConnector(
			"processing_attempt_ingestion",
			getenv("CLICKHOUSE_POC_ATTEMPTS_CONNECT_STATUS_URL", "http://localhost:28345/connectors/event-hunter-poc-processing-attempt-raw-landing/status"),
			readinessClient,
		))
	default:
		readinessProbes = append(readinessProbes, platformhealth.InvalidConfiguration(
			"processing_attempt_ingestion", "unsupported EVENT_HUNTER_ATTEMPTS_INGESTION_MODE",
		))
	}
	switch strings.ToLower(strings.TrimSpace(getenv("EVENT_HUNTER_INGESTION_MODE", "clickhouse-mv"))) {
	case "legacy", "shadow":
		readinessProbes = append(readinessProbes,
			platformhealth.RedpandaConnect("domain_event_ingestion", getenv("REDPANDA_CONNECT_READY_URL", "http://localhost:28325/ready"), readinessClient),
			platformhealth.KafkaConsumerGroup(
				"domain_event_ingestion_group",
				strings.Split(getenv("KAFKA_BROKERS", "localhost:28319"), ","),
				getenv("EVENT_HUNTER_FORENSICS_CONSUMER_GROUP", "event-hunter-forensics-ingestion-v1"),
			),
		)
	case "clickhouse-mv":
		readinessProbes = append(readinessProbes, platformhealth.KafkaConnectConnector(
			"domain_event_ingestion",
			getenv("CLICKHOUSE_POC_CONNECT_STATUS_URL", "http://localhost:28345/connectors/event-hunter-poc-raw-landing/status"),
			readinessClient,
		), platformhealth.HTTPStatus(
			"technical_dlq_projection",
			getenv("TECHNICAL_DLQ_PROJECTOR_READY_URL", "http://localhost:28346/health/ready"),
			readinessClient,
		))
	default:
		readinessProbes = append(readinessProbes, platformhealth.InvalidConfiguration(
			"domain_event_ingestion", "unsupported EVENT_HUNTER_INGESTION_MODE",
		))
	}
	readiness := platformhealth.Handler{Timeout: 2 * time.Second, Probes: readinessProbes}
	mux := newServeMuxWithReadiness(
		grafana.NewWebhookHandler(grafanaAlerts, grafanaWebhookSecret), &sessions, forensics,
		readiness,
		platformpostgres.NewEventSearchQualifierRepository(db),
	)
	mux.HandleFunc("POST /api/v1/auth/demo-session", sessions.demoSession)
	mux.HandleFunc("DELETE /api/v1/auth/demo-session", sessions.demoSession)
	mux.HandleFunc("GET /api/v1/auth/me", sessions.me)
	mux.HandleFunc("GET /api/v1/patterns", sessions.requireRead(patternsHandler))
	patternMetrics := patternEffectivenessAPI{service: patterneffectiveness.NewService(platformpostgres.NewPatternEffectivenessReader(db))}
	mux.HandleFunc("GET /api/v1/patterns/effectiveness", sessions.requireRead(patternMetrics.get))
	overviewService := overview.NewService(
		platformpostgres.NewOverviewReader(db),
		clickHouseReadModel,
		sourcehealth.NewHTTPProbe(overview.SourceTempo, strings.TrimRight(getenv("TEMPO_INTERNAL_URL", "http://localhost:28328"), "/")+"/ready", sourceHealthHTTPClient, time.Second),
		sourcehealth.NewHTTPProbe(overview.SourceLoki, strings.TrimRight(getenv("LOKI_INTERNAL_URL", "http://localhost:28327"), "/")+"/ready", sourceHealthHTTPClient, time.Second),
		sourcehealth.NewHTTPProbe(overview.SourceGrafana, strings.TrimRight(getenv("GRAFANA_INTERNAL_URL", "http://localhost:28332"), "/")+"/api/health", sourceHealthHTTPClient, time.Second),
	)
	overviewHandler := overviewAPI{service: overviewService}
	mux.HandleFunc("GET /api/v1/investigations/overview", sessions.requireRead(overviewHandler.get))
	mux.HandleFunc("GET /api/v1/source-health", sessions.requireRead(overviewHandler.sourceHealth))
	ingestionIssueHandler := ingestionIssuesAPI{service: ingestionissues.NewService(clickHouseReadModel)}
	mux.HandleFunc("GET /api/v1/ingestion-issues", sessions.requireRead(ingestionIssueHandler.list))
	mux.HandleFunc("POST /api/v1/search/identify", sessions.requireRead(identifySmartSearchInput))
	eventCheckEvaluator := evaluateeventcheck.NewService(clickHouseReadModel)
	eventCheckRepository := platformpostgres.NewCheckSnapshotRepository(db)
	eventCheckUnitOfWork := platformpostgres.NewUnitOfWork(db)
	eventChecks := eventCheckAPI{
		evaluator: eventCheckEvaluator, models: listcheckmodels.NewService(),
		saver:       savechecksnapshot.NewService(eventCheckEvaluator, eventCheckRepository, eventCheckRepository, eventCheckUnitOfWork),
		getter:      getchecksnapshot.NewService(eventCheckRepository),
		lister:      listchecksnapshots.NewService(eventCheckRepository),
		classifier:  classifycheckfinding.NewService(eventCheckRepository, eventCheckRepository, eventCheckUnitOfWork),
		attachments: attachchecksnapshot.NewService(eventCheckRepository, eventCheckRepository, eventCheckUnitOfWork),
		sessions:    sessions,
	}
	mux.HandleFunc("POST /api/v1/event-checks/evaluations", eventChecks.evaluate)
	mux.HandleFunc("GET /api/v1/check-models", eventChecks.listModels)
	mux.HandleFunc("GET /api/v1/check-models/{modelId}/versions/{version}", eventChecks.getModel)
	mux.HandleFunc("GET /api/v1/check-models/{modelId}/versions/{version}/source", eventChecks.getModelSource)
	mux.HandleFunc("POST /api/v1/check-snapshots", eventChecks.createSnapshot)
	mux.HandleFunc("GET /api/v1/check-snapshots", eventChecks.listSnapshots)
	mux.HandleFunc("GET /api/v1/check-snapshots/{snapshotId}", eventChecks.getSnapshot)
	mux.HandleFunc("PATCH /api/v1/check-findings/{findingId}/feedback", eventChecks.classifyFinding)
	mux.HandleFunc("GET /api/v1/investigations/{investigationId}/check-snapshots", eventChecks.listInvestigationSnapshots)
	mux.HandleFunc("POST /api/v1/investigations/{investigationId}/check-snapshots", eventChecks.attachInvestigationSnapshot)
	savedSearchHandler := savedSearchAPI{service: savedsearch.NewService(platformpostgres.NewSavedSearchRepository(db)), sessions: sessions}
	mux.HandleFunc("GET /api/v1/saved-searches", savedSearchHandler.list)
	mux.HandleFunc("POST /api/v1/saved-searches", savedSearchHandler.create)
	mux.HandleFunc("DELETE /api/v1/saved-searches/{id}", savedSearchHandler.delete)
	mux.HandleFunc("GET /api/v1/search-presets", savedSearchHandler.presets)
	caseRepository := platformpostgres.NewCaseRepository(db)
	detailsRepository := platformpostgres.NewInvestigationDetailsRepository(db)
	unitOfWork := platformpostgres.NewUnitOfWork(db)
	caseLifecycle := caselifecycle.NewService(caseRepository, detailsRepository, unitOfWork)
	investigations := investigationAPI{
		commands: caseLifecycle, queries: caseLifecycle, patterns: patternanalysis.NewPatternService(caseRepository, detailsRepository, forensics, clickHouseReadModel, unitOfWork),
		feedback:    patternfeedback.NewService(detailsRepository, detailsRepository, unitOfWork),
		attachments: evidenceattachment.NewService(caseRepository, forensics, detailsRepository, unitOfWork),
		summaries:   getinvestigationsummary.NewService(caseLifecycle, forensics),
		manifests:   generateevidencemanifest.NewService(caseLifecycle),
		sessions:    sessions,
	}
	mux.HandleFunc("GET /api/v1/investigations", investigations.list)
	mux.HandleFunc("POST /api/v1/investigations", investigations.create)
	mux.HandleFunc("GET /api/v1/investigations/{id}", investigations.get)
	mux.HandleFunc("PATCH /api/v1/investigations/{id}", investigations.patch)
	mux.HandleFunc("POST /api/v1/investigations/{id}/notes", investigations.addNote)
	mux.HandleFunc("POST /api/v1/investigations/{id}/evidence/events", investigations.attachEvent)
	mux.HandleFunc("POST /api/v1/investigations/{id}/analyze", investigations.analyze)
	mux.HandleFunc("PATCH /api/v1/investigations/{id}/findings/{findingId}/feedback", investigations.updateFindingFeedback)
	mux.HandleFunc("GET /api/v1/investigations/{id}/summary", investigations.summary)
	mux.HandleFunc("GET /api/v1/investigations/{id}/evidence-bundle", investigations.evidenceBundle)
	mux.HandleFunc("POST /api/v1/investigations/{id}/close", investigations.close)
	return protectAPI(cfg, defaultJSONContentType(mux))
}

func newServerWithWebhook(cfg config.Config, webhook http.Handler, sessions *sessionManager) http.Handler {
	return protectAPI(cfg, defaultJSONContentType(newServeMuxWithWebhook(webhook, sessions, newForensicsService(cfg))))
}

func newServeMuxWithWebhook(webhook http.Handler, sessions *sessionManager, forensics *forensics.ForensicsService, qualifierRepositories ...eventsearch.EventSearchQualifierRepository) *http.ServeMux {
	return newServeMuxWithReadiness(webhook, sessions, forensics, http.HandlerFunc(healthHandler), qualifierRepositories...)
}

func newServeMuxWithReadiness(webhook http.Handler, sessions *sessionManager, forensics *forensics.ForensicsService, readiness http.Handler, qualifierRepositories ...eventsearch.EventSearchQualifierRepository) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", healthHandler)
	mux.Handle("GET /health/ready", readiness)
	journeyProfiles := journeyProfilesAPI{service: journeyprofiles.NewService()}
	if sessions == nil {
		mux.HandleFunc("GET /api/v1/journey-profiles", journeyProfiles.list)
	} else {
		mux.HandleFunc("GET /api/v1/journey-profiles", sessions.requireRead(journeyProfiles.list))
	}
	if webhook != nil {
		mux.Handle("POST /api/v1/integrations/grafana/alerts", webhook)
		var qualifiers eventsearch.EventSearchQualifierRepository
		if len(qualifierRepositories) > 0 {
			qualifiers = qualifierRepositories[0]
		}
		timeline := timelineAPI{
			forensics: forensics,
			searcher:  eventsearch.NewEventSearchService(forensics, qualifiers),
			sessions:  sessions,
		}
		if sessions == nil {
			mux.HandleFunc("GET /api/v1/timelines/{correlationID}", timeline.timeline)
			mux.HandleFunc("GET /api/v1/events/search", timeline.search)
			mux.HandleFunc("GET /api/v1/business-journeys/{correlationID}", businessJourneyAPI{service: businessjourney.NewService(forensics)}.get)
		} else {
			mux.HandleFunc("GET /api/v1/timelines/{correlationID}", sessions.requireRead(timeline.timeline))
			mux.HandleFunc("GET /api/v1/events/search", sessions.requireRead(timeline.search))
			mux.HandleFunc("GET /api/v1/business-journeys/{correlationID}", sessions.requireRead(businessJourneyAPI{service: businessjourney.NewService(forensics)}.get))
		}
	}
	return mux
}

func newForensicsService(cfg config.Config) *forensics.ForensicsService {
	return forensics.NewForensicsService(newClickHouseReadModel(cfg))
}

func newClickHouseReadModel(cfg config.Config) *platformclickhouse.HTTPReadModel {
	cfg = cfg.WithDefaults()
	readModel := platformclickhouse.NewHTTPReadModel(platformclickhouse.HTTPReadModelConfig{
		URL: getenv("CLICKHOUSE_URL", "http://localhost:28317"), Database: getenv("CLICKHOUSE_DB", "event_hunter"),
		User: getenv("CLICKHOUSE_USER", "event_hunter"), Password: getenv("CLICKHOUSE_PASSWORD", "event_hunter_local_only"),
		QueryTimeout: cfg.ClickHouseQueryTimeout, MaxResultRows: cfg.ClickHouseMaxResultRows, MaxResultBytes: cfg.ClickHouseMaxResultBytes,
		MaxRowsToRead: cfg.ClickHouseMaxRowsToRead, MaxBytesToRead: cfg.ClickHouseMaxBytesToRead, MaxThreads: cfg.ClickHouseMaxThreads,
		Client: clickHouseHTTPClient,
	})
	return readModel
}

type timelineAPI struct {
	forensics *forensics.ForensicsService
	searcher  *eventsearch.EventSearchService
	sessions  *sessionManager
}

type timelineEventMetadata struct {
	EventID          string   `json:"event_id"`
	EventType        string   `json:"event_type"`
	EventVersion     uint32   `json:"event_version"`
	OccurredAt       string   `json:"occurred_at"`
	Producer         string   `json:"producer"`
	CorrelationID    string   `json:"correlation_id"`
	CausationID      *string  `json:"causation_id"`
	TraceID          *string  `json:"trace_id"`
	AggregateType    string   `json:"aggregate_type"`
	AggregateID      string   `json:"aggregate_id"`
	Sequence         uint64   `json:"sequence"`
	KafkaTopic       string   `json:"kafka_topic"`
	KafkaPartition   uint32   `json:"kafka_partition"`
	KafkaOffset      uint64   `json:"kafka_offset"`
	ServiceVersion   *string  `json:"service_version"`
	AdmissionStatus  string   `json:"admission_status"`
	QualityFlags     []string `json:"quality_flags"`
	AdmissionProfile string   `json:"admission_profile"`
	IngestedAt       string   `json:"ingested_at"`
}

type timelineEvent struct {
	timelineEventMetadata
	Payload           map[string]any `json:"payload,omitempty"`
	ProcessingSummary map[string]any `json:"processing_summary,omitempty"`
}

func timelineEventsFromForensics(values []forensics.ForensicsEvent, includePayload bool) ([]timelineEvent, error) {
	events := make([]timelineEvent, 0, len(values))
	for _, value := range values {
		event := timelineEvent{timelineEventMetadata: timelineEventMetadata{
			EventID: value.EventID, EventType: value.EventType, EventVersion: value.EventVersion, OccurredAt: value.OccurredAt,
			Producer: value.Producer, CorrelationID: value.CorrelationID, CausationID: value.CausationID, TraceID: value.TraceID,
			AggregateType: value.AggregateType, AggregateID: value.AggregateID, Sequence: value.Sequence, KafkaTopic: value.KafkaTopic,
			KafkaPartition: value.KafkaPartition, KafkaOffset: value.KafkaOffset, ServiceVersion: value.ServiceVersion,
			AdmissionStatus: value.AdmissionStatus, QualityFlags: value.QualityFlags, AdmissionProfile: value.AdmissionProfile,
			IngestedAt: value.IngestedAt,
		}}
		if includePayload && value.Payload != "" {
			if err := json.Unmarshal([]byte(value.Payload), &event.Payload); err != nil {
				return nil, err
			}
			event.Payload = maskPayload(event.Payload)
		}
		events = append(events, event)
	}
	return events, nil
}

func (api timelineAPI) timeline(writer http.ResponseWriter, request *http.Request) {
	from, err := time.Parse(time.RFC3339, request.URL.Query().Get("from"))
	if err != nil {
		writeTimelineError(writer, http.StatusUnprocessableEntity, "INVALID_TIME_WINDOW")
		return
	}
	to, err := time.Parse(time.RFC3339, request.URL.Query().Get("to"))
	if err != nil || !to.After(from) {
		writeTimelineError(writer, http.StatusUnprocessableEntity, "INVALID_TIME_WINDOW")
		return
	}
	if to.Sub(from) > 7*24*time.Hour {
		writeTimelineError(writer, http.StatusUnprocessableEntity, "QUERY_WINDOW_TOO_LARGE")
		return
	}
	correlationID := request.PathValue("correlationID")
	includePayload, authorized := api.authorizePayloadRequest(writer, request)
	if !authorized {
		return
	}
	values, err := api.forensics.Search(request.Context(), forensics.EventSearchFilter{
		From: from.UTC(), To: to.UTC(), Limit: 1000, IncludePayload: includePayload, CorrelationID: correlationID,
	})
	if err != nil {
		writeClickHouseError(writer, err, "TIMELINE_UNAVAILABLE", "TIMELINE_TIMEOUT")
		return
	}
	events, err := timelineEventsFromForensics(values, includePayload)
	if err != nil {
		writeTimelineError(writer, http.StatusServiceUnavailable, "TIMELINE_UNAVAILABLE")
		return
	}
	if request.URL.Query().Get("include_processing_attempts") == "true" {
		summaries, summaryErr := processingSummaries(request.Context(), api.forensics, events)
		if summaryErr != nil {
			writeClickHouseError(writer, summaryErr, "PROCESSING_SUMMARY_UNAVAILABLE", "PROCESSING_SUMMARY_TIMEOUT")
			return
		}
		for index := range events {
			if summary, exists := summaries[events[index].EventID]; exists {
				events[index].ProcessingSummary = summary
			} else {
				events[index].ProcessingSummary = map[string]any{"attempt_count": 0, "final_status": nil, "consumer_groups": []string{}, "retry_reasons": []string{}, "last_attempt_at": nil}
			}
		}
	}
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(map[string]any{"correlation_id": correlationID, "from": from, "to": to, "event_count": len(events), "truncated": len(events) == 1000, "events": events})
}

func (api timelineAPI) search(writer http.ResponseWriter, request *http.Request) {
	from, err := time.Parse(time.RFC3339, request.URL.Query().Get("from"))
	if err != nil {
		writeTimelineError(writer, http.StatusUnprocessableEntity, "INVALID_TIME_WINDOW")
		return
	}
	to, err := time.Parse(time.RFC3339, request.URL.Query().Get("to"))
	if err != nil || !to.After(from) || to.Sub(from) > 7*24*time.Hour {
		writeTimelineError(writer, http.StatusUnprocessableEntity, "INVALID_TIME_WINDOW")
		return
	}
	limit := 100
	if value := request.URL.Query().Get("limit"); value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil || limit < 1 || limit > 10000 {
			writeTimelineError(writer, http.StatusUnprocessableEntity, "INVALID_LIMIT")
			return
		}
	}
	filter := forensics.EventSearchFilter{
		From: from.UTC(), To: to.UTC(), Limit: limit,
		CorrelationID: strings.TrimSpace(request.URL.Query().Get("correlation_id")), EventType: strings.TrimSpace(request.URL.Query().Get("event_type")),
		AggregateID: strings.TrimSpace(request.URL.Query().Get("aggregate_id")), TraceID: strings.TrimSpace(request.URL.Query().Get("trace_id")),
		EventID: strings.TrimSpace(request.URL.Query().Get("event_id")), Producer: strings.TrimSpace(request.URL.Query().Get("producer")),
		CausationID: strings.TrimSpace(request.URL.Query().Get("causation_id")), KafkaTopic: strings.TrimSpace(request.URL.Query().Get("kafka_topic")),
	}
	if value := request.URL.Query().Get("event_version"); value != "" {
		version, parseErr := strconv.Atoi(value)
		if parseErr != nil || version < 1 {
			writeTimelineError(writer, http.StatusUnprocessableEntity, "INVALID_EVENT_VERSION")
			return
		}
		converted := uint32(version)
		filter.EventVersion = &converted
	}
	if value := request.URL.Query().Get("kafka_partition"); value != "" {
		partition, parseErr := strconv.Atoi(value)
		if parseErr != nil || partition < 0 {
			writeTimelineError(writer, http.StatusUnprocessableEntity, "INVALID_KAFKA_PARTITION")
			return
		}
		converted := uint32(partition)
		filter.KafkaPartition = &converted
	}
	if value := request.URL.Query().Get("kafka_offset"); value != "" {
		offset, parseErr := strconv.ParseUint(value, 10, 64)
		if parseErr != nil {
			writeTimelineError(writer, http.StatusUnprocessableEntity, "INVALID_KAFKA_OFFSET")
			return
		}
		filter.KafkaOffset = &offset
	}
	includePayload, authorized := api.authorizePayloadRequest(writer, request)
	if !authorized {
		return
	}
	filter.IncludePayload = includePayload
	values, err := api.searcher.Search(request.Context(), eventsearch.AdvancedEventSearchFilter{
		EventSearchFilter: filter,
		PatternID:         strings.TrimSpace(request.URL.Query().Get("pattern_id")),
		AlertID:           strings.TrimSpace(request.URL.Query().Get("alert_id")),
		MinimumSeverity:   strings.TrimSpace(request.URL.Query().Get("severity")),
	})
	if err != nil {
		if errors.Is(err, eventsearch.ErrUnknownPattern) {
			writeTimelineError(writer, http.StatusUnprocessableEntity, "UNKNOWN_PATTERN")
			return
		}
		if errors.Is(err, eventsearch.ErrInvalidSeverity) {
			writeTimelineError(writer, http.StatusUnprocessableEntity, "INVALID_SEVERITY")
			return
		}
		if errors.Is(err, eventsearch.ErrQualifierResultTooLarge) {
			writeTimelineError(writer, http.StatusUnprocessableEntity, "FILTER_RESULT_TOO_LARGE")
			return
		}
		writeClickHouseError(writer, err, "EVENT_SEARCH_UNAVAILABLE", "EVENT_SEARCH_TIMEOUT")
		return
	}
	events, err := timelineEventsFromForensics(values, includePayload)
	if err != nil {
		writeTimelineError(writer, http.StatusServiceUnavailable, "EVENT_SEARCH_UNAVAILABLE")
		return
	}
	if request.URL.Query().Get("include_processing_attempts") == "true" {
		summaries, summaryErr := processingSummaries(request.Context(), api.forensics, events)
		if summaryErr != nil {
			writeClickHouseError(writer, summaryErr, "PROCESSING_SUMMARY_UNAVAILABLE", "PROCESSING_SUMMARY_TIMEOUT")
			return
		}
		for index := range events {
			events[index].ProcessingSummary = summaries[events[index].EventID]
		}
	}
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(map[string]any{"events": events, "count": len(events), "truncated": len(events) == limit})
}

func processingSummaries(ctx context.Context, service *forensics.ForensicsService, events []timelineEvent) (map[string]map[string]any, error) {
	if len(events) == 0 {
		return map[string]map[string]any{}, nil
	}
	ids := make([]string, 0, len(events))
	seen := map[string]bool{}
	for _, event := range events {
		if !seen[event.EventID] {
			seen[event.EventID] = true
			ids = append(ids, event.EventID)
		}
	}
	rows, err := service.ProcessingSummaries(ctx, ids)
	if err != nil {
		return nil, err
	}
	result := make(map[string]map[string]any)
	for eventID, row := range rows {
		result[eventID] = map[string]any{"attempt_count": row.AttemptCount, "final_status": row.FinalStatus, "consumer_groups": row.ConsumerGroups, "retry_reasons": []string{}, "last_attempt_at": row.LastAttemptAt}
	}
	return result, nil
}

var clickHouseHTTPClient = &http.Client{}
var sourceHealthHTTPClient = &http.Client{}

func writeTimelineError(writer http.ResponseWriter, status int, code string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]string{"code": code})
}

func writeClickHouseError(writer http.ResponseWriter, err error, unavailableCode, timeoutCode string) {
	if errors.Is(err, context.DeadlineExceeded) {
		writeTimelineError(writer, http.StatusGatewayTimeout, timeoutCode)
		return
	}
	writeTimelineError(writer, http.StatusServiceUnavailable, unavailableCode)
}

func healthHandler(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(writer).Encode(map[string]string{"status": "ok"})
}

type defaultJSONResponseWriter struct {
	http.ResponseWriter
	status int
}

func (writer *defaultJSONResponseWriter) WriteHeader(status int) {
	writer.status = status
	if status != http.StatusNoContent && writer.Header().Get("Content-Type") == "" {
		writer.Header().Set("Content-Type", "application/json")
	}
	writer.ResponseWriter.WriteHeader(status)
}

func (writer *defaultJSONResponseWriter) Write(body []byte) (int, error) {
	if writer.status == 0 {
		writer.WriteHeader(http.StatusOK)
	}
	return writer.ResponseWriter.Write(body)
}

// defaultJSONContentType keeps every public API response truthful even when a
// handler writes JSON through json.Encoder without explicitly setting a
// header. Health endpoints and 204 responses retain their own semantics.
func defaultJSONContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.HasPrefix(request.URL.Path, "/api/") {
			next.ServeHTTP(writer, request)
			return
		}
		next.ServeHTTP(&defaultJSONResponseWriter{ResponseWriter: writer}, request)
	})
}

func postgresURL(queryTimeout time.Duration) string {
	return "postgres://" + getenv("POSTGRES_USER", "event_hunter") + ":" + getenv("POSTGRES_PASSWORD", "event_hunter_local_only") + "@" + getenv("POSTGRES_HOST", "localhost") + ":" + getenv("POSTGRES_PORT", "28313") + "/" + getenv("POSTGRES_DB", "event_hunter") + "?sslmode=disable&statement_timeout=" + strconv.FormatInt(queryTimeout.Milliseconds(), 10)
}

func getenv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}
