#!/bin/bash
#
# KC_IMPORT_OVERRIDE=true gives operators a one-shot, explicit way to re-import
# the baked realm over an existing database, using Keycloak's supported
# standalone import command (default --override true) BEFORE the server starts.
# Keep it unset for normal operation. Set it for exactly one restart, then
# remove it: every restart with it set would wipe runtime drift (users, orgs)
# created after the image was built.
#
# Uses `exec` per Keycloak's container guide so SIGTERM reaches kc.sh.
set -euo pipefail

if [ "${KC_IMPORT_OVERRIDE:-}" = "true" ]; then
    echo "entrypoint: KC_IMPORT_OVERRIDE=true → running 'kc.sh import --dir /opt/keycloak/data/import --override true' first"
    /opt/keycloak/bin/kc.sh import --dir /opt/keycloak/data/import --override true
    echo "entrypoint: override import finished"
fi

exec /opt/keycloak/bin/kc.sh "$@"