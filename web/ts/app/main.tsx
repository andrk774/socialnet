import { NotificationMenu } from "./notificationmenu.js";
import { NearbyPage } from "./nearby.js";
import { StatisticsPage } from "./statistics.js";
import { CommentArchivePage } from "./commentarchive.js";
import { CallsPage, CallManager } from "./calls.js";
import { DevelopersPage } from "./developers.js";
import { SettingsHub } from "./settingshub.js";
import { hiddenSections } from "./navprefs.js";
import { HelpPage, InfoPage, LegalIndex, SidebarFooter } from "./helpcenter.js";
import { MusicPage } from "./music.js";
import { MiniPlayer, MiniPlayerState } from "./miniplayer.js";
import { PhotosPage } from "./photos.js";
import { h, Fragment, render, ComponentChildren } from "../vendor/preact/index.js";
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

function routes(path: string, onRead: () => void, myId: number): ComponentChildren {
  if (match("/nearby", path)) return <NearbyPage />;
  let p;
  if (match("/help",path)) return <HelpPage/>;
  if (match("/legal-info",path)) return <LegalIndex/>;
  for (const kind of ["about","authors","business","safety"]) if(match(`/${kind}`,path)) return <InfoPage kind={kind}/>;
  if (path === "/" || path.startsWith("/?")) return <FeedPage />;
  if ((p = match("/u/:name", path))) return <ProfilePage username={p.name} />;
  if ((p = match("/p/:id", path))) return <PostPage id={+p.id} />;
  if ((p = match("/tag/:tag", path))) return <TagPage tag={p.tag} />;
  if (match("/im", path)) return <ConversationsPage />;
  if ((p = match("/im/u/:id", path))) return <DirectPage userId={+p.id} />;
  if ((p = match("/im/:id", path))) return <ChatPage id={+p.id} />;
  if (match("/notifications", path)) return <NotificationsPage onRead={onRead} />;
  if (match("/communities", path)) return <CommunitiesPage />;
  if (match("/photos", path)) return <PhotosPage />;
  if (match("/hikes/new", path)) return <HikeForm />;
  if (match("/hikes", path)) return <HikesPage />;
  if (match("/companions", path)) return <CompanionsPage />;
  if ((p = match("/events/:id", path))) return <EventPage id={+p.id} />;
  if ((p = match("/events/:id/print", path))) return <PrintPage eventId={+p.id} />;
  if ((p = match("/routes/:id/print", path))) return <PrintPage routeId={p.id} />;
  if (match("/offline", path)) return <OfflinePage />;
  if (match("/record", path)) return <RecordPage />;
  if (match("/qr-login", path)) return <QRApprovePage />;
  if (match("/articles", path)) return <ArticlesPage />;
  if (match("/rides", path)) return <RidesPage />;
  if (match("/feeds", path)) return <AntennasPage />;
  if (match("/plus", path)) return <PlusPage />;
  if (match("/fundraisers", path)) return <FundraisersPage />;
  if (match("/articles/new", path)) return <ArticleEditor />;
  if ((p = match("/articles/:id/edit", path))) return <ArticleEditor id={+p.id} />;
  if ((p = match("/articles/:id", path))) return <ArticlePage id={+p.id} />;
  if ((p = match("/c/:slug", path))) return <CommunityPage slug={p.slug} />;
  if (match("/music", path)) return null;
  if (match("/videos", path)) return <VideosPage userId={myId} />;
  if (match("/wallet", path)) return <WalletPage />;
  if (match("/statistics", path)) return <StatisticsPage />;
  if (match("/calls", path)) return <CallsPage />;
  if (match("/developers", path)) return <DevelopersPage />;
  if (match("/help/comments", path)) return <CommentArchivePage />;
  if (match("/settings", path)) return <SettingsHub />;
  if (match("/search", path)) return <SearchPage />;
  if (match("/bookmarks", path)) return <BookmarksPage />;
  if (match("/documents", path)) return <DocumentsPage />;
  if (match("/scheduled", path)) { const c = query("community"); return <ScheduledPage communityId={c ? +c : undefined} />; }
  if (match("/admin", path)) return <AdminPage />;
  if (match("/oauth/authorize", path)) return <OAuthPage />;
  return <section><h1>Страница не найдена</h1><p><a href="/">Вернуться в ленту</a></p></section>;
}

