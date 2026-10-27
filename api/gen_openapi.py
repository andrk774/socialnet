#!/usr/bin/env python3
"""Генератор api/openapi.json.

Источник правды — описания ниже. После добавления маршрута в server.go
добавьте его сюда и запустите:  python3 api/gen_openapi.py
Тест TestOpenAPICoversRoutes упадёт, если спецификация и роутер разошлись.
"""
import json, os, re

S = lambda name: {"$ref": f"#/components/schemas/{name}"}

def obj(**props):
    req = [k.rstrip("?") for k in props if not k.endswith("?")]
    p = {k.rstrip("?"): v for k, v in props.items()}
    o = {"type": "object", "properties": p}
    if req:
        o["required"] = req
    return o

str_ = {"type": "string"}
int_ = {"type": "integer", "format": "int64"}
bool_ = {"type": "boolean"}
num = {"type": "number"}
def enum_int(*v): return {"type": "integer", "enum": list(v)}
uuid = {"type": "string", "format": "uuid"}
dt = {"type": "string", "format": "date-time"}
b64 = {"type": "string", "format": "byte", "description": "base64"}
def arr(x): return {"type": "array", "items": x}
def enum(*v): return {"type": "string", "enum": list(v)}
def nullable(x): return {**x, "nullable": True}
def lst(item): return obj(items=arr(item), next_cursor=nullable(int_))

schemas = {
    "Error": obj(error=obj(code=str_, message=str_, **{"fields?": {"type": "object", "additionalProperties": str_}})),
    "UserCard": obj(id=int_, username=str_, first_name=str_, last_name=str_, avatar_url=nullable(str_)),
    "Privacy": obj(online_visibility=enum("everyone", "friends", "only_me"),
                   profile_visibility=enum("everyone", "friends", "only_me"),
                   friends_visibility=enum("everyone", "friends", "only_me"),
                   message_privacy=enum("everyone", "friends", "nobody"), searchable=bool_),
    "Self": obj(id=int_, consent_required=bool_, plus=bool_, email=str_, email_verified=bool_, username=str_, first_name=str_, last_name=str_,
                status_text=str_, city=str_, about=str_, avatar_url=nullable(str_), role=enum("user", "moderator", "admin"),
                two_factor_enabled=bool_, privacy=S("Privacy"), created_at=dt),
    "Presence": obj(user_id=int_, **{"online?": bool_, "last_seen?": dt,
                                     "last_seen_approx?": enum("recently", "within_week", "within_month", "long_ago")}),
    "Profile": obj(id=int_, plus=bool_, username=str_, first_name=str_, last_name=str_, avatar_url=nullable(str_), closed=bool_,
                   **{"status_text?": str_, "city?": str_, "about?": str_, "created_at?": dt},
                   relationship=obj(status=enum("self", "friends", "request_sent", "request_received", "none"),
                                    i_blocked=bool_, can_write=bool_),
                   presence=S("Presence")),
    "Auth": obj(access_token=str_, token_type=str_, expires_at=dt, **{"user?": S("Self")}),
    "MFARequired": obj(mfa_required=bool_, mfa_token=str_),
    "Photo": obj(id=uuid, url=str_, thumb_url=str_),
    "PostTrack": obj(id=uuid, title=str_, artist=str_, duration_sec=int_, url=str_),
    "PostVideo": obj(id=uuid, title=str_, duration_sec=int_, poster_url=str_, hls_url=str_, **{"mp4_url?": str_}),
    "PostDocument": obj(id=uuid, title=str_, ext=str_, size=int_, url=str_),
    "Document": obj(id=uuid, owner_id=int_, title=str_, ext=str_, size=int_, url=str_, created_at=dt),
    "Route": obj(id=uuid, owner_id=int_, title=str_, distance_m=int_, gain_m=int_, loss_m=int_,
                 min_ele=nullable(num), max_ele=nullable(num), duration_sec=int_, moving_sec=int_, points=int_,
                 hidden_ends_m=enum_int(0, 200, 500, 1000), bbox=arr(num), preview=arr(arr(num)),
                 profile=arr(obj(d=num, e=num)), gpx_url=str_, public=bool_, public_note=str_, created_at=dt,
                 **{"detail?": arr(arr(arr(num))), "waypoints?": arr(obj(lat=num, lon=num, name=str_, **{"ele?": num})),
                    "marks?": arr(S("RouteMark"))}),
    "RouteMark": obj(id=int_, route_id=uuid, author=S("UserCard"), kind=enum("spring", "camp", "ford", "danger", "view", "other"),
                     lat=num, lon=num, title=str_, note=str_, photo_url=nullable(str_), thumb_url=nullable(str_),
                     verified_on={"type": "string", "format": "date"}, checks=int_, my_check=bool_, created_at=dt),
    "Poll": obj(id=int_, question=str_, multiple=bool_, anonymous=bool_, closes_at=nullable(dt), closed=bool_,
                voters_count=int_, results_hidden=bool_, my_votes=arr(int_),
                options=arr(obj(id=int_, text=str_, votes=nullable(int_)))),
    "Post": obj(id=int_, author=S("UserCard"), **{"community?": obj(id=int_, slug=str_, name=str_)}, text=str_,
                photos=arr(S("Photo")), tracks=arr(S("PostTrack")), videos=arr(S("PostVideo")),
                documents=arr(S("PostDocument")), routes=arr(S("Route")), poll=nullable(S("Poll")),
                audience=enum("everyone", "friends", "only_me", "list"), bookmarked=bool_,
                likes_count=int_, comments_count=int_, liked_by_me=bool_,
                my_reaction=nullable(enum("like", "love", "haha", "wow", "sad", "angry")),
                reactions={"type": "object", "additionalProperties": int_},
                created_at=dt, **{"edited_at?": dt}),
    "Comment": obj(id=int_, post_id=int_, **{"parent_id?": int_, "reply_to?": S("UserCard")}, replies_count=int_,
                   author=S("UserCard"), text=str_, created_at=dt),
    "Attachment": obj(kind=enum("photo", "track", "voice", "document"), id=uuid, url=str_,
                      **{"thumb_url?": str_, "title?": str_, "artist?": str_, "duration_sec?": int_, "ext?": str_, "size?": int_}),
    "ChatMember": obj(id=int_, username=str_, first_name=str_, last_name=str_, avatar_url=nullable(str_),
                      role=enum("owner", "admin", "member")),
    "Story": obj(id=int_, author_id=int_, **{"photo?": S("Photo"), "video?": S("PostVideo"), "views_count?": int_},
                 caption=str_, created_at=dt, expires_at=dt, seen=bool_),
    "StoryGroup": obj(author=S("UserCard"), all_seen=bool_, stories=arr(S("Story"))),
    "Event": obj(id=int_, creator_id=int_, **{"community?": obj(id=int_, slug=str_, name=str_), "ends_at?": dt},
                 kind=enum("event", "hike"), title=str_, description=str_, place=str_, starts_at=dt,
                 difficulty=nullable(enum_int(1, 2, 3, 4, 5)), capacity=nullable(int_), gear=str_, route_id=nullable(str_),
                 meeting_point=str_, approval=bool_,
                 going_count=int_, maybe_count=int_, pending_count=int_, waitlist_count=int_,
                 my_status=nullable(enum("going", "maybe", "pending", "waitlist")), **{"my_waitlist_position?": int_},
                 control_at=nullable(dt), returned_at=nullable(dt), alert_sent=bool_, created_at=dt),
    "CompanionAd": obj(id=int_, author=S("UserCard"), region=str_, title=str_, body=str_,
                       date_from={"type": "string", "format": "date"}, date_to={"type": "string", "format": "date"},
                       difficulty=nullable(enum_int(1, 2, 3, 4, 5)), seats=int_, route_id=nullable(str_), closed=bool_, created_at=dt),
    "Span": obj(t=enum("text", "b", "i", "a"), text=str_, **{"href?": str_}),
    "Block": obj(kind=enum("h2", "h3", "p", "ul", "ol", "quote", "hr", "img"),
                 **{"spans?": arr(S("Span")), "items?": arr(arr(S("Span"))), "media_id?": uuid, "caption?": str_}),
    "Article": obj(id=int_, author=S("UserCard"), title=str_, cover_url=nullable(str_), cover_thumb_url=nullable(str_),
                   cover_media_id=nullable(uuid), route_id=nullable(uuid), status=enum("draft", "published"), excerpt=str_,
                   created_at=dt, updated_at=dt, published_at=nullable(dt), **{"body?": str_, "blocks?": arr(S("Block"))}),
    "Ride": obj(id=int_, driver=S("UserCard"), event_id=nullable(int_), **{"from": str_}, to=str_, departs_at=dt, seats=int_,
                taken=int_, pending=int_, note=str_, my_status=nullable(enum("pending", "accepted")), created_at=dt),
    "Application": obj(user=S("UserCard"), note=str_, created_at=dt),
    "Notification": obj(id=int_, type=str_, **{"actor?": S("UserCard"), "post_id?": int_, "comment_id?": int_, "event_id?": int_, "ride_id?": int_},
                        created_at=dt, read=bool_),
    "Message": obj(id=int_, conversation_id=int_, sender_id=int_, kind=enum("text", "service"),
                   **{"body?": str_, "ciphertext?": b64, "nonce?": b64},
                   encrypted=bool_, attachments=arr(S("Attachment")), created_at=dt),
    "Conversation": obj(id=int_, kind=enum("direct", "group"), **{"title?": str_, "members_count?": int_, "peer?": S("UserCard")},
                        last_message=S("Message"), unread=int_, peer_delivered_id=int_, peer_read_id=int_),
    "Community": obj(id=int_, slug=str_, name=str_, description=str_, owner_id=int_, members_count=int_,
                     my_role=enum("", "member", "editor", "admin", "owner"), visibility=enum("open", "closed"), created_at=dt),
    "Member": obj(id=int_, username=str_, first_name=str_, last_name=str_, avatar_url=nullable(str_),
                  role=enum("owner", "admin", "editor", "member")),
    "Session": obj(id=uuid, user_agent=str_, ip=nullable(str_), started_at=dt, last_active=dt, current=bool_),
    "SecurityEvent": obj(id=int_, type=str_, ip=nullable(str_), user_agent=str_, created_at=dt),
    "Report": obj(id=int_, reporter=S("UserCard"), target_type=str_, target_id=int_, **{"target_uuid?": uuid},
                  reason=str_, status=enum("open", "resolved", "dismissed"), created_at=dt),
    "ModAction": obj(id=int_, moderator_id=int_, action=str_, target_type=str_, target_id=int_, note=str_, created_at=dt),
    "VulnReport": obj(id=int_, contact=str_, title=str_, description=str_, status=str_, created_at=dt),
    "NotificationSettings": obj(muted_types=arr(str_), email_digest=bool_, push_enabled=bool_,
                                quiet_start=nullable(int_), quiet_end=nullable(int_), timezone=str_),
    "Tx": obj(id=int_, kind=str_, direction=enum("in", "out"), amount=int_,
              counterparty=obj(type=enum("user", "community", "system"), id=int_, name=str_), memo=str_, created_at=dt),
    "TxResult": obj(transaction_id=int_, amount=int_, replayed=bool_, balance=int_),
    "Gift": obj(id=int_, gift=obj(id=int_, name=str_, price=int_), sender=nullable(S("UserCard")), message=str_, created_at=dt),
    "Track": obj(id=uuid, title=str_, artist=str_, duration_sec=int_, url=str_, uploader_id=int_, in_my_music=bool_, created_at=dt),
    "Playlist": obj(id=int_, owner_id=int_, title=str_, track_count=int_, **{"tracks?": arr(S("Track"))}, created_at=dt),
    "App": obj(id=int_, owner_id=int_, name=str_, client_id=str_, confidential=bool_, redirect_uris=arr(str_),
               **{"client_secret?": str_}, created_at=dt),
    "AuthorizedApp": obj(app_id=int_, name=str_, scopes=arr(str_), granted_at=dt, last_used_at=nullable(dt)),
    "BotToken": obj(id=int_, name=str_, scopes=arr(str_), created_at=dt, last_used_at=nullable(dt)),
    "CallbackServer": obj(community_id=int_, url=str_, events=arr(str_), status=enum("unconfirmed", "active", "failing"),
                          last_error=str_, updated_at=dt),
    "Video": obj(id=uuid, owner_id=int_, title=str_, description=str_, status=enum("processing", "ready", "failed"),
                 **{"duration_sec?": int_, "width?": int_, "height?": int_, "poster_url?": str_, "hls_url?": str_, "mp4_url?": str_},
                 qualities=arr(str_), created_at=dt),
}

