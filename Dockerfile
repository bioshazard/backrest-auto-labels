# syntax=docker/dockerfile:1.7
ARG GO_VERSION=1.23

FROM golang:${GO_VERSION}-bookworm AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -ldflags="-s -w" -o /out/backrest-sidecar ./cmd/backrest-sidecar

FROM gcr.io/distroless/base-debian12:nonroot AS runtime
WORKDIR /app
COPY --from=build /out/backrest-sidecar /usr/local/bin/backrest-sidecar
ENTRYPOINT ["backrest-sidecar"]
CMD ["daemon","--config","/etc/backrest/config.json","--with-events","--apply"]
