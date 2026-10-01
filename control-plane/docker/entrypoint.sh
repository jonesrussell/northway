#!/bin/sh
set -eu
if [ "${1:-}" = "php-fpm" ]; then
    umask 022
    # Scan as the private database owner; publish as root so request workers
    # cannot modify the governed artifact directory.
    artifact=$(mktemp /app/.waaseyaa/.field-access-preflight.XXXXXX)
    trap 'rm -f "$artifact"' EXIT HUP INT TERM
    su-exec www-data php /app/vendor/bin/waaseyaa field-access:preflight --format=json > "$artifact"
    chmod 0644 "$artifact"
    mv "$artifact" /app/.waaseyaa/field-access-preflight.json
    trap - EXIT HUP INT TERM
    exec "$@"
fi
exec su-exec www-data "$@"