money = dict(amount=int_, **{"memo?": str_}, idempotency_key=str_)
upload_form = lambda **extra: {"multipart/form-data": {"schema": obj(file={"type": "string", "format": "binary"}, **extra)}}

# метод, путь: (тег, описание, тело запроса | None, ответ | None, код ответа)
R = {}
def r(method, path, tag, summary, body=None, resp=None, code=200, auth=True, content=None):
    R[(method, path)] = dict(tag=tag, summary=summary, body=body, resp=resp, code=code, auth=auth, content=content)

A = "/api/v1"
SCOPES = {"profile": "Профиль, друзья, поиск", "friends": "Заявки в друзья", "wall": "Публикации и комментарии",
          "photos": "Загрузка фото", "messages": "Сообщения", "notifications": "Уведомления", "music": "Музыка",
          "video": "Видео", "communities": "Сообщества", "stories": "Истории", "events": "Мероприятия",
          "docs": "Документы", "routes": "Маршруты", "bookmarks": "Закладки", "offline": "Токен без срока действия"}
# --- служебные ---
r("GET", "/health", "service", "Проверка живости", resp=obj(status=str_), auth=False)
r("GET", "/.well-known/security.txt", "service", "security.txt (RFC 9116)", auth=False, resp="text")
r("GET", "/trails", "service", "Открытый каталог маршрутов (HTML без входа): ?page=", auth=False, resp="text")
r("GET", "/trails/{id}", "service", "Страница маршрута (HTML); {id}.gpx — очищенный GPX", auth=False, resp="text")
r("GET", "/robots.txt", "service", "robots.txt: в поиск — каталог и документы", auth=False, resp="text")
r("GET", "/sitemap.xml", "service", "Карта сайта: каталог и документы", auth=False, resp="text")
r("GET", "/sw.js", "service", "Сервис-воркер (офлайн-режим приложения)", auth=False, resp="text")
r("GET", "/manifest.webmanifest", "service", "Манифест веб-приложения", auth=False, resp="json")
r("GET", A + "/openapi.json", "service", "Эта спецификация", auth=False, resp="json")
legal_doc = obj(key=enum("terms", "privacy", "consent"), title=str_, version=str_, url=str_)
r("GET", A + "/legal", "service", "Юридические документы и текущая редакция", auth=False, resp=obj(revision=int_, documents=arr(legal_doc)))
r("GET", "/legal/{doc}", "service", "Документ HTML-страницей: terms, privacy, consent", auth=False, resp="text")
r("GET", A + "/users/me/consents", "auth", "Принятые документы, журнал согласий; required — нужно принять новую редакцию",
  resp=obj(required=bool_, revision=int_, accepted_revision=int_, documents=arr(legal_doc),
           log=arr(obj(document=str_, version=str_, action=enum("accept", "revoke"), created_at=dt))))
