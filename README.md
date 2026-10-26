# socialnet

Соцсеть на Go в духе ВКонтакте: профили, друзья, лента, сообщения и беседы в реальном
времени, истории, фото, музыка, видео, монеты с покупкой через ЮKassa, сообщества и
мероприятия, API для разработчиков, веб-интерфейс на Preact. Безопасность продумана с первого дня:
каждая функция покрыта тестами на типичные атаки (IDOR, перебор, XSS, CSRF и т. д.).

## Быстрый старт

```bash
cp .env.example .env
# впишите секреты (каждый: openssl rand -hex 32):
#   POSTGRES_PASSWORD, JWT_SECRET, TOTP_ENC_KEY, REDIS_PASSWORD, BACKUP_ENC_KEY

docker compose up -d --build
curl http://127.0.0.1:8080/health          # {"status":"ok"}
# интерфейс: http://127.0.0.1:8080/   документация API: http://127.0.0.1:8080/docs/

docker compose --profile s3 up -d          # + локальное S3 (SeaweedFS) для медиа и копий
docker compose --profile monitoring up -d  # + Prometheus и Grafana: http://127.0.0.1:3001
```

- Миграции применяются автоматически при старте.
- Письма (подтверждение email, сброс пароля) видны в Mailpit: http://127.0.0.1:8025
- Push в браузер (по желанию): `docker compose run --rm api gen-vapid`,
  вывод вставьте в `.env` и перезапустите.
- Назначить себя администратором:
  `docker compose exec api /api make-admin you@example.com`

## Что умеет

| Раздел | Возможности |
|---|---|
| Аккаунт | регистрация, вход, JWT + refresh-токены с ротацией, подтверждение email, сброс и смена пароля |
| Безопасность | 2FA (TOTP, Google Authenticator) с резервными кодами, список сеансов и их завершение, журнал безопасности, письмо о входе с нового устройства |
| Профиль | имя, статус, город, «о себе», аватар; приватность профиля, списка друзей и сообщений; скрытие из поиска |
| Друзья | заявки как во ВКонтакте (заявка = подписка, взаимная подписка = дружба), входящие и исходящие, блокировки |
| Посты | текст до 10 000 символов и до 10 фото, шесть реакций, ветки комментариев с ответами, редактирование, #хэштеги, @упоминания |
| Лента | посты друзей, подписок и сообществ, курсорная пагинация |
| Сообщения | личные диалоги и беседы с ролями, вложения (фото, треки, голосовые), доставка по WebSocket, «доставлено», «прочитано», «печатает…», сквозное шифрование в личных диалогах |
| Истории | фото и видео на 24 часа, отметки просмотра, список зрителей — только автору |
| Уведомления | заявки, лайки, комментарии, упоминания, подарки, переводы; мгновенно по WebSocket, push в браузер, письмо о непрочитанном; отключение типов и тихий режим |
| Статус | «в сети» / «был в …» с настройкой приватности; скрывшим время показывается «был недавно» |
| Валюта | монеты: покупка через ЮKassa с чеком 54-ФЗ и обработкой возвратов, ежедневный бонус, переводы, подарки (в т. ч. анонимные), донаты сообществам, вывод из кошелька сообщества, начисление админом |
| Музыка | загрузка MP3/OGG/FLAC/WAV/M4A, «Моя музыка», плейлисты, треки в постах, поиск, снятие по жалобе правообладателя с запретом перезаливки |
| Видео | MP4/MOV/MKV/WEBM до 1 ГБ и 30 минут, фоновая обработка в HLS 360p/720p плюс MP4 для браузеров без HLS, превью, видео в постах |
| Фото | загрузка JPEG/PNG/GIF до 10 МБ, удаление EXIF, превью, правильный поворот снимков с телефона |
| Сообщества | открытые и закрытые (вступление по заявке), роли владелец / админ / редактор / участник, мероприятия с «пойду / возможно» |
| Разработчикам | OAuth 2.1 с PKCE, токены с правами (scopes), боты сообществ, Callback API с подписью событий |
| Веб-интерфейс | Preact + TypeScript без сборщика, строгая CSP, тёмная тема, мобильная вёрстка |
| Поиск | люди и сообщества с учётом приватности и блокировок |
| Модерация | жалобы, скрытие контента, баны, роли; журнал действий админов с цепочкой хешей, детектор аномалий с письмом ответственному |
| Bug bounty | `/.well-known/security.txt` и форма приёма сообщений об уязвимостях |

