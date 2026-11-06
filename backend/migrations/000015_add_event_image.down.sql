ALTER TABLE events DROP CONSTRAINT IF EXISTS events_image_parts_together;
ALTER TABLE events DROP CONSTRAINT IF EXISTS events_image_content_type_check;
ALTER TABLE events DROP COLUMN IF EXISTS image_content_type;
ALTER TABLE events DROP COLUMN IF EXISTS image_base64;
