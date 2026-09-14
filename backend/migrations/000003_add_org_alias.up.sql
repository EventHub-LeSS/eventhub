ALTER TABLE organizations ADD COLUMN alias TEXT;
UPDATE organizations SET alias = keycloak_org_id WHERE alias IS NULL;