// Разделы по группам: на компьютере — боковое меню с заголовками, на
// телефоне — нижняя панель из главных разделов и экран «Ещё».
type NavItem = [string, string];
const navGroups: [string, NavItem[]][] = [
  ["Общение", [["/", "Лента"], ["/feeds", "Мои ленты"], ["/im", "Сообщения"], ["/nearby", "Люди рядом"], ["/calls", "Звонки"], ["/notifications", "Уведомления"], ["/communities", "Сообщества"]]],
  ["Походы", [["/hikes", "Походы"], ["/record", "Записать трек"], ["/companions", "Попутчики"], ["/rides", "Подвоз"], ["/articles", "Статьи"], ["/fundraisers", "Сборы"], ["/offline", "Офлайн"]]],
  ["Моё", [["/photos", "Фото"], ["/music", "Музыка"], ["/videos", "Видео"], ["/bookmarks", "Закладки"], ["/documents", "Документы"], ["/scheduled", "Отложенные"],
    ["/wallet", "Кошелёк"], ["/plus", "Плюс"], ["/settings", "Настройки"]]],
];
// короткие подписи для узкой панели; полные — в aria-label
const tabs: [string, string, string][] = [["/", "Лента", "Лента"], ["/hikes", "Походы", "Походы"], ["/im", "Чаты", "Сообщения"], ["/notifications", "Новое", "Уведомления"]];

function Shell() {
  const path = usePath();
  const user = useMe();
  const [unread, setUnread] = useState(0);
  const [q, setQ] = useState("");
  const [more, setMore] = useState(false);
  const musicActive=path.split("?")[0]==="/music";
  const [musicLoaded,setMusicLoaded]=useState(musicActive);
  const [player,setPlayer]=useState<MiniPlayerState|null>(null);
  useEffect(()=>{if(musicActive)setMusicLoaded(true);},[musicActive]);
  const [hidden,setHidden] = useState<string[]>([]);
  useEffect(()=>{const update=()=>setHidden(user?hiddenSections(user.id):[]);update();window.addEventListener("sn-navigation",update);return()=>window.removeEventListener("sn-navigation",update);},[user?.id]);
  useEffect(() => setMore(false), [path]);

  const countUnread = () => get<{ unread: number }>("/notifications?limit=1").then((r) => setUnread(r.unread), () => {});
  useEffect(() => {
    countUnread();
    return on("notification", () => setUnread((n) => n + 1));
  }, []);

  if (!user) return <Loading />;
  if (user.consent_required && !path.startsWith("/settings")) return <ConsentGate />;
  const active = (href: string) => (href === "/" ? path === "/" : path.startsWith(href));
  const allGroups: [string, NavItem[]][] = user.role === "user" ? navGroups
    : [...navGroups.slice(0, 2), [navGroups[2][0], [...navGroups[2][1], ["/admin", user.role === "admin" ? "Админка" : "Модерация"]]]];
  const groups: [string,NavItem[]][] = allGroups.map(([title,items])=>[title,items.filter(([url])=>!hidden.includes(url))]);
  const link = (href: string, label: string, cls?: string, full?: string) => (
    <a key={href} href={href} class={cls} aria-label={full && full !== label ? (href === "/notifications" && unread > 0 ? `${full}, новых: ${unread}` : full) : undefined} aria-current={active(href) ? "page" : undefined}>
      <Icon name={navIcon(href)} /><span>{label}</span>{href === "/notifications" && unread > 0 && <span class="badge" aria-label={`новых: ${unread}`}>{unread}</span>}
    </a>
  );
  return (
    <div class="shell">
      <CallManager />
      <header class="topbar"><div class="topbar-in">
        <a href="/" class="wordmark" aria-label="socialnet — лента"><span class="brand-mark" aria-hidden="true">s</span>socialnet</a>
        <form class="search" role="search" onSubmit={(e) => { e.preventDefault(); navigate(`/search?q=${encodeURIComponent(q)}`); }}>
          <label class="sr-only" for="q">Поиск</label>
          <Icon name="search" /><input id="q" type="search" placeholder="Поиск: походы, маршруты, люди" value={q} onInput={(e) => setQ((e.target as HTMLInputElement).value)} />
        </form>
        <div class="topbar-actions"><NotificationMenu unread={unread} onRead={countUnread}/><a href="/music" class="header-action" aria-label="Открыть музыку"><Icon name="music" /></a></div>
        <MiniPlayer player={player}/>
        <AccountMenu user={user} />
      </div></header>
      <div class={path === "/" || path.startsWith("/?") ? "layout layout-feed" : "layout"}>
      <nav class="sidenav" aria-label="Разделы">
        <a href={`/u/${user.username}`} aria-label="Мой профиль" aria-current={active(`/u/${user.username}`) ? "page" : undefined}><Icon name="profile" /><span>Профиль</span></a>
        {groups.map(([title, items]) => (
          <section key={title} class="nav-group">
            <h2 class="nav-title">{title}</h2>
            <ul>{items.map(([href, label]) => <li key={href}>{link(href, label)}</li>)}</ul>
          </section>
        ))}
        <ul><li><button class="link" onClick={() => logout()}>Выйти</button></li></ul>
        <SidebarFooter/>
      </nav>
      <nav class="tabbar" aria-label="Главные разделы">
        {tabs.map(([href, label, full]) => link(href, label, "tab", full))}
        <button class={more ? "tab active" : "tab"} aria-expanded={more} onClick={() => setMore(!more)}>Ещё</button>
      </nav>
      {more && (
        <div class="more-sheet" role="dialog" aria-label="Все разделы" onClick={(e) => { if ((e.target as HTMLElement).closest("a")) setMore(false); }}>
          {groups.map(([title, items]) => (
            <section key={title}><h2 class="nav-title">{title}</h2><div class="more-grid">{items.map(([href, label]) => link(href, label, "more-item"))}</div></section>
          ))}
          <button class="ghost" onClick={() => logout()}>Выйти</button>
          <SidebarFooter/>
        </div>
      )}
      <main class="content" id="main">
        {!user.email_verified && <VerifyBanner email={user.email} />}
        {(musicLoaded||musicActive)&&<div hidden={!musicActive}><MusicPage key={user.id} userId={user.id} active={musicActive} onPlayerChange={setPlayer}/></div>}
        {routes(path, () => setUnread(0), user.id)}
      </main>
      {(path === "/" || path.startsWith("/?")) && <FeedAside />}
      </div>
    </div>
  );
}

