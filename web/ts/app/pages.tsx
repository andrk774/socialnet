import { h, Fragment } from "../vendor/preact/index.js";
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { Page, Post, UserCard, del, fullName, get, post, upload } from "./api.js";
import { Composer, Emoji, PostList, PostView, communityEmoji } from "./posts.js";
import { query } from "./router.js";
import { Avatar, Empty, ErrorLine, Loading, Time, UserLink, duration, useLoad } from "./ui.js";
import { DeleteAccount } from "./legal.js";

// ---------- лента, запись, хэштег ----------

export function FeedPage() {
  return (
    <section>
      <h1 class="sr-only">Лента</h1>
      <PostList path="/feed" empty="Лента пустая. Добавьте друзей или вступите в сообщество — их записи появятся здесь."
        header={(add) => <Composer onPosted={add} />} />
    </section>
  );
}

export function PostPage({ id }: { id: number }) {
  const p = useLoad(() => get<Post>(`/posts/${id}`), [id]);
  if (p.error) return <ErrorLine error={p.error} />;
  return p.data ? <PostView initial={p.data} /> : <Loading />;
}

export function TagPage({ tag }: { tag: string }) {
  return (
    <section>
      <h1>#{tag}</h1>
      <PostList path={`/hashtags/${encodeURIComponent(tag)}/posts`} empty="С этим хэштегом записей нет." />
    </section>
  );
}

// ---------- уведомления ----------

interface Notification { id: number; type: string; actor?: UserCard; post_id?: number; event_id?: number; ride_id?: number; created_at: string; read: boolean }
const notifText: Record<string, string> = {
  like: "оценил(а) вашу запись", comment: "прокомментировал(а) запись", reply: "ответил(а) на комментарий",
  mention: "упомянул(а) вас", friend_request: "хочет добавить вас в друзья", friend_accept: "принял(а) заявку в друзья",
  gift: "отправил(а) вам подарок", transfer: "перевёл(а) вам монеты", donation: "поддержал(а) сообщество", join_request: "просит вступить в сообщество", video_ready: "Видео обработано и опубликовано",
  hike_application: "подал(а) заявку в поход", hike_approved: "принял(а) вашу заявку в поход", hike_rejected: "отклонил(а) заявку в поход",
  hike_removed: "убрал(а) вас из похода", event_seat: "Освободилось место — вы в походе",
  hike_control_soon: "Через час контрольный срок похода — отметьте возвращение", hike_alert_sent: "Контрольный срок прошёл: доверенным контактам отправлены письма",
  hike_returned: "отметил(а), что группа вернулась",
  ride_request: "просится в вашу машину", ride_accepted: "берёт вас в поездку", ride_declined: "не сможет взять вас в поездку",
  ride_left: "передумал(а) ехать с вами", ride_canceled: "отменил(а) поездку",
};

