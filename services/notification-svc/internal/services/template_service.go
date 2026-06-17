package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/google/uuid"

	pkgerrors "github.com/sanusi/banking/pkg/errors"
	"github.com/sanusi/banking/pkg/observability"
	"github.com/sanusi/banking/services/notification-svc/internal/domain/dao"
	"github.com/sanusi/banking/services/notification-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/notification-svc/internal/repository"
	tmpl "github.com/sanusi/banking/services/notification-svc/internal/template"
)

// TemplateService manages notification templates.
type TemplateService interface {
	Create(ctx context.Context, req *dto.CreateTemplateRequest) (*dto.TemplateResponse, error)
	Update(ctx context.Context, id string, req *dto.UpdateTemplateRequest) (*dto.TemplateResponse, error)
	Delete(ctx context.Context, id string) error
	GetByID(ctx context.Context, id string) (*dto.TemplateResponse, error)
	GetByCode(ctx context.Context, code string) (*dto.TemplateResponse, error)
	List(ctx context.Context, filter dto.ListTemplatesFilter) (*dto.PaginatedTemplatesResponse, error)
	Preview(ctx context.Context, id string, req *dto.PreviewTemplateRequest) (*dto.PreviewTemplateResponse, error)
}

type templateService struct {
	tr     *observability.ServiceTracer
	repo   repository.TemplateRepository
	engine *tmpl.Engine
}

// NewTemplateService creates a TemplateService.
func NewTemplateService(repo repository.TemplateRepository, engine *tmpl.Engine) TemplateService {
	return &templateService{
		tr:     observability.NewServiceTracer("TemplateService"),
		repo:   repo,
		engine: engine,
	}
}

func (s *templateService) Create(ctx context.Context, req *dto.CreateTemplateRequest) (res *dto.TemplateResponse, err error) {
	ctx, span := s.tr.Start(ctx, "Create",
		attribute.String("template.code", req.Code),
		attribute.String("template.channel", req.Channel),
	)
	defer s.tr.Finish(span, &err)

	varsJSON, _ := json.Marshal(req.Variables)
	t := &dao.Template{
		ID:        uuid.New().String(),
		Code:      req.Code,
		Name:      req.Name,
		Channel:   req.Channel,
		Format:    req.Format,
		Subject:   req.Subject,
		Body:      req.Body,
		Variables: varsJSON,
		Version:   1,
		Active:    true,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err = s.repo.Create(ctx, t); err != nil {
		return nil, fmt.Errorf("template_service.Create: %w", err)
	}

	slog.InfoContext(ctx, "template created",
		slog.String("template_id", t.ID),
		slog.String("code", t.Code),
	)
	return toTemplateResponse(t), nil
}

func (s *templateService) Update(ctx context.Context, id string, req *dto.UpdateTemplateRequest) (res *dto.TemplateResponse, err error) {
	ctx, span := s.tr.Start(ctx, "Update", attribute.String("template.id", id))
	defer s.tr.Finish(span, &err)

	t, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrTemplateNotFound) {
			return nil, pkgerrors.NotFound("template", id)
		}
		return nil, fmt.Errorf("template_service.Update: get: %w", err)
	}

	varsJSON, _ := json.Marshal(req.Variables)
	t.Name = req.Name
	t.Subject = req.Subject
	t.Body = req.Body
	t.Variables = varsJSON

	if err = s.repo.Update(ctx, t); err != nil {
		return nil, fmt.Errorf("template_service.Update: %w", err)
	}

	t.Version++
	return toTemplateResponse(t), nil
}

func (s *templateService) Delete(ctx context.Context, id string) (err error) {
	ctx, span := s.tr.Start(ctx, "Delete", attribute.String("template.id", id))
	defer s.tr.Finish(span, &err)

	if err = s.repo.SoftDelete(ctx, id); err != nil {
		if errors.Is(err, repository.ErrTemplateNotFound) {
			return pkgerrors.NotFound("template", id)
		}
		return fmt.Errorf("template_service.Delete: %w", err)
	}
	return nil
}

func (s *templateService) GetByID(ctx context.Context, id string) (res *dto.TemplateResponse, err error) {
	ctx, span := s.tr.Start(ctx, "GetByID", attribute.String("template.id", id))
	defer s.tr.Finish(span, &err)

	t, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrTemplateNotFound) {
			return nil, pkgerrors.NotFound("template", id)
		}
		return nil, fmt.Errorf("template_service.GetByID: %w", err)
	}
	return toTemplateResponse(t), nil
}

func (s *templateService) GetByCode(ctx context.Context, code string) (res *dto.TemplateResponse, err error) {
	ctx, span := s.tr.Start(ctx, "GetByCode", attribute.String("template.code", code))
	defer s.tr.Finish(span, &err)

	t, err := s.repo.GetByCode(ctx, code)
	if err != nil {
		if errors.Is(err, repository.ErrTemplateNotFound) {
			return nil, pkgerrors.NotFound("template", code)
		}
		return nil, fmt.Errorf("template_service.GetByCode: %w", err)
	}
	return toTemplateResponse(t), nil
}

func (s *templateService) List(ctx context.Context, filter dto.ListTemplatesFilter) (res *dto.PaginatedTemplatesResponse, err error) {
	ctx, span := s.tr.Start(ctx, "List")
	defer s.tr.Finish(span, &err)

	items, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("template_service.List: %w", err)
	}

	responses := make([]*dto.TemplateResponse, len(items))
	for i, t := range items {
		responses[i] = toTemplateResponse(t)
	}

	pageSize := filter.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))

	return &dto.PaginatedTemplatesResponse{
		Items:      responses,
		Page:       filter.Page,
		PageSize:   pageSize,
		TotalCount: total,
		TotalPages: totalPages,
	}, nil
}

func (s *templateService) Preview(ctx context.Context, id string, req *dto.PreviewTemplateRequest) (res *dto.PreviewTemplateResponse, err error) {
	ctx, span := s.tr.Start(ctx, "Preview", attribute.String("template.id", id))
	defer s.tr.Finish(span, &err)

	t, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrTemplateNotFound) {
			return nil, pkgerrors.NotFound("template", id)
		}
		return nil, fmt.Errorf("template_service.Preview: get: %w", err)
	}

	body, err := s.engine.Render(t.Format, t.Body, req.Variables)
	if err != nil {
		return nil, fmt.Errorf("template_service.Preview: render body: %w", err)
	}

	subject, err := s.engine.RenderSubject(t.Subject, req.Variables)
	if err != nil {
		return nil, fmt.Errorf("template_service.Preview: render subject: %w", err)
	}

	return &dto.PreviewTemplateResponse{
		Subject: subject,
		Body:    body,
		Format:  t.Format,
	}, nil
}

// ── mapping helpers ───────────────────────────────────────────────────────────

func toTemplateResponse(t *dao.Template) *dto.TemplateResponse {
	r := &dto.TemplateResponse{
		ID:        t.ID,
		Code:      t.Code,
		Name:      t.Name,
		Channel:   t.Channel,
		Format:    t.Format,
		Subject:   t.Subject,
		Body:      t.Body,
		Version:   t.Version,
		Active:    t.Active,
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
	}
	if len(t.Variables) > 0 {
		_ = json.Unmarshal(t.Variables, &r.Variables)
	}
	return r
}
