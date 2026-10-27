// Админка: сводка, пользователи, жалобы, тревоги, журнал аудита.
// Модератор видит только жалобы. Паролей здесь нет и быть не может:
// сервер хранит лишь необратимый хэш, а помочь со входом можно письмом
// со ссылкой на сброс — её получает владелец почты, не администратор.
import { h, Fragment } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { Page, UserCard, fullName, get, post, put } from "./api.js";
import { navigate, query } from "./router.js";
import { me } from "./session.js";
import { Avatar, Empty, ErrorLine, Loading, Time, useLoad } from "./ui.js";

interface Stats {
  users: number; users_new_24h: number; users_new_7d: number; active_24h: number; banned: number;
  posts_24h: number; comments_24h: number; messages_24h: number; open_reports: number; open_alerts: number;
  new_vulnerability_reports: number; scheduled_posts_pending: number; documents: number; paid_kopecks_30d: number;
}
interface AdminUser extends UserCard {
  email: string; role: "user" | "moderator" | "admin"; email_verified: boolean; is_bot: boolean;
  banned_at?: string; ban_reason?: string; created_at: string; last_seen_at?: string;
}
interface SecEvent { id: number; type: string; ip: string | null; user_agent: string; created_at: string }
interface Card extends AdminUser {
  two_factor_enabled: boolean; active_sessions: number; posts: number; comments: number; friends: number;
  open_reports_against: number; reports_filed: number; balance: number; debt: number; last_login_at?: string;
  recent_security_events: SecEvent[];
}
interface ReportTarget { title: string; snippet: string; author_id: number; author: string; hidden: boolean; ref?: string }
interface Report { id: number; reporter: UserCard; target_type: string; target_id: number; target_uuid?: string; reason: string; status: string; created_at: string; target?: ReportTarget }
interface Alert { id: number; rule: string; subject: string; severity: "low" | "medium" | "high"; details: Record<string, unknown>; created_at: string; acknowledged_by: number | null }
interface Audit { id: number; actor_id: number; actor_role: string; action: string; target: string; status: number; ip: string; created_at: string }

type Tab = "stats" | "users" | "reports" | "alerts" | "audit";
const tabs: [Tab, string, boolean][] = [
  ["stats", "Сводка", true], ["users", "Пользователи", true], ["reports", "Жалобы", false],
  ["alerts", "Тревоги", true], ["audit", "Журнал", true],
];

export function AdminPage() {
  const isAdmin = me()?.role === "admin";
  const tab = (query("tab") as Tab | null) ?? (isAdmin ? "stats" : "reports");
  const user = query("user");
  if (me()?.role === "user") return <section><h1>Нет доступа</h1></section>;
  return (
    <section class="admin">
      <h1>{isAdmin ? "Администрирование" : "Модерация"}</h1>
      <div class="tabs" role="tablist">
        {tabs.filter(([, , adminOnly]) => isAdmin || !adminOnly).map(([k, label]) => (
          <button key={k} role="tab" aria-selected={tab === k} class={tab === k ? "tab on" : "tab"} onClick={() => navigate(`/admin?tab=${k}`)}>{label}</button>
        ))}
      </div>
      {tab === "stats" && isAdmin && <StatsTab />}
      {tab === "users" && isAdmin && (user ? <UserCardView id={+user} /> : <UsersTab />)}
      {tab === "reports" && <ReportsTab />}
      {tab === "alerts" && isAdmin && <AlertsTab />}
      {tab === "audit" && isAdmin && <AuditTab />}
    </section>
  );
}

