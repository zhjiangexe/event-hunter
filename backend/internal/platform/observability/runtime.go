package observability

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/plugin/kotel"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const instrumentationName = "event-hunter/backend"

// Runtime owns the providers used by one service process. Call Shutdown before
// process exit so batched spans, logs and metrics are flushed to the collector.
type Runtime struct {
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
	loggerProvider *sdklog.LoggerProvider
	propagator     propagation.TextMapPropagator
}

func New(ctx context.Context, serviceName, serviceVersion string) (*Runtime, error) {
	if strings.TrimSpace(serviceName) == "" {
		return nil, errors.New("OpenTelemetry service name is required")
	}
	endpoint, insecure, err := collectorEndpoint()
	if err != nil {
		return nil, err
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(
			attribute.String("service.name", serviceName),
			attribute.String("service.version", serviceVersion),
			attribute.String("deployment.environment.name", getenv("DEPLOYMENT_ENVIRONMENT", "local-demo")),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create OpenTelemetry resource: %w", err)
	}

	traceOptions := []otlptracehttp.Option{otlptracehttp.WithEndpoint(endpoint), otlptracehttp.WithTimeout(5 * time.Second)}
	logOptions := []otlploghttp.Option{otlploghttp.WithEndpoint(endpoint), otlploghttp.WithTimeout(5 * time.Second)}
	metricOptions := []otlpmetrichttp.Option{otlpmetrichttp.WithEndpoint(endpoint), otlpmetrichttp.WithTimeout(5 * time.Second)}
	if insecure {
		traceOptions = append(traceOptions, otlptracehttp.WithInsecure())
		logOptions = append(logOptions, otlploghttp.WithInsecure())
		metricOptions = append(metricOptions, otlpmetrichttp.WithInsecure())
	}

	traceExporter, err := otlptracehttp.New(ctx, traceOptions...)
	if err != nil {
		return nil, fmt.Errorf("create OTLP trace exporter: %w", err)
	}
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(traceExporter, sdktrace.WithBatchTimeout(time.Second)),
	)

	logExporter, err := otlploghttp.New(ctx, logOptions...)
	if err != nil {
		_ = tracerProvider.Shutdown(ctx)
		return nil, fmt.Errorf("create OTLP log exporter: %w", err)
	}
	loggerProvider := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter, sdklog.WithExportInterval(time.Second))),
	)

	metricExporter, err := otlpmetrichttp.New(ctx, metricOptions...)
	if err != nil {
		_ = loggerProvider.Shutdown(ctx)
		_ = tracerProvider.Shutdown(ctx)
		return nil, fmt.Errorf("create OTLP metric exporter: %w", err)
	}
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter, sdkmetric.WithInterval(10*time.Second))),
	)

	propagator := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
	otel.SetTextMapPropagator(propagator)
	global.SetLoggerProvider(loggerProvider)

	stdoutHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	otelHandler := otelslog.NewHandler(instrumentationName,
		otelslog.WithLoggerProvider(loggerProvider),
		otelslog.WithVersion(serviceVersion),
	)
	slog.SetDefault(slog.New(NewFanoutHandler(stdoutHandler, otelHandler)))

	return &Runtime{
		tracerProvider: tracerProvider,
		meterProvider:  meterProvider,
		loggerProvider: loggerProvider,
		propagator:     propagator,
	}, nil
}

func (runtime *Runtime) Shutdown(ctx context.Context) error {
	return errors.Join(
		runtime.meterProvider.Shutdown(ctx),
		runtime.loggerProvider.Shutdown(ctx),
		runtime.tracerProvider.Shutdown(ctx),
	)
}

// Kafka returns franz-go hooks plus the tracer used to create the explicit
// process span around business handling. The hooks inject and extract W3C
// trace context in Kafka record headers.
func (runtime *Runtime) Kafka(clientID, consumerGroup string) (*kotel.Tracer, []kgo.Hook) {
	tracerOptions := []kotel.TracerOpt{
		kotel.TracerProvider(runtime.tracerProvider),
		kotel.TracerPropagator(runtime.propagator),
		kotel.ClientID(clientID),
	}
	if consumerGroup != "" {
		tracerOptions = append(tracerOptions, kotel.ConsumerGroup(consumerGroup))
	}
	tracer := kotel.NewTracer(tracerOptions...)
	meter := kotel.NewMeter(kotel.MeterProvider(runtime.meterProvider))
	plugin := kotel.NewKotel(kotel.WithTracer(tracer), kotel.WithMeter(meter))
	return tracer, plugin.Hooks()
}

func collectorEndpoint() (string, bool, error) {
	raw := getenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:28330")
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Path != "" {
		return "", false, fmt.Errorf("OTEL_EXPORTER_OTLP_ENDPOINT must be an http(s) origin without a path: %q", raw)
	}
	return parsed.Host, parsed.Scheme == "http", nil
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
