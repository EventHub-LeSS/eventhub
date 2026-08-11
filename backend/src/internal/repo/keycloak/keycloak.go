package keycloak

import (
	"context"
	"fmt"

	"github.com/henning-kln/gocloak"
)

type Config struct {
	Host         string
	AdminRealm   string // Only for admin operations, e.g. creating users, creating orgs etc.
	UserRealm    string // Only for eventhub users
	ClientID     string
	ClientSecret string
}

type Client struct {
	cfg    Config
	client *gocloak.GoCloak
}

func NewKeycloakClient(cfg Config) *Client {
	return &Client{
		cfg:    cfg,
		client: gocloak.NewClient(cfg.Host),
	}
}

func (k *Client) login(ctx context.Context) (*gocloak.JWT, error) {
	token, err := k.client.LoginClient(ctx, k.cfg.ClientID, k.cfg.ClientSecret, k.cfg.AdminRealm)
	if err != nil {
		return nil, fmt.Errorf("keycloak admin login failed: %w", err)
	}
	return token, nil
}

func (k *Client) CreateOrganization(org gocloak.OrganizationRepresentation) (string, error) {
	ctx := context.Background()
	token, err := k.login(ctx)
	if err != nil {
		return "", err
	}

	oId, err := k.client.CreateOrganization(ctx, token.AccessToken, k.cfg.UserRealm, org)
	if err != nil {
		return "", fmt.Errorf("failed to create organization: %w", err)
	}
	return oId, nil
}

func (k *Client) AddUserToOrganization(orgID, userID string) error {
	ctx := context.Background()
	token, err := k.login(ctx)
	if err != nil {
		return err
	}
	return k.client.AddUserToOrganization(ctx, token.AccessToken, k.cfg.UserRealm, orgID, userID)
}
