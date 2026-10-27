// Витрина для гостей под формой входа: ближайшие походы и маршруты
// каталога (GET /showcase, без входа).
import { h, Fragment } from "../vendor/preact/index.js";
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { fullName } from "./api.js";
const dateFmt = new Intl.DateTimeFormat("ru-RU", { day: "numeric", month: "long", weekday: "short" });
const levels = ["", "Лёгкий", "Несложный", "Средний", "Трудный", "Очень трудный"];
export function Showcase() {
    const [d, setD] = useState(null);
    useEffect(() => { fetch("/api/v1/showcase").then((r) => (r.ok ? r.json() : null)).then(setD, () => { }); }, []);
    return (h("section", { class: "showcase", "aria-label": "\u0427\u0442\u043E \u0435\u0441\u0442\u044C \u043D\u0430 \u0441\u0430\u0439\u0442\u0435" },
        h("ul", { class: "showcase-points" },
            h("li", null,
                h("strong", null, "\u041F\u043E\u0445\u043E\u0434\u044B"),
                " \u0441 \u043C\u0435\u0441\u0442\u0430\u043C\u0438, \u0437\u0430\u044F\u0432\u043A\u0430\u043C\u0438 \u0438 \u043B\u0438\u0441\u0442\u043E\u043C \u043E\u0436\u0438\u0434\u0430\u043D\u0438\u044F"),
            h("li", null,
                h("strong", null, "\u041F\u043E\u043F\u0443\u0442\u0447\u0438\u043A\u0438 \u0438 \u043F\u043E\u0434\u0432\u043E\u0437"),
                " \u0434\u043E \u0422\u0430\u0433\u0430\u043D\u0430\u044F, \u0418\u0440\u0435\u043C\u0435\u043B\u044F, \u0417\u044E\u0440\u0430\u0442\u043A\u0443\u043B\u044F"),
            h("li", null,
                h("strong", null, "\u041C\u0430\u0440\u0448\u0440\u0443\u0442\u044B"),
                " \u0441 \u043F\u0440\u043E\u0444\u0438\u043B\u0435\u043C \u0432\u044B\u0441\u043E\u0442, \u0440\u043E\u0434\u043D\u0438\u043A\u0430\u043C\u0438 \u0438 \u0441\u0442\u043E\u044F\u043D\u043A\u0430\u043C\u0438"),
            h("li", null,
                h("strong", null, "\u041A\u043E\u043D\u0442\u0440\u043E\u043B\u044C\u043D\u044B\u0439 \u0441\u0440\u043E\u043A"),
                " \u2014 \u0435\u0441\u043B\u0438 \u0433\u0440\u0443\u043F\u043F\u0430 \u043D\u0435 \u0432\u0435\u0440\u043D\u0443\u043B\u0430\u0441\u044C, \u043F\u0438\u0441\u044C\u043C\u043E \u0434\u043E\u0432\u0435\u0440\u0435\u043D\u043D\u044B\u043C \u043B\u044E\u0434\u044F\u043C")),
        d && d.hikes.length > 0 && h(Fragment, null,
            h("h2", null, "\u0411\u043B\u0438\u0436\u0430\u0439\u0448\u0438\u0435 \u043F\u043E\u0445\u043E\u0434\u044B"),
            h("ul", { class: "showcase-hikes" }, d.hikes.map((x) => (h("li", { key: x.id },
                h("a", { href: `/events/${x.id}` }, x.title),
                h("span", { class: "muted" },
                    dateFmt.format(new Date(x.starts_at)),
                    x.place ? ` · ${x.place}` : ""),
                h("span", { class: "muted" },
                    x.difficulty ? `${levels[x.difficulty]} · ` : "",
                    "\u0438\u0434\u0443\u0442 ",
                    x.going_count,
                    x.capacity ? ` из ${x.capacity}` : "")))))),
        d && d.routes.length > 0 && h(Fragment, null,
            h("h2", null, "\u041C\u0430\u0440\u0448\u0440\u0443\u0442\u044B"),
            h("ul", { class: "plain-list" }, d.routes.map((r) => (h("li", { key: r.id },
                h("a", { href: r.url }, r.title),
                " ",
                h("span", { class: "muted" },
                    (r.distance_m / 1000).toFixed(1),
                    " \u043A\u043C, \u043D\u0430\u0431\u043E\u0440 ",
                    r.gain_m,
                    " \u043C \u00B7 ",
                    fullName(r.author))))))),
        h("p", null,
            h("a", { href: "/trails" }, "\u0412\u0435\u0441\u044C \u043A\u0430\u0442\u0430\u043B\u043E\u0433 \u043C\u0430\u0440\u0448\u0440\u0443\u0442\u043E\u0432 \u2192"))));
}
