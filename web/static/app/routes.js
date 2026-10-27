// Маршрут из GPX: сводка, схема трека, профиль высот и карта. Схема и профиль
// рисуются в SVG без внешних запросов; карта — Leaflet с тайлами через прокси
// нашего сервера, так что браузер посетителя к OSM не обращается.
import { h } from "../vendor/preact/index.js";
import { useEffect, useLayoutEffect, useRef, useState } from "../vendor/preact/hooks.js";
import * as L from "../vendor/leaflet/leaflet-src.esm.js";
import { del, get, patch, post, saveFile, upload } from "./api.js";
import { useMe } from "./session.js";
import { WeatherToggle } from "./weather.js";
import { isSaved, offlineSupported, removeOffline, saveOffline } from "./offline.js";
import { ErrorLine, Loading } from "./ui.js";
import { ReportButton } from "./report.js";
import { RouteReviews } from "./reviews.js";
import { NoticeBanner } from "./notices.js";
let mapConfig = null;
let clientConfig = null;
export function loadClientConfig() {
    clientConfig ??= get("/client-config").catch((e) => { clientConfig = null; throw e; });
    return clientConfig;
}
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
    const me = useMe();
    const [error, setError] = useState(null);
    const [map, setMap] = useState(false);
    const [canMap, setCanMap] = useState(false);
    const [maxZoom, setMaxZoom] = useState(15);
    const [offline, setOffline] = useState("no"); // строка — ход загрузки
    const [canWeather, setCanWeather] = useState(false);
    useEffect(() => {
        loadClientConfig().then((c) => setCanWeather(c.weather?.enabled ?? false), () => { });
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
            // с «Плюсом» — больше тайлов: подробнее карта без сети
            await saveOffline(route.id, maxZoom, (d, t) => setOffline(t ? `${Math.round((d / t) * 100)}%` : "…"), me?.plus ? 2000 : 600);
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
        map && h(MapPanel, { route: route }),
        h("div", { class: "route-graphics" },
            h(TrackSketch, { points: route.preview }),
            route.profile.length > 1 && h(ElevationProfile, { profile: route.profile })),
        me?.id === route.owner_id && h(Publish, { route: route }),
        canWeather && route.preview.length > 0 && h(WeatherToggle, { lat: route.preview[0][0], lon: route.preview[0][1], elevation: route.max_ele }),
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
export const markKinds = { spring: "Родник", camp: "Стоянка", ford: "Брод", danger: "Опасно", view: "Вид", other: "Другое" };
export function RouteMap({ route, printMode = false, onPick, picked }) {
    const box = useRef(null);
    const [full, setFull] = useState(false);
    const [detail, setDetail] = useState(route.detail ? route : null);
    const [error, setError] = useState(null);
    const mapRef = useRef(null);
    useEffect(() => {
        if (route.detail)
            setDetail(route);
        else
            get(`/routes/${route.id}`).then(setDetail, setError);
    }, [route]);
    const pickRef = useRef(onPick);
    pickRef.current = onPick;
    const pickLayer = useRef(null);
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
            const offset = detail.waypoints?.length ?? 0;
            (detail.marks ?? []).forEach((mk, i) => {
                const p = L.circleMarker([mk.lat, mk.lon], { className: `map-mark mark-${mk.kind}`, radius: 7, weight: 2, fillOpacity: 1 }).addTo(m);
                if (printMode)
                    p.bindTooltip(label(String(offset + i + 1)), { direction: "top", permanent: true });
                else
                    p.bindTooltip(label(`${markKinds[mk.kind]}: ${mk.title}`), { direction: "top" });
            });
            pickLayer.current = L.layerGroup().addTo(m);
            m.on("click", (ev) => pickRef.current?.(ev.latlng.lat, ev.latlng.lng));
            const first = segs[0][0], lastSeg = segs[segs.length - 1], last = lastSeg[lastSeg.length - 1];
            if (first)
                L.circleMarker(first, { className: "map-start", radius: 7, weight: 3, fillOpacity: 1 }).addTo(m).bindTooltip(label("Старт"));
            if (last)
                L.circleMarker(last, { className: "map-finish", radius: 7, weight: 3, fillOpacity: 1 }).addTo(m).bindTooltip(label("Финиш"));
            const [s, w, n, e] = detail.bbox;
            m.fitBounds([[s, w], [n, e]], { padding: [24, 24], maxZoom: 15 });
            mapRef.current = m;
        }, setError);
        return () => { alive = false; m?.remove(); mapRef.current = null; pickLayer.current = null; };
    }, [detail]);
    // точка, выбранная щелчком для новой отметки
    useEffect(() => {
        const g = pickLayer.current;
        if (!g)
            return;
        g.clearLayers();
        if (picked)
            L.circleMarker(picked, { className: "map-picked", radius: 8, weight: 3, fillOpacity: .4 }).addTo(g);
    }, [picked, detail]);
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
// MapPanel — карта и точки от ходивших (родники, стоянки, броды…).
function MapPanel({ route }) {
    const [detail, setDetail] = useState(null);
    const [error, setError] = useState(null);
    const [adding, setAdding] = useState(false);
    const [picked, setPicked] = useState(null);
    const load = () => get(`/routes/${route.id}`).then(setDetail, setError);
    useEffect(() => { load(); }, [route.id]);
    if (error)
        return h(ErrorLine, { error: error });
    if (!detail)
        return h(Loading, null);
    return (h("div", { class: "map-panel" },
        h(NoticeBanner, { notices: detail.notices }),
        h(RouteMap, { route: detail, onPick: adding ? (lat, lon) => setPicked([lat, lon]) : undefined, picked: adding ? picked : null }),
        h(Marks, { route: detail, onChange: load }),
        detail.public && h(RouteReviews, { routeId: detail.id, ownerId: detail.owner_id }),
        !adding ? h("button", { class: "ghost", onClick: () => { setAdding(true); setPicked(null); } }, "\u041E\u0442\u043C\u0435\u0442\u0438\u0442\u044C \u0442\u043E\u0447\u043A\u0443")
            : h(AddMark, { route: detail, picked: picked, onDone: (ok) => { setAdding(false); setPicked(null); if (ok)
                    load(); } })));
}
const dayFmt = new Intl.DateTimeFormat("ru", { day: "numeric", month: "long", year: "numeric" });
function Marks({ route, onChange }) {
    const me = useMe()?.id;
    const [error, setError] = useState(null);
    const marks = route.marks ?? [];
    if (marks.length === 0)
        return null;
    const run = (fn) => { setError(null); fn().then(onChange, setError); };
    return (h("section", { class: "marks" },
        h("h3", null, "\u0422\u043E\u0447\u043A\u0438 \u043D\u0430 \u043C\u0430\u0440\u0448\u0440\u0443\u0442\u0435"),
        h("ul", null, marks.map((mk) => (h("li", { key: mk.id, class: `mark mark-${mk.kind}` },
            mk.thumb_url && h("a", { href: mk.photo_url, target: "_blank", rel: "noopener" },
                h("img", { src: mk.thumb_url, alt: "", width: 64, height: 64, loading: "lazy" })),
            h("div", { class: "grow" },
                h("span", { class: "mark-kind" }, markKinds[mk.kind]),
                " ",
                h("strong", null, mk.title),
                mk.note && h("p", null, mk.note),
                h("p", { class: "muted" },
                    "\u041F\u0440\u043E\u0432\u0435\u0440\u0435\u043D\u043E ",
                    dayFmt.format(new Date(mk.verified_on + "T12:00:00")),
                    mk.checks > 0 ? ` · подтвердили: ${mk.checks}` : "",
                    " \u00B7 \u043E\u0442\u043C\u0435\u0442\u0438\u043B(\u0430) ",
                    h("a", { href: `/u/${mk.author.username}` }, mk.author.first_name || mk.author.username)),
                h("div", { class: "row tight" },
                    h("button", { class: "link", disabled: mk.my_check && mk.verified_on === new Date().toISOString().slice(0, 10), onClick: () => run(() => post(`/routes/${route.id}/marks/${mk.id}/check`)) }, mk.my_check ? "Снова на месте" : "На месте"),
                    (mk.author.id === me || route.owner_id === me) && h("button", { class: "link", onClick: () => run(() => del(`/routes/${route.id}/marks/${mk.id}`)) }, "\u0423\u0434\u0430\u043B\u0438\u0442\u044C"),
                    mk.author.id !== me && h(ReportButton, { target: { type: "route_point", id: mk.id } }))))))),
        h(ErrorLine, { error: error })));
}
function AddMark({ route, picked, onDone }) {
    const [f, setF] = useState({ kind: "spring", title: "", note: "" });
    const [file, setFile] = useState(null);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState(null);
    async function submit(e) {
        e.preventDefault();
        if (!picked)
            return;
        setBusy(true);
        setError(null);
        try {
            const photo = file ? await upload("/media", file) : null;
            await post(`/routes/${route.id}/marks`, { ...f, lat: picked[0], lon: picked[1], photo_id: photo?.id ?? null });
            onDone(true);
        }
        catch (err) {
            setError(err);
        }
        finally {
            setBusy(false);
        }
    }
    return (h("form", { class: "stack add-mark", onSubmit: submit },
        h("p", { class: "hint" }, picked ? `Место: ${picked[0].toFixed(5)}, ${picked[1].toFixed(5)}` : "Щёлкните по карте рядом с треком, где находится точка"),
        h("div", { class: "row-2" },
            h("label", null,
                "\u0427\u0442\u043E \u044D\u0442\u043E",
                h("select", { value: f.kind, onChange: (e) => setF({ ...f, kind: e.target.value }) }, Object.keys(markKinds).map((k) => h("option", { key: k, value: k }, markKinds[k])))),
            h("label", null,
                "\u041D\u0430\u0437\u0432\u0430\u043D\u0438\u0435",
                h("input", { value: f.title, required: true, maxLength: 80, onInput: (e) => setF({ ...f, title: e.target.value }), placeholder: "\u0420\u043E\u0434\u043D\u0438\u043A \u0443 \u0442\u0440\u043E\u043F\u044B" }))),
        h("label", null,
            "\u0417\u0430\u043C\u0435\u0442\u043A\u0430",
            h("textarea", { rows: 2, maxLength: 1000, value: f.note, onInput: (e) => setF({ ...f, note: e.target.value }), placeholder: "\u0422\u0435\u0447\u0451\u0442 \u043B\u0438 \u0432 \u0430\u0432\u0433\u0443\u0441\u0442\u0435, \u0433\u0434\u0435 \u043B\u0443\u0447\u0448\u0435 \u0441\u0442\u0430\u0432\u0438\u0442\u044C \u043F\u0430\u043B\u0430\u0442\u043A\u0438" })),
        h("label", null,
            "\u0424\u043E\u0442\u043E",
            h("input", { type: "file", accept: "image/jpeg,image/png", onChange: (e) => setFile(e.target.files?.[0] ?? null) })),
        h(ErrorLine, { error: error }),
        h("div", { class: "row" },
            h("button", { class: "primary", disabled: !picked || busy }, "\u0421\u043E\u0445\u0440\u0430\u043D\u0438\u0442\u044C \u0442\u043E\u0447\u043A\u0443"),
            h("button", { type: "button", class: "ghost", onClick: () => onDone(false) }, "\u041E\u0442\u043C\u0435\u043D\u0430"))));
}
// Publish — владелец открывает маршрут в каталоге /trails (без входа).
function Publish({ route }) {
    const [pub, setPub] = useState(route.public);
    const [note, setNote] = useState(route.public_note);
    const [open, setOpen] = useState(false);
    const [error, setError] = useState(null);
    const save = async (next) => {
        setError(null);
        try {
            const r = await patch(`/routes/${route.id}`, { public: next, public_note: note });
            setPub(r.public);
            setOpen(false);
        }
        catch (e) {
            setError(e);
        }
    };
    return (h("div", { class: "publish" },
        pub ? h("p", { class: "muted" },
            "\u0412 \u043E\u0442\u043A\u0440\u044B\u0442\u043E\u043C \u043A\u0430\u0442\u0430\u043B\u043E\u0433\u0435: ",
            h("a", { href: `/trails/${route.id}` }, "\u0441\u0442\u0440\u0430\u043D\u0438\u0446\u0430 \u043C\u0430\u0440\u0448\u0440\u0443\u0442\u0430"),
            " \u00B7 ",
            h("button", { class: "link", onClick: () => save(false) }, "\u0423\u0431\u0440\u0430\u0442\u044C \u0438\u0437 \u043A\u0430\u0442\u0430\u043B\u043E\u0433\u0430"))
            : !open ? h("button", { class: "link", onClick: () => setOpen(true) }, "\u041E\u0442\u043A\u0440\u044B\u0442\u044C \u0432 \u043A\u0430\u0442\u0430\u043B\u043E\u0433\u0435 \u043C\u0430\u0440\u0448\u0440\u0443\u0442\u043E\u0432\u2026") : (h("div", { class: "stack" },
                h("p", { class: "hint" },
                    "\u0422\u0440\u0435\u043A, \u0442\u043E\u0447\u043A\u0438 \u0438 \u043E\u043F\u0438\u0441\u0430\u043D\u0438\u0435 \u0443\u0432\u0438\u0434\u044F\u0442 \u0432\u0441\u0435, \u0432 \u0442\u043E\u043C \u0447\u0438\u0441\u043B\u0435 \u0431\u0435\u0437 \u0432\u0445\u043E\u0434\u0430 \u0438 \u043F\u043E\u0438\u0441\u043A\u043E\u0432\u0438\u043A\u0438. \u041D\u0430\u0447\u0430\u043B\u043E \u0438 \u043A\u043E\u043D\u0435\u0446 \u043E\u0441\u0442\u0430\u044E\u0442\u0441\u044F \u0441\u043A\u0440\u044B\u0442\u044B\u043C\u0438 (",
                    route.hidden_ends_m,
                    " \u043C)."),
                h("label", null,
                    "\u041E\u043F\u0438\u0441\u0430\u043D\u0438\u0435 \u0434\u043B\u044F \u043A\u0430\u0442\u0430\u043B\u043E\u0433\u0430",
                    h("textarea", { rows: 3, maxLength: 2000, value: note, onInput: (e) => setNote(e.target.value), placeholder: "\u041E\u0442\u043A\u0443\u0434\u0430 \u0441\u0442\u0430\u0440\u0442\u043E\u0432\u0430\u0442\u044C, \u0433\u0434\u0435 \u0432\u043E\u0434\u0430, \u043A\u043E\u0433\u0434\u0430 \u043B\u0443\u0447\u0448\u0435 \u0438\u0434\u0442\u0438" })),
                h("div", { class: "row" },
                    h("button", { class: "primary", onClick: () => save(true) }, "\u041E\u0442\u043A\u0440\u044B\u0442\u044C \u0434\u043B\u044F \u0432\u0441\u0435\u0445"),
                    h("button", { class: "ghost", onClick: () => setOpen(false) }, "\u041E\u0442\u043C\u0435\u043D\u0430")))),
        h(ErrorLine, { error: error })));
}
