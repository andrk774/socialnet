import { NotificationMenu } from "./notificationmenu.js";
import { StatisticsPage } from "./statistics.js";
import { CommentArchivePage } from "./commentarchive.js";
import { CallsPage, CallManager } from "./calls.js";
import { DevelopersPage } from "./developers.js";
import { SettingsHub } from "./settingshub.js";
import { hiddenSections } from "./navprefs.js";
import { HelpPage, InfoPage, LegalIndex, SidebarFooter } from "./helpcenter.js";
import { MusicPage } from "./music.js";
import { MiniPlayer } from "./miniplayer.js";
import { PhotosPage } from "./photos.js";
import { h, Fragment, render } from "../vendor/preact/index.js";
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { get, logout, onAuthChange, post, refresh, signedIn } from "./api.js";
import { AuthPage } from "./auth.js";
import { ChatPage, ConversationsPage, DirectPage } from "./messages.js";
import { CommunitiesPage, CommunityPage, FeedPage, NotificationsPage, OAuthPage, PostPage, TagPage, WalletPage } from "./pages.js";
import { ProfilePage } from "./profile.js";
import { VideosPage } from "./uploads.js";
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
import { ErrorLine, Loading } from "./ui.js";
import { RecordPage } from "./record.js";
import { SearchPage } from "./search.js";
import { Showcase } from "./landing.js";
import { setupNative } from "./native.js";
import { QRApprovePage } from "./qrlogin.js";
import { AccountMenu, FeedAside, Icon, navIcon } from "./shellparts.js";
function routes(path, onRead, myId) {
    let p;
    if (match("/help", path))
        return h(HelpPage, null);
    if (match("/legal-info", path))
        return h(LegalIndex, null);
    for (const kind of ["about", "authors", "business", "safety"])
        if (match(`/${kind}`, path))
            return h(InfoPage, { kind: kind });
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
    if (match("/photos", path))
        return h(PhotosPage, null);
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
    if (match("/qr-login", path))
        return h(QRApprovePage, null);
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
        return null;
    if (match("/videos", path))
        return h(VideosPage, { userId: myId });
    if (match("/wallet", path))
        return h(WalletPage, null);
    if (match("/statistics", path))
        return h(StatisticsPage, null);
    if (match("/calls", path))
        return h(CallsPage, null);
    if (match("/developers", path))
        return h(DevelopersPage, null);
    if (match("/help/comments", path))
        return h(CommentArchivePage, null);
    if (match("/settings", path))
        return h(SettingsHub, null);
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
    ["Общение", [["/", "Лента"], ["/feeds", "Мои ленты"], ["/im", "Сообщения"], ["/calls", "Звонки"], ["/notifications", "Уведомления"], ["/communities", "Сообщества"]]],
    ["Походы", [["/hikes", "Походы"], ["/record", "Записать трек"], ["/companions", "Попутчики"], ["/rides", "Подвоз"], ["/articles", "Статьи"], ["/fundraisers", "Сборы"], ["/offline", "Офлайн"]]],
    ["Моё", [["/photos", "Фото"], ["/music", "Музыка"], ["/videos", "Видео"], ["/bookmarks", "Закладки"], ["/documents", "Документы"], ["/scheduled", "Отложенные"],
            ["/wallet", "Кошелёк"], ["/plus", "Плюс"], ["/settings", "Настройки"]]],
];
// короткие подписи для узкой панели; полные — в aria-label
const tabs = [["/", "Лента", "Лента"], ["/hikes", "Походы", "Походы"], ["/im", "Чаты", "Сообщения"], ["/notifications", "Новое", "Уведомления"]];
function Shell() {
    const path = usePath();
    const user = useMe();
    const [unread, setUnread] = useState(0);
    const [q, setQ] = useState("");
    const [more, setMore] = useState(false);
    const musicActive = path.split("?")[0] === "/music";
    const [musicLoaded, setMusicLoaded] = useState(musicActive);
    const [player, setPlayer] = useState(null);
    useEffect(() => { if (musicActive)
        setMusicLoaded(true); }, [musicActive]);
    const [hidden, setHidden] = useState([]);
    useEffect(() => { const update = () => setHidden(user ? hiddenSections(user.id) : []); update(); window.addEventListener("sn-navigation", update); return () => window.removeEventListener("sn-navigation", update); }, [user?.id]);
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
    const allGroups = user.role === "user" ? navGroups
        : [...navGroups.slice(0, 2), [navGroups[2][0], [...navGroups[2][1], ["/admin", user.role === "admin" ? "Админка" : "Модерация"]]]];
    const groups = allGroups.map(([title, items]) => [title, items.filter(([url]) => !hidden.includes(url))]);
    const link = (href, label, cls, full) => (h("a", { key: href, href: href, class: cls, "aria-label": full && full !== label ? (href === "/notifications" && unread > 0 ? `${full}, новых: ${unread}` : full) : undefined, "aria-current": active(href) ? "page" : undefined },
        h(Icon, { name: navIcon(href) }),
        h("span", null, label),
        href === "/notifications" && unread > 0 && h("span", { class: "badge", "aria-label": `новых: ${unread}` }, unread)));
    return (h("div", { class: "shell" },
        h(CallManager, null),
        h("header", { class: "topbar" },
            h("div", { class: "topbar-in" },
                h("a", { href: "/", class: "wordmark", "aria-label": "socialnet \u2014 \u043B\u0435\u043D\u0442\u0430" },
                    h("span", { class: "brand-mark", "aria-hidden": "true" }, "s"),
                    "socialnet"),
                h("form", { class: "search", role: "search", onSubmit: (e) => { e.preventDefault(); navigate(`/search?q=${encodeURIComponent(q)}`); } },
                    h("label", { class: "sr-only", for: "q" }, "\u041F\u043E\u0438\u0441\u043A"),
                    h(Icon, { name: "search" }),
                    h("input", { id: "q", type: "search", placeholder: "\u041F\u043E\u0438\u0441\u043A: \u043F\u043E\u0445\u043E\u0434\u044B, \u043C\u0430\u0440\u0448\u0440\u0443\u0442\u044B, \u043B\u044E\u0434\u0438", value: q, onInput: (e) => setQ(e.target.value) })),
                h("div", { class: "topbar-actions" },
                    h(NotificationMenu, { unread: unread, onRead: countUnread }),
                    h("a", { href: "/music", class: "header-action", "aria-label": "\u041E\u0442\u043A\u0440\u044B\u0442\u044C \u043C\u0443\u0437\u044B\u043A\u0443" },
                        h(Icon, { name: "music" }))),
                h(MiniPlayer, { player: player }),
                h(AccountMenu, { user: user }))),
        h("div", { class: path === "/" || path.startsWith("/?") ? "layout layout-feed" : "layout" },
            h("nav", { class: "sidenav", "aria-label": "\u0420\u0430\u0437\u0434\u0435\u043B\u044B" },
                h("a", { href: `/u/${user.username}`, "aria-label": "\u041C\u043E\u0439 \u043F\u0440\u043E\u0444\u0438\u043B\u044C", "aria-current": active(`/u/${user.username}`) ? "page" : undefined },
                    h(Icon, { name: "profile" }),
                    h("span", null, "\u041F\u0440\u043E\u0444\u0438\u043B\u044C")),
                groups.map(([title, items]) => (h("section", { key: title, class: "nav-group" },
                    h("h2", { class: "nav-title" }, title),
                    h("ul", null, items.map(([href, label]) => h("li", { key: href }, link(href, label))))))),
                h("ul", null,
                    h("li", null,
                        h("button", { class: "link", onClick: () => logout() }, "\u0412\u044B\u0439\u0442\u0438"))),
                h(SidebarFooter, null)),
            h("nav", { class: "tabbar", "aria-label": "\u0413\u043B\u0430\u0432\u043D\u044B\u0435 \u0440\u0430\u0437\u0434\u0435\u043B\u044B" },
                tabs.map(([href, label, full]) => link(href, label, "tab", full)),
                h("button", { class: more ? "tab active" : "tab", "aria-expanded": more, onClick: () => setMore(!more) }, "\u0415\u0449\u0451")),
            more && (h("div", { class: "more-sheet", role: "dialog", "aria-label": "\u0412\u0441\u0435 \u0440\u0430\u0437\u0434\u0435\u043B\u044B", onClick: (e) => { if (e.target.closest("a"))
                    setMore(false); } },
                groups.map(([title, items]) => (h("section", { key: title },
                    h("h2", { class: "nav-title" }, title),
                    h("div", { class: "more-grid" }, items.map(([href, label]) => link(href, label, "more-item")))))),
                h("button", { class: "ghost", onClick: () => logout() }, "\u0412\u044B\u0439\u0442\u0438"),
                h(SidebarFooter, null))),
            h("main", { class: "content", id: "main" },
                !user.email_verified && h(VerifyBanner, { email: user.email }),
                (musicLoaded || musicActive) && h("div", { hidden: !musicActive },
                    h(MusicPage, { key: user.id, userId: user.id, active: musicActive, onPlayerChange: setPlayer })),
                routes(path, () => setUnread(0), user.id)),
            (path === "/" || path.startsWith("/?")) && h(FeedAside, null))));
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
setupNative(); // в приложении для Android — фоновая запись трека
render(h(App, null), document.getElementById("app"));
if ("serviceWorker" in navigator) {
    navigator.serviceWorker.register("/sw.js").catch((e) => console.warn("service worker", e));
}
