FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
ENV GOARCH=amd64
ENV CGO_ENABLED=0
RUN go build -o /unpush .

FROM golang:1.26-alpine AS uncloud
WORKDIR /src
ENV GOPATH=/
ENV GOARCH=amd64
ENV CGO_ENABLED=0
RUN go install github.com/psviderski/uncloud/cmd/uncloud@latest


FROM alpine:3.22
# git and ca-certificates are needed for repo mode: cloning over HTTPS and checking out commits.
RUN apk add --no-cache git ca-certificates
COPY --from=builder /unpush /unpush
COPY --from=uncloud /bin/linux_amd64/uncloud /uc

EXPOSE 8080

ENTRYPOINT ["/unpush"]
