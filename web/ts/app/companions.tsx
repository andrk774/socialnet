// «Ищу попутчиков»: поиск объявлений по району, датам и сложности, отклик
// личным сообщением, свои объявления.
import { Fragment, h } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { del, get, Page, patch, post, Route, UserCard } from "./api.js";
import { difficulties } from "./hikes.js";
import { useMe } from "./session.js";
import { Avatar, Empty, ErrorLine, Loading, UserLink, useLoad } from "./ui.js";
import { ReportButton } from "./report.js";

interface Ad {
  id: number; author: UserCard; region: string; title: string; body: string; date_from: string; date_to: string;
  difficulty: number | null; seats: number; route_id: string | null; closed: boolean; created_at: string;
}

const dayFmt = new Intl.DateTimeFormat("ru", { day: "numeric", month: "long", weekday: "short" });
const day = (d: string) => dayFmt.format(new Date(d + "T12:00:00"));
const dates = (a: Ad) => (a.date_from === a.date_to ? day(a.date_from) : `${day(a.date_from)} — ${day(a.date_to)}`);
const today = () => new Date(Date.now() - new Date().getTimezoneOffset() * 60000).toISOString().slice(0, 10);

export function CompanionsPage() {
  const me = useMe();
  const [filter, setFilter] = useState({ region: "", from: "", to: "", max_difficulty: "" });
  const [applied, setApplied] = useState(filter);
  const qs = new URLSearchParams(Object.entries(applied).filter(([, v]) => v)).toString();
  const list = useLoad(() => get<{ items: Ad[] }>(`/companions${qs ? "?" + qs : ""}`), [qs]);
  const mine = useLoad(() => get<{ items: Ad[] }>("/companions/mine"), []);
  const setF = (k: keyof typeof filter) => (e: Event) => setFilter({ ...filter, [k]: (e.target as HTMLInputElement).value });
  const reloadAll = () => { list.reload(); mine.reload(); };
  return (
    <section>
      <h1>Ищу попутчиков</h1>
      <form class="filters" onSubmit={(e) => { e.preventDefault(); setApplied(filter); }}>
        <label>Район<input value={filter.region} maxLength={80} placeholder="Таганай, Иремель…" onInput={setF("region")} /></label>
        <label>С<input type="date" value={filter.from} onInput={setF("from")} /></label>
        <label>По<input type="date" value={filter.to} onInput={setF("to")} /></label>
        <label>Сложность до
          <select value={filter.max_difficulty} onChange={setF("max_difficulty")}>
            <option value="">любая</option>
            {[1, 2, 3, 4, 5].map((d) => <option key={d} value={String(d)}>{difficulties[d]}</option>)}
          </select>
        </label>
        <button class="primary">Найти</button>
      </form>
      {list.error ? <ErrorLine error={list.error} /> : !list.data ? <Loading /> : list.data.items.length === 0 ? <Empty>Объявлений не нашлось. Разместите своё — вас найдут.</Empty> : (
        <ul class="ads">
          {list.data.items.map((a) => <AdCard key={a.id} ad={a} mine={a.author.id === me?.id} onChange={reloadAll} />)}
        </ul>
      )}
      <NewAd onCreated={reloadAll} />
      {mine.data && mine.data.items.length > 0 && <>
        <h2>Мои объявления</h2>
        <ul class="ads">{mine.data.items.map((a) => <AdCard key={a.id} ad={a} mine onChange={reloadAll} />)}</ul>
      </>}
    </section>
  );
}

