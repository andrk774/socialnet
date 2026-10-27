#!/usr/bin/env python3
"""Проверка боевой конфигурации (docker-compose.prod.yml) после запуска.

    DOMAIN=localhost docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d --wait
    python3 deploy/prod_check.py

С DOMAIN=localhost Caddy выпускает сертификат своим локальным центром —
его корневой сертификат скрипт берёт из контейнера и проверяет TLS по-настоящему.
На боевом сервере: BASE=https://ваш-домен python3 deploy/prod_check.py
"""
import http.client
import json
import os
import socket
import ssl
import subprocess
import sys
import uuid

BASE_HOST = os.environ.get("CHECK_HOST", "localhost")
COMPOSE = ["docker", "compose", "-f", "docker-compose.yml", "-f", "docker-compose.prod.yml"]
failed = []


def check(cond, what):
    print(("  ✓ " if cond else "  ✗ ") + what, flush=True)
    if not cond:
        failed.append(what)


def tls_context():
    ctx = ssl.create_default_context()
    if BASE_HOST == "localhost":
        root = subprocess.run(COMPOSE + ["exec", "-T", "caddy", "cat", "/data/caddy/pki/authorities/local/root.crt"],
                              capture_output=True, check=True).stdout.decode()
        ctx.load_verify_locations(cadata=root)
    return ctx


def request(method, path, body=None, headers=None, https=True):
    conn = (http.client.HTTPSConnection(BASE_HOST, 443, context=CTX, timeout=20) if https
            else http.client.HTTPConnection(BASE_HOST, 80, timeout=20))
    h = {"Content-Type": "application/json"} if body is not None else {}
    h.update(headers or {})
    conn.request(method, path, json.dumps(body) if body is not None else None, h)
    r = conn.getresponse()
    data = r.read()
    try:
        out = json.loads(data) if data else None
    except ValueError:
        out = data.decode(errors="replace")
    return r.status, dict((k.lower(), v) for k, v in r.getheaders()), out


def closed(port):
    with socket.socket() as s:
        s.settimeout(2)
        return s.connect_ex(("127.0.0.1", port)) != 0


CTX = tls_context()


def main():
    print("• HTTPS и заголовки")
    code, h, body = request("GET", "/health")
    check(code == 200 and body == {"status": "ok"}, f"/health по HTTPS: {code} {body}")
    check("max-age=31536000" in h.get("strict-transport-security", ""), "HSTS")
    check("server" not in h, "без заголовка Server")
    code, h, _ = request("GET", "/u/anna?x=1", https=False)
    check(code in (301, 308) and h.get("location", "").startswith("https://"), f"HTTP → HTTPS: {code} {h.get('location')}")
    code, h, page = request("GET", "/")
    check(code == 200 and "script-src 'self'" in h.get("content-security-policy", ""), "страница приложения с CSP")
    code, _, page = request("GET", "/legal/privacy")
    check(code == 200 and "152-ФЗ" in str(page), "документы открываются")

    print("• наружу открыт только Caddy")
    for port, name in ((8080, "API"), (5432, "PostgreSQL"), (6379, "Redis"), (8025, "Mailpit"), (9090, "метрики")):
        check(closed(port), f"порт {port} ({name}) закрыт")

    print("• cookie и реальный IP клиента")
    run = uuid.uuid4().hex[:6]
    code, h, body = request("POST", "/api/v1/auth/register", {
        "email": f"prod{run}@example.com", "username": f"prod{run}", "password": f"password-{run}",
        "age_confirmed": True, "accept_terms": True, "pd_consent": True,
    }, headers={"X-Forwarded-For": "203.0.113.77", "X-Real-IP": "203.0.113.77"})
    check(code == 201, f"регистрация через Caddy: {code}")
    cookie = h.get("set-cookie", "")
    check("Secure" in cookie and "HttpOnly" in cookie and "SameSite=Strict" in cookie, f"cookie: {cookie[:120]}")
    if code == 201:
        token = body["access_token"]
        _, _, ev = request("GET", "/api/v1/security/events", headers={"Authorization": "Bearer " + token})
        ips = {e.get("ip") for e in (ev or {}).get("items", [])}
        check(ips and "203.0.113.77" not in ips and None not in ips, f"подставной X-Forwarded-For не принят: {ips}")

    if failed:
        print(f"\nНЕ ПРОШЛО: {len(failed)}")
        sys.exit(1)
    print("\nБоевая конфигурация в порядке.")


if __name__ == "__main__":
    main()
