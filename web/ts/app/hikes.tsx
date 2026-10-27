// Походы: список ближайших, страница похода с записью, листом ожидания и
// заявками, форма организатора. Маршрут похода — карточка из routes.tsx.
import { Fragment, h } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { del, get, Page, patch, post, Route, saveFile, UserCard } from "./api.js";
import { navigate } from "./router.js";
import { RouteCard } from "./routes.js";
import { Weather } from "./weather.js";
import { Rides } from "./rides.js";
import { useMe } from "./session.js";
import { Avatar, Empty, ErrorLine, Loading, RichText, Time, UserLink, useLoad } from "./ui.js";

export interface HikeEvent {
  id: number; creator_id: number; community?: { id: number; slug: string; name: string }; kind: "event" | "hike";
  title: string; description: string; place: string; starts_at: string; ends_at?: string;
  difficulty: number | null; capacity: number | null; gear: string; route_id: string | null; meeting_point: string; approval: boolean;
  going_count: number; maybe_count: number; pending_count: number; waitlist_count: number;
  my_status: "going" | "maybe" | "pending" | "waitlist" | null; my_waitlist_position?: number;
  control_at: string | null; returned_at: string | null; alert_sent: boolean; created_at: string;
}
interface Contact { id: number; name: string; email: string; created_at: string }
interface Application { user: UserCard; note: string; created_at: string }

export const difficulties = ["", "Лёгкий", "Средний", "Сложный", "Очень сложный", "Экстремальный"];

const dateFmt = new Intl.DateTimeFormat("ru", { weekday: "short", day: "numeric", month: "long", hour: "2-digit", minute: "2-digit" });
const when = (iso: string) => dateFmt.format(new Date(iso));
// дата по екатеринбургскому времени (так считает прогноз)
const ymd = (iso: string) => new Date(new Date(iso).getTime() + 5 * 36e5).toISOString().slice(0, 10);

function Difficulty({ level }: { level: number | null }) {
  if (!level) return null;
  return <span class={`difficulty d${level}`} title="Сложность">{"●".repeat(level)}{"○".repeat(5 - level)} {difficulties[level]}</span>;
}

function Seats({ e }: { e: HikeEvent }) {
  if (e.capacity === null) return <span>{e.going_count} идут</span>;
  const free = Math.max(e.capacity - e.going_count, 0);
  return <span>{e.going_count} из {e.capacity}{free === 0 ? " · мест нет" : ` · свободно ${free}`}{e.waitlist_count ? ` · в очереди ${e.waitlist_count}` : ""}</span>;
}

