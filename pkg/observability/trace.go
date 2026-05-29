// Package observability — ServiceTracer
//
// Declare once on the constructor. Two lines per method. Zero manual span
// lifecycle, zero manual error recording.
//
// ─── Pattern ────────────────────────────────────────────────────────────────
//
//	type AccountService struct {
//	    tr   *observability.ServiceTracer
//	    repo AccountRepository
//	}
//
//	func NewAccountService(repo AccountRepository) AccountService {
//	    return &accountService{
//	        tr:   observability.NewServiceTracer("AccountService"),
//	        repo: repo,
//	    }
//	}
//
//	func (s *accountService) Credit(ctx context.Context, id string, amount int64) (res *dto.AccountResponse, err error) {
//	    ctx, span := s.tr.Start(ctx, "Credit",
//	        attribute.String("account.id", id),
//	        attribute.Int64("amount", amount),
//	    )
//	    defer s.tr.Finish(span, &err)
//
//	    // plain business logic — no tracing boilerplate
//	    account, err := s.repo.GetByID(ctx, id)
//	    if err != nil {
//	        return nil, err  // Finish sees err != nil → marks span Error automatically
//	    }
//	    ...
//	    return toResponse(account), nil  // Finish sees err == nil → marks span Ok
//	}
//
// ─── Jaeger waterfall ────────────────────────────────────────────────────────
//
//	HTTP POST /v1/accounts/{id}/credit        ← Tracing middleware (always automatic)
//	  └─ AccountService.Credit                ← tr declared in NewAccountService
//	       ├─ AccountRepository.GetByID       ← tr declared in NewAccountRepository
//	       └─ AccountRepository.Update        ← tr declared in NewAccountRepository
package observability

import (
	"context"
	"encoding/json"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// ServiceTracer is declared once per struct in its constructor.
// It scopes all spans to a named component (e.g. "AccountService").
type ServiceTracer struct {
	tracer    trace.Tracer
	component string
}

// NewServiceTracer creates a ServiceTracer for the named component.
// Use the struct name — e.g. "AccountService", "AccountRepository", "AuthHandler".
func NewServiceTracer(component string) *ServiceTracer {
	return &ServiceTracer{
		tracer:    otel.Tracer(component),
		component: component,
	}
}

// Start opens a child span named "Component.op".
// Always paired with defer tr.Finish(span, &err).
//
// The returned ctx carries the new span — pass it to all downstream calls
// so they automatically become child spans.
func (t *ServiceTracer) Start(ctx context.Context, op string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, t.component+"."+op,
		trace.WithAttributes(attrs...),
	)
}

// Finish closes the span and auto-records the outcome.
//
//   defer t.Finish(span, &err)
//
// err is a pointer to the function's named return error. The defer reads the
// final value at exit time — whichever return path was taken.
//
//   err == nil  → span status Ok
//   err != nil  → span status Error + RecordError event (visible in Jaeger)
func (t *ServiceTracer) Finish(span trace.Span, err *error) {
	if err != nil && *err != nil {
		span.SetStatus(codes.Error, (*err).Error())
		span.RecordError(*err)
	} else {
		span.SetStatus(codes.Ok, "")
	}
	span.End()
}

// ─── Utilities ───────────────────────────────────────────────────────────────

// RecordError marks the active span in ctx as failed.
// Use inside handlers (void return) where named-return err is not available.
func RecordError(ctx context.Context, err error) {
	if err == nil {
		return
	}
	span := trace.SpanFromContext(ctx)
	span.SetStatus(codes.Error, err.Error())
	span.RecordError(err)
}

// AddEvent records a named checkpoint on the active span.
// Good for: "cache miss", "retry attempt 2", "saga compensation started".
func AddEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	trace.SpanFromContext(ctx).AddEvent(name, trace.WithAttributes(attrs...))
}

// AddJSON serialises v and attaches it as a span attribute.
// Use only in local/staging — never attach PII in production.
func AddJSON(span trace.Span, key string, v any) {
	if b, err := json.Marshal(v); err == nil {
		span.SetAttributes(attribute.String(key, string(b)))
	}
}

// SpanFromContext returns the active span from ctx.
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}
