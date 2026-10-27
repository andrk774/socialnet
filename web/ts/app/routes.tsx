// Маршрут из GPX: сводка, схема трека, профиль высот и карта. Схема и профиль
// рисуются в SVG без внешних запросов; карта — Leaflet с тайлами через прокси
// нашего сервера, так что браузер посетителя к OSM не обращается.
import { h } from "../vendor/preact/index.js";
import { useEffect, useLayoutEffect, useRef, useState } from "../vendor/preact/hooks.js";
import * as L from "../vendor/leaflet/leaflet-src.esm.js";
import { del, get, Mark, MarkKind, post, Route, saveFile, upload } from "./api.js";
import { useMe } from "./session.js";
import { WeatherToggle } from "./weather.js";
import { isSaved, offlineSupported, removeOffline, saveOffline } from "./offline.js";
import { ErrorLine, Loading } from "./ui.js";

interface MapConfig { enabled: boolean; tiles: string; attribution: string; max_zoom: number }
let mapConfig: Promise<MapConfig> | null = null;
let clientConfig: Promise<{ weather?: { enabled: boolean } }> | null = null;
export function loadClientConfig(): Promise<{ weather?: { enabled: boolean } }> {
  clientConfig ??= get<{ weather?: { enabled: boolean } }>("/client-config").catch((e) => { clientConfig = null; throw e; });
  return clientConfig;
}
const CONFIG_KEY = "/offline/client-config.json";

// loadMapConfig: настройки карты с сервера; без сети — сохранённые вместе с
// офлайн-маршрутами (иначе карта без связи не построится).
export function loadMapConfig(): Promise<MapConfig> {
  mapConfig ??= get<{ map: MapConfig }>("/client-config").then((c) => {
    if (typeof caches !== "undefined") {
      caches.open("sn-offline-routes").then((cache) => cache.match(CONFIG_KEY).then((hit) =>
        hit ? cache.put(CONFIG_KEY, new Response(JSON.stringify(c.map))) : undefined)).catch(() => {});
    }
    return c.map;
  }).catch(async (e) => {
    mapConfig = null; // в следующий раз спросим снова
    const hit = typeof caches !== "undefined" ? await caches.open("sn-offline-routes").then((c) => c.match(CONFIG_KEY)) : undefined;
    if (hit) return (await hit.json()) as MapConfig;
    throw e;
  });
  return mapConfig;
}

