#!/usr/bin/env bash
# Подготовка чистого сервера Ubuntu 22.04/24.04 (например, VPS Timeweb Cloud)
# под socialnet. Запускать от root, можно повторно — сделанное пропускается.
#
#   scp deploy/setup-server.sh root@IP:/root/
#   ssh root@IP bash /root/setup-server.sh hike.example.ru admin@example.ru
#
# Что делает: обновления и автообновления безопасности, Docker, файрвол
# (открыты только SSH, 80 и 443), fail2ban, swap, вход по SSH только по
# ключу (если ключ уже добавлен), пользователь socialnet, ключ для доступа
# к закрытому репозиторию, .env со случайными секретами.
set -euo pipefail

DOMAIN="${1:-}"
ACME_EMAIL="${2:-}"
REPO="${REPO:-git@github.com:andrk774/socialnet.git}"
APP_DIR="${APP_DIR:-/opt/socialnet}"
APP_USER="${APP_USER:-socialnet}"
# Зеркало Docker Hub, если образы с hub.docker.com не скачиваются:
#   DOCKER_MIRROR=https://dockerhub.timeweb.cloud bash setup-server.sh ...
DOCKER_MIRROR="${DOCKER_MIRROR:-}"

say() { printf '\n\033[1;32m== %s\033[0m\n' "$*"; }
die() { printf '\033[1;31mОшибка: %s\033[0m\n' "$*" >&2; exit 1; }

[[ $EUID -eq 0 ]] || die "запустите от root (sudo bash $0 ...)"
[[ -n "$DOMAIN" && -n "$ACME_EMAIL" ]] || die "использование: $0 домен почта-для-lets-encrypt"
# shellcheck source=/dev/null
. /etc/os-release
[[ "${ID:-}" == "ubuntu" ]] || die "скрипт рассчитан на Ubuntu, а здесь ${PRETTY_NAME:-неизвестная ОС}"

say "Обновления системы"
export DEBIAN_FRONTEND=noninteractive
apt-get update -q
apt-get upgrade -yq
apt-get install -yq ca-certificates curl git gnupg ufw fail2ban unattended-upgrades openssl
dpkg-reconfigure -f noninteractive unattended-upgrades

say "Docker"
if ! command -v docker >/dev/null; then
	install -m 0755 -d /etc/apt/keyrings
	curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
	chmod a+r /etc/apt/keyrings/docker.asc
	echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu ${VERSION_CODENAME} stable" \
		>/etc/apt/sources.list.d/docker.list
	apt-get update -q
	apt-get install -yq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
fi
if [[ -n "$DOCKER_MIRROR" && ! -f /etc/docker/daemon.json ]]; then
	mkdir -p /etc/docker
	printf '{\n  "registry-mirrors": ["%s"],\n  "log-driver": "local"\n}\n' "$DOCKER_MIRROR" >/etc/docker/daemon.json
	systemctl restart docker
elif [[ ! -f /etc/docker/daemon.json ]]; then
	# журналы контейнеров с ротацией, чтобы не забить диск
	printf '{\n  "log-driver": "local"\n}\n' >/etc/docker/daemon.json
	systemctl restart docker
fi
systemctl enable --now docker

say "Файрвол: SSH, HTTP, HTTPS"
ufw default deny incoming
ufw default allow outgoing
ufw allow OpenSSH
ufw allow 80/tcp
ufw allow 443/tcp
ufw allow 443/udp
ufw --force enable
systemctl enable --now fail2ban

say "SSH"
if [[ -s /root/.ssh/authorized_keys ]]; then
	cat >/etc/ssh/sshd_config.d/10-socialnet.conf <<'CONF'
PasswordAuthentication no
KbdInteractiveAuthentication no
PermitRootLogin prohibit-password
CONF
	systemctl reload ssh 2>/dev/null || systemctl reload sshd
	echo "Вход по паролю выключен: только по SSH-ключу."
else
	echo "ВНИМАНИЕ: SSH-ключ не добавлен, вход по паролю оставлен. Добавьте ключ (ssh-copy-id root@IP) и запустите скрипт снова."
fi

