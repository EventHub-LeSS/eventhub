-- The unique indexes can be dropped; the collapsed duplicate rows cannot be restored.
DROP INDEX IF EXISTS organizations_name_key;
DROP INDEX IF EXISTS organizations_alias_key;
