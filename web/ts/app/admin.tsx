// Админка: сводка, пользователи, жалобы, тревоги, журнал аудита.
// Модератор видит только жалобы. Паролей здесь нет и быть не может:
// сервер хранит лишь необратимый хэш, а помочь со входом можно письмом
// со ссылкой на сброс — её получает владелец почты, не администратор.
import { h, Fragment } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { Page, UserCard, fullName, get, post, put } from "./api.js";
import { navigate, query } from "./router.js";
import { me } from "./session.js";
import { Avatar, Empty, ErrorLine, Loading, Time, useLoad } from "./ui.js";
import { NoticesTab } from "./notices.js";
import { SystemTab } from "./system.js";

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
  birthday: string|null;
  subscription: {active:boolean;expires_at:string|null;auto_renew:boolean};
  two_factor_enabled: boolean; active_sessions: number; posts: number; comments: number; friends: number;
  open_reports_against: number; reports_filed: number; balance: number; debt: number; last_login_at?: string;
  recent_security_events: SecEvent[];
}
interface ReportTarget { title: string; snippet: string; author_id: number; author: string; hidden: boolean; ref?: string }
interface Report { id: number; reporter: UserCard; target_type: string; target_id: number; target_uuid?: string; reason: string; status: string; created_at: string; target?: ReportTarget }
interface Alert { id: number; rule: string; subject: string; severity: "low" | "medium" | "high"; details: Record<string, unknown>; created_at: string; acknowledged_by: number | null }
interface Audit { id: number; actor_id: number; actor_role: string; action: string; target: string; status: number; ip: string; created_at: string }

type Tab = "stats" | "system" | "roles" | "users" | "reports" | "notices" | "alerts" | "audit";
const tabs: [Tab, string, boolean][] = [
  ["system", "Система", true], ["roles", "Роли", true],
  ["stats", "Сводка", true], ["users", "Пользователи", true], ["reports", "Жалобы", false],
  ["notices", "Предупреждения", false], ["alerts", "Тревоги", true], ["audit", "Журнал", true],
];

