import { h } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { Self, fullName, logout } from "./api.js";
import { Avatar } from "./ui.js";

const paths: Record<string, string> = {
  phone: "M6 3H3v3c0 8 7 15 15 15h3v-5l-5-2-2 2c-3-1-5-3-6-6l2-2-2-5z",
  photo: "M4 3h16a1 1 0 0 1 1 1v16a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1zM3 17l6-6 4 4 3-3 5 5M15 7h.01",
  profile: "M20 21v-2a7 7 0 0 0-14 0v2M12 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8",
  feed: "M4 4h16v16H4zM4 9h16M8 13h8M8 17h5",
  message: "M21 11a9 9 0 0 1-9 9H4l-2 2V11a9 9 0 0 1 19 0M7 9h9M7 13h6",
  bell: "M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9M10 21h4",
  people: "M16 21v-2a6 6 0 0 0-12 0v2M10 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8M17 4a4 4 0 0 1 0 7M20 21v-3a5 5 0 0 0-3-4",
  music: "M9 18V5l11-2v13M9 9l11-2M9 18a3 3 0 1 1-3-3c2 0 3 1 3 3M20 16a3 3 0 1 1-3-3c2 0 3 1 3 3",
  bookmark: "M6 3h12v19l-6-4-6 4z",
  video: "M3 4h18v16H3zM10 8l6 4-6 4z",
  upload: "M12 16V3M6 9l6-6 6 6M4 16v5h16v-5",
  file: "M5 2h9l5 5v15H5zM14 2v6h5M8 12h8M8 16h8",
  settings: "M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8M9 3h6l1 3 3 1 2 5-2 5-3 1-1 3H9l-1-3-3-1-2-5 2-5 3-1z",
  search: "M21 21l-5-5M10 18a8 8 0 1 0 0-16 8 8 0 0 0 0 16",
  map: "M3 5l6-3 6 3 6-3v17l-6 3-6-3-6 3zM9 2v17M15 5v17",
  wallet: "M3 5h17v16H3zM3 5V3h14v2M15 11h6v5h-6z",
  qr: "M3 3h6v6H3zM15 3h6v6h-6zM3 15h6v6H3zM15 15h2v2h4v4h-6z",
  theme: "M20 15A9 9 0 0 1 9 4a9 9 0 1 0 11 11",
  exit: "M9 3H3v18h6M8 12h13M17 8l4 4-4 4",
  plus: "M12 5v14M5 12h14",
};
export function Icon({ name }: { name: string }) {
  return <svg class="nav-icon" width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d={paths[name] ?? paths.map} /></svg>;
}
export function navIcon(href: string) {
  if (href === "/calls") return "phone";
  if (href === "/photos") return "photo";
  if (href === "/videos") return "video";
  return ({ "/": "feed", "/im": "message", "/notifications": "bell", "/communities": "people", "/music": "music", "/bookmarks": "bookmark", "/documents": "file", "/settings": "settings", "/wallet": "wallet", "/search": "search", "/articles": "file", "/plus": "plus" } as Record<string, string>)[href] ?? "map";
}

type Theme = "system" | "light" | "dark";
export function AccountMenu({ user }: { user: Self }) {
  const ref = useRef<HTMLDetailsElement>(null);
  const [theme, setTheme] = useState<Theme>(() => {
    try { const saved = localStorage.getItem("sn-theme"); return saved === "light" || saved === "dark" ? saved : "system"; } catch { return "system"; }
  });
  useEffect(() => {
    if (theme === "system") delete document.documentElement.dataset.theme;
    else document.documentElement.dataset.theme = theme;
    try { localStorage.setItem("sn-theme", theme); } catch { /* storage may be unavailable */ }
  }, [theme]);
  useEffect(()=>{const sync=()=>{try{const value=localStorage.getItem("sn-theme");setTheme(value==="light"||value==="dark"?value:"system");}catch{}};window.addEventListener("sn-theme-change",sync);return()=>window.removeEventListener("sn-theme-change",sync);},[]);
  useEffect(() => {
    const outside = (event: PointerEvent) => { if (ref.current && !ref.current.contains(event.target as Node)) ref.current.open = false; };
    const escape = (event: KeyboardEvent) => { if (event.key === "Escape" && ref.current?.open) { ref.current.open = false; ref.current.querySelector("summary")?.focus(); } };
    document.addEventListener("pointerdown", outside); document.addEventListener("keydown", escape);
    return () => { document.removeEventListener("pointerdown", outside); document.removeEventListener("keydown", escape); };
  }, []);
  return <details class="account-menu" ref={ref}>
    <summary aria-label="Меню аккаунта"><Avatar user={user} size={32} /><svg class="account-chevron" width="16" height="16" viewBox="0 0 16 16" aria-hidden="true"><path d="m4 6 4 4 4-4" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" /></svg></summary>
    <div class="account-panel" onClick={(event) => { if ((event.target as HTMLElement).closest("a") && ref.current) ref.current.open = false; }}>
      <a class="account-identity" href={`/u/${user.username}`}><Avatar user={user} size={64} /><strong>{fullName(user)}</strong><span class="muted">@{user.username}</span></a>
      <a class="account-manage" href="/settings">Управление аккаунтом</a>
      <nav aria-label="Аккаунт">
        <a href="/wallet"><Icon name="wallet" />Кошелёк</a>
        <a href="/bookmarks"><Icon name="bookmark" />Мои закладки</a>
        <a href="/settings"><Icon name="settings" />Настройки</a>
        <button onClick={() => setTheme(theme === "system" ? "light" : theme === "light" ? "dark" : "system")}><Icon name="theme" />Тема: {{ system: "Системная", light: "Светлая", dark: "Тёмная" }[theme]}</button>
        <button onClick={() => logout()}><Icon name="exit" />Выйти</button>
      </nav>
    </div>
  </details>;
}

export function FeedAside() {
  return <aside class="feed-aside" aria-label="Навигация по ленте">
    <nav class="aside-card"><a class="aside-selected" href="/">Лента <Icon name="feed" /></a><a href="/feeds">Настроить мои ленты</a><a href="/bookmarks">Сохранённые записи</a><a href="/search">Найти людей и сообщества</a><a href="/scheduled">Отложенные публикации</a></nav>
    <section class="aside-card aside-discover"><span class="muted">Вне ленты</span><h2>Планы на выходные</h2><p>Найдите поход и компанию для нового маршрута.</p><a href="/hikes">Посмотреть походы</a></section>
    <div class="aside-footer"><a href="/legal/terms">Условия использования</a><a href="/legal/privacy">Конфиденциальность</a><span>socialnet</span></div>
  </aside>;
}
