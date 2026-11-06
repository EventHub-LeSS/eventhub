DROP INDEX IF EXISTS locations_address_key;
ALTER TABLE locations DROP CONSTRAINT IF EXISTS locations_optional_parts_not_blank;

-- Names must be unique again. The address addition holds the former name; locations without one
-- are named after their address. Duplicates, e.g. rows differing only in accessibility, get their ID appended.
ALTER TABLE locations ADD COLUMN name TEXT;
UPDATE locations l
SET name = CASE WHEN n.rn = 1 THEN n.label ELSE n.label || ' (' || l.location_id || ')' END
FROM (
    SELECT location_id, label,
           ROW_NUMBER() OVER (PARTITION BY label ORDER BY created_at, location_id) AS rn
    FROM (
        SELECT location_id, created_at,
               COALESCE(address_addition,
                        concat_ws(' ', street, house_number) || ', ' || postal_code || ' ' || city) AS label
        FROM locations
    ) AS labelled
) AS n
WHERE n.location_id = l.location_id;
ALTER TABLE locations ALTER COLUMN name SET NOT NULL;
ALTER TABLE locations ADD CONSTRAINT locations_name_key UNIQUE (name);

ALTER TABLE locations DROP COLUMN accessible;
ALTER TABLE locations DROP COLUMN address_addition;
