-- Repairs deployments whose Keycloak realm was re-imported: every re-import re-created the
-- organizations with new IDs, and the seeder keyed its existence check only on keycloak_org_id,
-- so every realm generation inserted a fresh copy of each organization into the database.
--
-- This migration collapses every organization back into a single row, repoints the rows that
-- reference a removed copy, and adds unique indexes so a future ID change can no longer insert
-- a duplicate: the seeder and the API now update the existing row instead.
--
-- Keycloak keeps organization names unique per realm, so LOWER(name) identifies an organization
-- across generations no matter which Keycloak ID the copies carry. The newest copy wins, because
-- its Keycloak ID most likely belongs to the current realm generation; the seeder reconciles the
-- ID on its next run anyway. Audit log entries keep the historical Keycloak organization ID of
-- the copy they were recorded under and are left untouched.
WITH ranked AS (
    SELECT
        organization_id,
        LOWER(name) AS dedup_key,
        ROW_NUMBER() OVER (
            PARTITION BY LOWER(name)
            ORDER BY created_at DESC, organization_id ASC
        ) AS rn
    FROM organizations
),
repoint AS (
    SELECT dup.organization_id AS duplicate_id, keep.organization_id AS keep_id
    FROM ranked dup
    JOIN ranked keep ON keep.dedup_key = dup.dedup_key AND keep.rn = 1
    WHERE dup.rn > 1
),
-- Memberships of duplicate rows collapse into the kept row's membership. All copies of an
-- organization can carry a membership for the same user, so rank them per user and keep:
-- rows beyond the first, and rows whose user already sits in the kept organization.
ranked_memberships AS (
    SELECT
        om.user_id,
        om.organization_id AS duplicate_id,
        r.keep_id,
        ROW_NUMBER() OVER (
            PARTITION BY om.user_id, r.keep_id
            ORDER BY om.organization_id
        ) AS rn
    FROM organization_memberships om
    JOIN repoint r ON r.duplicate_id = om.organization_id
),
drop_memberships AS (
    DELETE FROM organization_memberships om
    USING ranked_memberships rm
    WHERE om.user_id = rm.user_id
      AND om.organization_id = rm.duplicate_id
      AND (
          rm.rn > 1
          OR EXISTS (
              SELECT 1
              FROM organization_memberships other
              WHERE other.user_id = rm.user_id
                AND other.organization_id = rm.keep_id
          )
      )
),
move_memberships AS (
    UPDATE organization_memberships om
    SET organization_id = rm.keep_id
    FROM ranked_memberships rm
    WHERE om.user_id = rm.user_id
      AND om.organization_id = rm.duplicate_id
      AND rm.rn = 1
      AND NOT EXISTS (
          SELECT 1
          FROM organization_memberships other
          WHERE other.user_id = rm.user_id
            AND other.organization_id = rm.keep_id
      )
),
move_events AS (
    UPDATE events e
    SET organizer_id = r.keep_id
    FROM repoint r
    WHERE e.organizer_id = r.duplicate_id
)
DELETE FROM organizations o
USING repoint r
WHERE o.organization_id = r.duplicate_id;

-- Keycloak rejects organizations whose alias or name another organization already uses, so the
-- database enforces the same. Rows created before the alias was persisted carry an empty alias;
-- they are exempt until the next sync fills the alias in.
CREATE UNIQUE INDEX organizations_alias_key
    ON organizations (LOWER(alias))
    WHERE COALESCE(alias, '') <> '';

CREATE UNIQUE INDEX organizations_name_key
    ON organizations (LOWER(name));
