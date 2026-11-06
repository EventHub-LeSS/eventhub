package seed

import (
	"embed"
	"encoding/base64"
	"fmt"
)

// The event images of the mock data, embedded as raw PNG bytes and base64 encoded when the
// events are created, exactly like the upload route stores them.

//go:embed images
var eventImageFiles embed.FS

// eventImage bundles the base64 encoded image with its MIME type.
type eventImage struct {
	base64      *string
	contentType *string
}

// seedEventImages maps the image file names of the embedded images to their prepared form.
func seedEventImages() (map[string]eventImage, error) {
	files := map[string]string{
		"festival":          "images/festival.png",
		"art-gallery":       "images/art-gallery.png",
		"stadthalle-bremen": "images/stadthalle-bremen.png",
	}
	images := make(map[string]eventImage, len(files))
	for key, path := range files {
		raw, err := eventImageFiles.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read embedded event image %q: %w", path, err)
		}
		encoded := base64.StdEncoding.EncodeToString(raw)
		contentType := "image/png"
		images[key] = eventImage{base64: &encoded, contentType: &contentType}
	}
	return images, nil
}

// eventImageFor returns the image stored under key, or ok=false when the event has none.
func eventImageFor(images map[string]eventImage, key string) (eventImage, bool) {
	if key == "" {
		return eventImage{}, false
	}
	image, ok := images[key]
	return image, ok
}