r("POST", A + "/users/me/consents", "auth", "Принять действующую редакцию (до этого изменения запрещены: 428 consent_required)",
  obj(age_confirmed=bool_, accept_terms=bool_, pd_consent=bool_), code=204)
r("DELETE", A + "/users/me", "auth", "Удалить аккаунт и отозвать согласие на обработку ПДн: публикации удаляются, аккаунт обезличивается",
  obj(password=str_), code=204)
r("GET", A + "/client-config", "service", "Что включено на сервере (карта: адрес тайлов, атрибуция, максимальный масштаб)", auth=False,
  resp=obj(map=obj(enabled=bool_, tiles=str_, attribution=str_, max_zoom=int_), weather=obj(enabled=bool_)))
r("GET", "/tiles/{z}/{x}/{y}", "service", "Тайл карты через кеширующий прокси ({y} с суффиксом .png); 404 — карта выключена или неверные координаты", auth=False, resp="binary")
# --- аккаунт ---
r("POST", A + "/auth/register", "auth", "Регистрация (18+; три отдельные отметки: возраст, соглашение и политика, согласие на обработку ПДн)",
  obj(email=str_, username=str_, password=str_, age_confirmed=bool_, accept_terms=bool_, pd_consent=bool_,
      **{"first_name?": str_, "last_name?": str_}), S("Auth"), 201, auth=False)
r("POST", A + "/auth/login", "auth", "Вход (шаг 1). При 2FA вернёт mfa_token", obj(email=str_, password=str_), {"oneOf": [S("Auth"), S("MFARequired")]}, auth=False)
r("POST", A + "/auth/2fa/verify", "auth", "Вход (шаг 2): код 2FA или резервный код", obj(mfa_token=str_, **{"code?": str_, "backup_code?": str_}), S("Auth"), auth=False)
r("POST", A + "/auth/refresh", "auth", "Новый access-токен по refresh-cookie", resp=S("Auth"), auth=False)
r("POST", A + "/auth/logout", "auth", "Выход", code=204, auth=False)
r("POST", A + "/auth/email/verify", "auth", "Подтвердить email токеном из письма", obj(token=str_), code=204, auth=False)
r("POST", A + "/auth/password/forgot", "auth", "Запросить сброс пароля (всегда 202)", obj(email=str_), obj(message=str_), 202, auth=False)
r("POST", A + "/auth/password/reset", "auth", "Сбросить пароль токеном из письма", obj(token=str_, password=str_), code=204, auth=False)
r("POST", A + "/users/me/email/verification", "auth", "Отправить письмо подтверждения ещё раз", code=202)
r("POST", A + "/users/me/password", "auth", "Сменить пароль", obj(current_password=str_, new_password=str_), code=204)
r("POST", A + "/users/me/2fa/setup", "auth", "Начать настройку 2FA", obj(password=str_), obj(secret=str_, otpauth_uri=str_))
r("POST", A + "/users/me/2fa/enable", "auth", "Включить 2FA кодом из приложения", obj(code=str_), obj(backup_codes=arr(str_)))
r("POST", A + "/users/me/2fa/disable", "auth", "Отключить 2FA", obj(password=str_, **{"code?": str_, "backup_code?": str_}), code=204)
r("POST", A + "/users/me/2fa/backup-codes", "auth", "Новые резервные коды", obj(password=str_, **{"code?": str_, "backup_code?": str_}), obj(backup_codes=arr(str_)))
r("GET", A + "/sessions", "auth", "Активные сеансы", resp=obj(items=arr(S("Session"))))
r("DELETE", A + "/sessions/{sid}", "auth", "Завершить сеанс", code=204)
r("POST", A + "/sessions/revoke-others", "auth", "Завершить все сеансы, кроме текущего", code=204)
r("GET", A + "/security/events", "auth", "Журнал безопасности", resp=lst(S("SecurityEvent")))
# --- профиль ---
r("GET", A + "/users/me", "users", "Свой профиль", resp=S("Self"))
r("PATCH", A + "/users/me", "users", "Изменить профиль", obj(**{"first_name?": str_, "last_name?": str_, "status_text?": str_, "city?": str_, "about?": str_}), S("Self"))
r("PATCH", A + "/users/me/privacy", "users", "Настройки приватности", S("Privacy"), S("Self"))
r("PUT", A + "/users/me/avatar", "users", "Установить аватар", obj(media_id=nullable(uuid)), S("Self"))
r("GET", A + "/users/{id}", "users", "Профиль пользователя", resp=S("Profile"))
r("GET", A + "/users/by-username/{username}", "users", "Профиль по username", resp=S("Profile"))
r("POST", A + "/presence", "users", "Онлайн-статусы (до 100)", obj(ids=arr(int_)), obj(items=arr(S("Presence"))))
# --- друзья ---
r("POST", A + "/users/{id}/friend", "friends", "Отправить заявку / принять", resp=obj(status=str_))
r("DELETE", A + "/users/{id}/friend", "friends", "Отменить заявку / удалить из друзей", resp=obj(status=str_))
r("POST", A + "/users/{id}/friend/reject", "friends", "Отклонить заявку", code=204)
r("GET", A + "/users/{id}/friends", "friends", "Друзья", resp=lst(S("UserCard")))
r("GET", A + "/friends/requests", "friends", "Заявки: ?direction=incoming|outgoing", resp=lst(S("UserCard")))
r("POST", A + "/users/{id}/block", "friends", "Заблокировать", code=204)
r("DELETE", A + "/users/{id}/block", "friends", "Разблокировать", code=204)
r("GET", A + "/blocks", "friends", "Чёрный список", resp=lst(S("UserCard")))
# --- посты ---
audience = enum("everyone", "friends", "only_me", "list")
new_poll = obj(question=str_, options=arr(str_), **{"multiple?": bool_, "anonymous?": bool_, "closes_at?": dt})
post_body = obj(**{"text?": str_, "photo_ids?": arr(uuid), "track_ids?": arr(uuid), "video_ids?": arr(uuid),
                   "document_ids?": arr(uuid), "route_ids?": arr(uuid), "community_id?": int_, "audience?": audience, "audience_ids?": arr(int_),
                   "poll?": new_poll, "publish_at?": dt})
scheduled = obj(id=int_, author_id=int_, **{"community_id?": int_, "post_id?": int_, "error?": str_}, publish_at=dt,
                payload={"type": "object"}, status=enum("pending", "published", "failed", "canceled"), created_at=dt)
r("POST", A + "/posts", "posts", "Опубликовать пост (с publish_at — отложить: ответ 202 и отложенный пост)", post_body, S("Post"), 201)
r("GET", A + "/posts/{id}", "posts", "Пост", resp=S("Post"))
r("PATCH", A + "/posts/{id}", "posts", "Изменить текст и/или аудиторию поста",
  obj(**{"text?": str_, "audience?": audience, "audience_ids?": arr(int_)}), S("Post"))
r("GET", A + "/posts/{id}/audience", "posts", "Кому виден пост (только автору)", resp=obj(audience=audience, user_ids=arr(int_)))
r("POST", A + "/posts/{id}/poll/votes", "posts", "Проголосовать в опросе", obj(option_ids=arr(int_)), S("Poll"))
r("DELETE", A + "/posts/{id}/poll/votes", "posts", "Отозвать голос", resp=S("Poll"))
r("GET", A + "/posts/{id}/poll/options/{oid}/voters", "posts", "Кто выбрал вариант (не в анонимных опросах)", resp=lst(S("UserCard")))
r("GET", A + "/scheduled-posts", "posts", "Отложенные посты: свои или ?community_id= (редакторам)", resp=obj(items=arr(scheduled)))
r("PATCH", A + "/scheduled-posts/{sid}", "posts", "Перенести отложенный пост", obj(publish_at=dt), scheduled)
r("DELETE", A + "/scheduled-posts/{sid}", "posts", "Отменить отложенный пост", code=204)
# --- маршруты ---
r("POST", A + "/routes", "routes", "Загрузить GPX (до 15 МБ): сервер считает статистику и хранит очищенный трек — без автора, устройства, времени и без концов (hidden_ends_m, по умолчанию 200 м)",
  resp=S("Route"), code=201, content=upload_form(**{"title?": str_, "hidden_ends_m?": enum("0", "200", "500", "1000")}))
