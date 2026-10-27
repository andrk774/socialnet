import { h } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { api, fullName, get, post } from "./api.js";
import { PostList, Composer } from "./posts.js";
import { navigate } from "./router.js";
import { Avatar, ErrorLine, Loading, useLoad } from "./ui.js";
import { ReportButton } from "./report.js";
const friendLabel = {
    none: "Добавить в друзья", request_sent: "Отменить заявку", request_received: "Принять заявку", friends: "Удалить из друзей",
};
export function ProfilePage({ username }) {
    const prof = useLoad(() => get(`/users/by-username/${encodeURIComponent(username)}`), [username]);
    const [error, setError] = useState(null);
    if (prof.error)
        return h(ErrorLine, { error: prof.error });
    const u = prof.data;
    if (!u)
        return h(Loading, null);
    const rel = u.relationship.status;
    async function friend() {
        setError(null);
        try {
            if (rel === "none" || rel === "request_received")
                await post(`/users/${u.id}/friend`);
            else
                await api("DELETE", `/users/${u.id}/friend`);
            prof.reload();
        }
        catch (e) {
            setError(e);
        }
    }
    return (h("div", { class: "profile" },
        h("header", { class: "profile-head" },
            h(Avatar, { user: u, size: 96 }),
            h("div", null,
                h("h1", null,
                    fullName(u),
                    u.plus && h("span", { class: "plus-badge", title: "\u041F\u043E\u0434\u043F\u0438\u0441\u043A\u0430 \u00AB\u041F\u043B\u044E\u0441\u00BB" }, "\u041F\u043B\u044E\u0441")),
                h("p", { class: "muted" },
                    "@",
                    u.username,
                    u.presence?.online ? " · в сети" : ""),
                u.status_text && h("p", { class: "status" }, u.status_text),
                u.city && h("p", { class: "muted" }, u.city)),
            rel !== "self" && (h("div", { class: "profile-actions" },
                h("button", { class: rel === "none" || rel === "request_received" ? "primary" : "ghost", onClick: friend }, friendLabel[rel]),
                u.relationship.can_write && h("button", { class: "ghost", onClick: () => navigate(`/im/u/${u.id}`) }, "\u041D\u0430\u043F\u0438\u0441\u0430\u0442\u044C"),
                h(ReportButton, { target: { type: "user", id: u.id }, cls: "ghost" })))),
        h(ErrorLine, { error: error }),
        u.about && h("p", { class: "about" }, u.about),
        u.closed ? h("p", { class: "empty" }, "\u041F\u0440\u043E\u0444\u0438\u043B\u044C \u0437\u0430\u043A\u0440\u044B\u0442. \u0417\u0430\u043F\u0438\u0441\u0438 \u0432\u0438\u0434\u044F\u0442 \u0442\u043E\u043B\u044C\u043A\u043E \u0434\u0440\u0443\u0437\u044C\u044F.") : (h(PostList, { path: `/users/${u.id}/posts`, empty: rel === "self" ? "Здесь появятся ваши записи." : "Записей пока нет.", header: (add) => (rel === "self" ? h(Composer, { onPosted: add }) : null) }))));
}
