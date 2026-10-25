package repository

import (
	"testing"

	"backend/internal/model"
	"backend/internal/testdb"

	"github.com/google/uuid"
)

// Migration 000011 keeps organizations unique per LOWER(alias)/LOWER(name); GetByAnyRef must
// compare the same way, or a casing drift between Keycloak and the kept row makes the lookup
// miss forever while the unique index blocks the insert. Empty refs (Keycloak did not send the
// field) must not match rows whose column is empty.
func TestOrganizationRepository_GetByAnyRefMatchesCaseInsensitively(t *testing.T) {
	db := testdb.Open(t)
	repo := NewOrganizationRepository(db)

	orgs := []*model.OrganizationModel{
		{OrganizationID: uuid.New(), KeycloakOrgID: "kc-1", Alias: "AcmeCorp", Name: "Acme Corporation"},
		{OrganizationID: uuid.New(), KeycloakOrgID: "kc-2", Alias: "", Name: "Beta"},
		{OrganizationID: uuid.New(), KeycloakOrgID: "kc-3", Alias: "Gamma", Name: ""},
	}
	for _, org := range orgs {
		if err := repo.CreateOrganization(org); err != nil {
			t.Fatal(err)
		}
	}

	cases := []struct {
		label                      string
		keycloakOrgID, alias, name string
		wantKeycloakOrgID          string
	}{
		{"keycloak id matches exactly", "kc-1", "", "", "kc-1"},
		{"alias matches ignoring case", "kc-new", "ACMECORP", "", "kc-1"},
		{"name matches ignoring case", "kc-new", "", "acme corporation", "kc-1"},
		{"name still matches when alias ref is empty", "kc-new", "", "Beta", "kc-2"},
		{"alias still matches when name ref is empty", "kc-new", "gamma", "", "kc-3"},
		{"empty refs never match an empty name column", "kc-new", "", "", ""},
	}
	for _, tc := range cases {
		org, err := repo.GetByAnyRef(tc.keycloakOrgID, tc.alias, tc.name)
		if err != nil {
			t.Fatalf("%s: %v", tc.label, err)
		}
		if tc.wantKeycloakOrgID == "" {
			if org != nil {
				t.Errorf("%s: got org %s, want none", tc.label, org.KeycloakOrgID)
			}
			continue
		}
		if org == nil {
			t.Errorf("%s: got no org, want %s", tc.label, tc.wantKeycloakOrgID)
			continue
		}
		if org.KeycloakOrgID != tc.wantKeycloakOrgID {
			t.Errorf("%s: got org %s, want %s", tc.label, org.KeycloakOrgID, tc.wantKeycloakOrgID)
		}
	}
}

// The create-reconcile path hands UpdateOrganization the stale row with every column of the
// submitted request applied; all of them must reach the kept row, which must move instead of
// duplicating.
func TestOrganizationRepository_UpdateOrganizationPersistsEveryColumn(t *testing.T) {
	db := testdb.Open(t)
	repo := NewOrganizationRepository(db)

	email := "info@acme.example"
	phone := "+49 228 1"
	street := "Weg"
	houseNumber := "1"
	postalCode := "53111"
	city := "Bonn"
	country := "DE"
	org := &model.OrganizationModel{
		OrganizationID: uuid.New(),
		KeycloakOrgID:  "kc-old",
		Alias:          "acme",
		Name:           "Acme",
	}
	if err := repo.CreateOrganization(org); err != nil {
		t.Fatal(err)
	}

	// A realm re-import re-created the organization under a new ID and different casing.
	existing, err := repo.GetByAnyRef("kc-new", "ACME", "ACME")
	if err != nil {
		t.Fatal(err)
	}
	if existing == nil || existing.OrganizationID != org.OrganizationID {
		t.Fatalf("reconcile lookup found %+v, want the seeded row", existing)
	}
	existing.KeycloakOrgID = "kc-new"
	existing.Alias = "acme-corp"
	existing.Name = "Acme Corporation"
	existing.ContactEmail = &email
	existing.ContactPhoneNumber = &phone
	existing.Street = &street
	existing.HouseNumber = &houseNumber
	existing.PostalCode = &postalCode
	existing.City = &city
	existing.CountryCode = &country
	if err := repo.UpdateOrganization(existing); err != nil {
		t.Fatal(err)
	}

	updated, err := repo.GetByKeycloakOrgID("kc-new")
	if err != nil {
		t.Fatal(err)
	}
	if updated == nil {
		t.Fatal("organization not found under its new Keycloak ID")
	}
	if updated.Alias != "acme-corp" || updated.Name != "Acme Corporation" {
		t.Errorf("identity columns not persisted: alias %q, name %q", updated.Alias, updated.Name)
	}
	assertPtr := func(label string, got *string, want string) {
		t.Helper()
		if got == nil || *got != want {
			t.Errorf("%s not persisted: %v", label, got)
		}
	}
	assertPtr("contactEmail", updated.ContactEmail, email)
	assertPtr("contactPhoneNumber", updated.ContactPhoneNumber, phone)
	assertPtr("street", updated.Street, street)
	assertPtr("houseNumber", updated.HouseNumber, houseNumber)
	assertPtr("postalCode", updated.PostalCode, postalCode)
	assertPtr("city", updated.City, city)
	assertPtr("countryCode", updated.CountryCode, country)

	var count int64
	if err := db.Model(&model.OrganizationModel{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("organization row count %d, want 1: the row must move, not duplicate", count)
	}
}