r("GET", A + "/routes", "routes", "Мои маршруты", resp=lst(S("Route")))
r("GET", A + "/routes/{rid}", "routes", "Маршрут с подробной геометрией (владелец и зрители записи)", resp=S("Route"))
r("PATCH", A + "/routes/{rid}", "routes", "Название; public — показать в открытом каталоге /trails (без входа и поисковикам)",
  obj(**{"title?": str_, "public?": bool_, "public_note?": str_}), S("Route"))
r("DELETE", A + "/routes/{rid}", "routes", "Удалить (пропадёт из записей)", code=204)
r("GET", A + "/routes/{rid}/gpx", "routes", "Скачать очищенный GPX", resp="binary")
r("POST", A + "/routes/{rid}/marks", "routes", "Отметить точку: родник, стоянка, брод, опасность, вид (не дальше 2 км от трека; фото — своё)",
  obj(kind=enum("spring", "camp", "ford", "danger", "view", "other"), lat=num, lon=num, title=str_,
      **{"note?": str_, "photo_id?": nullable(uuid)}), S("RouteMark"), 201)
r("POST", A + "/routes/{rid}/marks/{pid}/check", "routes", "«На месте»: дата проверки — сегодня", resp=S("RouteMark"))
r("DELETE", A + "/routes/{rid}/marks/{pid}", "routes", "Удалить (автор точки или владелец маршрута)", code=204)
# --- закладки ---
bookmark = obj(id=int_, kind=enum("post", "video", "track"), created_at=dt,
               **{"post?": S("Post"), "video?": S("Video"), "track?": S("Track")})
r("GET", A + "/bookmarks", "bookmarks", "Закладки: ?kind=post|video|track", resp=lst(bookmark))
r("PUT", A + "/bookmarks/{kind}/{target}", "bookmarks", "Добавить в закладки (kind: post|video|track)", code=204)
r("DELETE", A + "/bookmarks/{kind}/{target}", "bookmarks", "Убрать из закладок", code=204)
# --- документы ---
r("POST", A + "/documents", "documents", "Загрузить документ (PDF, DOC(X), XLS(X), PPT(X), ODF, EPUB, DjVu, RTF, TXT, ZIP, 7Z, RAR… до 50 МБ)",
  resp=S("Document"), code=201, content=upload_form(**{"title?": str_}))
