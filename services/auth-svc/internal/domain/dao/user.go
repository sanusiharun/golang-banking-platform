// Package dao contains GORM model definitions for auth-svc.
// The users table is owned exclusively by auth-svc — no other service
// may read or write it directly. Cross-service identity is carried via JWT.
package dao

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"time"
)

// ── StringArray ────────────────────────────────────────────────────────────────

// StringArray is a custom type that maps to a PostgreSQL TEXT[] column.
// Implements driver.Valuer and sql.Scanner without any external dependency.
type StringArray []string

func (s StringArray) Value() (driver.Value, error) {
	if len(s) == 0 {
		return "{}", nil
	}
	var sb strings.Builder
	sb.WriteByte('{')
	for i, v := range s {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteByte('"')
		sb.WriteString(strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(v))
		sb.WriteByte('"')
	}
	sb.WriteByte('}')
	return sb.String(), nil
}

func (s *StringArray) Scan(src any) error {
	if src == nil {
		*s = StringArray{}
		return nil
	}
	var raw string
	switch v := src.(type) {
	case string:
		raw = v
	case []byte:
		raw = string(v)
	default:
		return fmt.Errorf("StringArray.Scan: cannot scan type %T", src)
	}

	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "{") || !strings.HasSuffix(raw, "}") {
		return fmt.Errorf("StringArray.Scan: invalid array literal %q", raw)
	}
	inner := raw[1 : len(raw)-1]
	if inner == "" {
		*s = StringArray{}
		return nil
	}

	var result StringArray
	var cur strings.Builder
	inQuote, escaped := false, false

	for _, ch := range inner {
		switch {
		case escaped:
			cur.WriteRune(ch)
			escaped = false
		case ch == '\\':
			escaped = true
		case ch == '"':
			inQuote = !inQuote
		case ch == ',' && !inQuote:
			result = append(result, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(ch)
		}
	}
	result = append(result, cur.String())
	*s = result
	return nil
}

// ── User model ────────────────────────────────────────────────────────────────

// User is the GORM model for the users table.
// Passwords are NEVER stored in plain text — only bcrypt hashes (cost ≥ 12).
type User struct {
	ID           string      `gorm:"primaryKey;type:text"`
	Username     string      `gorm:"type:text;not null;uniqueIndex"`
	Email        string      `gorm:"type:text;not null;uniqueIndex"`
	PasswordHash string      `gorm:"column:password_hash;type:text;not null"`
	Roles        StringArray `gorm:"type:text[];not null;default:'{}'"`
	TenantID     string      `gorm:"type:text;not null;default:default"`
	IsActive     bool        `gorm:"not null;default:true"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (User) TableName() string { return "users" }
