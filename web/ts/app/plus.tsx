// Подписка «Плюс» за монеты.
import { h } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { get, patch, post } from "./api.js";
import { loadMe } from "./session.js";
import { ErrorLine, Loading, useLoad } from "./ui.js";

interface Plus { active: boolean; expires_at: string | null; auto_renew: boolean; price: number; days: number; limits: Record<string, [number, number]> }

const limitNames: Record<string, string> = {
  antennas: "Своих лент", companion_ads: "Открытых объявлений о попутчиках", articles_day: "Статей в сутки",
  offline_tiles: "Тайлов карты на офлайн-маршрут", club_emoji: "Эмодзи в вашем сообществе",
};
const dateFmt = new Intl.DateTimeFormat("ru", { day: "numeric", month: "long", year: "numeric" });
const newKey = () => (crypto.randomUUID ? crypto.randomUUID() : `${Date.now()}-${Math.random().toString(36).slice(2)}`);

export function PlusPage() {
  const st = useLoad(() => get<Plus>("/plus"), []);
  const [key, setKey] = useState(newKey); // один ключ на одну покупку: повтор не спишет дважды
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  if (st.error) return <ErrorLine error={st.error} />;
  if (!st.data) return <Loading />;
  const p = st.data;
  const buy = async () => {
    setBusy(true); setError(null);
    try { await post("/plus/buy", { idempotency_key: key }); setKey(newKey()); st.reload(); await loadMe(); } catch (e) { setError(e); } finally { setBusy(false); }
  };
  return (
    <section class="plus-page">
      <h1>socialnet <span class="plus-badge">Плюс</span></h1>
      {p.active
        ? <p class="note">Подписка действует до {dateFmt.format(new Date(p.expires_at!))}.{" "}
          <label class="check inline-check"><input type="checkbox" checked={p.auto_renew}
            onChange={(e) => patch("/plus", { auto_renew: (e.target as HTMLInputElement).checked }).then(st.reload, setError)} /> продлевать автоматически</label></p>
        : <p>Больше возможностей для походов и клубов — {p.price} монет за {p.days} дней.</p>}
      <table class="plus-table">
        <thead><tr><th></th><th>Обычно</th><th>С «Плюсом»</th></tr></thead>
        <tbody>
          {Object.entries(p.limits).map(([k, [a, b]]) => <tr key={k}><td>{limitNames[k] ?? k}</td><td>{a}</td><td><strong>{b}</strong></td></tr>)}
          <tr><td>Значок в профиле</td><td>—</td><td><span class="plus-badge">Плюс</span></td></tr>
          <tr><td>Список группы CSV для регистрации в МЧС</td><td>—</td><td>✓</td></tr>
        </tbody>
      </table>
      <ErrorLine error={error} />
      <button class="primary" disabled={busy} onClick={buy}>{p.active ? `Продлить на ${p.days} дней — ${p.price} монет` : `Подключить за ${p.price} монет`}</button>
      <p class="hint">Монеты списываются с вашего кошелька. Пополнить — в разделе <a href="/wallet">Кошелёк</a>.</p>
    </section>
  );
}
