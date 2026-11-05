package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/testdb"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Locations are entered as address fields with the event; the same address reuses the stored
// location, another one adds a location. GET /locations lists them to fill the fields.

func locationCount(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var count int64
	if err := db.Model(&model.LocationModel{}).Count(&count).Error; err != nil {
		t.Fatalf("count locations: %v", err)
	}
	return count
}

func storedLocationOf(t *testing.T, db *gorm.DB, eventID uuid.UUID) model.LocationModel {
	t.Helper()
	var location model.LocationModel
	if err := db.Joins("JOIN events ON events.location_id = locations.location_id").
		Where("events.event_id = ?", eventID).First(&location).Error; err != nil {
		t.Fatalf("load location of event %s: %v", eventID, err)
	}
	return location
}

func createdDraftID(t *testing.T, rec *httptest.ResponseRecorder) uuid.UUID {
	t.Helper()
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created model.EventModel
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return created.EventID
}

func TestSaveEventAsDraftHandler_ReusesOrCreatesLocation(t *testing.T) {
	db := testdb.Open(t)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusPublished)
	router := newEventRouter(db, principalManaging("kc-org-b"))
	before := locationCount(t, db)

	// The stored address in another case and with surrounding whitespace is the same location.
	sameAddress := map[string]any{"street": " WEG", "postalCode": "53111", "city": "bonn ", "addressAddition": "LOC-" + seeded.locationID.String()}
	reused := createdDraftID(t, postDraft(t, router, with(draftBody(seeded), "location", sameAddress)))
	if location := storedLocationOf(t, db, reused); location.LocationID != seeded.locationID {
		t.Errorf("same address linked location %s, want %s", location.LocationID, seeded.locationID)
	}
	if count := locationCount(t, db); count != before {
		t.Errorf("same address added %d locations", count-before)
	}

	newAddress := map[string]any{"street": " Hauptstraße ", "houseNumber": "5", "postalCode": "50667", "city": "Köln",
		"addressAddition": "  Halle 2, 1. OG ", "accessible": true}
	first := createdDraftID(t, postDraft(t, router, with(draftBody(seeded), "location", newAddress)))
	created := storedLocationOf(t, db, first)
	if created.LocationID == seeded.locationID || created.Street != "Hauptstraße" || created.HouseNumber == nil || *created.HouseNumber != "5" ||
		created.PostalCode != "50667" || created.City != "Köln" || created.AddressAddition == nil || *created.AddressAddition != "Halle 2, 1. OG" ||
		!created.Accessible {
		t.Errorf("new address not stored trimmed: %+v", created)
	}
	if count := locationCount(t, db); count != before+1 {
		t.Errorf("new address added %d locations, want 1", count-before)
	}

	// Saving the new address again reuses it; differing in accessibility only is another location.
	second := createdDraftID(t, postDraft(t, router, with(draftBody(seeded), "location", newAddress)))
	if location := storedLocationOf(t, db, second); location.LocationID != created.LocationID {
		t.Errorf("repeated address linked location %s, want %s", location.LocationID, created.LocationID)
	}
	notAccessible := with(newAddress, "accessible", false)
	third := createdDraftID(t, postDraft(t, router, with(draftBody(seeded), "location", notAccessible)))
	if location := storedLocationOf(t, db, third); location.LocationID == created.LocationID || location.Accessible {
		t.Errorf("address without accessibility linked %+v, want a new location", location)
	}
	if count := locationCount(t, db); count != before+2 {
		t.Errorf("drafts added %d locations, want 2", count-before)
	}
}

