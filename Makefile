.PHONY: up down logs build test test-db psql fmt lint openapi fuzz

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

lint:          ## линтер (golangci-lint v2)
	golangci-lint run ./...

openapi:       ## пересобрать api/openapi.json после изменения маршрутов
	python3 api/gen_openapi.py

fuzz:          ## короткий фаззинг всех целей (по 30 с)
	@for t in "textparse FuzzExtract" "media FuzzExif" "media FuzzProcess" "av FuzzSniff" \
	          "webpush FuzzDecrypt" "webpush FuzzValidateEndpoint" "auth FuzzVerifyPassword" \
	          "auth FuzzParseAccess" "httpapi FuzzCleanText"; do \
		set -- $$t; go test ./internal/$$1 -run '^$$' -fuzz "^$$2$$" -fuzztime 30s || exit 1; done