const escapeHTML = (s: string) => s.replace(/[&<>"']/g, (c) => `&#${c.charCodeAt(0)};`);

function label(text: string): HTMLElement {
  const el = document.createElement("span");
  el.textContent = text; // имена точек пишет автор GPX — только как текст
  return el;
}

const nf1 = new Intl.NumberFormat("ru", { maximumFractionDigits: 1 });
const nf0 = new Intl.NumberFormat("ru", { maximumFractionDigits: 0 });

export const km = (m: number) => (m < 1000 ? `${nf0.format(m)} м` : `${nf1.format(m / 1000)} км`);

export function hours(sec: number): string {
  const h = Math.floor(sec / 3600), m = Math.round((sec % 3600) / 60);
  return h > 0 ? `${h} ч${m ? ` ${m} мин` : ""}` : `${m} мин`;
}

export function RouteCard({ route }: { route: Route }) {
  const [error, setError] = useState<unknown>(null);
  const [map, setMap] = useState(false);
  const [canMap, setCanMap] = useState(false);
  const [maxZoom, setMaxZoom] = useState(15);
  const [offline, setOffline] = useState<"no" | "yes" | string>("no"); // строка — ход загрузки
  const [canWeather, setCanWeather] = useState(false);
  useEffect(() => {
    loadClientConfig().then((c) => setCanWeather(c.weather?.enabled ?? false), () => {});
    loadMapConfig().then((c) => { setCanMap(c.enabled); setMaxZoom(c.max_zoom); }, () => setCanMap(false));
    if (offlineSupported()) isSaved(route.id).then((ok) => setOffline(ok ? "yes" : "no"), () => {});
  }, []);
  const toggleOffline = async () => {
    setError(null);
    try {
      if (offline === "yes") { await removeOffline(route.id); setOffline("no"); return; }
      await saveOffline(route.id, maxZoom, (d, t) => setOffline(t ? `${Math.round((d / t) * 100)}%` : "…"));
      setOffline("yes");
    } catch (e) { setError(e); setOffline("no"); }
  };
  const stats: [string, string][] = [["Расстояние", km(route.distance_m)]];
  if (route.max_ele !== null) {
    stats.push(["Набор", `↑ ${nf0.format(route.gain_m)} м`], ["Сброс", `↓ ${nf0.format(route.loss_m)} м`]);
    stats.push(["Высота", `${nf0.format(route.min_ele ?? 0)}–${nf0.format(route.max_ele)} м`]);
  }
  if (route.moving_sec > 0) stats.push(["В движении", hours(route.moving_sec)]);
  else if (route.duration_sec > 0) stats.push(["Время", hours(route.duration_sec)]);

  return (
    <section class="route" aria-label={`Маршрут: ${route.title}`}>
      <header class="route-head">
        <strong class="route-title">{route.title}</strong>
        <span class="route-actions">
          {canMap && <button class="link" aria-expanded={map} onClick={() => setMap(!map)}>{map ? "Скрыть карту" : "Показать на карте"}</button>}
          {offlineSupported() && <button class="link" disabled={offline !== "no" && offline !== "yes"} onClick={toggleOffline}>
            {offline === "yes" ? "Сохранён офлайн ✓" : offline === "no" ? "Сохранить офлайн" : `Сохраняю… ${offline}`}</button>}
          <a class="link" href={`/routes/${route.id}/print`}>Печать</a>
          <button class="link" onClick={() => saveFile(route.gpx_url, `${route.title}.gpx`).catch(setError)}>Скачать GPX</button>
        </span>
      </header>
      <dl class="route-stats">
        {stats.map(([k, v]) => <div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}
      </dl>
      {map && <MapPanel route={route} />}
      <div class="route-graphics">
        <TrackSketch points={route.preview} />
        {route.profile.length > 1 && <ElevationProfile profile={route.profile} />}
      </div>
      {canWeather && route.preview.length > 0 && <WeatherToggle lat={route.preview[0][0]} lon={route.preview[0][1]} elevation={route.max_ele} />}
      {route.hidden_ends_m > 0 && <p class="muted route-note">Первые и последние {route.hidden_ends_m} м скрыты автором.</p>}
      <ErrorLine error={error} />
    </section>
  );
}

// RouteMap — трек на карте. Подробная геометрия грузится только по запросу:
// в ленте у записи лишь упрощённая схема.
// printMode — для листа маршрута: без кнопок, без перетаскивания, точки
// подписаны номерами из таблицы.
export const markKinds: Record<MarkKind, string> = { spring: "Родник", camp: "Стоянка", ford: "Брод", danger: "Опасно", view: "Вид", other: "Другое" };

export function RouteMap({ route, printMode = false, onPick, picked }: {
  route: Route; printMode?: boolean; onPick?: (lat: number, lon: number) => void; picked?: [number, number] | null;
}) {
  const box = useRef<HTMLDivElement>(null);
  const [full, setFull] = useState(false);
  const [detail, setDetail] = useState<Route | null>(route.detail ? route : null);
  const [error, setError] = useState<unknown>(null);
  const mapRef = useRef<L.LeafletMap | null>(null);

  useEffect(() => {
    if (route.detail) setDetail(route);
    else get<Route>(`/routes/${route.id}`).then(setDetail, setError);
  }, [route]);
  const pickRef = useRef(onPick);
  pickRef.current = onPick;
  const pickLayer = useRef<L.LayerGroup | null>(null);

  useEffect(() => {
    if (!detail || !box.current) return;
    let alive = true, m: L.LeafletMap | null = null;
    loadMapConfig().then((cfg) => {
      if (!alive || !box.current) return;
      m = L.map(box.current, printMode
        ? { zoomControl: false, scrollWheelZoom: false, dragging: false, touchZoom: false, doubleClickZoom: false, boxZoom: false, keyboard: false, maxZoom: cfg.max_zoom }
        : { scrollWheelZoom: false, maxZoom: cfg.max_zoom });
      m.attributionControl.setPrefix(false);
      L.tileLayer(cfg.tiles, { maxZoom: cfg.max_zoom, attribution: escapeHTML(cfg.attribution) }).addTo(m);
      const segs = detail.detail?.length ? detail.detail : [detail.preview];
      for (const seg of segs) {
        L.polyline(seg, { className: "map-route-halo", weight: 7, interactive: false }).addTo(m);
        L.polyline(seg, { className: "map-route", weight: 4, interactive: false }).addTo(m);
      }
      (detail.waypoints ?? []).forEach((wp, i) => {
        const p = L.circleMarker([wp.lat, wp.lon], { className: "map-waypoint", radius: 5, weight: 2, fillOpacity: 1 }).addTo(m!);
        if (printMode) p.bindTooltip(label(String(i + 1)), { direction: "top", permanent: true });
        else if (wp.name) p.bindTooltip(label(wp.name), { direction: "top" });
      });
      const offset = detail.waypoints?.length ?? 0;
      (detail.marks ?? []).forEach((mk, i) => {
        const p = L.circleMarker([mk.lat, mk.lon], { className: `map-mark mark-${mk.kind}`, radius: 7, weight: 2, fillOpacity: 1 }).addTo(m!);
        if (printMode) p.bindTooltip(label(String(offset + i + 1)), { direction: "top", permanent: true });
        else p.bindTooltip(label(`${markKinds[mk.kind]}: ${mk.title}`), { direction: "top" });
      });
      pickLayer.current = L.layerGroup().addTo(m);
      m.on("click", (ev) => pickRef.current?.(ev.latlng.lat, ev.latlng.lng));
      const first = segs[0][0], lastSeg = segs[segs.length - 1], last = lastSeg[lastSeg.length - 1];
      if (first) L.circleMarker(first, { className: "map-start", radius: 7, weight: 3, fillOpacity: 1 }).addTo(m).bindTooltip(label("Старт"));
      if (last) L.circleMarker(last, { className: "map-finish", radius: 7, weight: 3, fillOpacity: 1 }).addTo(m).bindTooltip(label("Финиш"));
      const [s, w, n, e] = detail.bbox;
      m.fitBounds([[s, w], [n, e]], { padding: [24, 24], maxZoom: 15 });
      mapRef.current = m;
    }, setError);
    return () => { alive = false; m?.remove(); mapRef.current = null; pickLayer.current = null; };
  }, [detail]);

  // точка, выбранная щелчком для новой отметки
  useEffect(() => {
    const g = pickLayer.current;
    if (!g) return;
    g.clearLayers();
    if (picked) L.circleMarker(picked, { className: "map-picked", radius: 8, weight: 3, fillOpacity: .4 }).addTo(g);
  }, [picked, detail]);

  // сразу после отрисовки, а не в следующем кадре: Esc работает с первого мгновения
  useLayoutEffect(() => {
    mapRef.current?.invalidateSize();
    if (!full) return;
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") setFull(false); };
    document.addEventListener("keydown", onKey);
    document.body.classList.add("no-scroll");
    return () => { document.removeEventListener("keydown", onKey); document.body.classList.remove("no-scroll"); };
  }, [full]);

  return (
    <div class={full ? "route-map-wrap route-map-full" : printMode ? "route-map-wrap route-map-print" : "route-map-wrap"} role={full ? "dialog" : undefined} aria-label={full ? `Карта: ${route.title}` : undefined}>
      {!detail && !error && <Loading />}
      <ErrorLine error={error} />
      <div ref={box} class="route-map" aria-label="Карта маршрута" />
      {detail && !printMode && <button class="map-full-toggle" onClick={() => setFull(!full)}>{full ? "Закрыть" : "Во весь экран"}</button>}
    </div>
  );
}

// TrackSketch — линия трека в своём масштабе (равнопромежуточная проекция с
// поправкой на широту, чтобы север был сверху и пропорции не искажались).
export function TrackSketch({ points }: { points: [number, number][] }) {
  if (points.length < 2) return null;
  const W = 240, H = 180, pad = 12;
  const lat0 = (points.reduce((s, p) => s + p[0], 0) / points.length) * Math.PI / 180;
  const xs = points.map((p) => p[1] * Math.cos(lat0)), ys = points.map((p) => p[0]);
  const minX = Math.min(...xs), maxX = Math.max(...xs), minY = Math.min(...ys), maxY = Math.max(...ys);
  const scale = Math.min((W - 2 * pad) / (maxX - minX || 1e-9), (H - 2 * pad) / (maxY - minY || 1e-9));
  const ox = (W - (maxX - minX) * scale) / 2, oy = (H - (maxY - minY) * scale) / 2;
  const X = (x: number) => ox + (x - minX) * scale, Y = (y: number) => H - oy - (y - minY) * scale;
  const d = xs.map((x, i) => `${i ? "L" : "M"}${X(x).toFixed(1)} ${Y(ys[i]).toFixed(1)}`).join("");
  const last = xs.length - 1;
  return (
    <svg class="route-sketch" viewBox={`0 0 ${W} ${H}`} role="img" aria-label="Схема трека">
      <path d={d} fill="none" class="route-line" stroke-width="2.5" stroke-linejoin="round" stroke-linecap="round" />
      <circle cx={X(xs[0])} cy={Y(ys[0])} r="4.5" class="route-start" />
      <circle cx={X(xs[last])} cy={Y(ys[last])} r="4.5" class="route-finish" />
      <text x={W - 6} y={14} text-anchor="end" class="route-north">С ↑</text>
    </svg>
  );
}

// ElevationProfile — высота по пройденному расстоянию.
export function ElevationProfile({ profile }: { profile: { d: number; e: number }[] }) {
  const W = 360, H = 180, L = 40, R = 8, T = 10, B = 24;
  const total = profile[profile.length - 1].d || 1;
  let lo = Math.min(...profile.map((p) => p.e)), hi = Math.max(...profile.map((p) => p.e));
  const step = niceStep((hi - lo) / 3 || 10);
  lo = Math.floor(lo / step) * step;
  hi = Math.max(Math.ceil(hi / step) * step, lo + step);
  const X = (d: number) => L + (d / total) * (W - L - R);
  const Y = (e: number) => T + (1 - (e - lo) / (hi - lo)) * (H - T - B);
  const line = profile.map((p, i) => `${i ? "L" : "M"}${X(p.d).toFixed(1)} ${Y(p.e).toFixed(1)}`).join("");
  const area = `${line}L${X(total).toFixed(1)} ${H - B}L${L} ${H - B}Z`;
  const yTicks: number[] = [];
  for (let e = lo; e <= hi + 1e-9; e += step) yTicks.push(e);
  const kmStep = niceStep(total / 1000 / 4) * 1000;
  const xTicks: number[] = [];
  for (let d = 0; d <= total + 1; d += kmStep) xTicks.push(d);
  return (
    <svg class="route-profile" viewBox={`0 0 ${W} ${H}`} role="img"
      aria-label={`Профиль высот: от ${Math.round(lo)} до ${Math.round(hi)} метров на ${km(total)}`}>
      {yTicks.map((e, i) => (
        <g key={`y${e}`}>
          <line x1={L} x2={W - R} y1={Y(e)} y2={Y(e)} class="route-grid" />
          <text x={L - 6} y={Y(e) + 4} text-anchor="end" class="route-axis">{nf0.format(e)}{i === yTicks.length - 1 ? " м" : ""}</text>
        </g>
      ))}
      {xTicks.map((d, i) => (
        <text key={`x${d}`} x={X(d)} y={H - 6} text-anchor={i === 0 ? "start" : i === xTicks.length - 1 && X(d) > W - R - 20 ? "end" : "middle"} class="route-axis">
          {nf1.format(d / 1000)}{i === xTicks.length - 1 ? " км" : ""}
        </text>
      ))}
      <path d={area} class="route-area" />
      <path d={line} fill="none" class="route-line" stroke-width="2" stroke-linejoin="round" />
    </svg>
  );
}

function niceStep(raw: number): number {
  if (!(raw > 0)) return 1;
  const p = Math.pow(10, Math.floor(Math.log10(raw)));
  const n = raw / p;
  return (n <= 1 ? 1 : n <= 2 ? 2 : n <= 5 ? 5 : 10) * p;
}

// MapPanel — карта и точки от ходивших (родники, стоянки, броды…).
function MapPanel({ route }: { route: Route }) {
  const [detail, setDetail] = useState<Route | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [adding, setAdding] = useState(false);
  const [picked, setPicked] = useState<[number, number] | null>(null);
  const load = () => get<Route>(`/routes/${route.id}`).then(setDetail, setError);
  useEffect(() => { load(); }, [route.id]);
  if (error) return <ErrorLine error={error} />;
  if (!detail) return <Loading />;
  return (
    <div class="map-panel">
      <RouteMap route={detail} onPick={adding ? (lat, lon) => setPicked([lat, lon]) : undefined} picked={adding ? picked : null} />
      <Marks route={detail} onChange={load} />
      {!adding ? <button class="ghost" onClick={() => { setAdding(true); setPicked(null); }}>Отметить точку</button>
        : <AddMark route={detail} picked={picked} onDone={(ok) => { setAdding(false); setPicked(null); if (ok) load(); }} />}
    </div>
  );
}

const dayFmt = new Intl.DateTimeFormat("ru", { day: "numeric", month: "long", year: "numeric" });

function Marks({ route, onChange }: { route: Route; onChange: () => void }) {
  const me = useMe()?.id;
  const [error, setError] = useState<unknown>(null);
  const marks = route.marks ?? [];
  if (marks.length === 0) return null;
  const run = (fn: () => Promise<unknown>) => { setError(null); fn().then(onChange, setError); };
  return (
    <section class="marks">
      <h3>Точки на маршруте</h3>
      <ul>
        {marks.map((mk) => (
          <li key={mk.id} class={`mark mark-${mk.kind}`}>
            {mk.thumb_url && <a href={mk.photo_url!} target="_blank" rel="noopener"><img src={mk.thumb_url} alt="" width={64} height={64} loading="lazy" /></a>}
            <div class="grow">
              <span class="mark-kind">{markKinds[mk.kind]}</span> <strong>{mk.title}</strong>
              {mk.note && <p>{mk.note}</p>}
              <p class="muted">Проверено {dayFmt.format(new Date(mk.verified_on + "T12:00:00"))}{mk.checks > 0 ? ` · подтвердили: ${mk.checks}` : ""} · отметил(а) <a href={`/u/${mk.author.username}`}>{mk.author.first_name || mk.author.username}</a></p>
              <div class="row tight">
                <button class="link" disabled={mk.my_check && mk.verified_on === new Date().toISOString().slice(0, 10)} onClick={() => run(() => post(`/routes/${route.id}/marks/${mk.id}/check`))}>{mk.my_check ? "Снова на месте" : "На месте"}</button>
                {(mk.author.id === me || route.owner_id === me) && <button class="link" onClick={() => run(() => del(`/routes/${route.id}/marks/${mk.id}`))}>Удалить</button>}
              </div>
            </div>
          </li>
        ))}
      </ul>
      <ErrorLine error={error} />
    </section>
  );
}

function AddMark({ route, picked, onDone }: { route: Route; picked: [number, number] | null; onDone: (ok: boolean) => void }) {
  const [f, setF] = useState({ kind: "spring" as MarkKind, title: "", note: "" });
  const [file, setFile] = useState<File | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  async function submit(e: Event) {
    e.preventDefault();
    if (!picked) return;
    setBusy(true); setError(null);
    try {
      const photo = file ? await upload<{ id: string }>("/media", file) : null;
      await post<Mark>(`/routes/${route.id}/marks`, { ...f, lat: picked[0], lon: picked[1], photo_id: photo?.id ?? null });
      onDone(true);
    } catch (err) { setError(err); } finally { setBusy(false); }
  }
  return (
    <form class="stack add-mark" onSubmit={submit}>
      <p class="hint">{picked ? `Место: ${picked[0].toFixed(5)}, ${picked[1].toFixed(5)}` : "Щёлкните по карте рядом с треком, где находится точка"}</p>
      <div class="row-2">
        <label>Что это
          <select value={f.kind} onChange={(e) => setF({ ...f, kind: (e.target as HTMLSelectElement).value as MarkKind })}>
            {(Object.keys(markKinds) as MarkKind[]).map((k) => <option key={k} value={k}>{markKinds[k]}</option>)}
          </select>
        </label>
        <label>Название<input value={f.title} required maxLength={80} onInput={(e) => setF({ ...f, title: (e.target as HTMLInputElement).value })} placeholder="Родник у тропы" /></label>
      </div>
      <label>Заметка<textarea rows={2} maxLength={1000} value={f.note} onInput={(e) => setF({ ...f, note: (e.target as HTMLTextAreaElement).value })} placeholder="Течёт ли в августе, где лучше ставить палатки" /></label>
      <label>Фото<input type="file" accept="image/jpeg,image/png" onChange={(e) => setFile((e.target as HTMLInputElement).files?.[0] ?? null)} /></label>
      <ErrorLine error={error} />
      <div class="row">
        <button class="primary" disabled={!picked || busy}>Сохранить точку</button>
        <button type="button" class="ghost" onClick={() => onDone(false)}>Отмена</button>
      </div>
    </form>
  );
}
