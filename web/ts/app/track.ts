// Запись трека телефоном: фильтрация точек GPS, расстояние, черновик в
// localStorage и сборка GPX для загрузки как обычного маршрута.
//
// Источник точек — watchPosition браузера; в приложении для Android его
// подменяет нативный фоновый источник (см. setPositionSource).

export interface Fix { lat: number; lon: number; ele: number | null; acc: number; t: number }
export interface Draft { startedAt: number; segments: Fix[][]; paused: boolean; elapsedMs: number; resumedAt: number | null }

export const MAX_ACCURACY_M = 50; // хуже — точка не пишется
export const MIN_STEP_M = 5;      // ближе к прошлой — стоим на месте
export const MAX_SPEED_MS = 60;   // быстрее — скачок GPS, не движение

const R = 6371008.8;
const rad = (d: number) => (d * Math.PI) / 180;
export function haversine(a: { lat: number; lon: number }, b: { lat: number; lon: number }): number {
  const dLat = rad(b.lat - a.lat), dLon = rad(b.lon - a.lon);
  const h = Math.sin(dLat / 2) ** 2 + Math.cos(rad(a.lat)) * Math.cos(rad(b.lat)) * Math.sin(dLon / 2) ** 2;
  return 2 * R * Math.asin(Math.min(1, Math.sqrt(h)));
}

// accept решает, писать ли точку после last (последней в текущем отрезке).
export function accept(last: Fix | undefined, f: Fix): boolean {
  if (!(f.acc <= MAX_ACCURACY_M) || !Number.isFinite(f.lat) || !Number.isFinite(f.lon)) return false;
  if (!last) return true;
  const d = haversine(last, f), dt = (f.t - last.t) / 1000;
  if (d < MIN_STEP_M) return false;
  return dt <= 0 ? false : d / dt <= MAX_SPEED_MS;
}

export function distance(segments: Fix[][]): number {
  let m = 0;
  for (const s of segments) for (let i = 1; i < s.length; i++) m += haversine(s[i - 1], s[i]);
  return m;
}

export const pointCount = (d: Draft) => d.segments.reduce((n, s) => n + s.length, 0);
export const elapsed = (d: Draft, now = Date.now()) => d.elapsedMs + (d.resumedAt ? now - d.resumedAt : 0);

export function newDraft(now = Date.now()): Draft {
  return { startedAt: now, segments: [[]], paused: false, elapsedMs: 0, resumedAt: now };
}

// addFix — новая версия черновика (или та же, если точка отброшена).
export function addFix(d: Draft, f: Fix): Draft {
  if (d.paused) return d;
  const seg = d.segments[d.segments.length - 1];
  if (!accept(seg[seg.length - 1], f)) return d;
  return { ...d, segments: [...d.segments.slice(0, -1), [...seg, f]] };
}

export function pause(d: Draft, now = Date.now()): Draft {
  return d.paused ? d : { ...d, paused: true, elapsedMs: elapsed(d, now), resumedAt: null };
}

// resume — после паузы новый отрезок: путь между ними не считается.
export function resume(d: Draft, now = Date.now()): Draft {
  if (!d.paused) return d;
  const segs = d.segments[d.segments.length - 1].length === 0 ? d.segments : [...d.segments, []];
  return { ...d, paused: false, resumedAt: now, segments: segs };
}

const esc = (s: string) => s.replace(/[<>&"']/g, (c) => `&#${c.charCodeAt(0)};`);

export function toGPX(d: Draft, name: string): string {
  const segs = d.segments.filter((s) => s.length > 0).map((s) => "<trkseg>" + s.map((f) =>
    `<trkpt lat="${f.lat.toFixed(7)}" lon="${f.lon.toFixed(7)}">` +
    (f.ele !== null && Number.isFinite(f.ele) ? `<ele>${f.ele.toFixed(1)}</ele>` : "") +
    `<time>${new Date(f.t).toISOString()}</time></trkpt>`).join("") + "</trkseg>");
  return `<?xml version="1.0" encoding="UTF-8"?>\n<gpx version="1.1" creator="socialnet" xmlns="http://www.topografix.com/GPX/1/1">` +
    `<trk><name>${esc(name)}</name>${segs.join("")}</trk></gpx>\n`;
}

// ---------- черновик: переживает перезагрузку и закрытие вкладки ----------

const KEY = "sn-track-draft";
export function saveDraft(d: Draft | null) {
  try { if (d) localStorage.setItem(KEY, JSON.stringify(d)); else localStorage.removeItem(KEY); } catch { /* приватный режим: только в памяти */ }
}
export function loadDraft(): Draft | null {
  try {
    const v = JSON.parse(localStorage.getItem(KEY) ?? "null") as Draft | null;
    return v && Array.isArray(v.segments) ? v : null;
  } catch { return null; }
}

// ---------- источник координат ----------

export type PositionSource = (onFix: (f: Fix) => void, onError: (msg: string) => void) => () => void;

export const browserSource: PositionSource = (onFix, onError) => {
  if (!("geolocation" in navigator)) { onError("Браузер не умеет определять местоположение."); return () => {}; }
  const id = navigator.geolocation.watchPosition(
    (p) => onFix({ lat: p.coords.latitude, lon: p.coords.longitude, ele: p.coords.altitude, acc: p.coords.accuracy, t: p.timestamp || Date.now() }),
    (e) => onError(e.code === e.PERMISSION_DENIED ? "Нет доступа к геолокации: разрешите его сайту в настройках браузера." :
      e.code === e.POSITION_UNAVAILABLE ? "Нет сигнала GPS. Выйдите на открытое место." : "GPS долго не отвечает…"),
    { enableHighAccuracy: true, maximumAge: 0, timeout: 30000 },
  );
  return () => navigator.geolocation.clearWatch(id);
};

let source: PositionSource = browserSource;
export const positionSource = () => source;
// setPositionSource — для приложения: фоновая запись при выключенном экране.
export function setPositionSource(s: PositionSource) { source = s; }
export let backgroundCapable = false;
export function markBackgroundCapable() { backgroundCapable = true; }
