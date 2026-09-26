FROM golang:1.25-alpine AS builder
RUN apk add --no-cache build-base
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/iconfess-server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
 && addgroup -g 10001 -S iconfess \
 && adduser -u 10001 -S -D -H -G iconfess iconfess \
 && mkdir -p /app/data/media /tmp/iconfess \
 && chown -R 10001:10001 /app /tmp/iconfess
WORKDIR /app
COPY --from=builder --chown=10001:10001 /out/iconfess-server ./iconfess-server
USER 10001:10001
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health/live || exit 1
ENTRYPOINT ["./iconfess-server"]
