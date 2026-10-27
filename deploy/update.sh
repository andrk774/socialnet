#!/usr/bin/env bash
# Обновление на сервере: свежий код, копия базы до миграций, сборка,
# перезапуск без простоя базы, проверка здоровья по HTTPS.
#
#   sudo -iu socialnet /opt/socialnet/deploy/update.sh
#
# Первый запуск делает то же самое (копии ещё нечего снимать).
set -euo pipefail

cd "$(dirname "$0")/.."
compose=(docker compose -f docker-compose.yml -f docker-compose.prod.yml)

say() { printf '\n\033[1;32m== %s\033[0m\n' "$*"; }
die() { printf '\033[1;31mОшибка: %s\033[0m\n' "$*" >&2; exit 1; }

[[ -f .env ]] || die ".env не найден: запустите сначала deploy/setup-server.sh"
if [[ "$(stat -c %a .env)" != "600" ]]; then
	chmod 600 .env
fi
domain="$(grep -E '^DOMAIN=' .env | cut -d= -f2- || true)"
[[ -n "$domain" ]] || die "в .env не задан DOMAIN"
smtp="$(grep -E '^SMTP_HOST=' .env | cut -d= -f2- || true)"
[[ -n "$smtp" ]] || die "в .env не задан SMTP_HOST: без почты не работают подтверждение и сброс пароля"

say "Код"
before="$(git rev-parse HEAD)"
git fetch -q origin
git log --oneline "HEAD..origin/master" || true
git pull -q --ff-only
after="$(git rev-parse HEAD)"

if "${compose[@]}" ps --status running --services 2>/dev/null | grep -qx db; then
	say "Копия базы перед обновлением"
	"${compose[@]}" run --rm -T backup backup || die "копия не удалась — обновление остановлено, сайт работает на прежней версии"
fi

say "Сборка"
"${compose[@]}" build --pull

say "Запуск"
"${compose[@]}" up -d --wait --wait-timeout 300 --remove-orphans

say "Проверка https://$domain/health"
ok=""
for _ in $(seq 1 30); do
	if curl -fsS --max-time 5 "https://$domain/health" >/dev/null 2>&1; then
		ok=1
		break
	fi
	sleep 2
done
if [[ -z "$ok" ]]; then
	"${compose[@]}" ps
	"${compose[@]}" logs --tail 30 caddy api
	cat <<TXT >&2

Сайт не отвечает по HTTPS. Частые причины: DNS ещё не указывает на сервер,
закрыт порт 80/443, Let's Encrypt временно ограничил выдачу.
Вернуть прежнюю версию кода:  git checkout $before && $0
(миграции базы назад не откатываются; копия снята перед обновлением)
TXT
	exit 1
fi

docker image prune -f >/dev/null
if [[ "$before" == "$after" ]]; then
	echo "Код не менялся ($after), контейнеры пересобраны."
else
	echo "Обновлено: ${before:0:7} → ${after:0:7}"
fi
echo "Готово: https://$domain"
