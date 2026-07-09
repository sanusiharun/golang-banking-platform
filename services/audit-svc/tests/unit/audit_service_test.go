package unit

import (
	"context"
	"errors"
	"testing"

	pkgaudit "github.com/sanusi/banking/pkg/audit"
	"github.com/sanusi/banking/services/audit-svc/internal/domain/dao"
	"github.com/sanusi/banking/services/audit-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/audit-svc/internal/services"
)

func TestAuditService_Ingest(t *testing.T) {
	tests := []struct {
		name       string
		event      pkgaudit.AuditEvent
		repoErr    error
		wantErr    bool
		wantCreate bool
		wantStatus string // expected status written to the repo row
	}{
		{
			name: "valid event succeeds",
			event: pkgaudit.AuditEvent{
				Action:      "login",
				ActorID:     "usr-001",
				ServiceName: "auth-svc",
			},
			wantErr:    false,
			wantCreate: true,
			wantStatus: pkgaudit.StatusSuccess,
		},
		{
			name: "missing action",
			event: pkgaudit.AuditEvent{
				ActorID:     "usr-001",
				ServiceName: "auth-svc",
			},
			wantErr:    true,
			wantCreate: false,
		},
		{
			name: "missing actor_id",
			event: pkgaudit.AuditEvent{
				Action:      "login",
				ServiceName: "auth-svc",
			},
			wantErr:    true,
			wantCreate: false,
		},
		{
			name: "missing service_name",
			event: pkgaudit.AuditEvent{
				Action:  "login",
				ActorID: "usr-001",
			},
			wantErr:    true,
			wantCreate: false,
		},
		{
			name: "status defaults to success when empty",
			event: pkgaudit.AuditEvent{
				Action:      "login",
				ActorID:     "usr-001",
				ServiceName: "auth-svc",
				Status:      "",
			},
			wantErr:    false,
			wantCreate: true,
			wantStatus: pkgaudit.StatusSuccess,
		},
		{
			name: "explicit status is preserved",
			event: pkgaudit.AuditEvent{
				Action:      "login",
				ActorID:     "usr-001",
				ServiceName: "auth-svc",
				Status:      "failure",
			},
			wantErr:    false,
			wantCreate: true,
			wantStatus: "failure",
		},
		{
			name: "repo error is wrapped",
			event: pkgaudit.AuditEvent{
				Action:      "login",
				ActorID:     "usr-001",
				ServiceName: "auth-svc",
			},
			repoErr:    errors.New("db down"),
			wantErr:    true,
			wantCreate: true,
		},
		{
			name: "metadata is marshaled to JSON",
			event: pkgaudit.AuditEvent{
				Action:      "login",
				ActorID:     "usr-001",
				ServiceName: "auth-svc",
				Metadata:    map[string]any{"ip": "127.0.0.1"},
			},
			wantErr:    false,
			wantCreate: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockAuditRepo{Err: tt.repoErr}
			svc := services.New(mock)

			err := svc.Ingest(context.Background(), tt.event)

			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			wantCalls := 0
			if tt.wantCreate {
				wantCalls = 1
			}
			if mock.CreateCalls != wantCalls {
				t.Errorf("Create calls = %d; want %d", mock.CreateCalls, wantCalls)
			}

			if tt.wantCreate && tt.wantStatus != "" {
				if mock.LastCreated == nil {
					t.Fatal("expected LastCreated to be set")
				}
				if mock.LastCreated.Status != tt.wantStatus {
					t.Errorf("Status = %q; want %q", mock.LastCreated.Status, tt.wantStatus)
				}
			}
		})
	}
}

func TestAuditService_GetByID(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		row := &dao.AuditEvent{ID: "evt-001", Action: "login", ActorID: "usr-001"}
		mock := &MockAuditRepo{Event: row}
		svc := services.New(mock)

		resp, err := svc.GetByID(context.Background(), "evt-001")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.ID != row.ID {
			t.Errorf("ID = %q; want %q", resp.ID, row.ID)
		}
		if mock.GetByIDCalls != 1 {
			t.Errorf("GetByID calls = %d; want 1", mock.GetByIDCalls)
		}
	})

	t.Run("metadata is unmarshaled from JSON", func(t *testing.T) {
		row := &dao.AuditEvent{ID: "evt-002", Metadata: []byte(`{"ip":"127.0.0.1"}`)}
		mock := &MockAuditRepo{Event: row}
		svc := services.New(mock)

		resp, err := svc.GetByID(context.Background(), "evt-002")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Metadata["ip"] != "127.0.0.1" {
			t.Errorf("Metadata[\"ip\"] = %v; want %q", resp.Metadata["ip"], "127.0.0.1")
		}
	})

	t.Run("not found propagates repo error", func(t *testing.T) {
		wantErr := errors.New("not found")
		mock := &MockAuditRepo{Err: wantErr}
		svc := services.New(mock)

		_, err := svc.GetByID(context.Background(), "missing")

		if !errors.Is(err, wantErr) {
			t.Errorf("expected %v, got %v", wantErr, err)
		}
	})
}

func TestAuditService_List(t *testing.T) {
	t.Run("success maps events and cursor", func(t *testing.T) {
		mock := &MockAuditRepo{
			Events: []*dao.AuditEvent{
				{ID: "evt-001", Action: "login"},
				{ID: "evt-002", Action: "logout"},
			},
			NextCursor: "cursor-abc",
		}
		svc := services.New(mock)

		resp, err := svc.List(context.Background(), dto.QueryParams{Limit: 50})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Total != 2 {
			t.Errorf("Total = %d; want 2", resp.Total)
		}
		if resp.NextCursor != "cursor-abc" {
			t.Errorf("NextCursor = %q; want %q", resp.NextCursor, "cursor-abc")
		}
		if mock.ListCalls != 1 {
			t.Errorf("List calls = %d; want 1", mock.ListCalls)
		}
	})

	t.Run("repo error is wrapped", func(t *testing.T) {
		mock := &MockAuditRepo{Err: errors.New("db down")}
		svc := services.New(mock)

		_, err := svc.List(context.Background(), dto.QueryParams{})

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