function AdCard({ ad, mine, onChange }: { ad: Ad; mine: boolean; onChange: () => void }) {
  const [open, setOpen] = useState(false);
  const [text, setText] = useState("");
  const [sent, setSent] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const run = async (fn: () => Promise<unknown>) => { setError(null); try { await fn(); onChange(); } catch (e) { setError(e); } };
  async function reply(e: Event) {
    e.preventDefault();
    setError(null);
    try { await post(`/companions/${ad.id}/reply`, { text }); setSent(true); setOpen(false); setText(""); } catch (err) { setError(err); }
  }
  return (
    <li class={ad.closed ? "ad closed" : "ad"}>
      <div class="ad-head">
        <Avatar user={ad.author} size={36} />
        <div class="grow">
          <strong class="ad-title">{ad.title}</strong>
          <div class="hike-meta">
            <span>{ad.region}</span><span>{dates(ad)}</span>
            {ad.difficulty && <span class={`difficulty d${ad.difficulty}`}>{difficulties[ad.difficulty]}</span>}
            <span>нужно: {ad.seats}</span>
            {ad.closed && <span class="pill">закрыто</span>}
          </div>
        </div>
      </div>
      {ad.body && <p class="ad-body">{ad.body}</p>}
      <div class="row tight">
        {!mine && <span class="muted">Автор: <UserLink user={ad.author} /></span>}
        {!mine && !ad.closed && !sent && <button class="ghost" onClick={() => setOpen(!open)} aria-expanded={open}>Откликнуться</button>}
        {sent && <span class="muted">Отклик отправлен — переписка в <a href={`/im/u/${ad.author.id}`}>Сообщениях</a>.</span>}
        {mine && <button class="ghost" onClick={() => run(() => patch(`/companions/${ad.id}`, { closed: !ad.closed }))}>{ad.closed ? "Открыть снова" : "Нашёл попутчиков"}</button>}
        {mine && <button class="ghost danger" onClick={() => run(() => del(`/companions/${ad.id}`))}>Удалить</button>}
        {!mine && <ReportButton target={{ type: "companion", id: ad.id }} />}
      </div>
      {open && (
        <form class="reply" onSubmit={reply}>
          <label>Сообщение автору<textarea rows={3} required maxLength={1000} value={text} placeholder="Опыт, снаряжение, откуда едете" onInput={(e) => setText((e.target as HTMLTextAreaElement).value)} /></label>
          <button class="primary">Отправить</button>
        </form>
      )}
      <ErrorLine error={error} />
    </li>
  );
}

function NewAd({ onCreated }: { onCreated: () => void }) {
  const routes = useLoad(() => get<Page<Route>>("/routes?limit=100"), []);
  const empty = { region: "", title: "", body: "", date_from: "", date_to: "", difficulty: "", seats: "1", route_id: "" };
  const [f, setF] = useState(empty);
  const [error, setError] = useState<unknown>(null);
  const set = (k: keyof typeof f) => (e: Event) => setF({ ...f, [k]: (e.target as HTMLInputElement).value });
  async function submit(e: Event) {
    e.preventDefault();
    setError(null);
    try {
      await post("/companions", {
        region: f.region, title: f.title, body: f.body, date_from: f.date_from, date_to: f.date_to || f.date_from,
        difficulty: f.difficulty ? +f.difficulty : null, seats: +f.seats, route_id: f.route_id || null,
      });
      setF(empty);
      onCreated();
    } catch (err) { setError(err); }
  }
  return (
    <details class="create">
      <summary>Разместить объявление</summary>
      <form class="stack hike-form" onSubmit={submit}>
        <label>Заголовок<input value={f.title} required maxLength={120} onInput={set("title")} placeholder="Ищу компанию на Откликной гребень" /></label>
        <label>Район<input value={f.region} required maxLength={80} onInput={set("region")} placeholder="Таганай" /></label>
        <div class="row-2">
          <label>С<input type="date" value={f.date_from} min={today()} required onInput={set("date_from")} /></label>
          <label>По<input type="date" value={f.date_to} min={f.date_from || today()} onInput={set("date_to")} /></label>
        </div>
        <div class="row-2">
          <label>Сложность
            <select value={f.difficulty} onChange={set("difficulty")}>
              <option value="">не указана</option>
              {[1, 2, 3, 4, 5].map((d) => <option key={d} value={String(d)}>{difficulties[d]}</option>)}
            </select>
          </label>
          <label>Сколько человек ищу<input type="number" min={1} max={20} value={f.seats} onInput={set("seats")} /></label>
        </div>
        <label>Маршрут
          <select value={f.route_id} onChange={set("route_id")}>
            <option value="">без маршрута</option>
            {routes.data?.items.map((r) => <option key={r.id} value={r.id}>{r.title}</option>)}
          </select>
        </label>
        <label>Подробности<textarea rows={3} maxLength={2000} value={f.body} onInput={set("body")} /></label>
        <ErrorLine error={error} />
        <button class="primary">Разместить</button>
      </form>
    </details>
  );
}
