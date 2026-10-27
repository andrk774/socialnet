// Лист маршрута для печати (и сохранения в PDF через «Печать» браузера):
// карта, профиль высот, точки с координатами, сведения о походе, снаряжение
// списком с квадратиками, телефоны экстренных служб.
import { Fragment, h } from "../vendor/preact/index.js";
import { get, Route } from "./api.js";
import { HikeEvent, difficulties } from "./hikes.js";
import { ElevationProfile, hours, km, RouteMap } from "./routes.js";
import { ErrorLine, Loading, useLoad } from "./ui.js";

const dateFmt = new Intl.DateTimeFormat("ru", { weekday: "long", day: "numeric", month: "long", year: "numeric", hour: "2-digit", minute: "2-digit" });
const nf0 = new Intl.NumberFormat("ru", { maximumFractionDigits: 0 });

function dms(v: number, pos: string, neg: string): string {
  const a = Math.abs(v), d = Math.floor(a), mf = (a - d) * 60, m = Math.floor(mf), s = (mf - m) * 60;
  return `${d}°${String(m).padStart(2, "0")}′${s.toFixed(1).padStart(4, "0")}″ ${v >= 0 ? pos : neg}`;
}
export const coords = (lat: number, lon: number) => `${dms(lat, "с. ш.", "ю. ш.")}, ${dms(lon, "в. д.", "з. д.")}`;

function dist(a: [number, number], b: [number, number]): number {
  const R = 6371000, rad = Math.PI / 180;
  const dLat = (b[0] - a[0]) * rad, dLon = (b[1] - a[1]) * rad;
  const x = Math.sin(dLat / 2) ** 2 + Math.cos(a[0] * rad) * Math.cos(b[0] * rad) * Math.sin(dLon / 2) ** 2;
  return 2 * R * Math.asin(Math.sqrt(x));
}

// alongTrack — сколько пройти от старта до ближайшей к точке вершины трека.
export function alongTrack(segs: [number, number][][], p: [number, number]): number {
  let best = Infinity, at = 0, run = 0;
  for (const seg of segs) {
    for (let i = 0; i < seg.length; i++) {
      if (i > 0) run += dist(seg[i - 1], seg[i]);
      const d = dist(seg[i], p);
      if (d < best) { best = d; at = run; }
    }
  }
  return at;
}

export function PrintPage({ routeId, eventId }: { routeId?: string; eventId?: number }) {
  const ev = useLoad(() => (eventId ? get<HikeEvent>(`/events/${eventId}`) : Promise.resolve(null)), [eventId]);
  const rid = routeId ?? ev.data?.route_id ?? undefined;
  const route = useLoad(() => (rid ? get<Route>(`/routes/${rid}`) : Promise.resolve(null)), [rid]);
  if (ev.error || route.error) return <ErrorLine error={ev.error || route.error} />;
  if ((eventId && !ev.data) || (rid && !route.data)) return <Loading />;
  const e = ev.data, r = route.data;
  const segs = r ? (r.detail?.length ? r.detail : [r.preview]) : [];
  const start = segs[0]?.[0], lastSeg = segs[segs.length - 1], finish = lastSeg?.[lastSeg.length - 1];
  const gear = (e?.gear ?? "").split("\n").map((g) => g.trim()).filter(Boolean);
  return (
    <article class="print-sheet">
      <div class="print-actions no-print">
        <button class="primary" onClick={() => window.print()}>Распечатать или сохранить в PDF</button>
        <a href={e ? `/events/${e.id}` : "/"}>← Назад</a>
      </div>
      <header>
        <h1>{e?.title ?? r?.title}</h1>
        {e && <p class="print-when">{dateFmt.format(new Date(e.starts_at))}{e.ends_at && <> — {dateFmt.format(new Date(e.ends_at))}</>}</p>}
      </header>

      {r && <>
        <dl class="facts print-facts">
          <div><dt>Маршрут</dt><dd>{r.title}</dd></div>
          <div><dt>Расстояние</dt><dd>{km(r.distance_m)}</dd></div>
          {r.max_ele !== null && <div><dt>Набор / сброс</dt><dd>↑ {nf0.format(r.gain_m)} м / ↓ {nf0.format(r.loss_m)} м</dd></div>}
          {r.max_ele !== null && <div><dt>Высоты</dt><dd>{nf0.format(r.min_ele ?? 0)}–{nf0.format(r.max_ele)} м</dd></div>}
          {r.moving_sec > 0 && <div><dt>В движении (по треку)</dt><dd>{hours(r.moving_sec)}</dd></div>}
          {e?.difficulty && <div><dt>Сложность</dt><dd>{difficulties[e.difficulty]}</dd></div>}
        </dl>
        <RouteMap route={r} printMode />
        {r.profile.length > 1 && <div class="print-profile"><ElevationProfile profile={r.profile} /></div>}
        <h2>Точки</h2>
        <table class="print-table">
          <thead><tr><th>№</th><th>Точка</th><th>Координаты</th><th>Высота</th><th>От старта</th></tr></thead>
          <tbody>
            {start && <tr><td>С</td><td>Старт{r.hidden_ends_m > 0 ? ` (≈${r.hidden_ends_m} м скрыты)` : ""}</td><td>{coords(start[0], start[1])}</td><td>—</td><td>0</td></tr>}
            {(r.waypoints ?? []).map((w, i) => (
              <tr key={i}><td>{i + 1}</td><td>{w.name || "—"}</td><td>{coords(w.lat, w.lon)}<br /><span class="muted">{w.lat.toFixed(5)}, {w.lon.toFixed(5)}</span></td>
                <td>{w.ele !== undefined && w.ele !== null ? `${nf0.format(w.ele)} м` : "—"}</td><td>{km(alongTrack(segs, [w.lat, w.lon]))}</td></tr>
            ))}
            {finish && <tr><td>Ф</td><td>Финиш</td><td>{coords(finish[0], finish[1])}</td><td>—</td><td>{km(r.distance_m)}</td></tr>}
          </tbody>
        </table>
      </>}

      {e && <>
        <h2>Поход</h2>
        <dl class="facts print-facts">
          {e.meeting_point && <div><dt>Место сбора</dt><dd>{e.meeting_point}</dd></div>}
          {e.place && <div><dt>Где</dt><dd>{e.place}</dd></div>}
          <div><dt>Участников</dt><dd>{e.going_count}{e.capacity !== null ? ` из ${e.capacity}` : ""}</dd></div>
          {e.control_at && <div><dt>Контрольный срок</dt><dd>{dateFmt.format(new Date(e.control_at))}</dd></div>}
        </dl>
        {gear.length > 0 && <>
          <h2>Снаряжение</h2>
          <ul class="checklist">{gear.map((g, i) => <li key={i}><span class="box" aria-hidden="true" /> {g}</li>)}</ul>
        </>}
      </>}

      <section class="print-emergency">
        <h2>Экстренная связь</h2>
        <p><strong>112</strong> — единый номер экстренных служб (работает без SIM-карты и при нуле на счёте, если есть сеть любого оператора).</p>
        <p>Сообщите: кто вы и сколько вас, что случилось, где находитесь (координаты из таблицы или с телефона), номер для обратной связи. Берегите заряд: держите телефон в тепле, выключайте лишнее.</p>
        <p class="muted">Перед выходом оставьте этот лист и контрольный срок близким. Сведения о маршруте опубликованы пользователями socialnet и могут быть неточными.</p>
      </section>
    </article>
  );
}