say "Swap"
if ! swapon --show | grep -q .; then
	fallocate -l 2G /swapfile
	chmod 600 /swapfile
	mkswap /swapfile >/dev/null
	swapon /swapfile
	echo '/swapfile none swap sw 0 0' >>/etc/fstab
	echo "Добавлен swap 2 ГБ."
fi

say "Пользователь $APP_USER"
if ! id "$APP_USER" >/dev/null 2>&1; then
	useradd --create-home --shell /bin/bash "$APP_USER"
fi
usermod -aG docker "$APP_USER"
install -d -o "$APP_USER" -g "$APP_USER" -m 0750 "$APP_DIR"
home="$(getent passwd "$APP_USER" | cut -d: -f6)"
if [[ ! -f "$home/.ssh/id_ed25519" ]]; then
	sudo -u "$APP_USER" mkdir -p "$home/.ssh"
	sudo -u "$APP_USER" ssh-keygen -q -t ed25519 -N '' -C "socialnet-deploy@$(hostname)" -f "$home/.ssh/id_ed25519"
fi
sudo -u "$APP_USER" sh -c "ssh-keyscan -t ed25519 github.com >>'$home/.ssh/known_hosts' 2>/dev/null; sort -u -o '$home/.ssh/known_hosts' '$home/.ssh/known_hosts'"

say "Код"
if [[ ! -d "$APP_DIR/.git" ]]; then
	if ! sudo -u "$APP_USER" git clone -q "$REPO" "$APP_DIR"; then
		echo
		echo "Репозиторий закрытый: добавьте этот ключ в GitHub → репозиторий → Settings → Deploy keys"
		echo "(Add deploy key, без галочки «Allow write access»):"
		echo
		cat "$home/.ssh/id_ed25519.pub"
		echo
		echo "и запустите скрипт ещё раз."
		exit 1
	fi
fi

say ".env"
env_file="$APP_DIR/.env"
if [[ ! -f "$env_file" ]]; then
	sudo -u "$APP_USER" cp "$APP_DIR/.env.example" "$env_file"
	set_env() {
		local key="$1" value="$2"
		if grep -q "^${key}=" "$env_file"; then
			sed -i "s|^${key}=.*|${key}=${value}|" "$env_file"
		else
			printf '%s=%s\n' "$key" "$value" >>"$env_file"
		fi
	}
	set_env POSTGRES_PASSWORD "$(openssl rand -hex 24)"
	set_env JWT_SECRET "$(openssl rand -hex 32)"
	set_env TOTP_ENC_KEY "$(openssl rand -hex 32)"
	set_env REDIS_PASSWORD "$(openssl rand -hex 24)"
	set_env BACKUP_ENC_KEY "$(openssl rand -hex 32)"
	set_env METRICS_TOKEN "$(openssl rand -hex 24)"
	set_env GRAFANA_ADMIN_PASSWORD "$(openssl rand -hex 16)"
	set_env DOMAIN "$DOMAIN"
	set_env ACME_EMAIL "$ACME_EMAIL"
	set_env APP_URL "https://$DOMAIN"
	set_env COOKIE_SECURE true
	set_env SMTP_HOST ""
	set_env MAIL_FROM "socialnet <no-reply@$DOMAIN>"
	chmod 600 "$env_file"
	chown "$APP_USER:$APP_USER" "$env_file"
	echo "Создан $env_file со случайными секретами."
	echo
	echo "Сохраните BACKUP_ENC_KEY вне сервера (менеджер паролей): без него копии не расшифровать."
	grep '^BACKUP_ENC_KEY=' "$env_file"
else
	echo "$env_file уже есть — не трогаю."
fi

say "Готово"
cat <<TXT
Дальше:
  1. Проверьте, что DNS-запись A для $DOMAIN указывает на этот сервер:  dig +short $DOMAIN
  2. Заполните в $env_file: SMTP_* (почта), LEGAL_* (реквизиты оператора),
     BACKUP_S3_* (внешнее хранилище копий), при оплате — YOOKASSA_*.
  3. Запуск и обновления:  sudo -iu $APP_USER $APP_DIR/deploy/update.sh
  4. Первый администратор: зарегистрируйтесь на https://$DOMAIN и выполните
     sudo -iu $APP_USER sh -c 'cd $APP_DIR && docker compose -f docker-compose.yml -f docker-compose.prod.yml exec api /api make-admin ваша@почта'
TXT
