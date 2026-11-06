-- The location of an event is entered as address fields: street, house number, postal code and
-- city, an optional address addition such as a hall or floor, and whether the place is accessible.
-- Each distinct combination is stored once and reused by every event at that address. Rows are
-- never changed: an event with a different address points to another row.
ALTER TABLE locations ADD COLUMN address_addition TEXT;
ALTER TABLE locations ADD COLUMN accessible BOOLEAN NOT NULL DEFAULT FALSE;

-- The venue name becomes the address addition, which takes over its role, e.g. "Musikhalle Hamburg".
UPDATE locations SET address_addition = NULLIF(btrim(name), '');
ALTER TABLE locations DROP COLUMN name;

-- Optional parts are NULL when absent, so the unique index below treats them alike.
UPDATE locations SET house_number = NULL WHERE btrim(house_number) = '';
ALTER TABLE locations
    ADD CONSTRAINT locations_optional_parts_not_blank CHECK (
        (house_number IS NULL OR btrim(house_number) <> '') AND
        (address_addition IS NULL OR btrim(address_addition) <> '')
    );

-- One row per address, compared case-insensitively. Names were unique, so only legacy rows whose
-- address and name differ in case alone collide; fail rather than silently merging them.
CREATE UNIQUE INDEX locations_address_key ON locations (
    lower(street), lower(house_number), postal_code, lower(city), lower(address_addition), accessible
) NULLS NOT DISTINCT;
