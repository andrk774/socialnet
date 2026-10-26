#!/usr/bin/env python3
"""Сквозная проверка запущенного docker compose.

    docker compose up -d --build
    python3 deploy/smoke.py

Проверяет через HTTP и docker compose: регистрацию, ленту, комментарии,
сообщения, кошелёк, обработку видео воркером (HLS и MP4), резервную копию
с проверкой восстановлением, метрики и изоляцию контейнеров. Только
стандартная библиотека Python; нужен docker compose в PATH.
"""
import json
import os
import subprocess
import sys
import time
import urllib.error
import urllib.request
import uuid

BASE = os.environ.get("BASE", "http://127.0.0.1:8080")
COMPOSE = os.environ.get("COMPOSE", "docker compose").split()
RUN = uuid.uuid4().hex[:6]
failed = []


def step(name):
    print(f"• {name}", flush=True)


def check(cond, what):
    if not cond:
        failed.append(what)
        print(f"  ✗ {what}", flush=True)
    return cond


def compose(*args, input=None, check_rc=True):
    r = subprocess.run(COMPOSE + list(args), input=input, capture_output=True)
    if check_rc and r.returncode != 0:
        raise RuntimeError(f"{' '.join(args)}: {r.stderr.decode(errors='replace')[-2000:]}")
    return r


def http(method, path, body=None, token=None, headers=None, raw=False):
    data, hdrs = None, dict(headers or {})
    if isinstance(body, bytes):
        data = body
    elif body is not None:
        data = json.dumps(body).encode()
        hdrs["Content-Type"] = "application/json"
    if token:
        hdrs["Authorization"] = "Bearer " + token
    req = urllib.request.Request(BASE + path, data=data, method=method, headers=hdrs)
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            payload = resp.read()
            return resp.status, (payload if raw else json.loads(payload or b"null")), resp.headers
    except urllib.error.HTTPError as e:
        payload = e.read()
        try:
            payload = json.loads(payload)
        except ValueError:
            pass
        return e.code, payload, e.headers


def multipart(field, filename, content, ctype):
    boundary = uuid.uuid4().hex
    body = (f"--{boundary}\r\nContent-Disposition: form-data; name=\"{field}\"; filename=\"{filename}\"\r\n"
            f"Content-Type: {ctype}\r\n\r\n").encode() + content + f"\r\n--{boundary}--\r\n".encode()
    return body, {"Content-Type": f"multipart/form-data; boundary={boundary}"}


def register(name):
    code, body, _ = http("POST", "/api/v1/auth/register", {
        "email": f"{name}{RUN}@example.com", "username": f"{name}{RUN}", "password": f"password-{name}-{RUN}",
        "first_name": name.capitalize()})
    if not check(code == 201, f"register {name}: {code} {body}"):
        sys.exit(1)
    return body["access_token"], body["user"]["id"]


def wait_health():
    step("API отвечает")
    for _ in range(90):
        try:
            if http("GET", "/health")[0] == 200:
                return
        except OSError:
            pass
        time.sleep(1)
    sys.exit("API did not become healthy in 90 s")


