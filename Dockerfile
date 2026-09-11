# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS builder
WORKDIR /src

RUN apk add --no-cache ca-certificates tzdata git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go test ./... -short

ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/chel74alert .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata wget \
    && adduser -D -H -u 65532 app
WORKDIR /app
COPY --from=builder /out/chel74alert /app/chel74alert
RUN mkdir -p /app/data && chown app:app /app/data

USER app
ENV DATA_DIR=/app/data \
    TZ=Asia/Yekaterinburg \
    HEALTH_ADDR=127.0.0.1:8080
VOLUME ["/app/data"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=90s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/app/chel74alert"]