function StatsTab() {
  const s = useLoad(() => get<Stats>("/admin/stats"), []);
  if (s.error) return <ErrorLine error={s.error} />;
  if (!s.data) return <Loading />;
  const d = s.data;
  const tiles: [string, number | string, string?][] = [
    ["Пользователей", d.users], ["Новых за сутки", d.users_new_24h], ["Новых за неделю", d.users_new_7d],
    ["Заходили за сутки", d.active_24h], ["Заблокировано", d.banned],
    ["Записей за сутки", d.posts_24h], ["Комментариев за сутки", d.comments_24h], ["Сообщений за сутки", d.messages_24h],
    ["Открытых жалоб", d.open_reports, "/admin?tab=reports"], ["Тревог детектора", d.open_alerts, "/admin?tab=alerts"],
    ["Новых сообщений об уязвимостях", d.new_vulnerability_reports], ["Отложенных записей", d.scheduled_posts_pending],
    ["Документов", d.documents], ["Оплачено за 30 дней", (d.paid_kopecks_30d / 100).toLocaleString("ru", { style: "currency", currency: "RUB" })],
  ];
  return (
    <ul class="tiles">
      {tiles.map(([label, value, href]) => (
        <li key={label} class={href && Number(value) > 0 ? "tile attn" : "tile"}>
          <span class="tile-value">{typeof value === "number" ? value.toLocaleString("ru") : value}</span>
          {href ? <a href={href}>{label}</a> : <span>{label}</span>}
        </li>
      ))}
    </ul>
  );
}

const roleLabel = { user: "пользователь", moderator: "модератор", admin: "администратор" };

