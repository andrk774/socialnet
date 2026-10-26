.PHONY: test-db-up smoke web vendor-preact e2e up down logs build test test-db psql fmt lint openapi fuzz

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
# Пароль берётся из самого контейнера базы — ровно тот, с которым Postgres её создал.
# Так не важно, как записан .env (кавычки, CRLF, пробелы, спецсимволы) и не задана ли
# POSTGRES_PASSWORD ещё и в переменных окружения Windows. В адрес пароль не вставляется:
# его читают из PGPASSWORD и pgx, и pg_dump, поэтому символы вроде @ / # % не ломают URL.
TEST_DB_URL = postgres://socialnet@127.0.0.1:5432/socialnet_test?sslmode=disable
TEST_DB_PASSWORD = $$(docker compose exec -T db printenv POSTGRES_PASSWORD | tr -d '\r\n')

test-db-up:    ## поднять Postgres и создать тестовую базу
	docker compose up -d --wait db
	docker compose exec -T db psql -U socialnet -d socialnet -c 'CREATE DATABASE socialnet_test' 2>/dev/null || true

test-db: test-db-up
	PGPASSWORD="$(TEST_DB_PASSWORD)" TEST_DATABASE_URL="$(TEST_DB_URL)" go test -count=1 -p 1 ./...

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
	          "auth FuzzParseAccess" "httpapi FuzzCleanText" "gpx FuzzParse"; do \
		set -- $$t; go test ./internal/$$1 -run '^$$' -fuzz "^$$2$$" -fuzztime 30s || exit 1; done

# Фронтенд: TypeScript → web/static/app (результат хранится в репозитории)
web:
	cd web && tsc -p .

# Обновить Preact: git clone --branch <версия> https://github.com/preactjs/preact /tmp/preact
vendor-preact:
	node web/tools/vendor-preact.mjs /tmp/preact

# Сквозной тест в Chromium: нужны node и playwright (npm i -g playwright &&
# npx playwright install chromium); ffmpeg необязателен — без него проверка видео пропускается
e2e: test-db-up
	PGPASSWORD="$(TEST_DB_PASSWORD)" TEST_DATABASE_URL="$(TEST_DB_URL)" NODE_PATH="$$(npm root -g)" \
		go test -count=1 -v -run TestFrontendE2E ./internal/httpapi/

# Весь стек в Docker + сквозная проверка (регистрация, видео, копия, метрики, изоляция)
smoke:
	docker compose up -d --build --wait
	python3 deploy/smoke.py
