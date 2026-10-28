-- The location of an event is free text entered with the event, e.g. "Musikhalle Hamburg,
-- Lombardsbrücke 1, 20354 Hamburg". It replaces the reference to the locations table, whose rows
-- organizers had no way to create.
ALTER TABLE events ADD COLUMN location TEXT;

-- Existing events keep their venue as text: name, street with house number, postal code and city.
-- Blank parts are skipped; events without a location stay NULL and cannot be published.
UPDATE events e
SET location = NULLIF(concat_ws(', ',
        NULLIF(btrim(l.name), ''),
        NULLIF(btrim(concat_ws(' ', l.street, l.house_number)), ''),
        NULLIF(btrim(concat_ws(' ', l.postal_code, l.city)), '')
    ), '')
FROM locations l
WHERE l.location_id = e.location_id;

ALTER TABLE events
    ADD CONSTRAINT events_location_not_blank CHECK (location IS NULL OR btrim(location) <> '');

ALTER TABLE events DROP COLUMN location_id;
DROP TABLE locations;
