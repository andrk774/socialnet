// Маршрут из GPX: сводка, схема трека, профиль высот и карта. Схема и профиль
// рисуются в SVG без внешних запросов; карта — Leaflet с тайлами через прокси
// нашего сервера, так что браузер посетителя к OSM не обращается.
import { h } from "../vendor/preact/index.js";
import { useEffect, useLayoutEffect, useRef, useState } from "../vendor/preact/hooks.js";
import * as L from "../vendor/leaflet/leaflet-src.esm.js";
import { get, saveFile } from "./api.js";
import { isSaved, offlineSupported, removeOffline, saveOffline } from "./offline.js";
import { ErrorLine, Loading } from "./ui.js";
let mapConfig = null;
const CONFIG_KEY = "/offline/client-config.json";
// loadMapConfig: настройки карты с сервера; без сети — сохранённые вместе с
// офлайн-маршрутами (иначе карта без связи не построится).
export function loadMapConfig() {
    mapConfig ??= get("/client-config").then((c) => {
        if (typeof caches !== "undefined") {
            caches.open("sn-offline-routes").then((cache) => cache.match(CONFIG_KEY).then((hit) => hit ? cache.put(CONFIG_KEY, new Response(JSON.stringify(c.map))) : undefined)).catch(() => { });
        }
        return c.map;
    }).catch(async (e) => {
        mapConfig = null; // в следующий раз спросим снова
        const hit = typeof caches !== "undefined" ? await caches.open("sn-offline-routes").then((c) => c.match(CONFIG_KEY)) : undefined;
        if (hit)
            return (await hit.json());
        throw e;
    });
    return mapConfig;
}
const escapeHTML = (s) => s.replace(/[&<>"']/g, (c) => `&#${c.charCodeAt(0)};`);
function label(text) {
    const el = document.createElement("span");
    el.textContent = text; // имена точек пишет автор GPX — только как текст
    return el;
}
const nf1 = new Intl.NumberFormat("ru", { maximumFractionDigits: 1 });
const nf0 = new Intl.NumberFormat("ru", { maximumFractionDigits: 0 });
export const km = (m) => (m < 1000 ? `${nf0.format(m)} м` : `${nf1.format(m / 1000)} км`);
export function hours(sec) {
    const h = Math.floor(sec / 3600), m = Math.round((sec % 3600) / 60);
    return h > 0 ? `${h} ч${m ? ` ${m} мин` : ""}` : `${m} мин`;
}
export function RouteCard({ route }) {
    const [error, setError] = useState(null);
    const [map, setMap] = useState(false);
    const [canMap, setCanMap] = useState(false);
    const [maxZoom, setMaxZoom] = useState(15);
    const [offline, setOffline] = useState("no"); // строка — ход загрузки
    useEffect(() => {
        loadMapConfig().then((c) => { setCanMap(c.enabled); setMaxZoom(c.max_zoom); }, () => setCanMap(false));
        if (offlineSupported())
            isSaved(route.id).then((ok) => setOffline(ok ? "yes" : "no"), () => { });
    }, []);
    const toggleOffline = async () => {
        setError(null);
        try {
            if (offline === "yes") {
                await removeOffline(route.id);
                setOffline("no");
                return;
            }
            await saveOffline(route.id, maxZoom, (d, t) => setOffline(t ? `${Math.round((d / t) * 100)}%` : "…"));
            setOffline("yes");
        }
        catch (e) {
            setError(e);
            setOffline("no");
        }
    };
    const stats = [["Расстояние", km(route.distance_m)]];
    if (route.max_ele !== null) {
        stats.push(["Набор", `↑ ${nf0.format(route.gain_m)} м`], ["Сброс", `↓ ${nf0.format(route.loss_m)} м`]);
        stats.push(["Высота", `${nf0.format(route.min_ele ?? 0)}–${nf0.format(route.max_ele)} м`]);
    }
    if (route.moving_sec > 0)
        stats.push(["В движении", hours(route.moving_sec)]);
    else if (route.duration_sec > 0)
        stats.push(["Время", hours(route.duration_sec)]);
    return (h("section", { class: "route", "aria-label": `Маршрут: ${route.title}` },
        h("header", { class: "route-head" },
            h("strong", { class: "route-title" }, route.title),
            h("span", { class: "route-actions" },
                canMap && h("button", { class: "link", "aria-expanded": map, onClick: () => setMap(!map) }, map ? "Скрыть карту" : "Показать на карте"),
                offlineSupported() && h("button", { class: "link", disabled: offline !== "no" && offline !== "yes", onClick: toggleOffline }, offline === "yes" ? "Сохранён офлайн ✓" : offline === "no" ? "Сохранить офлайн" : `Сохраняю… ${offline}`),
                h("a", { class: "link", href: `/routes/${route.id}/print` }, "\u041F\u0435\u0447\u0430\u0442\u044C"),
                h("button", { class: "link", onClick: () => saveFile(route.gpx_url, `${route.title}.gpx`).catch(setError) }, "\u0421\u043A\u0430\u0447\u0430\u0442\u044C GPX"))),
        h("dl", { class: "route-stats" }, stats.map(([k, v]) => h("div", { key: k },
            h("dt", null, k),
            h("dd", null, v)))),
        map && h(RouteMap, { route: route }),
        h("div", { class: "route-graphics" },
            h(TrackSketch, { points: route.preview }),
            route.profile.length > 1 && h(ElevationProfile, { profile: route.profile })),
        route.hidden_ends_m > 0 && h("p", { class: "muted route-note" },
            "\u041F\u0435\u0440\u0432\u044B\u0435 \u0438 \u043F\u043E\u0441\u043B\u0435\u0434\u043D\u0438\u0435 ",
            route.hidden_ends_m,
            " \u043C \u0441\u043A\u0440\u044B\u0442\u044B \u0430\u0432\u0442\u043E\u0440\u043E\u043C."),
        h(ErrorLine, { error: error })));
}
// RouteMap — трек на карте. Подробная геометрия грузится только по запросу:
// в ленте у записи лишь упрощённая схема.
// printMode — для листа маршрута: без кнопок, без перетаскивания, точки
// подписаны номерами из таблицы.
export function RouteMap({ route, printMode = false }) {
    const box = useRef(null);
    const [full, setFull] = useState(false);
    const [detail, setDetail] = useState(route.detail ? route : null);
    const [error, setError] = useState(null);
    const mapRef = useRef(null);
    useEffect(() => {
        if (!detail)
            get(`/routes/${route.id}`).then(setDetail, setError);
    }, [route.id]);
    useEffect(() => {
        if (!detail || !box.current)
            return;
        let alive = true, m = null;
        loadMapConfig().then((cfg) => {
            if (!alive || !box.current)
                return;
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
                const p = L.circleMarker([wp.lat, wp.lon], { className: "map-waypoint", radius: 5, weight: 2, fillOpacity: 1 }).addTo(m);
                if (printMode)
                    p.bindTooltip(label(String(i + 1)), { direction: "top", permanent: true });
                else if (wp.name)
                    p.bindTooltip(label(wp.name), { direction: "top" });
            });
            const first = segs[0][0], lastSeg = segs[segs.length - 1], last = lastSeg[lastSeg.length - 1];
            if (first)
                L.circleMarker(first, { className: "map-start", radius: 7, weight: 3, fillOpacity: 1 }).addTo(m).bindTooltip(label("Старт"));
            if (last)
                L.circleMarker(last, { className: "map-finish", radius: 7, weight: 3, fillOpacity: 1 }).addTo(m).bindTooltip(label("Финиш"));
            const [s, w, n, e] = detail.bbox;
            m.fitBounds([[s, w], [n, e]], { padding: [24, 24], maxZoom: 15 });
            mapRef.current = m;
        }, setError);
        return () => { alive = false; m?.remove(); mapRef.current = null; };
    }, [detail]);
    // сразу после отрисовки, а не в следующем кадре: Esc работает с первого мгновения
    useLayoutEffect(() => {
        mapRef.current?.invalidateSize();
        if (!full)
            return;
        const onKey = (e) => { if (e.key === "Escape")
            setFull(false); };
        document.addEventListener("keydown", onKey);
        document.body.classList.add("no-scroll");
        return () => { document.removeEventListener("keydown", onKey); document.body.classList.remove("no-scroll"); };
    }, [full]);
    return (h("div", { class: full ? "route-map-wrap route-map-full" : printMode ? "route-map-wrap route-map-print" : "route-map-wrap", role: full ? "dialog" : undefined, "aria-label": full ? `Карта: ${route.title}` : undefined },
        !detail && !error && h(Loading, null),
        h(ErrorLine, { error: error }),
        h("div", { ref: box, class: "route-map", "aria-label": "\u041A\u0430\u0440\u0442\u0430 \u043C\u0430\u0440\u0448\u0440\u0443\u0442\u0430" }),
        detail && !printMode && h("button", { class: "map-full-toggle", onClick: () => setFull(!full) }, full ? "Закрыть" : "Во весь экран")));
}
// TrackSketch — линия трека в своём масштабе (равнопромежуточная проекция с
// поправкой на широту, чтобы север был сверху и пропорции не искажались).
export function TrackSketch({ points }) {
    if (points.length < 2)
        return null;
    const W = 240, H = 180, pad = 12;
    const lat0 = (points.reduce((s, p) => s + p[0], 0) / points.length) * Math.PI / 180;
    const xs = points.map((p) => p[1] * Math.cos(lat0)), ys = points.map((p) => p[0]);
    const minX = Math.min(...xs), maxX = Math.max(...xs), minY = Math.min(...ys), maxY = Math.max(...ys);
    const scale = Math.min((W - 2 * pad) / (maxX - minX || 1e-9), (H - 2 * pad) / (maxY - minY || 1e-9));
    const ox = (W - (maxX - minX) * scale) / 2, oy = (H - (maxY - minY) * scale) / 2;
    const X = (x) => ox + (x - minX) * scale, Y = (y) => H - oy - (y - minY) * scale;
    const d = xs.map((x, i) => `${i ? "L" : "M"}${X(x).toFixed(1)} ${Y(ys[i]).toFixed(1)}`).join("");
    const last = xs.length - 1;
    return (h("svg", { class: "route-sketch", viewBox: `0 0 ${W} ${H}`, role: "img", "aria-label": "\u0421\u0445\u0435\u043C\u0430 \u0442\u0440\u0435\u043A\u0430" },
        h("path", { d: d, fill: "none", class: "route-line", "stroke-width": "2.5", "stroke-linejoin": "round", "stroke-linecap": "round" }),
        h("circle", { cx: X(xs[0]), cy: Y(ys[0]), r: "4.5", class: "route-start" }),
        h("circle", { cx: X(xs[last]), cy: Y(ys[last]), r: "4.5", class: "route-finish" }),
        h("text", { x: W - 6, y: 14, "text-anchor": "end", class: "route-north" }, "\u0421 \u2191")));
}
// ElevationProfile — высота по пройденному расстоянию.
export function ElevationProfile({ profile }) {
    const W = 360, H = 180, L = 40, R = 8, T = 10, B = 24;
    const total = profile[profile.length - 1].d || 1;
    let lo = Math.min(...profile.map((p) => p.e)), hi = Math.max(...profile.map((p) => p.e));
    const step = niceStep((hi - lo) / 3 || 10);
    lo = Math.floor(lo / step) * step;
    hi = Math.max(Math.ceil(hi / step) * step, lo + step);
    const X = (d) => L + (d / total) * (W - L - R);
    const Y = (e) => T + (1 - (e - lo) / (hi - lo)) * (H - T - B);
    const line = profile.map((p, i) => `${i ? "L" : "M"}${X(p.d).toFixed(1)} ${Y(p.e).toFixed(1)}`).join("");
    const area = `${line}L${X(total).toFixed(1)} ${H - B}L${L} ${H - B}Z`;
    const yTicks = [];
    for (let e = lo; e <= hi + 1e-9; e += step)
        yTicks.push(e);
    const kmStep = niceStep(total / 1000 / 4) * 1000;
    const xTicks = [];
    for (let d = 0; d <= total + 1; d += kmStep)
        xTicks.push(d);
    return (h("svg", { class: "route-profile", viewBox: `0 0 ${W} ${H}`, role: "img", "aria-label": `Профиль высот: от ${Math.round(lo)} до ${Math.round(hi)} метров на ${km(total)}` },
        yTicks.map((e, i) => (h("g", { key: `y${e}` },
            h("line", { x1: L, x2: W - R, y1: Y(e), y2: Y(e), class: "route-grid" }),
            h("text", { x: L - 6, y: Y(e) + 4, "text-anchor": "end", class: "route-axis" },
                nf0.format(e),
                i === yTicks.length - 1 ? " м" : "")))),
        xTicks.map((d, i) => (h("text", { key: `x${d}`, x: X(d), y: H - 6, "text-anchor": i === 0 ? "start" : i === xTicks.length - 1 && X(d) > W - R - 20 ? "end" : "middle", class: "route-axis" },
            nf1.format(d / 1000),
            i === xTicks.length - 1 ? " км" : ""))),
        h("path", { d: area, class: "route-area" }),
        h("path", { d: line, fill: "none", class: "route-line", "stroke-width": "2", "stroke-linejoin": "round" })));
}
function niceStep(raw) {
    if (!(raw > 0))
        return 1;
    const p = Math.pow(10, Math.floor(Math.log10(raw)));
    const n = raw / p;
    return (n <= 1 ? 1 : n <= 2 ? 2 : n <= 5 ? 5 : 10) * p;
}