## API

Все пути начинаются с `/api/v1`. Кроме `/auth/*`, нужен заголовок
`Authorization: Bearer <access_token>`. Ответы и тела запросов — JSON.

<details>
<summary><b>Аккаунт и вход</b></summary>

| Метод | Путь | Описание |
|---|---|---|
| POST | `/auth/register` | `{email, username, password, first_name, last_name}` |
| POST | `/auth/login` | `{email, password}`; при включённой 2FA вернёт `{mfa_required, mfa_token}` |
| POST | `/auth/2fa/verify` | `{mfa_token, code}` или `{mfa_token, backup_code}` |
| POST | `/auth/refresh` | новый access-токен по refresh-cookie |
| POST | `/auth/logout` | |
| POST | `/auth/email/verify` | `{token}` из письма |
| POST | `/auth/password/forgot` | `{email}`, ответ всегда 202 |
| POST | `/auth/password/reset` | `{token, password}`, завершает все сеансы |
| POST | `/users/me/email/verification` | отправить письмо повторно |
| POST | `/users/me/password` | `{current_password, new_password}` |
| POST | `/users/me/2fa/setup` | `{password}` → `{secret, otpauth_uri}` (URI — в QR-код) |
| POST | `/users/me/2fa/enable` | `{code}` → 10 резервных кодов (показываются один раз) |
| POST | `/users/me/2fa/disable` | `{password, code \| backup_code}` |
| POST | `/users/me/2fa/backup-codes` | новые резервные коды |
| GET | `/sessions` | активные сеансы, текущий помечен `current` |
| DELETE | `/sessions/{id}` | завершить сеанс |
| POST | `/sessions/revoke-others` | завершить все, кроме текущего |
| GET | `/security/events` | журнал: входы, ошибки, смены пароля, 2FA… |
</details>

<details>
<summary><b>Профили, друзья, блокировки</b></summary>

| Метод | Путь | Описание |
|---|---|---|
| GET / PATCH | `/users/me` | свой профиль / изменить `{first_name, last_name, status_text, city, about}` |
| PATCH | `/users/me/privacy` | `{profile_visibility, friends_visibility: everyone\|friends\|only_me, message_privacy: everyone\|friends\|nobody, searchable}` |
| PUT | `/users/me/avatar` | `{media_id}` или `{media_id: null}` |
| GET | `/users/{id}`, `/users/by-username/{name}` | профиль; закрытый показывает только имя (`closed: true`) |
| POST / DELETE | `/users/{id}/friend` | заявка или принятие / отмена или удаление из друзей |
| POST | `/users/{id}/friend/reject` | отклонить входящую заявку |
| GET | `/users/{id}/friends` | друзья (с учётом приватности) |
| GET | `/friends/requests?direction=incoming\|outgoing` | заявки |
| POST / DELETE | `/users/{id}/block` | заблокировать / разблокировать |
| GET | `/blocks` | мой чёрный список |
</details>

<details>
<summary><b>Посты, лента, уведомления</b></summary>

| Метод | Путь | Описание |
|---|---|---|
| POST | `/posts` | `{text, photo_ids: [], community_id?}` |
| GET / PATCH / DELETE | `/posts/{id}` | |
| POST / DELETE | `/posts/{id}/like` | |
| GET / POST | `/posts/{id}/comments` | `{text}` |
| DELETE | `/comments/{id}` | автор комментария, автор поста или админ сообщества |
| GET | `/users/{id}/posts` | стена |
| GET | `/feed` | лента |
| GET | `/hashtags/{tag}/posts` | посты по хэштегу |
| GET | `/notifications` | список + `unread` |
| POST | `/notifications/read` | `{ids: [...]}` или `{}` — все |

Пагинация: `?limit=20&before=<next_cursor>` (лента, стена, уведомления)
или `?after=<next_cursor>` (списки людей, комментарии).
</details>

<details>
<summary><b>Сообщения и WebSocket</b></summary>

