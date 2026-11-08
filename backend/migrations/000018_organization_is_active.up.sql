-- Organizations requested by signed-in users stay inactive until an admin approves
-- them. Rows that already exist were created before that approval step and stay active.
ALTER TABLE organizations
    ADD COLUMN is_active BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE organizations SET is_active = TRUE;
