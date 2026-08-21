package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/henning-kln/gocloak"
)

type KeycloakClientConfig struct {
	Host         string
	AdminRealm   string // Only for admin operations, e.g. creating users, creating orgs etc.
	UserRealm    string // Only for eventhub users
	ClientID     string
	ClientSecret string
}

type KeycloakService struct {
	cfg    KeycloakClientConfig
	client *gocloak.GoCloak
}

func NewKeycloakService(cfg KeycloakClientConfig) *KeycloakService {
	return &KeycloakService{
		cfg:    cfg,
		client: gocloak.NewClient(cfg.Host),
	}
}

func (k *KeycloakService) login(ctx context.Context) (*gocloak.JWT, error) {
	token, err := k.client.LoginClient(ctx, k.cfg.ClientID, k.cfg.ClientSecret, k.cfg.AdminRealm)
	if err != nil {
		return nil, fmt.Errorf("keycloak admin login failed: %w", err)
	}
	return token, nil
}

func (k *KeycloakService) CreateOrganization(org gocloak.OrganizationRepresentation) (string, error) {
	ctx := context.Background()
	token, err := k.login(ctx)
	if err != nil {
		return "", err
	}
	//check if org exists
	oid, err := k.GetOrganizationIDBySlug(*org.Name)
	if err == nil {
		return oid, fmt.Errorf("organization already exists")
	}

	oId, err := k.client.CreateOrganization(ctx, token.AccessToken, k.cfg.UserRealm, org)
	if err != nil {
		return "", fmt.Errorf("failed to create organization: %w", err)
	}
	return oId, nil
}

func (k *KeycloakService) GetOrganizationIDBySlug(slug string) (string, error) {
	ctx := context.Background()
	token, err := k.login(ctx)
	if err != nil {
		return "", err
	}
	pageSize := 50
	page := 0
	for {
		orgs, err := k.client.GetOrganizations(ctx, token.AccessToken, k.cfg.UserRealm,
			gocloak.GetOrganizationsParams{
				First: new(page * pageSize),
				Max:   &pageSize,
			},
		)
		if err != nil {
			return "", fmt.Errorf("failed to get organization %q: %w", orgs, err)
		}
		for _, org := range orgs {
			if strings.Compare(*org.Alias, slug) == 0 {
				return *org.ID, nil
			}
		}
		if len(orgs) < pageSize {
			break
		}
		page++
	}

	return "", fmt.Errorf("organization not found")
}

func (k *KeycloakService) GetOrganizationByID(id string) (*gocloak.OrganizationRepresentation, error) {
	ctx := context.Background()
	token, err := k.login(ctx)
	if err != nil {
		return nil, err
	}
	org, err := k.client.GetOrganizationByID(ctx, token.AccessToken, k.cfg.UserRealm, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization %q: %w", org, err)
	}
	return org, nil
}

func (k *KeycloakService) AddUserToOrganization(orgID, userID string) error {
	ctx := context.Background()
	token, err := k.login(ctx)
	if err != nil {
		return err
	}
	return k.client.AddUserToOrganization(ctx, token.AccessToken, k.cfg.UserRealm, orgID, userID)
}
