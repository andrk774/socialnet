// Подвоз к началу маршрута: водитель предлагает места, попутчики просятся.
// Оплату приложение не принимает — о бензине договариваются в сообщениях.
import { Fragment, h } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { del, get, post, UserCard } from "./api.js";
import { useMe } from "./session.js";
import { Avatar, Empty, ErrorLine, Loading, UserLink, useLoad } from "./ui.js";

interface Ride {
  id: number; driver: UserCard; event_id: number | null; from: string; to: string; departs_at: string;
  seats: number; taken: number; pending: number; note: string; my_status: "pending" | "accepted" | null;
}
interface Req { user: UserCard; status: "pending" | "accepted"; note: string; created_at: string }

const fmt = new Intl.DateTimeFormat("ru", { weekday: "short", day: "numeric", month: "long", hour: "2-digit", minute: "2-digit" });

function RideCard({ ride, onChange }: { ride: Ride; onChange: () => void }) {
  const me = useMe();
  const mine = ride.driver.id === me?.id;
  const [note, setNote] = useState("");
  const [asking, setAsking] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const reqs = useLoad(() => (mine ? get<{ items: Req[] }>(`/rides/${ride.id}/requests`) : Promise.resolve({ items: [] as Req[] })), [ride.id, mine, ride.taken, ride.pending]);
  const run = (fn: () => Promise<unknown>) => { setError(null); fn().then(onChange, setError); };
  const free = ride.seats - ride.taken;
  return (
    <li class="ride" id={`ride-${ride.id}`}>
      <div class="ad-head">
        <Avatar user={ride.driver} size={36} />
        <div class="grow">
          <strong>{ride.from} → {ride.to}</strong>
          <div class="hike-meta"><span>{fmt.format(new Date(ride.departs_at))}</span><span>мест: {free > 0 ? `свободно ${free} из ${ride.seats}` : "нет"}</span>
            <span>водитель <UserLink user={ride.driver} /></span></div>
        </div>
      </div>
      {ride.note && <p class="ad-body">{ride.note}</p>}
      <div class="row tight">
        {ride.my_status === "accepted" && <span class="pill st-going">Вы едете</span>}
        {ride.my_status === "pending" && <span class="pill st-pending">Заявка у водителя</span>}
        {!mine && !ride.my_status && !asking && <button class="ghost" onClick={() => setAsking(true)}>Попроситься</button>}
        {!mine && ride.my_status && <button class="link" onClick={() => run(() => del(`/rides/${ride.id}/request`))}>{ride.my_status === "accepted" ? "Не поеду" : "Отозвать"}</button>}
        {!mine && <a class="link" href={`/im/u/${ride.driver.id}`}>Написать водителю</a>}
        {mine && <button class="link" onClick={() => confirm("Отменить поездку? Попутчики получат уведомление.") && run(() => del(`/rides/${ride.id}`))}>Отменить поездку</button>}
      </div>
      {asking && (
        <form class="reply" onSubmit={(e) => { e.preventDefault(); run(() => post(`/rides/${ride.id}/request`, { note })); setAsking(false); }}>
          <label>Пара слов водителю<input value={note} maxLength={300} onInput={(e) => setNote((e.target as HTMLInputElement).value)} placeholder="Откуда заберёте, сколько вещей" /></label>
          <button class="primary">Отправить</button>
        </form>
      )}
      {mine && reqs.data && reqs.data.items.length > 0 && (
        <ul class="people">
          {reqs.data.items.map((q) => (
            <li key={q.user.id}>
              <Avatar user={q.user} size={28} />
              <span class="grow"><UserLink user={q.user} />{q.status === "accepted" ? " · едет" : ""}{q.note && <><br /><span class="muted">{q.note}</span></>}</span>
              {q.status === "pending" && <button class="primary" disabled={free <= 0} onClick={() => run(() => post(`/rides/${ride.id}/requests/${q.user.id}`, { accept: true }))}>Взять</button>}
              <button class="ghost" onClick={() => run(() => post(`/rides/${ride.id}/requests/${q.user.id}`, { accept: false }))}>{q.status === "accepted" ? "Высадить" : "Отказать"}</button>
            </li>
          ))}
        </ul>
      )}
      <ErrorLine error={error} />
    </li>
  );
}

function OfferRide({ eventId, meeting, onDone }: { eventId?: number; meeting?: string; onDone: () => void }) {
  const [f, setF] = useState({ from: "Миасс, ", to: meeting ?? "", departs_at: "", seats: "3", note: "" });
  const [error, setError] = useState<unknown>(null);
  const set = (k: keyof typeof f) => (e: Event) => setF({ ...f, [k]: (e.target as HTMLInputElement).value });
  return (
    <details class="create">
      <summary>Предложить места в машине</summary>
      <form class="stack hike-form" onSubmit={(e) => {
        e.preventDefault(); setError(null);
        post("/rides", { ...f, seats: +f.seats, departs_at: new Date(f.departs_at).toISOString(), event_id: eventId ?? null })
          .then(() => { setF({ ...f, departs_at: "", note: "" }); onDone(); }, setError);
      }}>
        <div class="row-2">
          <label>Откуда<input value={f.from} required maxLength={150} onInput={set("from")} /></label>
          <label>Куда<input value={f.to} required maxLength={150} onInput={set("to")} placeholder="Таганай, Белый ключ" /></label>
        </div>
        <div class="row-2">
          <label>Отправление<input type="datetime-local" value={f.departs_at} required onInput={set("departs_at")} /></label>
          <label>Свободных мест<input type="number" min={1} max={8} value={f.seats} onInput={set("seats")} /></label>
        </div>
        <label>Комментарий<input value={f.note} maxLength={500} onInput={set("note")} placeholder="Машина, багаж, где забрать" /></label>
        <p class="hint">Оплату приложение не принимает — о складчине на бензин договоритесь в сообщениях.</p>
        <ErrorLine error={error} />
        <button class="primary">Предложить</button>
      </form>
    </details>
  );
}

export function Rides({ eventId, meeting }: { eventId?: number; meeting?: string }) {
  const [to, setTo] = useState("");
  const [q, setQ] = useState("");
  const url = eventId ? `/rides?event_id=${eventId}` : `/rides${q ? `?to=${encodeURIComponent(q)}` : ""}`;
  const list = useLoad(() => get<{ items: Ride[] }>(url), [url]);
  return (
    <section class="rides">
      {!eventId && <form class="row" onSubmit={(e) => { e.preventDefault(); setQ(to); }}>
        <label>Куда<input value={to} onInput={(e) => setTo((e.target as HTMLInputElement).value)} placeholder="Таганай" /></label>
        <button class="primary">Найти</button>
      </form>}
      {list.error ? <ErrorLine error={list.error} /> : !list.data ? <Loading /> : list.data.items.length === 0
        ? <Empty>{eventId ? "Пока никто не предложил места." : "Поездок не нашлось."}</Empty>
        : <ul class="ads">{list.data.items.map((r) => <RideCard key={r.id} ride={r} onChange={list.reload} />)}</ul>}
      <OfferRide eventId={eventId} meeting={meeting} onDone={list.reload} />
    </section>
  );
}

export function RidesPage() {
  return <section><h1>Подвоз к маршрутам</h1><Rides /></section>;
}
