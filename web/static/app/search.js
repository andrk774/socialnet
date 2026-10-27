// /search?q= — общий поиск: походы, маршруты каталога, статьи, люди,
// сообщества, музыка. «Всё» — по несколько из каждого раздела.
import { h, Fragment } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { fullName, get } from "./api.js";
import { query } from "./router.js";
import { Avatar, Empty, ErrorLine, Loading, useLoad } from "./ui.js";
const tabs = [["all", "Всё"], ["hikes", "Походы"], ["routes", "Маршруты"], ["articles", "Статьи"],
    ["users", "Люди"], ["communities", "Сообщества"], ["tracks", "Музыка"]];
const sections = ["hikes", "routes", "articles", "users", "communities"];
const dateFmt = new Intl.DateTimeFormat("ru-RU", { day: "numeric", month: "long", weekday: "short" });
const km = (m) => `${(m / 1000).toFixed(m < 10000 ? 1 : 0)} км`;
function Item({ kind, x }) {
    switch (kind) {
        case "hikes": return h("li", null,
            h("a", { href: `/events/${x.id}` }, x.title),
            " ",
            h("span", { class: "muted" },
                dateFmt.format(new Date(x.starts_at)),
                " \u00B7 \u0438\u0434\u0443\u0442 ",
                x.going_count,
                x.capacity ? ` из ${x.capacity}` : ""));
        case "routes": return h("li", null,
            h("a", { href: x.url, target: "_blank", rel: "noopener" }, x.title),
            " ",
            h("span", { class: "muted" },
                km(x.distance_m),
                ", \u043D\u0430\u0431\u043E\u0440 ",
                x.gain_m,
                " \u043C \u00B7 ",
                fullName(x.author)));
        case "articles": return h("li", null,
            h("a", { href: `/articles/${x.id}` }, x.title),
            " ",
            h("span", { class: "muted" }, fullName(x.author)));
        case "users": return h("li", null,
            h(Avatar, { user: x, size: 32 }),
            " ",
            h("a", { href: `/u/${x.username}` }, fullName(x)),
            " ",
            h("span", { class: "muted" },
                "@",
                x.username));
        case "communities": return h("li", null,
            h("a", { href: `/c/${x.slug}` }, x.name),
            " ",
            h("span", { class: "muted" },
                x.members_count,
                " \u0443\u0447."));
        default: return h("li", null,
            x.artist,
            " \u2014 ",
            x.title,
            " ",
            h("audio", { controls: true, preload: "none", src: x.url }));
    }
}
export function SearchPage() {
    const q = query("q") ?? "";
    const [type, setType] = useState("all");
    const res = useLoad(() => (q.trim().length < 2 ? Promise.resolve(null) :
        get(`/search?q=${encodeURIComponent(q)}&type=${type}`)), [q, type]);
    const d = res.data;
    return (h("section", { class: "search-page" },
        h("h1", null,
            "\u041F\u043E\u0438\u0441\u043A",
            q && `: ${q}`),
        h("div", { class: "tabs", role: "tablist" }, tabs.map(([k, label]) => h("button", { key: k, role: "tab", "aria-selected": type === k, class: type === k ? "tab on" : "tab", onClick: () => setType(k) }, label))),
        q.trim().length < 2 ? h(Empty, null, "\u0412\u0432\u0435\u0434\u0438\u0442\u0435 \u0432 \u0441\u0442\u0440\u043E\u043A\u0435 \u043F\u043E\u0438\u0441\u043A\u0430 \u0445\u043E\u0442\u044F \u0431\u044B \u0434\u0432\u0435 \u0431\u0443\u043A\u0432\u044B: \u043D\u0430\u0437\u0432\u0430\u043D\u0438\u0435 \u043F\u043E\u0445\u043E\u0434\u0430, \u0432\u0435\u0440\u0448\u0438\u043D\u0443, \u0440\u0430\u0439\u043E\u043D, \u0438\u043C\u044F.") :
            res.error ? h(ErrorLine, { error: res.error }) : !d ? h(Loading, null) : type !== "all" ? (d.items.length === 0 ? h(Empty, null, "\u041D\u0438\u0447\u0435\u0433\u043E \u043D\u0435 \u043D\u0430\u0448\u043B\u043E\u0441\u044C. \u041F\u043E\u043F\u0440\u043E\u0431\u0443\u0439\u0442\u0435 \u0434\u0440\u0443\u0433\u043E\u0435 \u043D\u0430\u043F\u0438\u0441\u0430\u043D\u0438\u0435.") :
                h("ul", { class: "plain-list" }, d.items.map((x) => h(Item, { key: x.id, kind: type, x: x })))) : sections.every((k) => !d[k]?.length) ? h(Empty, null, "\u041D\u0438\u0447\u0435\u0433\u043E \u043D\u0435 \u043D\u0430\u0448\u043B\u043E\u0441\u044C. \u041F\u043E\u043F\u0440\u043E\u0431\u0443\u0439\u0442\u0435 \u0434\u0440\u0443\u0433\u043E\u0435 \u043D\u0430\u043F\u0438\u0441\u0430\u043D\u0438\u0435.") : (h(Fragment, null, sections.filter((k) => d[k]?.length).map((k) => (h("section", { key: k, class: "search-section", "data-kind": k },
                h("h2", null, tabs.find(([t]) => t === k)[1]),
                h("ul", { class: "plain-list" }, d[k].map((x) => h(Item, { key: x.id, kind: k, x: x }))),
                d[k].length >= 5 && h("button", { class: "link", onClick: () => setType(k) }, "\u041F\u043E\u043A\u0430\u0437\u0430\u0442\u044C \u0432\u0441\u0435"))))))));
}
