-- EVENTHUB-294: users apply via a contact form to become an organizer. A global admin reviews the
-- application; approving it creates the requested organization with the applicant as org_admin.
CREATE TABLE IF NOT EXISTS organizer_applications (
    application_id       UUID PRIMARY KEY,
    user_id              UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    organization_name    TEXT NOT NULL,
    organization_alias   TEXT NOT NULL,
    contact_email        TEXT NOT NULL,
    contact_phone_number TEXT,
    street               TEXT,
    house_number         TEXT,
    postal_code          TEXT,
    city                 TEXT,
    country_code         VARCHAR(2),
    message              TEXT NOT NULL,
    status               TEXT NOT NULL DEFAULT 'pending',
    decision_reason      TEXT,
    decided_by_subject   TEXT,
    decided_by_username  TEXT,
    decided_at           TIMESTAMPTZ,
    -- The organization created on approval; kept NULL for pending and rejected applications.
    organization_id      UUID REFERENCES organizations(organization_id) ON DELETE SET NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT organizer_applications_status_check
        CHECK (status IN ('pending', 'approved', 'rejected')),
    -- A decided application always records when it was decided, a pending one never does.
    CONSTRAINT organizer_applications_decision_check
        CHECK ((status = 'pending') = (decided_at IS NULL)),
    -- A rejection always carries the reason the applicant is told.
    CONSTRAINT organizer_applications_rejection_reason_check
        CHECK (status <> 'rejected' OR decision_reason IS NOT NULL)
);

-- One open application per user: further submissions wait for the decision.
CREATE UNIQUE INDEX IF NOT EXISTS idx_organizer_applications_one_pending
    ON organizer_applications (user_id) WHERE status = 'pending';

-- Admin review lists applications by status, oldest first.
CREATE INDEX IF NOT EXISTS idx_organizer_applications_status_created
    ON organizer_applications (status, created_at, application_id);
