CREATE TABLE organizations (
    organization_id UUID PRIMARY KEY,
    keycloak_org_id TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    contact_email TEXT,
    contact_phone_number TEXT,
    street TEXT,
    house_number TEXT,
    postal_code TEXT,
    city TEXT,
    country_code VARCHAR(2),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Keep existing organization accounts and their event references intact by
-- reusing the former organization user's UUID as the organization UUID.
INSERT INTO organizations (
    organization_id,
    keycloak_org_id,
    name,
    contact_email,
    contact_phone_number,
    created_at,
    updated_at
)
SELECT
    ou.user_id,
    u.keycloak_sub,
    ou.organization_name,
    u.email,
    u.phone_number,
    u.created_at,
    u.updated_at
FROM organization_users ou
JOIN users u ON u.user_id = ou.user_id;

CREATE TABLE organization_memberships (
    user_id UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    organization_id UUID NOT NULL REFERENCES organizations(organization_id) ON DELETE CASCADE,
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, organization_id)
);

INSERT INTO organization_memberships (user_id, organization_id, joined_at)
SELECT ou.user_id, ou.user_id, u.created_at
FROM organization_users ou
JOIN users u ON u.user_id = ou.user_id;

ALTER TABLE users
    ADD COLUMN first_name TEXT,
    ADD COLUMN last_name TEXT;

UPDATE users u
SET first_name = pu.first_name,
    last_name = pu.last_name
FROM private_users pu
WHERE pu.user_id = u.user_id;

UPDATE users u
SET first_name = COALESCE(NULLIF(ou.contact_person_name, ''), ou.organization_name),
    last_name = ''
FROM organization_users ou
WHERE ou.user_id = u.user_id
  AND u.first_name IS NULL;

UPDATE users
SET first_name = '', last_name = ''
WHERE first_name IS NULL OR last_name IS NULL;

ALTER TABLE users
    ALTER COLUMN first_name SET NOT NULL,
    ALTER COLUMN last_name SET NOT NULL;
ALTER TABLE users RENAME COLUMN keycloak_sub TO keycloak_user_id;

ALTER TABLE events DROP CONSTRAINT IF EXISTS events_organizer_id_fkey;
ALTER TABLE events
    ADD CONSTRAINT events_organizer_id_fkey
    FOREIGN KEY (organizer_id) REFERENCES organizations(organization_id) ON DELETE SET NULL;

DROP TABLE organization_users;
DROP TABLE private_users;
ALTER TABLE users DROP COLUMN user_type;
DROP TYPE user_type;
DROP TYPE user_role;

ALTER TABLE locations
    ALTER COLUMN postal_code TYPE VARCHAR(5) USING postal_code::VARCHAR(5);

ALTER TABLE ratings
    ADD CONSTRAINT ratings_booking_id_key UNIQUE (booking_id);