package unit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	"github.com/sanusi/banking/services/audit-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/audit-svc/internal/repository/postgres"
	"github.com/sanusi/banking/services/audit-svc/internal/transport"
)

// withURLParams attaches chi route params to a request's context so
// chi.URLParam(r, ...) works in handlers without a full router.
func withURLParams(r *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestAuditHandler_IngestEvent(t *testing.T) {
	validBody := `{
		"actor_type":"user","actor_id":"usr-001","action":"login",
		"status":"success","service_name":"auth-svc"
	}`

	tests := []struct {
		name       string
		body       string
		svcErr     error
		wantStatus int
		wantCalls  int
	}{
		{
			name:       "valid request returns 201",
			body:       validBody,
			wantStatus: http.StatusCreated,
			wantCalls:  1,
		},
		{
			name:       "malformed JSON returns 400",
			body:       `{invalid`,
			wantStatus: http.StatusBadRequest,
			wantCalls:  0,
		},
		{
			name:       "missing required field fails validation",
			body:       `{"actor_type":"user","action":"login","status":"success","service_name":"auth-svc"}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantCalls:  0,
		},
		{
			name:       "service error returns 500",
			body:       validBody,
			svcErr:     errors.New("db down"),
			wantStatus: http.StatusInternalServerError,
			wantCalls:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSvc := &MockAuditService{Err: tt.svcErr}
			handler := transport.NewAuditHandler(mockSvc, validator.New())

			req := httptest.NewRequest(http.MethodPost, "/v1/audit/events", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.IngestEvent(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d; want %d (body: %s)", w.Code, tt.wantStatus, w.Body.String())
			}
			if mockSvc.IngestCalls != tt.wantCalls {
				t.Errorf("Ingest calls = %d; want %d", mockSvc.IngestCalls, tt.wantCalls)
			}
		})
	}
}

func TestAuditHandler_GetEvent(t *testing.T) {
	t.Run("success returns 200 with event", func(t *testing.T) {
		mockSvc := &MockAuditService{Event: &dto.EventResponse{ID: "evt-001"}}
		handler := transport.NewAuditHandler(mockSvc, validator.New())

		req := httptest.NewRequest(http.MethodGet, "/v1/audit/events/evt-001", nil)
		req = withURLParams(req, map[string]string{"id": "evt-001"})
		w := httptest.NewRecorder()

		handler.GetEvent(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d; want %d", w.Code, http.StatusOK)
		}
	})

	t.Run("not found returns 404", func(t *testing.T) {
		mockSvc := &MockAuditService{Err: postgres.ErrNotFound}
		handler := transport.NewAuditHandler(mockSvc, validator.New())

		req := httptest.NewRequest(http.MethodGet, "/v1/audit/events/missing", nil)
		req = withURLParams(req, map[string]string{"id": "missing"})
		w := httptest.NewRecorder()

		handler.GetEvent(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d; want %d", w.Code, http.StatusNotFound)
		}
	})

	t.Run("other error returns 500", func(t *testing.T) {
		mockSvc := &MockAuditService{Err: errors.New("db down")}
		handler := transport.NewAuditHandler(mockSvc, validator.New())

		req := httptest.NewRequest(http.MethodGet, "/v1/audit/events/evt-001", nil)
		req = withURLParams(req, map[string]string{"id": "evt-001"})
		w := httptest.NewRecorder()

		handler.GetEvent(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("status = %d; want %d", w.Code, http.StatusInternalServerError)
		}
	})
}

func TestAuditHandler_ListEvents(t *testing.T) {
	t.Run("success returns 200", func(t *testing.T) {
		mockSvc := &MockAuditService{List_: &dto.EventListResponse{Total: 0}}
		handler := transport.NewAuditHandler(mockSvc, validator.New())

		req := httptest.NewRequest(http.MethodGet, "/v1/audit/events?limit=20", nil)
		w := httptest.NewRecorder()

		handler.ListEvents(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d; want %d", w.Code, http.StatusOK)
		}
		if mockSvc.LastListParams.Limit != 20 {
			t.Errorf("Limit = %d; want 20", mockSvc.LastListParams.Limit)
		}
	})

	t.Run("service error returns 500", func(t *testing.T) {
		mockSvc := &MockAuditService{Err: errors.New("db down")}
		handler := transport.NewAuditHandler(mockSvc, validator.New())

		req := httptest.NewRequest(http.MethodGet, "/v1/audit/events", nil)
		w := httptest.NewRecorder()

		handler.ListEvents(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("status = %d; want %d", w.Code, http.StatusInternalServerError)
		}
	})

	t.Run("parses from/to date range", func(t *testing.T) {
		mockSvc := &MockAuditService{List_: &dto.EventListResponse{}}
		handler := transport.NewAuditHandler(mockSvc, validator.New())

		req := httptest.NewRequest(http.MethodGet,
			"/v1/audit/events?from=2026-01-01T00:00:00Z&to=2026-01-31T00:00:00Z", nil)
		w := httptest.NewRecorder()

		handler.ListEvents(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d; want %d", w.Code, http.StatusOK)
		}
		if mockSvc.LastListParams.From == nil || mockSvc.LastListParams.To == nil {
			t.Fatal("expected From and To to be parsed")
		}
		if mockSvc.LastListParams.From.Year() != 2026 || mockSvc.LastListParams.To.Day() != 31 {
			t.Errorf("From/To parsed incorrectly: %v / %v", mockSvc.LastListParams.From, mockSvc.LastListParams.To)
		}
	})

	t.Run("ignores unparseable date range", func(t *testing.T) {
		mockSvc := &MockAuditService{List_: &dto.EventListResponse{}}
		handler := transport.NewAuditHandler(mockSvc, validator.New())

		req := httptest.NewRequest(http.MethodGet, "/v1/audit/events?from=not-a-date&to=also-bad", nil)
		w := httptest.NewRecorder()

		handler.ListEvents(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d; want %d", w.Code, http.StatusOK)
		}
		if mockSvc.LastListParams.From != nil || mockSvc.LastListParams.To != nil {
			t.Errorf("expected From/To to stay nil on unparseable input, got %v / %v",
				mockSvc.LastListParams.From, mockSvc.LastListParams.To)
		}
	})
}

func TestAuditHandler_ListActorEvents(t *testing.T) {
	t.Run("success returns 200", func(t *testing.T) {
		mockSvc := &MockAuditService{List_: &dto.EventListResponse{}}
		handler := transport.NewAuditHandler(mockSvc, validator.New())

		req := httptest.NewRequest(http.MethodGet, "/v1/audit/actors/usr-001/events", nil)
		req = withURLParams(req, map[string]string{"actor_id": "usr-001"})
		w := httptest.NewRecorder()

		handler.ListActorEvents(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d; want %d", w.Code, http.StatusOK)
		}
		if mockSvc.LastListParams.ActorID != "usr-001" {
			t.Errorf("ActorID = %q; want %q", mockSvc.LastListParams.ActorID, "usr-001")
		}
	})

	t.Run("service error returns 500", func(t *testing.T) {
		mockSvc := &MockAuditService{Err: errors.New("db down")}
		handler := transport.NewAuditHandler(mockSvc, validator.New())

		req := httptest.NewRequest(http.MethodGet, "/v1/audit/actors/usr-001/events", nil)
		req = withURLParams(req, map[string]string{"actor_id": "usr-001"})
		w := httptest.NewRecorder()

		handler.ListActorEvents(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("status = %d; want %d", w.Code, http.StatusInternalServerError)
		}
	})
}

func TestAuditHandler_ListResourceEvents(t *testing.T) {
	t.Run("success returns 200", func(t *testing.T) {
		mockSvc := &MockAuditService{List_: &dto.EventListResponse{}}
		handler := transport.NewAuditHandler(mockSvc, validator.New())

		req := httptest.NewRequest(http.MethodGet, "/v1/audit/resources/account/acc-001/events", nil)
		req = withURLParams(req, map[string]string{"resource": "account", "resource_id": "acc-001"})
		w := httptest.NewRecorder()

		handler.ListResourceEvents(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d; want %d", w.Code, http.StatusOK)
		}
		if mockSvc.LastListParams.Resource != "account" || mockSvc.LastListParams.ResourceID != "acc-001" {
			t.Errorf("Resource/ResourceID = %q/%q; want %q/%q",
				mockSvc.LastListParams.Resource, mockSvc.LastListParams.ResourceID, "account", "acc-001")
		}
	})

	t.Run("service error returns 500", func(t *testing.T) {
		mockSvc := &MockAuditService{Err: errors.New("db down")}
		handler := transport.NewAuditHandler(mockSvc, validator.New())

		req := httptest.NewRequest(http.MethodGet, "/v1/audit/resources/account/acc-001/events", nil)
		req = withURLParams(req, map[string]string{"resource": "account", "resource_id": "acc-001"})
		w := httptest.NewRecorder()

		handler.ListResourceEvents(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("status = %d; want %d", w.Code, http.StatusInternalServerError)
		}
	})
}

// sanity check that the JSON envelope shape is what we expect from httpx.WriteCreated.
func TestAuditHandler_IngestEvent_ResponseShape(t *testing.T) {
	mockSvc := &MockAuditService{}
	handler := transport.NewAuditHandler(mockSvc, validator.New())

	body := `{"actor_type":"user","actor_id":"usr-001","action":"login","status":"success","service_name":"auth-svc"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/audit/events", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.IngestEvent(w, req)

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if resp["success"] != true {
		t.Errorf("success = %v; want true", resp["success"])
	}
}