r("GET", A + "/documents", "documents", "Мои документы", resp=lst(S("Document")))
r("PATCH", A + "/documents/{did}", "documents", "Переименовать", obj(title=str_), S("Document"))
r("DELETE", A + "/documents/{did}", "documents", "Удалить (пропадёт из постов и сообщений)", code=204)
r("GET", A + "/documents/{did}/download", "documents", "Скачать (владелец, зрители поста, участники беседы)", resp="binary")
r("DELETE", A + "/posts/{id}", "posts", "Удалить пост", code=204)
r("POST", A + "/posts/{id}/like", "posts", "Лайк", resp=obj(likes_count=int_, liked_by_me=bool_))
r("DELETE", A + "/posts/{id}/like", "posts", "Убрать лайк", resp=obj(likes_count=int_, liked_by_me=bool_))
r("GET", A + "/posts/{id}/comments", "posts", "Комментарии", resp=lst(S("Comment")))
r("POST", A + "/posts/{id}/comments", "posts", "Комментировать", obj(text=str_), obj(id=int_, post_id=int_, text=str_), 201)
r("DELETE", A + "/comments/{id}", "posts", "Удалить комментарий", code=204)
r("GET", A + "/users/{id}/posts", "posts", "Стена", resp=lst(S("Post")))
r("GET", A + "/feed", "posts", "Лента", resp=lst(S("Post")))
r("GET", A + "/hashtags/{tag}/posts", "posts", "Посты по хэштегу", resp=lst(S("Post")))
# --- уведомления ---
r("GET", A + "/notifications", "notifications", "Уведомления", resp={"allOf": [lst(S("Notification")), obj(unread=int_)]})
r("POST", A + "/notifications/read", "notifications", "Прочитать (пустой ids — все)", obj(**{"ids?": arr(int_)}), code=204)
r("GET", A + "/users/me/notification-settings", "notifications", "Настройки уведомлений", resp=obj(settings=S("NotificationSettings"), available_types=arr(str_)))
r("PUT", A + "/users/me/notification-settings", "notifications", "Изменить настройки уведомлений", S("NotificationSettings"), obj(settings=S("NotificationSettings")))
r("GET", A + "/push/vapid-public-key", "notifications", "Ключ VAPID для Web Push", resp=obj(public_key=str_))
r("POST", A + "/push/subscriptions", "notifications", "Подписать браузер на push", obj(endpoint=str_, keys=obj(p256dh=str_, auth=str_)), code=204)
r("DELETE", A + "/push/subscriptions", "notifications", "Отписать браузер", obj(endpoint=str_), code=204)
# --- сообщения ---
r("POST", A + "/users/{id}/messages", "messages", "Отправить сообщение", obj(**{"body?": str_, "ciphertext?": b64, "nonce?": b64}), S("Message"), 201)
r("GET", A + "/conversations", "messages", "Диалоги", resp=lst(S("Conversation")))
r("GET", A + "/conversations/{id}/messages", "messages", "История диалога", resp=lst(S("Message")))
r("POST", A + "/conversations/{id}/read", "messages", "Отметить прочитанным", obj(up_to=int_), obj(last_read_id=int_))
r("POST", A + "/conversations/{id}/typing", "messages", "«Печатает…»", code=204)
r("PUT", A + "/users/me/keys", "messages", "Публичный ключ X25519 для E2E", obj(identity_key=b64), code=204)
r("GET", A + "/users/{id}/keys", "messages", "Ключ собеседника", resp=obj(user_id=int_, identity_key=b64, updated_at=dt))
r("POST", A + "/ws/ticket", "messages", "Одноразовый тикет WebSocket", resp=obj(ticket=str_))
r("GET", A + "/ws", "messages", "WebSocket: ?ticket=… События: message, delivered, read, typing, notification", auth=False, code=101)
# --- фото ---
r("POST", A + "/media", "media", "Загрузить фото (JPEG/PNG/GIF до 10 МБ)", resp=obj(id=uuid, url=str_, thumb_url=str_, width=int_, height=int_, mime=str_), code=201, content=upload_form())
r("DELETE", A + "/media/{id}", "media", "Удалить фото", code=204)
r("GET", "/media/{id}/{size}", "media", "Файл фото: size = full | thumb", auth=False, resp="binary")
# --- безопасность / модерация ---
r("POST", A + "/reports", "moderation", "Пожаловаться", obj(target_type=enum("post", "comment", "user", "community", "track", "video", "document", "route", "companion", "route_point", "article"), **{"target_id?": int_, "target_uuid?": uuid}, reason=str_), obj(id=int_, status=str_), 201)
r("GET", A + "/mod/reports", "moderation", "Очередь жалоб (модератор)", resp=lst(S("Report")))
r("POST", A + "/mod/reports/{id}/resolve", "moderation", "Решение по жалобе", obj(action=enum("dismiss", "hide", "ban"), **{"note?": str_}), code=204)
r("POST", A + "/mod/users/{id}/ban", "moderation", "Забанить", obj(**{"reason?": str_}), code=204)
r("POST", A + "/mod/users/{id}/unban", "moderation", "Разбанить", code=204)
r("POST", A + "/mod/{type}/{id}/hide", "moderation", "Скрыть: posts|comments|communities|tracks|videos|documents|routes", code=204)
r("POST", A + "/mod/{type}/{id}/unhide", "moderation", "Вернуть скрытое", code=204)
r("GET", A + "/mod/log", "moderation", "Журнал модерации", resp=lst(S("ModAction")))
r("PUT", A + "/admin/users/{id}/role", "moderation", "Назначить роль (админ)", obj(role=enum("user", "moderator", "admin")), code=204)
r("GET", A + "/admin/vulnerability-reports", "moderation", "Сообщения об уязвимостях (админ)", resp=lst(S("VulnReport")))
r("PATCH", A + "/admin/vulnerability-reports/{id}", "moderation", "Статус сообщения об уязвимости", obj(status=enum("new", "triaged", "fixed", "rejected")), code=204)
r("POST", A + "/security/vulnerability-reports", "moderation", "Сообщить об уязвимости (без входа)", obj(contact=str_, title=str_, description=str_), obj(id=int_, message=str_), 202, auth=False)
# --- сообщества ---
r("POST", A + "/communities", "communities", "Создать сообщество", obj(slug=str_, name=str_, **{"description?": str_}), S("Community"), 201)
r("GET", A + "/communities", "communities", "Мои сообщества", resp=lst(S("Community")))
r("GET", A + "/communities/by-slug/{slug}", "communities", "Сообщество по адресу", resp=S("Community"))
r("GET", A + "/communities/{id}", "communities", "Сообщество", resp=S("Community"))
r("PATCH", A + "/communities/{id}", "communities", "Изменить (админ)", obj(**{"name?": str_, "description?": str_}), S("Community"))
r("POST", A + "/communities/{id}/join", "communities", "Вступить", code=204)
r("POST", A + "/communities/{id}/leave", "communities", "Выйти", code=204)
r("GET", A + "/communities/{id}/members", "communities", "Участники", resp=lst(S("Member")))
r("PUT", A + "/communities/{id}/members/{userID}/role", "communities", "Роль участника", obj(role=enum("admin", "editor", "member")), obj(user_id=int_, role=str_))
r("DELETE", A + "/communities/{id}/members/{userID}", "communities", "Исключить участника", code=204)
r("GET", A + "/communities/{id}/posts", "communities", "Стена сообщества", resp=lst(S("Post")))
r("GET", A + "/search", "search", "Поиск: ?q=…&type=users|communities|tracks", resp=obj(items=arr({})))
# --- кошелёк ---
r("GET", A + "/wallet", "wallet", "Баланс и история", resp=obj(balance=int_, debt=int_, transactions=lst(S("Tx"))))
r("POST", A + "/wallet/transfer", "wallet", "Перевод", obj(to_user_id=int_, **money), S("TxResult"), 201)
r("POST", A + "/wallet/daily-bonus", "wallet", "Ежедневный бонус", resp=S("TxResult"), code=201)
r("GET", A + "/gifts/catalog", "wallet", "Каталог подарков", resp=obj(items=arr(obj(id=int_, name=str_, price=int_))))
r("POST", A + "/users/{id}/gifts", "wallet", "Подарить", obj(gift_id=int_, **{"message?": str_, "anonymous?": bool_}, idempotency_key=str_), obj(gift_id=int_, replayed=bool_, balance=int_), 201)
r("GET", A + "/users/{id}/gifts", "wallet", "Подарки в профиле", resp=lst(S("Gift")))
r("POST", A + "/communities/{id}/donate", "wallet", "Поддержать сообщество", obj(**money), S("TxResult"), 201)
r("GET", A + "/communities/{id}/wallet", "wallet", "Кошелёк сообщества (админ)", resp=obj(balance=int_, transactions=lst(S("Tx"))))
r("POST", A + "/communities/{id}/wallet/payout", "wallet", "Вывод участнику (владелец)", obj(to_user_id=int_, **money), S("TxResult"), 201)
r("POST", A + "/admin/wallet/mint", "wallet", "Начислить монеты (админ)", obj(user_id=int_, **money), S("TxResult"), 201)
# --- музыка ---
r("POST", A + "/tracks", "music", "Загрузить трек (до 50 МБ)", resp=S("Track"), code=201, content=upload_form(**{"title?": str_, "artist?": str_}))
r("GET", A + "/tracks/{tid}", "music", "Трек", resp=S("Track"))
r("DELETE", A + "/tracks/{tid}", "music", "Удалить свой трек", code=204)
r("POST", A + "/tracks/{tid}/my-music", "music", "Добавить в «Мою музыку»", code=204)
r("DELETE", A + "/tracks/{tid}/my-music", "music", "Убрать из «Моей музыки»", code=204)
r("GET", A + "/users/{id}/music", "music", "«Моя музыка» пользователя", resp=lst(S("Track")))
r("GET", A + "/users/{id}/playlists", "music", "Плейлисты", resp=obj(items=arr(S("Playlist"))))
r("POST", A + "/playlists", "music", "Создать плейлист", obj(title=str_), S("Playlist"), 201)
r("GET", A + "/playlists/{id}", "music", "Плейлист с треками", resp=S("Playlist"))
r("PATCH", A + "/playlists/{id}", "music", "Переименовать", obj(title=str_), code=204)
r("DELETE", A + "/playlists/{id}", "music", "Удалить плейлист", code=204)
r("POST", A + "/playlists/{id}/tracks", "music", "Добавить трек", obj(track_id=uuid), code=204)
r("DELETE", A + "/playlists/{id}/tracks/{tid}", "music", "Убрать трек", code=204)
r("GET", "/audio/{file}", "music", "MP3 для <audio>, поддерживает Range", auth=False, resp="binary")
# --- видео ---
r("POST", A + "/videos", "video", "Загрузить видео (до 1 ГБ), обработка в фоне", resp=S("Video"), code=202, content=upload_form(**{"title?": str_, "description?": str_}))
r("GET", A + "/videos/{vid}", "video", "Видео", resp=S("Video"))
r("PATCH", A + "/videos/{vid}", "video", "Изменить", obj(**{"title?": str_, "description?": str_}), S("Video"))
r("DELETE", A + "/videos/{vid}", "video", "Удалить", code=204)
r("GET", A + "/users/{id}/videos", "video", "Видео пользователя", resp=obj(items=arr(S("Video"))))
r("GET", "/video/{vid}/{file}", "video", "HLS: master.m3u8, 360p.m3u8, сегменты .ts, poster.jpg", auth=False, resp="binary")

# --- ветки комментариев и реакции ---
r("GET", A + "/comments/{id}/replies", "posts", "Ответы в ветке комментария", resp=lst(S("Comment")))
r("POST", A + "/posts/{id}/reactions", "posts", "Поставить или сменить реакцию",
  obj(reaction=enum("like", "love", "haha", "wow", "sad", "angry")), obj(likes_count=int_, my_reaction=str_))
