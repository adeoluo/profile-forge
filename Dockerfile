# syntax=docker/dockerfile:1
# Run with: docker run --rm -e GITHUB_TOKEN -v "$PWD:/work" -w /work profile-forge generate --config profile.yaml
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X github.com/adeoluo/profile-forge/internal/cli.Version=${VERSION}" \
    -o /out/profile-forge ./cmd/profile-forge

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/profile-forge /usr/local/bin/profile-forge
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/profile-forge"]
