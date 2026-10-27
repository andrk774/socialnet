// Прогноз на маршруте: на высоте вершины, с предупреждениями о грозе,
// сильном ветре и осадках.
import { h } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { get } from "./api.js";
import { ErrorLine, Loading, useLoad } from "./ui.js";
// WMO-коды погоды → короткая подпись и значок
function sky(code) {
    if (code === 0)
        return ["☀️", "Ясно"];
    if (code <= 2)
        return ["🌤️", "Переменная облачность"];
    if (code === 3)
        return ["☁️", "Пасмурно"];
    if (code === 45 || code === 48)
        return ["🌫️", "Туман"];
    if (code >= 51 && code <= 57)
        return ["🌦️", "Морось"];
    if (code >= 61 && code <= 67)
        return ["🌧️", "Дождь"];
    if (code >= 71 && code <= 77)
        return ["🌨️", "Снег"];
    if (code >= 80 && code <= 82)
        return ["🌧️", "Ливень"];
    if (code === 85 || code === 86)
        return ["🌨️", "Снегопад"];
    if (code >= 95)
        return ["⛈️", "Гроза"];
    return ["·", "—"];
}
export function warnings(d) {
    const w = [];
    if (d.code >= 95)
        w.push("гроза — уходите с гребней");
    if ((d.gust_max ?? 0) >= 17)
        w.push(`порывы до ${Math.round(d.gust_max)} м/с`);
    if ((d.precip_mm ?? 0) >= 15)
        w.push(`осадки ${Math.round(d.precip_mm)} мм — броды поднимутся`);
    if ((d.t_min ?? 99) <= -15)
        w.push(`мороз до ${Math.round(d.t_min)}°`);
    return w;
}
const dayFmt = new Intl.DateTimeFormat("ru", { weekday: "short", day: "numeric", month: "short" });
const t = (v) => (v === null ? "—" : `${v > 0 ? "+" : ""}${Math.round(v)}°`);
export function Weather({ lat, lon, elevation, from, to }) {
    const q = `/weather?lat=${lat.toFixed(4)}&lon=${lon.toFixed(4)}${elevation != null ? `&elevation=${Math.round(elevation)}` : ""}`;
    const rep = useLoad(() => get(q), [q]);
    if (rep.error)
        return h(ErrorLine, { error: rep.error });
    if (!rep.data)
        return h(Loading, null);
    const r = rep.data;
    return (h("section", { class: "weather", "aria-label": "\u041F\u0440\u043E\u0433\u043D\u043E\u0437 \u043F\u043E\u0433\u043E\u0434\u044B" },
        h("p", { class: "muted" },
            "\u041F\u0440\u043E\u0433\u043D\u043E\u0437",
            r.elevation != null ? ` на высоте ${Math.round(r.elevation)} м` : "",
            ". \u0414\u0430\u043D\u043D\u044B\u0435: ",
            r.source,
            ". \u0412 \u0433\u043E\u0440\u0430\u0445 \u043F\u043E\u0433\u043E\u0434\u0430 \u043C\u0435\u043D\u044F\u0435\u0442\u0441\u044F \u0431\u044B\u0441\u0442\u0440\u0435\u0435 \u043F\u0440\u043E\u0433\u043D\u043E\u0437\u0430."),
        h("ol", { class: "weather-days" }, r.days.map((d) => {
            const [icon, text] = sky(d.code), warn = warnings(d), inHike = from && d.date >= from && d.date <= (to ?? from);
            return (h("li", { key: d.date, class: `${inHike ? "hike-day" : ""} ${warn.length ? "warn" : ""}` },
                h("span", { class: "wd-date" }, dayFmt.format(new Date(d.date + "T12:00:00"))),
                h("span", { class: "wd-icon", title: text, "aria-label": text }, icon),
                h("span", { class: "wd-temp" },
                    h("strong", null, t(d.t_max)),
                    " / ",
                    t(d.t_min)),
                h("span", { class: "wd-more" },
                    d.precip_mm ? `${d.precip_mm.toFixed(1)} мм` : "без осадков",
                    d.precip_prob != null ? ` · ${Math.round(d.precip_prob)}%` : ""),
                h("span", { class: "wd-more" },
                    "\u0432\u0435\u0442\u0435\u0440 ",
                    d.wind_max != null ? Math.round(d.wind_max) : "—",
                    d.gust_max != null ? `, порывы ${Math.round(d.gust_max)}` : "",
                    " \u043C/\u0441"),
                warn.map((x) => h("span", { key: x, class: "wd-warn" },
                    "\u26A0 ",
                    x))));
        }))));
}
// WeatherToggle — кнопка «Погода» для карточки маршрута.
export function WeatherToggle(props) {
    const [open, setOpen] = useState(false);
    return (h("div", { class: "weather-toggle" },
        h("button", { class: "link", "aria-expanded": open, onClick: () => setOpen(!open) }, open ? "Скрыть погоду" : "Погода на маршруте"),
        open && h(Weather, { ...props })));
}
