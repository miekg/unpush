FROM golang:1.26 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 go build -o /deployer .

FROM debian:bookworm-slim
COPY --from=builder /deployer /deployer
ENTRYPOINT ["/deployer"]
