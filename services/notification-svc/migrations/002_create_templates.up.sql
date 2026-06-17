-- Migration: 002_create_templates
-- Creates the templates table for reusable, versioned notification templates.

BEGIN;

CREATE TABLE IF NOT EXISTS templates (
    id         TEXT        NOT NULL,
    code       TEXT        NOT NULL,
    name       TEXT        NOT NULL,
    channel    TEXT        NOT NULL,
    format     TEXT        NOT NULL DEFAULT 'TEXT',
    subject    TEXT,
    body       TEXT        NOT NULL,
    variables  JSONB,
    version    INTEGER     NOT NULL DEFAULT 1,
    active     BOOLEAN     NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_templates PRIMARY KEY (id),
    CONSTRAINT chk_templates_channel
        CHECK (channel IN ('EMAIL','SMS','PUSH','WHATSAPP','WEBHOOK')),
    CONSTRAINT chk_templates_format
        CHECK (format IN ('TEXT','HTML')),
    CONSTRAINT chk_templates_version CHECK (version >= 1)
);

-- Template lookup by code (only active templates are served).
CREATE INDEX IF NOT EXISTS idx_templates_code_active ON templates (code, active);

COMMENT ON TABLE  templates IS 'Reusable notification templates with version tracking.';
COMMENT ON COLUMN templates.code IS 'Human-readable identifier used by callers (e.g. account_created, otp_sms).';
COMMENT ON COLUMN templates.version IS 'Incremented on every UPDATE — provides an audit trail of template changes.';
COMMENT ON COLUMN templates.active IS 'Soft-delete flag. Inactive templates are not served by GetByCode.';
COMMENT ON COLUMN templates.variables IS 'JSON schema hint describing available template variables (documentation only).';

COMMIT;