r("DELETE", A + "/posts/{id}/reactions", "posts", "Убрать реакцию", resp=obj(likes_count=int_, liked_by_me=bool_))
# --- групповые чаты и голосовые ---
msg_body = obj(**{"body?": str_, "ciphertext?": b64, "nonce?": b64, "attachments?": arr(obj(kind=enum("photo", "track", "voice"), id=uuid))})
r("POST", A + "/conversations/{id}/messages", "messages", "Сообщение в диалог или беседу", msg_body, S("Message"), 201)
r("POST", A + "/chats", "messages", "Создать беседу", obj(title=str_, member_ids=arr(int_)), obj(id=int_, title=str_, members_count=int_), 201)
r("PATCH", A + "/conversations/{id}", "messages", "Переименовать беседу (админы)", obj(title=str_), code=204)
r("GET", A + "/conversations/{id}/members", "messages", "Участники", resp=obj(items=arr(S("ChatMember"))))
r("POST", A + "/conversations/{id}/members", "messages", "Добавить в беседу (админы)", obj(user_id=int_), code=204)
r("DELETE", A + "/conversations/{id}/members/{uid}", "messages", "Выйти или исключить", code=204)
r("PUT", A + "/conversations/{id}/members/{uid}/role", "messages", "Роль в беседе (владелец)", obj(role=enum("admin", "member")), code=204)
r("POST", A + "/voice", "messages", "Загрузить голосовое (до 5 минут)", resp=obj(id=uuid, duration_sec=int_, url=str_), code=201, content=upload_form())
r("GET", A + "/voice/{vid}", "messages", "Голосовое (только участникам чата)", resp="binary")
# --- закрытые сообщества ---
r("GET", A + "/communities/{id}/requests", "communities", "Заявки на вступление (админы)", resp=lst(S("UserCard")))
r("POST", A + "/communities/{id}/requests/{userID}/approve", "communities", "Принять заявку", code=204)
r("POST", A + "/communities/{id}/requests/{userID}/reject", "communities", "Отклонить заявку", code=204)
emoji_s = obj(id=int_, community_id=int_, shortcode=str_, url=str_, reaction=str_, created_at=dt)
r("GET", A + "/communities/{id}/emoji", "communities", "Эмодзи сообщества (реакция e:<id> — на записях сообщества)", resp=obj(items=arr(emoji_s)))
r("POST", A + "/communities/{id}/emoji", "communities", "Добавить эмодзи (админ; картинка — своё фото; до 50)", obj(shortcode=str_, media_id=uuid), emoji_s, 201)
r("DELETE", A + "/communities/{id}/emoji/{eid}", "communities", "Удалить эмодзи (реакции им станут «нравится»)", code=204)
r("GET", A + "/communities/{id}/events", "events", "Мероприятия сообщества", resp=obj(items=arr(S("Event"))))
# --- истории ---
r("POST", A + "/stories", "stories", "Опубликовать историю на 24 часа", obj(**{"photo_id?": uuid, "video_id?": uuid, "caption?": str_}), obj(id=int_, expires_in_hours=int_), 201)
r("GET", A + "/stories/feed", "stories", "Истории друзей и подписок", resp=obj(items=arr(S("StoryGroup"))))
r("GET", A + "/users/{id}/stories", "stories", "Истории пользователя", resp=obj(items=arr(S("StoryGroup"))))
r("POST", A + "/stories/{id}/view", "stories", "Отметить просмотр", code=204)
r("GET", A + "/stories/{id}/viewers", "stories", "Кто смотрел (автору)", resp=obj(items=arr(S("UserCard"))))
r("DELETE", A + "/stories/{id}", "stories", "Удалить историю", code=204)
# --- мероприятия ---
ev = obj(**{"kind?": enum("event", "hike"), "title?": str_, "description?": str_, "place?": str_, "starts_at?": dt,
            "ends_at?": nullable(dt), "community_id?": int_, "difficulty?": nullable(enum_int(1, 2, 3, 4, 5)),
            "capacity?": nullable(int_), "gear?": str_, "route_id?": nullable(str_), "meeting_point?": str_, "approval?": bool_,
            "control_at?": nullable(dt)})
r("POST", A + "/events", "events", "Создать мероприятие", ev, S("Event"), 201)
r("GET", A + "/events", "events", "Мои предстоящие мероприятия", resp=obj(items=arr(S("Event"))))
r("GET", A + "/events/{id}", "events", "Мероприятие", resp=S("Event"))
r("PATCH", A + "/events/{id}", "events", "Изменить (организатор)", ev, S("Event"))
r("DELETE", A + "/events/{id}", "events", "Удалить (организатор)", code=204)
r("POST", A + "/events/{id}/rsvp", "events", "Пойду / Возможно. Поход: место, лист ожидания (мест нет) или заявка организатору (approval); «возможно» нельзя",
  obj(status=enum("going", "maybe"), **{"note?": str_}), obj(status=enum("going", "maybe", "pending", "waitlist")))
r("DELETE", A + "/events/{id}/rsvp", "events", "Не пойду (место — первому в очереди)", code=204)
r("GET", A + "/events/{id}/attendees", "events", "Участники: ?status=going|maybe|waitlist", resp=lst(S("UserCard")))
r("DELETE", A + "/events/{id}/attendees/{uid}", "events", "Убрать участника (организатор)", code=204)
r("GET", A + "/events/{id}/applications", "events", "Заявки на участие (организатор)", resp=obj(items=arr(S("Application"))))
r("POST", A + "/events/{id}/applications/{uid}", "events", "Принять или отклонить заявку; принятый без мест встаёт в очередь",
  obj(approve=bool_), obj(status=enum("going", "waitlist", "rejected")))
date_ = {"type": "string", "format": "date"}
r("POST", A + "/companions", "events", "«Ищу попутчиков»: объявление (не больше 5 открытых)",
  obj(region=str_, title=str_, date_from=date_, **{"date_to?": date_, "body?": str_, "difficulty?": nullable(enum_int(1, 2, 3, 4, 5)),
                                                   "seats?": int_, "route_id?": nullable(str_)}), S("CompanionAd"), 201)
r("GET", A + "/companions", "events", "Открытые объявления: ?region=&from=&to=&max_difficulty=", resp=obj(items=arr(S("CompanionAd"))))
r("GET", A + "/companions/mine", "events", "Мои объявления", resp=obj(items=arr(S("CompanionAd"))))
r("GET", A + "/companions/{id}", "events", "Объявление", resp=S("CompanionAd"))
r("PATCH", A + "/companions/{id}", "events", "Закрыть (нашёл попутчиков) или открыть снова", obj(closed=bool_), S("CompanionAd"))
r("DELETE", A + "/companions/{id}", "events", "Удалить", code=204)
r("POST", A + "/companions/{id}/reply", "messages", "Откликнуться: личное сообщение автору (даже если он принимает сообщения только от друзей)",
  obj(text=str_), S("Message"), 201)
hike_contact = obj(id=int_, name=str_, email=str_, created_at=dt)
r("GET", A + "/events/{id}/contacts", "events", "Мои доверенные контакты в походе (чужие не видны никому)", resp=obj(items=arr(hike_contact)))
r("POST", A + "/events/{id}/contacts", "events", "Добавить доверенный контакт (до 3; нужен контрольный срок, участие и подтверждённая почта)",
  obj(name=str_, email=str_, consent=bool_), hike_contact, 201)
r("DELETE", A + "/events/{id}/contacts/{cid}", "events", "Удалить контакт", code=204)
r("POST", A + "/events/{id}/returned", "events", "Группа вернулась: контакты удаляются; если тревога уже ушла — им «отбой»", code=204)
art = obj(**{"title?": str_, "body?": str_, "cover_media_id?": nullable(uuid), "route_id?": nullable(uuid),
            "status?": enum("draft", "published")})
antenna = obj(id=int_, name=str_, keywords=arr(str_), excludes=arr(str_), unread=int_, created_at=dt)
antenna_body = obj(name=str_, keywords=arr(str_), **{"excludes?": arr(str_)})
r("GET", A + "/antennas", "wall", "Мои ленты по ключевым словам («антенны») с числом новых записей", resp=obj(items=arr(antenna)))
r("POST", A + "/antennas", "wall", "Новая лента: 1–10 слов (русские словоформы), до 10 исключений; до 5 лент", antenna_body, antenna, 201)
r("PUT", A + "/antennas/{id}", "wall", "Изменить ленту", antenna_body, antenna)
r("DELETE", A + "/antennas/{id}", "wall", "Удалить ленту", code=204)
r("GET", A + "/antennas/{id}/posts", "wall", "Записи ленты (только видимые); первая страница отмечает их просмотренными", resp=lst(S("Post")))
plus_s = obj(active=bool_, expires_at=nullable(dt), auto_renew=bool_, price=int_, days=int_, limits=obj())
r("GET", A + "/plus", "wallet", "Подписка «Плюс»: состояние, цена в монетах, лимиты (обычный и с «Плюсом»)", resp=plus_s)
r("POST", A + "/plus/buy", "wallet", "Купить или продлить на 30 дней за монеты (ключ идемпотентности обязателен)",
  obj(idempotency_key=str_), {"allOf": [plus_s, obj(replayed=bool_, balance=int_)]}, 201)
