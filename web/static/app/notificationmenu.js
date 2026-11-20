import { h } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { get, post } from "./api.js";
import { notifText } from "./pages.js";
import { Avatar, Empty, ErrorLine, Loading, Time, UserLink } from "./ui.js";
import { Icon } from "./shellparts.js";
import { usePath } from "./router.js";
export function NotificationMenu({ unread, onRead }) {
    const root = useRef(null);
    const trigger = useRef(null);
    const path = usePath();
    const [open, setOpen] = useState(false);
    const [items, setItems] = useState(null);
    const [error, setError] = useState(null);
    const [filter, setFilter] = useState("all");
    const [busy, setBusy] = useState(false);
    useEffect(() => setOpen(false), [path]);
    useEffect(() => { if (!open)
        return; let live = true; setError(null); get("/notifications?limit=50").then(r => { if (live)
        setItems(r.items); }, e => { if (live)
        setError(e); }); return () => { live = false; }; }, [open, unread]);
    useEffect(() => { const close = (e) => { if (root.current && !root.current.contains(e.target))
        setOpen(false); }; const esc = (e) => { if (e.key === "Escape" && open) {
        setOpen(false);
        trigger.current?.focus();
    } }; document.addEventListener("pointerdown", close); document.addEventListener("keydown", esc); return () => { document.removeEventListener("pointerdown", close); document.removeEventListener("keydown", esc); }; }, [open]);
    async function read() { if (!items || busy)
        return; const ids = items.filter(n => !n.read).map(n => n.id); if (!ids.length)
        return; setBusy(true); try {
        await post("/notifications/read", { ids });
        setItems(items.map(n => ({ ...n, read: true })));
        onRead();
    }
    catch (e) {
        setError(e);
    }
    finally {
        setBusy(false);
    } }
    const visible = (items ?? []).filter(n => filter === "all" || filter === "social" && ["like", "comment", "reply", "mention", "friend_request", "friend_accept"].includes(n.type) || filter === "other" && !["like", "comment", "reply", "mention", "friend_request", "friend_accept"].includes(n.type));
    const href = (n) => n.post_id ? `/p/${n.post_id}` : n.event_id ? `/events/${n.event_id}` : n.ride_id ? `/rides#ride-${n.ride_id}` : n.type === "video_ready" ? "/videos" : n.actor ? `/u/${n.actor.username}` : "/notifications";
    return h("div", { class: "notification-menu", ref: root },
        h("button", { ref: trigger, class: "header-action", "aria-label": "\u041E\u0442\u043A\u0440\u044B\u0442\u044C \u0443\u0432\u0435\u0434\u043E\u043C\u043B\u0435\u043D\u0438\u044F", "aria-expanded": open, "aria-controls": "notification-popover", onClick: () => setOpen(!open) },
            h(Icon, { name: "bell" }),
            unread > 0 && h("span", { class: "badge" }, unread)),
        open && h("section", { class: "notification-popover", id: "notification-popover", "aria-label": "\u041F\u0430\u043D\u0435\u043B\u044C \u0443\u0432\u0435\u0434\u043E\u043C\u043B\u0435\u043D\u0438\u0439" },
            h("header", null,
                h("strong", null, "\u0423\u0432\u0435\u0434\u043E\u043C\u043B\u0435\u043D\u0438\u044F"),
                h("select", { "aria-label": "\u0418\u0441\u0442\u043E\u0447\u043D\u0438\u043A\u0438 \u0443\u0432\u0435\u0434\u043E\u043C\u043B\u0435\u043D\u0438\u0439", value: filter, onChange: e => setFilter(e.currentTarget.value) },
                    h("option", { value: "all" }, "\u0412\u0441\u0435 \u0438\u0441\u0442\u043E\u0447\u043D\u0438\u043A\u0438"),
                    h("option", { value: "social" }, "\u041E\u0431\u0449\u0435\u043D\u0438\u0435"),
                    h("option", { value: "other" }, "\u0414\u0440\u0443\u0433\u0438\u0435 \u0441\u043E\u0431\u044B\u0442\u0438\u044F")),
                h("a", { href: "/settings?tab=notifications", "aria-label": "\u041D\u0430\u0441\u0442\u0440\u043E\u0439\u043A\u0438 \u0443\u0432\u0435\u0434\u043E\u043C\u043B\u0435\u043D\u0438\u0439" },
                    h(Icon, { name: "settings" }))),
            h("div", { class: "notification-scroll" },
                h(ErrorLine, { error: error }),
                !items && !error ? h(Loading, null) : visible.length === 0 ? h(Empty, null, "\u041D\u0435\u0442 \u0443\u0432\u0435\u0434\u043E\u043C\u043B\u0435\u043D\u0438\u0439 \u043F\u043E \u0432\u044B\u0431\u0440\u0430\u043D\u043D\u043E\u043C\u0443 \u0444\u0438\u043B\u044C\u0442\u0440\u0443.") : [false, true].map(read => h("section", null,
                    visible.some(n => n.read === read) && h("h2", null, read ? "Просмотренные" : "Новые"),
                    h("ul", null, visible.filter(n => n.read === read).map(n => h("li", { key: n.id, class: n.read ? "" : "unread" },
                        n.actor ? h(Avatar, { user: n.actor, size: 44 }) : h("span", { class: "notification-system" },
                            h(Icon, { name: "bell" })),
                        h("div", null,
                            n.actor && h(UserLink, { user: n.actor }),
                            h("a", { class: "notification-event", href: href(n) }, notifText[n.type] ?? "Новое событие"),
                            h(Time, { iso: n.created_at })),
                        !n.read && h("span", { class: "notification-dot", "aria-label": "\u041D\u0435 \u043F\u0440\u043E\u0447\u0438\u0442\u0430\u043D\u043E" }))))))),
            h("footer", null,
                h("button", { class: "link", disabled: busy || !items?.some(n => !n.read), onClick: read }, "\u041E\u0442\u043C\u0435\u0442\u0438\u0442\u044C \u043F\u0440\u043E\u0447\u0438\u0442\u0430\u043D\u043D\u044B\u043C\u0438"),
                h("a", { href: "/notifications", onClick: () => setOpen(false) }, "\u041F\u043E\u043A\u0430\u0437\u0430\u0442\u044C \u0432\u0441\u0435"))));
}
