ALTER TABLE ratings DROP CONSTRAINT IF EXISTS ratings_booking_id_key;


ALTER TABLE locations ALTER COLUMN postal_code TYPE TEXT;

CREATE TYPE user_role AS ENUM ('administrator', 'organizer', 'visitor');
CREATE TYPE user_type AS ENUM ('private', 'organization');

ALTER TABLE users ADD COLUMN user_type user_type;
UPDATE users u
SET user_type = CASE
    WHEN EXISTS (
        SELECT 1
        FROM organization_memberships om
        WHERE om.user_id = u.user_id
    ) THEN 'organization'::user_type
    ELSE 'private'::user_type
END;
ALTER TABLE users ALTER COLUMN user_type SET NOT NULL;

CREATE TABLE private_users (
    user_id UUID PRIMARY KEY REFERENCES users(user_id) ON DELETE SET NULL,
    first_name TEXT NOT NULL,
    last_name TEXT NOT NULL
);

INSERT INTO private_users (user_id, first_name, last_name)
SELECT user_id, first_name, last_name
FROM users
WHERE user_type = 'private';

CREATE TABLE organization_users (
    user_id UUID PRIMARY KEY REFERENCES users(user_id) ON DELETE SET NULL,
    organization_name TEXT NOT NULL,
    contact_person_name TEXT
);

INSERT INTO organization_users (user_id, organization_name, contact_person_name)
SELECT DISTINCT ON (om.user_id)
    om.user_id,
    o.name,
    NULLIF(CONCAT_WS(' ', u.first_name, u.last_name), '')
FROM organization_memberships om
JOIN organizations o ON o.organization_id = om.organization_id
JOIN users u ON u.user_id = om.user_id
ORDER BY om.user_id, om.joined_at;

ALTER TABLE events DROP CONSTRAINT IF EXISTS events_organizer_id_fkey;
UPDATE events e
SET organizer_id = NULL
WHERE organizer_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM users u WHERE u.user_id = e.organizer_id);
ALTER TABLE events
    ADD CONSTRAINT events_organizer_id_fkey
    FOREIGN KEY (organizer_id) REFERENCES users(user_id) ON DELETE SET NULL;

DROP TABLE organization_memberships;
DROP TABLE organizations;

ALTER TABLE users
    RENAME COLUMN keycloak_user_id TO keycloak_sub;
ALTER TABLE users
    DROP COLUMN first_name,
    DROP COLUMN last_name;