export function AdminPage() {
  const isAdmin = me()?.role === "admin";
  const tab = (query("tab") as Tab | null) ?? (isAdmin ? "stats" : "reports");
  const user = query("user");
  if (me()?.role === "user") return <section><h1>Нет доступа</h1></section>;
  return (
    <section class="admin">
      <header class="admin-heading"><div><span class="admin-eyebrow">Управление socialnet</span><h1>{isAdmin ? "Администрирование" : "Модерация"}</h1></div><span class={`admin-role-badge role-${isAdmin?"admin":"moderator"}`}>{isAdmin?"Администратор":"Модератор"}</span></header>
      <div class="tabs admin-navigation" role="tablist" aria-label="Разделы администрирования">
        {tabs.filter(([, , adminOnly]) => isAdmin || !adminOnly).map(([k, label]) => (
          <button key={k} role="tab" aria-selected={tab === k} class={tab === k ? "tab on" : "tab"} onClick={() => navigate(`/admin?tab=${k}`)}>{label}</button>
        ))}
      </div>
      {tab === "stats" && isAdmin && <StatsTab />}
      {tab === "system" && isAdmin && <SystemTab />}
      {tab === "roles" && isAdmin && <><section class="admin-role-panel"><div class="admin-section-heading"><div><h2>Роли и права</h2><p class="muted">Выберите пользователя ниже, чтобы открыть его карточку и изменить доступ.</p></div><a class="link" href="/admin?tab=audit">Журнал изменений →</a></div><div class="admin-role-grid">
        <article><span class="admin-role-badge role-user">Пользователь</span><h3>Личный аккаунт</h3><p>Профиль, публикации, сообщения и медиа в пределах настроек доступа.</p></article>
        <article><span class="admin-role-badge role-moderator">Модератор</span><h3>Порядок в сообществе</h3><p>Разбор жалоб и предупреждений. Без доступа к метрикам системы и назначению ролей.</p></article>
        <article><span class="admin-role-badge role-admin">Администратор</span><h3>Управление платформой</h3><p>Пользователи, роли, системные метрики, тревоги и журнал действий.</p></article>
      </div><p class="admin-role-note">Все изменения записываются в журнал. Свою роль изменить нельзя; последний администратор сохраняет доступ.</p></section><UsersTab /></>}
      {tab === "users" && isAdmin && (user ? <UserCardView id={+user} /> : <UsersTab />)}
      {tab === "reports" && <ReportsTab />}
      {tab === "notices" && <NoticesTab />}
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
  const [editing,setEditing] = useState<number|null>(null);
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
    <section class="admin-user-panel" aria-label="Управление пользователями">
      <div class="admin-section-heading"><div><h2>Пользователи</h2><p class="muted">Поиск аккаунта и управление доступом</p></div>{list.data&&<span class="admin-result-count">Показано: {all.length}{next?"+":""}</span>}</div>
      <form class="row admin-search" onSubmit={(e) => { e.preventDefault(); setCursor(null); setApplied({ ...applied, q }); }}>
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
        <div class="admin-table-scroll" tabIndex={0} role="region" aria-label="Список пользователей"><table class="tx admin-users">
          <thead><tr><th scope="col">Пользователь</th><th scope="col">Почта</th><th scope="col">Роль</th><th scope="col">Регистрация</th><th scope="col">Управление</th></tr></thead>
          <tbody>
            {all.map((u) => (
              <tr key={u.id}>
                <td><div class="admin-person"><Avatar user={u} size={38}/><div><a href={`/admin?tab=users&user=${u.id}`}>{fullName(u)}</a><span class="muted">@{u.username} · #{u.id}</span>
                  {u.banned_at && <span class="pill bad">Заблокирован</span>}{u.is_bot && <span class="pill">Бот</span>}</div></div></td>
                <td><span class="admin-email">{u.email}</span>{!u.email_verified && <small class="muted">Не подтверждена</small>}</td>
                <td><span class={`admin-role-badge role-${u.role}`}>{roleLabel[u.role]}</span></td>
                <td><Time iso={u.created_at} /></td>
                <td><button class="ghost" onClick={()=>setEditing(u.id)}>Редактировать</button></td>
              </tr>
            ))}
          </tbody>
        </table></div>
      )}
      {next ? <button class="ghost more" onClick={more}>Показать ещё</button> : null}
      {editing!==null&&<UserEditDialog key={editing} id={editing} onClose={()=>setEditing(null)} onSaved={(id,role)=>{list.set({...list.data!,items:list.data!.items.map(u=>u.id===id?{...u,role}:u)});setExtra(xs=>xs.map(u=>u.id===id?{...u,role}:u));}}/>}
    </section>
  );
}

function UserEditDialog({id,onClose,onSaved}:{id:number;onClose:()=>void;onSaved:(id:number,role:AdminUser["role"])=>void}){
  const dialog=useRef<HTMLDialogElement>(null);
  const card=useLoad(()=>get<Card>(`/admin/users/${id}`),[id]);
  const [role,setRole]=useState<AdminUser["role"]>("user");
  const [busy,setBusy]=useState(false),[error,setError]=useState<unknown>(null),[saved,setSaved]=useState(false);
  useEffect(()=>{dialog.current?.showModal();},[]);
  useEffect(()=>{if(card.data)setRole(card.data.role);},[card.data]);
  const u=card.data,locked=!u||u.id===me()?.id||u.is_bot;
  async function save(e:Event){e.preventDefault();if(!u||busy||locked)return;setBusy(true);setError(null);try{await put(`/admin/users/${id}/role`,{role});onSaved(id,role);card.set({...u,role});setSaved(true);}catch(e){setError(e);}finally{setBusy(false);}}
  return <dialog ref={dialog} class="admin-edit-dialog" aria-label="Редактирование пользователя" onCancel={e=>{if(busy)e.preventDefault();}} onClose={onClose}>
    <form onSubmit={save}><header><div><span class="admin-eyebrow">Управление аккаунтом</span><h2>Карточка пользователя</h2></div><button type="button" class="icon-btn" aria-label="Закрыть карточку" disabled={busy} onClick={()=>dialog.current?.close()}>×</button></header>
    <div class="admin-edit-body"><ErrorLine error={card.error||error}/>{!u&&!card.error&&<Loading/>}{u&&<>
      <div class="admin-edit-person"><Avatar user={u} size={96}/><h3>{fullName(u)}</h3><span class="muted">@{u.username} · #{u.id}</span><span class={`admin-role-badge role-${u.role}`}>{roleLabel[u.role]}</span></div>
      <div class="admin-edit-fields"><label>Имя и фамилия<input readOnly value={fullName(u)}/></label><label>Баланс, монеты<input readOnly value={u.balance.toLocaleString("ru")}/></label><label class="wide">Электронная почта<input readOnly value={u.email}/></label><label class="wide">Роль<select value={role} disabled={locked||busy} onChange={e=>{setRole(e.currentTarget.value as AdminUser["role"]);setSaved(false);}}><option value="user">Пользователь</option><option value="moderator">Модератор</option><option value="admin">Администратор</option></select></label></div>
      <p class="muted">{locked?"Роль собственного аккаунта и служебного бота изменить нельзя.":"Изменение роли применяется после сохранения и записывается в журнал."}</p>
      <div class="admin-edit-metrics"><div><strong>{u.posts}</strong><span>Публикаций</span></div><div><strong>{u.friends}</strong><span>Друзей</span></div><div><strong>{u.open_reports_against}</strong><span>Открытых жалоб</span></div></div>
      <AdminSubscription subscription={u.subscription}/>
      <BalanceAdjustment id={id} balance={u.balance} debt={u.debt} onSaved={()=>card.reload()}/>
      <AdminProfileEditor user={u} onSaved={()=>card.reload()}/>{saved&&<p role="status" class="note">Изменения сохранены.</p>}
    </>}</div><footer>{u&&<a href={`/admin?tab=users&user=${id}`}>Полная карточка</a>}<button type="button" class="ghost" disabled={busy} onClick={()=>dialog.current?.close()}>Закрыть</button><button class="primary" disabled={busy||locked||role===u?.role}>{busy?"Сохраняю…":"Сохранить"}</button></footer></form>
  </dialog>;
}

