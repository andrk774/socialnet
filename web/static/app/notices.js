// Предупреждения на территории: баннер на маршруте и походе, управление
// в админке (модераторы).
import { h, Fragment } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { del, get, patch, post } from "./api.js";
import { Empty, ErrorLine, Loading, useLoad } from "./ui.js";
const kindLabel = { fire: "Противопожарный режим", closure: "Тропа закрыта", flood: "Паводок", other: "Внимание" };
const dayFmt = new Intl.DateTimeFormat("ru-RU", { day: "numeric", month: "long" });
const day = (s) => dayFmt.format(new Date(s + "T12:00:00"));
const period = (n) => n.ends_on ? `${day(n.starts_on)} — ${day(n.ends_on)}` : `с ${day(n.starts_on)}, до отмены`;
export function NoticeBanner({ notices }) {
    if (!notices || notices.length === 0)
        return null;
    return (h("div", { class: "notices", role: "note" }, notices.map((n) => (h("div", { key: n.id, class: `notice notice-${n.kind}` },
        h("strong", null,
            kindLabel[n.kind],
            ": ",
            n.title),
        h("span", { class: "muted" },
            " \u00B7 ",
            period(n)),
        n.body && h("p", null, n.body),
        n.source_url && h(Fragment, null,
            " \u00B7 ",
            h("a", { href: n.source_url, target: "_blank", rel: "noopener noreferrer" }, "\u0438\u0441\u0442\u043E\u0447\u043D\u0438\u043A")))))));
}
// Прямоугольники основных районов — примерные, их можно поправить вручную.
const presets = [
    ["Таганай", [55.15, 59.75, 55.45, 60.10]],
    ["Зюраткуль", [54.80, 59.05, 55.05, 59.40]],
    ["Иремель", [54.40, 58.70, 54.65, 59.05]],
    ["Уральские Зори / Александровская сопка", [54.95, 59.45, 55.20, 59.75]],
    ["Ильменский заповедник", [54.95, 60.05, 55.25, 60.35]],
    ["Челябинская область целиком", [51.95, 57.10, 56.40, 63.40]],
];
const emptyForm = { kind: "fire", title: "", body: "", source_url: "", bbox: ["", "", "", ""], starts_on: new Date().toISOString().slice(0, 10), ends_on: "" };
export function NoticesTab() {
    const list = useLoad(() => get("/notices?all=1"), []);
    const [f, setF] = useState(emptyForm);
    const [editing, setEditing] = useState(null);
    const [error, setError] = useState(null);
    const set = (k) => (e) => setF({ ...f, [k]: e.target.value });
    const setBox = (i) => (e) => { const b = [...f.bbox]; b[i] = e.target.value; setF({ ...f, bbox: b }); };
    async function save(e) {
        e.preventDefault();
        setError(null);
        const body = { ...f, bbox: f.bbox.map(Number), ends_on: f.ends_on || null };
        try {
            if (editing)
                await patch(`/mod/notices/${editing}`, body);
            else
                await post("/mod/notices", body);
            setF(emptyForm);
            setEditing(null);
            list.reload();
        }
        catch (err) {
            setError(err);
        }
    }
    const edit = (n) => {
        setEditing(n.id);
        setF({ kind: n.kind, title: n.title, body: n.body, source_url: n.source_url, bbox: n.bbox.map(String), starts_on: n.starts_on, ends_on: n.ends_on ?? "" });
    };
    const today = new Date().toISOString().slice(0, 10);
    return (h(Fragment, null,
        h("p", { class: "muted" }, "\u041F\u043E\u043A\u0430\u0437\u044B\u0432\u0430\u044E\u0442\u0441\u044F \u043D\u0430 \u043C\u0430\u0440\u0448\u0440\u0443\u0442\u0430\u0445 \u0438 \u043F\u043E\u0445\u043E\u0434\u0430\u0445, \u043A\u043E\u0442\u043E\u0440\u044B\u0435 \u043F\u0440\u043E\u0445\u043E\u0434\u044F\u0442 \u0447\u0435\u0440\u0435\u0437 \u043F\u0440\u044F\u043C\u043E\u0443\u0433\u043E\u043B\u044C\u043D\u0438\u043A \u0432 \u0434\u043D\u0438 \u0434\u0435\u0439\u0441\u0442\u0432\u0438\u044F."),
        h("form", { class: "stack notice-form", onSubmit: save },
            h("div", { class: "row" },
                h("label", null,
                    "\u0427\u0442\u043E",
                    h("select", { value: f.kind, onChange: set("kind") }, Object.keys(kindLabel).map((k) => h("option", { key: k, value: k }, kindLabel[k])))),
                h("label", { class: "grow" },
                    "\u0417\u0430\u0433\u043E\u043B\u043E\u0432\u043E\u043A",
                    h("input", { required: true, maxLength: 200, value: f.title, onInput: set("title"), placeholder: "\u0417\u0430\u043F\u0440\u0435\u0442 \u043F\u043E\u0441\u0435\u0449\u0435\u043D\u0438\u044F \u043B\u0435\u0441\u043E\u0432" }))),
            h("label", null,
                "\u041F\u043E\u0434\u0440\u043E\u0431\u043D\u043E\u0441\u0442\u0438",
                h("textarea", { rows: 2, maxLength: 2000, value: f.body, onInput: set("body") })),
            h("label", null,
                "\u0421\u0441\u044B\u043B\u043A\u0430 \u043D\u0430 \u0438\u0441\u0442\u043E\u0447\u043D\u0438\u043A",
                h("input", { type: "url", maxLength: 500, value: f.source_url, onInput: set("source_url"), placeholder: "https://\u2026" })),
            h("div", { class: "row" },
                h("label", null,
                    "\u0420\u0430\u0439\u043E\u043D",
                    h("select", { onChange: (e) => { const p = presets[+e.target.value]; if (p)
                            setF({ ...f, bbox: p[1].map(String) }); } },
                        h("option", { value: "" }, "\u2014 \u0432\u0440\u0443\u0447\u043D\u0443\u044E \u2014"),
                        presets.map(([name], i) => h("option", { key: name, value: String(i) }, name)))),
                ["Широта от", "Долгота от", "Широта до", "Долгота до"].map((label, i) => (h("label", { key: label },
                    label,
                    h("input", { required: true, inputMode: "decimal", size: 7, value: f.bbox[i], onInput: setBox(i) }))))),
            h("div", { class: "row" },
                h("label", null,
                    "\u0421",
                    h("input", { type: "date", required: true, value: f.starts_on, onInput: set("starts_on") })),
                h("label", null,
                    "\u041F\u043E (\u043F\u0443\u0441\u0442\u043E \u2014 \u0434\u043E \u043E\u0442\u043C\u0435\u043D\u044B)",
                    h("input", { type: "date", value: f.ends_on, onInput: set("ends_on") }))),
            h(ErrorLine, { error: error }),
            h("div", { class: "row" },
                h("button", { class: "primary" }, editing ? "Сохранить" : "Добавить предупреждение"),
                editing && h("button", { type: "button", class: "ghost", onClick: () => { setEditing(null); setF(emptyForm); } }, "\u041E\u0442\u043C\u0435\u043D\u0430"))),
        h(ErrorLine, { error: list.error }),
        !list.data ? h(Loading, null) : list.data.items.length === 0 ? h(Empty, null, "\u041F\u0440\u0435\u0434\u0443\u043F\u0440\u0435\u0436\u0434\u0435\u043D\u0438\u0439 \u043D\u0435\u0442.") : (h("ul", { class: "plain-list notices-admin" }, list.data.items.map((n) => (h("li", { key: n.id, class: n.ends_on && n.ends_on < today ? "past" : "" },
            h("span", null,
                h("strong", null,
                    kindLabel[n.kind],
                    ": ",
                    n.title),
                h("br", null),
                h("span", { class: "muted" },
                    period(n),
                    " \u00B7 ",
                    n.bbox.map((v) => v.toFixed(2)).join(", "))),
            h("span", { class: "row tight" },
                h("button", { class: "ghost", onClick: () => edit(n) }, "\u0418\u0437\u043C\u0435\u043D\u0438\u0442\u044C"),
                h("button", { class: "ghost danger", onClick: () => confirm("Удалить предупреждение?") && del(`/mod/notices/${n.id}`).then(list.reload, setError) }, "\u0423\u0434\u0430\u043B\u0438\u0442\u044C")))))))));
}
