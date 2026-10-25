.PHONY: up down logs build test test-db psql fmt

up:            ## собрать и запустить всё в Docker
	docker compose up -d --build

down:          ## остановить
	docker compose down

logs:          ## логи API
	docker compose logs -f api

build:         ## собрать бинарник локально
	go build -o bin/api ./cmd/api

test:          ## юнит-тесты (без базы)
	go test ./...

# Интеграционные тесты против Postgres из docker compose.
# ВНИМАНИЕ: тесты пересоздают таблицы, поэтому используется отдельная база socialnet_test.
test-db:
	docker compose exec -T db psql -U socialnet -d socialnet -c 'CREATE DATABASE socialnet_test' 2>/dev/null || true
	TEST_DATABASE_URL="postgres://socialnet:$$(grep ^POSTGRES_PASSWORD .env | cut -d= -f2)@127.0.0.1:5432/socialnet_test?sslmode=disable" \
		go test -count=1 -v ./...

psql:          ## консоль базы
	docker compose exec db psql -U socialnet -d socialnet

fmt:
	gofmt -w .
