import { h, render, ComponentChildren } from "../vendor/preact/index.js";
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { get, logout, onAuthChange, post, refresh, signedIn } from "./api.js";
import { AuthPage } from "./auth.js";
import { ChatPage, ConversationsPage, DirectPage } from "./messages.js";
import { CommunitiesPage, CommunityPage, FeedPage, MusicPage, NotificationsPage, OAuthPage, PostPage, SearchPage, SettingsPage, TagPage, WalletPage } from "./pages.js";
import { ProfilePage } from "./profile.js";
import { EventPage, HikeForm, HikesPage } from "./hikes.js";
import { CompanionsPage } from "./companions.js";
import { AdminPage } from "./admin.js";
import { BookmarksPage, DocumentsPage, ScheduledPage } from "./library.js";
import { on, startRealtime, stopRealtime } from "./realtime.js";
import { match, navigate, query, usePath } from "./router.js";
import { loadMe, setMe, useMe } from "./session.js";
import { Avatar, ErrorLine, Loading } from "./ui.js";

function routes(path: string, onRead: () => void, myId: number): ComponentChildren {
  let p;
  if (path === "/" || path.startsWith("/?")) return <FeedPage />;
  if ((p = match("/u/:name", path))) return <ProfilePage username={p.name} />;
  if ((p = match("/p/:id", path))) return <PostPage id={+p.id} />;
  if ((p = match("/tag/:tag", path))) return <TagPage tag={p.tag} />;
  if (match("/im", path)) return <ConversationsPage />;
  if ((p = match("/im/u/:id", path))) return <DirectPage userId={+p.id} />;
  if ((p = match("/im/:id", path))) return <ChatPage id={+p.id} />;
  if (match("/notifications", path)) return <NotificationsPage onRead={onRead} />;
  if (match("/communities", path)) return <CommunitiesPage />;
  if (match("/hikes/new", path)) return <HikeForm />;
  if (match("/hikes", path)) return <HikesPage />;
  if (match("/companions", path)) return <CompanionsPage />;
  if ((p = match("/events/:id", path))) return <EventPage id={+p.id} />;
  if ((p = match("/c/:slug", path))) return <CommunityPage slug={p.slug} />;
  if (match("/music", path)) return <MusicPage userId={myId} />;
  if (match("/wallet", path)) return <WalletPage />;
  if (match("/settings", path)) return <SettingsPage />;
  if (match("/search", path)) return <SearchPage />;
  if (match("/bookmarks", path)) return <BookmarksPage />;
  if (match("/documents", path)) return <DocumentsPage />;
  if (match("/scheduled", path)) { const c = query("community"); return <ScheduledPage communityId={c ? +c : undefined} />; }
  if (match("/admin", path)) return <AdminPage />;
  if (match("/oauth/authorize", path)) return <OAuthPage />;
  return <section><h1>Страница не найдена</h1><p><a href="/">Вернуться в ленту</a></p></section>;
}

const nav: [string, string][] = [
  ["/", "Лента"], ["/hikes", "Походы"], ["/companions", "Попутчики"], ["/im", "Сообщения"], ["/notifications", "Уведомления"],
  ["/communities", "Сообщества"], ["/music", "Музыка"], ["/bookmarks", "Закладки"], ["/documents", "Документы"],
  ["/scheduled", "Отложенные"], ["/wallet", "Кошелёк"], ["/settings", "Безопасность"],
];

function Shell() {
  const path = usePath();
  const user = useMe();
  const [unread, setUnread] = useState(0);
  const [q, setQ] = useState("");

  const countUnread = () => get<{ unread: number }>("/notifications?limit=1").then((r) => setUnread(r.unread), () => {});
  useEffect(() => {
    countUnread();
    return on("notification", () => setUnread((n) => n + 1));
  }, []);

  if (!user) return <Loading />;
  const active = (href: string) => (href === "/" ? path === "/" : path.startsWith(href));
  return (
    <div class="shell">
      <header class="topbar"><div class="topbar-in">
        <a href="/" class="wordmark" aria-label="socialnet — лента">socialnet</a>
        <form class="search" role="search" onSubmit={(e) => { e.preventDefault(); navigate(`/search?q=${encodeURIComponent(q)}`); }}>
          <label class="sr-only" for="q">Поиск</label>
          <input id="q" type="search" placeholder="Поиск людей, сообществ, музыки" value={q} onInput={(e) => setQ((e.target as HTMLInputElement).value)} />
        </form>
        <a href={`/u/${user.username}`} class="me" aria-label="Мой профиль"><Avatar user={user} size={32} /></a>
      </div></header>
      <div class="layout">
      <nav class="sidenav" aria-label="Разделы">
        <ul>
          {[...nav, ...(user.role === "admin" ? [["/admin", "Админка"]] : user.role === "moderator" ? [["/admin", "Модерация"]] : [])].map(([href, label]) => (
            <li key={href}>
              <a href={href} aria-current={active(href) ? "page" : undefined}>
                {label}{href === "/notifications" && unread > 0 && <span class="badge" aria-label={`новых: ${unread}`}>{unread}</span>}
              </a>
            </li>
          ))}
          <li><button class="link" onClick={() => logout()}>Выйти</button></li>
        </ul>
      </nav>
      <main class="content" id="main">
        {!user.email_verified && <VerifyBanner email={user.email} />}
        {routes(path, () => setUnread(0), user.id)}
      </main>
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
  useEffect(() => {
    const off = onAuthChange(async (inNow) => {
      if (inNow) { await loadMe().catch(() => {}); startRealtime(); setState("in"); }
      else { stopRealtime(); setMe(null); setState("out"); }
    });
    // сеанс восстанавливается из HttpOnly-cookie
    refresh().then((ok) => { if (!ok && !signedIn()) setState("out"); });
    return () => { off(); };
  }, []);
  if (state === "loading") return <Loading />;
  return state === "in" ? <Shell /> : <AuthPage />;
}

render(<App />, document.getElementById("app")!);
