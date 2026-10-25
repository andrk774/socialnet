# --- сборка ---
FROM golang:1.24-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# --- рантайм: Alpine с ffmpeg (для музыки и видео), запуск не от root ---
FROM alpine:3.20
RUN apk add --no-cache ffmpeg ca-certificates \
    && adduser -D -H -u 65532 app \
    && mkdir -p /data/media \
    && chown -R 65532:65532 /data
COPY --from=build /out/api /api
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/api"]
