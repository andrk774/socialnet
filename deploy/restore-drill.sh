#!/usr/bin/env bash
# Учебное восстановление из резервной копии на ОТДЕЛЬНОМ сервере.
#
#   deploy/restore-drill.sh /путь/socialnet-20261027-030000.dump.enc
#
# Что делает: поднимает базу с нуля, восстанавливает дамп, запускает весь
# стек, проверяет, что сайт отвечает и данные на месте, и печатает отчёт:
# сколько заняло восстановление (RTO) и насколько свежие данные в копии (RPO).
#
# Подготовка сервера для учений:
#   1. Клон репозитория (та же версия, что на боевом, или новее).
#   2. .env: прежние BACKUP_ENC_KEY и JWT_SECRET, остальное — любые новые
#      пароли. DOMAIN не нужен: учения идут без Caddy, сайт на 127.0.0.1:8080.
#   3. Дамп из бакета (backups/…dump.enc) — скачать на сервер.
#
# Скрипт отказывается работать, если на сервере уже есть данные: это
# защита от случайного запуска на боевом сервере. Убрать учебный стек:
#   docker compose down -v
#
# Переменные: COMPOSE (по умолчанию "docker compose"), MIN_USERS — сколько
# пользователей как минимум должно быть в копии (по умолчанию 1),
# MAX_AGE_HOURS — копия старше считается провалом (по умолчанию 48).
set -euo pipefail

dump=${1:-}
[[ -n "$dump" && -f "$dump" ]] || { echo "usage: $0 <файл .dump или .dump.enc>" >&2; exit 2; }
dump=$(cd "$(dirname "$dump")" && pwd)/$(basename "$dump")
cd "$(dirname "$0")/.."
read -ra compose <<<"${COMPOSE:-docker compose}"
min_users=${MIN_USERS:-1}
max_age_h=${MAX_AGE_HOURS:-48}

die() { echo "✗ $*" >&2; exit 1; }
say() { echo "• $*"; }
[[ -f .env ]] || die "нет .env (нужны прежние BACKUP_ENC_KEY и JWT_SECRET)"
grep -q '^BACKUP_ENC_KEY=.' .env || [[ "$dump" != *.enc ]] || die "в .env нет BACKUP_ENC_KEY — зашифрованный дамп не прочитать"

# защита: только на пустом сервере
project=$("${compose[@]}" config --format json | python3 -c 'import json,sys; print(json.load(sys.stdin)["name"])')
if docker volume inspect "${project}_pgdata" >/dev/null 2>&1; then
	die "том ${project}_pgdata уже существует — здесь есть данные. Учения — только на отдельном пустом сервере (или после docker compose down -v на учебном)."
fi

start=$(date +%s)
say "база с нуля"
"${compose[@]}" up -d --wait db redis

say "восстановление $(basename "$dump") ($(du -h "$dump" | cut -f1))"
# контейнер работает не от root: отдаём ему копию файла, доступную на чтение
# (дамп зашифрован; копия удаляется по выходе)
ext=.dump; [[ "$dump" == *.enc ]] && ext=.dump.enc
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
install -m 0644 "$dump" "$tmp/restore$ext"; chmod 0755 "$tmp"
"${compose[@]}" run --rm -T -v "$tmp/restore$ext:/restore$ext:ro" api restore "/restore$ext" --yes

say "весь стек (миграции новой версии применятся при старте)"
"${compose[@]}" up -d --wait --wait-timeout 300
rto=$(( $(date +%s) - start ))

sql() { "${compose[@]}" exec -T db psql -U socialnet -d socialnet -Atc "$1"; }
fail=0
# check "что проверяли" 1|0
check() { if [[ "$2" == 1 ]]; then echo "  ✓ $1"; else echo "  ✗ $1"; fail=1; fi; }

say "проверки"
health=$("${compose[@]}" exec -T api /api probe http://127.0.0.1:8080/health 2>&1 || true)
ok=0; [[ "$health" == *ok* ]] && ok=1
check "API отвечает на /health" "$ok"
users=$(sql "SELECT count(*) FROM users")
posts=$(sql "SELECT count(*) FROM posts")
events=$(sql "SELECT count(*) FROM events")
routes=$(sql "SELECT count(*) FROM routes")
check "пользователей: $users (нужно ≥ $min_users)" "$(( users >= min_users ))"
admins=$(sql "SELECT count(*) FROM users WHERE role = 'admin'")
(( admins >= 1 )) || echo "  ! нет ни одного администратора — назначьте: ${compose[*]} run --rm api make-admin почта"
newest=$(sql "SELECT COALESCE(extract(epoch FROM max(t))::bigint, 0) FROM (
	SELECT max(created_at) t FROM users UNION ALL SELECT max(created_at) FROM posts
	UNION ALL SELECT max(created_at) FROM messages) x")
age_h=$(( ($(date +%s) - newest) / 3600 ))
check "свежесть данных: последняя запись $age_h ч назад (допустимо ≤ $max_age_h)" "$(( age_h <= max_age_h ))"
pw=$(sql "SELECT count(*) FROM users WHERE password_hash NOT LIKE '\$argon2id\$%'")
check "пароли только в виде Argon2id-хэшей" "$(( pw == 0 ))"
migr=$(sql "SELECT count(*) FROM schema_migrations" 2>/dev/null || echo "?")

cat <<EOF

Отчёт учений $(date -u +%F)
  Копия:         $(basename "$dump")
  RTO:           $((rto / 60)) мин $((rto % 60)) с (от пустого сервера до работающего сайта)
  RPO:           ~$age_h ч (возраст последней записи в копии)
  Данные:        пользователей $users, записей $posts, походов/событий $events, маршрутов $routes
  Миграций:      $migr
  Медиа:         не проверялись — восстановите: ${compose[*]} run --rm api restore-media
EOF
if (( fail )); then die "учения провалены — разберитесь до того, как копия понадобится по-настоящему"; fi
echo "✓ учения пройдены. Запишите отчёт в журнал (deploy/LAUNCH.md, раздел «Учения»)."
