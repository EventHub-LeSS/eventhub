package seed

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"backend/internal/model"
	"backend/internal/service"

	"gorm.io/gorm"
)

// Deterministic markers of the two seed flavors. Each seeder refuses to run when the
// target already carries the OTHER flavor's data, so a misconfigured deployment fails
// loudly instead of silently mixing demo data into live content (and vice versa). The
// markers are the deterministic seed UUIDs and marker accounts of the data definitions,
// so the check is cheap and needs no mutation.
const (
	mockMarkerEventTag  = "evt-1"                              // first mock event of the ref seeder
	mockMarkerUserEmail = "finn.betz@grossmeister.de"          // demo admin of the ref seeder
	liveMarkerEventTag  = "live-evt-1"                         // first event of the live seeder
	liveMarkerUserEmail = "sabine.vogt@provadis-hochschule.de" // live org admin
)

// EnsureNoLiveData lets the mock seeder refuse a target that already contains live
// deployment content in the database or realm.
func EnsureNoLiveData(ctx context.Context, db *gorm.DB, kc *service.KeycloakService, realm, token string) error {
	var found []string
	if exists, err := seedMarkerEventExists(db, liveMarkerEventTag); err != nil {
		return err
	} else if exists {
		found = append(found, fmt.Sprintf("live event %q", liveMarkerEventTag))
	}
	if exists, err := seedMarkerUserExists(ctx, db, kc, realm, token, liveMarkerUserEmail); err != nil {
		return err
	} else if exists {
		found = append(found, fmt.Sprintf("live user %q", liveMarkerUserEmail))
	}
	if len(found) == 0 {
		return nil
	}
	return fmt.Errorf("refusing to seed mock data: live deployment content found (%s). "+
		"This seeder creates the demo data of the ref deployment, which a live deployment must "+
		"not receive. Seed the live deployment with ./seed-live instead, or point this run at "+
		"the ref deployment", strings.Join(found, ", "))
}

// EnsureNoMockData lets the live seeder refuse a target that still carries mock seed
// data in the database or realm, and explains the one-time cleanup.
func EnsureNoMockData(ctx context.Context, db *gorm.DB, kc *service.KeycloakService, realm, token string) error {
	var found []string
	if exists, err := seedMarkerEventExists(db, mockMarkerEventTag); err != nil {
		return err
	} else if exists {
		found = append(found, fmt.Sprintf("mock event %q", mockMarkerEventTag))
	}
	if exists, err := seedMarkerUserExists(ctx, db, kc, realm, token, mockMarkerUserEmail); err != nil {
		return err
	} else if exists {
		found = append(found, fmt.Sprintf("mock user %q", mockMarkerUserEmail))
	}
	if len(found) == 0 {
		return nil
	}
	return fmt.Errorf("refusing to seed live content: mock seed data found (%s). "+
		"The live deployment must start clean: wipe the api-db volume, restart the keycloak "+
		"container once with KC_REALM_HARD_RESET=true to replace the polluted realm, and "+
		"re-run the seeder", strings.Join(found, ", "))
}

func seedMarkerEventExists(db *gorm.DB, tag string) (bool, error) {
	var existing model.EventModel
	err := db.Where("event_id = ?", seedUUID(tag)).First(&existing).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("look up seed marker event %q: %w", tag, err)
	default:
		return true, nil
	}
}

func seedMarkerUserExists(ctx context.Context, db *gorm.DB, kc *service.KeycloakService, realm, token, email string) (bool, error) {
	var existing model.UserModel
	err := db.Where("email = ?", email).First(&existing).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		// not in the database; the realm may still hold the account
	case err != nil:
		return false, fmt.Errorf("look up seed marker user %q: %w", email, err)
	default:
		return true, nil
	}
	if kc == nil {
		return false, nil
	}
	user, err := kc.GetUserByEmail(ctx, token, realm, email)
	if err != nil {
		return false, fmt.Errorf("look up seed marker user %q in Keycloak: %w", email, err)
	}
	return user != nil, nil
}
