#!/bin/sh
set -eu
if [ "${1:-}" = "php-fpm" ]; then
    umask 022
    php /app/vendor/bin/waaseyaa field-access:preflight --write-artifact >/dev/null
    exec "$@"
fi
exec su-exec www-data "$@"
