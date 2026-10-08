-- Append-only audit log of business-relevant changes made by organization members.
-- No foreign keys on purpose: the history must outlive renamed or deleted users, events and
-- organizations. organization_id is the Keycloak organization ID, which is shared by local data
-- and Keycloak role changes.
CREATE TABLE IF NOT EXISTS audit_logs (
    audit_log_id UUID PRIMARY KEY,
    operation_id UUID NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    actor_subject TEXT NOT NULL CHECK (actor_subject <> ''),
    actor_username TEXT NOT NULL DEFAULT '',
    organization_id TEXT NOT NULL CHECK (organization_id <> ''),
    action TEXT NOT NULL CHECK (action <> ''),
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    phase TEXT NOT NULL CHECK (phase IN ('completed', 'started', 'succeeded', 'incomplete')),
    changes JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_organization_time
    ON audit_logs (organization_id, occurred_at DESC, audit_log_id DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_operation ON audit_logs (operation_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_occurred_at ON audit_logs (occurred_at);
