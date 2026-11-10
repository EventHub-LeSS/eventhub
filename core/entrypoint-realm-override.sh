#!/bin/bash
#
# Realm import behavior
# ---------------------
# Every container start applies the realm file (eventhub-realm.json) with the
# bundled `realm-sync` tool once the server is up. The sync is non-destructive
# and idempotent: everything configured in the file is created or updated in
# place, while anything NOT in the file is preserved (organizations created
# during normal operations, self-registered users, changed passwords, ...).
# This is the default; opt out with KC_REALM_SYNC=false.
#
# KC_REALM_HARD_RESET=true is the escape hatch for a destructive full
# re-import BEFORE the server starts: the realm is replaced by the file and
# ALL runtime drift (users, orgs, role grants) is deleted. Use it for exactly
# one restart, then remove it.
#
# Uses `exec` per Keycloak's container guide so SIGTERM reaches kc.sh. The
# sync waits for the server in a background subshell, so it never delays the
# startup and dies with the container.
set -euo pipefail

if [ "${KC_REALM_HARD_RESET:-}" = "true" ]; then
    echo "entrypoint: KC_REALM_HARD_RESET=true → destructive re-import of /opt/keycloak/data/import (runtime data will be lost)"
    /opt/keycloak/bin/kc.sh import --dir /opt/keycloak/data/import --override true
    echo "entrypoint: hard reset import finished"
fi

if [ "${KC_REALM_SYNC:-true}" = "true" ]; then
    echo "entrypoint: starting background realm-sync (non-destructive apply of the realm file)"
    (
        export KEYCLOAK_HOST="${KEYCLOAK_SYNC_HOST:-http://localhost:8080}"
        export KEYCLOAK_REALM="${KEYCLOAK_REALM:-eventhub}"
        export KEYCLOAK_ADMIN_USERNAME="${KC_BOOTSTRAP_ADMIN_USERNAME:-admin}"
        export KEYCLOAK_ADMIN_PASSWORD="${KC_BOOTSTRAP_ADMIN_PASSWORD:-admin}"
        export REALM_IMPORT_FILE="${REALM_IMPORT_FILE:-/opt/keycloak/data/import/eventhub-realm.json}"
        if ! /opt/keycloak/realm-sync; then
            echo "entrypoint: realm-sync FAILED (see log above); the server keeps running with the previously applied configuration"
        fi
    ) &
fi

exec /opt/keycloak/bin/kc.sh "$@"
