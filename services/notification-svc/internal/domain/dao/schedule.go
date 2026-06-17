package dao

import (
	"database/sql"
	"time"
)

// Schedule is the GORM model for the schedules table.
type Schedule struct {
	ID           string         `gorm:"primaryKey;type:text"`
	Name         string         `gorm:"not null;type:text"`
	Description  string         `gorm:"type:text"`
	Channel      string         `gorm:"not null;type:text"`
	TemplateCode string         `gorm:"not null;type:text"`
	Recipient    string         `gorm:"not null;type:text"`
	TemplateVars []byte         `gorm:"type:jsonb"`
	CronExpr     sql.NullString `gorm:"type:text"`
	ScheduledAt  *time.Time     `gorm:"type:timestamptz"`
	Recurring    bool           `gorm:"not null;default:false"`
	Enabled      bool           `gorm:"not null;default:true;index:idx_schedules_enabled_next_run"`
	LastRunAt    *time.Time     `gorm:"type:timestamptz"`
	NextRunAt    *time.Time     `gorm:"type:timestamptz;index:idx_schedules_enabled_next_run"`
	CreatedAt    time.Time      `gorm:"not null;autoCreateTime"`
	UpdatedAt    time.Time      `gorm:"not null;autoUpdateTime"`
}

func (Schedule) TableName() string { return "schedules" }
