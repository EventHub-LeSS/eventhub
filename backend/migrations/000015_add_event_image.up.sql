-- Every event can carry one image. The image arrives as raw PNG or JPEG bytes and is stored
-- base64 encoded, together with its MIME type, so the binary route can serve the original
-- bytes again with the right Content-Type. Both columns are NULL for events without an image.
ALTER TABLE events ADD COLUMN image_base64 TEXT;
ALTER TABLE events ADD COLUMN image_content_type TEXT;

-- Only the two supported image types may be stored.
ALTER TABLE events
    ADD CONSTRAINT events_image_content_type_check CHECK (
        image_content_type IS NULL OR image_content_type IN ('image/png', 'image/jpeg')
    );

-- A stored image always has both parts: the bytes and its type.
ALTER TABLE events
    ADD CONSTRAINT events_image_parts_together CHECK (
        (image_base64 IS NULL) = (image_content_type IS NULL)
    );
