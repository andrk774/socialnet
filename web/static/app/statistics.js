import { h, Fragment } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { get } from "./api.js";
import { ErrorLine, Loading, useLoad } from "./ui.js";
const date = (d) => d.toISOString().slice(0, 10);
export function StatisticsPage() {
    const [start, setStart] = useState(date(new Date(Date.now() - 6 * 86400000)));
    const [end, setEnd] = useState(date(new Date()));
    const [period, setPeriod] = useState({ start, end });
    const [kind, setKind] = useState("post");
    const [tab, setTab] = useState("overview");
    const [status, setStatus] = useState("all");
    const stats = useLoad(async () => ({ ...await get(`/users/me/statistics?start=${period.start}&end=${period.end}`), period }), [period.start, period.end]);
    const ready = stats.data?.period === period;
    const rows = (ready ? stats.data?.items || [] : []).filter(r => r.kind === kind && (kind !== "story" || status === "all" || (status === "archive" ? r.archived : !r.archived)));
    const likes = rows.reduce((n, r) => n + r.likes, 0), comments = rows.reduce((n, r) => n + r.comments, 0), viewers = rows.reduce((n, r) => n + r.viewers, 0);
    function download() {
        const cell = (s) => '"' + String(s).replace(/^[=+@\-\t\r]/, "'$&").replace(/"/g, '""') + '"';
        const lines = [["Тип", "ID", "Описание", "Дата публикации UTC", "Реакции за период", "Комментарии за период", "Уникальные просмотры истории за период"], ...rows.map(r => [r.kind, r.id, r.text, r.created_at, r.kind === "post" ? r.likes : "", r.kind === "post" ? r.comments : "", r.kind === "story" ? r.viewers : ""])];
        const url = URL.createObjectURL(new Blob(["\uFEFF" + lines.map(row => row.map(cell).join(";")).join("\r\n")], { type: "text/csv;charset=utf-8" }));
        const a = document.createElement("a");
        a.href = url;
        a.download = `socialnet-${kind}-${period.start}-${period.end}.csv`;
        a.click();
        setTimeout(() => URL.revokeObjectURL(url), 1000);
    }
    return h("section", { class: "personal-statistics" },
        h("header", { class: "card" },
            h("h1", null, "\u041C\u043E\u044F \u0441\u0442\u0430\u0442\u0438\u0441\u0442\u0438\u043A\u0430"),
            h("p", { class: "muted" }, "\u0422\u043E\u043B\u044C\u043A\u043E \u0432\u0430\u0448\u0438 \u043F\u0443\u0431\u043B\u0438\u043A\u0430\u0446\u0438\u0438. \u0414\u0430\u0442\u044B \u0438 \u0433\u0440\u0430\u043D\u0438\u0446\u044B \u043F\u0435\u0440\u0438\u043E\u0434\u0430 \u2014 UTC."),
            h("form", { class: "stats-toolbar", onSubmit: e => { e.preventDefault(); setPeriod({ start, end }); } },
                h("label", null,
                    "\u0421 ",
                    h("input", { "aria-label": "\u041D\u0430\u0447\u0430\u043B\u043E \u043F\u0435\u0440\u0438\u043E\u0434\u0430", type: "date", required: true, value: start, max: end, onInput: e => setStart(e.currentTarget.value) })),
                h("label", null,
                    "\u041F\u043E ",
                    h("input", { "aria-label": "\u041A\u043E\u043D\u0435\u0446 \u043F\u0435\u0440\u0438\u043E\u0434\u0430", type: "date", required: true, value: end, min: start, onInput: e => setEnd(e.currentTarget.value) })),
                h("button", { class: "primary" }, "\u041F\u0440\u0438\u043C\u0435\u043D\u0438\u0442\u044C"),
                h("button", { type: "button", class: "ghost", disabled: !ready || !!stats.error, onClick: download }, "\u042D\u043A\u0441\u043F\u043E\u0440\u0442 CSV")),
            h("nav", { class: "stats-toolbar", "aria-label": "\u0422\u0438\u043F \u0441\u0442\u0430\u0442\u0438\u0441\u0442\u0438\u043A\u0438" }, [["post", "Посты"], ["story", "Истории"]].map(([value, label]) => h("button", { class: kind === value ? "primary" : "ghost", "aria-pressed": kind === value, onClick: () => setKind(value) }, label)))),
        h(ErrorLine, { error: stats.error }),
        !ready && !stats.error && h(Loading, null),
        ready && stats.data && !stats.error && h("div", { class: "card" },
            h("nav", { class: "stats-toolbar", "aria-label": "\u0420\u0430\u0437\u0434\u0435\u043B \u0441\u0442\u0430\u0442\u0438\u0441\u0442\u0438\u043A\u0438" },
                [["overview", "Общее"], ["content", "Контент"], ["audience", "Аудитория"]].map(([value, label]) => h("button", { class: tab === value ? "primary" : "ghost", "aria-pressed": tab === value, onClick: () => setTab(value) }, label)),
                kind === "story" && h("select", { "aria-label": "\u0421\u043E\u0441\u0442\u043E\u044F\u043D\u0438\u0435 \u0438\u0441\u0442\u043E\u0440\u0438\u0439", value: status, onChange: e => setStatus(e.currentTarget.value) },
                    h("option", { value: "all" }, "\u0412\u0441\u0435 \u0438\u0441\u0442\u043E\u0440\u0438\u0438"),
                    h("option", { value: "active" }, "\u0410\u043A\u0442\u0438\u0432\u043D\u044B\u0435 \u0441\u0435\u0439\u0447\u0430\u0441"),
                    h("option", { value: "archive" }, "\u0410\u0440\u0445\u0438\u0432\u043D\u044B\u0435"))),
            stats.data.truncated && h("p", { role: "status" }, "\u041F\u043E\u043A\u0430\u0437\u0430\u043D\u044B \u043F\u043E\u0441\u043B\u0435\u0434\u043D\u0438\u0435 1000 \u043F\u0443\u0431\u043B\u0438\u043A\u0430\u0446\u0438\u0439. \u0418\u0442\u043E\u0433\u0438 \u0438 \u044D\u043A\u0441\u043F\u043E\u0440\u0442 \u043E\u0442\u043D\u043E\u0441\u044F\u0442\u0441\u044F \u0442\u043E\u043B\u044C\u043A\u043E \u043A \u043D\u0438\u043C."),
            tab === "audience" ? h("p", { class: "empty" }, "\u0414\u0435\u043C\u043E\u0433\u0440\u0430\u0444\u0438\u044F \u0438 \u0443\u0441\u0442\u0440\u043E\u0439\u0441\u0442\u0432\u0430 \u0437\u0440\u0438\u0442\u0435\u043B\u0435\u0439 \u043F\u043E\u043A\u0430 \u043D\u0435 \u0441\u043E\u0431\u0438\u0440\u0430\u044E\u0442\u0441\u044F. \u0414\u0430\u043D\u043D\u044B\u0445 \u0434\u043B\u044F \u0434\u0438\u0430\u0433\u0440\u0430\u043C\u043C \u0430\u0443\u0434\u0438\u0442\u043E\u0440\u0438\u0438 \u043D\u0435\u0442.") : h(Fragment, null,
                tab === "overview" && h("div", { class: "stats-metrics" },
                    h("article", null,
                        h("span", null, "\u041F\u0443\u0431\u043B\u0438\u043A\u0430\u0446\u0438\u0439 \u0434\u043E \u043A\u043E\u043D\u0446\u0430 \u043F\u0435\u0440\u0438\u043E\u0434\u0430"),
                        h("strong", null, rows.length)),
                    kind === "post" ? h(Fragment, null,
                        h("article", null,
                            h("span", null, "\u0420\u0435\u0430\u043A\u0446\u0438\u0438 \u0437\u0430 \u043F\u0435\u0440\u0438\u043E\u0434"),
                            h("strong", null, likes)),
                        h("article", null,
                            h("span", null, "\u041A\u043E\u043C\u043C\u0435\u043D\u0442\u0430\u0440\u0438\u0438 \u0437\u0430 \u043F\u0435\u0440\u0438\u043E\u0434"),
                            h("strong", null, comments))) : h("article", null,
                        h("span", null, "\u0421\u0443\u043C\u043C\u0430 \u0443\u043D\u0438\u043A\u0430\u043B\u044C\u043D\u044B\u0445 \u043F\u0440\u043E\u0441\u043C\u043E\u0442\u0440\u043E\u0432 \u0438\u0441\u0442\u043E\u0440\u0438\u0439"),
                        h("strong", null, viewers))),
                h("p", { class: "muted" }, kind === "post" ? "Сохранённые реакции и комментарии, добавленные за выбранный период, включая старые посты. Просмотры и охват постов пока не учитываются." : "Каждый зритель учитывается один раз на историю по дате первого просмотра. Собственные просмотры исключены. Один человек может быть учтён в нескольких историях."),
                tab === "overview" && h("div", { class: "stats-ranking" },
                    h("h2", null, kind === "post" ? "Публикации по числу взаимодействий" : "Истории по числу зрителей"),
                    [...rows].sort((a, b) => (b.likes + b.comments + b.viewers) - (a.likes + a.comments + a.viewers)).slice(0, 10).map(r => { const value = r.likes + r.comments + r.viewers; return h("div", { class: "stats-bar-row" },
                        h("span", null, r.text || `№${r.id}`),
                        h("meter", { min: 0, max: Math.max(1, ...rows.map(x => x.likes + x.comments + x.viewers)), value: value, "aria-label": r.text || `Публикация ${r.id}` }),
                        h("strong", null, value)); }),
                    !rows.length && h("p", { class: "muted" }, "\u041D\u0435\u0442 \u0434\u0430\u043D\u043D\u044B\u0445 \u0434\u043B\u044F \u0434\u0438\u0430\u0433\u0440\u0430\u043C\u043C\u044B.")),
                rows.length ? h("div", { class: "stats-table" },
                    h("table", null,
                        h("thead", null,
                            h("tr", null,
                                h("th", null, "\u041F\u0443\u0431\u043B\u0438\u043A\u0430\u0446\u0438\u044F"),
                                h("th", null, "\u0414\u0430\u0442\u0430"),
                                kind === "post" ? h(Fragment, null,
                                    h("th", null, "\u0420\u0435\u0430\u043A\u0446\u0438\u0438"),
                                    h("th", null, "\u041A\u043E\u043C\u043C\u0435\u043D\u0442\u0430\u0440\u0438\u0438")) : h(Fragment, null,
                                    h("th", null, "\u0417\u0440\u0438\u0442\u0435\u043B\u0438"),
                                    h("th", null, "\u0421\u043E\u0441\u0442\u043E\u044F\u043D\u0438\u0435")))),
                        h("tbody", null, rows.map(r => h("tr", { key: r.id },
                            h("td", null, kind === "post" ? h("a", { href: `/p/${r.id}` }, r.text || `Запись №${r.id}`) : r.text || `История №${r.id}`),
                            h("td", null, new Date(r.created_at).toLocaleDateString("ru-RU", { timeZone: "UTC" })),
                            h("td", null, kind === "post" ? r.likes : r.viewers),
                            h("td", null, kind === "post" ? r.comments : r.archived ? "Архивная" : "Активная")))))) : h("p", { class: "empty" }, "\u041D\u0435\u0442 \u043F\u0443\u0431\u043B\u0438\u043A\u0430\u0446\u0438\u0439 \u0434\u043B\u044F \u0432\u044B\u0431\u0440\u0430\u043D\u043D\u043E\u0433\u043E \u043F\u0435\u0440\u0438\u043E\u0434\u0430 \u0438 \u0444\u0438\u043B\u044C\u0442\u0440\u0430."))));
}
