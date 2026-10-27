# Запуск на своём сервере (Timeweb Cloud)

Итог: socialnet на вашем домене по HTTPS, обновление одной командой,
ежедневные шифрованные копии базы и медиа во внешнее хранилище.

## 1. Что понадобится

- **Сервер** — облачный сервер Timeweb Cloud: Ubuntu 24.04, от 2 vCPU / 4 ГБ RAM /
  50 ГБ NVMe (видео обрабатывает ffmpeg — ему нужна память). Регион — Москва
  или Санкт-Петербург: данные россиян по 152-ФЗ хранятся в России.
- **Домен** — например, `hike-ural.ru`. В DNS создайте запись `A` на IP сервера
  (и `AAAA`, если у сервера есть IPv6). Проверка: `dig +short hike-ural.ru`.
- **Почта для писем** (подтверждение, сброс пароля) — SMTP любого сервиса:
  почта Timeweb для домена, Яндекс 360, Unisender Go, Mail.ru для бизнеса.
- **Хранилище для копий** — бакет S3 Timeweb Cloud **в другом регионе**, чем
  сервер (или другой поставщик: Yandex Object Storage, Selectel). Если сервер
  пропадёт целиком, копии останутся там.
- SSH-ключ на вашем компьютере: `ssh-keygen -t ed25519` (в Windows — в PowerShell).

## 2. Подготовка сервера

С вашего компьютера (Git Bash или PowerShell), в папке проекта:

```bash
ssh-copy-id root@IP_СЕРВЕРА            # в PowerShell: type $env:USERPROFILE\.ssh\id_ed25519.pub | ssh root@IP "cat >> ~/.ssh/authorized_keys"
scp deploy/setup-server.sh root@IP_СЕРВЕРА:/root/
ssh root@IP_СЕРВЕРА bash /root/setup-server.sh hike-ural.ru admin@hike-ural.ru
```

Скрипт обновит систему, поставит Docker, включит файрвол (открыты только 22,
80, 443), fail2ban, автообновления безопасности, swap, отключит вход по паролю.
Если образы с Docker Hub не скачиваются, добавьте зеркало Timeweb:
`DOCKER_MIRROR=https://dockerhub.timeweb.cloud bash /root/setup-server.sh …`.

Репозиторий закрытый, поэтому при первом запуске скрипт выведет **ключ
развёртывания**. Добавьте его в GitHub: репозиторий → Settings → Deploy keys →
Add deploy key (галочку «Allow write access» **не** ставьте) — и запустите
скрипт ещё раз. Он скачает код в `/opt/socialnet` и создаст `.env` со
случайными паролями и ключами.

**Сразу сохраните `BACKUP_ENC_KEY`** (скрипт его покажет) в менеджер паролей:
без него копии не расшифровать, а на сервере он пропадёт вместе с сервером.

## 3. Настройки в `.env`

```bash
ssh root@IP_СЕРВЕРА
nano /opt/socialnet/.env
```

Обязательно:

| Что | Переменные |
|---|---|
| Почта | `SMTP_HOST`, `SMTP_PORT` (обычно 587), `SMTP_USER`, `SMTP_PASSWORD`, `MAIL_FROM` |
| Оператор ПДн для документов | `LEGAL_OPERATOR` (ИП/ООО), `LEGAL_INN`, `LEGAL_ADDRESS`, `LEGAL_EMAIL` |
| Внешние копии | `BACKUP_S3_ENDPOINT=https://s3.twcstorage.ru`, `BACKUP_S3_REGION=ru-1`, `BACKUP_S3_BUCKET`, `BACKUP_S3_ACCESS_KEY`, `BACKUP_S3_SECRET_KEY` |

По желанию: `YOOKASSA_*` (приём оплаты), `SECURITY_CONTACT`, `TILE_URL` (свой
источник карт при большом трафике).

`.env` никому не показывайте и не делайте скриншоты с ним.

## 4. Запуск и обновления

```bash
sudo -iu socialnet /opt/socialnet/deploy/update.sh
```

Одна и та же команда и для первого запуска, и для каждого обновления: скачает
код, **снимет копию базы до миграций**, соберёт образы, перезапустит сервисы и
проверит `https://домен/health`. Сертификат Let's Encrypt Caddy получит сам
за минуту после старта (DNS уже должен указывать на сервер).

Сделать себя администратором — после регистрации на сайте:

```bash
sudo -iu socialnet sh -c 'cd /opt/socialnet && docker compose -f docker-compose.yml -f docker-compose.prod.yml exec api /api make-admin ваша@почта'
```

Проверить боевую конфигурацию (HTTPS, закрытые порты, cookie, IP клиентов):

```bash
cd /opt/socialnet && CHECK_HOST=hike-ural.ru python3 deploy/prod_check.py
```

## 5. Резервные копии

Сервис `backup` каждую ночь (`BACKUP_HOUR`, UTC): дамп базы → шифрование
AES-256-GCM → проверка восстановлением во временную базу → том `backups` на
сервере и бакет `BACKUP_S3_*`. Затем — шифрованная синхронизация папки медиа
(фото, документы, треки, видео) в тот же бакет, `media/`. Хранится
`BACKUP_KEEP` последних дампов (по умолчанию 14). Итог виден в админке и в
метрике `socialnet_backup`.

Копия вручную: `docker compose -f docker-compose.yml -f docker-compose.prod.yml run --rm backup backup`.

### Восстановление на новом сервере

1. Пройдите шаги 2–3, в `.env` впишите **прежний** `BACKUP_ENC_KEY` и `BACKUP_S3_*`.
2. Запустите только базу: `docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d db`.
3. Скачайте последний дамп из бакета (`backups/…dump.enc`, через панель Timeweb или rclone)
   в `/opt/socialnet/restore.dump.enc` и восстановите:
   ```bash
   docker compose -f docker-compose.yml -f docker-compose.prod.yml run --rm -v /opt/socialnet/restore.dump.enc:/restore.dump.enc:ro api restore /restore.dump.enc --yes
   ```
4. Медиа: `docker compose -f docker-compose.yml -f docker-compose.prod.yml run --rm api restore-media`.
5. `deploy/update.sh`.

Отрабатывайте восстановление хотя бы раз в квартал на отдельном сервере —
непроверенная копия не копия.

## 6. Перед открытием для людей

- Тексты `/legal/terms`, `/legal/privacy`, `/legal/consent` — шаблоны: покажите их юристу.
- Подайте уведомление об обработке персональных данных в Роскомнадзор (pd.rkn.gov.ru).
- Для приёма платежей — договор с ЮKassa и онлайн-касса (54-ФЗ).
- Мониторинг: `docker compose --profile monitoring …` (Grafana на 127.0.0.1:3001,
  смотреть через SSH-туннель `ssh -L 3001:127.0.0.1:3001 root@IP`).
