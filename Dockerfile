# syntax=docker/dockerfile:1.7
#
# Build context MUST be the integration-efi repo root (dir owning go.mod):
#   docker build -f Dockerfile -t integration-efi .

FROM golang:1.25-bookworm AS build

WORKDIR /build

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    GOPROXY=https://proxy.golang.org,direct

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    sh -c 'for attempt in 1 2 3; do go mod download && exit 0; sleep 5; done; exit 1'

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -o /bin/integration-efi ./cmd/adapter

FROM gcr.io/distroless/base-debian12:nonroot

WORKDIR /app

COPY --from=build /bin/integration-efi /app/integration-efi

# 8080 health, 8081 RPC HTTP, 9079 webhook HTTP
EXPOSE 8080 8081 9079

ENTRYPOINT ["/app/integration-efi"]
