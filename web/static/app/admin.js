// Админка: сводка, пользователи, жалобы, тревоги, журнал аудита.
// Модератор видит только жалобы. Паролей здесь нет и быть не может:
// сервер хранит лишь необратимый хэш, а помочь со входом можно письмом
// со ссылкой на сброс — её получает владелец почты, не администратор.
import { h, Fragment } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { fullName, get, post, put } from "./api.js";
import { navigate, query } from "./router.js";
import { me } from "./session.js";
import { Avatar, Empty, ErrorLine, Loading, Time, useLoad } from "./ui.js";
import { NoticesTab } from "./notices.js";
import { SystemTab } from "./system.js";
const tabs = [
    ["system", "Система", true], ["roles", "Роли", true],
    ["stats", "Сводка", true], ["users", "Пользователи", true], ["reports", "Жалобы", false],
    ["notices", "Предупреждения", false], ["alerts", "Тревоги", true], ["audit", "Журнал", true],
];
export function AdminPage() {
    const isAdmin = me()?.role === "admin";
    const tab = query("tab") ?? (isAdmin ? "stats" : "reports");
    const user = query("user");
    if (me()?.role === "user")
        return h("section", null,
            h("h1", null, "\u041D\u0435\u0442 \u0434\u043E\u0441\u0442\u0443\u043F\u0430"));
    return (h("section", { class: "admin" },
        h("header", { class: "admin-heading" },
            h("div", null,
                h("span", { class: "admin-eyebrow" }, "\u0423\u043F\u0440\u0430\u0432\u043B\u0435\u043D\u0438\u0435 socialnet"),
                h("h1", null, isAdmin ? "Администрирование" : "Модерация")),
            h("span", { class: `admin-role-badge role-${isAdmin ? "admin" : "moderator"}` }, isAdmin ? "Администратор" : "Модератор")),
        h("div", { class: "tabs admin-navigation", role: "tablist", "aria-label": "\u0420\u0430\u0437\u0434\u0435\u043B\u044B \u0430\u0434\u043C\u0438\u043D\u0438\u0441\u0442\u0440\u0438\u0440\u043E\u0432\u0430\u043D\u0438\u044F" }, tabs.filter(([, , adminOnly]) => isAdmin || !adminOnly).map(([k, label]) => (h("button", { key: k, role: "tab", "aria-selected": tab === k, class: tab === k ? "tab on" : "tab", onClick: () => navigate(`/admin?tab=${k}`) }, label)))),
        tab === "stats" && isAdmin && h(StatsTab, null),
        tab === "system" && isAdmin && h(SystemTab, null),
        tab === "roles" && isAdmin && h(Fragment, null,
            h("section", { class: "admin-role-panel" },
                h("div", { class: "admin-section-heading" },
                    h("div", null,
                        h("h2", null, "\u0420\u043E\u043B\u0438 \u0438 \u043F\u0440\u0430\u0432\u0430"),
                        h("p", { class: "muted" }, "\u0412\u044B\u0431\u0435\u0440\u0438\u0442\u0435 \u043F\u043E\u043B\u044C\u0437\u043E\u0432\u0430\u0442\u0435\u043B\u044F \u043D\u0438\u0436\u0435, \u0447\u0442\u043E\u0431\u044B \u043E\u0442\u043A\u0440\u044B\u0442\u044C \u0435\u0433\u043E \u043A\u0430\u0440\u0442\u043E\u0447\u043A\u0443 \u0438 \u0438\u0437\u043C\u0435\u043D\u0438\u0442\u044C \u0434\u043E\u0441\u0442\u0443\u043F.")),
                    h("a", { class: "link", href: "/admin?tab=audit" }, "\u0416\u0443\u0440\u043D\u0430\u043B \u0438\u0437\u043C\u0435\u043D\u0435\u043D\u0438\u0439 \u2192")),
                h("div", { class: "admin-role-grid" },
                    h("article", null,
                        h("span", { class: "admin-role-badge role-user" }, "\u041F\u043E\u043B\u044C\u0437\u043E\u0432\u0430\u0442\u0435\u043B\u044C"),
                        h("h3", null, "\u041B\u0438\u0447\u043D\u044B\u0439 \u0430\u043A\u043A\u0430\u0443\u043D\u0442"),
                        h("p", null, "\u041F\u0440\u043E\u0444\u0438\u043B\u044C, \u043F\u0443\u0431\u043B\u0438\u043A\u0430\u0446\u0438\u0438, \u0441\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u044F \u0438 \u043C\u0435\u0434\u0438\u0430 \u0432 \u043F\u0440\u0435\u0434\u0435\u043B\u0430\u0445 \u043D\u0430\u0441\u0442\u0440\u043E\u0435\u043A \u0434\u043E\u0441\u0442\u0443\u043F\u0430.")),
                    h("article", null,
                        h("span", { class: "admin-role-badge role-moderator" }, "\u041C\u043E\u0434\u0435\u0440\u0430\u0442\u043E\u0440"),
                        h("h3", null, "\u041F\u043E\u0440\u044F\u0434\u043E\u043A \u0432 \u0441\u043E\u043E\u0431\u0449\u0435\u0441\u0442\u0432\u0435"),
                        h("p", null, "\u0420\u0430\u0437\u0431\u043E\u0440 \u0436\u0430\u043B\u043E\u0431 \u0438 \u043F\u0440\u0435\u0434\u0443\u043F\u0440\u0435\u0436\u0434\u0435\u043D\u0438\u0439. \u0411\u0435\u0437 \u0434\u043E\u0441\u0442\u0443\u043F\u0430 \u043A \u043C\u0435\u0442\u0440\u0438\u043A\u0430\u043C \u0441\u0438\u0441\u0442\u0435\u043C\u044B \u0438 \u043D\u0430\u0437\u043D\u0430\u0447\u0435\u043D\u0438\u044E \u0440\u043E\u043B\u0435\u0439.")),
                    h("article", null,
                        h("span", { class: "admin-role-badge role-admin" }, "\u0410\u0434\u043C\u0438\u043D\u0438\u0441\u0442\u0440\u0430\u0442\u043E\u0440"),
                        h("h3", null, "\u0423\u043F\u0440\u0430\u0432\u043B\u0435\u043D\u0438\u0435 \u043F\u043B\u0430\u0442\u0444\u043E\u0440\u043C\u043E\u0439"),
                        h("p", null, "\u041F\u043E\u043B\u044C\u0437\u043E\u0432\u0430\u0442\u0435\u043B\u0438, \u0440\u043E\u043B\u0438, \u0441\u0438\u0441\u0442\u0435\u043C\u043D\u044B\u0435 \u043C\u0435\u0442\u0440\u0438\u043A\u0438, \u0442\u0440\u0435\u0432\u043E\u0433\u0438 \u0438 \u0436\u0443\u0440\u043D\u0430\u043B \u0434\u0435\u0439\u0441\u0442\u0432\u0438\u0439."))),
                h("p", { class: "admin-role-note" }, "\u0412\u0441\u0435 \u0438\u0437\u043C\u0435\u043D\u0435\u043D\u0438\u044F \u0437\u0430\u043F\u0438\u0441\u044B\u0432\u0430\u044E\u0442\u0441\u044F \u0432 \u0436\u0443\u0440\u043D\u0430\u043B. \u0421\u0432\u043E\u044E \u0440\u043E\u043B\u044C \u0438\u0437\u043C\u0435\u043D\u0438\u0442\u044C \u043D\u0435\u043B\u044C\u0437\u044F; \u043F\u043E\u0441\u043B\u0435\u0434\u043D\u0438\u0439 \u0430\u0434\u043C\u0438\u043D\u0438\u0441\u0442\u0440\u0430\u0442\u043E\u0440 \u0441\u043E\u0445\u0440\u0430\u043D\u044F\u0435\u0442 \u0434\u043E\u0441\u0442\u0443\u043F.")),
            h(UsersTab, null)),
        tab === "users" && isAdmin && (user ? h(UserCardView, { id: +user }) : h(UsersTab, null)),
        tab === "reports" && h(ReportsTab, null),
        tab === "notices" && h(NoticesTab, null),
        tab === "alerts" && isAdmin && h(AlertsTab, null),
        tab === "audit" && isAdmin && h(AuditTab, null)));
}
function StatsTab() {
    const s = useLoad(() => get("/admin/stats"), []);
    if (s.error)
        return h(ErrorLine, { error: s.error });
    if (!s.data)
        return h(Loading, null);
    const d = s.data;
    const tiles = [
        ["Пользователей", d.users], ["Новых за сутки", d.users_new_24h], ["Новых за неделю", d.users_new_7d],
        ["Заходили за сутки", d.active_24h], ["Заблокировано", d.banned],
        ["Записей за сутки", d.posts_24h], ["Комментариев за сутки", d.comments_24h], ["Сообщений за сутки", d.messages_24h],
        ["Открытых жалоб", d.open_reports, "/admin?tab=reports"], ["Тревог детектора", d.open_alerts, "/admin?tab=alerts"],
        ["Новых сообщений об уязвимостях", d.new_vulnerability_reports], ["Отложенных записей", d.scheduled_posts_pending],
        ["Документов", d.documents], ["Оплачено за 30 дней", (d.paid_kopecks_30d / 100).toLocaleString("ru", { style: "currency", currency: "RUB" })],
    ];
    return (h("ul", { class: "tiles" }, tiles.map(([label, value, href]) => (h("li", { key: label, class: href && Number(value) > 0 ? "tile attn" : "tile" },
        h("span", { class: "tile-value" }, typeof value === "number" ? value.toLocaleString("ru") : value),
        href ? h("a", { href: href }, label) : h("span", null, label))))));
}
const roleLabel = { user: "пользователь", moderator: "модератор", admin: "администратор" };
function UsersTab() {
    const [editing, setEditing] = useState(null);
    const [q, setQ] = useState("");
    const [applied, setApplied] = useState({ q: "", filter: "all" });
    const [extra, setExtra] = useState([]);
    const list = useLoad(() => {
        setExtra([]);
        return get(`/admin/users?limit=50&filter=${applied.filter}&q=${encodeURIComponent(applied.q)}`);
    }, [applied.q, applied.filter]);
    const [cursor, setCursor] = useState(null);
    const next = cursor ?? list.data?.next_cursor ?? null;
    async function more() {
        if (!next)
            return;
        const r = await get(`/admin/users?limit=50&filter=${applied.filter}&q=${encodeURIComponent(applied.q)}&before=${next}`);
        setExtra((xs) => [...xs, ...r.items]);
        setCursor(r.next_cursor ?? 0);
    }
    const all = [...(list.data?.items ?? []), ...extra];
    return (h("section", { class: "admin-user-panel", "aria-label": "\u0423\u043F\u0440\u0430\u0432\u043B\u0435\u043D\u0438\u0435 \u043F\u043E\u043B\u044C\u0437\u043E\u0432\u0430\u0442\u0435\u043B\u044F\u043C\u0438" },
        h("div", { class: "admin-section-heading" },
            h("div", null,
                h("h2", null, "\u041F\u043E\u043B\u044C\u0437\u043E\u0432\u0430\u0442\u0435\u043B\u0438"),
                h("p", { class: "muted" }, "\u041F\u043E\u0438\u0441\u043A \u0430\u043A\u043A\u0430\u0443\u043D\u0442\u0430 \u0438 \u0443\u043F\u0440\u0430\u0432\u043B\u0435\u043D\u0438\u0435 \u0434\u043E\u0441\u0442\u0443\u043F\u043E\u043C")),
            list.data && h("span", { class: "admin-result-count" },
                "\u041F\u043E\u043A\u0430\u0437\u0430\u043D\u043E: ",
                all.length,
                next ? "+" : "")),
        h("form", { class: "row admin-search", onSubmit: (e) => { e.preventDefault(); setCursor(null); setApplied({ ...applied, q }); } },
            h("label", { class: "sr-only", for: "admin-q" }, "\u041F\u043E\u0438\u0441\u043A"),
            h("input", { id: "admin-q", type: "search", placeholder: "id, \u043F\u043E\u0447\u0442\u0430, \u0438\u043C\u044F \u043F\u043E\u043B\u044C\u0437\u043E\u0432\u0430\u0442\u0435\u043B\u044F \u0438\u043B\u0438 \u0438\u043C\u044F", value: q, maxLength: 100, onInput: (e) => setQ(e.target.value) }),
            h("label", { class: "sr-only", for: "admin-filter" }, "\u0424\u0438\u043B\u044C\u0442\u0440"),
            h("select", { id: "admin-filter", value: applied.filter, onChange: (e) => { setCursor(null); setApplied({ ...applied, filter: e.target.value }); } },
                h("option", { value: "all" }, "\u0412\u0441\u0435"),
                h("option", { value: "banned" }, "\u0417\u0430\u0431\u043B\u043E\u043A\u0438\u0440\u043E\u0432\u0430\u043D\u043D\u044B\u0435"),
                h("option", { value: "staff" }, "\u041C\u043E\u0434\u0435\u0440\u0430\u0442\u043E\u0440\u044B \u0438 \u0430\u0434\u043C\u0438\u043D\u044B"),
                h("option", { value: "unverified" }, "\u041F\u043E\u0447\u0442\u0430 \u043D\u0435 \u043F\u043E\u0434\u0442\u0432\u0435\u0440\u0436\u0434\u0435\u043D\u0430")),
            h("button", { class: "primary" }, "\u041D\u0430\u0439\u0442\u0438")),
        h(ErrorLine, { error: list.error }),
        !list.data ? h(Loading, null) : all.length === 0 ? h(Empty, null, "\u041D\u0438\u043A\u043E\u0433\u043E \u043D\u0435 \u043D\u0430\u0448\u043B\u043E\u0441\u044C.") : (h("div", { class: "admin-table-scroll", tabIndex: 0, role: "region", "aria-label": "\u0421\u043F\u0438\u0441\u043E\u043A \u043F\u043E\u043B\u044C\u0437\u043E\u0432\u0430\u0442\u0435\u043B\u0435\u0439" },
            h("table", { class: "tx admin-users" },
                h("thead", null,
                    h("tr", null,
                        h("th", { scope: "col" }, "\u041F\u043E\u043B\u044C\u0437\u043E\u0432\u0430\u0442\u0435\u043B\u044C"),
                        h("th", { scope: "col" }, "\u041F\u043E\u0447\u0442\u0430"),
                        h("th", { scope: "col" }, "\u0420\u043E\u043B\u044C"),
                        h("th", { scope: "col" }, "\u0420\u0435\u0433\u0438\u0441\u0442\u0440\u0430\u0446\u0438\u044F"),
                        h("th", { scope: "col" }, "\u0423\u043F\u0440\u0430\u0432\u043B\u0435\u043D\u0438\u0435"))),
                h("tbody", null, all.map((u) => (h("tr", { key: u.id },
                    h("td", null,
                        h("div", { class: "admin-person" },
                            h(Avatar, { user: u, size: 38 }),
                            h("div", null,
                                h("a", { href: `/admin?tab=users&user=${u.id}` }, fullName(u)),
                                h("span", { class: "muted" },
                                    "@",
                                    u.username,
                                    " \u00B7 #",
                                    u.id),
                                u.banned_at && h("span", { class: "pill bad" }, "\u0417\u0430\u0431\u043B\u043E\u043A\u0438\u0440\u043E\u0432\u0430\u043D"),
                                u.is_bot && h("span", { class: "pill" }, "\u0411\u043E\u0442")))),
                    h("td", null,
                        h("span", { class: "admin-email" }, u.email),
                        !u.email_verified && h("small", { class: "muted" }, "\u041D\u0435 \u043F\u043E\u0434\u0442\u0432\u0435\u0440\u0436\u0434\u0435\u043D\u0430")),
                    h("td", null,
                        h("span", { class: `admin-role-badge role-${u.role}` }, roleLabel[u.role])),
                    h("td", null,
                        h(Time, { iso: u.created_at })),
                    h("td", null,
                        h("button", { class: "ghost", onClick: () => setEditing(u.id) }, "\u0420\u0435\u0434\u0430\u043A\u0442\u0438\u0440\u043E\u0432\u0430\u0442\u044C"))))))))),
        next ? h("button", { class: "ghost more", onClick: more }, "\u041F\u043E\u043A\u0430\u0437\u0430\u0442\u044C \u0435\u0449\u0451") : null,
        editing !== null && h(UserEditDialog, { key: editing, id: editing, onClose: () => setEditing(null), onSaved: (id, role) => { list.set({ ...list.data, items: list.data.items.map(u => u.id === id ? { ...u, role } : u) }); setExtra(xs => xs.map(u => u.id === id ? { ...u, role } : u)); } })));
}
function UserEditDialog({ id, onClose, onSaved }) {
    const dialog = useRef(null);
    const card = useLoad(() => get(`/admin/users/${id}`), [id]);
    const [role, setRole] = useState("user");
    const [busy, setBusy] = useState(false), [error, setError] = useState(null), [saved, setSaved] = useState(false);
    useEffect(() => { dialog.current?.showModal(); }, []);
    useEffect(() => { if (card.data)
        setRole(card.data.role); }, [card.data]);
    const u = card.data, locked = !u || u.id === me()?.id || u.is_bot;
    async function save(e) { e.preventDefault(); if (!u || busy || locked)
        return; setBusy(true); setError(null); try {
        await put(`/admin/users/${id}/role`, { role });
        onSaved(id, role);
        card.set({ ...u, role });
        setSaved(true);
    }
    catch (e) {
        setError(e);
    }
    finally {
        setBusy(false);
    } }
    return h("dialog", { ref: dialog, class: "admin-edit-dialog", "aria-label": "\u0420\u0435\u0434\u0430\u043A\u0442\u0438\u0440\u043E\u0432\u0430\u043D\u0438\u0435 \u043F\u043E\u043B\u044C\u0437\u043E\u0432\u0430\u0442\u0435\u043B\u044F", onCancel: e => { if (busy)
            e.preventDefault(); }, onClose: onClose },
        h("form", { onSubmit: save },
            h("header", null,
                h("div", null,
                    h("span", { class: "admin-eyebrow" }, "\u0423\u043F\u0440\u0430\u0432\u043B\u0435\u043D\u0438\u0435 \u0430\u043A\u043A\u0430\u0443\u043D\u0442\u043E\u043C"),
                    h("h2", null, "\u041A\u0430\u0440\u0442\u043E\u0447\u043A\u0430 \u043F\u043E\u043B\u044C\u0437\u043E\u0432\u0430\u0442\u0435\u043B\u044F")),
                h("button", { type: "button", class: "icon-btn", "aria-label": "\u0417\u0430\u043A\u0440\u044B\u0442\u044C \u043A\u0430\u0440\u0442\u043E\u0447\u043A\u0443", disabled: busy, onClick: () => dialog.current?.close() }, "\u00D7")),
            h("div", { class: "admin-edit-body" },
                h(ErrorLine, { error: card.error || error }),
                !u && !card.error && h(Loading, null),
                u && h(Fragment, null,
                    h("div", { class: "admin-edit-person" },
                        h(Avatar, { user: u, size: 96 }),
                        h("h3", null, fullName(u)),
                        h("span", { class: "muted" },
                            "@",
                            u.username,
                            " \u00B7 #",
                            u.id),
                        h("span", { class: `admin-role-badge role-${u.role}` }, roleLabel[u.role])),
                    h("div", { class: "admin-edit-fields" },
                        h("label", null,
                            "\u0418\u043C\u044F \u0438 \u0444\u0430\u043C\u0438\u043B\u0438\u044F",
                            h("input", { readOnly: true, value: fullName(u) })),
                        h("label", null,
                            "\u0411\u0430\u043B\u0430\u043D\u0441, \u043C\u043E\u043D\u0435\u0442\u044B",
                            h("input", { readOnly: true, value: u.balance.toLocaleString("ru") })),
                        h("label", { class: "wide" },
                            "\u042D\u043B\u0435\u043A\u0442\u0440\u043E\u043D\u043D\u0430\u044F \u043F\u043E\u0447\u0442\u0430",
                            h("input", { readOnly: true, value: u.email })),
                        h("label", { class: "wide" },
                            "\u0420\u043E\u043B\u044C",
                            h("select", { value: role, disabled: locked || busy, onChange: e => { setRole(e.currentTarget.value); setSaved(false); } },
                                h("option", { value: "user" }, "\u041F\u043E\u043B\u044C\u0437\u043E\u0432\u0430\u0442\u0435\u043B\u044C"),
                                h("option", { value: "moderator" }, "\u041C\u043E\u0434\u0435\u0440\u0430\u0442\u043E\u0440"),
                                h("option", { value: "admin" }, "\u0410\u0434\u043C\u0438\u043D\u0438\u0441\u0442\u0440\u0430\u0442\u043E\u0440")))),
                    h("p", { class: "muted" }, locked ? "Роль собственного аккаунта и служебного бота изменить нельзя." : "Изменение роли применяется после сохранения и записывается в журнал."),
                    h("div", { class: "admin-edit-metrics" },
                        h("div", null,
                            h("strong", null, u.posts),
                            h("span", null, "\u041F\u0443\u0431\u043B\u0438\u043A\u0430\u0446\u0438\u0439")),
                        h("div", null,
                            h("strong", null, u.friends),
                            h("span", null, "\u0414\u0440\u0443\u0437\u0435\u0439")),
                        h("div", null,
                            h("strong", null, u.open_reports_against),
                            h("span", null, "\u041E\u0442\u043A\u0440\u044B\u0442\u044B\u0445 \u0436\u0430\u043B\u043E\u0431"))),
                    h(AdminSubscription, { subscription: u.subscription }),
                    h(BalanceAdjustment, { id: id, balance: u.balance, debt: u.debt, onSaved: () => card.reload() }),
                    h(AdminProfileEditor, { user: u, onSaved: () => card.reload() }),
                    saved && h("p", { role: "status", class: "note" }, "\u0418\u0437\u043C\u0435\u043D\u0435\u043D\u0438\u044F \u0441\u043E\u0445\u0440\u0430\u043D\u0435\u043D\u044B."))),
            h("footer", null,
                u && h("a", { href: `/admin?tab=users&user=${id}` }, "\u041F\u043E\u043B\u043D\u0430\u044F \u043A\u0430\u0440\u0442\u043E\u0447\u043A\u0430"),
                h("button", { type: "button", class: "ghost", disabled: busy, onClick: () => dialog.current?.close() }, "\u0417\u0430\u043A\u0440\u044B\u0442\u044C"),
                h("button", { class: "primary", disabled: busy || locked || role === u?.role }, busy ? "Сохраняю…" : "Сохранить"))));
}
function BalanceAdjustment({ id, balance, debt, onSaved }) {
    const [direction, setDirection] = useState("add"), [amount, setAmount] = useState(""), [memo, setMemo] = useState("");
    const [key, setKey] = useState(() => crypto.randomUUID()), [busy, setBusy] = useState(false), [error, setError] = useState(null), [note, setNote] = useState("");
    const change = () => { setKey(crypto.randomUUID()); setNote(""); setError(null); };
    const n = Number(amount), valid = Number.isInteger(n) && n > 0 && n <= 1000000 && memo.trim().length > 0;
    async function apply() { if (!valid || busy)
        return; setBusy(true); setError(null); try {
        const r = await post(`/admin/users/${id}/balance`, { amount: n, direction, memo: memo.trim(), idempotency_key: key });
        setNote(`Операция выполнена. Баланс: ${r.balance} монет.`);
        setKey(crypto.randomUUID());
        setAmount("");
        setMemo("");
        onSaved();
    }
    catch (e) {
        setError(e);
    }
    finally {
        setBusy(false);
    } }
    return h("fieldset", { class: "admin-balance", disabled: busy },
        h("legend", null, "\u0418\u0437\u043C\u0435\u043D\u0438\u0442\u044C \u0431\u0430\u043B\u0430\u043D\u0441"),
        h("p", { class: "muted" },
            "\u0421\u0435\u0439\u0447\u0430\u0441: ",
            balance,
            " \u043C\u043E\u043D\u0435\u0442. ",
            debt > 0 ? `Долг: ${debt} монет. Начисление сначала погасит долг.` : "Корректировка появится в кошельке и журнале администратора."),
        h("div", { class: "admin-edit-fields" },
            h("label", null,
                "\u041E\u043F\u0435\u0440\u0430\u0446\u0438\u044F",
                h("select", { value: direction, onChange: e => { change(); setDirection(e.currentTarget.value); } },
                    h("option", { value: "add" }, "\u041D\u0430\u0447\u0438\u0441\u043B\u0438\u0442\u044C"),
                    h("option", { value: "remove" }, "\u0421\u043F\u0438\u0441\u0430\u0442\u044C"))),
            h("label", null,
                "\u0421\u0443\u043C\u043C\u0430, \u043C\u043E\u043D\u0435\u0442\u044B",
                h("input", { type: "number", min: 1, max: 1000000, step: 1, value: amount, onInput: e => { change(); setAmount(e.currentTarget.value); } })),
            h("label", { class: "wide" },
                "\u041F\u0440\u0438\u0447\u0438\u043D\u0430 \u043A\u043E\u0440\u0440\u0435\u043A\u0442\u0438\u0440\u043E\u0432\u043A\u0438",
                h("input", { maxLength: 160, value: memo, placeholder: "\u0423\u043A\u0430\u0436\u0438\u0442\u0435 \u043F\u0440\u0438\u0447\u0438\u043D\u0443", onInput: e => { change(); setMemo(e.currentTarget.value); } }))),
        h(ErrorLine, { error: error }),
        note && h("p", { role: "status", class: "note" }, note),
        h("button", { type: "button", class: "primary", disabled: !valid || busy, onClick: apply }, busy ? "Применяю…" : direction === "add" ? "Начислить монеты" : "Списать монеты"));
}
function AdminSubscription({ subscription: s }) {
    return h("section", { class: "admin-subscription" },
        h("div", null,
            h("h3", null, "\u041F\u043E\u0434\u043F\u0438\u0441\u043A\u0430 \u00AB\u041F\u043B\u044E\u0441\u00BB"),
            h("span", { class: s.active ? "pill" : "muted" }, s.active ? "Активна" : s.expires_at ? "Истекла" : "Не подключена")),
        h("dl", null,
            h("div", null,
                h("dt", null, s.active ? "Действует до" : "Дата окончания"),
                h("dd", null, s.expires_at ? h(Time, { iso: s.expires_at }) : "—")),
            h("div", null,
                h("dt", null, "\u0410\u0432\u0442\u043E\u043F\u0440\u043E\u0434\u043B\u0435\u043D\u0438\u0435"),
                h("dd", null, s.auto_renew ? "Включено" : "Выключено"))));
}
function AdminProfileEditor({ user: u, onSaved }) {
    const [first, setFirst] = useState(u.first_name), [last, setLast] = useState(u.last_name), [email, setEmail] = useState(u.email), [birthday, setBirthday] = useState(u.birthday || "");
    const localDate = (value) => { const d = new Date(value); return new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 16); };
    const [created, setCreated] = useState(() => localDate(u.created_at)), [password, setPassword] = useState(""), [repeat, setRepeat] = useState(""), [busy, setBusy] = useState(false), [error, setError] = useState(null), [note, setNote] = useState("");
    async function save(pass) { setBusy(true); setError(null); setNote(""); try {
        if (pass) {
            await put(`/admin/users/${u.id}/password`, { password });
            setPassword("");
            setRepeat("");
            setNote("Новый пароль установлен. Старые сеансы завершены.");
        }
        else {
            await put(`/admin/users/${u.id}/profile`, { first_name: first, last_name: last, email, birthday: birthday || null, created_at: created === localDate(u.created_at) ? u.created_at : new Date(created).toISOString() });
            setNote("Данные пользователя сохранены.");
        }
        onSaved();
    }
    catch (e) {
        setError(e);
    }
    finally {
        setBusy(false);
    } }
    return h("fieldset", { class: "admin-balance", disabled: busy },
        h("legend", null, "\u0414\u0430\u043D\u043D\u044B\u0435 \u043F\u043E\u043B\u044C\u0437\u043E\u0432\u0430\u0442\u0435\u043B\u044F"),
        h("div", { class: "admin-edit-fields" },
            h("label", null,
                "\u0418\u043C\u044F",
                h("input", { value: first, maxLength: 64, onInput: e => setFirst(e.currentTarget.value) })),
            h("label", null,
                "\u0424\u0430\u043C\u0438\u043B\u0438\u044F",
                h("input", { value: last, maxLength: 64, onInput: e => setLast(e.currentTarget.value) })),
            h("label", { class: "wide" },
                "\u041D\u043E\u0432\u044B\u0439 email",
                h("input", { type: "email", value: email, maxLength: 254, onInput: e => setEmail(e.currentTarget.value) })),
            h("label", null,
                "\u0414\u0430\u0442\u0430 \u0440\u043E\u0436\u0434\u0435\u043D\u0438\u044F",
                h("input", { type: "date", value: birthday, onInput: e => setBirthday(e.currentTarget.value) })),
            h("label", null,
                "\u0414\u0430\u0442\u0430 \u0441\u043E\u0437\u0434\u0430\u043D\u0438\u044F \u0430\u043A\u043A\u0430\u0443\u043D\u0442\u0430",
                h("input", { type: "datetime-local", value: created, onInput: e => setCreated(e.currentTarget.value) }))),
        h("p", { class: "muted" }, "\u041F\u0440\u0438 \u0441\u043C\u0435\u043D\u0435 email \u043F\u043E\u0434\u0442\u0432\u0435\u0440\u0436\u0434\u0435\u043D\u0438\u0435 \u0430\u0434\u0440\u0435\u0441\u0430 \u0441\u043D\u0438\u043C\u0430\u0435\u0442\u0441\u044F, \u0441\u0442\u0430\u0440\u044B\u0435 \u0441\u0435\u0430\u043D\u0441\u044B \u0438 \u0441\u0441\u044B\u043B\u043A\u0438 \u0432\u043E\u0441\u0441\u0442\u0430\u043D\u043E\u0432\u043B\u0435\u043D\u0438\u044F \u043E\u0442\u0437\u044B\u0432\u0430\u044E\u0442\u0441\u044F. \u0412\u0440\u0435\u043C\u044F \u0440\u0435\u0433\u0438\u0441\u0442\u0440\u0430\u0446\u0438\u0438 \u0443\u043A\u0430\u0437\u0430\u043D\u043E \u0432 \u0447\u0430\u0441\u043E\u0432\u043E\u043C \u043F\u043E\u044F\u0441\u0435 \u0443\u0441\u0442\u0440\u043E\u0439\u0441\u0442\u0432\u0430."),
        h("button", { type: "button", class: "primary", disabled: busy || !first.trim() || !email.trim() || !created, onClick: () => save(false) }, "\u0421\u043E\u0445\u0440\u0430\u043D\u0438\u0442\u044C \u0434\u0430\u043D\u043D\u044B\u0435"),
        !u.is_bot && h(Fragment, null,
            h("h3", null, "\u0417\u0430\u0434\u0430\u0442\u044C \u043D\u043E\u0432\u044B\u0439 \u043F\u0430\u0440\u043E\u043B\u044C"),
            h("div", { class: "admin-edit-fields" },
                h("label", null,
                    "\u041D\u043E\u0432\u044B\u0439 \u043F\u0430\u0440\u043E\u043B\u044C",
                    h("input", { type: "password", autoComplete: "new-password", value: password, minLength: 8, maxLength: 128, onInput: e => setPassword(e.currentTarget.value) })),
                h("label", null,
                    "\u041F\u043E\u0432\u0442\u043E\u0440\u0438\u0442\u0435 \u043F\u0430\u0440\u043E\u043B\u044C",
                    h("input", { type: "password", autoComplete: "new-password", value: repeat, onInput: e => setRepeat(e.currentTarget.value) }))),
            h("p", { class: "muted" }, "\u041E\u0442 8 \u0434\u043E 128 \u0441\u0438\u043C\u0432\u043E\u043B\u043E\u0432. \u0422\u0435\u043A\u0443\u0449\u0438\u0439 \u043F\u0430\u0440\u043E\u043B\u044C \u043F\u043E\u0441\u043C\u043E\u0442\u0440\u0435\u0442\u044C \u043D\u0435\u043B\u044C\u0437\u044F."),
            h("button", { type: "button", class: "ghost danger", disabled: busy || password.length < 8 || password !== repeat, onClick: () => save(true) }, "\u0423\u0441\u0442\u0430\u043D\u043E\u0432\u0438\u0442\u044C \u043D\u043E\u0432\u044B\u0439 \u043F\u0430\u0440\u043E\u043B\u044C")),
        h(ErrorLine, { error: error }),
        note && h("p", { class: "note", role: "status" }, note));
}
const eventLabel = {
    register: "Регистрация", login: "Вход", login_new_device: "Вход с нового устройства", login_failed: "Неудачный вход", qr_login_approved: "Подтвердил вход по QR-коду",
    "2fa_failed": "Неверный код 2FA", "2fa_enabled": "Включена 2FA", "2fa_disabled": "Отключена 2FA",
    backup_code_used: "Вход по резервному коду", backup_codes_regenerated: "Новые резервные коды",
    password_changed: "Смена пароля", password_reset: "Пароль сброшен по ссылке", admin_password_reset: "Админ отправил ссылку для сброса пароля",
    email_verified: "Почта подтверждена", session_revoked: "Сеанс завершён", sessions_revoked: "Завершены все сеансы",
    refresh_token_reuse: "Повтор refresh-токена (возможна кража)", app_authorized: "Доступ приложению",
    app_revoked: "Доступ приложения отозван", app_created: "Создано приложение", app_secret_rotated: "Новый секрет приложения",
};
function UserCardView({ id }) {
    const card = useLoad(() => get(`/admin/users/${id}`), [id]);
    const [error, setError] = useState(null);
    const [note, setNote] = useState("");
    const run = async (fn, done) => {
        setError(null);
        setNote("");
        try {
            await fn();
            setNote(done);
            card.reload();
        }
        catch (e) {
            setError(e);
        }
    };
    if (card.error)
        return h(ErrorLine, { error: card.error });
    if (!card.data)
        return h(Loading, null);
    const u = card.data;
    const self = me()?.id === u.id;
    const reset = () => {
        const revoke = confirm(`Отправить ${u.email} письмо со ссылкой для смены пароля?\n\nОК — отправить и завершить все сеансы (если аккаунт мог быть угнан).\nОтмена — ничего не делать.`);
        if (revoke)
            run(() => post(`/admin/users/${u.id}/password-reset`, { revoke_sessions: true }), "Письмо со ссылкой отправлено, сеансы завершены. Пароль сменит сам владелец почты.");
    };
    const ban = () => {
        const reason = prompt("Причина блокировки (увидит пользователь)");
        if (reason)
            run(() => post(`/mod/users/${u.id}/ban`, { reason }), "Пользователь заблокирован, его сеансы завершены.");
    };
    return (h("div", { class: "admin-card" },
        h("p", null,
            h("a", { href: "/admin?tab=users" }, "\u2039 \u0412\u0441\u0435 \u043F\u043E\u043B\u044C\u0437\u043E\u0432\u0430\u0442\u0435\u043B\u0438")),
        h("div", { class: "profile-head" },
            h(Avatar, { user: u, size: 64 }),
            h("div", null,
                h("h2", null,
                    fullName(u),
                    " ",
                    h("span", { class: "muted" },
                        "@",
                        u.username,
                        " \u00B7 #",
                        u.id)),
                h("p", { class: "muted" },
                    u.email,
                    " \u00B7 ",
                    u.email_verified ? "почта подтверждена" : "почта не подтверждена",
                    " \u00B7 ",
                    roleLabel[u.role],
                    u.two_factor_enabled ? " · 2FA включена" : " · без 2FA"),
                u.banned_at && h("p", { class: "error" },
                    "\u0417\u0430\u0431\u043B\u043E\u043A\u0438\u0440\u043E\u0432\u0430\u043D ",
                    h(Time, { iso: u.banned_at }),
                    ": ",
                    u.ban_reason))),
        h("dl", { class: "facts" },
            h("div", null,
                h("dt", null, "\u0420\u0435\u0433\u0438\u0441\u0442\u0440\u0430\u0446\u0438\u044F"),
                h("dd", null,
                    h(Time, { iso: u.created_at }))),
            h("div", null,
                h("dt", null, "\u041F\u043E\u0441\u043B\u0435\u0434\u043D\u0438\u0439 \u0432\u0445\u043E\u0434"),
                h("dd", null, u.last_login_at ? h(Time, { iso: u.last_login_at }) : "—")),
            h("div", null,
                h("dt", null, "\u0410\u043A\u0442\u0438\u0432\u043D\u044B\u0445 \u0441\u0435\u0430\u043D\u0441\u043E\u0432"),
                h("dd", null, u.active_sessions)),
            h("div", null,
                h("dt", null, "\u0417\u0430\u043F\u0438\u0441\u0435\u0439 / \u043A\u043E\u043C\u043C\u0435\u043D\u0442\u0430\u0440\u0438\u0435\u0432"),
                h("dd", null,
                    u.posts,
                    " / ",
                    u.comments)),
            h("div", null,
                h("dt", null, "\u0414\u0440\u0443\u0437\u0435\u0439"),
                h("dd", null, u.friends)),
            h("div", null,
                h("dt", null, "\u0416\u0430\u043B\u043E\u0431 \u043D\u0430 \u043D\u0435\u0433\u043E (\u043E\u0442\u043A\u0440\u044B\u0442\u044B\u0445)"),
                h("dd", null, u.open_reports_against)),
            h("div", null,
                h("dt", null, "\u041F\u043E\u0434\u0430\u043B \u0436\u0430\u043B\u043E\u0431"),
                h("dd", null, u.reports_filed)),
            h("div", null,
                h("dt", null, "\u0411\u0430\u043B\u0430\u043D\u0441"),
                h("dd", null,
                    u.balance,
                    " \u043C\u043E\u043D\u0435\u0442",
                    u.debt > 0 && `, долг ${u.debt}`))),
        h(AdminSubscription, { subscription: u.subscription }),
        h(BalanceAdjustment, { id: id, balance: u.balance, debt: u.debt, onSaved: () => card.reload() }),
        h(AdminProfileEditor, { key: id, user: u, onSaved: () => card.reload() }),
        !self && !u.is_bot && (h("div", { class: "row" },
            !u.banned_at && h("button", { class: "ghost", onClick: reset }, "\u041E\u0442\u043F\u0440\u0430\u0432\u0438\u0442\u044C \u0441\u0441\u044B\u043B\u043A\u0443 \u0434\u043B\u044F \u0441\u0431\u0440\u043E\u0441\u0430 \u043F\u0430\u0440\u043E\u043B\u044F"),
            h("button", { class: "ghost", onClick: () => confirm("Завершить все сеансы и отозвать токены приложений?") && run(() => post(`/admin/users/${u.id}/revoke-sessions`), "Все сеансы завершены.") }, "\u0412\u044B\u0439\u0442\u0438 \u043E\u0442\u043E\u0432\u0441\u044E\u0434\u0443"),
            u.banned_at
                ? h("button", { class: "ghost", onClick: () => run(() => post(`/mod/users/${u.id}/unban`), "Блокировка снята.") }, "\u0420\u0430\u0437\u0431\u043B\u043E\u043A\u0438\u0440\u043E\u0432\u0430\u0442\u044C")
                : h("button", { class: "ghost danger", onClick: ban }, "\u0417\u0430\u0431\u043B\u043E\u043A\u0438\u0440\u043E\u0432\u0430\u0442\u044C"),
            h("label", { class: "sr-only", for: "role" }, "\u0420\u043E\u043B\u044C"),
            h("select", { id: "role", value: u.role, onChange: (e) => {
                    const role = e.target.value;
                    if (confirm(`Назначить роль «${roleLabel[role]}»?`))
                        run(() => put(`/admin/users/${u.id}/role`, { role }), "Роль изменена.");
                    else
                        e.target.value = u.role;
                } },
                h("option", { value: "user" }, "\u041F\u043E\u043B\u044C\u0437\u043E\u0432\u0430\u0442\u0435\u043B\u044C"),
                h("option", { value: "moderator" }, "\u041C\u043E\u0434\u0435\u0440\u0430\u0442\u043E\u0440"),
                h("option", { value: "admin" }, "\u0410\u0434\u043C\u0438\u043D\u0438\u0441\u0442\u0440\u0430\u0442\u043E\u0440")))),
        h("p", { class: "muted" }, "\u041F\u0430\u0440\u043E\u043B\u044C \u0430\u0434\u043C\u0438\u043D\u0438\u0441\u0442\u0440\u0430\u0442\u043E\u0440\u0443 \u043D\u0435\u0434\u043E\u0441\u0442\u0443\u043F\u0435\u043D: \u0432 \u0431\u0430\u0437\u0435 \u0445\u0440\u0430\u043D\u0438\u0442\u0441\u044F \u0442\u043E\u043B\u044C\u043A\u043E \u0435\u0433\u043E \u043D\u0435\u043E\u0431\u0440\u0430\u0442\u0438\u043C\u044B\u0439 \u0445\u044D\u0448."),
        note && h("p", { class: "note", role: "status" }, note),
        h(ErrorLine, { error: error }),
        h("h3", null, "\u0416\u0443\u0440\u043D\u0430\u043B \u0431\u0435\u0437\u043E\u043F\u0430\u0441\u043D\u043E\u0441\u0442\u0438"),
        u.recent_security_events.length === 0 ? h(Empty, null, "\u0421\u043E\u0431\u044B\u0442\u0438\u0439 \u043D\u0435\u0442.") : (h("ul", { class: "plain-list" }, u.recent_security_events.map((e) => (h("li", { key: e.id },
            h("span", null,
                eventLabel[e.type] ?? e.type,
                h("br", null),
                h("span", { class: "muted" },
                    e.ip ?? "",
                    " ",
                    e.user_agent)),
            h(Time, { iso: e.created_at }))))))));
}
const targetLabel = {
    post: "Запись", comment: "Комментарий", user: "Пользователь", community: "Сообщество", track: "Трек", video: "Видео",
    document: "Документ", companion: "Объявление о попутчиках", article: "Статья", route_point: "Точка на маршруте", route: "Маршрут", review: "Отзыв",
};
// путь /mod/{type}/{id}/hide|unhide для каждого типа жалобы
const modPath = {
    post: "posts", comment: "comments", community: "communities", track: "tracks", video: "videos", document: "documents",
    companion: "companions", article: "articles", route_point: "route-points", route: "routes", review: "reviews",
};
// reportLink — куда смотреть объект целиком; null — отдельной страницы нет.
export function reportLink(r) {
    const ref = r.target?.ref;
    switch (r.target_type) {
        case "post": return `/p/${r.target_id}`;
        case "comment": return ref ? `/p/${ref}` : null;
        case "user": return `/admin?tab=users&user=${r.target_id}`;
        case "community": return ref ? `/c/${encodeURIComponent(ref)}` : null;
        case "article": return `/articles/${r.target_id}`;
        case "companion": return "/companions";
        case "route": return r.target_uuid ? `/trails/${r.target_uuid}` : null;
        case "route_point": return ref ? `/trails/${ref}` : null;
        default: return null;
    }
}
function ReportCard({ r }) {
    const label = targetLabel[r.target_type] ?? r.target_type;
    const link = reportLink(r);
    const t = r.target;
    // маршруты — серверные страницы каталога, не SPA
    const external = r.target_type === "route" || r.target_type === "route_point";
    return (h("div", { class: "report-target" },
        h("div", null,
            link ? h("a", { href: link, target: external ? "_blank" : undefined, rel: external ? "noopener" : undefined }, label) : h("span", null, label),
            " ",
            h("span", { class: "muted" }, r.target_uuid ?? `#${r.target_id}`),
            t?.hidden && h("span", { class: "badge" }, r.target_type === "user" ? "заблокирован" : "скрыто"),
            !t && h("span", { class: "badge" }, "\u0443\u0434\u0430\u043B\u0435\u043D\u043E")),
        t?.title && h("strong", null, t.title),
        t?.snippet && h("p", { class: "report-snippet" }, t.snippet),
        t && r.target_type !== "user" && h("span", { class: "muted" },
            "\u0430\u0432\u0442\u043E\u0440: ",
            h("a", { href: `/admin?tab=users&user=${t.author_id}` },
                "@",
                t.author))));
}
function ReportsTab() {
    const [status, setStatus] = useState("open");
    const list = useLoad(() => get(`/mod/reports?status=${status}&limit=50`), [status]);
    const [error, setError] = useState(null);
    const resolve = (r, action) => {
        const note = action === "dismiss" ? "" : prompt(action === "ban" ? "Причина блокировки" : "Комментарий (необязательно)") ?? null;
        if (note === null || (action === "ban" && !note))
            return;
        post(`/mod/reports/${r.id}/resolve`, { action, note }).then(list.reload, setError);
    };
    const setHidden = (r, hidden) => post(`/mod/${modPath[r.target_type]}/${r.target_uuid ?? r.target_id}/${hidden ? "hide" : "unhide"}`).then(list.reload, setError);
    return (h(Fragment, null,
        h("div", { class: "row" },
            h("label", { for: "rep-status" }, "\u041F\u043E\u043A\u0430\u0437\u0430\u0442\u044C"),
            h("select", { id: "rep-status", value: status, onChange: (e) => setStatus(e.target.value) },
                h("option", { value: "open" }, "\u041E\u0442\u043A\u0440\u044B\u0442\u044B\u0435"),
                h("option", { value: "resolved" }, "\u0420\u0435\u0448\u0451\u043D\u043D\u044B\u0435"),
                h("option", { value: "dismissed" }, "\u041E\u0442\u043A\u043B\u043E\u043D\u0451\u043D\u043D\u044B\u0435"))),
        h(ErrorLine, { error: error ?? list.error }),
        !list.data ? h(Loading, null) : list.data.items.length === 0 ? h(Empty, null, "\u0416\u0430\u043B\u043E\u0431 \u043D\u0435\u0442.") : (h("ul", { class: "plain-list reports" }, list.data.items.map((r) => (h("li", { key: r.id, "data-type": r.target_type },
            h("span", null,
                h(ReportCard, { r: r }),
                "\u00AB",
                r.reason,
                "\u00BB",
                h("br", null),
                h("span", { class: "muted" },
                    "\u043E\u0442 ",
                    fullName(r.reporter),
                    " \u00B7 ",
                    h(Time, { iso: r.created_at }))),
            h("span", { class: "row tight" },
                r.status === "open" && h("button", { class: "ghost", onClick: () => resolve(r, "dismiss") }, "\u041E\u0442\u043A\u043B\u043E\u043D\u0438\u0442\u044C"),
                r.status === "open" && r.target_type !== "user" && r.target && !r.target.hidden && h("button", { class: "ghost", onClick: () => resolve(r, "hide") }, "\u0421\u043A\u0440\u044B\u0442\u044C"),
                r.status !== "open" && r.target && r.target_type !== "user" && modPath[r.target_type] && (r.target.hidden
                    ? h("button", { class: "ghost", onClick: () => setHidden(r, false) }, "\u0412\u0435\u0440\u043D\u0443\u0442\u044C")
                    : h("button", { class: "ghost", onClick: () => setHidden(r, true) }, "\u0421\u043A\u0440\u044B\u0442\u044C")),
                r.status === "open" && r.target && h("button", { class: "ghost danger", onClick: () => resolve(r, "ban") }, "\u0411\u0430\u043D \u0430\u0432\u0442\u043E\u0440\u0430")))))))));
}
const ruleLabel = {
    login_bruteforce: "Перебор пароля", password_spray: "Перебор по многим аккаунтам", token_reuse: "Повтор refresh-токена",
    admin_burst: "Всплеск действий админа", admin_new_ip: "Админ с нового IP", mint_large: "Крупное начисление монет",
    transfer_burst: "Всплеск переводов",
};
function AlertsTab() {
    const [all, setAll] = useState(false);
    const list = useLoad(() => get(`/admin/alerts?limit=50${all ? "&status=all" : ""}`), [all]);
    const [error, setError] = useState(null);
    return (h(Fragment, null,
        h("label", { class: "row" },
            h("input", { type: "checkbox", checked: all, onChange: (e) => setAll(e.target.checked) }),
            " \u041F\u043E\u043A\u0430\u0437\u044B\u0432\u0430\u0442\u044C \u0440\u0430\u0437\u043E\u0431\u0440\u0430\u043D\u043D\u044B\u0435"),
        h(ErrorLine, { error: error ?? list.error }),
        !list.data ? h(Loading, null) : list.data.items.length === 0 ? h(Empty, null, "\u0422\u0440\u0435\u0432\u043E\u0433 \u043D\u0435\u0442.") : (h("ul", { class: "plain-list" }, list.data.items.map((a) => (h("li", { key: a.id },
            h("span", null,
                h("span", { class: `pill sev-${a.severity}` }, a.severity),
                " ",
                ruleLabel[a.rule] ?? a.rule,
                " \u2014 ",
                a.subject,
                h("br", null),
                h("code", { class: "muted" }, JSON.stringify(a.details))),
            h("span", { class: "row tight" },
                h(Time, { iso: a.created_at }),
                a.acknowledged_by === null && h("button", { class: "ghost", onClick: () => post(`/admin/alerts/${a.id}/ack`).then(list.reload, setError) }, "\u0420\u0430\u0437\u043E\u0431\u0440\u0430\u043D\u043E")))))))));
}
function AuditTab() {
    const list = useLoad(() => get("/admin/audit?limit=50"), []);
    const [check, setCheck] = useState(null);
    const [error, setError] = useState(null);
    return (h(Fragment, null,
        h("div", { class: "row" },
            h("button", { class: "ghost", onClick: () => get("/admin/audit/verify").then(setCheck, setError) }, "\u041F\u0440\u043E\u0432\u0435\u0440\u0438\u0442\u044C \u0446\u0435\u043B\u043E\u0441\u0442\u043D\u043E\u0441\u0442\u044C \u0436\u0443\u0440\u043D\u0430\u043B\u0430"),
            check && (check.ok
                ? h("span", { role: "status" },
                    "\u0426\u0435\u043F\u043E\u0447\u043A\u0430 \u0446\u0435\u043B\u0430, \u043F\u0440\u043E\u0432\u0435\u0440\u0435\u043D\u043E \u0437\u0430\u043F\u0438\u0441\u0435\u0439: ",
                    check.checked,
                    ".")
                : h("span", { class: "error", role: "alert" },
                    "\u0416\u0443\u0440\u043D\u0430\u043B \u0438\u0437\u043C\u0435\u043D\u0451\u043D \u0437\u0430\u0434\u043D\u0438\u043C \u0447\u0438\u0441\u043B\u043E\u043C \u043D\u0430\u0447\u0438\u043D\u0430\u044F \u0441 \u0437\u0430\u043F\u0438\u0441\u0438 #",
                    check.broken_at,
                    "."))),
        h(ErrorLine, { error: error ?? list.error }),
        !list.data ? h(Loading, null) : list.data.items.length === 0 ? h(Empty, null, "\u0417\u0430\u043F\u0438\u0441\u0435\u0439 \u043D\u0435\u0442.") : (h("div", { class: "table-scroll", tabIndex: 0, role: "region", "aria-label": "Audit log" },
            h("table", { class: "tx" },
                h("thead", null,
                    h("tr", null,
                        h("th", { scope: "col" }, "\u041A\u043E\u0433\u0434\u0430"),
                        h("th", { scope: "col" }, "\u041A\u0442\u043E"),
                        h("th", { scope: "col" }, "\u0414\u0435\u0439\u0441\u0442\u0432\u0438\u0435"),
                        h("th", { scope: "col", class: "num" }, "\u041A\u043E\u0434"))),
                h("tbody", null, list.data.items.map((a) => (h("tr", { key: a.id },
                    h("td", null,
                        h(Time, { iso: a.created_at })),
                    h("td", null,
                        h("a", { href: `/admin?tab=users&user=${a.actor_id}` },
                            "#",
                            a.actor_id),
                        " ",
                        h("span", { class: "muted" },
                            a.actor_role,
                            " \u00B7 ",
                            a.ip)),
                    h("td", null,
                        h("code", null, a.action),
                        a.target && h("span", { class: "muted" },
                            " ",
                            a.target)),
                    h("td", { class: "num" }, a.status))))))))));
}
