package seed

import (
	"backend/internal/model"
	"backend/internal/testdb"
	"context"
	"encoding/base64"
	"testing"

	"github.com/google/uuid"
)

// Exercises the real seeder against a real Postgres; see testdb.Open for how to enable it.
func TestSeedEventsCarryImages(t *testing.T) {
	db := testdb.Open(t)
	s := NewDBSeeder(db)

	orgNames := []string{"Provadis Hochschule", "Telekom", "ACME Events", "Stadthalle Bremen"}
	for _, name := range orgNames {
		org := &model.OrganizationModel{
			OrganizationID: uuid.New(),
			KeycloakOrgID:  "kc-" + name,
			Alias:          "alias-" + name,
			Name:           name,
		}
		if err := db.Create(org).Error; err != nil {
			t.Fatalf("seed organization %q: %v", name, err)
		}
	}

	if err := s.resolveIDs(context.Background()); err != nil {
		t.Fatalf("resolve ids: %v", err)
	}
	if err := s.seedCategories(context.Background()); err != nil {
		t.Fatalf("seed categories: %v", err)
	}
	if err := s.seedLocations(context.Background()); err != nil {
		t.Fatalf("seed locations: %v", err)
	}
	if err := s.seedEvents(context.Background()); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	var withImage int64
	if err := db.Raw(`SELECT COUNT(*) FROM events WHERE image_base64 IS NOT NULL AND image_content_type IS NOT NULL`).Scan(&withImage).Error; err != nil {
		t.Fatalf("count image events: %v", err)
	}
	// Every seeded event carries an image; the embedded images are assigned per event in the
	// event definitions of seedEvents.
	if withImage != 39 {
		t.Fatalf("events with image = %d, want 39", withImage)
	}

	// The stored base64 of a festival event decodes to the embedded image.
	var stored *string
	if err := db.Raw(`SELECT image_base64 FROM events WHERE event_id = ?`, seedUUID("evt-1")).Scan(&stored).Error; err != nil || stored == nil {
		t.Fatalf("load seeded image: %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(*stored)
	if err != nil {
		t.Fatalf("decode seeded image: %v", err)
	}
	images, err := seedEventImages()
	if err != nil {
		t.Fatalf("load embedded images: %v", err)
	}
	image, ok := eventImageFor(images, "festival")
	if !ok {
		t.Fatal("embedded festival image missing")
	}
	expected, err := base64.StdEncoding.DecodeString(*image.base64)
	if err != nil {
		t.Fatalf("decode embedded image: %v", err)
	}
	if string(decoded) != string(expected) {
		t.Errorf("seeded image differs from the embedded festival.png")
	}

	// Reseeding fills images missing from existing events without replacing user uploads.
	custom := base64.StdEncoding.EncodeToString([]byte("custom image"))
	if err := db.Model(&model.EventModel{}).Where("event_id = ?", seedUUID("evt-1")).
		Updates(map[string]any{"image_base64": nil, "image_content_type": nil}).Error; err != nil {
		t.Fatalf("clear seeded image: %v", err)
	}
	if err := db.Model(&model.EventModel{}).Where("event_id = ?", seedUUID("evt-2")).
		Update("image_base64", custom).Error; err != nil {
		t.Fatalf("set custom image: %v", err)
	}
	if err := s.seedEvents(context.Background()); err != nil {
		t.Fatalf("reseed events: %v", err)
	}
	var backfilled, preserved model.EventModel
	if err := db.First(&backfilled, "event_id = ?", seedUUID("evt-1")).Error; err != nil {
		t.Fatalf("load backfilled event: %v", err)
	}
	if backfilled.ImageBase64 == nil || *backfilled.ImageBase64 != *image.base64 {
		t.Error("missing image was not backfilled")
	}
	if err := db.First(&preserved, "event_id = ?", seedUUID("evt-2")).Error; err != nil {
		t.Fatalf("load custom event: %v", err)
	}
	if preserved.ImageBase64 == nil || *preserved.ImageBase64 != custom {
		t.Error("custom image was overwritten")
	}
}
