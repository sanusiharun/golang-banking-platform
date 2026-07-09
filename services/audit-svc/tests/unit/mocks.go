package unit

import (
	"context"

	pkgaudit "github.com/sanusi/banking/pkg/audit"
	"github.com/sanusi/banking/services/audit-svc/internal/domain/dao"
	"github.com/sanusi/banking/services/audit-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/audit-svc/internal/repository"
	"github.com/sanusi/banking/services/audit-svc/internal/services"
)

// ── Mock AuditRepository ────────────────────────────────────

type MockAuditRepo struct {
	Event      *dao.AuditEvent
	Events     []*dao.AuditEvent
	NextCursor string
	Err        error

	CreateCalls  int
	GetByIDCalls int
	ListCalls    int

	// LastCreated captures the row passed to Create, for asserting mapped fields.
	LastCreated *dao.AuditEvent
}

func (m *MockAuditRepo) Create(_ context.Context, event *dao.AuditEvent) error {
	m.CreateCalls++
	m.LastCreated = event
	return m.Err
}

func (m *MockAuditRepo) GetByID(_ context.Context, _ string) (*dao.AuditEvent, error) {
	m.GetByIDCalls++
	return m.Event, m.Err
}

func (m *MockAuditRepo) List(_ context.Context, _ dto.QueryParams) ([]*dao.AuditEvent, string, error) {
	m.ListCalls++
	return m.Events, m.NextCursor, m.Err
}

var _ repository.AuditRepository = (*MockAuditRepo)(nil)

// ── Mock AuditService ────────────────────────────────────────

type MockAuditService struct {
	Event *dto.EventResponse
	List_ *dto.EventListResponse
	Err   error

	IngestCalls  int
	GetByIDCalls int
	ListCalls    int

	// LastIngested captures the event passed to Ingest, for asserting mapping.
	LastIngested pkgaudit.AuditEvent
	// LastListParams captures the params passed to the most recent List call.
	LastListParams dto.QueryParams
}

func (m *MockAuditService) Ingest(_ context.Context, event pkgaudit.AuditEvent) error {
	m.IngestCalls++
	m.LastIngested = event
	return m.Err
}

func (m *MockAuditService) GetByID(_ context.Context, _ string) (*dto.EventResponse, error) {
	m.GetByIDCalls++
	return m.Event, m.Err
}

func (m *MockAuditService) List(_ context.Context, params dto.QueryParams) (*dto.EventListResponse, error) {
	m.ListCalls++
	m.LastListParams = params
	return m.List_, m.Err
}

var _ services.AuditService = (*MockAuditService)(nil)
