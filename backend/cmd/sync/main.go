package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"backend/internal/db"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	database, err := db.Connect()
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}

	keycloakCfg := service.KeycloakClientConfig{
		Host:             os.Getenv("KEYCLOAK_HOST"),
		UserRealm:        os.Getenv("KEYCLOAK_USER_REALM"),
		FrontendClientID: firstNonEmpty(os.Getenv("KEYCLOAK_FRONTEND_CLIENT_ID"), "frontend"),
	}
	if keycloakCfg.Host == "" || keycloakCfg.UserRealm == "" {
		log.Fatal("KEYCLOAK_HOST and KEYCLOAK_USER_REALM are required")
	}
	if os.Getenv("KEYCLOAK_REALM") != "" && keycloakCfg.UserRealm == "" {
		keycloakCfg.UserRealm = os.Getenv("KEYCLOAK_REALM")
	}
	keycloakService := service.NewKeycloakService(keycloakCfg)

	syncUsername := firstNonEmpty(os.Getenv("SYNC_USERNAME"), "großmeister_finn")
	syncPassword := firstNonEmpty(os.Getenv("SYNC_PASSWORD"), "password")

	log.Printf("logging in as %q to realm %q", syncUsername, keycloakCfg.UserRealm)
	jwt, err := keycloakService.LoginUser(ctx, syncUsername, syncPassword)
	if err != nil {
		log.Fatalf("login: %v", err)
	}
	accessToken := jwt.AccessToken

	userRepo := repository.NewUserRepository(database)
	orgRepo := repository.NewOrganizationRepository(database)

	usersSynced := 0
	orgsSynced := 0
	membershipsSynced := 0

	// --- Sync users ---
	log.Println("syncing users...")
	kcUsers, err := keycloakService.GetAllUsers(ctx, accessToken)
	if err != nil {
		log.Fatalf("get keycloak users: %v", err)
	}
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
		log.Printf("  created user %s (%s)", safeStr(kcUser.Username), user.UserID)
	}

	// --- Sync organizations ---
	log.Println("syncing organizations...")
	kcOrgs, err := keycloakService.GetAllOrganizations(ctx, accessToken)
	if err != nil {
		log.Fatalf("get keycloak organizations: %v", err)
	}
	var orgIDs []string
	for _, kcOrg := range kcOrgs {
		if kcOrg.ID == nil || *kcOrg.ID == "" {
			continue
		}
		orgIDs = append(orgIDs, *kcOrg.ID)
		existing, err := orgRepo.GetByKeycloakOrgID(*kcOrg.ID)
		if err != nil {
			log.Printf("  skip org %s: lookup error: %v", safeStr(kcOrg.Name), err)
			continue
		}
		if existing != nil {
			continue
		}
		org := &model.OrganizationModel{
			OrganizationID: uuid.New(),
			KeycloakOrgID:  *kcOrg.ID,
			Name:           safeStr(kcOrg.Name),
		}
		if err := orgRepo.CreateOrganization(org); err != nil {
			log.Printf("  skip org %s: create error: %v", safeStr(kcOrg.Name), err)
			continue
		}
		orgsSynced++
		log.Printf("  created org %s (%s)", safeStr(kcOrg.Name), org.OrganizationID)
	}

	// --- Sync memberships ---
	log.Println("syncing memberships...")
	for _, orgID := range orgIDs {
		members, err := keycloakService.GetOrganizationMembers(ctx, accessToken, orgID)
		if err != nil {
			log.Printf("  skip members for org %s: %v", orgID, err)
			continue
		}
		dbOrg, err := orgRepo.GetByKeycloakOrgID(orgID)
		if err != nil || dbOrg == nil {
			log.Printf("  skip members for org %s: not found in database", orgID)
			continue
		}
		for _, member := range members {
			if member.ID == nil || *member.ID == "" {
				continue
			}
			dbUser, err := userRepo.GetByKeycloakUserID(*member.ID)
			if err != nil || dbUser == nil {
				log.Printf("    skip member %s: not found in database", safeStr(member.Username))
				continue
			}
			membership := &model.OrganizationMembershipModel{
				UserID:         dbUser.UserID,
				OrganizationID: dbOrg.OrganizationID,
			}
			if err := orgRepo.AddMembership(membership); err != nil {
				log.Printf("    skip member %s: create error (likely already exists)", safeStr(member.Username))
				continue
			}
			membershipsSynced++
			log.Printf("    added %s to org %s", safeStr(member.Username), safeStr2(dbOrg.Name))
		}
	}

	fmt.Println()
	fmt.Printf("Sync complete: %d users, %d organizations, %d memberships\n", usersSynced, orgsSynced, membershipsSynced)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func safeStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func safeStr2(s string) string {
	return s
}
