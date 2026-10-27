// Витрина для гостей под формой входа: ближайшие походы и маршруты
// каталога (GET /showcase, без входа).
import { h, Fragment } from "../vendor/preact/index.js";
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { UserCard, fullName } from "./api.js";

interface Hike { id: number; title: string; place: string; starts_at: string; difficulty: number | null; capacity: number | null; going_count: number }
interface RouteHit { id: string; title: string; distance_m: number; gain_m: number; author: UserCard; url: string }

const dateFmt = new Intl.DateTimeFormat("ru-RU", { day: "numeric", month: "long", weekday: "short" });
const levels = ["", "Лёгкий", "Несложный", "Средний", "Трудный", "Очень трудный"];

export function Showcase() {
  const [d, setD] = useState<{ hikes: Hike[]; routes: RouteHit[] } | null>(null);
  useEffect(() => { fetch("/api/v1/showcase").then((r) => (r.ok ? r.json() : null)).then(setD, () => {}); }, []);
  return (
    <section class="showcase" aria-label="Что есть на сайте">
      <ul class="showcase-points">
        <li><strong>Походы</strong> с местами, заявками и листом ожидания</li>
        <li><strong>Попутчики и подвоз</strong> до Таганая, Иремеля, Зюраткуля</li>
        <li><strong>Маршруты</strong> с профилем высот, родниками и стоянками</li>
        <li><strong>Контрольный срок</strong> — если группа не вернулась, письмо доверенным людям</li>
      </ul>
      {d && d.hikes.length > 0 && <>
        <h2>Ближайшие походы</h2>
        <ul class="showcase-hikes">
          {d.hikes.map((x) => (
            <li key={x.id}>
              <a href={`/events/${x.id}`}>{x.title}</a>
              <span class="muted">{dateFmt.format(new Date(x.starts_at))}{x.place ? ` · ${x.place}` : ""}</span>
              <span class="muted">{x.difficulty ? `${levels[x.difficulty]} · ` : ""}идут {x.going_count}{x.capacity ? ` из ${x.capacity}` : ""}</span>
            </li>
          ))}
        </ul>
      </>}
      {d && d.routes.length > 0 && <>
        <h2>Маршруты</h2>
        <ul class="plain-list">
          {d.routes.map((r) => (
            <li key={r.id}><a href={r.url}>{r.title}</a> <span class="muted">{(r.distance_m / 1000).toFixed(1)} км, набор {r.gain_m} м · {fullName(r.author)}</span></li>
          ))}
        </ul>
      </>}
      <p><a href="/trails">Весь каталог маршрутов →</a></p>
    </section>
  );
}
