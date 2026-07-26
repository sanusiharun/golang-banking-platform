package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"gorm.io/gorm"

	"github.com/sanusi/banking/pkg/observability"
	"github.com/sanusi/banking/services/notification-svc/internal/domain/dao"
	"github.com/sanusi/banking/services/notification-svc/internal/domain/dto"
)

var ErrTemplateNotFound = errors.New("template not found")

// TemplateRepository defines data access for notification templates.
type TemplateRepository interface {
	Create(ctx context.Context, t *dao.Template) error
	GetByID(ctx context.Context, id string) (*dao.Template, error)
	GetByCode(ctx context.Context, code string) (*dao.Template, error)
	Update(ctx context.Context, t *dao.Template) error
	SoftDelete(ctx context.Context, id string) error
	List(ctx context.Context, filter dto.ListTemplatesFilter) ([]*dao.Template, int64, error)
}

type templateRepository struct {
	tr *observability.ServiceTracer
	db *gorm.DB
}

// NewTemplateRepository creates a Postgres-backed TemplateRepository.
func NewTemplateRepository(db *gorm.DB) TemplateRepository {
	return &templateRepository{
		tr: observability.NewServiceTracer("TemplateRepository"),
		db: db,
	}
}

func (r *templateRepository) Create(ctx context.Context, t *dao.Template) (err error) {
	ctx, span := r.tr.Start(ctx, "Create", attribute.String("template.code", t.Code))
	defer r.tr.Finish(span, &err)

	if err = r.db.WithContext(ctx).Create(t).Error; err != nil {
		slog.ErrorContext(ctx, "template_repository: create", slog.String("error", err.Error()))
		return fmt.Errorf("template_repository.Create: %w", err)
	}
	return nil
}

func (r *templateRepository) GetByID(ctx context.Context, id string) (res *dao.Template, err error) {
	ctx, span := r.tr.Start(ctx, "GetByID", attribute.String("template.id", id))
	defer r.tr.Finish(span, &err)

	var t dao.Template
	if err = r.db.WithContext(ctx).Where("id = ?", id).First(&t).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTemplateNotFound
		}
		return nil, fmt.Errorf("template_repository.GetByID: %w", err)
	}
	return &t, nil
}

func (r *templateRepository) GetByCode(ctx context.Context, code string) (res *dao.Template, err error) {
	ctx, span := r.tr.Start(ctx, "GetByCode", attribute.String("template.code", code))
	defer r.tr.Finish(span, &err)

	var t dao.Template
	if err = r.db.WithContext(ctx).Where("code = ? AND active = true", code).First(&t).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTemplateNotFound
		}
		return nil, fmt.Errorf("template_repository.GetByCode: %w", err)
	}
	return &t, nil
}

func (r *templateRepository) Update(ctx context.Context, t *dao.Template) (err error) {
	ctx, span := r.tr.Start(ctx, "Update", attribute.String("template.id", t.ID))
	defer r.tr.Finish(span, &err)

	result := r.db.WithContext(ctx).Model(&dao.Template{}).Where("id = ?", t.ID).Updates(map[string]any{
		"name":       t.Name,
		"subject":    t.Subject,
		"body":       t.Body,
		"variables":  t.Variables,
		"version":    gorm.Expr("version + 1"),
		"updated_at": time.Now().UTC(),
	})
	if result.Error != nil {
		return fmt.Errorf("template_repository.Update: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrTemplateNotFound
	}
	return nil
}

func (r *templateRepository) SoftDelete(ctx context.Context, id string) (err error) {
	ctx, span := r.tr.Start(ctx, "SoftDelete", attribute.String("template.id", id))
	defer r.tr.Finish(span, &err)

	result := r.db.WithContext(ctx).Model(&dao.Template{}).Where("id = ?", id).Updates(map[string]any{
		"active":     false,
		"updated_at": time.Now().UTC(),
	})
	if result.Error != nil {
		return fmt.Errorf("template_repository.SoftDelete: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrTemplateNotFound
	}
	return nil
}

func (r *templateRepository) List(ctx context.Context, filter dto.ListTemplatesFilter) (items []*dao.Template, total int64, err error) {
	ctx, span := r.tr.Start(ctx, "List")
	defer r.tr.Finish(span, &err)

	q := r.db.WithContext(ctx).Model(&dao.Template{})
	if filter.Channel != "" {
		q = q.Where("channel = ?", filter.Channel)
	}
	if filter.Code != "" {
		q = q.Where("code = ?", filter.Code)
	}
	if filter.Active != nil {
		q = q.Where("active = ?", *filter.Active)
	}

	if err = q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("template_repository.List count: %w", err)
	}

	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	if err = q.Offset((page - 1) * pageSize).Limit(pageSize).Order("created_at DESC").Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("template_repository.List: %w", err)
	}
	return items, total, nil
}