function VerifyBanner({ email }: { email: string }) {
  const [sent, setSent] = useState(false);
  const [error, setError] = useState<unknown>(null);
  return (
    <div class="banner" role="status">
      <p>Подтвердите почту {email}: без этого недоступны кошелёк и покупки.</p>
      {sent ? <p class="muted">Письмо отправлено. Проверьте почту и папку «Спам».</p> :
        <button class="ghost" onClick={() => post("/users/me/email/verification").then(() => setSent(true), setError)}>Отправить письмо ещё раз</button>}
      <ErrorLine error={error} />
    </div>
  );
}

function App() {
  const [state, setState] = useState<"loading" | "in" | "out">("loading");
  const lp = usePath().split("?")[0];
  useEffect(() => {
    const off = onAuthChange(async (inNow) => {
      if (inNow) { await loadMe().catch(() => {}); startRealtime(); setState("in"); }
      else { stopRealtime(); setMe(null); setState("out"); }
    });
    // сеанс восстанавливается из HttpOnly-cookie
    refresh().then((ok) => { if (!ok && !signedIn()) setState("out"); });
    return () => { off(); };
  }, []);
  // ссылки из писем и восстановление пароля — без входа
  if (lp === "/verify-email") return <VerifyEmailPage />;
  if (lp === "/reset-password") return <ResetPasswordPage />;
  if (lp === "/forgot-password" && state !== "in") return <ForgotPasswordPage />;
  if (state === "loading") return <Loading />;
  // без сети войти нельзя, но сохранённые маршруты доступны
  if (state === "out" && (!navigator.onLine || location.pathname === "/offline")) return <main class="content offline-only"><OfflinePage /><p><a href="/">Войти</a></p></main>;
  if (state === "in") return <Shell />;
  // гость на главной — форма входа и витрина; по ссылке вглубь — только вход
  return lp === "/" ? <><AuthPage /><Showcase /></> : <AuthPage />;
}

setupNative(); // в приложении для Android — фоновая запись трека
render(<App />, document.getElementById("app")!);

if ("serviceWorker" in navigator) {
  navigator.serviceWorker.register("/sw.js").catch((e) => console.warn("service worker", e));
}