r("PATCH", A + "/plus", "wallet", "Автопродление", obj(auto_renew=bool_), plus_s)
r("GET", A + "/events/{id}/participants.csv", "events", "Список группы CSV для регистрации в МЧС (организатору с «Плюсом»)", resp="text")
r("POST", A + "/articles", "wall", "Статья: разметка ## ### - 1. > --- **жирный** *курсив* [ссылка](https://…) ![подпись](media:UUID); фото и маршрут — свои",
  art, S("Article"), 201)
r("GET", A + "/articles", "wall", "Опубликованные статьи (без текста, с анонсом)", resp=lst(S("Article")))
r("GET", A + "/articles/mine", "wall", "Мои статьи и черновики", resp=obj(items=arr(S("Article"))))
r("POST", A + "/articles/preview", "wall", "Предпросмотр разметки", obj(body=str_), obj(blocks=arr(S("Block"))))
r("GET", A + "/articles/{id}", "wall", "Статья с деревом блоков (черновик — только автору)", resp=S("Article"))
r("PATCH", A + "/articles/{id}", "wall", "Изменить, опубликовать (status=published) или снять в черновики", art, S("Article"))
r("DELETE", A + "/articles/{id}", "wall", "Удалить", code=204)
r("POST", A + "/rides", "events", "Подвоз к началу маршрута: места в машине (деньги приложение не принимает)",
  obj(**{"from": str_}, to=str_, departs_at=dt, seats=int_, **{"event_id?": int_, "note?": str_}), S("Ride"), 201)
r("GET", A + "/rides", "events", "Предстоящие поездки: ?event_id=&to=", resp=obj(items=arr(S("Ride"))))
r("GET", A + "/rides/{id}", "events", "Поездка", resp=S("Ride"))
r("DELETE", A + "/rides/{id}", "events", "Отменить (водитель); попутчикам — уведомление", code=204)
r("POST", A + "/rides/{id}/request", "events", "Попроситься", obj(**{"note?": str_}), S("Ride"))
r("DELETE", A + "/rides/{id}/request", "events", "Отозвать заявку или выйти", code=204)
r("GET", A + "/rides/{id}/requests", "events", "Заявки и попутчики (водителю)",
  resp=obj(items=arr(obj(user=S("UserCard"), status=enum("pending", "accepted"), note=str_, created_at=dt))))
r("POST", A + "/rides/{id}/requests/{uid}", "events", "Принять (если есть место) или отказать/высадить", obj(accept=bool_), S("Ride"))
r("GET", A + "/weather", "routes", "Прогноз на 10 дней (Open-Meteo через сервер, кеш час, район ~5 км): ?lat=&lon=&elevation=",
  resp=obj(lat=num, lon=num, elevation=nullable(num), source=str_, fetched_at=dt,
           days=arr(obj(date={"type": "string", "format": "date"}, code=int_, t_max=nullable(num), t_min=nullable(num),
                        precip_mm=nullable(num), precip_prob=nullable(num), wind_max=nullable(num), gust_max=nullable(num)))))
r("GET", A + "/hikes", "events", "Ближайшие походы: ?max_difficulty=1..5", resp=obj(items=arr(S("Event"))))
# --- журнал аудита и тревоги ---
audit = obj(id=int_, actor_id=int_, actor_role=str_, action=str_, target=str_, **{"details?": {"type": "object"}},
            status=int_, ip=str_, user_agent=str_, request_id=str_, created_at=dt)
alert = obj(id=int_, rule=str_, subject=str_, severity=enum("low", "medium", "high"), details={"type": "object"},
            created_at=dt, acknowledged_by=nullable(int_))
admin_user = obj(id=int_, username=str_, email=str_, first_name=str_, last_name=str_, avatar_url=nullable(str_),
                 role=enum("user", "moderator", "admin"), email_verified=bool_, is_bot=bool_, created_at=dt,
                 **{"banned_at?": dt, "ban_reason?": str_, "last_seen_at?": dt})
r("GET", A + "/admin/stats", "moderation", "Сводка: пользователи, активность, жалобы, тревоги, платежи",
  resp=obj(users=int_, users_new_24h=int_, users_new_7d=int_, active_24h=int_, banned=int_, posts_24h=int_,
           comments_24h=int_, messages_24h=int_, open_reports=int_, open_alerts=int_, new_vulnerability_reports=int_,
           scheduled_posts_pending=int_, documents=int_, paid_kopecks_30d=int_))
r("GET", A + "/admin/users", "moderation", "Поиск пользователей: ?q= (id, email, username, имя)&filter=all|banned|staff|unverified", resp=lst(admin_user))
r("GET", A + "/admin/users/{id}", "moderation", "Карточка пользователя (без пароля и секретов — их нет в открытом виде)",
  resp={"allOf": [admin_user, obj(two_factor_enabled=bool_, active_sessions=int_, posts=int_, comments=int_, friends=int_,
                                  open_reports_against=int_, reports_filed=int_, balance=int_, debt=int_,
                                  **{"last_login_at?": dt}, recent_security_events=arr(S("SecurityEvent")))]})
r("POST", A + "/admin/users/{id}/password-reset", "moderation", "Отправить пользователю письмо со ссылкой на сброс пароля",
  obj(**{"revoke_sessions?": bool_}), obj(sent_to=str_, sessions_revoked=bool_), 202)
r("POST", A + "/admin/users/{id}/revoke-sessions", "moderation", "Завершить все сеансы и токены приложений пользователя", code=204)
r("GET", A + "/admin/audit", "moderation", "Журнал действий модераторов и админов: ?actor_id=&before=", resp=lst(audit))
r("GET", A + "/admin/audit/verify", "moderation", "Проверить цепочку хешей журнала", resp=obj(ok=bool_, checked=int_, broken_at=int_))
r("GET", A + "/admin/alerts", "moderation", "Тревоги детектора аномалий: ?status=all", resp=lst(alert))
r("POST", A + "/admin/alerts/{id}/ack", "moderation", "Отметить тревогу разобранной", code=204)

# --- покупка монет ---
pay = obj(id=uuid, package_id=str_, coins=int_, amount_kopecks=int_, currency=str_,
          status=enum("created", "pending", "succeeded", "canceled", "error", "refunded", "partially_refunded"),
          **{"confirmation_url?": str_}, created_at=dt)
r("GET", A + "/wallet/packages", "wallet", "Пакеты монет и цены", resp=obj(enabled=bool_, items=arr(obj(id=str_, coins=int_, price_kopecks=int_, price=str_))))
r("POST", A + "/wallet/purchases", "wallet", "Купить монеты: вернёт confirmation_url страницы оплаты ЮKassa", obj(package_id=str_), pay, 201)
r("GET", A + "/wallet/purchases", "wallet", "Мои покупки", resp=obj(items=arr(pay)))
r("GET", A + "/wallet/purchases/{pid}", "wallet", "Статус покупки (сверяется с ЮKassa)", resp=pay)
r("POST", A + "/payments/yookassa", "wallet", "Уведомления ЮKassa: payment.*, refund.succeeded (только с её адресов; всё перепроверяется через API)",
  obj(type=str_, event=str_, object={"type": "object"}), auth=False)

