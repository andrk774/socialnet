import { h, Fragment, render } from "../vendor/preact/index.js";
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { get, logout, onAuthChange, post, refresh, signedIn } from "./api.js";
import { AuthPage } from "./auth.js";
import { ChatPage, ConversationsPage, DirectPage } from "./messages.js";
import { CommunitiesPage, CommunityPage, FeedPage, MusicPage, NotificationsPage, OAuthPage, PostPage, SettingsPage, TagPage, WalletPage } from "./pages.js";
import { ProfilePage } from "./profile.js";
import { EventPage, HikeForm, HikesPage } from "./hikes.js";
import { CompanionsPage } from "./companions.js";
import { ConsentGate } from "./legal.js";
import { PrintPage } from "./print.js";
import { OfflinePage } from "./offlinepage.js";
import { ArticleEditor, ArticlePage, ArticlesPage } from "./articles.js";
import { RidesPage } from "./rides.js";
import { AntennasPage } from "./antennas.js";
import { PlusPage } from "./plus.js";
import { FundraisersPage } from "./fundraisers.js";
import { ForgotPasswordPage, ResetPasswordPage, VerifyEmailPage } from "./accountlinks.js";
import { AdminPage } from "./admin.js";
import { BookmarksPage, DocumentsPage, ScheduledPage } from "./library.js";
import { on, startRealtime, stopRealtime } from "./realtime.js";
import { match, navigate, query, usePath } from "./router.js";
import { loadMe, setMe, useMe } from "./session.js";
import { Avatar, ErrorLine, Loading } from "./ui.js";
import { RecordPage } from "./record.js";
import { SearchPage } from "./search.js";
import { Showcase } from "./landing.js";
function routes(path, onRead, myId) {
    let p;
    if (path === "/" || path.startsWith("/?"))
        return h(FeedPage, null);
    if ((p = match("/u/:name", path)))
        return h(ProfilePage, { username: p.name });
    if ((p = match("/p/:id", path)))
        return h(PostPage, { id: +p.id });
    if ((p = match("/tag/:tag", path)))
        return h(TagPage, { tag: p.tag });
    if (match("/im", path))
        return h(ConversationsPage, null);
    if ((p = match("/im/u/:id", path)))
        return h(DirectPage, { userId: +p.id });
    if ((p = match("/im/:id", path)))
        return h(ChatPage, { id: +p.id });
    if (match("/notifications", path))
        return h(NotificationsPage, { onRead: onRead });
    if (match("/communities", path))
        return h(CommunitiesPage, null);
    if (match("/hikes/new", path))
        return h(HikeForm, null);
    if (match("/hikes", path))
        return h(HikesPage, null);
    if (match("/companions", path))
        return h(CompanionsPage, null);
    if ((p = match("/events/:id", path)))
        return h(EventPage, { id: +p.id });
    if ((p = match("/events/:id/print", path)))
        return h(PrintPage, { eventId: +p.id });
    if ((p = match("/routes/:id/print", path)))
        return h(PrintPage, { routeId: p.id });
    if (match("/offline", path))
        return h(OfflinePage, null);
    if (match("/record", path))
        return h(RecordPage, null);
    if (match("/articles", path))
        return h(ArticlesPage, null);
    if (match("/rides", path))
        return h(RidesPage, null);
    if (match("/feeds", path))
        return h(AntennasPage, null);
    if (match("/plus", path))
        return h(PlusPage, null);
    if (match("/fundraisers", path))
        return h(FundraisersPage, null);
    if (match("/articles/new", path))
        return h(ArticleEditor, null);
    if ((p = match("/articles/:id/edit", path)))
        return h(ArticleEditor, { id: +p.id });
    if ((p = match("/articles/:id", path)))
        return h(ArticlePage, { id: +p.id });
    if ((p = match("/c/:slug", path)))
        return h(CommunityPage, { slug: p.slug });
    if (match("/music", path))
        return h(MusicPage, { userId: myId });
    if (match("/wallet", path))
        return h(WalletPage, null);
    if (match("/settings", path))
        return h(SettingsPage, null);
    if (match("/search", path))
        return h(SearchPage, null);
    if (match("/bookmarks", path))
        return h(BookmarksPage, null);
    if (match("/documents", path))
        return h(DocumentsPage, null);
    if (match("/scheduled", path)) {
        const c = query("community");
        return h(ScheduledPage, { communityId: c ? +c : undefined });
    }
    if (match("/admin", path))
        return h(AdminPage, null);
    if (match("/oauth/authorize", path))
        return h(OAuthPage, null);
    return h("section", null,
        h("h1", null, "\u0421\u0442\u0440\u0430\u043D\u0438\u0446\u0430 \u043D\u0435 \u043D\u0430\u0439\u0434\u0435\u043D\u0430"),
        h("p", null,
            h("a", { href: "/" }, "\u0412\u0435\u0440\u043D\u0443\u0442\u044C\u0441\u044F \u0432 \u043B\u0435\u043D\u0442\u0443")));
}
const navGroups = [
    ["Походы", [["/hikes", "Походы"], ["/record", "Записать трек"], ["/companions", "Попутчики"], ["/rides", "Подвоз"], ["/articles", "Статьи"], ["/fundraisers", "Сборы"], ["/offline", "Офлайн"]]],
    ["Общение", [["/", "Лента"], ["/feeds", "Мои ленты"], ["/im", "Сообщения"], ["/notifications", "Уведомления"], ["/communities", "Сообщества"]]],
    ["Моё", [["/music", "Музыка"], ["/bookmarks", "Закладки"], ["/documents", "Документы"], ["/scheduled", "Отложенные"],
            ["/wallet", "Кошелёк"], ["/plus", "Плюс"], ["/settings", "Безопасность"]]],
];
// короткие подписи для узкой панели; полные — в aria-label
const tabs = [["/", "Лента", "Лента"], ["/hikes", "Походы", "Походы"], ["/im", "Чаты", "Сообщения"], ["/notifications", "Новое", "Уведомления"]];
function Shell() {
    const path = usePath();
    const user = useMe();
    const [unread, setUnread] = useState(0);
    const [q, setQ] = useState("");
    const [more, setMore] = useState(false);
    useEffect(() => setMore(false), [path]);
    const countUnread = () => get("/notifications?limit=1").then((r) => setUnread(r.unread), () => { });
    useEffect(() => {
        countUnread();
        return on("notification", () => setUnread((n) => n + 1));
    }, []);
    if (!user)
        return h(Loading, null);
    if (user.consent_required && !path.startsWith("/settings"))
        return h(ConsentGate, null);
    const active = (href) => (href === "/" ? path === "/" : path.startsWith(href));
    const groups = user.role === "user" ? navGroups
        : [...navGroups.slice(0, 2), [navGroups[2][0], [...navGroups[2][1], ["/admin", user.role === "admin" ? "Админка" : "Модерация"]]]];
    const link = (href, label, cls, full) => (h("a", { key: href, href: href, class: cls, "aria-label": full && full !== label ? (href === "/notifications" && unread > 0 ? `${full}, новых: ${unread}` : full) : undefined, "aria-current": active(href) ? "page" : undefined },
        label,
        href === "/notifications" && unread > 0 && h("span", { class: "badge", "aria-label": `новых: ${unread}` }, unread)));
    return (h("div", { class: "shell" },
        h("header", { class: "topbar" },
            h("div", { class: "topbar-in" },
                h("a", { href: "/", class: "wordmark", "aria-label": "socialnet \u2014 \u043B\u0435\u043D\u0442\u0430" }, "socialnet"),
                h("form", { class: "search", role: "search", onSubmit: (e) => { e.preventDefault(); navigate(`/search?q=${encodeURIComponent(q)}`); } },
                    h("label", { class: "sr-only", for: "q" }, "\u041F\u043E\u0438\u0441\u043A"),
                    h("input", { id: "q", type: "search", placeholder: "\u041F\u043E\u0438\u0441\u043A: \u043F\u043E\u0445\u043E\u0434\u044B, \u043C\u0430\u0440\u0448\u0440\u0443\u0442\u044B, \u043B\u044E\u0434\u0438", value: q, onInput: (e) => setQ(e.target.value) })),
                h("a", { href: `/u/${user.username}`, class: "me", "aria-label": "\u041C\u043E\u0439 \u043F\u0440\u043E\u0444\u0438\u043B\u044C" },
                    h(Avatar, { user: user, size: 32 })))),
        h("div", { class: "layout" },
            h("nav", { class: "sidenav", "aria-label": "\u0420\u0430\u0437\u0434\u0435\u043B\u044B" },
                groups.map(([title, items]) => (h("section", { key: title, class: "nav-group" },
                    h("h2", { class: "nav-title" }, title),
                    h("ul", null, items.map(([href, label]) => h("li", { key: href }, link(href, label))))))),
                h("ul", null,
                    h("li", null,
                        h("button", { class: "link", onClick: () => logout() }, "\u0412\u044B\u0439\u0442\u0438")))),
            h("nav", { class: "tabbar", "aria-label": "\u0413\u043B\u0430\u0432\u043D\u044B\u0435 \u0440\u0430\u0437\u0434\u0435\u043B\u044B" },
                tabs.map(([href, label, full]) => link(href, label, "tab", full)),
                h("button", { class: more ? "tab active" : "tab", "aria-expanded": more, onClick: () => setMore(!more) }, "\u0415\u0449\u0451")),
            more && (h("div", { class: "more-sheet", role: "dialog", "aria-label": "\u0412\u0441\u0435 \u0440\u0430\u0437\u0434\u0435\u043B\u044B", onClick: (e) => { if (e.target.closest("a"))
                    setMore(false); } },
                groups.map(([title, items]) => (h("section", { key: title },
                    h("h2", { class: "nav-title" }, title),
                    h("div", { class: "more-grid" }, items.map(([href, label]) => link(href, label, "more-item")))))),
                h("button", { class: "ghost", onClick: () => logout() }, "\u0412\u044B\u0439\u0442\u0438"))),
            h("main", { class: "content", id: "main" },
                !user.email_verified && h(VerifyBanner, { email: user.email }),
                routes(path, () => setUnread(0), user.id)))));
}
function VerifyBanner({ email }) {
    const [sent, setSent] = useState(false);
    const [error, setError] = useState(null);
    return (h("div", { class: "banner", role: "status" },
        h("p", null,
            "\u041F\u043E\u0434\u0442\u0432\u0435\u0440\u0434\u0438\u0442\u0435 \u043F\u043E\u0447\u0442\u0443 ",
            email,
            ": \u0431\u0435\u0437 \u044D\u0442\u043E\u0433\u043E \u043D\u0435\u0434\u043E\u0441\u0442\u0443\u043F\u043D\u044B \u043A\u043E\u0448\u0435\u043B\u0451\u043A \u0438 \u043F\u043E\u043A\u0443\u043F\u043A\u0438."),
        sent ? h("p", { class: "muted" }, "\u041F\u0438\u0441\u044C\u043C\u043E \u043E\u0442\u043F\u0440\u0430\u0432\u043B\u0435\u043D\u043E. \u041F\u0440\u043E\u0432\u0435\u0440\u044C\u0442\u0435 \u043F\u043E\u0447\u0442\u0443 \u0438 \u043F\u0430\u043F\u043A\u0443 \u00AB\u0421\u043F\u0430\u043C\u00BB.") :
            h("button", { class: "ghost", onClick: () => post("/users/me/email/verification").then(() => setSent(true), setError) }, "\u041E\u0442\u043F\u0440\u0430\u0432\u0438\u0442\u044C \u043F\u0438\u0441\u044C\u043C\u043E \u0435\u0449\u0451 \u0440\u0430\u0437"),
        h(ErrorLine, { error: error })));
}
function App() {
    const [state, setState] = useState("loading");
    const lp = usePath().split("?")[0];
    useEffect(() => {
        const off = onAuthChange(async (inNow) => {
            if (inNow) {
                await loadMe().catch(() => { });
                startRealtime();
                setState("in");
            }
            else {
                stopRealtime();
                setMe(null);
                setState("out");
            }
        });
        // сеанс восстанавливается из HttpOnly-cookie
        refresh().then((ok) => { if (!ok && !signedIn())
            setState("out"); });
        return () => { off(); };
    }, []);
    // ссылки из писем и восстановление пароля — без входа
    if (lp === "/verify-email")
        return h(VerifyEmailPage, null);
    if (lp === "/reset-password")
        return h(ResetPasswordPage, null);
    if (lp === "/forgot-password" && state !== "in")
        return h(ForgotPasswordPage, null);
    if (state === "loading")
        return h(Loading, null);
    // без сети войти нельзя, но сохранённые маршруты доступны
    if (state === "out" && (!navigator.onLine || location.pathname === "/offline"))
        return h("main", { class: "content offline-only" },
            h(OfflinePage, null),
            h("p", null,
                h("a", { href: "/" }, "\u0412\u043E\u0439\u0442\u0438")));
    if (state === "in")
        return h(Shell, null);
    // гость на главной — форма входа и витрина; по ссылке вглубь — только вход
    return lp === "/" ? h(Fragment, null,
        h(AuthPage, null),
        h(Showcase, null)) : h(AuthPage, null);
}
render(h(App, null), document.getElementById("app"));
if ("serviceWorker" in navigator) {
    navigator.serviceWorker.register("/sw.js").catch((e) => console.warn("service worker", e));
}
