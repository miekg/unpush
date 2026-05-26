# Build context must be the parent directory of both uncloud and uncloud-deployer.
# Example: docker build -f uncloud-deployer/Dockerfile -t uncloud-deployer .
FROM golang:1.26 AS builder
WORKDIR /src

COPY uncloud/go.mod uncloud/go.sum ./uncloud/
COPY uncloud-deployer/go.mod uncloud-deployer/go.sum ./uncloud-deployer/
RUN cd uncloud-deployer && go mod download

COPY uncloud/ ./uncloud/
COPY uncloud-deployer/ ./uncloud-deployer/
RUN cd uncloud-deployer && CGO_ENABLED=0 go build -o /deployer .

FROM gcr.io/distroless/static-debian12
COPY --from=builder /deployer /deployer
ENTRYPOINT ["/deployer"]
