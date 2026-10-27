// «Ищу попутчиков»: поиск объявлений по району, датам и сложности, отклик
// личным сообщением, свои объявления.
import { Fragment, h } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { del, get, patch, post } from "./api.js";
import { difficulties } from "./hikes.js";
import { useMe } from "./session.js";
import { Avatar, Empty, ErrorLine, Loading, UserLink, useLoad } from "./ui.js";
import { ReportButton } from "./report.js";
const dayFmt = new Intl.DateTimeFormat("ru", { day: "numeric", month: "long", weekday: "short" });
const day = (d) => dayFmt.format(new Date(d + "T12:00:00"));
const dates = (a) => (a.date_from === a.date_to ? day(a.date_from) : `${day(a.date_from)} — ${day(a.date_to)}`);
const today = () => new Date(Date.now() - new Date().getTimezoneOffset() * 60000).toISOString().slice(0, 10);
export function CompanionsPage() {
    const me = useMe();
    const [filter, setFilter] = useState({ region: "", from: "", to: "", max_difficulty: "" });
    const [applied, setApplied] = useState(filter);
    const qs = new URLSearchParams(Object.entries(applied).filter(([, v]) => v)).toString();
    const list = useLoad(() => get(`/companions${qs ? "?" + qs : ""}`), [qs]);
    const mine = useLoad(() => get("/companions/mine"), []);
    const setF = (k) => (e) => setFilter({ ...filter, [k]: e.target.value });
    const reloadAll = () => { list.reload(); mine.reload(); };
    return (h("section", null,
        h("h1", null, "\u0418\u0449\u0443 \u043F\u043E\u043F\u0443\u0442\u0447\u0438\u043A\u043E\u0432"),
        h("form", { class: "filters", onSubmit: (e) => { e.preventDefault(); setApplied(filter); } },
            h("label", null,
                "\u0420\u0430\u0439\u043E\u043D",
                h("input", { value: filter.region, maxLength: 80, placeholder: "\u0422\u0430\u0433\u0430\u043D\u0430\u0439, \u0418\u0440\u0435\u043C\u0435\u043B\u044C\u2026", onInput: setF("region") })),
            h("label", null,
                "\u0421",
                h("input", { type: "date", value: filter.from, onInput: setF("from") })),
            h("label", null,
                "\u041F\u043E",
                h("input", { type: "date", value: filter.to, onInput: setF("to") })),
            h("label", null,
                "\u0421\u043B\u043E\u0436\u043D\u043E\u0441\u0442\u044C \u0434\u043E",
                h("select", { value: filter.max_difficulty, onChange: setF("max_difficulty") },
                    h("option", { value: "" }, "\u043B\u044E\u0431\u0430\u044F"),
                    [1, 2, 3, 4, 5].map((d) => h("option", { key: d, value: String(d) }, difficulties[d])))),
            h("button", { class: "primary" }, "\u041D\u0430\u0439\u0442\u0438")),
        list.error ? h(ErrorLine, { error: list.error }) : !list.data ? h(Loading, null) : list.data.items.length === 0 ? h(Empty, null, "\u041E\u0431\u044A\u044F\u0432\u043B\u0435\u043D\u0438\u0439 \u043D\u0435 \u043D\u0430\u0448\u043B\u043E\u0441\u044C. \u0420\u0430\u0437\u043C\u0435\u0441\u0442\u0438\u0442\u0435 \u0441\u0432\u043E\u0451 \u2014 \u0432\u0430\u0441 \u043D\u0430\u0439\u0434\u0443\u0442.") : (h("ul", { class: "ads" }, list.data.items.map((a) => h(AdCard, { key: a.id, ad: a, mine: a.author.id === me?.id, onChange: reloadAll })))),
        h(NewAd, { onCreated: reloadAll }),
        mine.data && mine.data.items.length > 0 && h(Fragment, null,
            h("h2", null, "\u041C\u043E\u0438 \u043E\u0431\u044A\u044F\u0432\u043B\u0435\u043D\u0438\u044F"),
            h("ul", { class: "ads" }, mine.data.items.map((a) => h(AdCard, { key: a.id, ad: a, mine: true, onChange: reloadAll }))))));
}
function AdCard({ ad, mine, onChange }) {
    const [open, setOpen] = useState(false);
    const [text, setText] = useState("");
    const [sent, setSent] = useState(false);
    const [error, setError] = useState(null);
    const run = async (fn) => { setError(null); try {
        await fn();
        onChange();
    }
    catch (e) {
        setError(e);
    } };
    async function reply(e) {
        e.preventDefault();
        setError(null);
        try {
            await post(`/companions/${ad.id}/reply`, { text });
            setSent(true);
            setOpen(false);
            setText("");
        }
        catch (err) {
            setError(err);
        }
    }
    return (h("li", { class: ad.closed ? "ad closed" : "ad" },
        h("div", { class: "ad-head" },
            h(Avatar, { user: ad.author, size: 36 }),
            h("div", { class: "grow" },
                h("strong", { class: "ad-title" }, ad.title),
                h("div", { class: "hike-meta" },
                    h("span", null, ad.region),
                    h("span", null, dates(ad)),
                    ad.difficulty && h("span", { class: `difficulty d${ad.difficulty}` }, difficulties[ad.difficulty]),
                    h("span", null,
                        "\u043D\u0443\u0436\u043D\u043E: ",
                        ad.seats),
                    ad.closed && h("span", { class: "pill" }, "\u0437\u0430\u043A\u0440\u044B\u0442\u043E")))),
        ad.body && h("p", { class: "ad-body" }, ad.body),
        h("div", { class: "row tight" },
            !mine && h("span", { class: "muted" },
                "\u0410\u0432\u0442\u043E\u0440: ",
                h(UserLink, { user: ad.author })),
            !mine && !ad.closed && !sent && h("button", { class: "ghost", onClick: () => setOpen(!open), "aria-expanded": open }, "\u041E\u0442\u043A\u043B\u0438\u043A\u043D\u0443\u0442\u044C\u0441\u044F"),
            sent && h("span", { class: "muted" },
                "\u041E\u0442\u043A\u043B\u0438\u043A \u043E\u0442\u043F\u0440\u0430\u0432\u043B\u0435\u043D \u2014 \u043F\u0435\u0440\u0435\u043F\u0438\u0441\u043A\u0430 \u0432 ",
                h("a", { href: `/im/u/${ad.author.id}` }, "\u0421\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u044F\u0445"),
                "."),
            mine && h("button", { class: "ghost", onClick: () => run(() => patch(`/companions/${ad.id}`, { closed: !ad.closed })) }, ad.closed ? "Открыть снова" : "Нашёл попутчиков"),
            mine && h("button", { class: "ghost danger", onClick: () => run(() => del(`/companions/${ad.id}`)) }, "\u0423\u0434\u0430\u043B\u0438\u0442\u044C"),
            !mine && h(ReportButton, { target: { type: "companion", id: ad.id } })),
        open && (h("form", { class: "reply", onSubmit: reply },
            h("label", null,
                "\u0421\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u0435 \u0430\u0432\u0442\u043E\u0440\u0443",
                h("textarea", { rows: 3, required: true, maxLength: 1000, value: text, placeholder: "\u041E\u043F\u044B\u0442, \u0441\u043D\u0430\u0440\u044F\u0436\u0435\u043D\u0438\u0435, \u043E\u0442\u043A\u0443\u0434\u0430 \u0435\u0434\u0435\u0442\u0435", onInput: (e) => setText(e.target.value) })),
            h("button", { class: "primary" }, "\u041E\u0442\u043F\u0440\u0430\u0432\u0438\u0442\u044C"))),
        h(ErrorLine, { error: error })));
}
function NewAd({ onCreated }) {
    const routes = useLoad(() => get("/routes?limit=100"), []);
    const empty = { region: "", title: "", body: "", date_from: "", date_to: "", difficulty: "", seats: "1", route_id: "" };
    const [f, setF] = useState(empty);
    const [error, setError] = useState(null);
    const set = (k) => (e) => setF({ ...f, [k]: e.target.value });
    async function submit(e) {
        e.preventDefault();
        setError(null);
        try {
            await post("/companions", {
                region: f.region, title: f.title, body: f.body, date_from: f.date_from, date_to: f.date_to || f.date_from,
                difficulty: f.difficulty ? +f.difficulty : null, seats: +f.seats, route_id: f.route_id || null,
            });
            setF(empty);
            onCreated();
        }
        catch (err) {
            setError(err);
        }
    }
    return (h("details", { class: "create" },
        h("summary", null, "\u0420\u0430\u0437\u043C\u0435\u0441\u0442\u0438\u0442\u044C \u043E\u0431\u044A\u044F\u0432\u043B\u0435\u043D\u0438\u0435"),
        h("form", { class: "stack hike-form", onSubmit: submit },
            h("label", null,
                "\u0417\u0430\u0433\u043E\u043B\u043E\u0432\u043E\u043A",
                h("input", { value: f.title, required: true, maxLength: 120, onInput: set("title"), placeholder: "\u0418\u0449\u0443 \u043A\u043E\u043C\u043F\u0430\u043D\u0438\u044E \u043D\u0430 \u041E\u0442\u043A\u043B\u0438\u043A\u043D\u043E\u0439 \u0433\u0440\u0435\u0431\u0435\u043D\u044C" })),
            h("label", null,
                "\u0420\u0430\u0439\u043E\u043D",
                h("input", { value: f.region, required: true, maxLength: 80, onInput: set("region"), placeholder: "\u0422\u0430\u0433\u0430\u043D\u0430\u0439" })),
            h("div", { class: "row-2" },
                h("label", null,
                    "\u0421",
                    h("input", { type: "date", value: f.date_from, min: today(), required: true, onInput: set("date_from") })),
                h("label", null,
                    "\u041F\u043E",
                    h("input", { type: "date", value: f.date_to, min: f.date_from || today(), onInput: set("date_to") }))),
            h("div", { class: "row-2" },
                h("label", null,
                    "\u0421\u043B\u043E\u0436\u043D\u043E\u0441\u0442\u044C",
                    h("select", { value: f.difficulty, onChange: set("difficulty") },
                        h("option", { value: "" }, "\u043D\u0435 \u0443\u043A\u0430\u0437\u0430\u043D\u0430"),
                        [1, 2, 3, 4, 5].map((d) => h("option", { key: d, value: String(d) }, difficulties[d])))),
                h("label", null,
                    "\u0421\u043A\u043E\u043B\u044C\u043A\u043E \u0447\u0435\u043B\u043E\u0432\u0435\u043A \u0438\u0449\u0443",
                    h("input", { type: "number", min: 1, max: 20, value: f.seats, onInput: set("seats") }))),
            h("label", null,
                "\u041C\u0430\u0440\u0448\u0440\u0443\u0442",
                h("select", { value: f.route_id, onChange: set("route_id") },
                    h("option", { value: "" }, "\u0431\u0435\u0437 \u043C\u0430\u0440\u0448\u0440\u0443\u0442\u0430"),
                    routes.data?.items.map((r) => h("option", { key: r.id, value: r.id }, r.title)))),
            h("label", null,
                "\u041F\u043E\u0434\u0440\u043E\u0431\u043D\u043E\u0441\u0442\u0438",
                h("textarea", { rows: 3, maxLength: 2000, value: f.body, onInput: set("body") })),
            h(ErrorLine, { error: error }),
            h("button", { class: "primary" }, "\u0420\u0430\u0437\u043C\u0435\u0441\u0442\u0438\u0442\u044C"))));
}
