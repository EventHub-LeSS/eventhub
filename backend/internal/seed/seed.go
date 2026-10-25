package seed

import (
	"context"
	"fmt"
	"log"

	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Seeder struct {
	db *gorm.DB
	kc *service.KeycloakService
}

func NewSeeder(db *gorm.DB, kc *service.KeycloakService) *Seeder {
	return &Seeder{db: db, kc: kc}
}

func (s *Seeder) SeedAll(ctx context.Context, realm, token string) error {
	kcSeeder := NewKeycloakSeeder(s.kc, realm, token)
	if err := kcSeeder.Seed(ctx); err != nil {
		return fmt.Errorf("keycloak seed: %w", err)
	}

	if err := s.syncDB(ctx, realm, token); err != nil {
		return fmt.Errorf("db sync: %w", err)
	}

	dbSeeder := NewDBSeeder(s.db)
	if err := dbSeeder.Seed(ctx); err != nil {
		return fmt.Errorf("db seed: %w", err)
	}

	return nil
}

func (s *Seeder) syncDB(ctx context.Context, realm, token string) error {
	log.Println("db: syncing users and organizations from Keycloak...")
	userRepo := repository.NewUserRepository(s.db)
	orgRepo := repository.NewOrganizationRepository(s.db)

	kcUsers, err := s.kc.GetAllUsers(ctx, token)
	if err != nil {
		return fmt.Errorf("get keycloak users: %w", err)
	}
	usersSynced := 0
	for _, kcUser := range kcUsers {
		if kcUser.ID == nil || *kcUser.ID == "" {
			continue
		}
		existing, err := userRepo.GetByKeycloakUserID(*kcUser.ID)
		if err != nil {
			log.Printf("  skip user %s: lookup error: %v", safeStr(kcUser.Username), err)
			continue
		}
		if existing != nil {
			continue
		}
		user := &model.UserModel{
			UserID:         uuid.New(),
			KeycloakUserID: *kcUser.ID,
			FirstName:      safeStr(kcUser.FirstName),
			LastName:       safeStr(kcUser.LastName),
			Email:          safeStr(kcUser.Email),
		}
		if err := userRepo.Create(user); err != nil {
			log.Printf("  skip user %s: create error: %v", safeStr(kcUser.Username), err)
			continue
		}
		usersSynced++
	}

	kcOrgs, err := s.kc.GetAllOrganizations(ctx, token)
	if err != nil {
		return fmt.Errorf("get keycloak organizations: %w", err)
	}
	orgsSynced := 0
	var orgIDs []string
	for _, kcOrg := range kcOrgs {
		if kcOrg.ID == nil || *kcOrg.ID == "" {
			continue
		}
		orgIDs = append(orgIDs, *kcOrg.ID)
		alias := safeStr(kcOrg.Alias)
		name := safeStr(kcOrg.Name)
		existing, err := orgRepo.GetByAnyRef(*kcOrg.ID, alias, name)
		if err != nil {
			log.Printf("  skip org %s: lookup error: %v", name, err)
			continue
		}
		if existing != nil {
			// A realm re-import re-creates organizations with new IDs. Follow the new ID
			// instead of inserting a duplicate row for the same organization (EVENTHUB fix:
			// every realm generation used to duplicate every organization in the database).
			if existing.KeycloakOrgID != *kcOrg.ID || existing.Alias != alias {
				if err := orgRepo.UpdateKeycloakRefs(existing.OrganizationID, *kcOrg.ID, alias, name); err != nil {
					log.Printf("  skip org %s: update error: %v", name, err)
					continue
				}
				log.Printf("  reconciled org %s to Keycloak ID %s", name, *kcOrg.ID)
			}
			continue
		}
		org := &model.OrganizationModel{
			OrganizationID: uuid.New(),
			KeycloakOrgID:  *kcOrg.ID,
			Alias:          alias,
			Name:           name,
		}
		if err := orgRepo.CreateOrganization(org); err != nil {
			log.Printf("  skip org %s: create error: %v", name, err)
			continue
		}
		orgsSynced++
	}

	membershipsSynced := 0
	for _, orgID := range orgIDs {
		members, err := s.kc.GetOrganizationMembers(ctx, token, orgID)
		if err != nil {
			log.Printf("  skip members for org %s: %v", orgID, err)
			continue
		}
		dbOrg, err := orgRepo.GetByKeycloakOrgID(orgID)
		if err != nil || dbOrg == nil {
			continue
		}
		for _, member := range members {
			if member.ID == nil || *member.ID == "" {
				continue
			}
			dbUser, err := userRepo.GetByKeycloakUserID(*member.ID)
			if err != nil || dbUser == nil {
				continue
			}
			membership := &model.OrganizationMembershipModel{
				UserID:         dbUser.UserID,
				OrganizationID: dbOrg.OrganizationID,
			}
			if err := orgRepo.AddMembership(membership); err != nil {
				continue
			}
			membershipsSynced++
		}
	}

	log.Printf("db sync complete: %d users, %d organizations, %d memberships", usersSynced, orgsSynced, membershipsSynced)
	return nil
}

func safeStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