function BalanceAdjustment({id,balance,debt,onSaved}:{id:number;balance:number;debt:number;onSaved:()=>void}){
 const [direction,setDirection]=useState("add"),[amount,setAmount]=useState(""),[memo,setMemo]=useState("");
 const [key,setKey]=useState(()=>crypto.randomUUID()),[busy,setBusy]=useState(false),[error,setError]=useState<unknown>(null),[note,setNote]=useState("");
 const change=()=>{setKey(crypto.randomUUID());setNote("");setError(null);};
 const n=Number(amount),valid=Number.isInteger(n)&&n>0&&n<=1000000&&memo.trim().length>0;
 async function apply(){if(!valid||busy)return;setBusy(true);setError(null);try{const r=await post<{balance:number}>(`/admin/users/${id}/balance`,{amount:n,direction,memo: memo.trim(),idempotency_key:key});setNote(`Операция выполнена. Баланс: ${r.balance} монет.`);setKey(crypto.randomUUID());setAmount("");setMemo("");onSaved();}catch(e){setError(e);}finally{setBusy(false);}}
 return <fieldset class="admin-balance" disabled={busy}><legend>Изменить баланс</legend><p class="muted">Сейчас: {balance} монет. {debt>0?`Долг: ${debt} монет. Начисление сначала погасит долг.`:"Корректировка появится в кошельке и журнале администратора."}</p><div class="admin-edit-fields"><label>Операция<select value={direction} onChange={e=>{change();setDirection(e.currentTarget.value);}}><option value="add">Начислить</option><option value="remove">Списать</option></select></label><label>Сумма, монеты<input type="number" min={1} max={1000000} step={1} value={amount} onInput={e=>{change();setAmount(e.currentTarget.value);}}/></label><label class="wide">Причина корректировки<input maxLength={160} value={memo} placeholder="Укажите причину" onInput={e=>{change();setMemo(e.currentTarget.value);}}/></label></div><ErrorLine error={error}/>{note&&<p role="status" class="note">{note}</p>}<button type="button" class="primary" disabled={!valid||busy} onClick={apply}>{busy?"Применяю…":direction==="add"?"Начислить монеты":"Списать монеты"}</button></fieldset>;
}

function AdminSubscription({subscription:s}:{subscription:Card["subscription"]}){
 return <section class="admin-subscription"><div><h3>Подписка «Плюс»</h3><span class={s.active?"pill":"muted"}>{s.active?"Активна":s.expires_at?"Истекла":"Не подключена"}</span></div><dl><div><dt>{s.active?"Действует до":"Дата окончания"}</dt><dd>{s.expires_at?<Time iso={s.expires_at}/>:"—"}</dd></div><div><dt>Автопродление</dt><dd>{s.auto_renew?"Включено":"Выключено"}</dd></div></dl></section>;
}