// Locations are never changed: editing the address of one event links another location and leaves
// the events at the previous address where they are.
func TestUpdateEventHandler_NewAddressLeavesPreviousLocation(t *testing.T) {
	db := testdb.Open(t)
	edited := seedEventForOrg(t, db, "kc-org-b", model.EventStatusDraft)
	other := seedEventForOrg(t, db, "kc-org-a", model.EventStatusPublished)
	if err := db.Exec("UPDATE events SET location_id = ? WHERE event_id = ?", edited.locationID, other.eventID).Error; err != nil {
		t.Fatalf("share location: %v", err)
	}
	router := newEventRouter(db, principalManaging("kc-org-b"))

	address := map[string]any{"street": "Neue Straße", "houseNumber": "17", "postalCode": "10115", "city": "Berlin"}
	rec := putEvent(t, router, edited.eventID.String(), with(validBody(edited), "location", address))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	moved := storedLocationOf(t, db, edited.eventID)
	if moved.LocationID == edited.locationID || moved.Street != "Neue Straße" || moved.City != "Berlin" || moved.AddressAddition != nil {
		t.Errorf("edited event location = %+v, want the new address", moved)
	}
	kept := storedLocationOf(t, db, other.eventID)
	if kept.LocationID != edited.locationID || kept.Street != "Weg" || kept.City != "Bonn" ||
		kept.AddressAddition == nil || *kept.AddressAddition != "loc-"+edited.locationID.String() {
		t.Errorf("other event location changed to %+v", kept)
	}
}

func getLocations(t *testing.T, router http.Handler) []model.PublishedEventLocation {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/locations", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var locations []model.PublishedEventLocation
	if err := json.Unmarshal(rec.Body.Bytes(), &locations); err != nil {
		t.Fatalf("decode: %v; body: %s", err, rec.Body.String())
	}
	if locations == nil {
		t.Fatal("expected array, got null")
	}
	return locations
}

// Drafts are only visible to their organization, so are locations only their drafts use.
func TestListLocationsHandler_ListsSelectableLocations(t *testing.T) {
	db := testdb.Open(t)
	router := newEventRouter(db, principalManaging("kc-org-a"))
	if locations := getLocations(t, router); len(locations) != 0 {
		t.Fatalf("expected no locations, got %+v", locations)
	}

	ownDraft := seedEventForOrg(t, db, "kc-org-a", model.EventStatusDraft)
	foreignDraft := seedEventForOrg(t, db, "kc-org-b", model.EventStatusDraft)
	foreignCancelled := seedEventForOrg(t, db, "kc-org-b", model.EventStatusCancelled)
	foreignPublished := seedEventForOrg(t, db, "kc-org-b", model.EventStatusPublished)
	for _, row := range []struct {
		event seededEvent
		city  string
	}{{ownDraft, "Bonn"}, {foreignDraft, "Aachen"}, {foreignCancelled, "Dresden"}, {foreignPublished, "Celle"}} {
		if err := db.Exec("UPDATE locations SET city = ? WHERE location_id = ?", row.city, row.event.locationID).Error; err != nil {
			t.Fatalf("set city: %v", err)
		}
	}
	if err := db.Exec("UPDATE locations SET accessible = TRUE WHERE location_id = ?", foreignPublished.locationID).Error; err != nil {
		t.Fatalf("set accessible: %v", err)
	}
	unused := uuid.New()
	if err := db.Exec("INSERT INTO locations (location_id, street, postal_code, city) VALUES (?, 'Ring', '10115', 'Berlin')", unused).Error; err != nil {
		t.Fatalf("seed unused location: %v", err)
	}

	locations := getLocations(t, router)
	got := make([]uuid.UUID, 0, len(locations))
	for _, location := range locations {
		got = append(got, location.LocationID)
	}
	want := []uuid.UUID{ownDraft.locationID, foreignPublished.locationID, foreignCancelled.locationID}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("locations = %v, want own draft, published and cancelled sorted by city %v", got, want)
	}
	published := locations[1]
	if published.Street != "Weg" || published.PostalCode != "53111" || published.City != "Celle" || published.HouseNumber != nil ||
		published.AddressAddition == nil || *published.AddressAddition != "loc-"+foreignPublished.locationID.String() || !published.Accessible {
		t.Errorf("unexpected location data: %+v", published)
	}
}

func TestListLocationsHandler_StatusCodes(t *testing.T) {
	db := testdb.Open(t)
	for _, tc := range []struct {
		name      string
		principal *middleware.Principal
		want      int
	}{
		{"no principal", nil, http.StatusUnauthorized},
		{"no event_manager role", &middleware.Principal{Subject: "u"}, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newEventRouter(db, tc.principal).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/locations", nil))
			if rec.Code != tc.want {
				t.Fatalf("expected %d, got %d: %s", tc.want, rec.Code, rec.Body.String())
			}
		})
	}
}
