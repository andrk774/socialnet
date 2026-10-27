// Сервис-воркер socialnet: приложение открывается без сети, а сохранённые
// для офлайна маршруты показываются вместе с картой.
//
// Список файлов и версию подставляет сервер при отдаче /sw.js — после
// обновления сайта воркер меняется, и старый кеш оболочки удаляется.
//
// Ответы API не кешируются никогда: в них личные данные. Маршруты для
// офлайна сохраняет сама страница по явной кнопке (кеш sn-offline-*), а при
// выходе из аккаунта они стираются.
const VERSION = "__VERSION__";
const ASSETS = __ASSETS__;
const SHELL = "sn-shell-" + VERSION;
const TILES = "sn-offline-tiles";

self.addEventListener("install", (event) => {
  event.waitUntil(caches.open(SHELL).then((c) => c.addAll(["/", ...ASSETS])).then(() => self.skipWaiting()));
});

self.addEventListener("activate", (event) => {
  event.waitUntil((async () => {
    for (const key of await caches.keys()) {
      if (key.startsWith("sn-shell-") && key !== SHELL) await caches.delete(key);
    }
    await self.clients.claim();
  })());
});

self.addEventListener("fetch", (event) => {
  const req = event.request;
  if (req.method !== "GET") return;
  const url = new URL(req.url);
  if (url.origin !== self.location.origin) return;
  const p = url.pathname;
  // личное и потоковое — только из сети
  if (p.startsWith("/api/") || p.startsWith("/media/") || p.startsWith("/video/") || p.startsWith("/audio/")) return;

  if (p.startsWith("/tiles/")) {
    // сохранённые тайлы — сразу из кеша, остальные из сети
    event.respondWith(caches.open(TILES).then((c) => c.match(req)).then((hit) => hit || fetch(req)));
    return;
  }
  if (p.startsWith("/static/")) {
    event.respondWith(caches.open(SHELL).then((c) => c.match(req, { ignoreSearch: true })).then((hit) => hit || fetch(req)));
    return;
  }
  if (req.mode === "navigate") {
    // страница — из сети, без сети — оболочка приложения
    event.respondWith(fetch(req).catch(() => caches.open(SHELL).then((c) => c.match("/"))));
  }
});