function AdminProfileEditor({user:u,onSaved}:{user:Card;onSaved:()=>void}){
 const [first,setFirst]=useState(u.first_name),[last,setLast]=useState(u.last_name),[email,setEmail]=useState(u.email),[birthday,setBirthday]=useState(u.birthday||"");
 const localDate=(value:string)=>{const d=new Date(value);return new Date(d.getTime()-d.getTimezoneOffset()*60000).toISOString().slice(0,16);};
 const [created,setCreated]=useState(()=>localDate(u.created_at)),[password,setPassword]=useState(""),[repeat,setRepeat]=useState(""),[busy,setBusy]=useState(false),[error,setError]=useState<unknown>(null),[note,setNote]=useState("");
 async function save(pass:boolean){setBusy(true);setError(null);setNote("");try{if(pass){await put(`/admin/users/${u.id}/password`,{password});setPassword("");setRepeat("");setNote("Новый пароль установлен. Старые сеансы завершены.");}else{await put(`/admin/users/${u.id}/profile`,{first_name:first,last_name:last,email,birthday:birthday||null,created_at:created===localDate(u.created_at)?u.created_at:new Date(created).toISOString()});setNote("Данные пользователя сохранены.");}onSaved();}catch(e){setError(e);}finally{setBusy(false);}}
 return <fieldset class="admin-balance" disabled={busy}><legend>Данные пользователя</legend><div class="admin-edit-fields"><label>Имя<input value={first} maxLength={64} onInput={e=>setFirst(e.currentTarget.value)}/></label><label>Фамилия<input value={last} maxLength={64} onInput={e=>setLast(e.currentTarget.value)}/></label><label class="wide">Новый email<input type="email" value={email} maxLength={254} onInput={e=>setEmail(e.currentTarget.value)}/></label><label>Дата рождения<input type="date" value={birthday} onInput={e=>setBirthday(e.currentTarget.value)}/></label><label>Дата создания аккаунта<input type="datetime-local" value={created} onInput={e=>setCreated(e.currentTarget.value)}/></label></div><p class="muted">При смене email подтверждение адреса снимается, старые сеансы и ссылки восстановления отзываются. Время регистрации указано в часовом поясе устройства.</p><button type="button" class="primary" disabled={busy||!first.trim()||!email.trim()||!created} onClick={()=>save(false)}>Сохранить данные</button>{!u.is_bot&&<><h3>Задать новый пароль</h3><div class="admin-edit-fields"><label>Новый пароль<input type="password" autoComplete="new-password" value={password} minLength={8} maxLength={128} onInput={e=>setPassword(e.currentTarget.value)}/></label><label>Повторите пароль<input type="password" autoComplete="new-password" value={repeat} onInput={e=>setRepeat(e.currentTarget.value)}/></label></div><p class="muted">От 8 до 128 символов. Текущий пароль посмотреть нельзя.</p><button type="button" class="ghost danger" disabled={busy||password.length<8||password!==repeat} onClick={()=>save(true)}>Установить новый пароль</button></>}<ErrorLine error={error}/>{note&&<p class="note" role="status">{note}</p>}</fieldset>;
}

const eventLabel: Record<string, string> = {
  register: "Регистрация", login: "Вход", login_new_device: "Вход с нового устройства", login_failed: "Неудачный вход", qr_login_approved: "Подтвердил вход по QR-коду",
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
      <AdminSubscription subscription={u.subscription}/>
      <BalanceAdjustment id={id} balance={u.balance} debt={u.debt} onSaved={()=>card.reload()}/>
      <AdminProfileEditor key={id} user={u} onSaved={()=>card.reload()}/>
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
  document: "Документ", companion: "Объявление о попутчиках", article: "Статья", route_point: "Точка на маршруте", route: "Маршрут", review: "Отзыв",
};
// путь /mod/{type}/{id}/hide|unhide для каждого типа жалобы
const modPath: Record<string, string> = {
  post: "posts", comment: "comments", community: "communities", track: "tracks", video: "videos", document: "documents",
  companion: "companions", article: "articles", route_point: "route-points", route: "routes", review: "reviews",
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
        <div class="table-scroll" tabIndex={0} role="region" aria-label="Audit log"><table class="tx">
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
        </table></div>
      )}
    </Fragment>
  );
}