export function HikesPage() {
  const [maxDiff, setMaxDiff] = useState("");
  const list = useLoad(() => get<{ items: HikeEvent[] }>(`/hikes${maxDiff ? `?max_difficulty=${maxDiff}` : ""}`), [maxDiff]);
  return (
    <section>
      <div class="page-head">
        <h1>Походы</h1>
        <a class="primary button-link" href="/hikes/new">Организовать поход</a>
      </div>
      <label class="inline-field">Сложность до
        <select value={maxDiff} onChange={(e) => setMaxDiff((e.target as HTMLSelectElement).value)}>
          <option value="">любой</option>
          {[1, 2, 3, 4, 5].map((d) => <option key={d} value={String(d)}>{difficulties[d]}</option>)}
        </select>
      </label>
      {list.error ? <ErrorLine error={list.error} /> : !list.data ? <Loading /> : list.data.items.length === 0 ? <Empty>Ближайших походов нет. Организуйте свой!</Empty> : (
        <ul class="hike-list">
          {list.data.items.map((e) => (
            <li key={e.id} class="hike-item">
              <a href={`/events/${e.id}`} class="hike-title">{e.title}</a>
              <div class="hike-meta">
                <time dateTime={e.starts_at}>{when(e.starts_at)}</time>
                <Difficulty level={e.difficulty} />
                <Seats e={e} />
                {e.approval && <span class="muted">по заявке</span>}
                {e.my_status && <span class={`pill st-${e.my_status}`}>{statusText(e)}</span>}
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function statusText(e: HikeEvent): string {
  switch (e.my_status) {
    case "going": return "Вы идёте";
    case "pending": return "Заявка на рассмотрении";
    case "waitlist": return `Вы в очереди: ${e.my_waitlist_position ?? "?"}-й`;
    case "maybe": return "Возможно";
  }
  return "";
}

export function EventPage({ id }: { id: number }) {
  const me = useMe();
  const ev = useLoad(() => get<HikeEvent>(`/events/${id}`), [id]);
  const route = useLoad(() => (ev.data?.route_id ? get<Route>(`/routes/${ev.data.route_id}`) : Promise.resolve(null)), [ev.data?.route_id]);
  const going = useLoad(() => get<Page<UserCard>>(`/events/${id}/attendees?status=going&limit=100`), [id, ev.data]);
  const queue = useLoad(() => get<Page<UserCard>>(`/events/${id}/attendees?status=waitlist&limit=100`), [id, ev.data]);
  const [note, setNote] = useState("");
  const [error, setError] = useState<unknown>(null);
  const [editing, setEditing] = useState(false);
  if (ev.error) return <ErrorLine error={ev.error} />;
  if (!ev.data || !me) return <Loading />;
  const e = ev.data;
  const isOrganizer = e.creator_id === me.id;
  const act = async (fn: () => Promise<unknown>) => { setError(null); try { await fn(); ev.reload(); } catch (err) { setError(err); } };

  if (editing) return <HikeForm initial={e} onDone={() => { setEditing(false); ev.reload(); }} />;
  return (
    <section class="event">
      <p class="muted"><a href="/hikes">← Походы</a></p>
      <h1>{e.title}</h1>
      <dl class="facts">
        <div><dt>Когда</dt><dd><time dateTime={e.starts_at}>{when(e.starts_at)}</time>{e.ends_at && <> — <time dateTime={e.ends_at}>{when(e.ends_at)}</time></>}</dd></div>
        {e.difficulty && <div><dt>Сложность</dt><dd><Difficulty level={e.difficulty} /></dd></div>}
        <div><dt>Участники</dt><dd><Seats e={e} /></dd></div>
        {e.meeting_point && <div><dt>Место сбора</dt><dd>{e.meeting_point}</dd></div>}
        {e.place && <div><dt>Где</dt><dd>{e.place}</dd></div>}
        {e.community && <div><dt>Сообщество</dt><dd><a href={`/c/${e.community.slug}`}>{e.community.name}</a></dd></div>}
      </dl>

      <div class="event-actions">
        {e.my_status && <span class={`pill st-${e.my_status}`}>{statusText(e)}</span>}
        {!e.my_status && (
          <form class="join" onSubmit={(x) => { x.preventDefault(); act(() => post(`/events/${id}/rsvp`, { status: "going", note })); }}>
            {e.approval && <label>Пара слов организатору: опыт, снаряжение
              <textarea value={note} maxLength={500} rows={2} onInput={(x) => setNote((x.target as HTMLTextAreaElement).value)} />
            </label>}
            <button class="primary">{e.approval ? "Подать заявку" : e.capacity !== null && e.going_count >= e.capacity ? "Встать в очередь" : "Иду"}</button>
          </form>
        )}
        {e.my_status && !isOrganizer && <button class="ghost" onClick={() => act(() => del(`/events/${id}/rsvp`))}>
          {e.my_status === "pending" ? "Отозвать заявку" : e.my_status === "waitlist" ? "Выйти из очереди" : "Не пойду"}
        </button>}
        <a class="ghost button-link" href={`/events/${id}/print`}>Лист для печати</a>
        {isOrganizer && me.plus && <button class="ghost" onClick={() => saveFile(`/events/${id}/participants.csv`, `участники-${id}.csv`).catch(setError)}>Список группы (CSV)</button>}
        {isOrganizer && <button class="ghost" onClick={() => setEditing(true)}>Изменить</button>}
        {isOrganizer && <button class="ghost danger" onClick={() => confirm("Удалить поход? Участники его больше не увидят.") && act(async () => { await del(`/events/${id}`); navigate("/hikes"); })}>Удалить</button>}
      </div>
      <ErrorLine error={error} />

      {e.description && <div class="event-text"><RichText text={e.description} /></div>}
      {e.gear && (
        <section class="gear">
          <h2>Снаряжение</h2>
          <ul>{e.gear.split("\n").map((g) => g.trim()).filter(Boolean).map((g, i) => <li key={i}>{g}</li>)}</ul>
        </section>
      )}
      {route.data && <section><h2>Маршрут</h2><RouteCard route={route.data} /></section>}
      {route.data && route.data.preview.length > 0 && new Date(e.starts_at).getTime() - Date.now() < 10 * 864e5 && (e.ends_at ?? e.starts_at) >= new Date().toISOString().slice(0, 10) && (
        <section><h2>Погода</h2><Weather lat={route.data.preview[0][0]} lon={route.data.preview[0][1]} elevation={route.data.max_ele}
          from={ymd(e.starts_at)} to={ymd(e.ends_at ?? e.starts_at)} /></section>
      )}

      {e.kind === "hike" && e.control_at && <Control e={e} canMark={e.my_status === "going" || isOrganizer} onChange={ev.reload} />}

      <h2>Подвоз</h2>
      <Rides eventId={e.id} meeting={e.meeting_point} />

      {isOrganizer && e.pending_count > 0 && <Applications id={id} onChange={ev.reload} />}

      <h2>Идут</h2>
      {going.data ? <People list={going.data.items} canRemove={isOrganizer} creator={e.creator_id} onRemove={(u) => act(() => del(`/events/${id}/attendees/${u.id}`))} /> : <Loading />}
      {queue.data && queue.data.items.length > 0 && <>
        <h2>Лист ожидания</h2>
        <People list={queue.data.items} canRemove={isOrganizer} creator={e.creator_id} onRemove={(u) => act(() => del(`/events/${id}/attendees/${u.id}`))} />
      </>}
    </section>
  );
}

// Control — контрольный срок: «вернулись» и доверенные контакты участника.
function Control({ e, canMark, onChange }: { e: HikeEvent; canMark: boolean; onChange: () => void }) {
  const going = e.my_status === "going";
  const contacts = useLoad(() => (going && !e.returned_at ? get<{ items: Contact[] }>(`/events/${e.id}/contacts`) : Promise.resolve({ items: [] as Contact[] })), [e.id, going, e.returned_at]);
  const [f, setF] = useState({ name: "", email: "", consent: false });
  const [error, setError] = useState<unknown>(null);
  const run = async (fn: () => Promise<unknown>, after: () => void) => { setError(null); try { await fn(); after(); } catch (err) { setError(err); } };
  const open = !e.returned_at;
  return (
    <section class={`control ${e.returned_at ? "ok" : e.alert_sent ? "alert" : ""}`}>
      <h2>Контрольный срок</h2>
      <p><strong><time dateTime={e.control_at!}>{when(e.control_at!)}</time></strong>
        {e.returned_at ? <> — группа вернулась <time dateTime={e.returned_at}>{when(e.returned_at)}</time>.</>
          : e.alert_sent ? " — срок прошёл, доверенным контактам отправлены письма. Отметьте возвращение, как только будет связь." : null}</p>
      {open && <p class="muted">Если к сроку никто из группы не нажмёт «Мы вернулись», доверенные контакты участников получат письмо: что сообщить по номеру 112. Контакты видите только вы; они удаляются после возвращения или через 7 дней после срока.</p>}
      {open && canMark && <button class="primary" onClick={() => run(() => post(`/events/${e.id}/returned`), onChange)}>Мы вернулись</button>}
      {open && going && !e.alert_sent && contacts.data && (
        <div class="contacts">
          <h3>Мои доверенные контакты</h3>
          {contacts.data.items.length === 0 ? <p class="muted">Пока нет. Добавьте того, кто будет знать, что вы в походе.</p> : (
            <ul class="people">
              {contacts.data.items.map((c) => (
                <li key={c.id}><span class="grow">{c.name} · <span class="muted">{c.email}</span></span>
                  <button class="link" onClick={() => run(() => del(`/events/${e.id}/contacts/${c.id}`), contacts.reload)}>Удалить</button></li>
              ))}
            </ul>
          )}
          {contacts.data.items.length < 3 && (
            <form class="stack" onSubmit={(x) => { x.preventDefault(); run(() => post(`/events/${e.id}/contacts`, f), () => { setF({ name: "", email: "", consent: false }); contacts.reload(); }); }}>
              <div class="row-2">
                <label>Кто это<input value={f.name} required maxLength={80} placeholder="Мама" onInput={(x) => setF({ ...f, name: (x.target as HTMLInputElement).value })} /></label>
                <label>Почта<input type="email" value={f.email} required onInput={(x) => setF({ ...f, email: (x.target as HTMLInputElement).value })} /></label>
              </div>
              <label class="check"><input type="checkbox" checked={f.consent} required onChange={(x) => setF({ ...f, consent: (x.target as HTMLInputElement).checked })} /> Этот человек знает о походе и согласен получить письмо</label>
              <button class="ghost">Добавить контакт</button>
            </form>
          )}
        </div>
      )}
      <ErrorLine error={error} />
    </section>
  );
}

function People({ list, canRemove, creator, onRemove }: { list: UserCard[]; canRemove: boolean; creator: number; onRemove: (u: UserCard) => void }) {
  if (list.length === 0) return <Empty>Пока никого.</Empty>;
  return (
    <ul class="people">
      {list.map((u) => (
        <li key={u.id}>
          <Avatar user={u} size={32} /> <UserLink user={u} />
          {u.id === creator && <span class="muted"> · организатор</span>}
          {canRemove && u.id !== creator && <button class="link" onClick={() => confirm("Убрать участника из похода?") && onRemove(u)}>Убрать</button>}
        </li>
      ))}
    </ul>
  );
}

function Applications({ id, onChange }: { id: number; onChange: () => void }) {
  const apps = useLoad(() => get<{ items: Application[] }>(`/events/${id}/applications`), [id]);
  const [error, setError] = useState<unknown>(null);
  const decide = async (u: UserCard, approve: boolean) => {
    setError(null);
    try { await post(`/events/${id}/applications/${u.id}`, { approve }); apps.reload(); onChange(); } catch (e) { setError(e); }
  };
  if (!apps.data) return <Loading />;
  return (
    <section class="applications">
      <h2>Заявки</h2>
      <ErrorLine error={error} />
      <ul class="people">
        {apps.data.items.map((a) => (
          <li key={a.user.id}>
            <Avatar user={a.user} size={32} />
            <span class="grow"><UserLink user={a.user} /> <Time iso={a.created_at} />{a.note && <><br /><span class="muted">{a.note}</span></>}</span>
            <button class="primary" onClick={() => decide(a.user, true)}>Принять</button>
            <button class="ghost" onClick={() => decide(a.user, false)}>Отклонить</button>
          </li>
        ))}
      </ul>
    </section>
  );
}

const localInput = (iso?: string) => {
  if (!iso) return "";
  const d = new Date(iso);
  return new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
};

export function HikeForm({ initial, onDone }: { initial?: HikeEvent; onDone?: () => void }) {
  const routes = useLoad(() => get<Page<Route>>("/routes?limit=100"), []);
  const [f, setF] = useState({
    title: initial?.title ?? "", starts_at: localInput(initial?.starts_at), ends_at: localInput(initial?.ends_at),
    difficulty: initial?.difficulty ? String(initial.difficulty) : "", capacity: initial?.capacity ? String(initial.capacity) : "",
    meeting_point: initial?.meeting_point ?? "", gear: initial?.gear ?? "", description: initial?.description ?? "",
    route_id: initial?.route_id ?? "", approval: initial?.approval ?? false, control_at: localInput(initial?.control_at ?? undefined),
  });
  const [error, setError] = useState<unknown>(null);
  const set = (k: keyof typeof f) => (e: Event) => {
    const t = e.target as HTMLInputElement;
    setF({ ...f, [k]: t.type === "checkbox" ? t.checked : t.value });
  };
  async function submit(e: Event) {
    e.preventDefault();
    setError(null);
    const body = {
      title: f.title, starts_at: new Date(f.starts_at).toISOString(), ends_at: f.ends_at ? new Date(f.ends_at).toISOString() : null,
      difficulty: f.difficulty ? +f.difficulty : null, capacity: f.capacity ? +f.capacity : null,
      meeting_point: f.meeting_point, gear: f.gear, description: f.description, route_id: f.route_id || null, approval: f.approval,
      ...(initial?.alert_sent ? {} : { control_at: f.control_at ? new Date(f.control_at).toISOString() : null }),
    };
    try {
      if (initial) { await patch(`/events/${initial.id}`, body); onDone?.(); }
      else { const ev = await post<HikeEvent>("/events", { ...body, kind: "hike" }); navigate(`/events/${ev.id}`); }
    } catch (err) { setError(err); }
  }
  return (
    <section>
      <h1>{initial ? "Изменить поход" : "Новый поход"}</h1>
      <form class="stack hike-form" onSubmit={submit}>
        <label>Название<input value={f.title} required maxLength={200} onInput={set("title")} placeholder="Таганай: Двуглавая сопка" /></label>
        <div class="row-2">
          <label>Начало<input type="datetime-local" value={f.starts_at} required onInput={set("starts_at")} /></label>
          <label>Окончание<input type="datetime-local" value={f.ends_at} onInput={set("ends_at")} /></label>
        </div>
        <div class="row-2">
          <label>Сложность
            <select value={f.difficulty} onChange={set("difficulty")}>
              <option value="">не указана</option>
              {[1, 2, 3, 4, 5].map((d) => <option key={d} value={String(d)}>{difficulties[d]}</option>)}
            </select>
          </label>
          <label>Мест (с организатором)<input type="number" min={1} max={500} value={f.capacity} onInput={set("capacity")} placeholder="без ограничения" /></label>
        </div>
        <label>Контрольный срок возвращения<input type="datetime-local" value={f.control_at} disabled={initial?.alert_sent} onInput={set("control_at")} />
          <span class="hint">Не отметитесь к этому времени — доверенным контактам участников уйдёт письмо</span></label>
        <label>Место сбора<input value={f.meeting_point} maxLength={300} onInput={set("meeting_point")} placeholder="Ж/д вокзал Златоуста, 7:00" /></label>
        <label>Маршрут
          <select value={f.route_id} onChange={set("route_id")}>
            <option value="">без маршрута</option>
            {routes.data?.items.map((r) => <option key={r.id} value={r.id}>{r.title}</option>)}
          </select>
        </label>
        <label>Снаряжение — по пункту в строке<textarea rows={4} value={f.gear} maxLength={2000} onInput={set("gear")} placeholder={"Треккинговые ботинки\nДождевик\n2 л воды"} /></label>
        <label>Описание<textarea rows={4} value={f.description} maxLength={5000} onInput={set("description")} /></label>
        <label class="check"><input type="checkbox" checked={f.approval} onChange={set("approval")} /> Участие по заявке: я одобряю каждого</label>
        <ErrorLine error={error} />
        <div class="row">
          <button class="primary">{initial ? "Сохранить" : "Создать поход"}</button>
          {initial && <button type="button" class="ghost" onClick={onDone}>Отмена</button>}
        </div>
      </form>
    </section>
  );
}