function UsersTab() {
  const [q, setQ] = useState("");
  const [applied, setApplied] = useState({ q: "", filter: "all" });
  const [extra, setExtra] = useState<AdminUser[]>([]);
  const list = useLoad(() => {
    setExtra([]);
    return get<Page<AdminUser>>(`/admin/users?limit=50&filter=${applied.filter}&q=${encodeURIComponent(applied.q)}`);
  }, [applied.q, applied.filter]);
  const [cursor, setCursor] = useState<number | null>(null);
  const next = cursor ?? list.data?.next_cursor ?? null;

  async function more() {
    if (!next) return;
    const r = await get<Page<AdminUser>>(`/admin/users?limit=50&filter=${applied.filter}&q=${encodeURIComponent(applied.q)}&before=${next}`);
    setExtra((xs) => [...xs, ...r.items]); setCursor(r.next_cursor ?? 0);
  }
  const all = [...(list.data?.items ?? []), ...extra];
  return (
    <Fragment>
      <form class="row" onSubmit={(e) => { e.preventDefault(); setCursor(null); setApplied({ ...applied, q }); }}>
        <label class="sr-only" for="admin-q">Поиск</label>
        <input id="admin-q" type="search" placeholder="id, почта, имя пользователя или имя" value={q} maxLength={100} onInput={(e) => setQ((e.target as HTMLInputElement).value)} />
        <label class="sr-only" for="admin-filter">Фильтр</label>
        <select id="admin-filter" value={applied.filter} onChange={(e) => { setCursor(null); setApplied({ ...applied, filter: (e.target as HTMLSelectElement).value }); }}>
          <option value="all">Все</option><option value="banned">Заблокированные</option>
          <option value="staff">Модераторы и админы</option><option value="unverified">Почта не подтверждена</option>
        </select>
        <button class="primary">Найти</button>
      </form>
      <ErrorLine error={list.error} />
      {!list.data ? <Loading /> : all.length === 0 ? <Empty>Никого не нашлось.</Empty> : (
        <table class="tx admin-users">
          <thead><tr><th scope="col">Пользователь</th><th scope="col">Почта</th><th scope="col">Роль</th><th scope="col">Регистрация</th></tr></thead>
          <tbody>
            {all.map((u) => (
              <tr key={u.id}>
                <td><a href={`/admin?tab=users&user=${u.id}`}>{fullName(u)}</a> <span class="muted">@{u.username} · #{u.id}</span>
                  {u.banned_at && <span class="pill bad">бан</span>}{u.is_bot && <span class="pill">бот</span>}</td>
                <td>{u.email}{!u.email_verified && <span class="muted"> (не подтв.)</span>}</td>
                <td>{roleLabel[u.role]}</td>
                <td><Time iso={u.created_at} /></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {next ? <button class="ghost more" onClick={more}>Показать ещё</button> : null}
    </Fragment>
  );
}

const eventLabel: Record<string, string> = {
  register: "Регистрация", login: "Вход", login_new_device: "Вход с нового устройства", login_failed: "Неудачный вход",
  "2fa_failed": "Неверный код 2FA", "2fa_enabled": "Включена 2FA", "2fa_disabled": "Отключена 2FA",
  backup_code_used: "Вход по резервному коду", backup_codes_regenerated: "Новые резервные коды",
  password_changed: "Смена пароля", password_reset: "Пароль сброшен по ссылке", admin_password_reset: "Админ отправил ссылку для сброса пароля",
  email_verified: "Почта подтверждена", session_revoked: "Сеанс завершён", sessions_revoked: "Завершены все сеансы",
  refresh_token_reuse: "Повтор refresh-токена (возможна кража)", app_authorized: "Доступ приложению",
  app_revoked: "Доступ приложения отозван", app_created: "Создано приложение", app_secret_rotated: "Новый секрет приложения",
};

function UserCardView({ id }: { id: number }) {
  const card = useLoad(() => get<Card>(`/admin/users/${id}`), [id]);
  const [error, setError] = useState<unknown>(null);
  const [note, setNote] = useState("");
  const run = async (fn: () => Promise<unknown>, done: string) => {
    setError(null); setNote("");
    try { await fn(); setNote(done); card.reload(); } catch (e) { setError(e); }
  };
  if (card.error) return <ErrorLine error={card.error} />;
  if (!card.data) return <Loading />;
  const u = card.data;
  const self = me()?.id === u.id;

  const reset = () => {
    const revoke = confirm(`Отправить ${u.email} письмо со ссылкой для смены пароля?\n\nОК — отправить и завершить все сеансы (если аккаунт мог быть угнан).\nОтмена — ничего не делать.`);
    if (revoke) run(() => post(`/admin/users/${u.id}/password-reset`, { revoke_sessions: true }), "Письмо со ссылкой отправлено, сеансы завершены. Пароль сменит сам владелец почты.");
  };
  const ban = () => {
    const reason = prompt("Причина блокировки (увидит пользователь)");
    if (reason) run(() => post(`/mod/users/${u.id}/ban`, { reason }), "Пользователь заблокирован, его сеансы завершены.");
  };

  return (
    <div class="admin-card">
      <p><a href="/admin?tab=users">‹ Все пользователи</a></p>
      <div class="profile-head">
        <Avatar user={u} size={64} />
        <div>
          <h2>{fullName(u)} <span class="muted">@{u.username} · #{u.id}</span></h2>
          <p class="muted">{u.email} · {u.email_verified ? "почта подтверждена" : "почта не подтверждена"} · {roleLabel[u.role]}
            {u.two_factor_enabled ? " · 2FA включена" : " · без 2FA"}</p>
          {u.banned_at && <p class="error">Заблокирован <Time iso={u.banned_at} />: {u.ban_reason}</p>}
        </div>
      </div>
      <dl class="facts">
        <div><dt>Регистрация</dt><dd><Time iso={u.created_at} /></dd></div>
        <div><dt>Последний вход</dt><dd>{u.last_login_at ? <Time iso={u.last_login_at} /> : "—"}</dd></div>
        <div><dt>Активных сеансов</dt><dd>{u.active_sessions}</dd></div>
        <div><dt>Записей / комментариев</dt><dd>{u.posts} / {u.comments}</dd></div>
        <div><dt>Друзей</dt><dd>{u.friends}</dd></div>
        <div><dt>Жалоб на него (открытых)</dt><dd>{u.open_reports_against}</dd></div>
        <div><dt>Подал жалоб</dt><dd>{u.reports_filed}</dd></div>
        <div><dt>Баланс</dt><dd>{u.balance} монет{u.debt > 0 && `, долг ${u.debt}`}</dd></div>
      </dl>
      {!self && !u.is_bot && (
        <div class="row">
          {!u.banned_at && <button class="ghost" onClick={reset}>Отправить ссылку для сброса пароля</button>}
          <button class="ghost" onClick={() => confirm("Завершить все сеансы и отозвать токены приложений?") && run(() => post(`/admin/users/${u.id}/revoke-sessions`), "Все сеансы завершены.")}>Выйти отовсюду</button>
          {u.banned_at
            ? <button class="ghost" onClick={() => run(() => post(`/mod/users/${u.id}/unban`), "Блокировка снята.")}>Разблокировать</button>
            : <button class="ghost danger" onClick={ban}>Заблокировать</button>}
          <label class="sr-only" for="role">Роль</label>
          <select id="role" value={u.role} onChange={(e) => {
            const role = (e.target as HTMLSelectElement).value;
            if (confirm(`Назначить роль «${roleLabel[role as AdminUser["role"]]}»?`)) run(() => put(`/admin/users/${u.id}/role`, { role }), "Роль изменена.");
            else (e.target as HTMLSelectElement).value = u.role;
          }}>
            <option value="user">Пользователь</option><option value="moderator">Модератор</option><option value="admin">Администратор</option>
          </select>
        </div>
      )}
      <p class="muted">Пароль администратору недоступен: в базе хранится только его необратимый хэш.</p>
      {note && <p class="note" role="status">{note}</p>}
      <ErrorLine error={error} />
      <h3>Журнал безопасности</h3>
      {u.recent_security_events.length === 0 ? <Empty>Событий нет.</Empty> : (
        <ul class="plain-list">
          {u.recent_security_events.map((e) => (
            <li key={e.id}><span>{eventLabel[e.type] ?? e.type}<br /><span class="muted">{e.ip ?? ""} {e.user_agent}</span></span><Time iso={e.created_at} /></li>
          ))}
        </ul>
      )}
    </div>
  );
}

const targetLabel: Record<string, string> = {
  post: "Запись", comment: "Комментарий", user: "Пользователь", community: "Сообщество", track: "Трек", video: "Видео",
  document: "Документ", companion: "Объявление о попутчиках", article: "Статья", route_point: "Точка на маршруте", route: "Маршрут",
};
// путь /mod/{type}/{id}/hide|unhide для каждого типа жалобы
const modPath: Record<string, string> = {
  post: "posts", comment: "comments", community: "communities", track: "tracks", video: "videos", document: "documents",
  companion: "companions", article: "articles", route_point: "route-points", route: "routes",
};

// reportLink — куда смотреть объект целиком; null — отдельной страницы нет.
export function reportLink(r: Report): string | null {
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

function ReportCard({ r }: { r: Report }) {
  const label = targetLabel[r.target_type] ?? r.target_type;
  const link = reportLink(r);
  const t = r.target;
  // маршруты — серверные страницы каталога, не SPA
  const external = r.target_type === "route" || r.target_type === "route_point";
  return (
    <div class="report-target">
      <div>
        {link ? <a href={link} target={external ? "_blank" : undefined} rel={external ? "noopener" : undefined}>{label}</a> : <span>{label}</span>}
        {" "}<span class="muted">{r.target_uuid ?? `#${r.target_id}`}</span>
        {t?.hidden && <span class="badge">{r.target_type === "user" ? "заблокирован" : "скрыто"}</span>}
        {!t && <span class="badge">удалено</span>}
      </div>
      {t?.title && <strong>{t.title}</strong>}
      {t?.snippet && <p class="report-snippet">{t.snippet}</p>}
      {t && r.target_type !== "user" && <span class="muted">автор: <a href={`/admin?tab=users&user=${t.author_id}`}>@{t.author}</a></span>}
    </div>
  );
}

function ReportsTab() {
  const [status, setStatus] = useState("open");
  const list = useLoad(() => get<Page<Report>>(`/mod/reports?status=${status}&limit=50`), [status]);
  const [error, setError] = useState<unknown>(null);
  const resolve = (r: Report, action: "dismiss" | "hide" | "ban") => {
    const note = action === "dismiss" ? "" : prompt(action === "ban" ? "Причина блокировки" : "Комментарий (необязательно)") ?? null;
    if (note === null || (action === "ban" && !note)) return;
    post(`/mod/reports/${r.id}/resolve`, { action, note }).then(list.reload, setError);
  };
  const setHidden = (r: Report, hidden: boolean) =>
    post(`/mod/${modPath[r.target_type]}/${r.target_uuid ?? r.target_id}/${hidden ? "hide" : "unhide"}`).then(list.reload, setError);
  return (
    <Fragment>
      <div class="row">
        <label for="rep-status">Показать</label>
        <select id="rep-status" value={status} onChange={(e) => setStatus((e.target as HTMLSelectElement).value)}>
          <option value="open">Открытые</option><option value="resolved">Решённые</option><option value="dismissed">Отклонённые</option>
        </select>
      </div>
      <ErrorLine error={error ?? list.error} />
      {!list.data ? <Loading /> : list.data.items.length === 0 ? <Empty>Жалоб нет.</Empty> : (
        <ul class="plain-list reports">
          {list.data.items.map((r) => (
            <li key={r.id} data-type={r.target_type}>
              <span>
                <ReportCard r={r} />
                «{r.reason}»<br /><span class="muted">от {fullName(r.reporter)} · <Time iso={r.created_at} /></span>
              </span>
              <span class="row tight">
                {r.status === "open" && <button class="ghost" onClick={() => resolve(r, "dismiss")}>Отклонить</button>}
                {r.status === "open" && r.target_type !== "user" && r.target && !r.target.hidden && <button class="ghost" onClick={() => resolve(r, "hide")}>Скрыть</button>}
                {r.status !== "open" && r.target && r.target_type !== "user" && modPath[r.target_type] && (
                  r.target.hidden
                    ? <button class="ghost" onClick={() => setHidden(r, false)}>Вернуть</button>
                    : <button class="ghost" onClick={() => setHidden(r, true)}>Скрыть</button>
                )}
                {r.status === "open" && r.target && <button class="ghost danger" onClick={() => resolve(r, "ban")}>Бан автора</button>}
              </span>
            </li>
          ))}
        </ul>
      )}
    </Fragment>
  );
}

const ruleLabel: Record<string, string> = {
  login_bruteforce: "Перебор пароля", password_spray: "Перебор по многим аккаунтам", token_reuse: "Повтор refresh-токена",
  admin_burst: "Всплеск действий админа", admin_new_ip: "Админ с нового IP", mint_large: "Крупное начисление монет",
  transfer_burst: "Всплеск переводов",
};

function AlertsTab() {
  const [all, setAll] = useState(false);
  const list = useLoad(() => get<Page<Alert>>(`/admin/alerts?limit=50${all ? "&status=all" : ""}`), [all]);
  const [error, setError] = useState<unknown>(null);
  return (
    <Fragment>
      <label class="row"><input type="checkbox" checked={all} onChange={(e) => setAll((e.target as HTMLInputElement).checked)} /> Показывать разобранные</label>
      <ErrorLine error={error ?? list.error} />
      {!list.data ? <Loading /> : list.data.items.length === 0 ? <Empty>Тревог нет.</Empty> : (
        <ul class="plain-list">
          {list.data.items.map((a) => (
            <li key={a.id}>
              <span><span class={`pill sev-${a.severity}`}>{a.severity}</span> {ruleLabel[a.rule] ?? a.rule} — {a.subject}
                <br /><code class="muted">{JSON.stringify(a.details)}</code></span>
              <span class="row tight"><Time iso={a.created_at} />
                {a.acknowledged_by === null && <button class="ghost" onClick={() => post(`/admin/alerts/${a.id}/ack`).then(list.reload, setError)}>Разобрано</button>}</span>
            </li>
          ))}
        </ul>
      )}
    </Fragment>
  );
}

function AuditTab() {
  const list = useLoad(() => get<Page<Audit>>("/admin/audit?limit=50"), []);
  const [check, setCheck] = useState<{ ok: boolean; checked: number; broken_at: number } | null>(null);
  const [error, setError] = useState<unknown>(null);
  return (
    <Fragment>
      <div class="row">
        <button class="ghost" onClick={() => get<{ ok: boolean; checked: number; broken_at: number }>("/admin/audit/verify").then(setCheck, setError)}>Проверить целостность журнала</button>
        {check && (check.ok
          ? <span role="status">Цепочка цела, проверено записей: {check.checked}.</span>
          : <span class="error" role="alert">Журнал изменён задним числом начиная с записи #{check.broken_at}.</span>)}
      </div>
      <ErrorLine error={error ?? list.error} />
      {!list.data ? <Loading /> : list.data.items.length === 0 ? <Empty>Записей нет.</Empty> : (
        <table class="tx">
          <thead><tr><th scope="col">Когда</th><th scope="col">Кто</th><th scope="col">Действие</th><th scope="col" class="num">Код</th></tr></thead>
          <tbody>
            {list.data.items.map((a) => (
              <tr key={a.id}>
                <td><Time iso={a.created_at} /></td>
                <td><a href={`/admin?tab=users&user=${a.actor_id}`}>#{a.actor_id}</a> <span class="muted">{a.actor_role} · {a.ip}</span></td>
                <td><code>{a.action}</code>{a.target && <span class="muted"> {a.target}</span>}</td>
                <td class="num">{a.status}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </Fragment>
  );
}
