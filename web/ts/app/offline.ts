// Маршруты для офлайна: подробный трек и тайлы карты в Cache Storage
// браузера. Сохраняются только по кнопке; при выходе из аккаунта стираются.
import { get, Route } from "./api.js";

const ROUTES = "sn-offline-routes";
const TILES = "sn-offline-tiles";
export const MAX_TILES = 600;

export const offlineSupported = () => typeof caches !== "undefined" && "serviceWorker" in navigator;

interface Saved { route: Route; tiles: string[]; saved_at: string }

const lon2x = (lon: number, z: number) => Math.floor(((lon + 180) / 360) * 2 ** z);
const lat2y = (lat: number, z: number) => {
  const r = (lat * Math.PI) / 180;
  return Math.floor(((1 - Math.log(Math.tan(r) + 1 / Math.cos(r)) / Math.PI) / 2) * 2 ** z);
};

// tilesFor — тайлы рамки маршрута (с запасом) на масштабах 10…maxZoom;
// если их больше лимита, крупные масштабы отбрасываются.
export function tilesFor(bbox: [number, number, number, number], maxZoom: number): string[] {
  const [s, w, n, e] = bbox;
  const padLat = Math.max((n - s) * 0.15, 0.005), padLon = Math.max((e - w) * 0.15, 0.005);
  for (let top = Math.min(maxZoom, 16); top >= 10; top--) {
    const out: string[] = [];
    for (let z = 10; z <= top; z++) {
      const x0 = lon2x(w - padLon, z), x1 = lon2x(e + padLon, z), y0 = lat2y(n + padLat, z), y1 = lat2y(s - padLat, z);
      for (let x = x0; x <= x1; x++) for (let y = y0; y <= y1; y++) out.push(`/tiles/${z}/${x}/${y}.png`);
    }
    if (out.length <= MAX_TILES) return out;
  }
  return [];
}

async function fetchTile(url: string, cache: Cache): Promise<void> {
  for (let attempt = 0; attempt < 5; attempt++) {
    const res = await fetch(url);
    if (res.ok) { await cache.put(url, res); return; }
    if (res.status === 429) { await new Promise((r) => setTimeout(r, 1000 * (attempt + 1))); continue; }
    await res.arrayBuffer().catch(() => null);
    return; // нет тайла (например, карта выключена) — маршрут всё равно сохранится
  }
}

export async function saveOffline(id: string, maxZoom: number, onProgress: (done: number, total: number) => void): Promise<void> {
  const route = await get<Route>(`/routes/${id}`);
  const tiles = tilesFor(route.bbox, maxZoom);
  const tc = await caches.open(TILES);
  let done = 0, next = 0;
  onProgress(0, tiles.length);
  const worker = async () => {
    while (next < tiles.length) {
      const url = tiles[next++];
      if (!(await tc.match(url))) await fetchTile(url, tc);
      onProgress(++done, tiles.length);
    }
  };
  await Promise.all([worker(), worker(), worker(), worker()]);
  const rc = await caches.open(ROUTES);
  // настройки карты — чтобы построить её без сети
  const cfg = await get<{ map: unknown }>("/client-config");
  await rc.put("/offline/client-config.json", new Response(JSON.stringify(cfg.map), { headers: { "Content-Type": "application/json" } }));
  const saved: Saved = { route, tiles, saved_at: new Date().toISOString() };
  await rc.put(`/offline/routes/${id}.json`, new Response(JSON.stringify(saved), { headers: { "Content-Type": "application/json" } }));
}

export async function savedRoutes(): Promise<Saved[]> {
  if (typeof caches === "undefined") return [];
  const rc = await caches.open(ROUTES);
  const out: Saved[] = [];
  for (const req of await rc.keys()) {
    if (!new URL(req.url).pathname.startsWith("/offline/routes/")) continue;
    const res = await rc.match(req);
    if (res) out.push((await res.json()) as Saved);
  }
  return out.sort((a, b) => b.saved_at.localeCompare(a.saved_at));
}

export async function isSaved(id: string): Promise<boolean> {
  if (typeof caches === "undefined") return false;
  return !!(await (await caches.open(ROUTES)).match(`/offline/routes/${id}.json`));
}

// removeOffline удаляет маршрут и тайлы, которые не нужны другим сохранённым.
export async function removeOffline(id: string): Promise<void> {
  const all = await savedRoutes();
  const keep = new Set(all.filter((s) => s.route.id !== id).flatMap((s) => s.tiles));
  const mine = all.find((s) => s.route.id === id);
  const tc = await caches.open(TILES);
  for (const t of mine?.tiles ?? []) if (!keep.has(t)) await tc.delete(t);
  await (await caches.open(ROUTES)).delete(`/offline/routes/${id}.json`);
}

export async function clearOffline(): Promise<void> {
  if (typeof caches === "undefined") return;
  await caches.delete(ROUTES);
  await caches.delete(TILES);
}
