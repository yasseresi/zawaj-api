// Package observability wires optional OpenTelemetry tracing. It is a no-op
// unless an OTLP endpoint is configured, so dev/CI and unconfigured deployments
// run with zero tracing overhead and no external dependency.
package observability

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// InitTracing configures the global tracer provider from the standard OTEL_*
// environment (endpoint, headers, sampling). If neither OTEL_EXPORTER_OTLP_ENDPOINT
// nor OTEL_EXPORTER_OTLP_TRACES_ENDPOINT is set, tracing stays disabled and the
// returned shutdown is a no-op. serviceName labels emitted spans.
func InitTracing(ctx context.Context, serviceName string, log *slog.Logger) (func(context.Context) error, error) {
	noop := func(context.Context) error { return nil }

	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" && os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") == "" {
		log.Info("tracing disabled (set OTEL_EXPORTER_OTLP_ENDPOINT to enable)")
		return noop, nil
	}

	exp, err := otlptracehttp.New(ctx)
	if err != nil {
		return noop, err
	}
	res, err := resource.New(ctx, resource.WithAttributes(attribute.String("service.name", serviceName)))
	if err != nil {
		return noop, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	log.Info("tracing enabled", "service", serviceName)
	return tp.Shutdown, nil
}
