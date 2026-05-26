FROM golang:1.26 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 go build -o /deployer .

FROM debian:bookworm-slim
# git and ca-certificates are needed for DEPLOYER_REPO mode: cloning over HTTPS and checking out commits.
RUN apt-get update && apt-get install -y --no-install-recommends git ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=builder /deployer /deployer
ENTRYPOINT ["/deployer"]
