// Лист маршрута для печати (и сохранения в PDF через «Печать» браузера):
// карта, профиль высот, точки с координатами, сведения о походе, снаряжение
// списком с квадратиками, телефоны экстренных служб.
import { Fragment, h } from "../vendor/preact/index.js";
import { get } from "./api.js";
import { difficulties } from "./hikes.js";
import { ElevationProfile, hours, km, markKinds, RouteMap } from "./routes.js";
import { ErrorLine, Loading, useLoad } from "./ui.js";
const dateFmt = new Intl.DateTimeFormat("ru", { weekday: "long", day: "numeric", month: "long", year: "numeric", hour: "2-digit", minute: "2-digit" });
const nf0 = new Intl.NumberFormat("ru", { maximumFractionDigits: 0 });
function dms(v, pos, neg) {
    const a = Math.abs(v), d = Math.floor(a), mf = (a - d) * 60, m = Math.floor(mf), s = (mf - m) * 60;
    return `${d}°${String(m).padStart(2, "0")}′${s.toFixed(1).padStart(4, "0")}″ ${v >= 0 ? pos : neg}`;
}
export const coords = (lat, lon) => `${dms(lat, "с. ш.", "ю. ш.")}, ${dms(lon, "в. д.", "з. д.")}`;
function dist(a, b) {
    const R = 6371000, rad = Math.PI / 180;
    const dLat = (b[0] - a[0]) * rad, dLon = (b[1] - a[1]) * rad;
    const x = Math.sin(dLat / 2) ** 2 + Math.cos(a[0] * rad) * Math.cos(b[0] * rad) * Math.sin(dLon / 2) ** 2;
    return 2 * R * Math.asin(Math.sqrt(x));
}
// alongTrack — сколько пройти от старта до ближайшей к точке вершины трека.
export function alongTrack(segs, p) {
    let best = Infinity, at = 0, run = 0;
    for (const seg of segs) {
        for (let i = 0; i < seg.length; i++) {
            if (i > 0)
                run += dist(seg[i - 1], seg[i]);
            const d = dist(seg[i], p);
            if (d < best) {
                best = d;
                at = run;
            }
        }
    }
    return at;
}
export function PrintPage({ routeId, eventId }) {
    const ev = useLoad(() => (eventId ? get(`/events/${eventId}`) : Promise.resolve(null)), [eventId]);
    const rid = routeId ?? ev.data?.route_id ?? undefined;
    const route = useLoad(() => (rid ? get(`/routes/${rid}`) : Promise.resolve(null)), [rid]);
    if (ev.error || route.error)
        return h(ErrorLine, { error: ev.error || route.error });
    if ((eventId && !ev.data) || (rid && !route.data))
        return h(Loading, null);
    const e = ev.data, r = route.data;
    const segs = r ? (r.detail?.length ? r.detail : [r.preview]) : [];
    const start = segs[0]?.[0], lastSeg = segs[segs.length - 1], finish = lastSeg?.[lastSeg.length - 1];
    const gear = (e?.gear ?? "").split("\n").map((g) => g.trim()).filter(Boolean);
    return (h("article", { class: "print-sheet" },
        h("div", { class: "print-actions no-print" },
            h("button", { class: "primary", onClick: () => window.print() }, "\u0420\u0430\u0441\u043F\u0435\u0447\u0430\u0442\u0430\u0442\u044C \u0438\u043B\u0438 \u0441\u043E\u0445\u0440\u0430\u043D\u0438\u0442\u044C \u0432 PDF"),
            h("a", { href: e ? `/events/${e.id}` : "/" }, "\u2190 \u041D\u0430\u0437\u0430\u0434")),
        h("header", null,
            h("h1", null, e?.title ?? r?.title),
            e && h("p", { class: "print-when" },
                dateFmt.format(new Date(e.starts_at)),
                e.ends_at && h(Fragment, null,
                    " \u2014 ",
                    dateFmt.format(new Date(e.ends_at))))),
        r && h(Fragment, null,
            h("dl", { class: "facts print-facts" },
                h("div", null,
                    h("dt", null, "\u041C\u0430\u0440\u0448\u0440\u0443\u0442"),
                    h("dd", null, r.title)),
                h("div", null,
                    h("dt", null, "\u0420\u0430\u0441\u0441\u0442\u043E\u044F\u043D\u0438\u0435"),
                    h("dd", null, km(r.distance_m))),
                r.max_ele !== null && h("div", null,
                    h("dt", null, "\u041D\u0430\u0431\u043E\u0440 / \u0441\u0431\u0440\u043E\u0441"),
                    h("dd", null,
                        "\u2191 ",
                        nf0.format(r.gain_m),
                        " \u043C / \u2193 ",
                        nf0.format(r.loss_m),
                        " \u043C")),
                r.max_ele !== null && h("div", null,
                    h("dt", null, "\u0412\u044B\u0441\u043E\u0442\u044B"),
                    h("dd", null,
                        nf0.format(r.min_ele ?? 0),
                        "\u2013",
                        nf0.format(r.max_ele),
                        " \u043C")),
                r.moving_sec > 0 && h("div", null,
                    h("dt", null, "\u0412 \u0434\u0432\u0438\u0436\u0435\u043D\u0438\u0438 (\u043F\u043E \u0442\u0440\u0435\u043A\u0443)"),
                    h("dd", null, hours(r.moving_sec))),
                e?.difficulty && h("div", null,
                    h("dt", null, "\u0421\u043B\u043E\u0436\u043D\u043E\u0441\u0442\u044C"),
                    h("dd", null, difficulties[e.difficulty]))),
            h(RouteMap, { route: r, printMode: true }),
            r.profile.length > 1 && h("div", { class: "print-profile" },
                h(ElevationProfile, { profile: r.profile })),
            h("h2", null, "\u0422\u043E\u0447\u043A\u0438"),
            h("table", { class: "print-table" },
                h("thead", null,
                    h("tr", null,
                        h("th", null, "\u2116"),
                        h("th", null, "\u0422\u043E\u0447\u043A\u0430"),
                        h("th", null, "\u041A\u043E\u043E\u0440\u0434\u0438\u043D\u0430\u0442\u044B"),
                        h("th", null, "\u0412\u044B\u0441\u043E\u0442\u0430"),
                        h("th", null, "\u041E\u0442 \u0441\u0442\u0430\u0440\u0442\u0430"))),
                h("tbody", null,
                    start && h("tr", null,
                        h("td", null, "\u0421"),
                        h("td", null,
                            "\u0421\u0442\u0430\u0440\u0442",
                            r.hidden_ends_m > 0 ? ` (≈${r.hidden_ends_m} м скрыты)` : ""),
                        h("td", null, coords(start[0], start[1])),
                        h("td", null, "\u2014"),
                        h("td", null, "0")),
                    (r.waypoints ?? []).map((w, i) => (h("tr", { key: i },
                        h("td", null, i + 1),
                        h("td", null, w.name || "—"),
                        h("td", null,
                            coords(w.lat, w.lon),
                            h("br", null),
                            h("span", { class: "muted" },
                                w.lat.toFixed(5),
                                ", ",
                                w.lon.toFixed(5))),
                        h("td", null, w.ele !== undefined && w.ele !== null ? `${nf0.format(w.ele)} м` : "—"),
                        h("td", null, km(alongTrack(segs, [w.lat, w.lon])))))),
                    (r.marks ?? []).map((mk, i) => (h("tr", { key: "m" + mk.id },
                        h("td", null, (r.waypoints?.length ?? 0) + i + 1),
                        h("td", null,
                            h("strong", null,
                                markKinds[mk.kind],
                                ":"),
                            " ",
                            mk.title,
                            mk.note && h(Fragment, null,
                                h("br", null),
                                h("span", { class: "muted" }, mk.note)),
                            h("br", null),
                            h("span", { class: "muted" },
                                "\u043F\u0440\u043E\u0432\u0435\u0440\u0435\u043D\u043E ",
                                mk.verified_on)),
                        h("td", null,
                            coords(mk.lat, mk.lon),
                            h("br", null),
                            h("span", { class: "muted" },
                                mk.lat.toFixed(5),
                                ", ",
                                mk.lon.toFixed(5))),
                        h("td", null, "\u2014"),
                        h("td", null, km(alongTrack(segs, [mk.lat, mk.lon])))))),
                    finish && h("tr", null,
                        h("td", null, "\u0424"),
                        h("td", null, "\u0424\u0438\u043D\u0438\u0448"),
                        h("td", null, coords(finish[0], finish[1])),
                        h("td", null, "\u2014"),
                        h("td", null, km(r.distance_m)))))),
        e && h(Fragment, null,
            h("h2", null, "\u041F\u043E\u0445\u043E\u0434"),
            h("dl", { class: "facts print-facts" },
                e.meeting_point && h("div", null,
                    h("dt", null, "\u041C\u0435\u0441\u0442\u043E \u0441\u0431\u043E\u0440\u0430"),
                    h("dd", null, e.meeting_point)),
                e.place && h("div", null,
                    h("dt", null, "\u0413\u0434\u0435"),
                    h("dd", null, e.place)),
                h("div", null,
                    h("dt", null, "\u0423\u0447\u0430\u0441\u0442\u043D\u0438\u043A\u043E\u0432"),
                    h("dd", null,
                        e.going_count,
                        e.capacity !== null ? ` из ${e.capacity}` : "")),
                e.control_at && h("div", null,
                    h("dt", null, "\u041A\u043E\u043D\u0442\u0440\u043E\u043B\u044C\u043D\u044B\u0439 \u0441\u0440\u043E\u043A"),
                    h("dd", null, dateFmt.format(new Date(e.control_at))))),
            gear.length > 0 && h(Fragment, null,
                h("h2", null, "\u0421\u043D\u0430\u0440\u044F\u0436\u0435\u043D\u0438\u0435"),
                h("ul", { class: "checklist" }, gear.map((g, i) => h("li", { key: i },
                    h("span", { class: "box", "aria-hidden": "true" }),
                    " ",
                    g))))),
        h("section", { class: "print-emergency" },
            h("h2", null, "\u042D\u043A\u0441\u0442\u0440\u0435\u043D\u043D\u0430\u044F \u0441\u0432\u044F\u0437\u044C"),
            h("p", null,
                h("strong", null, "112"),
                " \u2014 \u0435\u0434\u0438\u043D\u044B\u0439 \u043D\u043E\u043C\u0435\u0440 \u044D\u043A\u0441\u0442\u0440\u0435\u043D\u043D\u044B\u0445 \u0441\u043B\u0443\u0436\u0431 (\u0440\u0430\u0431\u043E\u0442\u0430\u0435\u0442 \u0431\u0435\u0437 SIM-\u043A\u0430\u0440\u0442\u044B \u0438 \u043F\u0440\u0438 \u043D\u0443\u043B\u0435 \u043D\u0430 \u0441\u0447\u0451\u0442\u0435, \u0435\u0441\u043B\u0438 \u0435\u0441\u0442\u044C \u0441\u0435\u0442\u044C \u043B\u044E\u0431\u043E\u0433\u043E \u043E\u043F\u0435\u0440\u0430\u0442\u043E\u0440\u0430)."),
            h("p", null, "\u0421\u043E\u043E\u0431\u0449\u0438\u0442\u0435: \u043A\u0442\u043E \u0432\u044B \u0438 \u0441\u043A\u043E\u043B\u044C\u043A\u043E \u0432\u0430\u0441, \u0447\u0442\u043E \u0441\u043B\u0443\u0447\u0438\u043B\u043E\u0441\u044C, \u0433\u0434\u0435 \u043D\u0430\u0445\u043E\u0434\u0438\u0442\u0435\u0441\u044C (\u043A\u043E\u043E\u0440\u0434\u0438\u043D\u0430\u0442\u044B \u0438\u0437 \u0442\u0430\u0431\u043B\u0438\u0446\u044B \u0438\u043B\u0438 \u0441 \u0442\u0435\u043B\u0435\u0444\u043E\u043D\u0430), \u043D\u043E\u043C\u0435\u0440 \u0434\u043B\u044F \u043E\u0431\u0440\u0430\u0442\u043D\u043E\u0439 \u0441\u0432\u044F\u0437\u0438. \u0411\u0435\u0440\u0435\u0433\u0438\u0442\u0435 \u0437\u0430\u0440\u044F\u0434: \u0434\u0435\u0440\u0436\u0438\u0442\u0435 \u0442\u0435\u043B\u0435\u0444\u043E\u043D \u0432 \u0442\u0435\u043F\u043B\u0435, \u0432\u044B\u043A\u043B\u044E\u0447\u0430\u0439\u0442\u0435 \u043B\u0438\u0448\u043D\u0435\u0435."),
            h("p", { class: "muted" }, "\u041F\u0435\u0440\u0435\u0434 \u0432\u044B\u0445\u043E\u0434\u043E\u043C \u043E\u0441\u0442\u0430\u0432\u044C\u0442\u0435 \u044D\u0442\u043E\u0442 \u043B\u0438\u0441\u0442 \u0438 \u043A\u043E\u043D\u0442\u0440\u043E\u043B\u044C\u043D\u044B\u0439 \u0441\u0440\u043E\u043A \u0431\u043B\u0438\u0437\u043A\u0438\u043C. \u0421\u0432\u0435\u0434\u0435\u043D\u0438\u044F \u043E \u043C\u0430\u0440\u0448\u0440\u0443\u0442\u0435 \u043E\u043F\u0443\u0431\u043B\u0438\u043A\u043E\u0432\u0430\u043D\u044B \u043F\u043E\u043B\u044C\u0437\u043E\u0432\u0430\u0442\u0435\u043B\u044F\u043C\u0438 socialnet \u0438 \u043C\u043E\u0433\u0443\u0442 \u0431\u044B\u0442\u044C \u043D\u0435\u0442\u043E\u0447\u043D\u044B\u043C\u0438."))));
}
