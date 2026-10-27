// «Антенны» — свои ленты по ключевым словам: например, всё про Таганай и
// Иремель, кроме объявлений «продам».
import { h } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { del, get, post, put } from "./api.js";
import { PostList } from "./posts.js";
import { query } from "./router.js";
import { Empty, ErrorLine, Loading, useLoad } from "./ui.js";
const words = (s) => s.split(",").map((w) => w.trim()).filter(Boolean);
function AntennaForm({ initial, onDone }) {
    const [name, setName] = useState(initial?.name ?? "");
    const [kw, setKw] = useState(initial?.keywords.join(", ") ?? "");
    const [ex, setEx] = useState(initial?.excludes.join(", ") ?? "");
    const [error, setError] = useState(null);
    const submit = async (e) => {
        e.preventDefault();
        setError(null);
        const body = { name, keywords: words(kw), excludes: words(ex) };
        try {
            onDone(initial ? await put(`/antennas/${initial.id}`, body) : await post("/antennas", body));
        }
        catch (err) {
            setError(err);
        }
    };
    return (h("form", { class: "stack hike-form antenna-form", onSubmit: submit },
        h("label", null,
            "\u041D\u0430\u0437\u0432\u0430\u043D\u0438\u0435",
            h("input", { value: name, required: true, maxLength: 50, onInput: (e) => setName(e.target.value), placeholder: "\u0422\u0430\u0433\u0430\u043D\u0430\u0439 \u0438 \u0418\u0440\u0435\u043C\u0435\u043B\u044C" })),
        h("label", null,
            "\u0421\u043B\u043E\u0432\u0430 \u0447\u0435\u0440\u0435\u0437 \u0437\u0430\u043F\u044F\u0442\u0443\u044E",
            h("input", { value: kw, required: true, onInput: (e) => setKw(e.target.value), placeholder: "\u0442\u0430\u0433\u0430\u043D\u0430\u0439, \u0438\u0440\u0435\u043C\u0435\u043B\u044C, \u0437\u044E\u0440\u0430\u0442\u043A\u0443\u043B\u044C" }),
            h("span", { class: "hint" }, "\u0421\u043B\u043E\u0432\u043E\u0444\u043E\u0440\u043C\u044B \u043D\u0430\u0445\u043E\u0434\u044F\u0442\u0441\u044F \u0441\u0430\u043C\u0438: \u00AB\u0422\u0430\u0433\u0430\u043D\u0430\u0439\u00BB \u043D\u0430\u0439\u0434\u0451\u0442 \u0438 \u00AB\u043D\u0430 \u0422\u0430\u0433\u0430\u043D\u0430\u0435\u00BB.")),
        h("label", null,
            "\u041A\u0440\u043E\u043C\u0435 \u0441\u043B\u043E\u0432",
            h("input", { value: ex, onInput: (e) => setEx(e.target.value), placeholder: "\u043F\u0440\u043E\u0434\u0430\u043C, \u043A\u0443\u043F\u043B\u044E" })),
        h(ErrorLine, { error: error }),
        h("div", { class: "row" },
            h("button", { class: "primary" }, initial ? "Сохранить" : "Создать ленту"),
            h("button", { type: "button", class: "ghost", onClick: () => onDone() }, "\u041E\u0442\u043C\u0435\u043D\u0430"))));
}
export function AntennasPage() {
    const list = useLoad(() => get("/antennas"), []);
    const [active, setActive] = useState(query("id") ? +query("id") : null);
    const [mode, setMode] = useState("view");
    const [error, setError] = useState(null);
    if (list.error)
        return h(ErrorLine, { error: list.error });
    if (!list.data)
        return h(Loading, null);
    const items = list.data.items;
    const cur = items.find((a) => a.id === active) ?? null;
    return (h("section", null,
        h("div", { class: "page-head" },
            h("h1", null, "\u041C\u043E\u0438 \u043B\u0435\u043D\u0442\u044B"),
            items.length < 5 && mode === "view" && h("button", { class: "primary", onClick: () => setMode("new") }, "\u041D\u043E\u0432\u0430\u044F \u043B\u0435\u043D\u0442\u0430")),
        mode === "new" && h(AntennaForm, { onDone: (a) => { setMode("view"); if (a) {
                setActive(a.id);
                list.reload();
            } } }),
        items.length === 0 && mode === "view" ? h(Empty, null, "\u0421\u043E\u0431\u0435\u0440\u0438\u0442\u0435 \u0441\u0432\u043E\u044E \u043B\u0435\u043D\u0442\u0443 \u043F\u043E \u0441\u043B\u043E\u0432\u0430\u043C: \u043D\u0430\u043F\u0440\u0438\u043C\u0435\u0440, \u0432\u0441\u0435 \u0437\u0430\u043F\u0438\u0441\u0438 \u043F\u0440\u043E \u0422\u0430\u0433\u0430\u043D\u0430\u0439 \u0438 \u0418\u0440\u0435\u043C\u0435\u043B\u044C.") : (h("div", { class: "row tight antenna-tabs", role: "tablist" }, items.map((a) => (h("button", { key: a.id, role: "tab", "aria-selected": a.id === active, class: a.id === active ? "chip active" : "chip", onClick: () => { setActive(a.id); setMode("view"); list.reload(); } },
            a.name,
            a.unread > 0 && a.id !== active ? h("span", { class: "badge", "aria-label": `новых: ${a.unread}` }, a.unread >= 99 ? "99+" : a.unread) : null))))),
        cur && mode === "edit" && h(AntennaForm, { initial: cur, onDone: (a) => { setMode("view"); if (a)
                list.reload(); } }),
        cur && mode === "view" && (h("div", { class: "antenna" },
            h("p", { class: "muted" },
                "\u0421\u043B\u043E\u0432\u0430: ",
                cur.keywords.join(", "),
                cur.excludes.length > 0 ? ` · кроме: ${cur.excludes.join(", ")}` : "",
                " \u00B7",
                " ",
                h("button", { class: "link", onClick: () => setMode("edit") }, "\u0418\u0437\u043C\u0435\u043D\u0438\u0442\u044C"),
                " \u00B7",
                " ",
                h("button", { class: "link", onClick: () => confirm("Удалить ленту?") && del(`/antennas/${cur.id}`).then(() => { setActive(null); list.reload(); }, setError) }, "\u0423\u0434\u0430\u043B\u0438\u0442\u044C")),
            h(ErrorLine, { error: error }),
            h(PostList, { key: cur.id, path: `/antennas/${cur.id}/posts`, empty: "\u041F\u043E\u043A\u0430 \u043D\u0435\u0442 \u0437\u0430\u043F\u0438\u0441\u0435\u0439 \u0441 \u044D\u0442\u0438\u043C\u0438 \u0441\u043B\u043E\u0432\u0430\u043C\u0438." })))));
}
