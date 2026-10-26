FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine@sha256:b8c9b82affbbb82a020bbcf998dad2ba9516944ac7e883524642a481a9505e9c AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG REVISION=unknown
WORKDIR /src
COPY go.mod go.sum ./
COPY db ./db
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -buildvcs=false \
    -ldflags="-s -w -X github.com/jonesrussell/northway/internal/app.Version=${VERSION} -X github.com/jonesrussell/northway/internal/app.Revision=${REVISION}" \
    -o /out/northway ./cmd/northway
RUN mkdir -p /out/data/northway && chmod 0700 /out/data/northway

FROM scratch
ARG REVISION=unknown
LABEL org.opencontainers.image.source="https://github.com/jonesrussell/northway" \
      org.opencontainers.image.revision="${REVISION}"
COPY --from=build /out/northway /northway
# Trust roots come from the same pinned builder; no custom publisher trust.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /usr/local/go/LICENSE /licenses/Go-LICENSE
COPY THIRD_PARTY_NOTICES.txt /licenses/THIRD_PARTY_NOTICES.txt
# Volume roots may be initialized with engine-specific permissions. Keep the
# private database directory as a copied child whose ownership/mode are explicit.
COPY --from=build --chown=65532:65532 --chmod=0700 /out/data/ /data/
USER 65532:65532
ENV NORTHWAY_LISTEN_ADDR=0.0.0.0:8080
EXPOSE 8080
ENTRYPOINT ["/northway"]
CMD ["serve"]
