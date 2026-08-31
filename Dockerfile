FROM --platform=$BUILDPLATFORM golang:1.27.0-alpine@sha256:4c9fe60190a2a3350ddc51de80d0224b8a6698d12bdfc999fee45ea9d6c46dbc AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG REVISION=unknown
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -buildvcs=false \
    -ldflags="-s -w -X github.com/jonesrussell/northway/internal/app.Version=${VERSION} -X github.com/jonesrussell/northway/internal/app.Revision=${REVISION}" \
    -o /out/northway ./cmd/northway

FROM scratch
ARG REVISION=unknown
LABEL org.opencontainers.image.source="https://github.com/jonesrussell/northway" \
      org.opencontainers.image.revision="${REVISION}"
COPY --from=build /out/northway /northway
COPY --from=build /usr/local/go/LICENSE /licenses/Go-LICENSE
USER 65532:65532
ENV NORTHWAY_LISTEN_ADDR=0.0.0.0:8080
EXPOSE 8080
ENTRYPOINT ["/northway"]
CMD ["serve"]