# --- API для разработчиков ---
authz = obj(response_type=enum("code"), client_id=str_, redirect_uri=str_, scope=str_, **{"state?": str_},
            code_challenge=str_, code_challenge_method=enum("S256"), approve=bool_)
form = lambda **f: {"application/x-www-form-urlencoded": {"schema": obj(**f)}}
r("GET", A + "/oauth/authorize", "developers", "Страница согласия: что запрашивает приложение (те же параметры в query)",
  resp=obj(app=obj(name=str_, client_id=str_, developer=str_), scopes=arr(obj(name=str_, description=str_)), redirect_uri=str_))
r("POST", A + "/oauth/authorize", "developers", "Разрешить/отклонить доступ; ответ — куда перенаправить браузер", authz, obj(redirect_to=str_))
r("POST", A + "/oauth/token", "developers", "Обмен кода на токен (PKCE обязателен; секрет — Basic или client_secret)",
  resp=obj(access_token=str_, token_type=str_, scope=str_, user_id=int_, **{"expires_in?": int_}), auth=False,
  content=form(grant_type=enum("authorization_code"), code=str_, redirect_uri=str_, client_id=str_, code_verifier=str_, **{"client_secret?": str_}))
r("POST", A + "/oauth/revoke", "developers", "Отозвать токен (RFC 7009)", auth=False,
  content=form(token=str_, client_id=str_, **{"client_secret?": str_}))
r("GET", A + "/apps", "developers", "Мои приложения", resp=obj(items=arr(S("App"))))
r("POST", A + "/apps", "developers", "Создать приложение (секрет показывается один раз)",
  obj(name=str_, redirect_uris=arr(str_), **{"confidential?": bool_}), S("App"), 201)
r("GET", A + "/apps/{id}", "developers", "Приложение", resp=S("App"))
r("PATCH", A + "/apps/{id}", "developers", "Изменить приложение", obj(**{"name?": str_, "redirect_uris?": arr(str_)}), S("App"))
r("DELETE", A + "/apps/{id}", "developers", "Удалить приложение (все его токены отзываются)", code=204)
r("POST", A + "/apps/{id}/secret", "developers", "Новый секрет приложения", resp=obj(client_secret=str_))
r("GET", A + "/users/me/authorized-apps", "developers", "Приложения с доступом к аккаунту", resp=obj(items=arr(S("AuthorizedApp"))))
r("DELETE", A + "/users/me/authorized-apps/{id}", "developers", "Отозвать доступ приложения", code=204)
r("GET", A + "/communities/{id}/bot/tokens", "developers", "Токены бота сообщества (админ)", resp=obj(bot_user_id=nullable(int_), items=arr(S("BotToken"))))
r("POST", A + "/communities/{id}/bot/tokens", "developers", "Новый токен бота: scopes из messages, wall, photos",
  obj(**{"name?": str_}, scopes=arr(str_)), obj(id=int_, token=str_, scopes=arr(str_), bot_user_id=int_), 201)
r("DELETE", A + "/communities/{id}/bot/tokens/{tid}", "developers", "Отозвать токен бота", code=204)
r("GET", A + "/communities/{id}/callback", "developers", "Callback-сервер и последние доставки",
  resp=obj(server=S("CallbackServer"), confirmation_code=str_, recent=arr({"type": "object"})))
r("PUT", A + "/communities/{id}/callback", "developers", "Задать Callback-сервер: message_new, wall_post_new, wall_reply_new, group_join, group_leave",
  obj(url=str_, events=arr(str_), **{"secret?": str_}), obj(status=str_, confirmation_code=str_, **{"secret?": str_}, next=str_))
r("POST", A + "/communities/{id}/callback/confirm", "developers", "Проверить сервер событием confirmation", resp=obj(status=str_))
r("DELETE", A + "/communities/{id}/callback", "developers", "Отключить Callback API", code=204)


def build(routes_file=None):
    paths = {}
    for (method, path), d in sorted(R.items(), key=lambda x: (x[0][1], x[0][0])):
        # числовые id, кроме /mod/{type}/{id}: там бывает и UUID трека или видео
        params = [{"name": n, "in": "path", "required": True,
                   "schema": int_ if (n in ("id", "userID", "uid", "tid", "oid") or (n == "sid" and "scheduled" in path)) and "/tracks/" not in path and "{type}" not in path else str_}
                  for n in re.findall(r"{(\w+)}", path)]
        op = {"tags": [d["tag"]], "summary": d["summary"], "operationId": (method.lower() + re.sub(r"[^a-zA-Z0-9]+", "_", path)).strip("_")}
        if params:
            op["parameters"] = params
        if not d["auth"]:
            op["security"] = []
        if d["content"]:
            op["requestBody"] = {"required": True, "content": d["content"]}
        elif d["body"] is not None:
            op["requestBody"] = {"required": True, "content": {"application/json": {"schema": d["body"]}}}
        resp = d["resp"]
        if resp == "binary":
            ok = {"description": "Файл", "content": {"application/octet-stream": {"schema": {"type": "string", "format": "binary"}}}}
        elif resp == "text":
            ok = {"description": "Текст", "content": {"text/plain": {"schema": str_}}}
        elif resp == "json":
            ok = {"description": "JSON", "content": {"application/json": {"schema": {"type": "object"}}}}
        elif resp is None:
            ok = {"description": "Успешно"}
        else:
            ok = {"description": "Успешно", "content": {"application/json": {"schema": resp}}}
        op["responses"] = {str(d["code"]): ok, "default": {"description": "Ошибка", "content": {"application/json": {"schema": S("Error")}}}}
        paths.setdefault(path, {})[method.lower()] = op
    return {
        "openapi": "3.0.3",
        "info": {"title": "Socialnet API", "version": "5.0",
                 "description": "Соцсеть в духе ВКонтакте. Авторизация: `Authorization: Bearer <access_token>` "
                                "из /auth/login; refresh-токен — в HttpOnly-cookie. Ошибки — схема Error."},
        "servers": [{"url": "/"}],
        "tags": [{"name": n, "description": d} for n, d in [
            ("auth", "Вход, регистрация, пароль, 2FA, сеансы"), ("users", "Профиль, приватность, статус"),
            ("friends", "Друзья и блокировки"), ("posts", "Посты, лента, комментарии"),
            ("messages", "Личные сообщения и WebSocket"), ("notifications", "Уведомления и push"),
            ("media", "Фото"), ("documents", "Документы"), ("routes", "Маршруты GPX"), ("bookmarks", "Закладки"), ("music", "Музыка и плейлисты"), ("video", "Видео"),
            ("stories", "Истории"), ("events", "Мероприятия"), ("communities", "Сообщества"), ("wallet", "Монеты, подарки"), ("search", "Поиск"),
            ("developers", "OAuth-приложения, боты сообществ, Callback API"),
            ("moderation", "Жалобы, модерация, администрирование"), ("service", "Служебное")]],
        "security": [{"bearer": []}],
        "components": {"securitySchemes": {
                           "bearer": {"type": "http", "scheme": "bearer",
                                      "description": "JWT из /auth/login или токен приложения/бота snt_…"},
                           "oauth": {"type": "oauth2", "flows": {"authorizationCode": {
                               "authorizationUrl": "/oauth/authorize", "tokenUrl": "/api/v1/oauth/token",
                               "scopes": {k: v for k, v in SCOPES.items()}}}}},
                       "schemas": schemas},
        "paths": paths,
    }


if __name__ == "__main__":
    out = os.path.join(os.path.dirname(os.path.abspath(__file__)), "openapi.json")
    with open(out, "w", encoding="utf-8") as f:
        json.dump(build(), f, ensure_ascii=False, indent=1)
        f.write("\n")
    print(f"{out}: {len(R)} operations")
