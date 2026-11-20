import { h } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { get, post } from "./api.js";
import { Empty, ErrorLine, Loading, useLoad } from "./ui.js";
import { useMe } from "./session.js";
import { Icon } from "./shellparts.js";
export function rememberCommunity(c, userId) {
    try {
        const key = `sn-communities-${userId}`;
        const old = JSON.parse(localStorage.getItem(key) ?? "[]");
        localStorage.setItem(key, JSON.stringify([c, ...(Array.isArray(old) ? old : []).filter((x) => x.id !== c.id)].slice(0, 6)));
    }
    catch { }
}
export function CommunitiesPage() {
    const me = useMe();
    const list = useLoad(async () => { const all = []; let cursor = null; do {
        const p = await get(`/communities?limit=50${cursor ? `&after=${cursor}` : ""}`);
        all.push(...p.items);
        cursor = p.next_cursor;
    } while (cursor); return all; }, []);
    const popular = useLoad(() => get("/communities/popular"), []);
    const [q, setQ] = useState("");
    const [search, setSearch] = useState(null);
    const [searching, setSearching] = useState(false);
    const [managed, setManaged] = useState(false);
    const [error, setError] = useState(null);
    const [busy, setBusy] = useState(false);
    const [recent, setRecent] = useState(() => { try {
        const v = JSON.parse(localStorage.getItem(`sn-communities-${me?.id}`) ?? "[]");
        return Array.isArray(v) ? v.filter(c => typeof c?.slug === "string" && typeof c?.name === "string") : [];
    }
    catch {
        return [];
    } });
    const [f, setF] = useState({ name: "", slug: "", visibility: "open" });
    const dialog = useRef(null);
    useEffect(() => { let active = true; setSearch(null); setError(null); if (q.trim().length < 2) {
        setSearching(false);
        return;
    } setSearching(true); const timer = setTimeout(() => { get(`/search?type=communities&limit=50&q=${encodeURIComponent(q.trim())}`).then(r => { if (active)
        setSearch(r.items); }, e => { if (active)
        setError(e); }).finally(() => { if (active)
        setSearching(false); }); }, 300); return () => { active = false; clearTimeout(timer); }; }, [q]);
    async function create(e) { e.preventDefault(); if (busy)
        return; setBusy(true); setError(null); try {
        await post("/communities", f);
        setF({ name: "", slug: "", visibility: "open" });
        dialog.current?.close();
        list.reload();
        popular.reload();
    }
    catch (e) {
        setError(e);
    }
    finally {
        setBusy(false);
    } }
    async function join(c) { if (busy)
        return; setBusy(true); setError(null); try {
        await post(`/communities/${c.id}/join`);
        list.reload();
        popular.reload();
    }
    catch (e) {
        setError(e);
    }
    finally {
        setBusy(false);
    } }
    const mine = list.data ?? [];
    const admins = mine.filter(c => ["owner", "admin", "editor"].includes(c.my_role));
    const items = q.trim().length >= 2 ? (search ?? []) : managed ? admins : mine;
    function card(c, compact = false) { return h("div", { class: "community-tile", key: c.id },
        h("a", { class: "community-avatar", href: `/c/${encodeURIComponent(c.slug)}`, "aria-label": c.name }, c.name.slice(0, 1).toUpperCase()),
        h("a", { class: "community-description", href: `/c/${encodeURIComponent(c.slug)}` },
            h("strong", null, c.name),
            h("span", { class: "muted" }, compact ? `${c.members_count} участников` : c.visibility === "closed" ? "Закрытое сообщество" : c.description || `${c.members_count} участников`)),
        compact && !c.my_role && h("button", { disabled: busy, "aria-label": `Вступить в ${c.name}`, onClick: () => join(c) },
            h(Icon, { name: "plus" }))); }
    return h("section", { class: "community-directory" },
        h("div", { class: "community-main" },
            h("h1", { class: "sr-only" }, "\u0421\u043E\u043E\u0431\u0449\u0435\u0441\u0442\u0432\u0430"),
            h("div", { class: "library-panel" },
                h("label", { class: "community-search" },
                    h(Icon, { name: "search" }),
                    h("input", { type: "search", placeholder: "\u041F\u043E\u0438\u0441\u043A \u0441\u043E\u043E\u0431\u0449\u0435\u0441\u0442\u0432", "aria-label": "\u041F\u043E\u0438\u0441\u043A \u0441\u043E\u043E\u0431\u0449\u0435\u0441\u0442\u0432", maxLength: 64, value: q, onInput: e => setQ(e.target.value) })),
                q.length === 1 && h("p", { class: "muted" }, "\u0412\u0432\u0435\u0434\u0438\u0442\u0435 \u043D\u0435 \u043C\u0435\u043D\u0435\u0435 \u0434\u0432\u0443\u0445 \u0441\u0438\u043C\u0432\u043E\u043B\u043E\u0432.")),
            recent.length > 0 && h("section", { class: "library-panel" },
                h("div", { class: "library-toolbar" },
                    h("h2", null, "\u041D\u0435\u0434\u0430\u0432\u043D\u043E \u043F\u043E\u0441\u0435\u0449\u0430\u043B\u0438"),
                    h("button", { "aria-label": "\u041E\u0447\u0438\u0441\u0442\u0438\u0442\u044C \u043D\u0435\u0434\u0430\u0432\u043D\u043E \u043F\u043E\u0441\u0435\u0449\u0451\u043D\u043D\u044B\u0435", onClick: () => { setRecent([]); try {
                            localStorage.removeItem(`sn-communities-${me?.id}`);
                        }
                        catch { } } }, "\u00D7")),
                h("div", { class: "community-recent" }, recent.map(c => h("a", { href: `/c/${encodeURIComponent(c.slug)}` },
                    h("span", { class: "community-avatar" }, c.name.slice(0, 1)),
                    h("span", null, c.name))))),
            h("section", { class: "library-panel" },
                h("nav", { class: "library-tabs", "aria-label": "\u041C\u043E\u0438 \u0441\u043E\u043E\u0431\u0449\u0435\u0441\u0442\u0432\u0430" },
                    h("button", { "aria-pressed": !managed, onClick: () => { setManaged(false); setQ(""); } },
                        "\u0412\u0441\u0435 \u0441\u043E\u043E\u0431\u0449\u0435\u0441\u0442\u0432\u0430 ",
                        mine.length),
                    h("button", { "aria-pressed": managed, onClick: () => { setManaged(true); setQ(""); } },
                        "\u0423\u043F\u0440\u0430\u0432\u043B\u044F\u0435\u043C\u044B\u0435 ",
                        admins.length)),
                h(ErrorLine, { error: error || list.error }),
                searching || !list.data && !list.error ? h(Loading, null) : items.length ? h("div", { class: "community-grid" }, items.map(c => card(c))) : h(Empty, null, q.trim().length >= 2 ? "Сообщества не найдены." : managed ? "Вы пока не управляете сообществами." : "Найдите сообщество или создайте своё."))),
        h("aside", { class: "community-side" },
            h("div", { class: "library-panel" },
                h("button", { class: "primary", onClick: () => dialog.current?.showModal() }, "\u0421\u043E\u0437\u0434\u0430\u0442\u044C \u0441\u043E\u043E\u0431\u0449\u0435\u0441\u0442\u0432\u043E"),
                h("a", { class: "community-events", href: "/hikes" }, "\u041C\u0435\u0440\u043E\u043F\u0440\u0438\u044F\u0442\u0438\u044F \u0438 \u043F\u043E\u0445\u043E\u0434\u044B \u2192")),
            h("section", { class: "library-panel" },
                h("h2", null, "\u041F\u043E\u043F\u0443\u043B\u044F\u0440\u043D\u043E\u0435"),
                h(ErrorLine, { error: popular.error }),
                popular.data ? popular.data.items.length ? popular.data.items.map(c => card(c, true)) : h(Empty, null, "\u041E\u0442\u043A\u0440\u044B\u0442\u044B\u0445 \u0441\u043E\u043E\u0431\u0449\u0435\u0441\u0442\u0432 \u043F\u043E\u043A\u0430 \u043D\u0435\u0442.") : !popular.error && h(Loading, null))),
        h("dialog", { class: "music-dialog", ref: dialog, "aria-label": "\u0421\u043E\u0437\u0434\u0430\u0442\u044C \u0441\u043E\u043E\u0431\u0449\u0435\u0441\u0442\u0432\u043E" },
            h("form", { onSubmit: create, class: "stack" },
                h("h2", null, "\u0421\u043E\u0437\u0434\u0430\u0442\u044C \u0441\u043E\u043E\u0431\u0449\u0435\u0441\u0442\u0432\u043E"),
                h("label", null,
                    "\u041D\u0430\u0437\u0432\u0430\u043D\u0438\u0435",
                    h("input", { required: true, maxLength: 100, value: f.name, onInput: e => setF({ ...f, name: e.target.value }) })),
                h("label", null,
                    "\u0410\u0434\u0440\u0435\u0441",
                    h("input", { required: true, pattern: "[A-Za-z0-9_]{3,32}", title: "3\u201332 \u0441\u0438\u043C\u0432\u043E\u043B\u0430: \u043B\u0430\u0442\u0438\u043D\u0438\u0446\u0430, \u0446\u0438\u0444\u0440\u044B, _", value: f.slug, onInput: e => setF({ ...f, slug: e.target.value }) })),
                h("label", null,
                    "\u041A\u0442\u043E \u0432\u0438\u0434\u0438\u0442 \u0437\u0430\u043F\u0438\u0441\u0438",
                    h("select", { value: f.visibility, onChange: e => setF({ ...f, visibility: e.target.value }) },
                        h("option", { value: "open" }, "\u0412\u0441\u0435"),
                        h("option", { value: "closed" }, "\u0422\u043E\u043B\u044C\u043A\u043E \u0443\u0447\u0430\u0441\u0442\u043D\u0438\u043A\u0438 (\u0432\u0441\u0442\u0443\u043F\u043B\u0435\u043D\u0438\u0435 \u043F\u043E \u0437\u0430\u044F\u0432\u043A\u0435)"))),
                h(ErrorLine, { error: error }),
                h("div", { class: "library-actions" },
                    h("button", { class: "primary", disabled: busy }, "\u0421\u043E\u0437\u0434\u0430\u0442\u044C"),
                    h("button", { type: "button", onClick: () => dialog.current?.close() }, "\u041E\u0442\u043C\u0435\u043D\u0430")))));
}
