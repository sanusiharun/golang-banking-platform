// Package database provides a GORM database connection factory for PostgreSQL.
// All services share this package for consistent pool configuration.
package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Config holds all parameters required to open a PostgreSQL connection pool.
type Config struct {
	Host            string
	Port            int
	Database        string
	User            string
	Password        string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	LogLevel        string // silent | error | warn | info
}

// New opens a GORM *gorm.DB backed by a pgx connection pool.
// Returns an error if the connection cannot be established within the timeout.
func New(cfg *Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=UTC",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Database, cfg.SSLMode,
	)

	gormCfg := &gorm.Config{
		Logger:                                   newGormLogger(cfg.LogLevel),
		PrepareStmt:                              true,  // cache prepared statements
		DisableForeignKeyConstraintWhenMigrating: false,
		// Disable automatic timestamp on fields NOT tagged — prevents surprise updates.
		NowFunc: func() time.Time { return time.Now().UTC() },
	}

	db, err := gorm.Open(postgres.Open(dsn), gormCfg)
	if err != nil {
		return nil, fmt.Errorf("gorm open: %w", err)
	}

	// Configure the underlying sql.DB connection pool.
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get underlying sql.DB: %w", err)
	}

	maxOpen := cfg.MaxOpenConns
	if maxOpen == 0 {
		maxOpen = 25
	}
	maxIdle := cfg.MaxIdleConns
	if maxIdle == 0 {
		maxIdle = 5
	}
	lifetime := cfg.ConnMaxLifetime
	if lifetime == 0 {
		lifetime = 5 * time.Minute
	}

	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(lifetime)

	return db, nil
}

// HealthCheck pings the database. Use in readiness probes.
func HealthCheck(ctx context.Context, db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get sql.DB: %w", err)
	}
	return sqlDB.PingContext(ctx)
}

// Close closes the underlying connection pool.
func Close(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// ── GORM slog-compatible logger ───────────────────────────────────────────────

type gormSlogLogger struct {
	level logger.LogLevel
}

func newGormLogger(level string) logger.Interface {
	var l logger.LogLevel
	switch level {
	case "info":
		l = logger.Info
	case "warn":
		l = logger.Warn
	case "error":
		l = logger.Error
	default:
		l = logger.Silent
	}
	return &gormSlogLogger{level: l}
}

func (g *gormSlogLogger) LogMode(level logger.LogLevel) logger.Interface {
	return &gormSlogLogger{level: level}
}

func (g *gormSlogLogger) Info(_ context.Context, msg string, data ...any) {
	if g.level >= logger.Info {
		slog.Info(fmt.Sprintf(msg, data...), "component", "gorm")
	}
}

func (g *gormSlogLogger) Warn(_ context.Context, msg string, data ...any) {
	if g.level >= logger.Warn {
		slog.Warn(fmt.Sprintf(msg, data...), "component", "gorm")
	}
}

func (g *gormSlogLogger) Error(_ context.Context, msg string, data ...any) {
	if g.level >= logger.Error {
		slog.Error(fmt.Sprintf(msg, data...), "component", "gorm")
	}
}

func (g *gormSlogLogger) Trace(_ context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if g.level <= logger.Silent {
		return
	}
	elapsed := time.Since(begin)
	sql, rows := fc()
	if err != nil {
		slog.Error("gorm query", "sql", sql, "rows", rows, "duration_ms", elapsed.Milliseconds(), "error", err)
		return
	}
	if g.level == logger.Info {
		slog.Debug("gorm query", "sql", sql, "rows", rows, "duration_ms", elapsed.Milliseconds())
	}
}
