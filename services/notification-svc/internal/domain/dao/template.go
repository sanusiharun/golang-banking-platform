package dao

import "time"

// Template format constants.
const (
	FormatText = "TEXT"
	FormatHTML = "HTML"
)

// Template is the GORM model for the templates table.
type Template struct {
	ID        string    `gorm:"primaryKey;type:text"`
	Code      string    `gorm:"not null;type:text;index:idx_templates_code_active"`
	Name      string    `gorm:"not null;type:text"`
	Channel   string    `gorm:"not null;type:text"`
	Format    string    `gorm:"not null;type:text;default:'TEXT'"`
	Subject   string    `gorm:"type:text"`
	Body      string    `gorm:"not null;type:text"`
	Variables []byte    `gorm:"type:jsonb"`
	Version   int       `gorm:"not null;default:1"`
	Active    bool      `gorm:"not null;default:true;index:idx_templates_code_active"`
	CreatedAt time.Time `gorm:"not null;autoCreateTime"`
	UpdatedAt time.Time `gorm:"not null;autoUpdateTime"`
}

func (Template) TableName() string { return "templates" }
