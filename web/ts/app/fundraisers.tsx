// Сборы сообществ: цель, срок, прогресс, пожертвования монетами.
import { h } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { get, post, UserCard } from "./api.js";
import { Empty, ErrorLine, Loading, UserLink, useLoad } from "./ui.js";

interface Fund {
  id: number; community: { id: number; slug: string; name: string }; title: string; description: string;
  goal: number; raised: number; donors: number; deadline: string; open: boolean;
}
interface Donation { user: UserCard | null; amount: number; message: string; created_at: string }

const nf = new Intl.NumberFormat("ru");
const dayFmt = new Intl.DateTimeFormat("ru", { day: "numeric", month: "long" });
const newKey = () => (crypto.randomUUID ? crypto.randomUUID() : `${Date.now()}-${Math.random().toString(36).slice(2)}`);

function FundCard({ f, onChange, showCommunity }: { f: Fund; onChange: () => void; showCommunity?: boolean }) {
  const [open, setOpen] = useState(false);
  const [amount, setAmount] = useState("100");
  const [memo, setMemo] = useState("");
  const [anon, setAnon] = useState(false);
  const [key, setKey] = useState(newKey);
  const [error, setError] = useState<unknown>(null);
  const [thanks, setThanks] = useState(false);
  const donations = useLoad(() => (open ? get<{ donations: Donation[] }>(`/fundraisers/${f.id}`) : Promise.resolve({ donations: [] as Donation[] })), [open, f.raised]);
  const pct = Math.min(100, Math.round((f.raised / f.goal) * 100));
  const donate = async (e: Event) => {
    e.preventDefault(); setError(null);
    try { await post(`/fundraisers/${f.id}/donate`, { amount: +amount, memo, anonymous: anon, idempotency_key: key }); setKey(newKey()); setThanks(true); onChange(); } catch (err) { setError(err); }
  };
  return (
    <li class="fund">
      <strong class="fund-title">{f.title}</strong>
      {showCommunity && <span class="muted"> · <a href={`/c/${f.community.slug}`}>{f.community.name}</a></span>}
      {f.description && <p class="ad-body">{f.description}</p>}
      <div class="progress" role="progressbar" aria-valuemin={0} aria-valuemax={f.goal} aria-valuenow={f.raised} aria-label="Собрано">
        <div class="progress-bar" style={{ width: `${pct}%` }} />
      </div>
      <p class="muted">Собрано <strong>{nf.format(f.raised)}</strong> из {nf.format(f.goal)} монет ({pct}%) · {f.donors} чел. · {f.open ? `до ${dayFmt.format(new Date(f.deadline))}` : "сбор завершён"}</p>
      <div class="row tight">
        {f.open && !thanks && <button class="primary" onClick={() => setOpen(!open)}>Поддержать</button>}
        {thanks && <span class="pill st-going">Спасибо!</span>}
        {!f.open && <button class="link" onClick={() => setOpen(!open)}>Кто поддержал</button>}
      </div>
      {open && f.open && !thanks && (
        <form class="row" onSubmit={donate}>
          <label>Монет<input type="number" min={1} value={amount} onInput={(e) => setAmount((e.target as HTMLInputElement).value)} /></label>
          <label>Пожелание<input value={memo} maxLength={200} onInput={(e) => setMemo((e.target as HTMLInputElement).value)} /></label>
          <label class="check"><input type="checkbox" checked={anon} onChange={(e) => setAnon((e.target as HTMLInputElement).checked)} /> анонимно</label>
          <button class="primary">Отправить</button>
        </form>
      )}
      {open && donations.data && donations.data.donations.length > 0 && (
        <ul class="plain-list donations">
          {donations.data.donations.map((d, i) => <li key={i}>{d.user ? <UserLink user={d.user} /> : <span class="muted">Аноним</span>} — {nf.format(d.amount)}{d.message && <span class="muted"> · {d.message}</span>}</li>)}
        </ul>
      )}
      <ErrorLine error={error} />
    </li>
  );
}

export function CommunityFundraisers({ communityId, admin }: { communityId: number; admin: boolean }) {
  const list = useLoad(() => get<{ items: Fund[] }>(`/communities/${communityId}/fundraisers`), [communityId]);
  const [f, setF] = useState({ title: "", description: "", goal: "", deadline: "" });
  const [error, setError] = useState<unknown>(null);
  if (!list.data) return list.error ? <ErrorLine error={list.error} /> : null;
  if (list.data.items.length === 0 && !admin) return null;
  const set = (k: keyof typeof f) => (e: Event) => setF({ ...f, [k]: (e.target as HTMLInputElement).value });
  return (
    <section class="fundraisers">
      <h2>Сборы</h2>
      {list.data.items.length > 0 && <ul class="ads">{list.data.items.map((x) => <FundCard key={x.id} f={x} onChange={list.reload} />)}</ul>}
      {admin && (
        <details class="create">
          <summary>Объявить сбор</summary>
          <form class="stack hike-form" onSubmit={(e) => {
            e.preventDefault(); setError(null);
            post("/communities/" + communityId + "/fundraisers", { title: f.title, description: f.description, goal: +f.goal, deadline: new Date(f.deadline).toISOString() })
              .then(() => { setF({ title: "", description: "", goal: "", deadline: "" }); list.reload(); }, setError);
          }}>
            <label>На что<input value={f.title} required maxLength={120} onInput={set("title")} placeholder="Рации для клуба" /></label>
            <label>Подробности<textarea rows={2} maxLength={2000} value={f.description} onInput={set("description")} /></label>
            <div class="row-2">
              <label>Цель, монет<input type="number" min={1} required value={f.goal} onInput={set("goal")} /></label>
              <label>До<input type="datetime-local" required value={f.deadline} onInput={set("deadline")} /></label>
            </div>
            <ErrorLine error={error} />
            <button class="primary">Объявить</button>
          </form>
        </details>
      )}
    </section>
  );
}

export function FundraisersPage() {
  const list = useLoad(() => get<{ items: Fund[] }>("/fundraisers"), []);
  return (
    <section>
      <h1>Сборы</h1>
      <p class="muted">Клубы собирают на снаряжение, спасфонд и экологические акции. Пожертвования — монетами на счёт сообщества.</p>
      {list.error ? <ErrorLine error={list.error} /> : !list.data ? <Loading /> : list.data.items.length === 0 ? <Empty>Открытых сборов нет.</Empty>
        : <ul class="ads">{list.data.items.map((x) => <FundCard key={x.id} f={x} onChange={list.reload} showCommunity />)}</ul>}
    </section>
  );
}