| Метод | Путь | Описание |
|---|---|---|
| POST | `/users/{id}/messages` | `{body}` или `{ciphertext, nonce}` (base64) |
| GET | `/conversations` | диалоги: собеседник, последнее сообщение, `unread`, `peer_read_id` |
| GET | `/conversations/{id}/messages` | история |
| POST | `/conversations/{id}/read` | `{up_to: <id сообщения>}` |
| POST | `/conversations/{id}/typing` | «печатает…» |
| PUT | `/users/me/keys` | `{identity_key}` — публичный ключ X25519 (32 байта, base64) |
| GET | `/users/{id}/keys` | ключ собеседника |
| POST | `/ws/ticket` | одноразовый тикет для WebSocket (живёт 30 с) |
| GET | `/ws?ticket=...` | WebSocket |

Подключение:

```js
const { ticket } = await api.post("/api/v1/ws/ticket");
const ws = new WebSocket(`wss://api.example.com/api/v1/ws?ticket=${ticket}`);
ws.onmessage = (e) => {
  const { type, data } = JSON.parse(e.data);
  // type: message | delivered | read | typing | notification
};
ws.send(JSON.stringify({ type: "typing", conversation_id: 7 }));
ws.send(JSON.stringify({ type: "delivered", conversation_id: 7, up_to: 123 }));
```

Зачем тикет: браузер не может передать заголовок `Authorization` при открытии
WebSocket, а JWT в адресе попадёт в логи прокси. Тикет одноразовый и живёт 30 секунд.
</details>

<details>
<summary><b>Фото</b></summary>

| Метод | Путь | Описание |
|---|---|---|
| POST | `/media` | `multipart/form-data`, поле `file` → `{id, url, thumb_url, width, height}` |
| DELETE | `/media/{id}` | удалить своё фото (пропадёт из постов и аватара) |
| GET | `/media/{id}/full`, `/media/{id}/thumb` | без `/api/v1`, без авторизации — для `<img>` |
</details>

<details>
<summary><b>Сообщества, поиск, жалобы</b></summary>

| Метод | Путь | Описание |
|---|---|---|
| POST | `/communities` | `{slug, name, description}` |
| GET | `/communities` | мои сообщества |
| GET / PATCH | `/communities/{id}` | изменять могут админ и владелец |
| GET | `/communities/by-slug/{slug}` | |
| POST | `/communities/{id}/join`, `/leave` | |
| GET | `/communities/{id}/members`, `/posts` | |
| PUT | `/communities/{id}/members/{userId}/role` | `{role: admin\|editor\|member}` |
| DELETE | `/communities/{id}/members/{userId}` | исключить |
| GET | `/search?q=...&type=users\|communities` | |
| POST | `/reports` | `{target_type: post\|comment\|user\|community, target_id, reason}` |

Права в сообществе: владельца не трогает никто; админов назначает и снимает только
владелец; админ управляет редакторами и участниками; публикуют редакторы и выше.
</details>

<details>
<summary><b>Модерация и администрирование</b></summary>

| Метод | Путь | Кто | Описание |
|---|---|---|---|
| GET | `/mod/reports?status=open` | модератор | очередь жалоб |
| POST | `/mod/reports/{id}/resolve` | модератор | `{action: dismiss\|hide\|ban, note}` |
| POST | `/mod/users/{id}/ban`, `/unban` | модератор | бан сразу обрывает все сеансы |
| POST | `/mod/{posts\|comments\|communities}/{id}/hide`, `/unhide` | модератор | |
| GET | `/mod/log` | модератор | журнал действий |
| PUT | `/admin/users/{id}/role` | админ | `{role: user\|moderator\|admin}` |
| GET / PATCH | `/admin/vulnerability-reports[/{id}]` | админ | сообщения об уязвимостях |
| POST | `/security/vulnerability-reports` | любой | `{contact, title, description}` |
| GET | `/.well-known/security.txt` | любой | RFC 9116 |

Модератор банит только обычных пользователей, админ — ещё и модераторов.
Жалобы на треки и видео: `{target_type: "track"|"video", target_uuid}`; решение `hide` снимает
контент и запрещает повторную загрузку того же файла. Восстановить: `/mod/tracks/{id}/unhide`.
</details>

<details>
<summary><b>Статус и уведомления</b></summary>

| Метод | Путь | Описание |
|---|---|---|
| POST | `/presence` | `{ids: [...]}` до 100 → `online`, `last_seen` или `last_seen_approx` |
| PATCH | `/users/me/privacy` | `online_visibility: everyone\|friends\|only_me` |
| GET / PUT | `/users/me/notification-settings` | `{muted_types, email_digest, push_enabled, quiet_start, quiet_end, timezone}`; тихий режим — минуты от полуночи, можно «через полночь» (1380 → 480) |
| GET | `/push/vapid-public-key` | ключ для `pushManager.subscribe` |
| POST / DELETE | `/push/subscriptions` | тело — результат `PushSubscription.toJSON()` |

Push приходит, только если пользователь не в приложении (нет WebSocket), тип не отключён
и сейчас не тихий режим. Текст сообщений в push не попадает — только «Сообщение от Анны».
</details>

<details>
<summary><b>Кошелёк и подарки</b></summary>

| Метод | Путь | Описание |
|---|---|---|
| GET | `/wallet` | баланс и история (`direction: in\|out`, контрагент) |
| POST | `/wallet/transfer` | `{to_user_id, amount, memo, idempotency_key}` |
| POST | `/wallet/daily-bonus` | +5 монет раз в сутки |
| GET | `/gifts/catalog` | каталог подарков |
| POST | `/users/{id}/gifts` | `{gift_id, message, anonymous, idempotency_key}` |
| GET | `/users/{id}/gifts` | подарки в профиле |
| POST | `/communities/{id}/donate` | `{amount, memo, idempotency_key}` |
| GET | `/communities/{id}/wallet` | кошелёк сообщества (админы) |
| POST | `/communities/{id}/wallet/payout` | `{to_user_id, amount, idempotency_key}` — только владелец, только участнику |
| POST | `/admin/wallet/mint` | начисление админом |

`idempotency_key` (или заголовок `Idempotency-Key`) обязателен: клиент создаёт UUID
на операцию и повторяет его при сетевой ошибке — деньги не спишутся дважды.
Тратить монеты можно только с подтверждённым email.
</details>

<details>
<summary><b>Музыка</b></summary>

| Метод | Путь | Описание |
|---|---|---|
| POST | `/tracks` | `multipart`: `file` (до 50 МБ, до 20 минут), `title`, `artist` — иначе берутся из тегов |
| GET / DELETE | `/tracks/{id}` | удалить может загрузивший |
| POST / DELETE | `/tracks/{id}/my-music` | добавить в «Мою музыку» / убрать |
| GET | `/users/{id}/music` | «Моя музыка» (по правилам видимости профиля) |
| GET | `/users/{id}/playlists` | плейлисты |
| POST | `/playlists` | `{title}` |
| GET / PATCH / DELETE | `/playlists/{id}` | |
| POST | `/playlists/{id}/tracks` | `{track_id}` |
| DELETE | `/playlists/{id}/tracks/{trackId}` | |
| GET | `/search?type=tracks&q=...` | поиск |
| GET | `/audio/{id}.mp3` | без `/api/v1` — для `<audio>`, с перемоткой |

В посте: `POST /posts {"track_ids": [...]}`.
</details>

<details>
<summary><b>Видео</b></summary>

| Метод | Путь | Описание |
|---|---|---|
| POST | `/videos` | `multipart`: `file`, `title`, `description` → 202, `status: processing` |
| GET / PATCH / DELETE | `/videos/{id}` | пока идёт обработка, видео видит только владелец |
| GET | `/users/{id}/videos` | |
| GET | `/video/{id}/master.m3u8` | без `/api/v1` — HLS для плеера (hls.js или нативный в Safari) |
| GET | `/video/{id}/poster.jpg` | превью |

Когда видео готово, владельцу приходит уведомление `video_ready`.
В посте: `POST /posts {"video_ids": [...]}`.
</details>

Ошибки всегда в одном формате:

```json
{"error": {"code": "validation_failed", "message": "invalid input",
           "fields": {"username": "3-32 characters: latin letters, digits, underscore"}}}
