FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download
COPY *.go ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -o /unpush .

FROM alpine:3.22
# git and ca-certificates are needed for repo mode: cloning over HTTPS and checking out commits.
RUN apk add --no-cache git ca-certificates
COPY --from=builder /unpush /unpush
ENTRYPOINT ["/unpush"]
