# --- сборка ---
FROM golang:1.24-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# --- рантайм: Alpine с ffmpeg (музыка и видео) и клиентом PostgreSQL 17
# (pg_dump/pg_restore для резервных копий), запуск не от root ---
FROM alpine:3.21
RUN apk add --no-cache ffmpeg ca-certificates postgresql17-client \
    && adduser -D -H -u 65532 app \
    && mkdir -p /data/media /backups \
    && chown -R 65532:65532 /data /backups
COPY --from=build /out/api /api
# pg_dump/pg_restore 17 в Alpine лежат здесь
ENV PATH=/usr/libexec/postgresql17:$PATH
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/api"]
