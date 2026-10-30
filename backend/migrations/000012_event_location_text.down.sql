-- The free text cannot be split back into address fields: every distinct location text becomes a
-- location named after it with empty address fields, so events keep their venue.
CREATE TABLE IF NOT EXISTS locations (
    location_id UUID PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    city TEXT NOT NULL,
    postal_code VARCHAR(5) NOT NULL,
    street TEXT NOT NULL,
    house_number TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE events ADD COLUMN location_id UUID REFERENCES locations(location_id) ON DELETE SET NULL;

INSERT INTO locations (location_id, name, city, postal_code, street)
SELECT gen_random_uuid(), location, '', '', ''
FROM (SELECT DISTINCT location FROM events WHERE location IS NOT NULL) AS texts;

UPDATE events e
SET location_id = l.location_id
FROM locations l
WHERE l.name = e.location;

ALTER TABLE events DROP CONSTRAINT IF EXISTS events_location_not_blank;
ALTER TABLE events DROP COLUMN location;
