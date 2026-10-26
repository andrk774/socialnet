import { h, Fragment } from "../vendor/preact/index.js";
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { del, fullName, get, post } from "./api.js";
import { Composer, PostList, PostView } from "./posts.js";
import { query } from "./router.js";
import { Avatar, Empty, ErrorLine, Loading, Time, UserLink, duration, useLoad } from "./ui.js";
// ---------- лента, запись, хэштег ----------
export function FeedPage() {
    return (h("section", null,
        h("h1", { class: "sr-only" }, "\u041B\u0435\u043D\u0442\u0430"),
        h(PostList, { path: "/feed", empty: "\u041B\u0435\u043D\u0442\u0430 \u043F\u0443\u0441\u0442\u0430\u044F. \u0414\u043E\u0431\u0430\u0432\u044C\u0442\u0435 \u0434\u0440\u0443\u0437\u0435\u0439 \u0438\u043B\u0438 \u0432\u0441\u0442\u0443\u043F\u0438\u0442\u0435 \u0432 \u0441\u043E\u043E\u0431\u0449\u0435\u0441\u0442\u0432\u043E \u2014 \u0438\u0445 \u0437\u0430\u043F\u0438\u0441\u0438 \u043F\u043E\u044F\u0432\u044F\u0442\u0441\u044F \u0437\u0434\u0435\u0441\u044C.", header: (add) => h(Composer, { onPosted: add }) })));
}
export function PostPage({ id }) {
    const p = useLoad(() => get(`/posts/${id}`), [id]);
    if (p.error)
        return h(ErrorLine, { error: p.error });
    return p.data ? h(PostView, { initial: p.data }) : h(Loading, null);
}
export function TagPage({ tag }) {
    return (h("section", null,
        h("h1", null,
            "#",
            tag),
        h(PostList, { path: `/hashtags/${encodeURIComponent(tag)}/posts`, empty: "\u0421 \u044D\u0442\u0438\u043C \u0445\u044D\u0448\u0442\u0435\u0433\u043E\u043C \u0437\u0430\u043F\u0438\u0441\u0435\u0439 \u043D\u0435\u0442." })));
}
const notifText = {
    like: "оценил(а) вашу запись", comment: "прокомментировал(а) запись", reply: "ответил(а) на комментарий",
    mention: "упомянул(а) вас", friend_request: "хочет добавить вас в друзья", friend_accept: "принял(а) заявку в друзья",
    gift: "отправил(а) вам подарок", transfer: "перевёл(а) вам монеты", donation: "поддержал(а) сообщество", join_request: "просит вступить в сообщество", video_ready: "Видео обработано и опубликовано",
};
export function NotificationsPage({ onRead }) {
    const list = useLoad(() => get("/notifications?limit=50"), []);
    useEffect(() => { if (list.data?.unread)
        post("/notifications/read", {}).then(onRead, () => { }); }, [list.data]);
    if (list.error)
        return h(ErrorLine, { error: list.error });
    if (!list.data)
        return h(Loading, null);
    return (h("section", null,
        h("h1", null, "\u0423\u0432\u0435\u0434\u043E\u043C\u043B\u0435\u043D\u0438\u044F"),
        list.data.items.length === 0 ? h(Empty, null, "\u041D\u043E\u0432\u044B\u0445 \u0441\u043E\u0431\u044B\u0442\u0438\u0439 \u043D\u0435\u0442.") : (h("ul", { class: "notif-list" }, list.data.items.map((n) => (h("li", { key: n.id, class: n.read ? "" : "unread" },
            n.actor && h(Avatar, { user: n.actor, size: 32 }),
            h("span", null,
                n.actor && h(UserLink, { user: n.actor }),
                " ",
                n.post_id ? h("a", { href: `/p/${n.post_id}` }, notifText[n.type] ?? n.type) : notifText[n.type] ?? n.type,
                h("br", null),
                h(Time, { iso: n.created_at })))))))));
}
export function CommunitiesPage() {
    const list = useLoad(() => get("/communities?limit=50"), []);
    const [f, setF] = useState({ name: "", slug: "", visibility: "open" });
    const [error, setError] = useState(null);
    async function create(e) {
        e.preventDefault();
        try {
            await post("/communities", f);
            setF({ name: "", slug: "", visibility: "open" });
            list.reload();
        }
        catch (err) {
            setError(err);
        }
    }
    return (h("section", null,
        h("h1", null, "\u0421\u043E\u043E\u0431\u0449\u0435\u0441\u0442\u0432\u0430"),
        list.error ? h(ErrorLine, { error: list.error }) : !list.data ? h(Loading, null) : list.data.items.length === 0 ? h(Empty, null, "\u0412\u044B \u043F\u043E\u043A\u0430 \u043D\u0435 \u0441\u043E\u0441\u0442\u043E\u0438\u0442\u0435 \u0432 \u0441\u043E\u043E\u0431\u0449\u0435\u0441\u0442\u0432\u0430\u0445. \u041D\u0430\u0439\u0434\u0438\u0442\u0435 \u0438\u0445 \u0447\u0435\u0440\u0435\u0437 \u043F\u043E\u0438\u0441\u043A.") : (h("ul", { class: "plain-list" }, list.data.items.map((c) => h("li", { key: c.id },
            h("a", { href: `/c/${c.slug}` }, c.name),
            " ",
            h("span", { class: "muted" },
                c.members_count,
                " \u0443\u0447."))))),
        h("details", { class: "create" },
            h("summary", null, "\u0421\u043E\u0437\u0434\u0430\u0442\u044C \u0441\u043E\u043E\u0431\u0449\u0435\u0441\u0442\u0432\u043E"),
            h("form", { onSubmit: create, class: "stack" },
                h("label", null,
                    "\u041D\u0430\u0437\u0432\u0430\u043D\u0438\u0435",
                    h("input", { value: f.name, required: true, maxLength: 100, onInput: (e) => setF({ ...f, name: e.target.value }) })),
                h("label", null,
                    "\u0410\u0434\u0440\u0435\u0441",
                    h("input", { value: f.slug, required: true, pattern: "[A-Za-z0-9_]{3,32}", title: "3\u201332 \u0441\u0438\u043C\u0432\u043E\u043B\u0430: \u043B\u0430\u0442\u0438\u043D\u0438\u0446\u0430, \u0446\u0438\u0444\u0440\u044B, _", onInput: (e) => setF({ ...f, slug: e.target.value }) })),
                h("label", null,
                    "\u041A\u0442\u043E \u0432\u0438\u0434\u0438\u0442 \u0437\u0430\u043F\u0438\u0441\u0438",
                    h("select", { value: f.visibility, onChange: (e) => setF({ ...f, visibility: e.target.value }) },
                        h("option", { value: "open" }, "\u0412\u0441\u0435"),
                        h("option", { value: "closed" }, "\u0422\u043E\u043B\u044C\u043A\u043E \u0443\u0447\u0430\u0441\u0442\u043D\u0438\u043A\u0438 (\u0432\u0441\u0442\u0443\u043F\u043B\u0435\u043D\u0438\u0435 \u043F\u043E \u0437\u0430\u044F\u0432\u043A\u0435)"))),
                h(ErrorLine, { error: error }),
                h("button", { class: "primary" }, "\u0421\u043E\u0437\u0434\u0430\u0442\u044C")))));
}
export function CommunityPage({ slug }) {
    const c = useLoad(() => get(`/communities/by-slug/${encodeURIComponent(slug)}`), [slug]);
    const [msg, setMsg] = useState("");
    const [error, setError] = useState(null);
    if (c.error)
        return h(ErrorLine, { error: c.error });
    if (!c.data)
        return h(Loading, null);
    const com = c.data;
    async function toggle() {
        setError(null);
        try {
            if (com.my_role) {
                await post(`/communities/${com.id}/leave`);
                setMsg("");
            }
            else {
                const r = await post(`/communities/${com.id}/join`);
                setMsg(r && r.status === "requested" ? "Заявка отправлена. Администраторы рассмотрят её." : "");
            }
            c.reload();
        }
        catch (e) {
            setError(e);
        }
    }
    const canPost = ["editor", "admin", "owner"].includes(com.my_role);
    return (h("section", null,
        h("header", { class: "community-head" },
            h("h1", null, com.name),
            h("p", { class: "muted" },
                com.members_count,
                " \u0443\u0447\u0430\u0441\u0442\u043D\u0438\u043A\u043E\u0432",
                com.visibility === "closed" ? ", закрытое" : ""),
            com.description && h("p", null, com.description),
            com.my_role !== "owner" && h("button", { class: com.my_role ? "ghost" : "primary", onClick: toggle }, com.my_role ? "Выйти" : com.visibility === "closed" ? "Подать заявку" : "Вступить"),
            msg && h("p", { class: "note" }, msg),
            h(ErrorLine, { error: error })),
        h(PostList, { path: `/communities/${com.id}/posts`, empty: com.visibility === "closed" && !com.my_role ? "Записи видят только участники." : "Записей пока нет.", header: (add) => (canPost ? h(Composer, { communityId: com.id, onPosted: add }) : null) })));
}
const txKind = { transfer: "Перевод", gift: "Подарок", donation: "Пожертвование", reward: "Бонус", mint: "Начисление", purchase: "Покупка", payout: "Вывод", refund: "Возврат платежа", debt: "Погашение долга" };
export function WalletPage() {
    const w = useLoad(() => get("/wallet"), []);
    const packs = useLoad(() => get("/wallet/packages"), []);
    const [error, setError] = useState(null);
    const [status, setStatus] = useState("");
    const paymentId = query("payment");
    useEffect(() => {
        if (!paymentId)
            return;
        // вернулись со страницы оплаты: сервер сам сверит статус с ЮKassa
        get(`/wallet/purchases/${encodeURIComponent(paymentId)}`).then((p) => {
            setStatus(p.status === "succeeded" ? `Готово: начислено ${p.coins} монет.` : p.status === "pending" ? "Платёж обрабатывается. Монеты придут в течение минуты." : "Платёж не прошёл.");
            w.reload();
        }, setError);
    }, [paymentId]);
    async function buy(id) {
        setError(null);
        try {
            const p = await post("/wallet/purchases", { package_id: id });
            const u = new URL(p.confirmation_url);
            if (u.protocol === "https:")
                location.assign(u.href); // страница оплаты ЮKassa
        }
        catch (e) {
            setError(e);
        }
    }
    async function bonus() {
        setError(null);
        try {
            await post("/wallet/daily-bonus");
            w.reload();
        }
        catch (e) {
            setError(e);
        }
    }
    return (h("section", null,
        h("h1", null, "\u041A\u043E\u0448\u0435\u043B\u0451\u043A"),
        w.data && h("p", { class: "balance" },
            h("strong", null, w.data.balance.toLocaleString("ru")),
            " \u043C\u043E\u043D\u0435\u0442"),
        w.data && w.data.debt > 0 && (h("p", { class: "banner", role: "status" },
            "\u0417\u0430 \u0432\u043E\u0437\u0432\u0440\u0430\u0449\u0451\u043D\u043D\u0443\u044E \u043F\u043E\u043A\u0443\u043F\u043A\u0443 \u043E\u0441\u0442\u0430\u043B\u043E\u0441\u044C \u043F\u043E\u0433\u0430\u0441\u0438\u0442\u044C ",
            w.data.debt,
            " \u043C\u043E\u043D\u0435\u0442 \u2014 \u043E\u043D\u0438 \u0443\u0436\u0435 \u0431\u044B\u043B\u0438 \u043F\u043E\u0442\u0440\u0430\u0447\u0435\u043D\u044B. \u041F\u043E\u043A\u0430 \u0434\u043E\u043B\u0433 \u043D\u0435 \u043F\u043E\u0433\u0430\u0448\u0435\u043D, \u043F\u0435\u0440\u0435\u0432\u043E\u0434\u044B \u0438 \u043F\u043E\u0434\u0430\u0440\u043A\u0438 \u043D\u0435\u0434\u043E\u0441\u0442\u0443\u043F\u043D\u044B; \u043D\u043E\u0432\u044B\u0435 \u043C\u043E\u043D\u0435\u0442\u044B \u0441\u043D\u0430\u0447\u0430\u043B\u0430 \u0438\u0434\u0443\u0442 \u0432 \u0441\u0447\u0451\u0442 \u0434\u043E\u043B\u0433\u0430.")),
        status && h("p", { class: "note", role: "status" }, status),
        h(ErrorLine, { error: error }),
        h("div", { class: "row" },
            h("button", { class: "ghost", onClick: bonus }, "\u0417\u0430\u0431\u0440\u0430\u0442\u044C \u0435\u0436\u0435\u0434\u043D\u0435\u0432\u043D\u044B\u0439 \u0431\u043E\u043D\u0443\u0441")),
        packs.data?.enabled && (h(Fragment, null,
            h("h2", null, "\u041A\u0443\u043F\u0438\u0442\u044C \u043C\u043E\u043D\u0435\u0442\u044B"),
            h("ul", { class: "packs" }, packs.data.items.map((p) => h("li", { key: p.id },
                h("button", { class: "pack", onClick: () => buy(p.id) },
                    h("strong", null, p.coins),
                    " \u043C\u043E\u043D\u0435\u0442 ",
                    h("span", null, p.price))))),
            h("p", { class: "muted" }, "\u041E\u043F\u043B\u0430\u0442\u0430 \u0447\u0435\u0440\u0435\u0437 \u042EKassa. \u0427\u0435\u043A \u043F\u0440\u0438\u0434\u0451\u0442 \u043D\u0430 \u043F\u043E\u0434\u0442\u0432\u0435\u0440\u0436\u0434\u0451\u043D\u043D\u0443\u044E \u043F\u043E\u0447\u0442\u0443."))),
        h("h2", null, "\u0418\u0441\u0442\u043E\u0440\u0438\u044F"),
        !w.data ? h(Loading, null) : w.data.transactions.items.length === 0 ? h(Empty, null, "\u041E\u043F\u0435\u0440\u0430\u0446\u0438\u0439 \u043F\u043E\u043A\u0430 \u043D\u0435 \u0431\u044B\u043B\u043E.") : (h("table", { class: "tx" },
            h("thead", null,
                h("tr", null,
                    h("th", { scope: "col" }, "\u041E\u043F\u0435\u0440\u0430\u0446\u0438\u044F"),
                    h("th", { scope: "col" }, "\u0421 \u043A\u0435\u043C"),
                    h("th", { scope: "col", class: "num" }, "\u0421\u0443\u043C\u043C\u0430"),
                    h("th", { scope: "col" }, "\u041A\u043E\u0433\u0434\u0430"))),
            h("tbody", null, w.data.transactions.items.map((t) => (h("tr", { key: t.id },
                h("td", null,
                    txKind[t.kind] ?? t.kind,
                    t.memo && h("span", { class: "muted" },
                        " \u00B7 ",
                        t.memo)),
                h("td", null, t.counterparty.name),
                h("td", { class: t.direction === "in" ? "num in" : "num out" },
                    t.direction === "in" ? "+" : "−",
                    t.amount),
                h("td", null,
                    h(Time, { iso: t.created_at }))))))))));
}
export function SettingsPage() {
    const sessions = useLoad(() => get("/sessions"), []);
    const apps = useLoad(() => get("/users/me/authorized-apps"), []);
    const [error, setError] = useState(null);
    const act = (fn, then) => fn().then(then, setError);
    return (h("section", { class: "settings" },
        h("h1", null, "\u0411\u0435\u0437\u043E\u043F\u0430\u0441\u043D\u043E\u0441\u0442\u044C"),
        h(ErrorLine, { error: error }),
        h("h2", null, "\u0413\u0434\u0435 \u0432\u044B\u043F\u043E\u043B\u043D\u0435\u043D \u0432\u0445\u043E\u0434"),
        !sessions.data ? h(Loading, null) : (h("ul", { class: "plain-list" }, sessions.data.items.map((s) => (h("li", { key: s.id },
            h("span", null,
                s.user_agent || "Неизвестное устройство",
                s.current && h("strong", null, " \u2014 \u044D\u0442\u043E \u0443\u0441\u0442\u0440\u043E\u0439\u0441\u0442\u0432\u043E"),
                h("br", null),
                h("span", { class: "muted" },
                    s.ip ?? "",
                    " ",
                    h(Time, { iso: s.last_active }))),
            !s.current && h("button", { class: "ghost", onClick: () => act(() => del(`/sessions/${s.id}`), sessions.reload) }, "\u0417\u0430\u0432\u0435\u0440\u0448\u0438\u0442\u044C")))))),
        h("button", { class: "ghost", onClick: () => act(() => post("/sessions/revoke-others"), sessions.reload) }, "\u0417\u0430\u0432\u0435\u0440\u0448\u0438\u0442\u044C \u0432\u0441\u0435, \u043A\u0440\u043E\u043C\u0435 \u044D\u0442\u043E\u0433\u043E"),
        h("h2", null, "\u041F\u0440\u0438\u043B\u043E\u0436\u0435\u043D\u0438\u044F \u0441 \u0434\u043E\u0441\u0442\u0443\u043F\u043E\u043C"),
        !apps.data ? h(Loading, null) : apps.data.items.length === 0 ? h(Empty, null, "\u0421\u0442\u043E\u0440\u043E\u043D\u043D\u0438\u043C \u043F\u0440\u0438\u043B\u043E\u0436\u0435\u043D\u0438\u044F\u043C \u0434\u043E\u0441\u0442\u0443\u043F \u043D\u0435 \u0432\u044B\u0434\u0430\u0432\u0430\u043B\u0441\u044F.") : (h("ul", { class: "plain-list" }, apps.data.items.map((a) => (h("li", { key: a.app_id },
            h("span", null,
                h("strong", null, a.name),
                h("br", null),
                h("span", { class: "muted" }, a.scopes.join(", "))),
            h("button", { class: "ghost", onClick: () => act(() => del(`/users/me/authorized-apps/${a.app_id}`), apps.reload) }, "\u041E\u0442\u043E\u0437\u0432\u0430\u0442\u044C \u0434\u043E\u0441\u0442\u0443\u043F"))))))));
}
export function OAuthPage() {
    const params = new URLSearchParams(location.search);
    const info = useLoad(() => get(`/oauth/authorize?${params.toString()}`), []);
    const [error, setError] = useState(null);
    async function decide(approve) {
        try {
            const body = Object.fromEntries(["response_type", "client_id", "redirect_uri", "scope", "state", "code_challenge", "code_challenge_method"].map((k) => [k, params.get(k) ?? ""]));
            const r = await post("/oauth/authorize", { ...body, approve });
            location.assign(r.redirect_to); // адрес проверен сервером по списку приложения
        }
        catch (e) {
            setError(e);
        }
    }
    if (info.error)
        return h("section", { class: "consent" },
            h("h1", null, "\u0417\u0430\u043F\u0440\u043E\u0441 \u0434\u043E\u0441\u0442\u0443\u043F\u0430 \u043E\u0442\u043A\u043B\u043E\u043D\u0451\u043D"),
            h(ErrorLine, { error: info.error }));
    if (!info.data)
        return h(Loading, null);
    const d = info.data;
    const host = (() => { try {
        return new URL(d.redirect_uri).host || d.redirect_uri;
    }
    catch {
        return d.redirect_uri;
    } })();
    return (h("section", { class: "consent" },
        h("h1", null,
            d.app.name,
            " \u043F\u0440\u043E\u0441\u0438\u0442 \u0434\u043E\u0441\u0442\u0443\u043F \u043A \u0430\u043A\u043A\u0430\u0443\u043D\u0442\u0443"),
        h("p", { class: "muted" },
            "\u0420\u0430\u0437\u0440\u0430\u0431\u043E\u0442\u0447\u0438\u043A \u2014 @",
            d.app.developer,
            ". \u041F\u043E\u0441\u043B\u0435 \u043F\u043E\u0434\u0442\u0432\u0435\u0440\u0436\u0434\u0435\u043D\u0438\u044F \u0432\u044B \u0432\u0435\u0440\u043D\u0451\u0442\u0435\u0441\u044C \u043D\u0430 ",
            host,
            "."),
        h("ul", { class: "scopes" }, d.scopes.map((s) => h("li", { key: s.name }, s.description))),
        h("p", { class: "muted" }, "\u041F\u0430\u0440\u043E\u043B\u044C, \u043F\u043E\u0447\u0442\u0430, \u0441\u0435\u0430\u043D\u0441\u044B \u0438 \u043A\u043E\u0448\u0435\u043B\u0451\u043A \u043F\u0440\u0438\u043B\u043E\u0436\u0435\u043D\u0438\u044E \u043D\u0435\u0434\u043E\u0441\u0442\u0443\u043F\u043D\u044B. \u041E\u0442\u043E\u0437\u0432\u0430\u0442\u044C \u0434\u043E\u0441\u0442\u0443\u043F \u043C\u043E\u0436\u043D\u043E \u0432 \u0440\u0430\u0437\u0434\u0435\u043B\u0435 \u00AB\u0411\u0435\u0437\u043E\u043F\u0430\u0441\u043D\u043E\u0441\u0442\u044C\u00BB."),
        h(ErrorLine, { error: error }),
        h("div", { class: "row" },
            h("button", { class: "primary", onClick: () => decide(true) }, "\u0420\u0430\u0437\u0440\u0435\u0448\u0438\u0442\u044C"),
            h("button", { class: "ghost", onClick: () => decide(false) }, "\u041E\u0442\u043A\u043B\u043E\u043D\u0438\u0442\u044C"))));
}
export function MusicPage({ userId }) {
    const list = useLoad(() => get(`/users/${userId}/music?limit=50`), [userId]);
    const [playing, setPlaying] = useState(null);
    if (list.error)
        return h(ErrorLine, { error: list.error });
    if (!list.data)
        return h(Loading, null);
    return (h("section", null,
        h("h1", null, "\u041C\u0443\u0437\u044B\u043A\u0430"),
        list.data.items.length === 0 ? h(Empty, null, "\u0412 \u00AB\u041C\u043E\u0435\u0439 \u043C\u0443\u0437\u044B\u043A\u0435\u00BB \u043F\u043E\u043A\u0430 \u043F\u0443\u0441\u0442\u043E. \u0414\u043E\u0431\u0430\u0432\u043B\u044F\u0439\u0442\u0435 \u0442\u0440\u0435\u043A\u0438 \u0438\u0437 \u0437\u0430\u043F\u0438\u0441\u0435\u0439 \u0438 \u043F\u043E\u0438\u0441\u043A\u0430.") : (h("ol", { class: "tracks" }, list.data.items.map((t) => (h("li", { key: t.id },
            h("button", { class: playing?.id === t.id ? "track-btn on" : "track-btn", onClick: () => setPlaying(t), "aria-label": `Слушать: ${t.artist} — ${t.title}` }, "\u25B6"),
            h("span", null,
                t.artist,
                " \u2014 ",
                t.title),
            h("span", { class: "muted" }, duration(t.duration_sec))))))),
        playing && h("div", { class: "player" },
            h("span", null,
                playing.artist,
                " \u2014 ",
                playing.title),
            h("audio", { controls: true, autoPlay: true, src: playing.url }))));
}
export function SearchPage() {
    const q = query("q") ?? "";
    const [type, setType] = useState("users");
    const res = useLoad(() => (q ? get(`/search?q=${encodeURIComponent(q)}&type=${type}`) : Promise.resolve({ items: [] })), [q, type]);
    return (h("section", null,
        h("h1", null,
            "\u041F\u043E\u0438\u0441\u043A",
            q && `: ${q}`),
        h("div", { class: "tabs", role: "tablist" }, [["users", "Люди"], ["communities", "Сообщества"], ["tracks", "Музыка"]].map(([k, label]) => (h("button", { role: "tab", "aria-selected": type === k, class: type === k ? "tab on" : "tab", onClick: () => setType(k) }, label)))),
        !q ? h(Empty, null, "\u0412\u0432\u0435\u0434\u0438\u0442\u0435 \u0437\u0430\u043F\u0440\u043E\u0441 \u0432 \u0441\u0442\u0440\u043E\u043A\u0435 \u043F\u043E\u0438\u0441\u043A\u0430.") : res.error ? h(ErrorLine, { error: res.error }) : !res.data ? h(Loading, null) :
            res.data.items.length === 0 ? h(Empty, null, "\u041D\u0438\u0447\u0435\u0433\u043E \u043D\u0435 \u043D\u0430\u0448\u043B\u043E\u0441\u044C. \u041F\u043E\u043F\u0440\u043E\u0431\u0443\u0439\u0442\u0435 \u0434\u0440\u0443\u0433\u043E\u0435 \u043D\u0430\u043F\u0438\u0441\u0430\u043D\u0438\u0435.") : (h("ul", { class: "plain-list" }, res.data.items.map((x) => type === "users" ? h("li", { key: x.id },
                h(Avatar, { user: x, size: 32 }),
                " ",
                h("a", { href: `/u/${x.username}` }, fullName(x)),
                " ",
                h("span", { class: "muted" },
                    "@",
                    x.username)) :
                type === "communities" ? h("li", { key: x.id },
                    h("a", { href: `/c/${x.slug}` }, x.name),
                    " ",
                    h("span", { class: "muted" },
                        x.members_count,
                        " \u0443\u0447.")) :
                    h("li", { key: x.id },
                        x.artist,
                        " \u2014 ",
                        x.title,
                        " ",
                        h("audio", { controls: true, preload: "none", src: x.url })))))));
}
