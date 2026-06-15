FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN -mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build go mod download
COPY *.go ./
ENV CGO_ENABLED=0
RUN -mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build go build -o /unpush .

FROM golang:1.26-alpine AS uncloud
WORKDIR /src
ENV GOBIN=/
ENV CGO_ENABLED=0
RUN -mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build go install github.com/psviderski/uncloud/cmd/uncloud@latest


FROM alpine:3.22
# git and ca-certificates are needed for repo mode: cloning over HTTPS and checking out commits.
RUN apk add --no-cache git ca-certificates
COPY --from=builder /unpush /unpush
COPY --from=uncloud /uncloud /uc

EXPOSE 8080

ENTRYPOINT ["/unpush"]
