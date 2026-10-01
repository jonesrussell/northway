FROM php:8.5-fpm-alpine@sha256:fa01fb1645cd0fc566a5f146b099adace33b906571f972f71f2182a7c12d1cd7 AS base
RUN apk add --no-cache icu-libs libzip su-exec \
 && apk add --no-cache --virtual .build-deps icu-dev libzip-dev \
 && docker-php-ext-install intl zip && apk del .build-deps
COPY --from=composer:2.10.0@sha256:1b73755de4f19775ba6087fd5313664493e06fab72b6fc27dc2044e87bb7c4c3 /usr/bin/composer /usr/local/bin/composer
WORKDIR /app
FROM base AS deps
COPY composer.json composer.lock ./
RUN composer install --no-dev --no-plugins --no-scripts --no-interaction --no-progress --prefer-dist --optimize-autoloader
FROM base AS production
ARG REVISION=unknown
LABEL org.opencontainers.image.source="https://github.com/jonesrussell/northway" org.opencontainers.image.revision="$REVISION"
COPY --from=deps /app/vendor ./vendor
COPY composer.json composer.lock ./
COPY .waaseyaa/ ./.waaseyaa/
COPY config/ ./config/
COPY src/ ./src/
COPY templates/ ./templates/
COPY public/ ./public/
COPY bin/ ./bin/
COPY docker/entrypoint.sh /usr/local/bin/northcloud-php-entrypoint
COPY docker/worker.conf /usr/local/etc/php-fpm.d/zz-northcloud.conf
RUN composer dump-autoload --no-dev --no-scripts --optimize \
 && mkdir -p /app/storage && chown www-data:www-data /app/storage \
 && chmod 0700 /app/storage && chmod 0755 /usr/local/bin/northcloud-php-entrypoint
ENV APP_ENV=production WAASEYAA_DB=/app/storage/waaseyaa.sqlite
ENTRYPOINT ["/usr/local/bin/northcloud-php-entrypoint"]
CMD ["php-fpm", "-F"]

FROM nginx:1.29.4-alpine@sha256:4870c12cd2ca986de501a804b4f506ad3875a0b1874940ba0a2c7f763f1855b2 AS web
ARG REVISION=unknown
LABEL org.opencontainers.image.source="https://github.com/jonesrussell/northway" org.opencontainers.image.revision="$REVISION"
COPY docker/nginx.conf /etc/nginx/conf.d/default.conf
COPY public/app.css public/app.js public/home.css /usr/share/nginx/html/
EXPOSE 8081
