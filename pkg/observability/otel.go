// Package observability bootstraps the OpenTelemetry SDK for distributed
// tracing and metrics. It is cloud-agnostic — exporters are configured via
// environment variables (OTEL_ENDPOINT).
package observability

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/metric"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// Provider holds the OTel tracer, meter, and log providers and manages graceful shutdown.
type Provider struct {
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
	logProvider    *sdklog.LoggerProvider
	ServiceName    string
}

// Config holds OTel bootstrap configuration.
type Config struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	OTLPEndpoint   string  // gRPC endpoint, e.g. "localhost:4317"
	SamplingRate   float64 // 0.0–1.0
	Enabled        bool
	// LogsEnabled controls whether logs are exported via OTLP.
	// Set false when the backend is Jaeger (traces only) — Jaeger does not
	// implement opentelemetry.proto.collector.logs.v1.LogsService and will
	// return Unimplemented errors on every retry if this is true.
	// Enable only when using a backend that supports OTLP logs (e.g. Grafana Loki, OpenTelemetry Collector).
	LogsEnabled bool
}

// Bootstrap initialises the global OTel tracer and meter providers.
// Returns a Provider whose Shutdown method must be deferred by the caller.
func Bootstrap(ctx context.Context, cfg Config) (*Provider, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(
			attribute.String("service.name", cfg.ServiceName),
			attribute.String("service.version", cfg.ServiceVersion),
			attribute.String("deployment.environment", cfg.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create OTel resource: %w", err)
	}

	// --- Tracer provider ---
	var sampler sdktrace.Sampler
	if cfg.SamplingRate >= 1.0 {
		sampler = sdktrace.AlwaysSample()
	} else if cfg.SamplingRate <= 0.0 {
		sampler = sdktrace.NeverSample()
	} else {
		sampler = sdktrace.TraceIDRatioBased(cfg.SamplingRate)
	}

	traceOpts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	}

	if cfg.Enabled && cfg.OTLPEndpoint != "" {
		traceExporter, err := otlptracegrpc.New(ctx,
			otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
			otlptracegrpc.WithInsecure(),
		)
		if err != nil {
			return nil, fmt.Errorf("create OTLP trace exporter: %w", err)
		}
		traceOpts = append(traceOpts,
			sdktrace.WithBatcher(traceExporter,
				sdktrace.WithBatchTimeout(5*time.Second),
				sdktrace.WithMaxExportBatchSize(512),
			),
		)
	}

	tp := sdktrace.NewTracerProvider(traceOpts...)
	otel.SetTracerProvider(tp)

	// --- Meter provider (Prometheus exporter) ---
	promExporter, err := prometheus.New()
	if err != nil {
		return nil, fmt.Errorf("create Prometheus metric exporter: %w", err)
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(promExporter),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	// --- Log provider (OTLP gRPC) ---
	// Only wired when both OTel and LogsEnabled are true.
	// Requires a backend that implements LogsService (e.g. OpenTelemetry Collector,
	// Grafana Loki). Jaeger does NOT support this — keep LogsEnabled=false with Jaeger.
	var lp *sdklog.LoggerProvider
	if cfg.Enabled && cfg.LogsEnabled && cfg.OTLPEndpoint != "" {
		logExporter, err := otlploggrpc.New(ctx,
			otlploggrpc.WithEndpoint(cfg.OTLPEndpoint),
			otlploggrpc.WithInsecure(),
		)
		if err != nil {
			return nil, fmt.Errorf("create OTLP log exporter: %w", err)
		}
		lp = sdklog.NewLoggerProvider(
			sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
			sdklog.WithResource(res),
		)
		global.SetLoggerProvider(lp)
	}

	return &Provider{
		tracerProvider: tp,
		meterProvider:  mp,
		logProvider:    lp,
		ServiceName:    cfg.ServiceName,
	}, nil
}

// Shutdown flushes and closes both providers. Should be deferred after Bootstrap.
func (p *Provider) Shutdown(ctx context.Context) error {
	var errs []error
	if err := p.tracerProvider.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("tracer provider shutdown: %w", err))
	}
	if err := p.meterProvider.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("meter provider shutdown: %w", err))
	}
	if p.logProvider != nil {
		if err := p.logProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("log provider shutdown: %w", err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("OTel shutdown errors: %v", errs)
	}
	return nil
}

// Tracer returns a named tracer from the global provider.
func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}

// Meter returns a named meter from the global provider.
func Meter(name string) metric.Meter {
	return otel.Meter(name)
}