def main():
    wait_health()
    code, page, hdrs = http("GET", "/", raw=True)
    check(code == 200 and b'id="app"' in page and "script-src 'self'" in hdrs.get("Content-Security-Policy", ""),
          "веб-интерфейс отдаётся со строгой CSP")

    step("регистрация, лента, комментарии, реакции, сообщения")
    anna, anna_id = register("anna")
    bob, bob_id = register("bob")
    code, post, _ = http("POST", "/api/v1/posts", {"text": f"Привет из docker compose #smoke{RUN}"}, anna)
    check(code == 201, f"публикация: {code}")
    check(http("POST", f"/api/v1/posts/{post['id']}/comments", {"text": "Работает!"}, bob)[0] == 201, "комментарий")
    check(http("POST", f"/api/v1/posts/{post['id']}/reactions", {"reaction": "love"}, bob)[0] == 200, "реакция")
    code, msg, _ = http("POST", f"/api/v1/users/{bob_id}/messages", {"body": "Привет, Боб"}, anna)
    check(code == 201, f"сообщение: {code} {msg}")
    code, convs, _ = http("GET", "/api/v1/conversations", token=bob)
    check(code == 200 and convs["items"] and convs["items"][0]["unread"] >= 1, "диалог у получателя")
    check(http("POST", "/api/v1/ws/ticket", token=anna)[0] == 200, "тикет WebSocket (Redis)")

    step("кошелёк (почта подтверждается прямо в базе)")
    compose("exec", "-T", "db", "psql", "-U", "socialnet", "-d", "socialnet", "-tAc",
            f"UPDATE users SET email_verified_at = now() WHERE username LIKE '%{RUN}'")
    check(http("POST", "/api/v1/wallet/daily-bonus", token=anna)[0] == 201, "ежедневный бонус")
    code, w, _ = http("GET", "/api/v1/wallet", token=anna)
    check(code == 200 and w["balance"] > 0 and w["debt"] == 0, f"баланс: {w}")

    step("видео: загрузка → воркер (изолированная сеть) → HLS и MP4")
    clip = compose("run", "--rm", "-T", "--no-deps", "--entrypoint", "ffmpeg", "worker", "-hide_banner", "-v", "error",
                   "-f", "lavfi", "-i", "testsrc=size=1280x720:rate=24:duration=3", "-f", "lavfi", "-i", "sine=duration=3",
                   "-shortest", "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac",
                   "-movflags", "frag_keyframe+empty_moov", "-f", "mp4", "pipe:1").stdout
    body, hdrs = multipart("file", "smoke.mp4", clip, "video/mp4")
    code, v, _ = http("POST", "/api/v1/videos", body, anna, hdrs)
    check(code == 202, f"загрузка видео: {code} {v}")
    for _ in range(120):
        code, v, _ = http("GET", f"/api/v1/videos/{v['id']}", token=anna)
        if v.get("status") != "processing":
            break
        time.sleep(1)
    if check(v.get("status") == "ready", f"видео обработано: {v.get('status')} {v.get('error', '')}"):
        code, master, _ = http("GET", v["hls_url"], raw=True)
        check(code == 200 and b"#EXTM3U" in master, "HLS-плейлист")
        code, part, hdrs = http("GET", v["mp4_url"], headers={"Range": "bytes=0-99"}, raw=True)
        check(code == 206 and len(part) == 100 and hdrs.get("Content-Type") == "video/mp4", f"MP4 с Range: {code}")

    step("резервная копия с проверкой восстановлением")
    r = compose("run", "--rm", "-T", "backup", "backup", check_rc=False)
    out = r.stdout.decode(errors="replace") + r.stderr.decode(errors="replace")
    check(r.returncode == 0 and "backup ok" in out, f"backup: rc={r.returncode} {out[-600:]}")

    step("метрики Prometheus (внутренний порт 9090)")
    m = compose("exec", "-T", "api", "/api", "probe", "http://127.0.0.1:9090/metrics").stdout.decode()
    check('socialnet_backup{metric="last_restore_ok"} 1' in m, "метрика успешной копии")
    check('route="/api/v1/posts",status="201"' in m, "метрика запросов")
    check('socialnet_events_total{event="message_sent"}' in m, "бизнес-метрики")

    step("изоляция контейнеров")
    r = compose("exec", "-T", "worker", "/api", "probe", "https://example.com/", check_rc=False)
    check(r.returncode != 0, "воркер не выходит в интернет")
    r = compose("exec", "-T", "worker", "/api", "probe", "http://api:8080/health", check_rc=False)
    check(r.returncode == 0, "воркер видит API во внутренней сети")
    # mailpit — только во внешней сети: API до него достаёт, воркер — нет
    r = compose("exec", "-T", "api", "/api", "probe", "http://mailpit:8025/", check_rc=False)
    reachable = r.returncode == 0 or b"probe: " in r.stdout + r.stderr  # любой HTTP-ответ = сеть есть
    check(reachable, "API видит сервисы внешней сети")
    r = compose("exec", "-T", "worker", "/api", "probe", "http://mailpit:8025/", check_rc=False)
    check(r.returncode != 0 and b"probe: " not in r.stdout + r.stderr, "воркер не видит внешнюю сеть")
    for svc in ("api", "worker", "redis"):
        cid = compose("ps", "-q", svc).stdout.decode().strip()
        info = json.loads(subprocess.run(["docker", "inspect", cid], capture_output=True).stdout)[0]["HostConfig"]
        check(info["ReadonlyRootfs"] and info["CapDrop"] == ["ALL"] and "no-new-privileges:true" in (info["SecurityOpt"] or []),
              f"{svc}: read-only, без capabilities, no-new-privileges")
    for svc in ("api", "worker"):
        cid = compose("ps", "-q", svc).stdout.decode().strip()
        uid = json.loads(subprocess.run(["docker", "inspect", cid], capture_output=True).stdout)[0]["Config"]["User"]
        check(uid.startswith("65532"), f"{svc} работает не от root: {uid!r}")

    if failed:
        print(f"\nНЕ ПРОШЛО: {len(failed)}")
        sys.exit(1)
    print("\nВсё в порядке.")


if __name__ == "__main__":
    main()
