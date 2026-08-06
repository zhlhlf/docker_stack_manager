FROM golang:1.22-alpine AS builder
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags "-s -w -X main.Version=${VERSION}" \
    -o /out/docker_stack_manager .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/docker_stack_manager /usr/local/bin/docker_stack_manager
ENV LISTEN_ADDR=:8080
ENV DB_PATH=/data/data.json
EXPOSE 8080
VOLUME ["/data"]
# Mount host docker socket: -v /var/run/docker.sock:/var/run/docker.sock
ENTRYPOINT ["docker_stack_manager"]