```

## Безопасность

| Угроза | Защита |
|---|---|
| Утечка базы | пароли в Argon2id; refresh-токены, токены из писем и резервные коды хранятся как SHA-256; секреты 2FA зашифрованы AES-256-GCM |
| SQL-инъекции | только параметризованные запросы; имена таблиц — из белого списка |
| IDOR (доступ к чужому) | права проверяются на каждый объект; недоступное отдаётся как 404, чтобы не раскрывать существование |
| Единые правила видимости | одно SQL-условие видимости поста для ленты, стены, хэштегов и прямого запроса — расхождений нет |
| Перебор паролей и кодов | rate limit по IP на `/auth/*`; 5 попыток кода 2FA за 5 минут на аккаунт |
| Перечисление пользователей | одинаковые ответы и время для «нет такого email» и «неверный пароль»; сброс пароля всегда 202; письма уходят в фоне |
| Кража refresh-токена | ротация; повторное предъявление заменённого токена завершает все сеансы и шлёт письмо |
| Украденный access-токен | живёт 15 минут, содержит id сеанса — завершённый сеанс перестаёт работать сразу; опасные действия требуют пароль |
| Повтор кода 2FA | использованный шаг TOTP запоминается, тот же код второй раз не пройдёт |
| Подделка JWT | только HS256, `alg=none` отвергается, проверяются `exp`, `iss` и аудитория (токен второго шага не заменит обычный) |
| Mass assignment | неизвестные поля в JSON → 400 |
| XSS, кража токена | refresh-токен в `HttpOnly`-cookie; API отдаёт только JSON со строгим CSP |
| CSRF | `SameSite=Strict`, cookie видна только `/api/v1/auth` |
| Cross-Site WebSocket Hijacking | одноразовый тикет + проверка `Origin` |
| Вредоносные файлы | тип по содержимому, а не по расширению; перекодирование удаляет EXIF (GPS), «приклеенные» скрипты и архивы; SVG и прочее отклоняются |
| Декомпрессионные бомбы | размеры проверяются по заголовку до декодирования (до 8000×8000) |
| Path traversal | путь к файлу строится только из проверенного UUID |
| LIKE-инъекция в поиске | `%` и `_` экранируются |
| Внедрение заголовков в письма | переводы строк в адресе и теме запрещены |
| Спам | лимиты на сообщения (60 в минуту), загрузки, повторные письма, форму уязвимостей |
| Slowloris, большие запросы | таймаут заголовков 5 с, тело JSON ≤ 1 МБ, файл ≤ 10 МБ, дедлайны на каждый маршрут |
| Забаненный пользователь | бан и роль проверяются по базе на каждом запросе, сеансы обрываются сразу |
| Двойное списание денег | ключ идемпотентности; блокировка счетов в фиксированном порядке (без deadlock); `CHECK balance >= 0` в базе; сумма всех балансов всегда 0 — проверяется тестом с параллельной нагрузкой |
| Фарм бонусов | денежные операции только с подтверждённым email, лимит 20 операций в минуту |
| SSRF через Web Push | адрес подписки — только известные push-сервисы и только https; HTTP-клиент не ходит на внутренние адреса и не следует редиректам |
| Чтение push-уведомлений посредником | payload шифруется по RFC 8291 (проверено контрольным примером из RFC) |
| Атаки через ffmpeg | контейнер определяется по сигнатуре в Go и передаётся явно; плейлисты HLS, ffconcat, SDP и AVI (атака GAB2) отклоняются; только протокол `file`; таймауты; воркер в изолированной сети без интернета, read-only, без capabilities, с лимитами |
| Метаданные в медиа | аудио и видео перекодируются без метаданных (GPS-координаты, устройство) |
| Перезаливка снятого контента | хэш исходника снятого файла попадает в блок-лист |
| Подделка названий | из названий треков и видео вырезаются управляющие символы и символы смены направления текста |
| Исчерпание памяти при загрузке | файлы до 1 ГБ принимаются потоком на диск; не больше 2 одновременных перекодирований в API |

Осознанные компромиссы:

- `/register` сообщает, что email занят: так делают почти все соцсети ради удобства.
- Фото доступны по прямой ссылке без авторизации (как во ВКонтакте). Адрес содержит
  случайный UUID, угадать его нельзя, но тот, кому ссылку переслали, фото увидит.

## Сквозное шифрование сообщений

Сервер не участвует в шифровании и не видит содержимого: он хранит публичные ключи
и пересылает шифротекст. Схема на клиенте (например, через WebCrypto или libsodium):

1. При первом входе клиент создаёт пару ключей X25519, приватный хранит у себя
   (IndexedDB), публичный отправляет в `PUT /users/me/keys`.
2. Перед отправкой берёт ключ собеседника `GET /users/{id}/keys`, считает общий
   секрет X25519, выводит из него ключ через HKDF-SHA256.
3. Шифрует сообщение AES-256-GCM со случайным nonce и отправляет
   `{ciphertext, nonce}` вместо `{body}`.

Ограничения простой схемы: нет прямой секретности (forward secrecy) и нескольких
устройств на один ключ. Для этого нужен протокол Signal (Double Ratchet) —
используйте готовую библиотеку `libsignal`, не пишите криптографию сами.
Отпечаток ключа собеседника стоит показывать пользователю для сверки.

## Тесты

```bash
make test       # юнит-тесты без базы
make test-db    # все тесты на Postgres из compose (отдельная база socialnet_test)
make e2e        # сценарий в Chromium через Playwright (нужны node и playwright)
make smoke      # весь стек в Docker + deploy/smoke.py: API, видео через воркер,
                # резервная копия, метрики, изоляция контейнеров
make fuzz       # фаззинг всех целей
```

Сквозные сценарии через HTTP, настоящий WebSocket-клиент, настоящий ffmpeg и два
экземпляра API на общем Redis; контрольные векторы RFC 6238 (TOTP), RFC 8291 (Web Push) и
AWS SigV4; имитации ЮKassa и S3 с проверкой подписи каждого запроса; резервная копия с
восстановлением в настоящий Postgres; браузерный тест (регистрация, публикация, попытка XSS,
реакции, комментарии, переписка двух пользователей). Любая запись `ERROR` в логе сервера во
время теста роняет тест.

## Настройки (.env)

| Переменная | По умолчанию | Описание |
|---|---|---|
| `POSTGRES_PASSWORD` | — | пароль БД, обязателен |
| `JWT_SECRET` | — | ключ подписи JWT, не короче 32 байт |
| `TOTP_ENC_KEY` | из `JWT_SECRET` | ключ шифрования секретов 2FA, 64 hex-символа |
| `ACCESS_TOKEN_TTL` / `REFRESH_TOKEN_TTL` | `15m` / `720h` | сроки токенов |
| `COOKIE_SECURE` | `true` | `false` только для локальной разработки по http |
| `TRUST_PROXY` | `false` | `true`, если перед API стоит ваш Nginx |
| `AUTH_RATE_PER_MIN` / `AUTH_RATE_BURST` | `10` / `20` | лимит `/auth/*` с одного IP |
| `APP_URL` | `http://localhost:3000` | адрес фронтенда для ссылок в письмах |
| `ALLOWED_ORIGINS` | пусто | домены фронтенда для WebSocket |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`, `MAIL_FROM` | Mailpit | почта |
| `SECURITY_CONTACT` | `security@localhost` | контакт в security.txt |
| `MEDIA_DIR` | `./data/media` (в Docker `/data/media`) | где хранить фото, музыку и видео |
| `VAPID_PUBLIC_KEY` / `VAPID_PRIVATE_KEY` | пусто (push выключен) | ключи Web Push, `api gen-vapid` |
| `REDIS_URL` | пусто (всё в памяти) | лимиты, тикеты WebSocket и события между экземплярами API |
| `S3_ENDPOINT`, `S3_BUCKET`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_REGION`, `S3_PATH_STYLE` | пусто (диск) | объектное хранилище для медиа |
| `MEDIA_CDN_URL` | пусто | CDN перед бакетом; голосовые через CDN не отдаются никогда |
| `YOOKASSA_SHOP_ID`, `YOOKASSA_SECRET_KEY` | пусто (покупка выключена) | ключи магазина ЮKassa |
| `YOOKASSA_VAT_CODE` | `1` | ставка НДС в чеке: 1 — без НДС, 11 — 22%, 12 — 22/122 |
| `YOOKASSA_CHECK_IP` | `true` | принимать уведомления только с адресов ЮKassa |
| `BACKUP_ENC_KEY` | пусто | ключ шифрования копий, 64 hex-символа — храните отдельно от копий |
| `BACKUP_DIR`, `BACKUP_KEEP`, `BACKUP_HOUR`, `BACKUP_S3` | `/backups`, `14`, `3`, `false` | резервные копии |
| `METRICS_ADDR`, `METRICS_TOKEN` | `:9090`, пусто | адрес `/metrics` (наружу не публикуется) |

## На сервер

1. Поставьте перед API Nginx или Caddy с HTTPS, выставьте `TRUST_PROXY=true` и `COOKIE_SECURE=true`.
   Для WebSocket в Nginx нужны заголовки `Upgrade` и `Connection`.
2. Порты 8080 и 5432 в compose слушают только `127.0.0.1`, наружу их не открывайте.
3. Подключите настоящий SMTP и удалите сервис `mailpit`.
4. Задайте отдельный `TOTP_ENC_KEY` и храните его вместе с бэкапами.
5. Сервис `backup` раз в сутки снимает копию базы, шифрует, проверяет восстановлением во
   временную базу и пишет итог в метрику `socialnet_backup` (алерт — если копии нет больше 26 часов).
   Восстановление: `docker compose run --rm backup restore /backups/<файл> --yes`.
   Медиа держите в S3 с версионированием или бэкапьте том `media`.
6. URL уведомлений ЮKassa в личном кабинете: `https://<домен>/api/v1/payments/yookassa`;
   включите события `payment.succeeded`, `payment.canceled` и `refund.succeeded`.
   Возврат списывает монеты пропорционально возвращённой сумме; если их уже
   потратили, остаток становится долгом — тратить монеты нельзя, пока новые
   поступления его не погасят.
7. Видео, обработанные до появления MP4, досоздаются командой
   `docker compose run --rm api backfill-mp4` (задачи выполнит воркер).
8. Видео тяжёлое: следите за местом на диске и при нагрузке запускайте больше воркеров
   (`docker compose up -d --scale worker=3`) — очередь сама распределит задачи.

## Масштабирование

API можно запускать в нескольких экземплярах: с `REDIS_URL` лимиты, одноразовые тикеты
WebSocket и события общие (сообщение, отправленное через один экземпляр, приходит
подключённому к другому), фото, музыка и видео — в S3, детектор аномалий работает только в
одном экземпляре (advisory lock). Следующий шаг при росте — отдельный поиск (Meilisearch).

## Структура

```
cmd/api/main.go        запуск; команды make-admin, gen-vapid, worker, backup, backup-loop, restore
internal/config        настройки из окружения
internal/db            пул соединений и миграции (0001..0008)
internal/auth          Argon2id, JWT, TOTP, резервные коды, AES-GCM
internal/store         все запросы к PostgreSQL
internal/httpapi       маршруты, middleware, обработчики, тесты
internal/realtime      хаб WebSocket и шина событий на Redis
internal/redisx        клиент Redis (RESP2, pub/sub)
internal/s3            клиент S3 с подписью SigV4; s3test — S3 в памяти для тестов
internal/safehttp      HTTP-клиент без доступа во внутреннюю сеть (Web Push, Callback API)
internal/yookassa      клиент ЮKassa
internal/backup        резервные копии: pg_dump, шифрование, проверка восстановлением
internal/metrics       метрики в формате Prometheus
internal/media         обработка изображений и хранилище фото
internal/av            ffmpeg: проверка форматов, перекодирование, HLS, хранилище
internal/worker        фоновая очередь задач (видео)
internal/mailer        SMTP, лог-заглушка и ящик для тестов
internal/textparse     #хэштеги и @упоминания
internal/webpush       Web Push: VAPID, шифрование RFC 8291
web/ts/app             исходники интерфейса (TypeScript); web/static — собранное и Preact
deploy/                Prometheus, алерты, Grafana, SeaweedFS
```

## Что можно сделать дальше

- Нативные push на телефон (Firebase / APNs) и мобильное приложение поверх того же API.
- Несколько качеств MP4 (сейчас для браузеров без HLS собирается одно — лучшее).
- Отдельный поисковый движок при росте базы.