export function NotificationsPage({ onRead }: { onRead: () => void }) {
  const list = useLoad(() => get<Page<Notification> & { unread: number }>("/notifications?limit=50"), []);
  useEffect(() => { if (list.data?.unread) post("/notifications/read", {}).then(onRead, () => {}); }, [list.data]);
  if (list.error) return <ErrorLine error={list.error} />;
  if (!list.data) return <Loading />;
  return (
    <section>
      <h1>Уведомления</h1>
      {list.data.items.length === 0 ? <Empty>Новых событий нет.</Empty> : (
        <ul class="notif-list">
          {list.data.items.map((n) => (
            <li key={n.id} class={n.read ? "" : "unread"}>
              {n.actor && <Avatar user={n.actor} size={32} />}
              <span>
                {n.actor && <UserLink user={n.actor} />} {n.post_id ? <a href={`/p/${n.post_id}`}>{notifText[n.type] ?? n.type}</a> : n.event_id ? <a href={`/events/${n.event_id}`}>{notifText[n.type] ?? n.type}</a>
                  : n.ride_id ? <a href={`/rides#ride-${n.ride_id}`}>{notifText[n.type] ?? n.type}</a> : notifText[n.type] ?? n.type}
                <br /><Time iso={n.created_at} />
              </span>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

// ---------- сообщества ----------

interface Community { id: number; slug: string; name: string; description: string; members_count: number; my_role: string; visibility: "open" | "closed" }

export function CommunitiesPage() {
  const list = useLoad(() => get<Page<Community>>("/communities?limit=50"), []);
  const [f, setF] = useState({ name: "", slug: "", visibility: "open" });
  const [error, setError] = useState<unknown>(null);
  async function create(e: Event) {
    e.preventDefault();
    try { await post("/communities", f); setF({ name: "", slug: "", visibility: "open" }); list.reload(); } catch (err) { setError(err); }
  }
  return (
    <section>
      <h1>Сообщества</h1>
      {list.error ? <ErrorLine error={list.error} /> : !list.data ? <Loading /> : list.data.items.length === 0 ? <Empty>Вы пока не состоите в сообществах. Найдите их через поиск.</Empty> : (
        <ul class="plain-list">
          {list.data.items.map((c) => <li key={c.id}><a href={`/c/${c.slug}`}>{c.name}</a> <span class="muted">{c.members_count} уч.</span></li>)}
        </ul>
      )}
      <details class="create">
        <summary>Создать сообщество</summary>
        <form onSubmit={create} class="stack">
          <label>Название<input value={f.name} required maxLength={100} onInput={(e) => setF({ ...f, name: (e.target as HTMLInputElement).value })} /></label>
          <label>Адрес<input value={f.slug} required pattern="[A-Za-z0-9_]{3,32}" title="3–32 символа: латиница, цифры, _" onInput={(e) => setF({ ...f, slug: (e.target as HTMLInputElement).value })} /></label>
          <label>Кто видит записи
            <select value={f.visibility} onChange={(e) => setF({ ...f, visibility: (e.target as HTMLSelectElement).value })}>
              <option value="open">Все</option><option value="closed">Только участники (вступление по заявке)</option>
            </select>
          </label>
          <ErrorLine error={error} />
          <button class="primary">Создать</button>
        </form>
      </details>
    </section>
  );
}

export function CommunityPage({ slug }: { slug: string }) {
  const c = useLoad(() => get<Community>(`/communities/by-slug/${encodeURIComponent(slug)}`), [slug]);
  const [msg, setMsg] = useState("");
  const [error, setError] = useState<unknown>(null);
  if (c.error) return <ErrorLine error={c.error} />;
  if (!c.data) return <Loading />;
  const com = c.data;
  async function toggle() {
    setError(null);
    try {
      if (com.my_role) { await post(`/communities/${com.id}/leave`); setMsg(""); }
      else {
        const r = await post<{ status?: string } | null>(`/communities/${com.id}/join`);
        setMsg(r && r.status === "requested" ? "Заявка отправлена. Администраторы рассмотрят её." : "");
      }
      c.reload();
    } catch (e) { setError(e); }
  }
  const canPost = ["editor", "admin", "owner"].includes(com.my_role);
  return (
    <section>
      <header class="community-head">
        <h1>{com.name}</h1>
        <p class="muted">{com.members_count} участников{com.visibility === "closed" ? ", закрытое" : ""}</p>
        {com.description && <p>{com.description}</p>}
        {com.my_role !== "owner" && <button class={com.my_role ? "ghost" : "primary"} onClick={toggle}>{com.my_role ? "Выйти" : com.visibility === "closed" ? "Подать заявку" : "Вступить"}</button>}
        {canPost && <a class="ghost-link" href={`/scheduled?community=${com.id}`}>Отложенные записи сообщества</a>}
        {msg && <p class="note">{msg}</p>}
        <ErrorLine error={error} />
        {["admin", "owner"].includes(com.my_role) && <EmojiManager communityId={com.id} />}
      </header>
      <PostList path={`/communities/${com.id}/posts`} empty={com.visibility === "closed" && !com.my_role ? "Записи видят только участники." : "Записей пока нет."}
        header={(add) => (canPost ? <Composer communityId={com.id} onPosted={add} /> : null)} />
    </section>
  );
}

// EmojiManager — свои эмодзи сообщества (администраторам).
function EmojiManager({ communityId }: { communityId: number }) {
  const [list, setList] = useState<Emoji[] | null>(null);
  const [code, setCode] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [error, setError] = useState<unknown>(null);
  const load = (fresh = false) => communityEmoji(communityId, fresh).then(setList);
  useEffect(() => { load(); }, [communityId]);
  const add = async (e: Event) => {
    e.preventDefault();
    if (!file) return;
    setError(null);
    try {
      const m = await upload<{ id: string }>("/media", file);
      await post(`/communities/${communityId}/emoji`, { shortcode: code, media_id: m.id });
      setCode(""); setFile(null); load(true);
    } catch (err) { setError(err); }
  };
  return (
    <details class="create emoji-manager">
      <summary>Эмодзи сообщества{list ? ` (${list.length})` : ""}</summary>
      {list && list.length > 0 && <ul class="emoji-list">
        {list.map((e) => <li key={e.id}><img class="emoji" src={e.url} alt="" width={28} height={28} /> :{e.shortcode}:
          <button class="link" onClick={() => del(`/communities/${communityId}/emoji/${e.id}`).then(() => load(true), setError)}>Удалить</button></li>)}
      </ul>}
      <form class="row" onSubmit={add}>
        <label>Код<input value={code} required pattern="[a-z0-9_]{2,32}" title="2–32 символа: a–z, 0–9, _" onInput={(e) => setCode((e.target as HTMLInputElement).value)} placeholder="gora" /></label>
        <label>Картинка<input type="file" accept="image/png,image/jpeg" required onChange={(e) => setFile((e.target as HTMLInputElement).files?.[0] ?? null)} /></label>
        <button class="primary">Добавить</button>
      </form>
      <p class="hint">Участники смогут реагировать этими эмодзи на записи сообщества.</p>
      <ErrorLine error={error} />
    </details>
  );
}

// ---------- кошелёк ----------

interface Tx { id: number; kind: string; direction: "in" | "out"; amount: number; counterparty: { name: string }; memo: string; created_at: string }
interface Pack { id: string; coins: number; price: string }
const txKind: Record<string, string> = { transfer: "Перевод", gift: "Подарок", donation: "Пожертвование", reward: "Бонус", mint: "Начисление", purchase: "Покупка", payout: "Вывод", refund: "Возврат платежа", debt: "Погашение долга" };

export function WalletPage() {
  const w = useLoad(() => get<{ balance: number; debt: number; transactions: Page<Tx> }>("/wallet"), []);
  const packs = useLoad(() => get<{ enabled: boolean; items: Pack[] }>("/wallet/packages"), []);
  const [error, setError] = useState<unknown>(null);
  const [status, setStatus] = useState("");
  const paymentId = query("payment");

  useEffect(() => {
    if (!paymentId) return;
    // вернулись со страницы оплаты: сервер сам сверит статус с ЮKassa
    get<{ status: string; coins: number }>(`/wallet/purchases/${encodeURIComponent(paymentId)}`).then((p) => {
      setStatus(p.status === "succeeded" ? `Готово: начислено ${p.coins} монет.` : p.status === "pending" ? "Платёж обрабатывается. Монеты придут в течение минуты." : "Платёж не прошёл.");
      w.reload();
    }, setError);
  }, [paymentId]);

  async function buy(id: string) {
    setError(null);
    try {
      const p = await post<{ confirmation_url: string }>("/wallet/purchases", { package_id: id });
      const u = new URL(p.confirmation_url);
      if (u.protocol === "https:") location.assign(u.href); // страница оплаты ЮKassa
    } catch (e) { setError(e); }
  }

  async function bonus() {
    setError(null);
    try { await post("/wallet/daily-bonus"); w.reload(); } catch (e) { setError(e); }
  }

  return (
    <section>
      <h1>Кошелёк</h1>
      {w.data && <p class="balance"><strong>{w.data.balance.toLocaleString("ru")}</strong> монет</p>}
      {w.data && w.data.debt > 0 && (
        <p class="banner" role="status">
          За возвращённую покупку осталось погасить {w.data.debt} монет — они уже были потрачены.
          Пока долг не погашен, переводы и подарки недоступны; новые монеты сначала идут в счёт долга.
        </p>
      )}
      {status && <p class="note" role="status">{status}</p>}
      <ErrorLine error={error} />
      <div class="row"><button class="ghost" onClick={bonus}>Забрать ежедневный бонус</button></div>
      {packs.data?.enabled && (
        <Fragment>
          <h2>Купить монеты</h2>
          <ul class="packs">
            {packs.data.items.map((p) => <li key={p.id}><button class="pack" onClick={() => buy(p.id)}><strong>{p.coins}</strong> монет <span>{p.price}</span></button></li>)}
          </ul>
          <p class="muted">Оплата через ЮKassa. Чек придёт на подтверждённую почту.</p>
        </Fragment>
      )}
      <h2>История</h2>
      {!w.data ? <Loading /> : w.data.transactions.items.length === 0 ? <Empty>Операций пока не было.</Empty> : (
        <table class="tx">
          <thead><tr><th scope="col">Операция</th><th scope="col">С кем</th><th scope="col" class="num">Сумма</th><th scope="col">Когда</th></tr></thead>
          <tbody>
            {w.data.transactions.items.map((t) => (
              <tr key={t.id}>
                <td>{txKind[t.kind] ?? t.kind}{t.memo && <span class="muted"> · {t.memo}</span>}</td>
                <td>{t.counterparty.name}</td>
                <td class={t.direction === "in" ? "num in" : "num out"}>{t.direction === "in" ? "+" : "−"}{t.amount}</td>
                <td><Time iso={t.created_at} /></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}

// ---------- настройки: сеансы и приложения ----------

interface Session { id: string; user_agent: string; ip: string | null; last_active: string; current: boolean }
interface AuthorizedApp { app_id: number; name: string; scopes: string[]; granted_at: string }

export function SettingsPage() {
  const sessions = useLoad(() => get<{ items: Session[] }>("/sessions"), []);
  const apps = useLoad(() => get<{ items: AuthorizedApp[] }>("/users/me/authorized-apps"), []);
  const [error, setError] = useState<unknown>(null);
  const act = (fn: () => Promise<unknown>, then: () => void) => fn().then(then, setError);
  return (
    <section class="settings">
      <h1>Безопасность</h1>
      <ErrorLine error={error} />
      <h2>Где выполнен вход</h2>
      {!sessions.data ? <Loading /> : (
        <ul class="plain-list">
          {sessions.data.items.map((s) => (
            <li key={s.id}>
              <span>{s.user_agent || "Неизвестное устройство"}{s.current && <strong> — это устройство</strong>}<br />
                <span class="muted">{s.ip ?? ""} <Time iso={s.last_active} /></span></span>
              {!s.current && <button class="ghost" onClick={() => act(() => del(`/sessions/${s.id}`), sessions.reload)}>Завершить</button>}
            </li>
          ))}
        </ul>
      )}
      <button class="ghost" onClick={() => act(() => post("/sessions/revoke-others"), sessions.reload)}>Завершить все, кроме этого</button>
      <h2>Приложения с доступом</h2>
      {!apps.data ? <Loading /> : apps.data.items.length === 0 ? <Empty>Сторонним приложениям доступ не выдавался.</Empty> : (
        <ul class="plain-list">
          {apps.data.items.map((a) => (
            <li key={a.app_id}>
              <span><strong>{a.name}</strong><br /><span class="muted">{a.scopes.join(", ")}</span></span>
              <button class="ghost" onClick={() => act(() => del(`/users/me/authorized-apps/${a.app_id}`), apps.reload)}>Отозвать доступ</button>
            </li>
          ))}
        </ul>
      )}
      <h2>Документы</h2>
      <ul class="plain-list">
        <li><a href="/legal/terms">Пользовательское соглашение</a></li>
        <li><a href="/legal/privacy">Политика обработки персональных данных</a></li>
        <li><a href="/legal/consent">Согласие на обработку персональных данных</a></li>
      </ul>
      <DeleteAccount />
    </section>
  );
}

// ---------- согласие OAuth ----------

interface Consent { app: { name: string; client_id: string; developer: string }; scopes: { name: string; description: string }[]; redirect_uri: string }

export function OAuthPage() {
  const params = new URLSearchParams(location.search);
  const info = useLoad(() => get<Consent>(`/oauth/authorize?${params.toString()}`), []);
  const [error, setError] = useState<unknown>(null);
  async function decide(approve: boolean) {
    try {
      const body = Object.fromEntries(["response_type", "client_id", "redirect_uri", "scope", "state", "code_challenge", "code_challenge_method"].map((k) => [k, params.get(k) ?? ""]));
      const r = await post<{ redirect_to: string }>("/oauth/authorize", { ...body, approve });
      location.assign(r.redirect_to); // адрес проверен сервером по списку приложения
    } catch (e) { setError(e); }
  }
  if (info.error) return <section class="consent"><h1>Запрос доступа отклонён</h1><ErrorLine error={info.error} /></section>;
  if (!info.data) return <Loading />;
  const d = info.data;
  const host = (() => { try { return new URL(d.redirect_uri).host || d.redirect_uri; } catch { return d.redirect_uri; } })();
  return (
    <section class="consent">
      <h1>{d.app.name} просит доступ к аккаунту</h1>
      <p class="muted">Разработчик — @{d.app.developer}. После подтверждения вы вернётесь на {host}.</p>
      <ul class="scopes">{d.scopes.map((s) => <li key={s.name}>{s.description}</li>)}</ul>
      <p class="muted">Пароль, почта, сеансы и кошелёк приложению недоступны. Отозвать доступ можно в разделе «Безопасность».</p>
      <ErrorLine error={error} />
      <div class="row">
        <button class="primary" onClick={() => decide(true)}>Разрешить</button>
        <button class="ghost" onClick={() => decide(false)}>Отклонить</button>
      </div>
    </section>
  );
}

// ---------- музыка и поиск ----------

interface Track { id: string; title: string; artist: string; duration_sec: number; url: string; in_my_music: boolean }

export function MusicPage({ userId }: { userId: number }) {
  const list = useLoad(() => get<Page<Track>>(`/users/${userId}/music?limit=50`), [userId]);
  const [playing, setPlaying] = useState<Track | null>(null);
  if (list.error) return <ErrorLine error={list.error} />;
  if (!list.data) return <Loading />;
  return (
    <section>
      <h1>Музыка</h1>
      {list.data.items.length === 0 ? <Empty>В «Моей музыке» пока пусто. Добавляйте треки из записей и поиска.</Empty> : (
        <ol class="tracks">
          {list.data.items.map((t) => (
            <li key={t.id}>
              <button class={playing?.id === t.id ? "track-btn on" : "track-btn"} onClick={() => setPlaying(t)} aria-label={`Слушать: ${t.artist} — ${t.title}`}>▶</button>
              <span>{t.artist} — {t.title}</span><span class="muted">{duration(t.duration_sec)}</span>
            </li>
          ))}
        </ol>
      )}
      {playing && <div class="player"><span>{playing.artist} — {playing.title}</span><audio controls autoPlay src={playing.url} /></div>}
    </section>
  );
}

export function SearchPage() {
  const q = query("q") ?? "";
  const [type, setType] = useState("users");
  const res = useLoad(() => (q ? get<{ items: any[] }>(`/search?q=${encodeURIComponent(q)}&type=${type}`) : Promise.resolve({ items: [] })), [q, type]);
  return (
    <section>
      <h1>Поиск{q && `: ${q}`}</h1>
      <div class="tabs" role="tablist">
        {[["users", "Люди"], ["communities", "Сообщества"], ["tracks", "Музыка"]].map(([k, label]) => (
          <button role="tab" aria-selected={type === k} class={type === k ? "tab on" : "tab"} onClick={() => setType(k)}>{label}</button>
        ))}
      </div>
      {!q ? <Empty>Введите запрос в строке поиска.</Empty> : res.error ? <ErrorLine error={res.error} /> : !res.data ? <Loading /> :
        res.data.items.length === 0 ? <Empty>Ничего не нашлось. Попробуйте другое написание.</Empty> : (
          <ul class="plain-list">
            {res.data.items.map((x) =>
              type === "users" ? <li key={x.id}><Avatar user={x} size={32} /> <a href={`/u/${x.username}`}>{fullName(x)}</a> <span class="muted">@{x.username}</span></li> :
              type === "communities" ? <li key={x.id}><a href={`/c/${x.slug}`}>{x.name}</a> <span class="muted">{x.members_count} уч.</span></li> :
              <li key={x.id}>{x.artist} — {x.title} <audio controls preload="none" src={x.url} /></li>)}
          </ul>
        )}
    </section>
  );
}
